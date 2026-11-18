package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/Zifeldev/Billsplit/internal/platform/metrics"
)

// ErrMiss is returned when the key is absent.
var ErrMiss = errors.New("cache miss")

// Cache is a small JSON cache abstraction used by services (cache-aside).
type Cache interface {
	Get(ctx context.Context, key string, dst any) error
	Set(ctx context.Context, key string, v any, ttl time.Duration) error
	Delete(ctx context.Context, keys ...string) error
	// Incr is used for version-based invalidation of key families.
	Incr(ctx context.Context, key string) (int64, error)
	GetInt(ctx context.Context, key string) (int64, error)
}

type Redis struct {
	rdb *redis.Client
}

func NewRedisClient(ctx context.Context, addr, password string, db int) (*redis.Client, error) {
	rdb := redis.NewClient(&redis.Options{Addr: addr, Password: password, DB: db})
	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis ping: %w", err)
	}
	return rdb, nil
}

func NewRedis(rdb *redis.Client) *Redis { return &Redis{rdb: rdb} }

func (c *Redis) Get(ctx context.Context, key string, dst any) error {
	b, err := c.rdb.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		metrics.CacheRequests.WithLabelValues("miss").Inc()
		return ErrMiss
	}
	if err != nil {
		metrics.CacheRequests.WithLabelValues("error").Inc()
		return err
	}
	metrics.CacheRequests.WithLabelValues("hit").Inc()
	return json.Unmarshal(b, dst)
}

func (c *Redis) Set(ctx context.Context, key string, v any, ttl time.Duration) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return c.rdb.Set(ctx, key, b, ttl).Err()
}

func (c *Redis) Delete(ctx context.Context, keys ...string) error {
	return c.rdb.Del(ctx, keys...).Err()
}

func (c *Redis) Incr(ctx context.Context, key string) (int64, error) {
	return c.rdb.Incr(ctx, key).Result()
}

func (c *Redis) GetInt(ctx context.Context, key string) (int64, error) {
	v, err := c.rdb.Get(ctx, key).Int64()
	if errors.Is(err, redis.Nil) {
		return 0, nil
	}
	return v, err
}
