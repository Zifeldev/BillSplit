//go:build integration

package social

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
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

func newPool(t *testing.T) *pgxpool.Pool {
	return testutil.Postgres(t, "social", migrations.For("social"))
}

func TestFriendPairOrdering(t *testing.T) {
	ctx := context.Background()
	r := NewFriendRepo(newPool(t))
	a, b := uuid.NewString(), uuid.NewString()

	if err := r.CreateRequest(ctx, b, a); err != nil { // either order
		t.Fatal(err)
	}
	if err := r.CreateRequest(ctx, a, b); !errors.Is(err, domain.ErrAlreadyExists) {
		t.Fatalf("one row per pair, got %v", err)
	}
	for _, pair := range [][2]string{{a, b}, {b, a}} {
		f, err := r.Get(ctx, pair[0], pair[1])
		if err != nil {
			t.Fatal(err)
		}
		if f.Other != pair[1] || f.RequestedBy != b || f.Accepted {
			t.Fatalf("Get(%v) = %+v", pair, f)
		}
	}
	if err := r.Accept(ctx, a, b); err != nil {
		t.Fatal(err)
	}
	fs, err := r.List(ctx, b)
	if err != nil || len(fs) != 1 || fs[0].Other != a || !fs[0].Accepted || fs[0].AcceptedAt == nil {
		t.Fatalf("List = %+v, %v", fs, err)
	}
	if err := r.Delete(ctx, b, a); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Get(ctx, a, b); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("want not found, got %v", err)
	}
}

func newGroup(t *testing.T, pool *pgxpool.Pool, owner string) *domain.Group {
	t.Helper()
	ctx := context.Background()
	gr := NewGroupRepo(pool)
	g := &domain.Group{Name: "Trip", OwnerID: owner, Currency: "USD"}
	if err := gr.Create(ctx, g); err != nil {
		t.Fatal(err)
	}
	if err := gr.AddMember(ctx, g.ID, owner, domain.RoleOwner); err != nil {
		t.Fatal(err)
	}
	return g
}

func TestInvitesLockAndUses(t *testing.T) {
	ctx := context.Background()
	pool := newPool(t)
	owner := uuid.NewString()
	g := newGroup(t, pool, owner)
	ir, tx := NewInviteRepo(pool), postgres.NewTxManager(pool)

	inv := &domain.Invite{Token: "tok-1", Kind: domain.InviteGroup, InviterID: owner, GroupID: g.ID,
		ExpiresAt: time.Now().Add(time.Hour), MaxUses: 2}
	if err := ir.Create(ctx, inv); err != nil {
		t.Fatal(err)
	}
	// Concurrent redemptions must serialize on the row lock and never exceed max_uses.
	errs := make(chan error, 5)
	for range 5 {
		go func() {
			errs <- tx.WithinTx(ctx, func(ctx context.Context) error {
				got, err := ir.GetForUpdate(ctx, "tok-1")
				if err != nil {
					return err
				}
				if !got.Usable(time.Now()) {
					return domain.ErrFailedPrecondition
				}
				time.Sleep(20 * time.Millisecond)
				return ir.IncUses(ctx, "tok-1")
			})
		}()
	}
	ok := 0
	for range 5 {
		if err := <-errs; err == nil {
			ok++
		} else if !errors.Is(err, domain.ErrFailedPrecondition) {
			t.Fatal(err)
		}
	}
	got, err := ir.Get(ctx, "tok-1")
	if err != nil {
		t.Fatal(err)
	}
	if ok != 2 || got.Uses != 2 || got.Usable(time.Now()) {
		t.Fatalf("ok=%d uses=%d", ok, got.Uses)
	}
	if _, err := ir.GetForUpdate(ctx, "missing"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("want not found, got %v", err)
	}
	if err := ir.IncUses(ctx, "missing"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("want not found, got %v", err)
	}
}

