// Command ocr-worker recognizes receipt photos: Kafka receipt.uploaded ->
// RapidOCR (self-hosted) -> OCR.space fallback -> receipt.parsed.
package main

import (
	"context"
	"log/slog"

	"golang.org/x/sync/errgroup"

	"github.com/Zifeldev/Billsplit/internal/clients"
	"github.com/Zifeldev/Billsplit/internal/domain"
	"github.com/Zifeldev/Billsplit/internal/ocrworker"
	"github.com/Zifeldev/Billsplit/internal/platform/app"
	"github.com/Zifeldev/Billsplit/internal/platform/broker"
	"github.com/Zifeldev/Billsplit/internal/platform/config"
	"github.com/Zifeldev/Billsplit/internal/platform/grpcx"
	"github.com/Zifeldev/Billsplit/internal/platform/metrics"
)

func main() { app.Main("ocr-worker", run) }

func run(ctx context.Context, cfg *config.Config, log *slog.Logger) error {
	splitConn, err := grpcx.Dial(cfg.Services.Split, cfg.Security.InternalToken)
	if err != nil {
		return err
	}
	defer splitConn.Close()

	if err := broker.EnsureTopics(cfg.Kafka.Brokers, domain.AllTopics...); err != nil {
		log.Warn("ensure kafka topics", "err", err)
	}
	producer := broker.NewProducer(cfg.Kafka.Brokers)
	defer producer.Close()

	// Redis caches product names found by receipt barcodes (Open Food/Products Facts).
	rdb, cache, err := app.Redis(ctx, cfg)
	if err != nil {
		return err
	}
	defer rdb.Close()

	w := ocrworker.New(clients.NewSplit(splitConn), producer, ocrworker.Options{
		RapidOCRURL: cfg.OCR.URL, OCRSpaceAPIKey: cfg.OCR.OCRSpaceAPIKey,
		AnthropicAPIKey: cfg.OCR.AnthropicAPIKey, AnthropicModel: cfg.OCR.AnthropicModel,
		Names: ocrworker.NewNames(cache),
	})
	log.Info("ocr providers", "rapidocr", true, "ocrspace", cfg.OCR.OCRSpaceAPIKey != "", "claude", cfg.OCR.AnthropicAPIKey != "")
	c := broker.NewConsumer(cfg.Kafka.Brokers, cfg.Kafka.GroupID, domain.TopicReceiptUploaded,
		broker.JSONHandler(log, w.Handle), log)

	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error { defer c.Close(); return c.Run(ctx) })
	g.Go(func() error { return metrics.Serve(ctx, cfg.Metrics.Addr) })
	log.Info("ocr worker started")
	return g.Wait()
}
