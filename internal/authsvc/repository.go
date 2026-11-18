// Package authsvc is the auth microservice: registration, login, JWT issuing
// and the internal user directory used by other services.
package authsvc

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Zifeldev/Billsplit/internal/domain"
	"github.com/Zifeldev/Billsplit/internal/platform/postgres"
)

// User is the auth-side view of a user: the shared domain fields plus the
// payment handles that only the auth schema stores.
type User struct {
	domain.User
	Payment PaymentHandles
}

type UserRepository interface {
	Create(ctx context.Context, u *User) error
	GetByID(ctx context.Context, id string) (*User, error)
	GetByIDs(ctx context.Context, ids []string) ([]User, error)
	GetByEmail(ctx context.Context, email string) (*User, error)
	Update(ctx context.Context, u *User) error
}

type PGUserRepo struct{ postgres.DB }

func NewUserRepo(pool *pgxpool.Pool) *PGUserRepo { return &PGUserRepo{postgres.DB{Pool: pool}} }

const userCols = `id, email, password_hash, name, currency, created_at, venmo, paypal, zelle`

func scanUser(row pgx.Row) (User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Name, &u.Currency, &u.CreatedAt,
		&u.Payment.Venmo, &u.Payment.PayPal, &u.Payment.Zelle)
	return u, err
}

func (r *PGUserRepo) Create(ctx context.Context, u *User) error {
	err := r.Q(ctx).QueryRow(ctx,
		`INSERT INTO users (email, password_hash, name, currency) VALUES ($1, $2, $3, $4) RETURNING id, created_at`,
		u.Email, u.PasswordHash, u.Name, u.Currency,
	).Scan(&u.ID, &u.CreatedAt)
	return postgres.MapErr(err)
}

func (r *PGUserRepo) GetByID(ctx context.Context, id string) (*User, error) {
	u, err := scanUser(r.Q(ctx).QueryRow(ctx, `SELECT `+userCols+` FROM users WHERE id = $1`, id))
	if err != nil {
		return nil, postgres.MapErr(err)
	}
	return &u, nil
}

func (r *PGUserRepo) GetByIDs(ctx context.Context, ids []string) ([]User, error) {
	rows, err := r.Q(ctx).Query(ctx, `SELECT `+userCols+` FROM users WHERE id = ANY($1::uuid[])`, ids)
	if err != nil {
		return nil, postgres.MapErr(err)
	}
	us, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (User, error) { return scanUser(row) })
	return us, postgres.MapErr(err)
}

func (r *PGUserRepo) GetByEmail(ctx context.Context, email string) (*User, error) {
	u, err := scanUser(r.Q(ctx).QueryRow(ctx, `SELECT `+userCols+` FROM users WHERE email = $1`, email))
	if err != nil {
		return nil, postgres.MapErr(err)
	}
	return &u, nil
}

func (r *PGUserRepo) Update(ctx context.Context, u *User) error {
	return postgres.Affected(r.Q(ctx).Exec(ctx,
		`UPDATE users SET name = $2, currency = $3, venmo = $4, paypal = $5, zelle = $6 WHERE id = $1`,
		u.ID, u.Name, u.Currency, u.Payment.Venmo, u.Payment.PayPal, u.Payment.Zelle))
}
