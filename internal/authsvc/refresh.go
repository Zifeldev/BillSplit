package authsvc

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/Zifeldev/Billsplit/internal/domain"
	"github.com/Zifeldev/Billsplit/internal/platform/auth"
	"github.com/Zifeldev/Billsplit/internal/platform/logger"
)

// Refresh tokens are JWTs (same secret/format as platform/auth) that also carry
// a unique jti. Every issued jti is registered in Redis; Refresh consumes it
// (single use) and issues a new pair. A consumed jti is kept as a "used"
// marker until the token would expire: presenting it again is a theft signal
// (someone else holds a copy), so all of the user's refresh tokens are revoked.
// Logout deletes the jti. Access tokens stay stateless.

// ConsumeResult is the state of a refresh token id at the moment of use.
type ConsumeResult int

const (
	RefreshUnknown ConsumeResult = iota // never issued, logged out, revoked or expired
	RefreshLive                         // valid: now marked used
	RefreshReused                       // already used once: theft signal
)

// RefreshStore keeps refresh token ids.
type RefreshStore interface {
	Save(ctx context.Context, userID, jti string, ttl time.Duration) error
	// Consume atomically marks a live jti as used.
	Consume(ctx context.Context, userID, jti string) (ConsumeResult, error)
	Revoke(ctx context.Context, userID, jti string) error
	RevokeAll(ctx context.Context, userID string) error
}

// RedisRefreshStore: auth:refresh:<jti> -> user id, or "used" after rotation
// (TTL = refresh TTL), plus a per-user set auth:refresh:user:<uid> of jtis for revoke-all.
type RedisRefreshStore struct{ rdb *redis.Client }

func NewRedisRefreshStore(rdb *redis.Client) *RedisRefreshStore { return &RedisRefreshStore{rdb: rdb} }

func refreshKey(jti string) string     { return "auth:refresh:" + jti }
func userRefreshKey(uid string) string { return "auth:refresh:user:" + uid }

// saveScript extends the per-user set TTL only upwards so it outlives every member.
var saveScript = redis.NewScript(`
redis.call('SET', KEYS[1], ARGV[1], 'PX', ARGV[3])
redis.call('SADD', KEYS[2], ARGV[2])
local ttl = redis.call('PTTL', KEYS[2])
if ttl < tonumber(ARGV[3]) then redis.call('PEXPIRE', KEYS[2], ARGV[3]) end
return 1`)

// consumeScript: 1 = was live (now "used", TTL kept), 2 = already used, 0 = unknown.
var consumeScript = redis.NewScript(`
local v = redis.call('GET', KEYS[1])
if v == ARGV[1] then redis.call('SET', KEYS[1], 'used', 'KEEPTTL'); return 1 end
if v == 'used' then return 2 end
return 0`)

var revokeAllScript = redis.NewScript(`
local ids = redis.call('SMEMBERS', KEYS[1])
for _, id in ipairs(ids) do redis.call('DEL', ARGV[1] .. id) end
redis.call('DEL', KEYS[1])
return #ids`)

func (s *RedisRefreshStore) Save(ctx context.Context, userID, jti string, ttl time.Duration) error {
	return saveScript.Run(ctx, s.rdb, []string{refreshKey(jti), userRefreshKey(userID)},
		userID, jti, ttl.Milliseconds()).Err()
}

func (s *RedisRefreshStore) Consume(ctx context.Context, userID, jti string) (ConsumeResult, error) {
	n, err := consumeScript.Run(ctx, s.rdb, []string{refreshKey(jti)}, userID).Int()
	if err != nil {
		return RefreshUnknown, err
	}
	return ConsumeResult(n), nil
}

func (s *RedisRefreshStore) Revoke(ctx context.Context, userID, jti string) error {
	_, err := s.rdb.TxPipelined(ctx, func(p redis.Pipeliner) error {
		p.Del(ctx, refreshKey(jti))
		p.SRem(ctx, userRefreshKey(userID), jti)
		return nil
	})
	return err
}

