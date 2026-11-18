// Package split is the split microservice: receipts, item claims, debts,
// settlements and balances.
package split

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Zifeldev/Billsplit/internal/domain"
	"github.com/Zifeldev/Billsplit/internal/platform/postgres"
)

// ReceiptKind distinguishes itemized receipts from single-amount expenses
// (values match pb.ReceiptKind).
type ReceiptKind int16

const (
	KindReceipt ReceiptKind = 1
	KindExpense ReceiptKind = 2
)

// Receipt is domain.Receipt plus split-only fields.
type Receipt struct {
	domain.Receipt
	Kind ReceiptKind
}

type ReceiptRepository interface {
	// Create inserts the receipt header; f is optional (nil for manual receipts and expenses).
	Create(ctx context.Context, r *Receipt, f *domain.ReceiptFile) error
	GetByID(ctx context.Context, id string) (*Receipt, error)
	GetFile(ctx context.Context, receiptID string) (*domain.ReceiptFile, error)
	ListByGroup(ctx context.Context, groupID string, limit, offset int) ([]Receipt, error)
	// ReplaceItems overwrites items and header fields (claims are dropped). Item IDs are filled in.
	ReplaceItems(ctx context.Context, r *Receipt) error
	SetStatus(ctx context.Context, id string, status domain.ReceiptStatus, parseErr string) error
	// TransitionStatus changes the status only if it is still `from`
	// (ErrFailedPrecondition otherwise): guards against double finalization.
	TransitionStatus(ctx context.Context, id string, from, to domain.ReceiptStatus) error
	SetClaim(ctx context.Context, itemID, userID string, shares int32) error
	Delete(ctx context.Context, id string) error
	// TouchStalePending bumps updated_at of PENDING receipts not touched since
	// `idle` (but created after `since`) and returns their ids — for re-queueing OCR.
	TouchStalePending(ctx context.Context, idle time.Duration, since time.Time, limit int) ([]string, error)
	// FailPendingBefore marks PENDING receipts created before t as FAILED.
	FailPendingBefore(ctx context.Context, t time.Time, msg string) (int64, error)
}

// Amount is a signed sum in one currency.
type Amount struct {
	Currency string
	Cents    int64
}

// PairBalance is the net between the user and one counterparty in one currency.
type PairBalance struct {
	Other string
	Amount
}

type LedgerRepository interface {
	AddDebts(ctx context.Context, debts []domain.Debt) error
	CreateSettlement(ctx context.Context, s *domain.Settlement) error
	// NetBalances: userID -> net cents in the group (positive = is owed).
	NetBalances(ctx context.Context, groupID string) (map[string]int64, error)
	// GroupNets: per group net of the user (for owed/owe totals), with currency.
	GroupNets(ctx context.Context, userID string) ([]Amount, error)
	PairBalances(ctx context.Context, userID string) ([]PairBalance, error)
	GroupHasActivity(ctx context.Context, groupID string) (bool, error)
	// PairHistory returns every debt and settlement between exactly users a and b
	// (in either direction, across all groups), newest first.
	PairHistory(ctx context.Context, a, b string) ([]PairEvent, error)
	// LockPair serializes writes for the pair {a, b} until the current tx ends
	// (call inside a tx).
	LockPair(ctx context.Context, a, b string) error
}

// PairEvent is one ledger row between two users: a debt created by a
// receipt/expense, or a settlement. Amount is always positive (from owes/paid to).
type PairEvent struct {
	GroupID    string
	Currency   string
	Settlement bool
	ReceiptID  string      // debts only; "" if the debt has no receipt
	Kind       ReceiptKind // debts only
	Title      string      // receipt store / expense description
	FromUser   string
	ToUser     string
	Amount     int64
	OccurredAt time.Time
}

// ================= postgres =================

type PGReceiptRepo struct{ postgres.DB }

func NewReceiptRepo(p *pgxpool.Pool) *PGReceiptRepo { return &PGReceiptRepo{postgres.DB{Pool: p}} }

func (r *PGReceiptRepo) Create(ctx context.Context, rc *Receipt, f *domain.ReceiptFile) error {
	if rc.Kind == 0 {
		rc.Kind = KindReceipt
	}
	err := r.Q(ctx).QueryRow(ctx,
		`INSERT INTO receipts (group_id, payer_id, kind, store, status, subtotal_cents, tax_cents, tip_cents, total_cents,
		                       currency, ocr_provider, purchased_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12) RETURNING id, created_at`,
		rc.GroupID, rc.PayerID, rc.Kind, rc.Store, rc.Status, rc.SubtotalCents, rc.TaxCents, rc.TipCents, rc.TotalCents,
		rc.Currency, rc.OCRProvider, rc.PurchasedAt,
	).Scan(&rc.ID, &rc.CreatedAt)
	if err != nil {
		return postgres.MapErr(err)
	}
	if f == nil {
		return nil
	}
	_, err = r.Q(ctx).Exec(ctx,
		`INSERT INTO receipt_files (receipt_id, content_type, data, raw_text) VALUES ($1, $2, $3, $4)`,
		rc.ID, f.ContentType, f.Data, f.RawText)
	return postgres.MapErr(err)
}

