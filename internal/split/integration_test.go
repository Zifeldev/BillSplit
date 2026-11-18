//go:build integration

package split

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Zifeldev/Billsplit/internal/clients"
	"github.com/Zifeldev/Billsplit/internal/domain"
	"github.com/Zifeldev/Billsplit/internal/platform/auth"
	"github.com/Zifeldev/Billsplit/internal/platform/outbox"
	"github.com/Zifeldev/Billsplit/internal/platform/postgres"
	"github.com/Zifeldev/Billsplit/internal/platform/testutil"
	"github.com/Zifeldev/Billsplit/migrations"
)

const (
	group = "00000000-0000-0000-0000-0000000000a1"
	alice = "00000000-0000-0000-0000-000000000001"
	bob   = "00000000-0000-0000-0000-000000000002"
	carol = "00000000-0000-0000-0000-000000000003"
)

func TestIntegration(t *testing.T) {
	pool := testutil.Postgres(t, "split", migrations.For("split"))
	ctx := context.Background()

	t.Run("ReceiptRepo", func(t *testing.T) { testReceiptRepo(t, ctx, pool) })
	t.Run("Ledger", func(t *testing.T) { testLedger(t, ctx, pool) })
	t.Run("OutboxSameTx", func(t *testing.T) { testOutboxTx(t, ctx, pool) })
	t.Run("ServiceFlow", func(t *testing.T) { testServiceFlow(t, ctx, pool) })
	t.Run("PendingRecovery", func(t *testing.T) { testPendingRecovery(t, ctx, pool) })
	t.Run("FriendLedger", func(t *testing.T) { testFriendLedger(t, ctx, pool) })
}

type failingPub struct{}

func (failingPub) Publish(context.Context, string, string, any) error {
	return errors.New("outbox down")
}

