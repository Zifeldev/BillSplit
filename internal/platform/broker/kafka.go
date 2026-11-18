package broker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"time"

	"github.com/segmentio/kafka-go"

	"github.com/Zifeldev/Billsplit/internal/platform/lifecycle"
	"github.com/Zifeldev/Billsplit/internal/platform/logger"
	"github.com/Zifeldev/Billsplit/internal/platform/metrics"
)

// Publisher is what services depend on.
type Publisher interface {
	Publish(ctx context.Context, topic, key string, event any) error
}

type Producer struct {
	w *kafka.Writer
}

func NewProducer(brokers []string) *Producer {
	return &Producer{w: &kafka.Writer{
		Addr:                   kafka.TCP(brokers...),
		Balancer:               &kafka.Hash{}, // same key -> same partition -> ordering per entity
		RequiredAcks:           kafka.RequireAll,
		AllowAutoTopicCreation: true,
		BatchTimeout:           10 * time.Millisecond,
	}}
}

func (p *Producer) Publish(ctx context.Context, topic, key string, event any) error {
	b, err := json.Marshal(event)
	if err != nil {
		return err
	}
	err = p.w.WriteMessages(ctx, kafka.Message{Topic: topic, Key: []byte(key), Value: b})
	status := "ok"
	if err != nil {
		status = "error"
	}
	metrics.KafkaMessages.WithLabelValues(topic, "produce", status).Inc()
	return err
}

func (p *Producer) Close() error { return p.w.Close() }

// Writer exposes the underlying Kafka writer (used by the outbox relay).
func (p *Producer) Writer() *kafka.Writer { return p.w }

// EnsureTopics creates topics if they don't exist (idempotent).
func EnsureTopics(brokers []string, topics ...string) error {
	conn, err := kafka.Dial("tcp", brokers[0])
	if err != nil {
		return err
	}
	defer conn.Close()
	ctrl, err := conn.Controller()
	if err != nil {
		return err
	}
	cc, err := kafka.Dial("tcp", net.JoinHostPort(ctrl.Host, strconv.Itoa(ctrl.Port)))
	if err != nil {
		return err
	}
	defer cc.Close()
	// Every topic gets its dead-letter twin, so the first DLQ write never hits "unknown topic".
	cfgs := make([]kafka.TopicConfig, 0, 2*len(topics))
	for _, t := range topics {
		cfgs = append(cfgs,
			kafka.TopicConfig{Topic: t, NumPartitions: 3, ReplicationFactor: 1},
			kafka.TopicConfig{Topic: t + ".dlq", NumPartitions: 1, ReplicationFactor: 1})
	}
	return cc.CreateTopics(cfgs...)
}

// Handler processes one message. Returning an error triggers retries; after
// maxAttempts (exponential backoff) the message is sent to <topic>.dlq and committed.
type Handler func(ctx context.Context, msg kafka.Message) error

type Consumer struct {
	r           *kafka.Reader
	dlq         *kafka.Writer
	handler     Handler
	log         *slog.Logger
	maxAttempts int
}

func NewConsumer(brokers []string, groupID, topic string, h Handler, log *slog.Logger) *Consumer {
	return &Consumer{
		r: kafka.NewReader(kafka.ReaderConfig{
			Brokers:  brokers,
			GroupID:  groupID,
			Topic:    topic,
			MinBytes: 1,
			MaxBytes: 10e6,
			// Short long-poll so Close() (graceful shutdown) doesn't wait the default 10s.
			MaxWait: 500 * time.Millisecond,
		}),
		dlq: &kafka.Writer{
			Addr:                   kafka.TCP(brokers...),
			Topic:                  topic + ".dlq",
			AllowAutoTopicCreation: true,
		},
		handler:     h,
		log:         log.With("topic", topic),
		maxAttempts: 6, // backoff 1+2+4+8+15s ≈ 30s: survives a neighbour's restart
	}
}

// errStopping means shutdown interrupted the retries: the message must be
// neither committed nor dead-lettered — Kafka redelivers it after restart.
var errStopping = errors.New("consumer stopping")

