// Package finance is the personal finance microservice: transactions, bank
// statement import, categorization, budgets, goals and dashboards.
package finance

import (
	"context"
	"time"

	"github.com/Zifeldev/Billsplit/internal/domain"
)

type Repository interface {
	ListCategories(ctx context.Context) ([]domain.Category, error)
	CategoryExists(ctx context.Context, id string) (bool, error)
	// Rules returns user rules first, then global ones (longest pattern first).
	Rules(ctx context.Context, userID string) ([]domain.CategoryRule, error)
	UpsertRule(ctx context.Context, r domain.CategoryRule) error

	CreateTransaction(ctx context.Context, t *domain.Transaction) error
	// InsertTransactions skips duplicates by (user_id, dedup_key) and returns inserted count.
	InsertTransactions(ctx context.Context, ts []domain.Transaction) (int, error)
	GetTransaction(ctx context.Context, userID, id string) (*domain.Transaction, error)
	ListTransactions(ctx context.Context, userID string, f domain.TransactionFilter) ([]domain.Transaction, error)
	UpdateTransactionCategory(ctx context.Context, userID, id, categoryID string) error
	// UpdateTransaction overwrites the editable fields of t (matched by t.ID and t.UserID).
	UpdateTransaction(ctx context.Context, t *domain.Transaction) error
	DeleteTransaction(ctx context.Context, userID, id string) error

	CreateImport(ctx context.Context, imp *domain.StatementImport) error
	GetImport(ctx context.Context, id string) (*domain.StatementImport, error)
	FinishImport(ctx context.Context, id, status string, imported, duplicates int, errMsg string) error
	// ListImports returns the user's imports, newest first, without file bytes.
	ListImports(ctx context.Context, userID string, limit int) ([]domain.StatementImport, error)

	UpsertBudget(ctx context.Context, userID string, b *domain.Budget) error
	ListBudgets(ctx context.Context, userID string) ([]domain.Budget, error)
	DeleteBudget(ctx context.Context, userID, categoryID string) error
	UpsertGoal(ctx context.Context, g *domain.SavingsGoal) error
	ListGoals(ctx context.Context, userID string) ([]domain.SavingsGoal, error)
	DeleteGoal(ctx context.Context, userID, id string) error
	UpsertInvestment(ctx context.Context, inv *domain.Investment) error
	ListInvestments(ctx context.Context, userID string) ([]domain.Investment, error)
	DeleteInvestment(ctx context.Context, userID, id string) error
	UpsertAccount(ctx context.Context, a *domain.Account) error
	ListAccounts(ctx context.Context, userID string) ([]domain.Account, error)
	DeleteAccount(ctx context.Context, userID, id string) error
	UpsertNetWorthSnapshot(ctx context.Context, userID string, s domain.NetWorthSnapshot) error
	// NetWorthHistory returns snapshots since `from`, oldest first.
	NetWorthHistory(ctx context.Context, userID string, from time.Time) ([]domain.NetWorthSnapshot, error)
	// UsersWithHoldings lists users having accounts or investments (daily snapshots).
	UsersWithHoldings(ctx context.Context, afterID string, limit int) ([]string, error)

	// Expenses returns the user's expenses (amount < 0, no transfers) since `from`, oldest first.
	Expenses(ctx context.Context, userID string, from time.Time) ([]domain.Transaction, error)
	// UsersWithExpenses lists users having expenses since `from`, ordered by id,
	// with id > afterID (keyset pagination), at most limit.
	UsersWithExpenses(ctx context.Context, from time.Time, afterID string, limit int) ([]string, error)

	// Aggregates for [from, to), grouped by currency.
	IncomeExpense(ctx context.Context, userID string, from, to time.Time) ([]domain.CurrencyTotals, error)
	SpendByCategory(ctx context.Context, userID string, from, to time.Time) ([]domain.CategoryTotal, error)
	SpendByDay(ctx context.Context, userID string, from, to time.Time) ([]domain.DailyTotal, error)
	TopMerchants(ctx context.Context, userID string, from, to time.Time) ([]domain.MerchantTotal, error)
	MonthlyTotals(ctx context.Context, userID string, from, to time.Time) ([]domain.MonthTotal, error)
	DailyFlows(ctx context.Context, userID string, from, to time.Time) ([]domain.DailyFlow, error)
}