const receiptCols = `id, group_id, payer_id, store, status, subtotal_cents, tax_cents, tip_cents,
	total_cents, currency, parse_error, ocr_provider, purchased_at, created_at, kind`

func scanReceipt(row pgx.Row, rc *Receipt) error {
	return row.Scan(&rc.ID, &rc.GroupID, &rc.PayerID, &rc.Store, &rc.Status, &rc.SubtotalCents, &rc.TaxCents,
		&rc.TipCents, &rc.TotalCents, &rc.Currency, &rc.ParseError, &rc.OCRProvider, &rc.PurchasedAt, &rc.CreatedAt, &rc.Kind)
}

func (r *PGReceiptRepo) GetByID(ctx context.Context, id string) (*Receipt, error) {
	var rc Receipt
	if err := scanReceipt(r.Q(ctx).QueryRow(ctx, `SELECT `+receiptCols+` FROM receipts WHERE id = $1`, id), &rc); err != nil {
		return nil, postgres.MapErr(err)
	}
	rows, err := r.Q(ctx).Query(ctx,
		`SELECT i.id, i.name, i.raw_name, i.code, i.quantity, i.price_cents, c.user_id, c.shares
		   FROM receipt_items i LEFT JOIN item_claims c ON c.item_id = i.id
		  WHERE i.receipt_id = $1 ORDER BY i.position, c.user_id`, id)
	if err != nil {
		return nil, postgres.MapErr(err)
	}
	defer rows.Close()
	for rows.Next() {
		var it domain.ReceiptItem
		var uid *string
		var shares *int32
		if err := rows.Scan(&it.ID, &it.Name, &it.RawName, &it.Code, &it.Quantity, &it.PriceCents, &uid, &shares); err != nil {
			return nil, err
		}
		if n := len(rc.Items); n == 0 || rc.Items[n-1].ID != it.ID {
			rc.Items = append(rc.Items, it)
		}
		if uid != nil {
			last := &rc.Items[len(rc.Items)-1]
			last.Claims = append(last.Claims, domain.ItemClaim{UserID: *uid, Shares: *shares})
		}
	}
	return &rc, postgres.MapErr(rows.Err())
}

func (r *PGReceiptRepo) GetFile(ctx context.Context, receiptID string) (*domain.ReceiptFile, error) {
	f := domain.ReceiptFile{ReceiptID: receiptID}
	err := r.Q(ctx).QueryRow(ctx,
		`SELECT content_type, COALESCE(data, ''::bytea), raw_text FROM receipt_files WHERE receipt_id = $1`, receiptID,
	).Scan(&f.ContentType, &f.Data, &f.RawText)
	if err != nil {
		return nil, postgres.MapErr(err)
	}
	return &f, nil
}

func (r *PGReceiptRepo) ListByGroup(ctx context.Context, groupID string, limit, offset int) ([]Receipt, error) {
	rows, err := r.Q(ctx).Query(ctx,
		`SELECT `+receiptCols+` FROM receipts WHERE group_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		groupID, limit, offset)
	if err != nil {
		return nil, postgres.MapErr(err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Receipt, error) {
		var rc Receipt
		err := scanReceipt(row, &rc)
		return rc, err
	})
}

func (r *PGReceiptRepo) ReplaceItems(ctx context.Context, rc *Receipt) error {
	q := r.Q(ctx)
	if _, err := q.Exec(ctx, `DELETE FROM receipt_items WHERE receipt_id = $1`, rc.ID); err != nil {
		return postgres.MapErr(err)
	}
	b := &pgx.Batch{}
	for i := range rc.Items {
		it := &rc.Items[i]
		b.Queue(`INSERT INTO receipt_items (receipt_id, position, name, quantity, price_cents, raw_name, code)
		         VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
			rc.ID, i, it.Name, it.Quantity, it.PriceCents, it.RawName, it.Code).
			QueryRow(func(row pgx.Row) error { return row.Scan(&it.ID) })
	}
	b.Queue(`UPDATE receipts SET store = $2, status = $3, subtotal_cents = $4, tax_cents = $5, tip_cents = $6,
	                total_cents = $7, parse_error = $8, purchased_at = $9, currency = $10, ocr_provider = $11, updated_at = now()
	          WHERE id = $1`,
		rc.ID, rc.Store, rc.Status, rc.SubtotalCents, rc.TaxCents, rc.TipCents, rc.TotalCents, rc.ParseError,
		rc.PurchasedAt, rc.Currency, rc.OCRProvider)
	return postgres.MapErr(q.SendBatch(ctx, b).Close())
}