func TestListActiveByGroup(t *testing.T) {
	ctx := context.Background()
	pool := newPool(t)
	owner := uuid.NewString()
	g, other := newGroup(t, pool, owner), newGroup(t, pool, owner)
	ir := NewInviteRepo(pool)
	future := time.Now().Add(time.Hour)

	mk := func(token, groupID string, exp time.Time, maxUses int32) {
		t.Helper()
		if err := ir.Create(ctx, &domain.Invite{Token: token, Kind: domain.InviteGroup, InviterID: owner,
			GroupID: groupID, ExpiresAt: exp, MaxUses: maxUses}); err != nil {
			t.Fatal(err)
		}
	}
	mk("active-unlimited", g.ID, future, 0)
	mk("active-limited", g.ID, future, 2)
	mk("expired", g.ID, time.Now().Add(-time.Minute), 0)
	mk("revoked", g.ID, future, 0)
	mk("used-up", g.ID, future, 1)
	mk("other-group", other.ID, future, 0)
	if err := ir.Create(ctx, &domain.Invite{Token: "friend", Kind: domain.InviteFriend, InviterID: owner, ExpiresAt: future}); err != nil {
		t.Fatal(err)
	}
	if err := ir.Revoke(ctx, "revoked"); err != nil {
		t.Fatal(err)
	}
	if err := ir.IncUses(ctx, "used-up"); err != nil {
		t.Fatal(err)
	}
	if err := ir.IncUses(ctx, "active-limited"); err != nil {
		t.Fatal(err)
	}

	got, err := ir.ListActiveByGroup(ctx, g.ID)
	if err != nil {
		t.Fatal(err)
	}
	tokens := map[string]bool{}
	for _, i := range got {
		tokens[i.Token] = true
	}
	if len(got) != 2 || !tokens["active-unlimited"] || !tokens["active-limited"] {
		t.Fatalf("got %v", tokens)
	}
}

// ---------- service with outbox ----------

type fakeUsers map[string]clients.UserInfo

func (f fakeUsers) GetUsers(_ context.Context, ids []string) (map[string]clients.UserInfo, error) {
	out := map[string]clients.UserInfo{}
	for _, id := range ids {
		if u, ok := f[id]; ok {
			out[id] = u
		}
	}
	return out, nil
}

func (f fakeUsers) GetUser(_ context.Context, id string) (clients.UserInfo, error) {
	if u, ok := f[id]; ok {
		return u, nil
	}
	return clients.UserInfo{}, domain.ErrNotFound
}

func (f fakeUsers) GetUserByEmail(_ context.Context, email string) (clients.UserInfo, error) {
	for _, u := range f {
		if u.Email == email {
			return u, nil
		}
	}
	return clients.UserInfo{}, domain.ErrNotFound
}

type noCache struct{}

func (noCache) Get(context.Context, string, any) error                { return cache.ErrMiss }
func (noCache) Set(context.Context, string, any, time.Duration) error { return nil }
func (noCache) Delete(context.Context, ...string) error               { return nil }
func (noCache) Incr(context.Context, string) (int64, error)           { return 0, nil }
func (noCache) GetInt(context.Context, string) (int64, error)         { return 0, cache.ErrMiss }

type usdOnly struct{}

func (usdOnly) Validate(_ context.Context, code, def string) (string, error) {
	if code == "" {
		return def, nil
	}
	return code, nil
}

func outboxEvents(t *testing.T, pool *pgxpool.Pool) []domain.NotificationEvent {
	t.Helper()
	rows, err := pool.Query(context.Background(), `SELECT payload FROM outbox WHERE topic = $1 ORDER BY id`, domain.TopicNotification)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []domain.NotificationEvent
	for rows.Next() {
		var b []byte
		if err := rows.Scan(&b); err != nil {
			t.Fatal(err)
		}
		var ev domain.NotificationEvent
		if err := json.Unmarshal(b, &ev); err != nil {
			t.Fatal(err)
		}
		out = append(out, ev)
	}
	return out
}

