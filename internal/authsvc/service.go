package authsvc

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"

	"github.com/Zifeldev/Billsplit/internal/domain"
	"github.com/Zifeldev/Billsplit/internal/platform/auth"
	"github.com/Zifeldev/Billsplit/internal/platform/logger"
)

// CurrencyValidator is satisfied by fx.Client.
type CurrencyValidator interface {
	Validate(ctx context.Context, code, def string) (string, error)
}

type Service struct {
	users  UserRepository
	tokens *TokenIssuer
	fx     CurrencyValidator
}

func NewService(users UserRepository, tokens *TokenIssuer, fx CurrencyValidator) *Service {
	return &Service{users: users, tokens: tokens, fx: fx}
}

type AuthResult struct {
	Tokens auth.TokenPair
	User   *User
}

func (s *Service) Register(ctx context.Context, email, password, name, currency string) (*AuthResult, error) {
	email = normEmail(email)
	if _, err := mail.ParseAddress(email); err != nil {
		return nil, fmt.Errorf("%w: invalid email", domain.ErrInvalidArgument)
	}
	if len(password) < 8 || len(password) > 72 { // bcrypt limit
		return nil, fmt.Errorf("%w: password must be 8..72 characters", domain.ErrInvalidArgument)
	}
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 100 {
		return nil, fmt.Errorf("%w: name is required", domain.ErrInvalidArgument)
	}
	currency, err := s.fx.Validate(ctx, currency, "USD")
	if err != nil {
		return nil, err
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		return nil, err
	}
	u := &User{User: domain.User{Email: email, PasswordHash: hash, Name: name, Currency: currency}}
	if err := s.users.Create(ctx, u); err != nil {
		if errors.Is(err, domain.ErrAlreadyExists) {
			return nil, fmt.Errorf("%w: email already registered", domain.ErrAlreadyExists)
		}
		return nil, err
	}
	logger.FromContext(ctx).Info("user registered", "user_id", u.ID)
	return s.issue(ctx, u)
}

func (s *Service) Login(ctx context.Context, email, password string) (*AuthResult, error) {
	u, err := s.users.GetByEmail(ctx, normEmail(email))
	if errors.Is(err, domain.ErrNotFound) {
		auth.DummyCheck(password) // same timing as a real check
		return nil, domain.ErrInvalidCredentials
	}
	if err != nil {
		return nil, err
	}
	if !auth.CheckPassword(u.PasswordHash, password) {
		logger.FromContext(ctx).Warn("failed login", "user_id", u.ID)
		return nil, domain.ErrInvalidCredentials
	}
	return s.issue(ctx, u)
}

// Refresh rotates the refresh token: the old one is consumed (single use).
func (s *Service) Refresh(ctx context.Context, refreshToken string) (*AuthResult, error) {
	uid, err := s.tokens.Rotate(ctx, refreshToken)
	if err != nil {
		if errors.Is(err, domain.ErrUnauthenticated) {
			logger.FromContext(ctx).Warn("refresh rejected", "err", err)
		}
		return nil, err
	}
	u, err := s.users.GetByID(ctx, uid)
	if err != nil {
		return nil, domain.ErrUnauthenticated
	}
	return s.issue(ctx, u)
}

// Logout revokes the given refresh token. The access token simply expires.
func (s *Service) Logout(ctx context.Context, refreshToken string) error {
	uid, _ := auth.UserID(ctx) // the gateway requires a JWT; empty for internal callers
	return s.tokens.Revoke(ctx, refreshToken, uid)
}

func (s *Service) Me(ctx context.Context) (*User, error) {
	uid, err := auth.UserID(ctx)
	if err != nil {
		return nil, err
	}
	return s.users.GetByID(ctx, uid)
}

func (s *Service) UpdateProfile(ctx context.Context, name, currency string, pay PaymentUpdate) (*User, error) {
	u, err := s.Me(ctx)
	if err != nil {
		return nil, err
	}
	if name = strings.TrimSpace(name); name != "" {
		if len(name) > 100 {
			return nil, fmt.Errorf("%w: name is too long", domain.ErrInvalidArgument)
		}
		u.Name = name
	}
	if err := pay.apply(&u.Payment); err != nil {
		return nil, err
	}
	if u.Currency, err = s.fx.Validate(ctx, currency, u.Currency); err != nil {
		return nil, err
	}
	if err := s.users.Update(ctx, u); err != nil {
		return nil, err
	}
	return u, nil
}

func (s *Service) GetUsers(ctx context.Context, ids []string) ([]User, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	if len(ids) > 500 {
		return nil, fmt.Errorf("%w: too many ids", domain.ErrInvalidArgument)
	}
	return s.users.GetByIDs(ctx, ids)
}

func (s *Service) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	return s.users.GetByEmail(ctx, normEmail(email))
}

func (s *Service) issue(ctx context.Context, u *User) (*AuthResult, error) {
	tp, err := s.tokens.Issue(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	return &AuthResult{Tokens: tp, User: u}, nil
}

func normEmail(e string) string { return strings.ToLower(strings.TrimSpace(e)) }