func (r *PGReceiptRepo) SetStatus(ctx context.Context, id string, status domain.ReceiptStatus, parseErr string) error {
	return postgres.Affected(r.Q(ctx).Exec(ctx,
		`UPDATE receipts SET status = $2, parse_error = $3, updated_at = now() WHERE id = $1`, id, status, parseErr))
}

func (r *PGReceiptRepo) TransitionStatus(ctx context.Context, id string, from, to domain.ReceiptStatus) error {
	err := postgres.Affected(r.Q(ctx).Exec(ctx,
		`UPDATE receipts SET status = $3, parse_error = '', updated_at = now() WHERE id = $1 AND status = $2`, id, from, to))
	if errors.Is(err, domain.ErrNotFound) {
		return fmt.Errorf("%w: receipt status changed concurrently", domain.ErrFailedPrecondition)
	}
	return err
}

func (r *PGReceiptRepo) SetClaim(ctx context.Context, itemID, userID string, shares int32) error {
	var err error
	if shares <= 0 {
		_, err = r.Q(ctx).Exec(ctx, `DELETE FROM item_claims WHERE item_id = $1 AND user_id = $2`, itemID, userID)
	} else {
		_, err = r.Q(ctx).Exec(ctx,
			`INSERT INTO item_claims (item_id, user_id, shares) VALUES ($1, $2, $3)
			 ON CONFLICT (item_id, user_id) DO UPDATE SET shares = EXCLUDED.shares`, itemID, userID, shares)
	}
	return postgres.MapErr(err)
}

func (r *PGReceiptRepo) Delete(ctx context.Context, id string) error {
	return postgres.Affected(r.Q(ctx).Exec(ctx, `DELETE FROM receipts WHERE id = $1`, id))
}

type PGLedgerRepo struct{ postgres.DB }

func NewLedgerRepo(p *pgxpool.Pool) *PGLedgerRepo { return &PGLedgerRepo{postgres.DB{Pool: p}} }

func (r *PGLedgerRepo) AddDebts(ctx context.Context, debts []domain.Debt) error {
	b := &pgx.Batch{}
	for _, d := range debts {
		b.Queue(`INSERT INTO debts (group_id, receipt_id, from_user, to_user, amount, currency) VALUES ($1, $2, $3, $4, $5, $6)`,
			d.GroupID, postgres.Nullable(d.ReceiptID), d.FromUser, d.ToUser, d.Amount, d.Currency)
	}
	return postgres.MapErr(r.Q(ctx).SendBatch(ctx, b).Close())
}

func (r *PGLedgerRepo) CreateSettlement(ctx context.Context, s *domain.Settlement) error {
	return postgres.MapErr(r.Q(ctx).QueryRow(ctx,
		`INSERT INTO settlements (group_id, from_user, to_user, amount, currency) VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, created_at`,
		s.GroupID, s.FromUser, s.ToUser, s.Amount, s.Currency).Scan(&s.ID, &s.CreatedAt))
}

// Debts increase the creditor's balance; settlements (debtor paying back) do the opposite.
func (r *PGLedgerRepo) NetBalances(ctx context.Context, groupID string) (map[string]int64, error) {
	rows, err := r.Q(ctx).Query(ctx, `
		SELECT user_id, SUM(delta)::bigint FROM (
			SELECT to_user AS user_id, amount AS delta FROM debts WHERE group_id = $1
			UNION ALL SELECT from_user, -amount FROM debts       WHERE group_id = $1
			UNION ALL SELECT from_user,  amount FROM settlements WHERE group_id = $1
			UNION ALL SELECT to_user,   -amount FROM settlements WHERE group_id = $1
		) t GROUP BY user_id`, groupID)
	if err != nil {
		return nil, postgres.MapErr(err)
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var uid string
		var net int64
		if err := rows.Scan(&uid, &net); err != nil {
			return nil, err
		}
		out[uid] = net
	}
	return out, postgres.MapErr(rows.Err())
}

func (r *PGLedgerRepo) GroupNets(ctx context.Context, userID string) ([]Amount, error) {
	rows, err := r.Q(ctx).Query(ctx, `
		SELECT currency, SUM(delta)::bigint FROM (
			SELECT group_id, currency,  amount AS delta FROM debts WHERE to_user = $1
			UNION ALL SELECT group_id, currency, -amount FROM debts       WHERE from_user = $1
			UNION ALL SELECT group_id, currency,  amount FROM settlements WHERE from_user = $1
			UNION ALL SELECT group_id, currency, -amount FROM settlements WHERE to_user   = $1
		) t GROUP BY group_id, currency HAVING SUM(delta) <> 0`, userID)
	if err != nil {
		return nil, postgres.MapErr(err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Amount, error) {
		var a Amount
		err := row.Scan(&a.Currency, &a.Cents)
		return a, err
	})
}

