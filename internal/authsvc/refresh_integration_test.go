//go:build integration

package authsvc

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Zifeldev/Billsplit/internal/domain"
	"github.com/Zifeldev/Billsplit/internal/platform/auth"
	"github.com/Zifeldev/Billsplit/internal/platform/testutil"
)

const testSecret = "test-secret-test-secret-test-secret"

func newIssuer(t *testing.T) (*TokenIssuer, *RedisRefreshStore) {
	t.Helper()
	store := NewRedisRefreshStore(testutil.Redis(t))
	jwt := auth.NewJWTManager(testSecret, time.Minute, time.Hour)
	return NewTokenIssuer(jwt, testSecret, time.Hour, store), store
}

func TestRefreshRotation(t *testing.T) {
	ctx := context.Background()
	iss, _ := newIssuer(t)
	const uid = "11111111-1111-1111-1111-111111111111"

	tp, err := iss.Issue(ctx, uid)
	if err != nil {
		t.Fatal(err)
	}
	// Access token stays a normal stateless platform token.
	if got, err := auth.NewJWTManager(testSecret, time.Minute, time.Hour).Parse(tp.Access, auth.TokenAccess); err != nil || got != uid {
		t.Fatalf("access token: %v %v", got, err)
	}
	// Refresh token is also parseable by platform/auth (backward compatible).
	if _, err := auth.NewJWTManager(testSecret, time.Minute, time.Hour).Parse(tp.Refresh, auth.TokenRefresh); err != nil {
		t.Fatalf("refresh token not compatible: %v", err)
	}

	got, err := iss.Rotate(ctx, tp.Refresh)
	if err != nil || got != uid {
		t.Fatalf("rotate: %v %v", got, err)
	}
	tp2, err := iss.Issue(ctx, uid)
	if err != nil {
		t.Fatal(err)
	}
	if tp2.Refresh == tp.Refresh {
		t.Fatal("refresh token must change on rotation")
	}
	if _, err := iss.Rotate(ctx, tp2.Refresh); err != nil {
		t.Fatalf("new token must work: %v", err)
	}
}

func TestRefreshReuseRevokesAll(t *testing.T) {
	ctx := context.Background()
	iss, _ := newIssuer(t)
	const uid = "22222222-2222-2222-2222-222222222222"

	stolen, _ := iss.Issue(ctx, uid)
	otherDevice, _ := iss.Issue(ctx, uid)
	if _, err := iss.Rotate(ctx, stolen.Refresh); err != nil { // legit use
		t.Fatal(err)
	}
	rotated, _ := iss.Issue(ctx, uid)

	// Replay of the consumed token = theft signal.
	if _, err := iss.Rotate(ctx, stolen.Refresh); !errors.Is(err, domain.ErrUnauthenticated) {
		t.Fatalf("reuse must fail, got %v", err)
	}
	for name, tok := range map[string]string{"rotated": rotated.Refresh, "other device": otherDevice.Refresh} {
		if _, err := iss.Rotate(ctx, tok); !errors.Is(err, domain.ErrUnauthenticated) {
			t.Fatalf("%s token must be revoked after reuse, got %v", name, err)
		}
	}
	// Other users are unaffected.
	tp, _ := iss.Issue(ctx, "33333333-3333-3333-3333-333333333333")
	if _, err := iss.Rotate(ctx, tp.Refresh); err != nil {
		t.Fatal(err)
	}
}

func TestLogout(t *testing.T) {
	ctx := context.Background()
	iss, _ := newIssuer(t)
	const uid = "44444444-4444-4444-4444-444444444444"

	tp, _ := iss.Issue(ctx, uid)
	keep, _ := iss.Issue(ctx, uid)
	if err := iss.Revoke(ctx, tp.Refresh, "someone-else"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("foreign logout must be forbidden, got %v", err)
	}
	if err := iss.Revoke(ctx, tp.Refresh, uid); err != nil {
		t.Fatal(err)
	}
	if err := iss.Revoke(ctx, tp.Refresh, uid); err != nil { // idempotent
		t.Fatal(err)
	}
	if _, err := iss.Rotate(ctx, tp.Refresh); !errors.Is(err, domain.ErrUnauthenticated) {
		t.Fatalf("logged-out token must not refresh, got %v", err)
	}
	if err := iss.Revoke(ctx, "garbage", uid); !errors.Is(err, domain.ErrUnauthenticated) {
		t.Fatalf("garbage token: %v", err)
	}
	if _, err := iss.Rotate(ctx, keep.Refresh); err != nil {
		t.Fatalf("logout must not touch other sessions: %v", err)
	}
}

func TestRefreshRejectsAccessToken(t *testing.T) {
	ctx := context.Background()
	iss, _ := newIssuer(t)
	tp, _ := iss.Issue(ctx, "55555555-5555-5555-5555-555555555555")
	if _, err := iss.Rotate(ctx, tp.Access); !errors.Is(err, domain.ErrUnauthenticated) {
		t.Fatalf("access token must not refresh, got %v", err)
	}
}

func TestRefreshStoreTTL(t *testing.T) {
	ctx := context.Background()
	rdb := testutil.Redis(t)
	store := NewRedisRefreshStore(rdb)
	if err := store.Save(ctx, "u1", "j1", 2*time.Second); err != nil {
		t.Fatal(err)
	}
	if ttl := rdb.PTTL(ctx, refreshKey("j1")).Val(); ttl <= 0 || ttl > 2*time.Second {
		t.Fatalf("ttl = %v", ttl)
	}
	if ttl := rdb.PTTL(ctx, userRefreshKey("u1")).Val(); ttl <= 0 {
		t.Fatalf("user set ttl = %v", ttl)
	}
	if res, err := store.Consume(ctx, "u2", "j1"); err != nil || res != RefreshUnknown {
		t.Fatalf("foreign user consume: %v %v", res, err)
	}
	if res, err := store.Consume(ctx, "u1", "j1"); err != nil || res != RefreshLive {
		t.Fatalf("consume: %v %v", res, err)
	}
	if ttl := rdb.PTTL(ctx, refreshKey("j1")).Val(); ttl <= 0 {
		t.Fatalf("used marker must keep ttl, got %v", ttl)
	}
	if res, _ := store.Consume(ctx, "u1", "j1"); res != RefreshReused {
		t.Fatalf("second consume = %v, want reused", res)
	}
	if err := store.RevokeAll(ctx, "u1"); err != nil {
		t.Fatal(err)
	}
	if n := rdb.Exists(ctx, refreshKey("j1"), userRefreshKey("u1")).Val(); n != 0 {
		t.Fatalf("revoke-all left %d keys", n)
	}
}