// Cross-group netting between alice and bob, then settling everything at once.
func testFriendLedger(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	reset(t, ctx, pool)
	const (
		groupB = "00000000-0000-0000-0000-0000000000b1"
		groupE = "00000000-0000-0000-0000-0000000000e1"
	)
	repo, l := NewReceiptRepo(pool), NewLedgerRepo(pool)
	svc := NewService(repo, l, postgres.NewTxManager(pool), fakeGroups{}, fakeUsers{}, outbox.NewPublisher(pool), flFX{})
	actx := auth.WithUserID(ctx, alice)
	bought := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	mkReceipt := func(g, payer, store string, kind ReceiptKind, cur string, purchased *time.Time) string {
		r := &Receipt{Kind: kind, Receipt: domain.Receipt{GroupID: g, PayerID: payer, Store: store,
			Status: domain.ReceiptFinalized, Currency: cur, PurchasedAt: purchased}}
		must(t, repo.Create(ctx, r, nil))
		return r.ID
	}

	// A: alice owes bob 30.00 for bob's receipt (carol's share must not leak in).
	rA := mkReceipt(group, bob, "Walmart", KindReceipt, "USD", &bought)
	must(t, l.AddDebts(ctx, []domain.Debt{
		{GroupID: group, ReceiptID: rA, FromUser: alice, ToUser: bob, Amount: 3000, Currency: "USD"},
		{GroupID: group, ReceiptID: rA, FromUser: carol, ToUser: bob, Amount: 999, Currency: "USD"},
	}))
	// B: bob owes alice 15.00 for a taxi expense, then pays back 5.00.
	rB := mkReceipt(groupB, alice, "Taxi", KindExpense, "USD", nil)
	must(t, l.AddDebts(ctx, []domain.Debt{{GroupID: groupB, ReceiptID: rB, FromUser: bob, ToUser: alice, Amount: 1500, Currency: "USD"}}))
	must(t, l.CreateSettlement(ctx, &domain.Settlement{GroupID: groupB, FromUser: bob, ToUser: alice, Amount: 500, Currency: "USD"}))

	led, err := svc.FriendLedger(actx, bob)
	must(t, err)
	if led.Net != -2000 || led.Currency != "USD" || len(led.Groups) != 2 || led.Friend.Name != "Bob" || led.Friend.Venmo != "Bob-v" {
		t.Fatalf("ledger = %+v", led)
	}
	byID := map[string]LedgerGroup{}
	for _, g := range led.Groups {
		byID[g.GroupID] = g
		if sumEntries(g) != g.Net {
			t.Fatalf("group %s: entries sum %d != net %d", g.GroupID, sumEntries(g), g.Net)
		}
	}
	a, b := byID[group], byID[groupB]
	if a.GroupName != "Flat" || a.Net != -3000 || len(a.Entries) != 1 || a.Entries[0].Kind != EntryReceipt ||
		a.Entries[0].Title != "Walmart" || a.Entries[0].ReceiptID != rA || !a.Entries[0].OccurredAt.Equal(bought) {
		t.Fatalf("group A = %+v", a)
	}
	if b.GroupName != "Group" || b.Net != 1000 || len(b.Entries) != 2 || b.Entries[0].Kind != EntrySettlement ||
		b.Entries[0].Amount != -500 || b.Entries[1].Kind != EntryExpense || b.Entries[1].Title != "Taxi" || b.Entries[1].Amount != 1500 {
		t.Fatalf("group B = %+v", b)
	}

	// E (EUR): bob owes alice EUR 10.00 = $15.00 at the fake rate.
	rE := mkReceipt(groupE, alice, "Spar", KindReceipt, "EUR", nil)
	must(t, l.AddDebts(ctx, []domain.Debt{{GroupID: groupE, ReceiptID: rE, FromUser: bob, ToUser: alice, Amount: 1000, Currency: "EUR"}}))
	led, err = svc.FriendLedger(actx, bob)
	must(t, err)
	if led.Net != -500 || len(led.Groups) != 3 || led.Groups[0].GroupID != groupE ||
		led.Groups[0].Currency != "EUR" || led.Groups[0].Net != 1000 || led.Groups[0].NetHome != 1500 {
		t.Fatalf("ledger with EUR = %+v", led)
	}
	// Bob's view is the mirror image.
	bled, err := svc.FriendLedger(auth.WithUserID(ctx, bob), alice)
	must(t, err)
	if bled.Net != 500 {
		t.Fatalf("bob's net = %d", bled.Net)
	}

	// A failing outbox rolls the settlements back too.
	bad := NewService(repo, l, postgres.NewTxManager(pool), fakeGroups{}, fakeUsers{}, failingPub{}, flFX{})
	if _, err := bad.SettleWithFriend(actx, bob); err == nil {
		t.Fatal("settle with failing outbox succeeded")
	}
	if n := count(t, ctx, pool, `SELECT count(*) FROM settlements`); n != 1 {
		t.Fatalf("settlements after rollback = %d", n)
	}

	res, err := svc.SettleWithFriend(actx, bob)
	must(t, err)
	if len(res.Settlements) != 3 || res.NetPaid != -500 || res.Currency != "USD" {
		t.Fatalf("settle = %+v", res)
	}
	for _, uid := range []string{alice, bob} {
		pairs, err := l.PairBalances(ctx, uid)
		must(t, err)
		for _, p := range pairs {
			if p.Other == alice || p.Other == bob {
				t.Fatalf("%s still has pair balance %+v", uid, p)
			}
		}
	}
	// carol's debt is untouched.
	net, err := l.NetBalances(ctx, group)
	must(t, err)
	if net[carol] != -999 || net[alice] != 0 {
		t.Fatalf("group A net after settle = %v", net)
	}
	notes := outboxRows(t, ctx, pool, domain.TopicNotification)
	if len(notes) != 1 || notes[0]["type"] != domain.NotifySettlement || notes[0]["user_ids"].([]any)[0] != bob ||
		notes[0]["body"] != "Alice settled up with you: $5.00 across 3 groups" {
		t.Fatalf("notifications = %v", notes)
	}
	led, err = svc.FriendLedger(actx, bob)
	must(t, err)
	if led.Net != 0 || len(led.Groups) != 3 {
		t.Fatalf("ledger after settle = %+v", led)
	}

	// Settling again is a no-op.
	res, err = svc.SettleWithFriend(actx, bob)
	must(t, err)
	if len(res.Settlements) != 0 || res.NetPaid != 0 {
		t.Fatalf("second settle = %+v", res)
	}
	if n := count(t, ctx, pool, `SELECT count(*) FROM settlements`); n != 4 {
		t.Fatalf("settlements after second settle = %d", n)
	}
	if n := len(outboxRows(t, ctx, pool, domain.TopicNotification)); n != 1 {
		t.Fatalf("notifications after second settle = %d", n)
	}
}

