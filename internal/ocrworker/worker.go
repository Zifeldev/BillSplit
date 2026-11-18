package ocrworker

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Zifeldev/Billsplit/internal/clients"
	"github.com/Zifeldev/Billsplit/internal/domain"
	"github.com/Zifeldev/Billsplit/internal/parser/receipt"
	"github.com/Zifeldev/Billsplit/internal/platform/broker"
	"github.com/Zifeldev/Billsplit/internal/platform/logger"
	"github.com/Zifeldev/Billsplit/internal/platform/metrics"
	pb "github.com/Zifeldev/Billsplit/pkg/pb/billsplit/v1"
)

type Provider interface {
	Name() string
	Recognize(ctx context.Context, img []byte, contentType string) (string, error)
	SupportsPDF() bool
}

// Extractor returns structured data directly (vision LLM), skipping text parsing.
type Extractor interface {
	Name() string
	Extract(ctx context.Context, img []byte, contentType string) (*receipt.Parsed, error)
}

type Worker struct {
	split     clients.Split
	pub       broker.Publisher
	providers []Provider
	extractor Extractor // optional paid last resort
	names     *Names    // product-code → readable name (nil: disabled)
}

type Options struct {
	RapidOCRURL     string
	OCRSpaceAPIKey  string // free fallback, optional
	AnthropicAPIKey string // Claude vision last resort, optional
	AnthropicModel  string
	Names           *Names // optional
}

// New builds the provider chain: RapidOCR (free, self-hosted) → OCR.space
// (free cloud, if a key is set) → Claude vision (paid, if a key is set).
func New(split clients.Split, pub broker.Publisher, o Options) *Worker {
	ps := []Provider{RapidOCR{URL: o.RapidOCRURL, HTTP: httpClient()}}
	if o.OCRSpaceAPIKey != "" {
		ps = append(ps, OCRSpace{APIKey: o.OCRSpaceAPIKey, HTTP: httpClient()})
	}
	w := &Worker{split: split, pub: pub, providers: ps, names: o.Names}
	if o.AnthropicAPIKey != "" {
		w.extractor = NewClaude(o.AnthropicAPIKey, o.AnthropicModel)
	}
	return w
}

// Handle processes receipt.uploaded. Business failures are published as ok=false;
// only infrastructure errors are returned (so Kafka retries them).
func (w *Worker) Handle(ctx context.Context, ev domain.ReceiptUploadedEvent) error {
	log := logger.FromContext(ctx).With("receipt_id", ev.ReceiptID)
	img, err := w.split.ReceiptImage(ctx, ev.ReceiptID)
	if errors.Is(err, domain.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if img.GetStatus() != pb.ReceiptStatus_RECEIPT_STATUS_PENDING.String() {
		return nil // already processed or edited manually
	}

	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	out := w.process(ctx, img.GetImage(), img.GetContentType(), img.GetRawText())
	out.ReceiptID = ev.ReceiptID
	if out.OK {
		log.Info("receipt recognized", "provider", out.Provider, "items", len(out.Items), "warning", out.Warning)
	} else {
		log.Warn("receipt not recognized", "err", out.Error)
	}
	return w.pub.Publish(ctx, domain.TopicReceiptParsed, ev.ReceiptID, out)
}

func (w *Worker) process(ctx context.Context, img []byte, contentType, rawText string) domain.ReceiptParsedEvent {
	ev := w.recognize(ctx, img, contentType, rawText)
	// Text-based providers keep the store's abbreviations ("GV HH 320Z"): look the
	// printed product codes up. Claude already returns readable names.
	if ev.OK && w.names != nil && ev.Provider != "claude" {
		items := make([]receipt.Item, len(ev.Items))
		for i, it := range ev.Items {
			items[i] = receipt.Item{Name: it.Name, RawName: it.RawName, Code: it.Code}
		}
		w.names.Enrich(ctx, items)
		for i := range items {
			ev.Items[i].Name, ev.Items[i].RawName = items[i].Name, items[i].RawName
		}
	}
	return ev
}

func (w *Worker) recognize(ctx context.Context, img []byte, contentType, rawText string) domain.ReceiptParsedEvent {
	if strings.TrimSpace(rawText) != "" {
		p, err := receipt.ParseText(rawText)
		return toEvent("text", p, err)
	}

	// Try providers in order; stop at the first clean result (items sum == subtotal).
	var best *domain.ReceiptParsedEvent
	var errs []string
	for _, pr := range w.providers {
		if contentType == "application/pdf" && !pr.SupportsPDF() {
			continue
		}
		text, err := pr.Recognize(ctx, img, contentType)
		if err != nil {
			metrics.OCRRequests.WithLabelValues(pr.Name(), "error").Inc()
			errs = append(errs, err.Error())
			continue
		}
		p, err := receipt.ParseText(text)
		ev := toEvent(pr.Name(), p, err)
		switch {
		case !ev.OK:
			metrics.OCRRequests.WithLabelValues(pr.Name(), "no_items").Inc()
			errs = append(errs, fmt.Sprintf("%s: %s", pr.Name(), ev.Error))
			continue
		case ev.Warning == "":
			metrics.OCRRequests.WithLabelValues(pr.Name(), "ok").Inc()
			return ev
		default:
			metrics.OCRRequests.WithLabelValues(pr.Name(), "mismatch").Inc()
			if best == nil || len(ev.Items) > len(best.Items) {
				best = &ev
			}
		}
	}
	// Free providers failed or disagree with the printed subtotal: ask the vision model.
	if w.extractor != nil {
		p, err := w.extractor.Extract(ctx, img, contentType)
		ev := toEvent(w.extractor.Name(), p, err)
		switch {
		case err != nil:
			metrics.OCRRequests.WithLabelValues(w.extractor.Name(), "error").Inc()
			errs = append(errs, err.Error())
		case ev.Warning == "" || best == nil:
			metrics.OCRRequests.WithLabelValues(w.extractor.Name(), "ok").Inc()
			return ev
		}
	}
	if best != nil {
		return *best
	}
	return domain.ReceiptParsedEvent{OK: false,
		Error: "could not read the receipt — retake the photo in good light or enter items manually (" + strings.Join(errs, "; ") + ")"}
}

func toEvent(provider string, p *receipt.Parsed, err error) domain.ReceiptParsedEvent {
	if err != nil {
		return domain.ReceiptParsedEvent{OK: false, Provider: provider, Error: err.Error()}
	}
	ev := domain.ReceiptParsedEvent{OK: true, Provider: provider, Store: p.Store, SubtotalCents: p.SubtotalCents,
		TaxCents: p.TaxCents, TotalCents: p.TotalCents, PurchasedAt: p.PurchasedAt, Warning: strings.Join(p.Warnings, "; ")}
	for _, it := range p.Items {
		ev.Items = append(ev.Items, domain.ParsedItem{Name: it.Name, RawName: it.RawName, Code: it.Code,
			Quantity: it.Quantity, PriceCents: it.PriceCents})
	}
	return ev
}
