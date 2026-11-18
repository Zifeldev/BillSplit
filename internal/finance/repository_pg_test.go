//go:build integration

package finance

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Zifeldev/Billsplit/internal/clients"
	"github.com/Zifeldev/Billsplit/internal/domain"
	"github.com/Zifeldev/Billsplit/internal/platform/auth"
	"github.com/Zifeldev/Billsplit/internal/platform/cache"
	"github.com/Zifeldev/Billsplit/internal/platform/outbox"
	"github.com/Zifeldev/Billsplit/internal/platform/postgres"
	"github.com/Zifeldev/Billsplit/internal/platform/testutil"
	"github.com/Zifeldev/Billsplit/migrations"
)

const (
	userA = "11111111-1111-1111-1111-111111111111"
	userB = "22222222-2222-2222-2222-222222222222"
)

// Each test gets its own container (removed via t.Cleanup) to stay isolated.
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	return testutil.Postgres(t, "finance", migrations.For("finance"))
}

// ---------- fakes ----------

type memCache struct {
	mu sync.Mutex
	m  map[string][]byte
	n  map[string]int64
}

func newMemCache() *memCache { return &memCache{m: map[string][]byte{}, n: map[string]int64{}} }

func (c *memCache) Get(_ context.Context, key string, dst any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	b, ok := c.m[key]
	if !ok {
		return cache.ErrMiss
	}
	return json.Unmarshal(b, dst)
}

func (c *memCache) Set(_ context.Context, key string, v any, _ time.Duration) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[key] = b
	return nil
}

func (c *memCache) Delete(_ context.Context, keys ...string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, k := range keys {
		delete(c.m, k)
		delete(c.n, k)
	}
	return nil
}

func (c *memCache) Incr(_ context.Context, key string) (int64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.n[key]++
	return c.n[key], nil
}

func (c *memCache) GetInt(_ context.Context, key string) (int64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.n[key], nil
}

// SetNX makes memCache a Locker too.
func (c *memCache) SetNX(_ context.Context, key string, _ time.Duration) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.m[key]; ok {
		return false, nil
	}
	c.m[key] = []byte("1")
	return true, nil
}

type fakeFX struct{}

func (fakeFX) Currencies(context.Context) (map[string]string, error) {
	return map[string]string{"USD": "US Dollar", "EUR": "Euro", "JPY": "Japanese Yen"}, nil
}

func (f fakeFX) Validate(ctx context.Context, code, def string) (string, error) {
	code = strings.ToUpper(code)
	if code == "" {
		return def, nil
	}
	m, _ := f.Currencies(ctx)
	if _, ok := m[code]; !ok {
		return "", domain.ErrInvalidArgument
	}
	return code, nil
}

func (fakeFX) Rate(_ context.Context, from, to string, _ time.Time) (float64, error) {
	if from == to {
		return 1, nil
	}
	if from == "EUR" && to == "USD" {
		return 1.1, nil
	}
	return 0, errors.New("no rate")
}

type fakeUsers struct{ clients.Users }

func (fakeUsers) GetUser(_ context.Context, id string) (clients.UserInfo, error) {
	return clients.UserInfo{ID: id, Currency: "USD"}, nil
}

type fakeSplit struct{ clients.Split }

func newTestService(p *pgxpool.Pool, c *memCache) *Service {
	return NewService(NewRepo(p), postgres.NewTxManager(p), fakeUsers{}, fakeSplit{}, outbox.NewPublisher(p),
		fakeFX{}, c, c, time.Minute)
}

func outboxRows(t *testing.T, p *pgxpool.Pool, topic string) []json.RawMessage {
	t.Helper()
	rows, err := p.Query(context.Background(), `SELECT payload FROM outbox WHERE topic = $1 ORDER BY id`, topic)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []json.RawMessage
	for rows.Next() {
		var b []byte
		if err := rows.Scan(&b); err != nil {
			t.Fatal(err)
		}
		out = append(out, b)
	}
	return out
}

// ---------- tests ----------

func TestRepoInsertTransactionsDedup(t *testing.T) {
	p := testPool(t)
	r := NewRepo(p)
	ctx := context.Background()
	at := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	ts := []domain.Transaction{
		{UserID: userA, AmountCents: -100, Currency: "USD", CategoryID: "other", Source: domain.SourceStatement, DedupKey: "k1", OccurredAt: at},
		{UserID: userA, AmountCents: -200, Currency: "USD", CategoryID: "other", Source: domain.SourceStatement, DedupKey: "k2", OccurredAt: at},
	}
	n, err := r.InsertTransactions(ctx, ts)
	if err != nil || n != 2 {
		t.Fatalf("first insert: n=%d err=%v", n, err)
	}
	ts = append(ts, domain.Transaction{UserID: userA, AmountCents: -300, Currency: "USD", CategoryID: "other",
		Source: domain.SourceStatement, DedupKey: "k3", OccurredAt: at})
	n, err = r.InsertTransactions(ctx, ts)
	if err != nil || n != 1 {
		t.Fatalf("re-import: n=%d err=%v (want only k3)", n, err)
	}
	// Same dedup key for another user is a different transaction.
	n, err = r.InsertTransactions(ctx, []domain.Transaction{{UserID: userB, AmountCents: -100, Currency: "USD",
		CategoryID: "other", Source: domain.SourceStatement, DedupKey: "k1", OccurredAt: at}})
	if err != nil || n != 1 {
		t.Fatalf("other user: n=%d err=%v", n, err)
	}
}

