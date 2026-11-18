// Package ratelimit is an in-memory token bucket per client key (IP).
// Simple and fast (no network hop). Limits are per gateway instance: with N
// replicas the effective limit is N×RPS — move buckets to Redis if that matters.
package ratelimit

import (
	"context"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type Limiter struct {
	mu      sync.Mutex
	clients map[string]*client

	rps, strictRPS     rate.Limit
	burst, strictBurst int
}

type client struct {
	general, strict *rate.Limiter
	lastSeen        time.Time
}

func New(rps float64, burst int, strictRPS float64, strictBurst int) *Limiter {
	return &Limiter{
		clients: make(map[string]*client),
		rps:     rate.Limit(rps), burst: burst,
		strictRPS: rate.Limit(strictRPS), strictBurst: strictBurst,
	}
}

// Allow consumes a token for key; strict requests (login, register) also need a strict token.
func (l *Limiter) Allow(key string, strict bool) bool {
	l.mu.Lock()
	c, ok := l.clients[key]
	if !ok {
		c = &client{
			general: rate.NewLimiter(l.rps, l.burst),
			strict:  rate.NewLimiter(l.strictRPS, l.strictBurst),
		}
		l.clients[key] = c
	}
	c.lastSeen = time.Now()
	l.mu.Unlock()

	if !c.general.Allow() {
		return false
	}
	return !strict || c.strict.Allow()
}

// Cleanup evicts idle clients so the map doesn't grow forever.
func (l *Limiter) Cleanup(ctx context.Context, idle time.Duration) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			l.mu.Lock()
			for k, c := range l.clients {
				if time.Since(c.lastSeen) > idle {
					delete(l.clients, k)
				}
			}
			l.mu.Unlock()
		}
	}
}
