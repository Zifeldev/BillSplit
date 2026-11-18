package finance

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// PDFTexter extracts the text of a PDF, one slice of visual lines per page
// (text layer in reading order; scanned pages are OCR'd).
type PDFTexter interface {
	PDFText(ctx context.Context, pdf []byte) ([][]string, error)
}

// ErrPDFRejected is a permanent OCR failure (bad or unreadable PDF): retrying won't help.
var ErrPDFRejected = errors.New("pdf rejected")

// OCRClient calls the OCR service: POST {url}/pdf/text with the raw PDF.
type OCRClient struct {
	url  string
	http *http.Client
}

func NewOCRClient(url string) *OCRClient {
	return &OCRClient{url: strings.TrimRight(url, "/"), http: &http.Client{Timeout: 60 * time.Second}}
}

func (c *OCRClient) PDFText(ctx context.Context, pdf []byte) ([][]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url+"/pdf/text", bytes.NewReader(pdf))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/pdf")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ocr: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, fmt.Errorf("ocr: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Detail any `json:"detail"`
		}
		_ = json.Unmarshal(body, &e)
		msg := fmt.Sprint(e.Detail)
		if e.Detail == nil {
			msg = http.StatusText(resp.StatusCode)
		}
		if resp.StatusCode >= 400 && resp.StatusCode < 500 && resp.StatusCode != http.StatusTooManyRequests &&
			resp.StatusCode != http.StatusRequestTimeout {
			return nil, fmt.Errorf("%w: %s", ErrPDFRejected, msg)
		}
		return nil, fmt.Errorf("ocr: status %d: %s", resp.StatusCode, msg)
	}
	var out struct {
		Pages []struct {
			Lines []string `json:"lines"`
		} `json:"pages"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("ocr: bad response: %w", err)
	}
	pages := make([][]string, 0, len(out.Pages))
	for _, p := range out.Pages {
		pages = append(pages, p.Lines)
	}
	if len(pages) == 0 && out.Text != "" {
		pages = append(pages, strings.Split(out.Text, "\n"))
	}
	return pages, nil
}