func (s *RedisRefreshStore) RevokeAll(ctx context.Context, userID string) error {
	return revokeAllScript.Run(ctx, s.rdb, []string{userRefreshKey(userID)}, refreshKey("")).Err()
}

// TokenIssuer issues access tokens via platform/auth and its own refresh tokens with a jti.
type TokenIssuer struct {
	jwt        *auth.JWTManager
	secret     []byte
	refreshTTL time.Duration
	store      RefreshStore
}

func NewTokenIssuer(jwt *auth.JWTManager, secret string, refreshTTL time.Duration, store RefreshStore) *TokenIssuer {
	return &TokenIssuer{jwt: jwt, secret: []byte(secret), refreshTTL: refreshTTL, store: store}
}

// Issue returns a new access/refresh pair and registers the refresh jti.
func (t *TokenIssuer) Issue(ctx context.Context, userID string) (auth.TokenPair, error) {
	tp, err := t.jwt.Issue(userID) // its refresh token has no jti; replaced below
	if err != nil {
		return auth.TokenPair{}, err
	}
	now, jti := time.Now(), uuid.NewString()
	claims := auth.Claims{
		Type: auth.TokenRefresh,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        jti,
			Subject:   userID,
			Issuer:    "billsplit",
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(t.refreshTTL)),
		},
	}
	if tp.Refresh, err = jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(t.secret); err != nil {
		return auth.TokenPair{}, err
	}
	if err := t.store.Save(ctx, userID, jti, t.refreshTTL); err != nil {
		return auth.TokenPair{}, fmt.Errorf("save refresh token: %w", err)
	}
	return tp, nil
}

type refreshClaims struct {
	UserID, JTI string
	Expired     bool
}

// parse validates signature, issuer and type. Expired tokens are reported
// (Expired=true) only when allowExpired is set, for idempotent logout.
func (t *TokenIssuer) parse(token string, allowExpired bool) (refreshClaims, error) {
	var c auth.Claims
	_, err := jwt.ParseWithClaims(token, &c, func(*jwt.Token) (any, error) { return t.secret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithIssuer("billsplit"))
	expired := errors.Is(err, jwt.ErrTokenExpired)
	if err != nil && !(allowExpired && expired) {
		return refreshClaims{}, fmt.Errorf("%w: %v", domain.ErrUnauthenticated, err)
	}
	if c.Type != auth.TokenRefresh || c.Subject == "" || c.ID == "" {
		return refreshClaims{}, fmt.Errorf("%w: not a refresh token", domain.ErrUnauthenticated)
	}
	return refreshClaims{UserID: c.Subject, JTI: c.ID, Expired: expired}, nil
}

// Rotate consumes a refresh token and returns its user id. Reusing a consumed
// token revokes every refresh token of that user.
func (t *TokenIssuer) Rotate(ctx context.Context, token string) (string, error) {
	c, err := t.parse(token, false)
	if err != nil {
		return "", err
	}
	res, err := t.store.Consume(ctx, c.UserID, c.JTI)
	if err != nil {
		return "", err
	}
	switch res {
	case RefreshLive:
		return c.UserID, nil
	case RefreshReused:
		if err := t.store.RevokeAll(ctx, c.UserID); err != nil {
			return "", err
		}
		logger.FromContext(ctx).Warn("refresh token reuse detected, all sessions revoked", "user_id", c.UserID)
		return "", fmt.Errorf("%w: refresh token reused; please log in again", domain.ErrUnauthenticated)
	default:
		return "", fmt.Errorf("%w: refresh token revoked", domain.ErrUnauthenticated)
	}
}

// Revoke deletes the refresh token (logout). Already-expired tokens are a no-op.
func (t *TokenIssuer) Revoke(ctx context.Context, token, callerID string) error {
	c, err := t.parse(token, true)
	if err != nil {
		return err
	}
	if callerID != "" && callerID != c.UserID {
		return fmt.Errorf("%w: refresh token belongs to another user", domain.ErrForbidden)
	}
	if c.Expired {
		return nil
	}
	return t.store.Revoke(ctx, c.UserID, c.JTI)
}
