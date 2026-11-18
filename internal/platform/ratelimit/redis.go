package ratelimit

import (
	"context"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

// Redis is a distributed token bucket shared by all gateway replicas.
// The bucket lives in a Redis hash and is updated atomically by a Lua script.
// If Redis is unavailable it fails over to the in-memory limiter (per instance)
// instead of rejecting everyone or letting everyone through.
type Redis struct {
	rdb      *redis.Client
	fallback *Limiter
	log      *slog.Logger

	rps, strictRPS     float64
	burst, strictBurst int
}

func NewRedis(rdb *redis.Client, rps float64, burst int, strictRPS float64, strictBurst int, log *slog.Logger) *Redis {
	return &Redis{rdb: rdb, fallback: New(rps, burst, strictRPS, strictBurst), log: log,
		rps: rps, burst: burst, strictRPS: strictRPS, strictBurst: strictBurst}
}

// KEYS[1] bucket; ARGV: rate per second, burst, now (ms). Returns 1 if allowed.
var tokenBucket = redis.NewScript(`
local b = redis.call('HMGET', KEYS[1], 't', 'ts')
local rate, burst, now = tonumber(ARGV[1]), tonumber(ARGV[2]), tonumber(ARGV[3])
local tokens = tonumber(b[1]) or burst
local ts = tonumber(b[2]) or now
tokens = math.min(burst, tokens + (now - ts) / 1000 * rate)
local allowed = 0
if tokens >= 1 then tokens = tokens - 1; allowed = 1 end
redis.call('HSET', KEYS[1], 't', tokens, 'ts', now)
redis.call('PEXPIRE', KEYS[1], math.ceil(burst / rate * 1000) + 1000)
return allowed`)

func (r *Redis) Allow(key string, strict bool) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	now := time.Now().UnixMilli()
	ok, err := tokenBucket.Run(ctx, r.rdb, []string{"rl:g:" + key}, r.rps, r.burst, now).Int()
	if err == nil && ok == 1 && strict {
		ok, err = tokenBucket.Run(ctx, r.rdb, []string{"rl:s:" + key}, r.strictRPS, r.strictBurst, now).Int()
	}
	if err != nil {
		r.log.Warn("redis rate limiter unavailable, using local fallback", "err", err)
		return r.fallback.Allow(key, strict)
	}
	return ok == 1
}

func (r *Redis) Cleanup(ctx context.Context, idle time.Duration) { r.fallback.Cleanup(ctx, idle) }
