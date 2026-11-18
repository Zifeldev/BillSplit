// Command finance runs the personal finance microservice and its Kafka consumers
// (bank statement import, split receipt -> personal expenses). PDF statements
// are converted to text by the OCR service (OCR_URL).
package main

import (
	"context"
	"log/slog"

	"golang.org/x/sync/errgroup"

	"github.com/Zifeldev/Billsplit/internal/clients"
	"github.com/Zifeldev/Billsplit/internal/domain"
	"github.com/Zifeldev/Billsplit/internal/finance"
	"github.com/Zifeldev/Billsplit/internal/platform/app"
	"github.com/Zifeldev/Billsplit/internal/platform/broker"
	"github.com/Zifeldev/Billsplit/internal/platform/config"
	"github.com/Zifeldev/Billsplit/internal/platform/fx"
	"github.com/Zifeldev/Billsplit/internal/platform/grpcx"
	"github.com/Zifeldev/Billsplit/internal/platform/market"
	"github.com/Zifeldev/Billsplit/internal/platform/metrics"
	"github.com/Zifeldev/Billsplit/internal/platform/outbox"
	"github.com/Zifeldev/Billsplit/internal/platform/postgres"
	"github.com/Zifeldev/Billsplit/migrations"
	pb "github.com/Zifeldev/Billsplit/pkg/pb/billsplit/v1"
)

func main() { app.Main("finance", run) }

func run(ctx context.Context, cfg *config.Config, log *slog.Logger) error {
	pool, err := app.Postgres(ctx, cfg, migrations.For("finance"), log)
	if err != nil {
		return err
	}
	defer pool.Close()
	rdb, cache, err := app.Redis(ctx, cfg)
	if err != nil {
		return err
	}
	defer rdb.Close()

	authConn, err := grpcx.Dial(cfg.Services.Auth, cfg.Security.InternalToken)
	if err != nil {
		return err
	}
	defer authConn.Close()
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

	// Events go through the transactional outbox; the relay below moves them to Kafka.
	svc := finance.NewService(finance.NewRepo(pool), postgres.NewTxManager(pool), clients.NewUsers(authConn),
		clients.NewSplit(splitConn), outbox.NewPublisher(pool), fx.New(cfg.FX.BaseURL, cache), cache,
		finance.NewRedisLocker(rdb), cfg.Redis.TTL).
		WithOCR(finance.NewOCRClient(cfg.OCR.URL)). // PDF statements: text layer / OCR via the OCR service
		WithMarket(market.New(cfg.Market.BaseURL, cache))

	srv, hs := grpcx.NewServer(cfg, log)
	pb.RegisterFinanceServiceServer(srv, finance.NewHandler(svc))

	consumers := []*broker.Consumer{
		broker.NewConsumer(cfg.Kafka.Brokers, cfg.Kafka.GroupID, domain.TopicStatementUploaded,
			broker.JSONHandler(log, svc.HandleStatementUploaded), log),
		broker.NewConsumer(cfg.Kafka.Brokers, cfg.Kafka.GroupID, domain.TopicReceiptFinalized,
			broker.JSONHandler(log, svc.HandleReceiptFinalized), log),
	}

	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error { return grpcx.Serve(ctx, srv, hs, cfg.GRPC.Addr, log) })
	g.Go(func() error { return metrics.Serve(ctx, cfg.Metrics.Addr) })
	g.Go(func() error { return outbox.NewRelay(pool, producer.Writer(), log).Run(ctx) })
	g.Go(func() error { return svc.RunRecurringReminders(ctx, log) })
	g.Go(func() error { return svc.RunNetWorthSnapshots(ctx, log) })
	for _, c := range consumers {
		g.Go(func() error { defer c.Close(); return c.Run(ctx) })
	}
	return g.Wait()
}