// Stuck PENDING receipts are re-queued for OCR; very old ones are given up on.
func testPendingRecovery(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	reset(t, ctx, pool)
	repo := NewReceiptRepo(pool)
	mk := func(age time.Duration) string {
		r := &Receipt{Kind: KindReceipt, Receipt: domain.Receipt{GroupID: group, PayerID: alice, Status: domain.ReceiptPending, Currency: "USD"}}
		must(t, repo.Create(ctx, r, &domain.ReceiptFile{ContentType: "image/png", Data: []byte{1}}))
		_, err := pool.Exec(ctx, `UPDATE receipts SET created_at = now() - $2::interval, updated_at = now() - $2::interval WHERE id = $1`, r.ID, age.String())
		must(t, err)
		return r.ID
	}
	fresh, stuck, ancient := mk(10*time.Second), mk(5*time.Minute), mk(2*time.Hour)

	svc := newTestService(pool)
	requeued, failed, err := svc.RecoverPending(ctx, time.Now())
	must(t, err)
	if requeued != 1 || failed != 1 {
		t.Fatalf("requeued=%d failed=%d, want 1/1", requeued, failed)
	}
	ev := outboxRows(t, ctx, pool, domain.TopicReceiptUploaded)
	if len(ev) != 1 || ev[0]["receipt_id"] != stuck {
		t.Fatalf("outbox = %v, want re-queue of %s only (fresh=%s)", ev, stuck, fresh)
	}
	r, err := repo.GetByID(ctx, ancient)
	must(t, err)
	if r.Status != domain.ReceiptFailed || r.ParseError == "" {
		t.Fatalf("ancient receipt = %v %q, want FAILED with message", r.Status, r.ParseError)
	}
	// Just re-queued: the next pass must not publish it again immediately.
	requeued, _, err = svc.RecoverPending(ctx, time.Now())
	must(t, err)
	if requeued != 0 {
		t.Fatalf("second pass requeued %d, want 0", requeued)
	}
}

func reset(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(ctx, `TRUNCATE receipts, debts, settlements, outbox CASCADE`); err != nil {
		t.Fatal(err)
	}
}

