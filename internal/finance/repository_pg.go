package finance

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Zifeldev/Billsplit/internal/domain"
	"github.com/Zifeldev/Billsplit/internal/platform/postgres"
)

// PGRepo implements Repository on Postgres.
type PGRepo struct{ postgres.DB }

func NewRepo(pool *pgxpool.Pool) *PGRepo { return &PGRepo{postgres.DB{Pool: pool}} }

// ---------- categories & rules ----------

func (r *PGRepo) ListCategories(ctx context.Context) ([]domain.Category, error) {
	rows, err := r.Q(ctx).Query(ctx, `SELECT id, name, kind, icon FROM categories ORDER BY kind, name`)
	if err != nil {
		return nil, postgres.MapErr(err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.Category, error) {
		var c domain.Category
		err := row.Scan(&c.ID, &c.Name, &c.Kind, &c.Icon)
		return c, err
	})
}

func (r *PGRepo) CategoryExists(ctx context.Context, id string) (bool, error) {
	var ok bool
	err := r.Q(ctx).QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM categories WHERE id = $1)`, id).Scan(&ok)
	return ok, postgres.MapErr(err)
}

func (r *PGRepo) Rules(ctx context.Context, userID string) ([]domain.CategoryRule, error) {
	// User rules first, then longer (more specific) patterns first: "uber eats" beats "uber".
	rows, err := r.Q(ctx).Query(ctx,
		`SELECT pattern, category_id, COALESCE(user_id::text, '')
		   FROM category_rules
		  WHERE user_id = $1 OR user_id IS NULL
		  ORDER BY (user_id IS NULL), length(pattern) DESC`, postgres.Nullable(userID))
	if err != nil {
		return nil, postgres.MapErr(err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.CategoryRule, error) {
		var c domain.CategoryRule
		err := row.Scan(&c.Pattern, &c.CategoryID, &c.UserID)
		return c, err
	})
}

func (r *PGRepo) UpsertRule(ctx context.Context, rule domain.CategoryRule) error {
	_, err := r.Q(ctx).Exec(ctx,
		`INSERT INTO category_rules (user_id, pattern, category_id) VALUES ($1, $2, $3)
		 ON CONFLICT (user_id, pattern) DO UPDATE SET category_id = EXCLUDED.category_id`,
		rule.UserID, rule.Pattern, rule.CategoryID)
	return postgres.MapErr(err)
}

// ---------- transactions ----------

const txCols = `id, user_id, amount_cents, currency, description, merchant, category_id, source,
	COALESCE(dedup_key, ''), occurred_at`

func scanTx(row pgx.Row) (domain.Transaction, error) {
	var t domain.Transaction
	err := row.Scan(&t.ID, &t.UserID, &t.AmountCents, &t.Currency, &t.Description, &t.Merchant,
		&t.CategoryID, &t.Source, &t.DedupKey, &t.OccurredAt)
	return t, err
}

func (r *PGRepo) CreateTransaction(ctx context.Context, t *domain.Transaction) error {
	err := r.Q(ctx).QueryRow(ctx,
		`INSERT INTO transactions (user_id, amount_cents, currency, description, merchant, category_id, source, dedup_key, occurred_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING id`,
		t.UserID, t.AmountCents, t.Currency, t.Description, t.Merchant, t.CategoryID, t.Source,
		postgres.Nullable(t.DedupKey), t.OccurredAt,
	).Scan(&t.ID)
	return postgres.MapErr(err)
}

func (r *PGRepo) InsertTransactions(ctx context.Context, ts []domain.Transaction) (int, error) {
	b := &pgx.Batch{}
	for _, t := range ts {
		b.Queue(`INSERT INTO transactions (user_id, amount_cents, currency, description, merchant, category_id, source, dedup_key, occurred_at)
		         VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		         ON CONFLICT (user_id, dedup_key) DO NOTHING`,
			t.UserID, t.AmountCents, t.Currency, t.Description, t.Merchant, t.CategoryID, t.Source,
			postgres.Nullable(t.DedupKey), t.OccurredAt)
	}
	br := r.Q(ctx).SendBatch(ctx, b)
	defer br.Close()
	inserted := 0
	for range ts {
		tag, err := br.Exec()
		if err != nil {
			return inserted, postgres.MapErr(err)
		}
		inserted += int(tag.RowsAffected())
	}
	return inserted, nil
}

func (r *PGRepo) GetTransaction(ctx context.Context, userID, id string) (*domain.Transaction, error) {
	t, err := scanTx(r.Q(ctx).QueryRow(ctx, `SELECT `+txCols+` FROM transactions WHERE id = $1 AND user_id = $2`, id, userID))
	if err != nil {
		return nil, postgres.MapErr(err)
	}
	return &t, nil
}

func (r *PGRepo) ListTransactions(ctx context.Context, userID string, f domain.TransactionFilter) ([]domain.Transaction, error) {
	rows, err := r.Q(ctx).Query(ctx,
		`SELECT `+txCols+` FROM transactions
		  WHERE user_id = $1 AND occurred_at >= $2 AND occurred_at < $3
		    AND ($4 = '' OR category_id = $4)
		  ORDER BY occurred_at DESC LIMIT $5 OFFSET $6`,
		userID, f.From, f.To, f.CategoryID, f.Limit, f.Offset)
	if err != nil {
		return nil, postgres.MapErr(err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.Transaction, error) { return scanTx(row) })
}

func (r *PGRepo) UpdateTransactionCategory(ctx context.Context, userID, id, categoryID string) error {
	tag, err := r.Q(ctx).Exec(ctx,
		`UPDATE transactions SET category_id = $3 WHERE id = $1 AND user_id = $2`, id, userID, categoryID)
	if err != nil {
		return postgres.MapErr(err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *PGRepo) DeleteTransaction(ctx context.Context, userID, id string) error {
	tag, err := r.Q(ctx).Exec(ctx, `DELETE FROM transactions WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return postgres.MapErr(err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// ---------- statement imports ----------

func (r *PGRepo) CreateImport(ctx context.Context, imp *domain.StatementImport) error {
	err := r.Q(ctx).QueryRow(ctx,
		`INSERT INTO statement_imports (user_id, format, bank, currency, file) VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, status, created_at`,
		imp.UserID, imp.Format, imp.Bank, imp.Currency, imp.File,
	).Scan(&imp.ID, &imp.Status, &imp.CreatedAt)
	return postgres.MapErr(err)
}

func (r *PGRepo) GetImport(ctx context.Context, id string) (*domain.StatementImport, error) {
	var i domain.StatementImport
	err := r.Q(ctx).QueryRow(ctx,
		`SELECT id, user_id, status, format, bank, currency, file, imported, duplicates, error, created_at
		   FROM statement_imports WHERE id = $1`, id,
	).Scan(&i.ID, &i.UserID, &i.Status, &i.Format, &i.Bank, &i.Currency, &i.File, &i.Imported, &i.Duplicates, &i.Error, &i.CreatedAt)
	if err != nil {
		return nil, postgres.MapErr(err)
	}
	return &i, nil
}

func (r *PGRepo) FinishImport(ctx context.Context, id, status string, imported, duplicates int, errMsg string) error {
	_, err := r.Q(ctx).Exec(ctx,
		`UPDATE statement_imports SET status = $2, imported = $3, duplicates = $4, error = $5 WHERE id = $1`,
		id, status, imported, duplicates, errMsg)
	return postgres.MapErr(err)
}

// ---------- budgets & goals ----------

func (r *PGRepo) UpsertBudget(ctx context.Context, userID string, b *domain.Budget) error {
	err := r.Q(ctx).QueryRow(ctx,
		`INSERT INTO budgets (user_id, category_id, limit_cents) VALUES ($1, $2, $3)
		 ON CONFLICT (user_id, category_id) DO UPDATE SET limit_cents = EXCLUDED.limit_cents
		 RETURNING id`, userID, b.CategoryID, b.LimitCents,
	).Scan(&b.ID)
	return postgres.MapErr(err)
}

func (r *PGRepo) ListBudgets(ctx context.Context, userID string) ([]domain.Budget, error) {
	rows, err := r.Q(ctx).Query(ctx,
		`SELECT id, category_id, limit_cents FROM budgets WHERE user_id = $1 ORDER BY category_id`, userID)
	if err != nil {
		return nil, postgres.MapErr(err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.Budget, error) {
		var b domain.Budget
		err := row.Scan(&b.ID, &b.CategoryID, &b.LimitCents)
		return b, err
	})
}

func (r *PGRepo) UpsertGoal(ctx context.Context, g *domain.SavingsGoal) error {
	if g.ID == "" {
		return postgres.MapErr(r.Q(ctx).QueryRow(ctx,
			`INSERT INTO savings_goals (user_id, name, target_cents, saved_cents, deadline, investment_id)
			 VALUES ($1, $2, $3, $4, $5, $6::uuid) RETURNING id`,
			g.UserID, g.Name, g.TargetCents, g.SavedCents, g.Deadline, postgres.Nullable(g.InvestmentID),
		).Scan(&g.ID))
	}
	tag, err := r.Q(ctx).Exec(ctx,
		`UPDATE savings_goals SET name = $3, target_cents = $4, saved_cents = $5, deadline = $6, investment_id = $7::uuid
		  WHERE id = $1 AND user_id = $2`,
		g.ID, g.UserID, g.Name, g.TargetCents, g.SavedCents, g.Deadline, postgres.Nullable(g.InvestmentID))
	if err != nil {
		return postgres.MapErr(err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *PGRepo) ListGoals(ctx context.Context, userID string) ([]domain.SavingsGoal, error) {
	rows, err := r.Q(ctx).Query(ctx,
		`SELECT id, user_id, name, target_cents, saved_cents, deadline, COALESCE(investment_id::text, '')
		   FROM savings_goals WHERE user_id = $1 ORDER BY created_at`, userID)
	if err != nil {
		return nil, postgres.MapErr(err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.SavingsGoal, error) {
		var g domain.SavingsGoal
		err := row.Scan(&g.ID, &g.UserID, &g.Name, &g.TargetCents, &g.SavedCents, &g.Deadline, &g.InvestmentID)
		return g, err
	})
}

// ---------- dashboard aggregates ----------
// Every aggregate is grouped by currency; the service converts to the home currency.
// Transfers between own accounts are excluded from income/expense.

func (r *PGRepo) IncomeExpense(ctx context.Context, userID string, from, to time.Time) ([]domain.CurrencyTotals, error) {
	rows, err := r.Q(ctx).Query(ctx,
		`SELECT currency,
		        COALESCE(SUM(amount_cents) FILTER (WHERE amount_cents > 0), 0)::bigint,
		        COALESCE(-SUM(amount_cents) FILTER (WHERE amount_cents < 0), 0)::bigint
		   FROM transactions
		  WHERE user_id = $1 AND occurred_at >= $2 AND occurred_at < $3 AND category_id <> 'transfer'
		  GROUP BY currency`, userID, from, to)
	if err != nil {
		return nil, postgres.MapErr(err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.CurrencyTotals, error) {
		var t domain.CurrencyTotals
		err := row.Scan(&t.Currency, &t.IncomeCents, &t.ExpenseCents)
		return t, err
	})
}

func (r *PGRepo) SpendByCategory(ctx context.Context, userID string, from, to time.Time) ([]domain.CategoryTotal, error) {
	rows, err := r.Q(ctx).Query(ctx,
		`SELECT t.currency, c.id, c.name, c.kind, (-SUM(t.amount_cents))::bigint
		   FROM transactions t JOIN categories c ON c.id = t.category_id
		  WHERE t.user_id = $1 AND t.occurred_at >= $2 AND t.occurred_at < $3 AND t.amount_cents < 0 AND t.category_id <> 'transfer'
		  GROUP BY t.currency, c.id, c.name, c.kind`, userID, from, to)
	if err != nil {
		return nil, postgres.MapErr(err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.CategoryTotal, error) {
		var c domain.CategoryTotal
		err := row.Scan(&c.Currency, &c.CategoryID, &c.Name, &c.Kind, &c.SpentCents)
		return c, err
	})
}

func (r *PGRepo) SpendByDay(ctx context.Context, userID string, from, to time.Time) ([]domain.DailyTotal, error) {
	rows, err := r.Q(ctx).Query(ctx,
		`SELECT currency, date_trunc('day', occurred_at), (-SUM(amount_cents))::bigint
		   FROM transactions
		  WHERE user_id = $1 AND occurred_at >= $2 AND occurred_at < $3 AND amount_cents < 0
		    AND category_id <> 'transfer'
		  GROUP BY 1, 2 ORDER BY 2`, userID, from, to)
	if err != nil {
		return nil, postgres.MapErr(err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.DailyTotal, error) {
		var d domain.DailyTotal
		err := row.Scan(&d.Currency, &d.Date, &d.SpentCents)
		return d, err
	})
}

func (r *PGRepo) DailyFlows(ctx context.Context, userID string, from, to time.Time) ([]domain.DailyFlow, error) {
	rows, err := r.Q(ctx).Query(ctx,
		`SELECT currency, date_trunc('day', occurred_at),
		        COALESCE(SUM(amount_cents) FILTER (WHERE amount_cents > 0), 0)::bigint,
		        COALESCE(-SUM(amount_cents) FILTER (WHERE amount_cents < 0), 0)::bigint
		   FROM transactions
		  WHERE user_id = $1 AND occurred_at >= $2 AND occurred_at < $3 AND category_id <> 'transfer'
		  GROUP BY 1, 2 ORDER BY 2`, userID, from, to)
	if err != nil {
		return nil, postgres.MapErr(err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.DailyFlow, error) {
		var d domain.DailyFlow
		err := row.Scan(&d.Currency, &d.Date, &d.IncomeCents, &d.ExpenseCents)
		return d, err
	})
}

func (r *PGRepo) TopMerchants(ctx context.Context, userID string, from, to time.Time) ([]domain.MerchantTotal, error) {
	rows, err := r.Q(ctx).Query(ctx,
		`SELECT currency, merchant, (-SUM(amount_cents))::bigint, COUNT(*)
		   FROM transactions
		  WHERE user_id = $1 AND occurred_at >= $2 AND occurred_at < $3 AND amount_cents < 0 AND merchant <> ''
		    AND category_id <> 'transfer'
		  GROUP BY currency, merchant`, userID, from, to)
	if err != nil {
		return nil, postgres.MapErr(err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.MerchantTotal, error) {
		var m domain.MerchantTotal
		err := row.Scan(&m.Currency, &m.Merchant, &m.SpentCents, &m.Count)
		return m, err
	})
}

func (r *PGRepo) MonthlyTotals(ctx context.Context, userID string, from, to time.Time) ([]domain.MonthTotal, error) {
	rows, err := r.Q(ctx).Query(ctx,
		`SELECT currency, date_trunc('month', occurred_at),
		        COALESCE(SUM(amount_cents) FILTER (WHERE amount_cents > 0), 0)::bigint,
		        COALESCE(-SUM(amount_cents) FILTER (WHERE amount_cents < 0), 0)::bigint
		   FROM transactions
		  WHERE user_id = $1 AND occurred_at >= $2 AND occurred_at < $3 AND category_id <> 'transfer'
		  GROUP BY 1, 2 ORDER BY 2`, userID, from, to)
	if err != nil {
		return nil, postgres.MapErr(err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.MonthTotal, error) {
		var m domain.MonthTotal
		err := row.Scan(&m.Currency, &m.Month, &m.IncomeCents, &m.ExpenseCents)
		return m, err
	})
}

func (r *PGRepo) DeleteBudget(ctx context.Context, userID, categoryID string) error {
	_, err := r.Q(ctx).Exec(ctx, `DELETE FROM budgets WHERE user_id = $1 AND category_id = $2`, userID, categoryID)
	return postgres.MapErr(err)
}

func (r *PGRepo) UpdateTransaction(ctx context.Context, t *domain.Transaction) error {
	return postgres.Affected(r.Q(ctx).Exec(ctx,
		`UPDATE transactions
		    SET amount_cents = $3, currency = $4, description = $5, merchant = $6, category_id = $7, occurred_at = $8
		  WHERE id = $1 AND user_id = $2`,
		t.ID, t.UserID, t.AmountCents, t.Currency, t.Description, t.Merchant, t.CategoryID, t.OccurredAt))
}

func (r *PGRepo) ListImports(ctx context.Context, userID string, limit int) ([]domain.StatementImport, error) {
	rows, err := r.Q(ctx).Query(ctx,
		`SELECT id, user_id, status, format, bank, currency, imported, duplicates, error, created_at
		   FROM statement_imports WHERE user_id = $1
		  ORDER BY created_at DESC, id DESC LIMIT $2`, userID, limit)
	if err != nil {
		return nil, postgres.MapErr(err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.StatementImport, error) {
		var i domain.StatementImport
		err := row.Scan(&i.ID, &i.UserID, &i.Status, &i.Format, &i.Bank, &i.Currency, &i.Imported, &i.Duplicates, &i.Error, &i.CreatedAt)
		return i, err
	})
}

func (r *PGRepo) DeleteGoal(ctx context.Context, userID, id string) error {
	return postgres.Affected(r.Q(ctx).Exec(ctx, `DELETE FROM savings_goals WHERE id = $1 AND user_id = $2`, id, userID))
}

func (r *PGRepo) Expenses(ctx context.Context, userID string, from time.Time) ([]domain.Transaction, error) {
	rows, err := r.Q(ctx).Query(ctx,
		`SELECT `+txCols+` FROM transactions
		  WHERE user_id = $1 AND occurred_at >= $2 AND amount_cents < 0 AND category_id <> 'transfer'
		  ORDER BY occurred_at`, userID, from)
	if err != nil {
		return nil, postgres.MapErr(err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.Transaction, error) { return scanTx(row) })
}

func (r *PGRepo) UsersWithExpenses(ctx context.Context, from time.Time, afterID string, limit int) ([]string, error) {
	rows, err := r.Q(ctx).Query(ctx,
		`SELECT u.user_id::text FROM (
		     SELECT DISTINCT user_id FROM transactions
		      WHERE occurred_at >= $1 AND amount_cents < 0 AND ($2::uuid IS NULL OR user_id > $2::uuid)) u
		  ORDER BY u.user_id LIMIT $3`, from, postgres.Nullable(afterID), limit)
	if err != nil {
		return nil, postgres.MapErr(err)
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

// ---------- investments ----------

const invCols = `id, user_id, kind, name, symbol, currency, quantity, invested_cents, rate_percent,
	capitalization, monthly_contribution_cents, opened_at, term_months`

func (r *PGRepo) UpsertInvestment(ctx context.Context, i *domain.Investment) error {
	if i.ID == "" {
		return postgres.MapErr(r.Q(ctx).QueryRow(ctx,
			`INSERT INTO investments (user_id, kind, name, symbol, currency, quantity, invested_cents, rate_percent,
			                          capitalization, monthly_contribution_cents, opened_at, term_months)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12) RETURNING id`,
			i.UserID, i.Kind, i.Name, i.Symbol, i.Currency, i.Quantity, i.InvestedCents, i.RatePercent,
			i.Capitalization, i.MonthlyContributionCents, i.OpenedAt, i.TermMonths,
		).Scan(&i.ID))
	}
	return postgres.Affected(r.Q(ctx).Exec(ctx,
		`UPDATE investments SET kind = $3, name = $4, symbol = $5, currency = $6, quantity = $7, invested_cents = $8,
		        rate_percent = $9, capitalization = $10, monthly_contribution_cents = $11, opened_at = $12, term_months = $13
		  WHERE id = $1 AND user_id = $2`,
		i.ID, i.UserID, i.Kind, i.Name, i.Symbol, i.Currency, i.Quantity, i.InvestedCents, i.RatePercent,
		i.Capitalization, i.MonthlyContributionCents, i.OpenedAt, i.TermMonths))
}

func (r *PGRepo) ListInvestments(ctx context.Context, userID string) ([]domain.Investment, error) {
	rows, err := r.Q(ctx).Query(ctx,
		`SELECT `+invCols+` FROM investments WHERE user_id = $1 ORDER BY created_at`, userID)
	if err != nil {
		return nil, postgres.MapErr(err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.Investment, error) {
		var i domain.Investment
		err := row.Scan(&i.ID, &i.UserID, &i.Kind, &i.Name, &i.Symbol, &i.Currency, &i.Quantity, &i.InvestedCents,
			&i.RatePercent, &i.Capitalization, &i.MonthlyContributionCents, &i.OpenedAt, &i.TermMonths)
		return i, err
	})
}

func (r *PGRepo) DeleteInvestment(ctx context.Context, userID, id string) error {
	return postgres.Affected(r.Q(ctx).Exec(ctx, `DELETE FROM investments WHERE id = $1 AND user_id = $2`, id, userID))
}

// ---------- net worth ----------

func (r *PGRepo) UpsertAccount(ctx context.Context, a *domain.Account) error {
	if a.ID == "" {
		return postgres.MapErr(r.Q(ctx).QueryRow(ctx,
			`INSERT INTO accounts (user_id, name, kind, currency, balance_cents)
			 VALUES ($1, $2, $3, $4, $5) RETURNING id, updated_at`,
			a.UserID, a.Name, a.Kind, a.Currency, a.BalanceCents,
		).Scan(&a.ID, &a.UpdatedAt))
	}
	err := r.Q(ctx).QueryRow(ctx,
		`UPDATE accounts SET name = $3, kind = $4, currency = $5, balance_cents = $6, updated_at = now()
		  WHERE id = $1 AND user_id = $2 RETURNING updated_at`,
		a.ID, a.UserID, a.Name, a.Kind, a.Currency, a.BalanceCents).Scan(&a.UpdatedAt)
	return postgres.MapErr(err)
}

func (r *PGRepo) ListAccounts(ctx context.Context, userID string) ([]domain.Account, error) {
	rows, err := r.Q(ctx).Query(ctx,
		`SELECT id, user_id, name, kind, currency, balance_cents, updated_at
		   FROM accounts WHERE user_id = $1 ORDER BY created_at`, userID)
	if err != nil {
		return nil, postgres.MapErr(err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.Account, error) {
		var a domain.Account
		err := row.Scan(&a.ID, &a.UserID, &a.Name, &a.Kind, &a.Currency, &a.BalanceCents, &a.UpdatedAt)
		return a, err
	})
}

func (r *PGRepo) DeleteAccount(ctx context.Context, userID, id string) error {
	return postgres.Affected(r.Q(ctx).Exec(ctx, `DELETE FROM accounts WHERE id = $1 AND user_id = $2`, id, userID))
}

func (r *PGRepo) UpsertNetWorthSnapshot(ctx context.Context, userID string, s domain.NetWorthSnapshot) error {
	_, err := r.Q(ctx).Exec(ctx,
		`INSERT INTO net_worth_snapshots (user_id, day, currency, assets_cents, liabilities_cents)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (user_id, day) DO UPDATE
		   SET currency = EXCLUDED.currency, assets_cents = EXCLUDED.assets_cents,
		       liabilities_cents = EXCLUDED.liabilities_cents`,
		userID, s.Day, s.Currency, s.AssetsCents, s.LiabilitiesCents)
	return postgres.MapErr(err)
}

func (r *PGRepo) NetWorthHistory(ctx context.Context, userID string, from time.Time) ([]domain.NetWorthSnapshot, error) {
	rows, err := r.Q(ctx).Query(ctx,
		`SELECT day, currency, assets_cents, liabilities_cents
		   FROM net_worth_snapshots WHERE user_id = $1 AND day >= $2 ORDER BY day`, userID, from)
	if err != nil {
		return nil, postgres.MapErr(err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.NetWorthSnapshot, error) {
		var s domain.NetWorthSnapshot
		err := row.Scan(&s.Day, &s.Currency, &s.AssetsCents, &s.LiabilitiesCents)
		return s, err
	})
}

func (r *PGRepo) UsersWithHoldings(ctx context.Context, afterID string, limit int) ([]string, error) {
	rows, err := r.Q(ctx).Query(ctx,
		`SELECT u.user_id::text FROM (
		     SELECT user_id FROM accounts UNION SELECT user_id FROM investments) u
		  WHERE $1::uuid IS NULL OR u.user_id > $1::uuid
		  ORDER BY u.user_id LIMIT $2`, postgres.Nullable(afterID), limit)
	if err != nil {
		return nil, postgres.MapErr(err)
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}
