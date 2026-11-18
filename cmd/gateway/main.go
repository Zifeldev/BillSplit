// Command gateway is the public HTTP/JSON entry point for web and mobile apps.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"

	"github.com/Zifeldev/Billsplit/internal/gateway"
	"github.com/Zifeldev/Billsplit/internal/platform/app"
	"github.com/Zifeldev/Billsplit/internal/platform/auth"
	"github.com/Zifeldev/Billsplit/internal/platform/config"
	"github.com/Zifeldev/Billsplit/internal/platform/grpcx"
	"github.com/Zifeldev/Billsplit/internal/platform/lifecycle"
	"github.com/Zifeldev/Billsplit/internal/platform/metrics"
	"github.com/Zifeldev/Billsplit/internal/platform/ratelimit"
)

func main() { app.Main("gateway", run) }

func run(ctx context.Context, cfg *config.Config, log *slog.Logger) error {
	var conns gateway.Conns
	for _, c := range []struct {
		dst  **grpc.ClientConn
		addr string
	}{{&conns.Auth, cfg.Services.Auth}, {&conns.Social, cfg.Services.Social},
		{&conns.Split, cfg.Services.Split}, {&conns.Finance, cfg.Services.Finance},
		{&conns.Notify, cfg.Services.Notify}} {
		cc, err := grpcx.Dial(c.addr, cfg.Security.InternalToken)
		if err != nil {
			return err
		}
		defer cc.Close()
		*c.dst = cc
	}

	jwt := auth.NewJWTManager(cfg.JWT.Secret, cfg.JWT.AccessTTL, cfg.JWT.RefreshTTL)
	// Shared buckets in Redis so limits hold across gateway replicas (falls back to in-memory if Redis is down).
	rdb, _, err := app.Redis(ctx, cfg)
	if err != nil {
		return err
	}
	defer rdb.Close()
	rl := ratelimit.NewRedis(rdb, cfg.Limits.RPS, cfg.Limits.Burst, cfg.Limits.AuthRPS, cfg.Limits.AuthBurst, log)
	hub := gateway.NewHub(rdb, log)
	h, err := gateway.NewHandler(ctx, cfg, conns, jwt, rl, hub, log)
	if err != nil {
		return err
	}
	srv := &http.Server{
		Addr:              cfg.Gateway.Addr,
		Handler:           h,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       60 * time.Second, // large receipt uploads on slow mobile networks
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		log.Info("gateway listening", "addr", cfg.Gateway.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	})
	g.Go(func() error {
		<-ctx.Done()
		// /healthz already returns 503; keep serving while load balancers notice,
		// then stop accepting and wait for in-flight requests (e.g. receipt uploads).
		lifecycle.WaitDrainDelay()
		log.Info("http: stopping, waiting for in-flight requests")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), lifecycle.Timeout()-lifecycle.DrainDelay())
		defer cancel()
		srv.SetKeepAlivesEnabled(false)
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Warn("http: graceful shutdown timed out, closing connections", "err", err)
			return srv.Close()
		}
		log.Info("http: stopped gracefully")
		return nil
	})
	g.Go(func() error { rl.Cleanup(ctx, 10*time.Minute); return nil })
	g.Go(func() error { return hub.Run(ctx) })
	g.Go(func() error { return metrics.Serve(ctx, cfg.Metrics.Addr) })
	return g.Wait()
}