func testReceiptRepo(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	reset(t, ctx, pool)
	repo := NewReceiptRepo(pool)
	r := &Receipt{Kind: KindReceipt, Receipt: domain.Receipt{GroupID: group, PayerID: alice, Status: domain.ReceiptPending, Currency: "USD"}}
	if err := repo.Create(ctx, r, &domain.ReceiptFile{ContentType: "image/png", Data: []byte{1, 2}}); err != nil {
		t.Fatal(err)
	}
	r.Items = []domain.ReceiptItem{{Name: "milk", Quantity: 1, PriceCents: 300}, {Name: "pizza", Quantity: 2, PriceCents: 1000}}
	r.Store, r.Status, r.SubtotalCents, r.TotalCents = "Walmart", domain.ReceiptParsed, 1300, 1300
	if err := repo.ReplaceItems(ctx, r); err != nil {
		t.Fatal(err)
	}
	if r.Items[0].ID == "" || r.Items[1].ID == "" {
		t.Fatal("item IDs not filled")
	}
	must(t, repo.SetClaim(ctx, r.Items[1].ID, alice, 2))
	must(t, repo.SetClaim(ctx, r.Items[1].ID, bob, 1))
	must(t, repo.SetClaim(ctx, r.Items[0].ID, bob, 1))
	must(t, repo.SetClaim(ctx, r.Items[1].ID, bob, 3)) // upsert
	must(t, repo.SetClaim(ctx, r.Items[0].ID, bob, 0)) // unclaim

	got, err := repo.GetByID(ctx, r.ID)
	must(t, err)
	if got.Store != "Walmart" || got.Status != domain.ReceiptParsed || got.Kind != KindReceipt || len(got.Items) != 2 {
		t.Fatalf("receipt = %+v", got)
	}
	if got.Items[0].Name != "milk" || len(got.Items[0].Claims) != 0 {
		t.Fatalf("milk = %+v", got.Items[0])
	}
	cl := got.Items[1].Claims
	if len(cl) != 2 || cl[0] != (domain.ItemClaim{UserID: alice, Shares: 2}) || cl[1] != (domain.ItemClaim{UserID: bob, Shares: 3}) {
		t.Fatalf("pizza claims = %+v", cl)
	}

	// ReplaceItems drops claims.
	got.Items = []domain.ReceiptItem{{Name: "bread", Quantity: 1, PriceCents: 250}}
	must(t, repo.ReplaceItems(ctx, got))
	got, err = repo.GetByID(ctx, r.ID)
	must(t, err)
	if len(got.Items) != 1 || got.Items[0].Name != "bread" || len(got.Items[0].Claims) != 0 {
		t.Fatalf("after replace = %+v", got.Items)
	}

	// Guarded status transition.
	must(t, repo.TransitionStatus(ctx, r.ID, domain.ReceiptParsed, domain.ReceiptFinalized))
	if err := repo.TransitionStatus(ctx, r.ID, domain.ReceiptParsed, domain.ReceiptFinalized); !errors.Is(err, domain.ErrFailedPrecondition) {
		t.Fatalf("second transition: %v", err)
	}

	// Manual receipts have no file row.
	m := &Receipt{Kind: KindExpense, Receipt: domain.Receipt{GroupID: group, PayerID: alice, Status: domain.ReceiptParsed, Currency: "JPY"}}
	must(t, repo.Create(ctx, m, nil))
	if _, err := repo.GetFile(ctx, m.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("file of manual receipt: %v", err)
	}
	list, err := repo.ListByGroup(ctx, group, 10, 0)
	must(t, err)
	if len(list) != 2 || list[0].ID != m.ID || list[0].Kind != KindExpense {
		t.Fatalf("list = %+v", list)
	}
}

func testLedger(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	reset(t, ctx, pool)
	l := NewLedgerRepo(pool)
	has, err := l.GroupHasActivity(ctx, group)
	must(t, err)
	if has {
		t.Fatal("empty group has activity")
	}
	// bob owes alice 30, carol owes alice 20; bob pays back 10.
	must(t, l.AddDebts(ctx, []domain.Debt{
		{GroupID: group, FromUser: bob, ToUser: alice, Amount: 3000, Currency: "USD"},
		{GroupID: group, FromUser: carol, ToUser: alice, Amount: 2000, Currency: "USD"},
	}))
	st := &domain.Settlement{GroupID: group, FromUser: bob, ToUser: alice, Amount: 1000, Currency: "USD"}
	must(t, l.CreateSettlement(ctx, st))
	if st.ID == "" || st.CreatedAt.IsZero() {
		t.Fatalf("settlement = %+v", st)
	}

	net, err := l.NetBalances(ctx, group)
	must(t, err)
	if net[alice] != 4000 || net[bob] != -2000 || net[carol] != -2000 {
		t.Fatalf("net = %v", net)
	}

	pairs, err := l.PairBalances(ctx, alice)
	must(t, err)
	byOther := map[string]int64{}
	for _, p := range pairs {
		byOther[p.Other] = p.Cents
	}
	if len(pairs) != 2 || byOther[bob] != 2000 || byOther[carol] != 2000 {
		t.Fatalf("alice pairs = %+v", pairs)
	}
	pairs, err = l.PairBalances(ctx, bob)
	must(t, err)
	if len(pairs) != 1 || pairs[0].Other != alice || pairs[0].Cents != -2000 || pairs[0].Currency != "USD" {
		t.Fatalf("bob pairs = %+v", pairs)
	}

	has, err = l.GroupHasActivity(ctx, group)
	must(t, err)
	if !has {
		t.Fatal("group with a settlement has no activity")
	}
}

