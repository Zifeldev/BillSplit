// Command social runs the social microservice (friends, groups, invite links).
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
	"github.com/Zifeldev/Billsplit/internal/social"
	"github.com/Zifeldev/Billsplit/migrations"
	ipb "github.com/Zifeldev/Billsplit/pkg/pb/billsplit/internalapi/v1"
	pb "github.com/Zifeldev/Billsplit/pkg/pb/billsplit/v1"
)

func main() { app.Main("social", run) }

func run(ctx context.Context, cfg *config.Config, log *slog.Logger) error {
	pool, err := app.Postgres(ctx, cfg, migrations.For("social"), log)
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

	svc := social.NewService(
		social.NewFriendRepo(pool), social.NewGroupRepo(pool), social.NewInviteRepo(pool), postgres.NewTxManager(pool),
		clients.NewUsers(authConn), clients.NewSplit(splitConn), fx.New(cfg.FX.BaseURL, cache), cache, cfg.Redis.TTL,
		social.LinkBuilder{WebURL: cfg.Gateway.WebURL, Scheme: cfg.Gateway.AppScheme}, outbox.NewPublisher(pool),
	)
	h := social.NewHandler(svc)

	srv, hs := grpcx.NewServer(cfg, log)
	pb.RegisterFriendServiceServer(srv, h)
	pb.RegisterGroupServiceServer(srv, h)
	ipb.RegisterGroupDirectoryServer(srv, h.Directory())

	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error { return grpcx.Serve(ctx, srv, hs, cfg.GRPC.Addr, log) })
	g.Go(func() error { return metrics.Serve(ctx, cfg.Metrics.Addr) })
	g.Go(func() error { return outbox.NewRelay(pool, producer.Writer(), log).Run(ctx) })
	return g.Wait()
}