// Run blocks until ctx is cancelled. Offsets are committed only after handling (at-least-once).
//
// Graceful shutdown: cancelling ctx stops fetching new messages, but the
// message already in hand is processed to the end on a detached context
// (bounded by the shutdown timeout) and committed.
func (c *Consumer) Run(ctx context.Context) error {
	topic := c.r.Config().Topic
	for {
		msg, err := c.r.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				c.log.Info("consumer stopped")
				return nil
			}
			return fmt.Errorf("fetch: %w", err)
		}
		// kafka-go may return an already-buffered message after ctx is cancelled:
		// don't start new work during shutdown — leave it uncommitted for redelivery.
		if ctx.Err() != nil {
			c.log.Info("consumer stopped")
			return nil
		}

		start := time.Now()
		status := "ok"
		err = c.handleWithRetry(ctx, msg)
		switch {
		case errors.Is(err, errStopping):
			c.log.Info("shutdown during retries, message left uncommitted for redelivery", "offset", msg.Offset)
			return nil
		case err != nil:
			status = "dlq"
			c.log.Error("moving message to DLQ", "offset", msg.Offset, "key", string(msg.Key), "err", err)
			// Never commit a message we failed to park: retry the DLQ write until it
			// succeeds; on shutdown leave it uncommitted so Kafka redelivers it.
			if !c.writeDLQ(ctx, msg, err) {
				c.log.Warn("shutdown before DLQ write succeeded, message left uncommitted", "offset", msg.Offset)
				return nil
			}
		}
		metrics.KafkaMessages.WithLabelValues(topic, "consume", status).Inc()
		metrics.KafkaHandleDuration.WithLabelValues(topic).Observe(time.Since(start).Seconds())

		cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), opTimeout)
		if err := c.r.CommitMessages(cctx, msg); err != nil {
			c.log.Error("commit failed", "err", err)
		}
		cancel()
	}
}

// opTimeout bounds a single DLQ write or commit (each gets a fresh context).
const opTimeout = 10 * time.Second

func (c *Consumer) writeDLQ(ctx context.Context, msg kafka.Message, cause error) bool {
	dm := kafka.Message{Key: msg.Key, Value: msg.Value, Headers: []kafka.Header{
		{Key: "error", Value: []byte(cause.Error())},
		{Key: "source_offset", Value: []byte(strconv.FormatInt(msg.Offset, 10))},
	}}
	for attempt := 1; ; attempt++ {
		wctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), opTimeout)
		err := c.dlq.WriteMessages(wctx, dm)
		cancel()
		if err == nil {
			return true
		}
		c.log.Error("dlq write failed, retrying", "attempt", attempt, "err", err)
		select {
		case <-ctx.Done():
			return false
		case <-time.After(backoff(attempt)):
		}
	}
}

// backoff: 1s, 2s, 4s, 8s, then 15s.
func backoff(attempt int) time.Duration {
	return min(time.Duration(1<<(attempt-1))*time.Second, 15*time.Second)
}

// handleWithRetry gives every attempt its own context that survives shutdown
// (bounded by the shutdown timeout), so an attempt already running finishes and
// a slow first attempt can't starve the next ones. ctx only stops the waiting
// between retries once shutdown has begun.
func (c *Consumer) handleWithRetry(ctx context.Context, msg kafka.Message) error {
	var err error
	for attempt := 1; attempt <= c.maxAttempts; attempt++ {
		actx, cancel := lifecycle.Detached(ctx)
		err = c.handler(actx, msg)
		cancel()
		if err == nil {
			return nil
		}
		c.log.Warn("handler failed", "attempt", attempt, "err", err)
		if attempt == c.maxAttempts {
			break
		}
		select {
		case <-ctx.Done():
			return errStopping
		case <-time.After(backoff(attempt)):
		}
	}
	return err
}

func (c *Consumer) Close() error {
	_ = c.dlq.Close()
	return c.r.Close()
}

// JSONHandler decodes the message into T and runs fn with a per-message logger.
// Malformed messages are logged and skipped: retrying them can't help.
func JSONHandler[T any](log *slog.Logger, fn func(context.Context, T) error) Handler {
	return func(ctx context.Context, msg kafka.Message) error {
		var ev T
		if err := json.Unmarshal(msg.Value, &ev); err != nil {
			log.Error("bad message, skipping", "topic", msg.Topic, "offset", msg.Offset, "err", err)
			return nil
		}
		l := log.With("topic", msg.Topic, "partition", msg.Partition, "offset", msg.Offset, "key", string(msg.Key))
		return fn(logger.WithContext(ctx, l), ev)
	}
}
