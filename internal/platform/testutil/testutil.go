// Package testutil starts real Postgres and Redis in Docker for integration
// tests (testcontainers). Tests using it are tagged `integration`:
//
//	go test -tags integration ./...
package testutil

import (
	"context"
	"io/fs"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/Zifeldev/Billsplit/internal/platform/postgres"
)

// Postgres starts postgres:17, applies the service migrations into `schema`
// and returns a pool bound to that schema. The container is removed after the test.
func Postgres(t *testing.T, schema string, migrations fs.FS) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()
	c, err := tcpostgres.Run(ctx, "postgres:17-alpine",
		tcpostgres.WithDatabase("billsplit"), tcpostgres.WithUsername("test"), tcpostgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").
			WithOccurrence(2).WithStartupTimeout(60*time.Second)))
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	t.Cleanup(func() { _ = testcontainers.TerminateContainer(c) })

	dsn, err := c.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	pool, err := postgres.NewPool(ctx, dsn, schema, 5)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := postgres.Migrate(ctx, pool, schema, migrations); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return pool
}

// Redis starts redis:8 and returns a connected client.
func Redis(t *testing.T) *redis.Client {
	t.Helper()
	ctx := context.Background()
	c, err := tcredis.Run(ctx, "redis:8-alpine")
	if err != nil {
		t.Fatalf("start redis: %v", err)
	}
	t.Cleanup(func() { _ = testcontainers.TerminateContainer(c) })
	url, err := c.ConnectionString(ctx)
	if err != nil {
		t.Fatal(err)
	}
	opt, err := redis.ParseURL(url)
	if err != nil {
		t.Fatal(err)
	}
	rdb := redis.NewClient(opt)
	t.Cleanup(func() { _ = rdb.Close() })
	return rdb
}
