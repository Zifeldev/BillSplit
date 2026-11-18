// Package lifecycle coordinates graceful shutdown across a service.
//
// Sequence on SIGTERM/SIGINT (see app.Main):
//
//  1. Draining() becomes true: readiness probes (/healthz, gRPC health) start
//     failing so load balancers / Kubernetes stop sending new traffic.
//  2. After DrainDelay, listeners stop accepting; in-flight HTTP/gRPC requests
//     finish (GracefulStop / http.Server.Shutdown).
//  3. Background workers stop taking new work: Kafka consumers finish the
//     message in hand and commit it, the outbox relay flushes pending events.
//  4. Connections close in reverse order (Kafka writers flush, then Redis, Postgres).
//
// The whole sequence is bounded by Timeout; a second signal or the timeout
// forces exit(1).
package lifecycle

import (
	"context"
	"sync/atomic"
	"time"
)

var (
	draining   atomic.Bool
	drainDelay atomic.Int64
	timeout    atomic.Int64
)

func init() {
	drainDelay.Store(int64(2 * time.Second))
	timeout.Store(int64(25 * time.Second))
}

// Configure sets the shutdown timings (called once from app.Main).
func Configure(delay, total time.Duration) {
	drainDelay.Store(int64(delay))
	timeout.Store(int64(total))
}

// StartDraining marks the process as shutting down.
func StartDraining() { draining.Store(true) }

// Draining reports whether shutdown has begun (readiness must fail).
func Draining() bool { return draining.Load() }

// DrainDelay is how long to keep serving after readiness flips, so that
// load balancers notice before connections are refused.
func DrainDelay() time.Duration { return time.Duration(drainDelay.Load()) }

// Timeout bounds the whole graceful shutdown.
func Timeout() time.Duration { return time.Duration(timeout.Load()) }

// Detached returns a context that is NOT cancelled when ctx is (keeps values
// like the request logger) but is bounded by the shutdown timeout. Use it to
// finish work already started when shutdown begins.
func Detached(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), Timeout())
}

// WaitDrainDelay sleeps for DrainDelay (returns early if done is closed).
func WaitDrainDelay() {
	if d := DrainDelay(); d > 0 {
		time.Sleep(d)
	}
}
