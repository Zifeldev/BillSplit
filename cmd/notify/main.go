// Command notify runs the notification microservice: Expo push devices and the
// in-app inbox. It consumes user-facing notifications from Kafka.
package main

import (
	"context"
	"log/slog"

	"github.com/segmentio/kafka-go"
	"golang.org/x/sync/errgroup"

	"github.com/Zifeldev/Billsplit/internal/domain"
	"github.com/Zifeldev/Billsplit/internal/notify"
	"github.com/Zifeldev/Billsplit/internal/platform/app"
	"github.com/Zifeldev/Billsplit/internal/platform/broker"
	"github.com/Zifeldev/Billsplit/internal/platform/config"
	"github.com/Zifeldev/Billsplit/internal/platform/grpcx"
	"github.com/Zifeldev/Billsplit/internal/platform/logger"
	"github.com/Zifeldev/Billsplit/internal/platform/metrics"
	"github.com/Zifeldev/Billsplit/migrations"
	pb "github.com/Zifeldev/Billsplit/pkg/pb/billsplit/v1"
)

func main() { app.Main("notify", run) }

func run(ctx context.Context, cfg *config.Config, log *slog.Logger) error {
	pool, err := app.Postgres(ctx, cfg, migrations.For("notify"), log)
	if err != nil {
		return err
	}
	defer pool.Close()

	if err := broker.EnsureTopics(cfg.Kafka.Brokers, domain.AllTopics...); err != nil {
		log.Warn("ensure kafka topics", "err", err)
	}

	svc := notify.NewService(notify.NewRepo(pool), notify.NewExpoClient("", cfg.Push.ExpoAccessToken))

	srv, hs := grpcx.NewServer(cfg, log)
	pb.RegisterNotificationServiceServer(srv, notify.NewHandler(svc))

	consumer := broker.NewConsumer(cfg.Kafka.Brokers, cfg.Kafka.GroupID, domain.TopicNotification,
		func(ctx context.Context, msg kafka.Message) error {
			return svc.HandleMessage(logger.WithContext(ctx, log), msg)
		}, log)

	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error { return grpcx.Serve(ctx, srv, hs, cfg.GRPC.Addr, log) })
	g.Go(func() error { return metrics.Serve(ctx, cfg.Metrics.Addr) })
	g.Go(func() error { defer consumer.Close(); return consumer.Run(ctx) })
	return g.Wait()
}
