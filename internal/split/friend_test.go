package split

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Zifeldev/Billsplit/internal/clients"
	"github.com/Zifeldev/Billsplit/internal/domain"
	"github.com/Zifeldev/Billsplit/internal/platform/auth"
)

const (
	flMe     = "10000000-0000-0000-0000-000000000001"
	flFriend = "10000000-0000-0000-0000-000000000002"
	flGA     = "10000000-0000-0000-0000-0000000000a1"
	flGB     = "10000000-0000-0000-0000-0000000000b1"
	flGE     = "10000000-0000-0000-0000-0000000000e1"
)

func at(min int) time.Time { return time.Date(2026, 10, 1, 12, min, 0, 0, time.UTC) }

func sumEntries(g LedgerGroup) int64 {
	var s int64
	for _, e := range g.Entries {
		s += e.Amount
	}
	return s
}

func TestPairLedgerSigns(t *testing.T) {
	// Newest first, as the repository returns them.
	events := []PairEvent{
		// friend paid me 4.00 in B -> I'm owed less.
		{GroupID: flGB, Currency: "USD", Settlement: true, FromUser: flFriend, ToUser: flMe, Amount: 400, OccurredAt: at(5)},
		// I paid the friend 5.00 in A -> I owe less.
		{GroupID: flGA, Currency: "USD", Settlement: true, FromUser: flMe, ToUser: flFriend, Amount: 500, OccurredAt: at(4)},
		// friend owes me 10.00 for a taxi in B.
		{GroupID: flGB, Currency: "USD", ReceiptID: "r2", Kind: KindExpense, Title: "Taxi", FromUser: flFriend, ToUser: flMe, Amount: 1000, OccurredAt: at(3)},
		// I owe the friend 30.00 for a receipt in A, stored as two debt rows.
		{GroupID: flGA, Currency: "USD", ReceiptID: "r1", Kind: KindReceipt, Title: "Walmart", FromUser: flMe, ToUser: flFriend, Amount: 1000, OccurredAt: at(2)},
		{GroupID: flGA, Currency: "USD", ReceiptID: "r1", Kind: KindReceipt, Title: "Walmart", FromUser: flMe, ToUser: flFriend, Amount: 2000, OccurredAt: at(2)},
		// Debt without a receipt and without a title.
		{GroupID: flGA, Currency: "USD", Kind: KindExpense, FromUser: flFriend, ToUser: flMe, Amount: 100, OccurredAt: at(1)},
	}
	gs := pairLedger(events, flMe)
	if len(gs) != 2 || gs[0].GroupID != flGB || gs[1].GroupID != flGA {
		t.Fatalf("groups = %+v", gs)
	}
	b, a := gs[0], gs[1]
	if b.Net != 600 || len(b.Entries) != 2 || b.Entries[0].Kind != EntrySettlement || b.Entries[0].Amount != -400 ||
		b.Entries[0].Title != "Payment" || b.Entries[1].Kind != EntryExpense || b.Entries[1].Amount != 1000 || b.Entries[1].Title != "Taxi" {
		t.Fatalf("group B = %+v", b)
	}
	if a.Net != -2400 || len(a.Entries) != 3 || a.Entries[0].Amount != 500 || a.Entries[1].Amount != -3000 ||
		a.Entries[1].Kind != EntryReceipt || a.Entries[1].ReceiptID != "r1" || a.Entries[2].Title != "Expense" {
		t.Fatalf("group A = %+v", a)
	}
	for _, g := range gs {
		if sumEntries(g) != g.Net {
			t.Fatalf("group %s entries sum %d != net %d", g.GroupID, sumEntries(g), g.Net)
		}
	}
	// The friend sees the exact mirror image.
	for i, g := range pairLedger(events, flFriend) {
		if g.Net != -gs[i].Net {
			t.Fatalf("mirror net %d vs %d", g.Net, gs[i].Net)
		}
	}
}