func testOutboxTx(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	reset(t, ctx, pool)
	tx := postgres.NewTxManager(pool)
	pub := outbox.NewPublisher(pool)
	repo := NewReceiptRepo(pool)
	boom := errors.New("boom")

	err := tx.WithinTx(ctx, func(ctx context.Context) error {
		r := &Receipt{Receipt: domain.Receipt{GroupID: group, PayerID: alice, Status: domain.ReceiptPending, Currency: "USD"}}
		if err := repo.Create(ctx, r, &domain.ReceiptFile{}); err != nil {
			return err
		}
		if err := pub.Publish(ctx, domain.TopicReceiptUploaded, r.ID, domain.ReceiptUploadedEvent{ReceiptID: r.ID}); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatal(err)
	}
	if n := count(t, ctx, pool, `SELECT count(*) FROM outbox`); n != 0 {
		t.Fatalf("outbox rows after rollback = %d", n)
	}
	if n := count(t, ctx, pool, `SELECT count(*) FROM receipts`); n != 0 {
		t.Fatalf("receipts after rollback = %d", n)
	}

	// Through the service: Upload writes the receipt and the event together.
	svc := newTestService(pool)
	r, err := svc.Upload(auth.WithUserID(ctx, alice), UploadInput{GroupID: group, RawText: "MILK 3.00"})
	must(t, err)
	evs := outboxRows(t, ctx, pool, domain.TopicReceiptUploaded)
	if len(evs) != 1 || evs[0]["receipt_id"] != r.ID {
		t.Fatalf("outbox = %v", evs)
	}
}

func testServiceFlow(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	reset(t, ctx, pool)
	svc := newTestService(pool)
	actx := auth.WithUserID(ctx, alice)

	// $100 equally among all three members, paid by alice.
	v, err := svc.CreateExpense(actx, ExpenseInput{GroupID: group, Description: "Rent", AmountCents: 10000, SplitType: SplitEqual})
	must(t, err)
	if v.Receipt.Kind != KindExpense || v.Receipt.Status != domain.ReceiptFinalized || len(v.Shares) != 3 {
		t.Fatalf("view = %+v", v)
	}
	r, err := svc.receipts.GetByID(ctx, v.Receipt.ID)
	must(t, err)
	if r.Kind != KindExpense || r.Status != domain.ReceiptFinalized || r.Store != "Rent" || r.TotalCents != 10000 {
		t.Fatalf("stored = %+v", r)
	}
	net, err := svc.ledger.NetBalances(ctx, group)
	must(t, err)
	if net[alice] != 6666 || net[bob]+net[carol] != -6666 {
		t.Fatalf("net = %v", net)
	}
	fin := outboxRows(t, ctx, pool, domain.TopicReceiptFinalized)
	if len(fin) != 1 || fin[0]["store"] != "Rent" || len(fin[0]["shares"].([]any)) != 3 {
		t.Fatalf("finalized events = %v", fin)
	}
	notes := outboxRows(t, ctx, pool, domain.TopicNotification)
	if len(notes) != 2 || notes[0]["type"] != domain.NotifyExpenseAdded {
		t.Fatalf("notifications = %v", notes)
	}

	// Itemized manual receipt -> claims -> finalize -> receipt_finalized notification.
	rc, err := svc.CreateReceipt(actx, CreateReceiptInput{GroupID: group,
		ItemsUpdate: ItemsUpdate{Store: "Walmart", Items: []domain.ReceiptItem{{Name: "pizza", PriceCents: 884}}}})
	must(t, err)
	if rc.Status != domain.ReceiptParsed || rc.OCRProvider != "manual" || rc.Kind != KindReceipt {
		t.Fatalf("manual = %+v", rc)
	}
	_, err = svc.Claim(auth.WithUserID(ctx, bob), rc.ID, []ClaimInput{{ItemID: rc.Items[0].ID, Shares: 1}})
	must(t, err)
	_, err = svc.Finalize(actx, rc.ID)
	must(t, err)
	if _, err := svc.Finalize(actx, rc.ID); !errors.Is(err, domain.ErrFailedPrecondition) {
		t.Fatalf("double finalize: %v", err)
	}
	notes = outboxRows(t, ctx, pool, domain.TopicNotification)
	last := notes[len(notes)-1]
	if last["type"] != domain.NotifyReceiptFinalized || last["body"] != "You owe $8.84 to Alice for Walmart" {
		t.Fatalf("finalized notification = %v", last)
	}

	// SettleUp: the creditor records money received from bob; bob gets notified.
	st, err := svc.SettleUp(actx, group, bob, alice, 500)
	must(t, err)
	if st.FromUser != bob || st.ToUser != alice {
		t.Fatalf("settlement = %+v", st)
	}
	notes = outboxRows(t, ctx, pool, domain.TopicNotification)
	last = notes[len(notes)-1]
	if last["type"] != domain.NotifySettlement || last["user_ids"].([]any)[0] != bob {
		t.Fatalf("settlement notification = %v", last)
	}
	// A third party can't record a payment between others.
	if _, err := svc.SettleUp(auth.WithUserID(ctx, carol), group, bob, alice, 100); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("third-party settle: %v", err)
	}
}