func TestServiceNotificationsAndPayments(t *testing.T) {
	pool := newPool(t)
	alice := clients.UserInfo{ID: uuid.NewString(), Name: "Alice", Email: "alice@x.com", Venmo: "alice-v"}
	bob := clients.UserInfo{ID: uuid.NewString(), Name: "Bob", Email: "bob@x.com", PayPal: "bobpp"}
	carol := clients.UserInfo{ID: uuid.NewString(), Name: "Carol", Email: "carol@x.com", Zelle: "4155550123"}
	users := fakeUsers{alice.ID: alice, bob.ID: bob, carol.ID: carol}
	svc := NewService(NewFriendRepo(pool), NewGroupRepo(pool), NewInviteRepo(pool), postgres.NewTxManager(pool),
		users, nil, usdOnly{}, noCache{}, time.Minute, LinkBuilder{WebURL: "https://w", Scheme: "bs"}, outbox.NewPublisher(pool))
	as := func(u clients.UserInfo) context.Context { return auth.WithUserID(context.Background(), u.ID) }

	// friend_request -> Bob; pending friends carry no payment handles.
	f, err := svc.SendFriendRequest(as(alice), "bob@x.com")
	if err != nil {
		t.Fatal(err)
	}
	if f.Payment != nil {
		t.Fatal("pending friend must not expose payment handles")
	}
	// friend_accepted -> Alice
	f, err = svc.RespondFriendRequest(as(bob), alice.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if f.Payment == nil || f.Payment.Venmo != "alice-v" {
		t.Fatalf("accepted friend payment = %+v", f.Payment)
	}
	acc, _, _, err := svc.ListFriends(as(alice))
	if err != nil || len(acc) != 1 || acc[0].Payment == nil || acc[0].Payment.PayPal != "bobpp" {
		t.Fatalf("ListFriends = %+v %v", acc, err)
	}

	g, err := svc.CreateGroup(as(alice), "Trip", "", []string{bob.ID})
	if err != nil {
		t.Fatal(err)
	}
	if p := g.Payments[bob.ID]; p == nil || p.PayPal != "bobpp" {
		t.Fatalf("member payment = %+v", p)
	}
	inv, err := svc.CreateGroupInvite(as(alice), g.ID, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if invs, err := svc.ListGroupInvites(as(bob), g.ID); err != nil || len(invs) != 1 {
		t.Fatalf("ListGroupInvites = %v %v", invs, err)
	}
	if _, err := svc.ListGroupInvites(as(carol), g.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("non-member must not list invites, got %v", err)
	}

	// Anonymous preview works; already_member is false.
	p, err := svc.PreviewInvite(context.Background(), inv.Token)
	if err != nil || p.AlreadyMember || p.GroupName != "Trip" || p.MembersCount != 2 {
		t.Fatalf("anonymous preview = %+v %v", p, err)
	}
	if p, err := svc.PreviewInvite(as(bob), inv.Token); err != nil || !p.AlreadyMember {
		t.Fatalf("member preview = %+v %v", p, err)
	}

	// group_joined -> Alice and Bob
	if _, err := svc.JoinGroup(as(carol), inv.Token); err != nil {
		t.Fatal(err)
	}

	evs := outboxEvents(t, pool)
	type want struct {
		typ, body string
		to        []string
	}
	wants := []want{
		{domain.NotifyFriendRequest, "Alice wants to be friends", []string{bob.ID}},
		{domain.NotifyFriendAccepted, "Bob accepted your friend request", []string{alice.ID}},
		{domain.NotifyGroupJoined, "Alice added you to Trip", []string{bob.ID}},
		{domain.NotifyGroupJoined, "Carol joined Trip", []string{alice.ID, bob.ID}},
	}
	if len(evs) != len(wants) {
		t.Fatalf("got %d events: %+v", len(evs), evs)
	}
	for i, w := range wants {
		e := evs[i]
		if e.Type != w.typ || e.Body != w.body || len(e.UserIDs) != len(w.to) {
			t.Fatalf("event %d = %+v, want %+v", i, e, w)
		}
		for j := range w.to {
			if e.UserIDs[j] != w.to[j] {
				t.Fatalf("event %d recipients = %v, want %v", i, e.UserIDs, w.to)
			}
		}
	}
	if evs[3].Data["group_id"] != g.ID || evs[0].Data["user_id"] != alice.ID {
		t.Fatalf("data = %v / %v", evs[3].Data, evs[0].Data)
	}
}

type failingPublisher struct{}

func (failingPublisher) Publish(context.Context, string, string, any) error {
	return errors.New("outbox down")
}

// The notification is written in the same tx: if it fails, the business change rolls back.
func TestNotificationSharesTransaction(t *testing.T) {
	pool := newPool(t)
	alice := clients.UserInfo{ID: uuid.NewString(), Name: "Alice", Email: "alice@x.com"}
	bob := clients.UserInfo{ID: uuid.NewString(), Name: "Bob", Email: "bob@x.com"}
	svc := NewService(NewFriendRepo(pool), NewGroupRepo(pool), NewInviteRepo(pool), postgres.NewTxManager(pool),
		fakeUsers{alice.ID: alice, bob.ID: bob}, nil, usdOnly{}, noCache{}, time.Minute, LinkBuilder{}, failingPublisher{})
	ctx := auth.WithUserID(context.Background(), alice.ID)
	if _, err := svc.SendFriendRequest(ctx, "bob@x.com"); err == nil {
		t.Fatal("want error")
	}
	if _, err := NewFriendRepo(pool).Get(context.Background(), alice.ID, bob.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("friend request must be rolled back, got %v", err)
	}
}