func TestRepoAggregatesByCurrency(t *testing.T) {
	p := testPool(t)
	r := NewRepo(p)
	ctx := context.Background()
	at := time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)
	for _, x := range []struct {
		amount   int64
		cur, cat string
	}{
		{500000, "USD", "income"}, {-12000, "USD", "groceries"}, {-3000, "USD", "groceries"},
		{-5000, "EUR", "dining"}, {-99999, "USD", "transfer"}, {-2000, "JPY", "dining"},
	} {
		if err := r.CreateTransaction(ctx, &domain.Transaction{UserID: userA, AmountCents: x.amount, Currency: x.cur,
			CategoryID: x.cat, Source: domain.SourceManual, OccurredAt: at}); err != nil {
			t.Fatal(err)
		}
	}
	from, to := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	totals, err := r.IncomeExpense(ctx, userA, from, to)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]domain.CurrencyTotals{}
	for _, x := range totals {
		got[x.Currency] = x
	}
	if len(got) != 3 || got["USD"].IncomeCents != 500000 || got["USD"].ExpenseCents != 15000 ||
		got["EUR"].ExpenseCents != 5000 || got["JPY"].ExpenseCents != 2000 {
		t.Fatalf("totals %+v (transfers must be excluded)", got)
	}
	cats, err := r.SpendByCategory(ctx, userA, from, to)
	if err != nil {
		t.Fatal(err)
	}
	dining := map[string]int64{}
	for _, c := range cats {
		if c.CategoryID == "dining" {
			dining[c.Currency] = c.SpentCents
		}
	}
	if dining["EUR"] != 5000 || dining["JPY"] != 2000 {
		t.Fatalf("dining by currency %+v", dining)
	}
}