// ---------- helpers & fakes ----------

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func count(t *testing.T, ctx context.Context, pool *pgxpool.Pool, q string) int {
	t.Helper()
	var n int
	must(t, pool.QueryRow(ctx, q).Scan(&n))
	return n
}

func outboxRows(t *testing.T, ctx context.Context, pool *pgxpool.Pool, topic string) []map[string]any {
	t.Helper()
	rows, err := pool.Query(ctx, `SELECT payload FROM outbox WHERE topic = $1 ORDER BY id`, topic)
	must(t, err)
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var b []byte
		must(t, rows.Scan(&b))
		var m map[string]any
		must(t, json.Unmarshal(b, &m))
		out = append(out, m)
	}
	return out
}

func newTestService(pool *pgxpool.Pool) *Service {
	return NewService(NewReceiptRepo(pool), NewLedgerRepo(pool), postgres.NewTxManager(pool),
		fakeGroups{}, fakeUsers{}, outbox.NewPublisher(pool), fakeFX{})
}

type fakeGroups struct{}

func (fakeGroups) GetGroup(_ context.Context, id string) (clients.GroupInfo, error) {
	if id != group {
		return clients.GroupInfo{}, domain.ErrNotFound
	}
	return clients.GroupInfo{ID: group, Name: "Flat", OwnerID: alice, Currency: "USD", MemberIDs: []string{alice, bob, carol}}, nil
}

type fakeUsers struct{}

var names = map[string]string{alice: "Alice", bob: "Bob", carol: "Carol"}

func (fakeUsers) GetUsers(_ context.Context, ids []string) (map[string]clients.UserInfo, error) {
	out := map[string]clients.UserInfo{}
	for _, id := range ids {
		out[id] = clients.UserInfo{ID: id, Name: names[id], Currency: "USD", Venmo: names[id] + "-v"}
	}
	return out, nil
}

func (u fakeUsers) GetUser(ctx context.Context, id string) (clients.UserInfo, error) {
	m, _ := u.GetUsers(ctx, []string{id})
	return m[id], nil
}

func (fakeUsers) GetUserByEmail(context.Context, string) (clients.UserInfo, error) {
	return clients.UserInfo{}, domain.ErrNotFound
}

type fakeFX struct{}

func (fakeFX) Validate(_ context.Context, code, def string) (string, error) {
	if code == "" {
		return def, nil
	}
	return code, nil
}

func (fakeFX) Rate(context.Context, string, string, time.Time) (float64, error) { return 1, nil }
