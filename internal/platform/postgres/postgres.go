// Package postgres has shared helpers: pool, per-service schema migrations,
// transaction manager and error mapping. Each service owns its own schema
// (database-per-service pattern on a single Postgres instance).
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/Zifeldev/Billsplit/internal/domain"
)

// NewPool connects with search_path set to the service schema, so SQL needs no schema prefix.
func NewPool(ctx context.Context, dsn, schema string, maxConns int32) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse dsn: %w", err)
	}
	cfg.MaxConns = maxConns
	if schema != "" {
		cfg.ConnConfig.RuntimeParams["search_path"] = schema
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return pool, nil
}

// Migrate creates the schema and applies goose migrations from fsys.
func Migrate(ctx context.Context, pool *pgxpool.Pool, schema string, fsys fs.FS) error {
	// In prod the schema is pre-created for the service role (deploy/postgres/init.sql),
	// and the role has no CREATE on the database — so only create it when missing.
	var exists bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_namespace WHERE nspname = $1)`, schema).Scan(&exists); err != nil {
		return fmt.Errorf("check schema: %w", err)
	}
	if !exists {
		if _, err := pool.Exec(ctx, `CREATE SCHEMA `+pgx.Identifier{schema}.Sanitize()); err != nil {
			return fmt.Errorf("create schema: %w", err)
		}
	}
	db := stdlib.OpenDBFromPool(pool)
	defer db.Close()
	p, err := goose.NewProvider(goose.DialectPostgres, db, fsys)
	if err != nil {
		return err
	}
	_, err = p.Up(ctx)
	return err
}

// Querier is the common subset of pgxpool.Pool and pgx.Tx.
type Querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults
}

type txKey struct{}

// TxManager runs fn in one transaction; repositories using DB.Q(ctx) join it.
type TxManager struct{ pool *pgxpool.Pool }

func NewTxManager(pool *pgxpool.Pool) *TxManager { return &TxManager{pool: pool} }

func (m *TxManager) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if _, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return fn(ctx)
	}
	return pgx.BeginFunc(ctx, m.pool, func(tx pgx.Tx) error {
		return fn(context.WithValue(ctx, txKey{}, tx))
	})
}

// DB picks the transaction from ctx if present, otherwise the pool.
type DB struct{ Pool *pgxpool.Pool }

func (b DB) Q(ctx context.Context) Querier {
	if tx, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return tx
	}
	return b.Pool
}

// MapErr converts driver errors to domain errors.
func MapErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
		return domain.ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505": // unique_violation
			return fmt.Errorf("%w: %s", domain.ErrAlreadyExists, pgErr.ConstraintName)
		case "23503", "22P02", "23514": // fk, bad uuid, check
			return fmt.Errorf("%w: %s", domain.ErrInvalidArgument, pgErr.Message)
		}
	}
	return err
}

// Nullable turns "" into SQL NULL.
func Nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// Affected returns ErrNotFound when nothing was updated/deleted.
func Affected(tag pgconn.CommandTag, err error) error {
	if err != nil {
		return MapErr(err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}