func TestFriendLedgerService(t *testing.T) {
	l := &flLedger{events: []PairEvent{
		{GroupID: flGE, Currency: "EUR", ReceiptID: "r3", Kind: KindReceipt, Title: "Spar", FromUser: flFriend, ToUser: flMe, Amount: 1000, OccurredAt: at(9)},
		{GroupID: flGB, Currency: "USD", ReceiptID: "r2", Kind: KindExpense, Title: "Taxi", FromUser: flFriend, ToUser: flMe, Amount: 1000, OccurredAt: at(3)},
		{GroupID: flGA, Currency: "USD", ReceiptID: "r1", Kind: KindReceipt, Title: "Walmart", FromUser: flMe, ToUser: flFriend, Amount: 3000, OccurredAt: at(2)},
	}}
	svc := NewService(nil, l, flTx{}, flGroups{}, flUsers{}, &flPub{}, flFX{})
	ctx := auth.WithUserID(context.Background(), flMe)

	led, err := svc.FriendLedger(ctx, flFriend)
	if err != nil {
		t.Fatal(err)
	}
	// EUR 10.00 at 1.5 = $15.00; +15 +10 -30 = -5.
	if led.Currency != "USD" || led.Net != -500 || len(led.Groups) != 3 {
		t.Fatalf("ledger = %+v", led)
	}
	if led.Friend.Name != "Friend" || led.Friend.Venmo != "friend-v" || led.Friend.ID != flFriend {
		t.Fatalf("friend = %+v", led.Friend)
	}
	e := led.Groups[0]
	if e.GroupName != "Group" || e.Currency != "EUR" || e.Net != 1000 || e.NetHome != 1500 {
		t.Fatalf("EUR group = %+v", e)
	}
	if led.Groups[2].GroupName != "Group A" || led.Groups[2].NetHome != -3000 {
		t.Fatalf("group A = %+v", led.Groups[2])
	}

	// Validation.
	for _, id := range []string{"", "nope", flMe} {
		if _, err := svc.FriendLedger(ctx, id); !errors.Is(err, domain.ErrInvalidArgument) {
			t.Fatalf("FriendLedger(%q) err = %v", id, err)
		}
	}
	// No history: empty ledger, no error.
	empty, err := svc.FriendLedger(ctx, "10000000-0000-0000-0000-000000000009")
	if err != nil || empty.Net != 0 || len(empty.Groups) != 0 {
		t.Fatalf("empty = %+v, %v", empty, err)
	}
}

func TestSettleWithFriendService(t *testing.T) {
	l := &flLedger{events: []PairEvent{
		{GroupID: flGB, Currency: "USD", ReceiptID: "r2", Kind: KindExpense, FromUser: flFriend, ToUser: flMe, Amount: 1000},
		{GroupID: flGA, Currency: "USD", ReceiptID: "r1", Kind: KindReceipt, FromUser: flMe, ToUser: flFriend, Amount: 3000},
		{GroupID: flGE, Currency: "EUR", Settlement: true, FromUser: flMe, ToUser: flFriend, Amount: 700},
		{GroupID: flGE, Currency: "EUR", ReceiptID: "r3", Kind: KindReceipt, FromUser: flMe, ToUser: flFriend, Amount: 700},
	}}
	pub := &flPub{}
	svc := NewService(nil, l, flTx{}, flGroups{}, flUsers{}, pub, flFX{})
	ctx := auth.WithUserID(context.Background(), flMe)

	res, err := svc.SettleWithFriend(ctx, flFriend)
	if err != nil {
		t.Fatal(err)
	}
	if !l.locked || res.NetPaid != -2000 || res.Currency != "USD" || len(res.Settlements) != 2 {
		t.Fatalf("result = %+v (locked=%v)", res, l.locked)
	}
	byGroup := map[string]domain.Settlement{}
	for _, s := range res.Settlements {
		byGroup[s.GroupID] = s
	}
	if s := byGroup[flGA]; s.FromUser != flMe || s.ToUser != flFriend || s.Amount != 3000 {
		t.Fatalf("A settlement = %+v", s)
	}
	if s := byGroup[flGB]; s.FromUser != flFriend || s.ToUser != flMe || s.Amount != 1000 {
		t.Fatalf("B settlement = %+v", s)
	}
	for _, g := range pairLedger(l.events, flMe) {
		if g.Net != 0 {
			t.Fatalf("group %s still has net %d", g.GroupID, g.Net)
		}
	}
	if len(pub.events) != 1 || pub.events[0].UserIDs[0] != flFriend || pub.events[0].Type != domain.NotifySettlement ||
		pub.events[0].Body != "Me settled up with you: $20.00 across 2 groups" {
		t.Fatalf("notifications = %+v", pub.events)
	}

	// Second call: nothing to settle, nothing published.
	res, err = svc.SettleWithFriend(ctx, flFriend)
	if err != nil || len(res.Settlements) != 0 || res.NetPaid != 0 || len(pub.events) != 1 {
		t.Fatalf("second settle = %+v, %v, events %d", res, err, len(pub.events))
	}
}

