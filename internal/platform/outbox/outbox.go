// Package outbox implements the transactional outbox pattern.
//
// Services publish events by inserting them into an `outbox` table in the SAME
// database transaction as their business data (Publisher joins the tx from
// ctx). A Relay goroutine moves rows to Kafka and deletes them. If Kafka is
// down, events wait in the table instead of being lost; delivery is
// at-least-once, so consumers must be idempotent (they are).
//
// Every service schema has the table (migration 00002_outbox.sql):
//
//	CREATE TABLE outbox (id BIGSERIAL PRIMARY KEY, topic TEXT NOT NULL, key TEXT NOT NULL,
//	                     payload JSONB NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT now());
package outbox

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/segmentio/kafka-go"

	"github.com/Zifeldev/Billsplit/internal/platform/lifecycle"
	"github.com/Zifeldev/Billsplit/internal/platform/postgres"
)

var (
	relayed = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "billsplit_outbox_relayed_total",
		Help: "Outbox rows moved to Kafka.",
	}, []string{"topic"})
	pending = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "billsplit_outbox_pending",
		Help: "Rows waiting in the outbox (last relay iteration).",
	})
)

// Publisher implements broker.Publisher by writing to the outbox table.
// Call it inside TxManager.WithinTx together with the business writes.
type Publisher struct{ db postgres.DB }

func NewPublisher(pool *pgxpool.Pool) *Publisher { return &Publisher{db: postgres.DB{Pool: pool}} }

func (p *Publisher) Publish(ctx context.Context, topic, key string, event any) error {
	b, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = p.db.Q(ctx).Exec(ctx, `INSERT INTO outbox (topic, key, payload) VALUES ($1, $2, $3)`, topic, key, b)
	return postgres.MapErr(err)
}

// Writer is the Kafka side (kafka.Writer satisfies it).
type Writer interface {
	WriteMessages(ctx context.Context, msgs ...kafka.Message) error
}

type Relay struct {
	pool     *pgxpool.Pool
	w        Writer
	log      *slog.Logger
	interval time.Duration
	batch    int
}

func NewRelay(pool *pgxpool.Pool, w Writer, log *slog.Logger) *Relay {
	return &Relay{pool: pool, w: w, log: log.With("component", "outbox"), interval: 200 * time.Millisecond, batch: 100}
}

// Run polls the outbox until ctx is cancelled, then flushes what's left once
// more (events committed by requests that finished during shutdown).
func (r *Relay) Run(ctx context.Context) error {
	t := time.NewTicker(r.interval)
	defer t.Stop()
	defer r.flush(ctx)
	for {
		for { // drain while there is a full batch
			n, err := r.once(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return nil
				}
				r.log.Warn("relay failed, will retry", "err", err)
				break
			}
			if n < r.batch {
				break
			}
		}
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
		}
	}
}

func (r *Relay) flush(ctx context.Context) {
	// Wait for in-flight requests (gRPC graceful stop) to commit their events first.
	lifecycle.WaitDrainDelay()
	fctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	total := 0
	for {
		n, err := r.once(fctx)
		total += n
		if err != nil {
			r.log.Warn("final outbox flush failed; events stay in the table for the next start", "err", err)
			return
		}
		if n < r.batch {
			break
		}
	}
	r.log.Info("outbox flushed on shutdown", "events", total)
}

// once moves up to r.batch rows in one transaction. SKIP LOCKED lets several
// replicas relay concurrently without sending the same row twice.
func (r *Relay) once(ctx context.Context) (int, error) {
	var n int
	err := pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT id, topic, key, payload FROM outbox ORDER BY id LIMIT $1 FOR UPDATE SKIP LOCKED`, r.batch)
		if err != nil {
			return err
		}
		var ids []int64
		var msgs []kafka.Message
		for rows.Next() {
			var id int64
			var m kafka.Message
			var key string
			if err := rows.Scan(&id, &m.Topic, &key, &m.Value); err != nil {
				rows.Close()
				return err
			}
			m.Key = []byte(key)
			ids = append(ids, id)
			msgs = append(msgs, m)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		n = len(msgs)
		pending.Set(float64(n))
		if n == 0 {
			return nil
		}
		if err := r.w.WriteMessages(ctx, msgs...); err != nil {
			return err // rollback: rows stay for the next attempt
		}
		if _, err := tx.Exec(ctx, `DELETE FROM outbox WHERE id = ANY($1)`, ids); err != nil {
			return err
		}
		for _, m := range msgs {
			relayed.WithLabelValues(m.Topic).Inc()
		}
		return nil
	})
	return n, err
}
