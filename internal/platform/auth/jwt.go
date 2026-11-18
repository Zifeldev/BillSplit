package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/Zifeldev/Billsplit/internal/domain"
)

const (
	TokenAccess  = "access"
	TokenRefresh = "refresh"
)

type Claims struct {
	Type string `json:"typ"`
	jwt.RegisteredClaims
}

type JWTManager struct {
	secret     []byte
	accessTTL  time.Duration
	refreshTTL time.Duration
}

func NewJWTManager(secret string, accessTTL, refreshTTL time.Duration) *JWTManager {
	return &JWTManager{secret: []byte(secret), accessTTL: accessTTL, refreshTTL: refreshTTL}
}

type TokenPair struct {
	Access          string
	Refresh         string
	AccessExpiresAt time.Time
}

func (m *JWTManager) Issue(userID string) (TokenPair, error) {
	now := time.Now()
	access, accessExp, err := m.sign(userID, TokenAccess, now, m.accessTTL)
	if err != nil {
		return TokenPair{}, err
	}
	refresh, _, err := m.sign(userID, TokenRefresh, now, m.refreshTTL)
	if err != nil {
		return TokenPair{}, err
	}
	return TokenPair{Access: access, Refresh: refresh, AccessExpiresAt: accessExp}, nil
}

func (m *JWTManager) sign(userID, typ string, now time.Time, ttl time.Duration) (string, time.Time, error) {
	exp := now.Add(ttl)
	claims := Claims{
		Type: typ,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			Issuer:    "billsplit",
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
		},
	}
	s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.secret)
	return s, exp, err
}

// Parse validates the token and its type and returns the user ID.
func (m *JWTManager) Parse(token, expectedType string) (string, error) {
	var c Claims
	_, err := jwt.ParseWithClaims(token, &c, func(t *jwt.Token) (any, error) {
		return m.secret, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithIssuer("billsplit"))
	if err != nil {
		return "", fmt.Errorf("%w: %v", domain.ErrUnauthenticated, err)
	}
	if c.Type != expectedType {
		return "", fmt.Errorf("%w: wrong token type", domain.ErrUnauthenticated)
	}
	return c.Subject, nil
}

func HashPassword(pw string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	return string(b), err
}

// dummyHash lets login spend the same bcrypt time for unknown emails (no user enumeration by timing).
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("billsplit-dummy-password"), bcrypt.DefaultCost)

func DummyCheck(pw string) { _ = bcrypt.CompareHashAndPassword(dummyHash, []byte(pw)) }

func CheckPassword(hash, pw string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil
}

type userIDKey struct{}

func WithUserID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, userIDKey{}, id)
}

// UserID returns the authenticated user from ctx (set by the auth interceptor).
func UserID(ctx context.Context) (string, error) {
	id, ok := ctx.Value(userIDKey{}).(string)
	if !ok || id == "" {
		return "", domain.ErrUnauthenticated
	}
	return id, nil
}
