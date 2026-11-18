// Command auth runs the auth microservice (users, JWT, internal user directory).
package main

import (
	"context"
	"log/slog"

	"golang.org/x/sync/errgroup"

	"github.com/Zifeldev/Billsplit/internal/authsvc"
	"github.com/Zifeldev/Billsplit/internal/platform/app"
	"github.com/Zifeldev/Billsplit/internal/platform/auth"
	"github.com/Zifeldev/Billsplit/internal/platform/config"
	"github.com/Zifeldev/Billsplit/internal/platform/fx"
	"github.com/Zifeldev/Billsplit/internal/platform/grpcx"
	"github.com/Zifeldev/Billsplit/internal/platform/metrics"
	"github.com/Zifeldev/Billsplit/migrations"
	ipb "github.com/Zifeldev/Billsplit/pkg/pb/billsplit/internalapi/v1"
	pb "github.com/Zifeldev/Billsplit/pkg/pb/billsplit/v1"
)

func main() { app.Main("auth", run) }

func run(ctx context.Context, cfg *config.Config, log *slog.Logger) error {
	pool, err := app.Postgres(ctx, cfg, migrations.For("auth"), log)
	if err != nil {
		return err
	}
	defer pool.Close()
	rdb, cache, err := app.Redis(ctx, cfg)
	if err != nil {
		return err
	}
	defer rdb.Close()

	jwt := auth.NewJWTManager(cfg.JWT.Secret, cfg.JWT.AccessTTL, cfg.JWT.RefreshTTL)
	tokens := authsvc.NewTokenIssuer(jwt, cfg.JWT.Secret, cfg.JWT.RefreshTTL, authsvc.NewRedisRefreshStore(rdb))
	svc := authsvc.NewService(authsvc.NewUserRepo(pool), tokens, fx.New(cfg.FX.BaseURL, cache))
	h := authsvc.NewHandler(svc)

	srv, hs := grpcx.NewServer(cfg, log)
	pb.RegisterAuthServiceServer(srv, h)
	ipb.RegisterUserDirectoryServer(srv, h)

	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error { return grpcx.Serve(ctx, srv, hs, cfg.GRPC.Addr, log) })
	g.Go(func() error { return metrics.Serve(ctx, cfg.Metrics.Addr) })
	return g.Wait()
}