func (r *PGLedgerRepo) PairBalances(ctx context.Context, userID string) ([]PairBalance, error) {
	rows, err := r.Q(ctx).Query(ctx, `
		SELECT other, currency, SUM(delta)::bigint FROM (
			SELECT from_user AS other, currency,  amount AS delta FROM debts WHERE to_user = $1
			UNION ALL SELECT to_user,   currency, -amount FROM debts       WHERE from_user = $1
			UNION ALL SELECT to_user,   currency,  amount FROM settlements WHERE from_user = $1
			UNION ALL SELECT from_user, currency, -amount FROM settlements WHERE to_user   = $1
		) t GROUP BY other, currency HAVING SUM(delta) <> 0`, userID)
	if err != nil {
		return nil, postgres.MapErr(err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (PairBalance, error) {
		var p PairBalance
		err := row.Scan(&p.Other, &p.Currency, &p.Cents)
		return p, err
	})
}

func (r *PGLedgerRepo) GroupHasActivity(ctx context.Context, groupID string) (bool, error) {
	var ok bool
	err := r.Q(ctx).QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM receipts WHERE group_id = $1 AND status = $2)
	                                  OR EXISTS (SELECT 1 FROM settlements WHERE group_id = $1)`,
		groupID, domain.ReceiptFinalized).Scan(&ok)
	return ok, postgres.MapErr(err)
}

func (r *PGLedgerRepo) PairHistory(ctx context.Context, a, b string) ([]PairEvent, error) {
	rows, err := r.Q(ctx).Query(ctx, `
		SELECT d.group_id::text, d.currency, false, d.receipt_id::text, COALESCE(rc.kind, $3::smallint),
		       COALESCE(rc.store, ''), d.from_user::text, d.to_user::text, d.amount,
		       COALESCE(rc.purchased_at, rc.created_at, d.created_at) AS at
		  FROM debts d LEFT JOIN receipts rc ON rc.id = d.receipt_id
		 WHERE (d.from_user = $1 AND d.to_user = $2) OR (d.from_user = $2 AND d.to_user = $1)
		UNION ALL
		SELECT s.group_id::text, s.currency, true, NULL, 0::smallint, '', s.from_user::text, s.to_user::text, s.amount,
		       s.created_at
		  FROM settlements s
		 WHERE (s.from_user = $1 AND s.to_user = $2) OR (s.from_user = $2 AND s.to_user = $1)
		ORDER BY at DESC`, a, b, KindExpense)
	if err != nil {
		return nil, postgres.MapErr(err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (PairEvent, error) {
		var e PairEvent
		var receiptID *string
		err := row.Scan(&e.GroupID, &e.Currency, &e.Settlement, &receiptID, &e.Kind, &e.Title,
			&e.FromUser, &e.ToUser, &e.Amount, &e.OccurredAt)
		if receiptID != nil {
			e.ReceiptID = *receiptID
		}
		return e, err
	})
}

func (r *PGLedgerRepo) LockPair(ctx context.Context, a, b string) error {
	if b < a {
		a, b = b, a
	}
	_, err := r.Q(ctx).Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('split:pair:' || $1 || ':' || $2, 0))`, a, b)
	return postgres.MapErr(err)
}

func (r *PGReceiptRepo) TouchStalePending(ctx context.Context, idle time.Duration, since time.Time, limit int) ([]string, error) {
	// FOR UPDATE SKIP LOCKED: several split replicas never re-queue the same receipt twice at once.
	rows, err := r.Q(ctx).Query(ctx, `
		UPDATE receipts SET updated_at = now()
		 WHERE id IN (SELECT id FROM receipts
		               WHERE status = $1 AND updated_at < now() - $2::interval AND created_at > $3
		               ORDER BY created_at LIMIT $4 FOR UPDATE SKIP LOCKED)
		RETURNING id`, domain.ReceiptPending, idle.String(), since, limit)
	if err != nil {
		return nil, postgres.MapErr(err)
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

func (r *PGReceiptRepo) FailPendingBefore(ctx context.Context, t time.Time, msg string) (int64, error) {
	tag, err := r.Q(ctx).Exec(ctx,
		`UPDATE receipts SET status = $1, parse_error = $2, updated_at = now() WHERE status = $3 AND created_at < $4`,
		domain.ReceiptFailed, msg, domain.ReceiptPending, t)
	if err != nil {
		return 0, postgres.MapErr(err)
	}
	return tag.RowsAffected(), nil
}
