// Command split runs the split microservice (receipts, claims, debts, balances).
// It also consumes OCR results from Kafka.
package main

import (
	"context"
	"log/slog"

	"golang.org/x/sync/errgroup"

	"github.com/Zifeldev/Billsplit/internal/clients"
	"github.com/Zifeldev/Billsplit/internal/domain"
	"github.com/Zifeldev/Billsplit/internal/platform/app"
	"github.com/Zifeldev/Billsplit/internal/platform/broker"
	"github.com/Zifeldev/Billsplit/internal/platform/config"
	"github.com/Zifeldev/Billsplit/internal/platform/fx"
	"github.com/Zifeldev/Billsplit/internal/platform/grpcx"
	"github.com/Zifeldev/Billsplit/internal/platform/metrics"
	"github.com/Zifeldev/Billsplit/internal/platform/outbox"
	"github.com/Zifeldev/Billsplit/internal/platform/postgres"
	"github.com/Zifeldev/Billsplit/internal/platform/realtime"
	"github.com/Zifeldev/Billsplit/internal/split"
	"github.com/Zifeldev/Billsplit/migrations"
	ipb "github.com/Zifeldev/Billsplit/pkg/pb/billsplit/internalapi/v1"
	pb "github.com/Zifeldev/Billsplit/pkg/pb/billsplit/v1"
)

func main() { app.Main("split", run) }

func run(ctx context.Context, cfg *config.Config, log *slog.Logger) error {
	pool, err := app.Postgres(ctx, cfg, migrations.For("split"), log)
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
	socialConn, err := grpcx.Dial(cfg.Services.Social, cfg.Security.InternalToken)
	if err != nil {
		return err
	}
	defer socialConn.Close()

	if err := broker.EnsureTopics(cfg.Kafka.Brokers, domain.AllTopics...); err != nil {
		log.Warn("ensure kafka topics", "err", err)
	}
	producer := broker.NewProducer(cfg.Kafka.Brokers)
	defer producer.Close()
	// Events are written to the outbox table in the same tx as business data;
	// the relay moves them to Kafka.
	relay := outbox.NewRelay(pool, producer.Writer(), log)

	svc := split.NewService(split.NewReceiptRepo(pool), split.NewLedgerRepo(pool), postgres.NewTxManager(pool),
		clients.NewGroups(socialConn), clients.NewUsers(authConn), outbox.NewPublisher(pool), fx.New(cfg.FX.BaseURL, cache)).
		WithRealtime(realtime.NewRedis(rdb)) // live claim updates for open receipt screens
	h := split.NewHandler(svc)

	srv, hs := grpcx.NewServer(cfg, log)
	pb.RegisterReceiptServiceServer(srv, h)
	pb.RegisterBalanceServiceServer(srv, h)
	ipb.RegisterSplitInternalServer(srv, h)

	parsed := broker.NewConsumer(cfg.Kafka.Brokers, cfg.Kafka.GroupID, domain.TopicReceiptParsed,
		broker.JSONHandler(log, svc.HandleParsed), log)

	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error { return grpcx.Serve(ctx, srv, hs, cfg.GRPC.Addr, log) })
	g.Go(func() error { return metrics.Serve(ctx, cfg.Metrics.Addr) })
	g.Go(func() error { defer parsed.Close(); return parsed.Run(ctx) })
	g.Go(func() error { return relay.Run(ctx) })
	g.Go(func() error { return svc.RunPendingRecovery(ctx, log) })
	return g.Wait()
}
