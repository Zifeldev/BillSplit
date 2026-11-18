// Package app holds the common bootstrap used by every service binary.
package app

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/Zifeldev/Billsplit/internal/platform/cache"
	"github.com/Zifeldev/Billsplit/internal/platform/config"
	"github.com/Zifeldev/Billsplit/internal/platform/lifecycle"
	"github.com/Zifeldev/Billsplit/internal/platform/logger"
	"github.com/Zifeldev/Billsplit/internal/platform/postgres"
)

// Main loads config, sets up logging and signals, then runs fn. Exits 1 on error.
//
// The ctx passed to fn is cancelled on the first SIGINT/SIGTERM; fn must then
// stop gracefully (see package lifecycle). A second signal, or exceeding
// SHUTDOWN_TIMEOUT, forces exit(1) so a stuck dependency can't hang the deploy.
func Main(service string, fn func(ctx context.Context, cfg *config.Config, log *slog.Logger) error) {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("config", "err", err)
		os.Exit(1)
	}
	if cfg.Kafka.GroupID == "" {
		cfg.Kafka.GroupID = "billsplit-" + service
	}
	log := logger.New(cfg.Env, cfg.LogLevel).With("service", service)
	slog.SetDefault(log)

	lifecycle.Configure(cfg.Shutdown.DrainDelay, cfg.Shutdown.Timeout)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sigs := make(chan os.Signal, 2)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)
	var started atomic.Int64 // unix nanos of the first signal
	go func() {
		sig := <-sigs
		started.Store(time.Now().UnixNano())
		log.Info("shutdown started", "signal", sig.String(), "timeout", cfg.Shutdown.Timeout, "drain_delay", cfg.Shutdown.DrainDelay)
		lifecycle.StartDraining()
		cancel()
		select {
		case sig = <-sigs:
			log.Error("second signal received, forcing exit", "signal", sig.String())
		case <-time.After(cfg.Shutdown.Timeout + 3*time.Second):
			log.Error("graceful shutdown timed out, forcing exit")
		}
		os.Exit(1)
	}()

	err = fn(ctx, cfg, log)
	if ctx.Err() == nil { // stopped by itself, not by a signal
		if err != nil {
			log.Error("service stopped", "err", err)
			os.Exit(1)
		}
		log.Info("service stopped")
		return
	}
	if err != nil {
		log.Warn("error during shutdown", "err", err)
	}
	log.Info("shutdown complete", "took", time.Since(time.Unix(0, started.Load())).Round(time.Millisecond))
}

// Postgres connects to the service schema and applies its migrations.
func Postgres(ctx context.Context, cfg *config.Config, migrations fs.FS, log *slog.Logger) (*pgxpool.Pool, error) {
	if cfg.Postgres.Schema == "" {
		return nil, fmt.Errorf("POSTGRES_SCHEMA is required")
	}
	pool, err := postgres.NewPool(ctx, cfg.Postgres.DSN, cfg.Postgres.Schema, cfg.Postgres.MaxConns)
	if err != nil {
		return nil, fmt.Errorf("postgres: %w", err)
	}
	if err := postgres.Migrate(ctx, pool, cfg.Postgres.Schema, migrations); err != nil {
		pool.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	log.Info("migrations applied", "schema", cfg.Postgres.Schema)
	return pool, nil
}

func Redis(ctx context.Context, cfg *config.Config) (*redis.Client, cache.Cache, error) {
	rdb, err := cache.NewRedisClient(ctx, cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.DB)
	if err != nil {
		return nil, nil, err
	}
	return rdb, cache.NewRedis(rdb), nil
}
