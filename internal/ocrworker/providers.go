// Package ocrworker turns uploaded receipt photos into items. It consumes
// receipt.uploaded, runs OCR providers in order and publishes receipt.parsed.
package ocrworker

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png" // decoders for downscaling
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

// RapidOCR calls the self-hosted Python service (PaddleOCR models on ONNX Runtime).
// Free and unlimited; the service returns text lines already grouped into rows.
type RapidOCR struct {
	URL  string
	HTTP *http.Client
}

func (r RapidOCR) Name() string { return "rapidocr" }

// SupportsPDF: the OCR service reads the PDF text layer or renders scanned pages.
func (r RapidOCR) SupportsPDF() bool { return true }

func (r RapidOCR) Recognize(ctx context.Context, img []byte, contentType string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(r.URL, "/")+"/ocr", bytes.NewReader(img))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", contentType)
	resp, err := r.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("rapidocr: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("rapidocr: status %d: %s", resp.StatusCode, b)
	}
	var out struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("rapidocr: %w", err)
	}
	return out.Text, nil
}

// OCRSpace is the free cloud fallback (https://ocr.space/OCRAPI):
// 25,000 requests/month, 1 MB per file on the free plan — images are downscaled first.
type OCRSpace struct {
	APIKey string
	HTTP   *http.Client
}

func (o OCRSpace) Name() string { return "ocrspace" }

// SupportsPDF: free plan PDFs are limited and our downscaling is image-only.
func (o OCRSpace) SupportsPDF() bool { return false }

func (o OCRSpace) Recognize(ctx context.Context, img []byte, _ string) (string, error) {
	small, err := Downscale(img, 1400, 900<<10)
	if err != nil {
		return "", fmt.Errorf("ocrspace: %w", err)
	}
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	_ = w.WriteField("base64Image", "data:image/jpeg;base64,"+base64.StdEncoding.EncodeToString(small))
	_ = w.WriteField("OCREngine", "2")
	_ = w.WriteField("isTable", "true") // keeps name and price of a row on one line
	_ = w.WriteField("scale", "true")
	_ = w.WriteField("detectOrientation", "true")
	_ = w.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.ocr.space/parse/image", &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("apikey", o.APIKey)
	resp, err := o.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("ocrspace: %w", err)
	}
	defer resp.Body.Close()
	var out struct {
		ParsedResults []struct {
			ParsedText string `json:"ParsedText"`
		} `json:"ParsedResults"`
		IsErroredOnProcessing bool `json:"IsErroredOnProcessing"`
		ErrorMessage          any  `json:"ErrorMessage"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("ocrspace: status %d: %w", resp.StatusCode, err)
	}
	if out.IsErroredOnProcessing || len(out.ParsedResults) == 0 {
		return "", fmt.Errorf("ocrspace: %v", out.ErrorMessage)
	}
	return strings.ReplaceAll(out.ParsedResults[0].ParsedText, "\t", " "), nil
}

// Downscale re-encodes the image as JPEG no wider than maxW and (best effort) under maxBytes.
func Downscale(img []byte, maxW, maxBytes int) ([]byte, error) {
	src, _, err := image.Decode(bytes.NewReader(img))
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}
	b := src.Bounds()
	dst := src
	if b.Dx() > maxW {
		h := b.Dy() * maxW / b.Dx()
		r := image.NewRGBA(image.Rect(0, 0, maxW, h))
		draw.CatmullRom.Scale(r, r.Bounds(), src, b, draw.Over, nil)
		dst = r
	}
	for q := 85; q >= 40; q -= 15 {
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: q}); err != nil {
			return nil, err
		}
		if buf.Len() <= maxBytes || q-15 < 40 {
			return buf.Bytes(), nil
		}
	}
	return nil, fmt.Errorf("unreachable")
}

func httpClient() *http.Client { return &http.Client{Timeout: 60 * time.Second} }