func TestServiceUpdateTransaction(t *testing.T) {
	p := testPool(t)
	c := newMemCache()
	svc := newTestService(p, c)
	ctx := auth.WithUserID(context.Background(), userA)
	created, err := svc.CreateTransaction(ctx, TxInput{AmountCents: -1000, Description: "lunch", Merchant: "cafe"})
	if err != nil {
		t.Fatal(err)
	}
	ver, _ := c.GetInt(ctx, dashVersionKey(userA))

	amount, cur, cat := int64(-2500), "eur", "dining"
	upd, err := svc.UpdateTransaction(ctx, created.ID, TxPatch{AmountCents: &amount, Currency: &cur, CategoryID: &cat})
	if err != nil {
		t.Fatal(err)
	}
	if upd.AmountCents != -2500 || upd.Currency != "EUR" || upd.CategoryID != "dining" ||
		upd.Description != "lunch" || upd.Merchant != "CAFE" {
		t.Fatalf("updated %+v", upd)
	}
	stored, err := svc.repo.GetTransaction(ctx, userA, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.AmountCents != -2500 || stored.Currency != "EUR" || stored.CategoryID != "dining" || stored.Description != "lunch" {
		t.Fatalf("stored %+v", stored)
	}
	if v, _ := c.GetInt(ctx, dashVersionKey(userA)); v != ver+1 {
		t.Fatalf("dashboard version not bumped: %d -> %d", ver, v)
	}

	zero, bad, badCat := int64(0), "XXX", "nope"
	for name, patch := range map[string]TxPatch{"zero": {AmountCents: &zero}, "currency": {Currency: &bad}, "category": {CategoryID: &badCat}} {
		if _, err := svc.UpdateTransaction(ctx, created.ID, patch); !errors.Is(err, domain.ErrInvalidArgument) {
			t.Errorf("%s: want invalid argument, got %v", name, err)
		}
	}
	other := auth.WithUserID(context.Background(), userB)
	if _, err := svc.UpdateTransaction(other, created.ID, TxPatch{AmountCents: &amount}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("foreign transaction: %v", err)
	}
}

func TestServiceImportsOutboxAndList(t *testing.T) {
	p := testPool(t)
	svc := newTestService(p, newMemCache())
	ctx := auth.WithUserID(context.Background(), userA)

	var ids []string
	for i := range 3 {
		imp, err := svc.ImportStatement(ctx, []byte("Date,Description,Amount\n"), 0, "chase", "")
		if err != nil {
			t.Fatal(err)
		}
		if imp.Status != domain.ImportPending || imp.Currency != "USD" || imp.File != nil {
			t.Fatalf("import %d: %+v", i, imp)
		}
		ids = append(ids, imp.ID)
	}
	events := outboxRows(t, p, domain.TopicStatementUploaded)
	if len(events) != 3 {
		t.Fatalf("want 3 outbox rows, got %d", len(events))
	}
	var ev domain.StatementUploadedEvent
	if err := json.Unmarshal(events[0], &ev); err != nil || ev.ImportID != ids[0] || ev.UserID != userA {
		t.Fatalf("event %+v err=%v", ev, err)
	}

	list, err := svc.ListImports(ctx, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].ID != ids[2] || list[1].ID != ids[1] || list[0].File != nil {
		t.Fatalf("list %+v, want newest first: %v", list, ids)
	}
	if list, _ := svc.ListImports(auth.WithUserID(context.Background(), userB), 0); len(list) != 0 {
		t.Fatalf("other user sees %d imports", len(list))
	}
}

func TestOutboxRolledBackWithTx(t *testing.T) {
	p := testPool(t)
	r, txm, pub := NewRepo(p), postgres.NewTxManager(p), outbox.NewPublisher(p)
	ctx := context.Background()
	boom := errors.New("boom")
	err := txm.WithinTx(ctx, func(ctx context.Context) error {
		imp := &domain.StatementImport{UserID: userA, Currency: "USD", File: []byte("x")}
		if err := r.CreateImport(ctx, imp); err != nil {
			return err
		}
		if err := pub.Publish(ctx, domain.TopicStatementUploaded, userA, domain.StatementUploadedEvent{ImportID: imp.ID}); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatal(err)
	}
	if n := len(outboxRows(t, p, domain.TopicStatementUploaded)); n != 0 {
		t.Fatalf("outbox row survived rollback: %d", n)
	}
	if imps, _ := r.ListImports(ctx, userA, 10); len(imps) != 0 {
		t.Fatalf("import survived rollback")
	}
}

func TestServiceDeleteGoalAndBudget(t *testing.T) {
	p := testPool(t)
	svc := newTestService(p, newMemCache())
	ctx := auth.WithUserID(context.Background(), userA)
	g, err := svc.SetGoal(ctx, &domain.SavingsGoal{Name: "Trip", TargetCents: 100000})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteGoal(auth.WithUserID(context.Background(), userB), g.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("non-owner delete: %v", err)
	}
	if err := svc.DeleteGoal(ctx, g.ID); err != nil {
		t.Fatal(err)
	}
	if gs, _ := svc.ListGoals(ctx); len(gs) != 0 {
		t.Fatalf("goal not deleted")
	}

	if _, err := svc.SetBudget(ctx, "dining", 30000); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteBudget(ctx, "dining"); err != nil {
		t.Fatal(err)
	}
	if bs, _ := svc.ListBudgets(ctx); len(bs) != 0 {
		t.Fatalf("budget not deleted: %+v", bs)
	}
}

func TestRecurringRemindersDeduped(t *testing.T) {
	p := testPool(t)
	c := newMemCache()
	svc := newTestService(p, c)
	ctx := context.Background()
	now := time.Date(2026, 10, 28, 9, 0, 0, 0, time.UTC)
	// Netflix every 30 days, the next charge falls on tomorrow.
	tomorrow := time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC)
	var ts []domain.Transaction
	for i := 1; i <= 4; i++ {
		ts = append(ts, domain.Transaction{UserID: userA, AmountCents: -1549, Currency: "USD", Merchant: "NETFLIX.COM",
			CategoryID: "subscriptions", Source: domain.SourceStatement, DedupKey: "nf" + string(rune('0'+i)),
			OccurredAt: tomorrow.AddDate(0, 0, -30*i)})
	}
	// Another user with an unrelated expense.
	ts = append(ts, domain.Transaction{UserID: userB, AmountCents: -500, Currency: "EUR", CategoryID: "dining",
		Source: domain.SourceManual, OccurredAt: now.AddDate(0, 0, -1)})
	if _, err := svc.repo.InsertTransactions(ctx, ts); err != nil {
		t.Fatal(err)
	}

	users, err := svc.repo.UsersWithExpenses(ctx, now.Add(-reminderActiveFor), "", 10)
	if err != nil || len(users) != 2 || users[0] != userA || users[1] != userB {
		t.Fatalf("users %v err=%v", users, err)
	}
	if rest, _ := svc.repo.UsersWithExpenses(ctx, now.Add(-reminderActiveFor), userA, 10); len(rest) != 1 || rest[0] != userB {
		t.Fatalf("keyset page %v", rest)
	}

	n, err := svc.SendRecurringReminders(ctx, now)
	if err != nil || n != 1 {
		t.Fatalf("first pass: n=%d err=%v", n, err)
	}
	n, err = svc.SendRecurringReminders(ctx, now.Add(time.Hour))
	if err != nil || n != 0 {
		t.Fatalf("second pass must be deduped: n=%d err=%v", n, err)
	}
	rows := outboxRows(t, p, domain.TopicNotification)
	if len(rows) != 1 {
		t.Fatalf("want 1 notification, got %d", len(rows))
	}
	var ev domain.NotificationEvent
	if err := json.Unmarshal(rows[0], &ev); err != nil {
		t.Fatal(err)
	}
	if ev.Type != domain.NotifyRecurringUpcoming || len(ev.UserIDs) != 1 || ev.UserIDs[0] != userA ||
		ev.Body != "Netflix will charge ~$15.49 tomorrow" {
		t.Fatalf("event %+v", ev)
	}
}