// ---------- fakes ----------

type flLedger struct {
	LedgerRepository
	events []PairEvent
	locked bool
}

func (l *flLedger) PairHistory(_ context.Context, a, b string) ([]PairEvent, error) {
	var out []PairEvent
	for _, e := range l.events {
		if (e.FromUser == a && e.ToUser == b) || (e.FromUser == b && e.ToUser == a) {
			out = append(out, e)
		}
	}
	return out, nil
}

func (l *flLedger) LockPair(context.Context, string, string) error { l.locked = true; return nil }

func (l *flLedger) CreateSettlement(_ context.Context, s *domain.Settlement) error {
	s.ID = "s" + s.GroupID
	l.events = append([]PairEvent{{GroupID: s.GroupID, Currency: s.Currency, Settlement: true,
		FromUser: s.FromUser, ToUser: s.ToUser, Amount: s.Amount}}, l.events...)
	return nil
}

type flTx struct{}

func (flTx) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error { return fn(ctx) }

type flPub struct{ events []domain.NotificationEvent }

func (p *flPub) Publish(_ context.Context, _, _ string, v any) error {
	p.events = append(p.events, v.(domain.NotificationEvent))
	return nil
}

type flGroups struct{}

func (flGroups) GetGroup(_ context.Context, id string) (clients.GroupInfo, error) {
	switch id {
	case flGA:
		return clients.GroupInfo{ID: id, Name: "Group A", Currency: "USD"}, nil
	case flGB:
		return clients.GroupInfo{ID: id, Name: "Group B", Currency: "USD"}, nil
	}
	return clients.GroupInfo{}, domain.ErrNotFound
}

type flUsers struct{}

func (flUsers) GetUsers(_ context.Context, ids []string) (map[string]clients.UserInfo, error) {
	out := map[string]clients.UserInfo{}
	for _, id := range ids {
		switch id {
		case flMe:
			out[id] = clients.UserInfo{ID: id, Name: "Me", Currency: "USD"}
		case flFriend:
			out[id] = clients.UserInfo{ID: id, Name: "Friend", Currency: "EUR", Venmo: "friend-v"}
		}
	}
	return out, nil
}

func (u flUsers) GetUser(ctx context.Context, id string) (clients.UserInfo, error) {
	m, _ := u.GetUsers(ctx, []string{id})
	return m[id], nil
}

func (flUsers) GetUserByEmail(context.Context, string) (clients.UserInfo, error) {
	return clients.UserInfo{}, domain.ErrNotFound
}

// flFX: 1 EUR = 1.5 USD.
type flFX struct{}

func (flFX) Validate(_ context.Context, code, def string) (string, error) {
	return titleOr(code, def), nil
}

func (flFX) Rate(_ context.Context, from, to string, _ time.Time) (float64, error) {
	switch {
	case from == to:
		return 1, nil
	case from == "EUR" && to == "USD":
		return 1.5, nil
	case from == "USD" && to == "EUR":
		return 1 / 1.5, nil
	}
	return 0, errors.New("no rate")
}
