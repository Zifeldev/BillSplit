package ocrworker

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/Zifeldev/Billsplit/internal/parser/receipt"
)

// DefaultClaudeModel is used when ANTHROPIC_MODEL is empty.
const DefaultClaudeModel = "claude-opus-5-5"

// Claude reads the receipt photo with Claude vision and returns structured
// items directly (no text parsing). It's the most accurate provider on
// crumpled, long or low-light receipts, but it's paid — so the worker calls it
// only when the free providers failed or produced a total mismatch.
type Claude struct {
	client anthropic.Client
	model  string
}

func NewClaude(apiKey, model string) *Claude {
	if model == "" {
		model = DefaultClaudeModel
	}
	return &Claude{
		client: anthropic.NewClient(option.WithAPIKey(apiKey), option.WithRequestTimeout(90*time.Second)),
		model:  model,
	}
}

func (c *Claude) Name() string { return "claude" }

const claudePrompt = `Extract the purchase from this receipt (photo or PDF).
Rules:
- One entry per purchased line item, in receipt order. Use the printed line total as price (after quantity), in the receipt's minor units (cents): "3.48" -> 348.
- Apply discounts/rollbacks/coupons printed under an item to that item's price instead of listing them separately.
- Do not include payment lines (VISA, CASH, CHANGE), subtotal, tax or total as items.
- name: readable product name — expand store abbreviations when certain ("GV MILK 2%" -> "Great Value Milk 2%"); otherwise keep the printed text.
- raw_name: the item text exactly as printed. code: the UPC/EAN digits printed on the item line, or empty string.
- subtotal_cents, tax_cents, total_cents: as printed; use 0 if absent.
- purchase_date: YYYY-MM-DD if printed, otherwise empty string. store: merchant name, otherwise empty string.
- If the image is not a receipt, return an empty items list.`

// Strict JSON schema for structured outputs.
var claudeSchema = map[string]any{
	"type":                 "object",
	"additionalProperties": false,
	"required":             []string{"store", "items", "subtotal_cents", "tax_cents", "total_cents", "purchase_date"},
	"properties": map[string]any{
		"store": map[string]any{"type": "string"},
		"items": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"required":             []string{"name", "raw_name", "code", "quantity", "price_cents"},
				"properties": map[string]any{
					"name":        map[string]any{"type": "string"},
					"raw_name":    map[string]any{"type": "string"},
					"code":        map[string]any{"type": "string"},
					"quantity":    map[string]any{"type": "integer"},
					"price_cents": map[string]any{"type": "integer"},
				},
			},
		},
		"subtotal_cents": map[string]any{"type": "integer"},
		"tax_cents":      map[string]any{"type": "integer"},
		"total_cents":    map[string]any{"type": "integer"},
		"purchase_date":  map[string]any{"type": "string"},
	},
}

type claudeItem struct {
	Name       string `json:"name"`
	RawName    string `json:"raw_name"`
	Code       string `json:"code"`
	Quantity   int32  `json:"quantity"`
	PriceCents int64  `json:"price_cents"`
}

type claudeReceipt struct {
	Store         string       `json:"store"`
	Items         []claudeItem `json:"items"`
	SubtotalCents int64        `json:"subtotal_cents"`
	TaxCents      int64        `json:"tax_cents"`
	TotalCents    int64        `json:"total_cents"`
	PurchaseDate  string       `json:"purchase_date"`
}

// Extract returns parsed receipt data from a photo or a PDF.
func (c *Claude) Extract(ctx context.Context, img []byte, contentType string) (*receipt.Parsed, error) {
	var doc anthropic.BetaContentBlockParamUnion
	if contentType == "application/pdf" {
		doc = anthropic.NewBetaDocumentBlock(anthropic.BetaBase64PDFSourceParam{Data: base64.StdEncoding.EncodeToString(img)})
	} else {
		// Keep requests small and fast: long side ≤ 1600px is plenty for receipt text.
		small, err := Downscale(img, 1600, 3<<20)
		if err != nil {
			return nil, fmt.Errorf("claude: %w", err)
		}
		doc = anthropic.NewBetaImageBlock(anthropic.BetaBase64ImageSourceParam{
			MediaType: anthropic.BetaBase64ImageSourceMediaTypeImageJPEG,
			Data:      base64.StdEncoding.EncodeToString(small),
		})
	}

	resp, err := c.client.Beta.Messages.New(ctx, anthropic.BetaMessageNewParams{
		Model:     anthropic.Model(c.model),
		MaxTokens: 16000,
		// Policy declines are re-served by Anthropic's recommended fallback model.
		Betas:     []anthropic.AnthropicBeta{anthropic.AnthropicBetaServerSideFallback2026_07_01},
		Fallbacks: anthropic.BetaFallbacksParamOfDefault(),
		OutputConfig: anthropic.BetaOutputConfigParam{
			Effort: anthropic.BetaOutputConfigEffortLow, // extraction, not reasoning
			Format: anthropic.BetaJSONOutputFormatParam{Schema: claudeSchema},
		},
		Messages: []anthropic.BetaMessageParam{
			anthropic.NewBetaUserMessage(doc, anthropic.NewBetaTextBlock(claudePrompt)),
		},
	})
	if err != nil {
		var apiErr *anthropic.Error
		if errors.As(err, &apiErr) {
			return nil, fmt.Errorf("claude: api status %d", apiErr.StatusCode)
		}
		return nil, fmt.Errorf("claude: %w", err)
	}
	if resp.StopReason == anthropic.BetaStopReasonRefusal {
		return nil, errors.New("claude: request declined")
	}
	if resp.StopReason == anthropic.BetaStopReasonMaxTokens {
		return nil, errors.New("claude: output truncated")
	}

	var text strings.Builder
	for _, block := range resp.Content {
		if t, ok := block.AsAny().(anthropic.BetaTextBlock); ok {
			text.WriteString(t.Text)
		}
	}
	var out claudeReceipt
	if err := json.Unmarshal([]byte(text.String()), &out); err != nil {
		return nil, fmt.Errorf("claude: bad json: %w", err)
	}
	return out.toParsed()
}

func (r claudeReceipt) toParsed() (*receipt.Parsed, error) {
	p := &receipt.Parsed{Store: strings.TrimSpace(r.Store), SubtotalCents: r.SubtotalCents, TaxCents: r.TaxCents, TotalCents: r.TotalCents}
	var sum int64
	for _, it := range r.Items {
		name := strings.TrimSpace(it.Name)
		if name == "" || it.PriceCents <= 0 {
			continue
		}
		q := it.Quantity
		if q <= 0 {
			q = 1
		}
		raw := strings.TrimSpace(it.RawName)
		if raw == "" {
			raw = name
		}
		p.Items = append(p.Items, receipt.Item{Name: name, RawName: raw, Code: strings.TrimSpace(it.Code), Quantity: q, PriceCents: it.PriceCents})
		sum += it.PriceCents
	}
	if len(p.Items) == 0 {
		return nil, receipt.ErrNoItems
	}
	if p.SubtotalCents == 0 {
		p.SubtotalCents = sum
	} else if p.SubtotalCents != sum {
		p.Warnings = append(p.Warnings, fmt.Sprintf("items sum %.2f differs from subtotal %.2f — check items",
			float64(sum)/100, float64(p.SubtotalCents)/100))
	}
	if p.TotalCents == 0 {
		p.TotalCents = p.SubtotalCents + p.TaxCents
	}
	if t, err := time.Parse(time.DateOnly, r.PurchaseDate); err == nil && t.Year() >= 2000 {
		p.PurchasedAt = &t
	}
	return p, nil
}
