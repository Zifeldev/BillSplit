// Package realtime carries live UI events from services to connected clients.
//
// Services publish small JSON events to Redis pub/sub channels (best effort,
// after the business transaction committed); the gateway holds the client
// WebSockets, subscribes once to all channels and fans events out. Clients
// treat an event as "refetch now" — the database stays the source of truth,
// so a lost event only means the screen updates on the next refetch.
package realtime

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/Zifeldev/Billsplit/internal/platform/logger"
)

// Event types.
const (
	ReceiptUpdated = "receipt.updated"
	ReceiptDeleted = "receipt.deleted"
	Presence       = "presence"
)

// Receipt update actions.
const (
	ActionClaim     = "claim"
	ActionItems     = "items"
	ActionParsed    = "parsed"
	ActionFinalized = "finalized"
)

type Event struct {
	Type      string   `json:"type"`
	ReceiptID string   `json:"receipt_id,omitempty"`
	Action    string   `json:"action,omitempty"`
	UserID    string   `json:"user_id,omitempty"`  // who made the change
	ItemIDs   []string `json:"item_ids,omitempty"` // items touched by a claim
	Shares    []int32  `json:"shares,omitempty"`   // the claimant's new shares, aligned with ItemIDs (0 — unclaimed)
	Viewers   []string `json:"viewers,omitempty"`  // presence: user ids with the receipt open
}

const receiptPrefix = "rt:receipt:"

// ChannelPattern matches every receipt channel (gateway subscription).
const ChannelPattern = receiptPrefix + "*"

func ReceiptChannel(id string) string { return receiptPrefix + id }

// ReceiptFromChannel returns the receipt id of a receipt channel.
func ReceiptFromChannel(ch string) (string, bool) { return strings.CutPrefix(ch, receiptPrefix) }

// Publisher sends events; failures are logged, never returned — live updates
// must not fail the write that triggered them.
type Publisher interface {
	Publish(ctx context.Context, channel string, ev Event)
}

type Nop struct{}

func (Nop) Publish(context.Context, string, Event) {}

type Redis struct{ rdb *redis.Client }

func NewRedis(rdb *redis.Client) *Redis { return &Redis{rdb: rdb} }

func (r *Redis) Publish(ctx context.Context, channel string, ev Event) {
	b, err := json.Marshal(ev)
	if err != nil {
		return
	}
	// Detached: the request may be finishing, the event should still go out.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	if err := r.rdb.Publish(ctx, channel, b).Err(); err != nil {
		logger.FromContext(ctx).Warn("realtime publish failed", "channel", channel, "err", err)
	}
}
