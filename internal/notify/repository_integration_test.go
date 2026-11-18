//go:build integration

package notify

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"

	"github.com/Zifeldev/Billsplit/internal/domain"
	"github.com/Zifeldev/Billsplit/internal/platform/auth"
	"github.com/Zifeldev/Billsplit/internal/platform/testutil"
	"github.com/Zifeldev/Billsplit/migrations"
)

type fakePusher struct {
	sent []PushMessage
	dead map[string]bool
}

func (f *fakePusher) Send(_ context.Context, msgs []PushMessage) []PushResult {
	f.sent = append(f.sent, msgs...)
	out := make([]PushResult, len(msgs))
	for i, m := range msgs {
		out[i] = PushResult{Token: m.To, OK: !f.dead[m.To]}
		if f.dead[m.To] {
			out[i].Error, out[i].Unregistered = "DeviceNotRegistered", true
		}
	}
	return out
}

func TestNotifyRepoAndService(t *testing.T) {
	pool := testutil.Postgres(t, "notify", migrations.For("notify"))
	repo := NewRepo(pool)
	pusher := &fakePusher{dead: map[string]bool{"ExponentPushToken[dead]": true}}
	svc := NewService(repo, pusher)

	alice, bob := uuid.NewString(), uuid.NewString()
	actx := auth.WithUserID(context.Background(), alice)
	bctx := auth.WithUserID(context.Background(), bob)

	// Devices: validation, upsert, ownership.
	if err := svc.RegisterDevice(actx, "bad", "ios"); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("bad token: %v", err)
	}
	if err := svc.RegisterDevice(actx, "ExponentPushToken[a1]", "symbian"); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("bad platform: %v", err)
	}
	for _, tok := range []string{"ExponentPushToken[a1]", "ExponentPushToken[dead]"} {
		if err := svc.RegisterDevice(actx, tok, "ios"); err != nil {
			t.Fatal(err)
		}
	}
	if err := svc.RegisterDevice(actx, "ExponentPushToken[a1]", "android"); err != nil { // upsert
		t.Fatal(err)
	}
	if err := svc.RegisterDevice(bctx, "ExpoPushToken[b1]", "web"); err != nil {
		t.Fatal(err)
	}
	// Bob can't unregister Alice's token.
	if err := svc.UnregisterDevice(bctx, "ExponentPushToken[a1]"); err != nil {
		t.Fatal(err)
	}
	toks, err := repo.TokensByUsers(context.Background(), []string{alice, bob})
	if err != nil {
		t.Fatal(err)
	}
	if len(toks[alice]) != 2 || len(toks[bob]) != 1 {
		t.Fatalf("tokens = %v", toks)
	}

	// Consume an event twice (redelivery): one inbox row per user, one push round.
	ev := domain.NotificationEvent{UserIDs: []string{alice, bob, alice, "not-a-uuid"}, Type: domain.NotifyExpenseAdded,
		Title: "New expense", Body: "Alice added $5", Data: map[string]string{"group_id": "g1"}}
	b, _ := json.Marshal(ev)
	msg := kafka.Message{Topic: domain.TopicNotification, Partition: 1, Offset: 42, Value: b}
	for range 2 {
		if err := svc.HandleMessage(context.Background(), msg); err != nil {
			t.Fatal(err)
		}
	}
	if len(pusher.sent) != 3 { // alice x2 devices + bob x1
		t.Fatalf("pushes = %d, want 3", len(pusher.sent))
	}
	if d := pusher.sent[0].Data; d["group_id"] != "g1" || d["type"] != domain.NotifyExpenseAdded || d["notification_id"] == "" {
		t.Fatalf("push data = %v", d)
	}
	// DeviceNotRegistered token was removed.
	toks, _ = repo.TokensByUsers(context.Background(), []string{alice})
	if len(toks[alice]) != 1 || toks[alice][0] != "ExponentPushToken[a1]" {
		t.Fatalf("alice tokens after push = %v", toks[alice])
	}

	// Same payload at another offset is a new notification.
	msg.Offset = 43
	if err := svc.HandleMessage(context.Background(), msg); err != nil {
		t.Fatal(err)
	}

	items, unread, err := svc.List(actx, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || unread != 2 {
		t.Fatalf("alice list = %d items, %d unread", len(items), unread)
	}
	if items[0].Data["group_id"] != "g1" || items[0].Type != domain.NotifyExpenseAdded || items[0].Read {
		t.Fatalf("item = %+v", items[0])
	}

	// Bob can't mark Alice's notification read.
	if err := svc.MarkRead(bctx, []string{items[0].ID}); err != nil {
		t.Fatal(err)
	}
	if _, unread, _ = svc.List(actx, 0, false); unread != 2 {
		t.Fatalf("unread after foreign mark = %d", unread)
	}
	if err := svc.MarkRead(actx, []string{"nope"}); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("bad id: %v", err)
	}
	if err := svc.MarkRead(actx, []string{items[0].ID}); err != nil {
		t.Fatal(err)
	}
	items, unread, _ = svc.List(actx, 0, true)
	if len(items) != 1 || unread != 1 {
		t.Fatalf("unread_only = %d items, %d unread", len(items), unread)
	}
	if err := svc.MarkRead(actx, nil); err != nil { // all
		t.Fatal(err)
	}
	if _, unread, _ = svc.List(actx, 0, false); unread != 0 {
		t.Fatalf("unread after mark all = %d", unread)
	}
	if _, unread, _ = svc.List(bctx, 0, false); unread != 2 {
		t.Fatalf("bob unread = %d", unread)
	}

	// Limit is clamped.
	items, _, _ = svc.List(actx, 1, false)
	if len(items) != 1 {
		t.Fatalf("limit 1 -> %d", len(items))
	}

	// Unregister own token.
	if err := svc.UnregisterDevice(actx, "ExponentPushToken[a1]"); err != nil {
		t.Fatal(err)
	}
	toks, _ = repo.TokensByUsers(context.Background(), []string{alice})
	if len(toks[alice]) != 0 {
		t.Fatalf("alice tokens after unregister = %v", toks[alice])
	}
}
