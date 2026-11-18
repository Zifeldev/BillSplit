package finance

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/Zifeldev/Billsplit/internal/clients"
	"github.com/Zifeldev/Billsplit/internal/domain"
	"github.com/Zifeldev/Billsplit/internal/parser/statement"
	"github.com/Zifeldev/Billsplit/internal/platform/auth"
	"github.com/Zifeldev/Billsplit/internal/platform/broker"
	"github.com/Zifeldev/Billsplit/internal/platform/cache"
	"github.com/Zifeldev/Billsplit/internal/platform/fx"
	"github.com/Zifeldev/Billsplit/internal/platform/logger"
)

const (
	maxStatementBytes    = 5 << 20  // CSV/OFX
	maxPDFStatementBytes = 15 << 20 // PDF (the gateway body limit is 16 MB)
)

type TxManager interface {
	WithinTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// FX is satisfied by *fx.Client.
type FX interface {
	Currencies(ctx context.Context) (map[string]string, error)
	Validate(ctx context.Context, code, def string) (string, error)
	Rate(ctx context.Context, from, to string, date time.Time) (float64, error)
}

// Locker is a "set if not exists" primitive (Redis SETNX) used to dedupe
// background notifications across restarts and replicas.
type Locker interface {
	SetNX(ctx context.Context, key string, ttl time.Duration) (bool, error)
}

type Service struct {
	repo   Repository
	tx     TxManager
	users  clients.Users
	split  clients.Split
	pub    broker.Publisher // transactional outbox: Publish joins the tx from ctx
	fx     FX
	cache  cache.Cache
	locker Locker
	ttl    time.Duration
	ocr    PDFTexter // PDF statements; nil — PDF imports fail
	market Market    // quotes for investments; nil — market assets unavailable
}

func NewService(repo Repository, tx TxManager, users clients.Users, split clients.Split, pub broker.Publisher,
	fx FX, c cache.Cache, locker Locker, ttl time.Duration) *Service {
	return &Service{repo: repo, tx: tx, users: users, split: split, pub: pub, fx: fx, cache: c, locker: locker, ttl: ttl}
}

// WithOCR enables PDF statement imports through the OCR service.
func (s *Service) WithOCR(o PDFTexter) *Service {
	s.ocr = o
	return s
}

type Currency struct {
	Code, Name string
	MinorUnits int
}

func (s *Service) Currencies(ctx context.Context) ([]Currency, error) {
	m, err := s.fx.Currencies(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Currency, 0, len(m))
	for code, name := range m {
		out = append(out, Currency{Code: code, Name: name, MinorUnits: fx.MinorUnits(code)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	return out, nil
}

func (s *Service) Categories(ctx context.Context) ([]domain.Category, error) {
	const key = "finance:categories"
	var cats []domain.Category
	if err := s.cache.Get(ctx, key, &cats); err == nil {
		return cats, nil
	}
	cats, err := s.repo.ListCategories(ctx)
	if err != nil {
		return nil, err
	}
	_ = s.cache.Set(ctx, key, cats, time.Hour)
	return cats, nil
}

// Categorize picks a category by user rules, then global merchant rules.
func Categorize(rules []domain.CategoryRule, merchant, description string, amount int64) string {
	text := strings.ToLower(merchant + " " + description)
	for _, r := range rules {
		if strings.Contains(text, r.Pattern) {
			return r.CategoryID
		}
	}
	if amount > 0 {
		return "income"
	}
	return "other"
}

type TxInput struct {
	AmountCents int64
	Currency    string
	Description string
	Merchant    string
	CategoryID  string
	OccurredAt  time.Time
}

func (s *Service) home(ctx context.Context) (string, string, error) {
	uid, err := auth.UserID(ctx)
	if err != nil {
		return "", "", err
	}
	u, err := s.users.GetUser(ctx, uid)
	if err != nil {
		return "", "", err
	}
	return uid, u.Currency, nil
}

func (s *Service) CreateTransaction(ctx context.Context, in TxInput) (*domain.Transaction, error) {
	uid, home, err := s.home(ctx)
	if err != nil {
		return nil, err
	}
	if in.AmountCents == 0 {
		return nil, fmt.Errorf("%w: amount must not be zero", domain.ErrInvalidArgument)
	}
	if in.Currency, err = s.fx.Validate(ctx, in.Currency, home); err != nil {
		return nil, err
	}
	if in.OccurredAt.IsZero() {
		in.OccurredAt = time.Now()
	}
	if in.CategoryID == "" {
		rules, err := s.repo.Rules(ctx, uid)
		if err != nil {
			return nil, err
		}
		in.CategoryID = Categorize(rules, in.Merchant, in.Description, in.AmountCents)
	} else if err := s.checkCategory(ctx, in.CategoryID); err != nil {
		return nil, err
	}
	t := &domain.Transaction{UserID: uid, AmountCents: in.AmountCents, Currency: in.Currency,
		Description: strings.TrimSpace(in.Description), Merchant: strings.ToUpper(strings.TrimSpace(in.Merchant)),
		CategoryID: in.CategoryID, Source: domain.SourceManual, OccurredAt: in.OccurredAt}
	if err := s.repo.CreateTransaction(ctx, t); err != nil {
		return nil, err
	}
	s.bump(ctx, uid)
	return t, nil
}

func (s *Service) ListTransactions(ctx context.Context, f domain.TransactionFilter) ([]domain.Transaction, error) {
	uid, err := auth.UserID(ctx)
	if err != nil {
		return nil, err
	}
	if f.To.IsZero() {
		f.To = time.Now().AddDate(0, 0, 1)
	}
	if f.From.IsZero() {
		f.From = f.To.AddDate(0, -1, 0)
	}
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 50
	}
	f.Offset = max(f.Offset, 0)
	return s.repo.ListTransactions(ctx, uid, f)
}

func (s *Service) UpdateCategory(ctx context.Context, id, categoryID string, rememberRule bool) (*domain.Transaction, error) {
	uid, err := auth.UserID(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.checkCategory(ctx, categoryID); err != nil {
		return nil, err
	}
	t, err := s.repo.GetTransaction(ctx, uid, id)
	if err != nil {
		return nil, err
	}
	if err := s.repo.UpdateTransactionCategory(ctx, uid, id, categoryID); err != nil {
		return nil, err
	}
	if rememberRule && t.Merchant != "" {
		// Learn from the user: future imports of this merchant go to the same category.
		rule := domain.CategoryRule{UserID: uid, Pattern: strings.ToLower(t.Merchant), CategoryID: categoryID}
		if err := s.repo.UpsertRule(ctx, rule); err != nil {
			return nil, err
		}
	}
	t.CategoryID = categoryID
	s.bump(ctx, uid)
	return t, nil
}

// TxPatch holds the fields of UpdateTransaction; nil (zero time) means "keep".
type TxPatch struct {
	AmountCents *int64
	Currency    *string
	Description *string
	Merchant    *string
	CategoryID  *string
	OccurredAt  time.Time
}

func (s *Service) UpdateTransaction(ctx context.Context, id string, p TxPatch) (*domain.Transaction, error) {
	uid, home, err := s.home(ctx)
	if err != nil {
		return nil, err
	}
	t, err := s.repo.GetTransaction(ctx, uid, id)
	if err != nil {
		return nil, err
	}
	if p.AmountCents != nil {
		if *p.AmountCents == 0 {
			return nil, fmt.Errorf("%w: amount must not be zero", domain.ErrInvalidArgument)
		}
		t.AmountCents = *p.AmountCents
	}
	if p.Currency != nil {
		if t.Currency, err = s.fx.Validate(ctx, *p.Currency, home); err != nil {
			return nil, err
		}
	}
	if p.Description != nil {
		t.Description = strings.TrimSpace(*p.Description)
	}
	if p.Merchant != nil {
		t.Merchant = strings.ToUpper(strings.TrimSpace(*p.Merchant))
	}
	if p.CategoryID != nil {
		if err := s.checkCategory(ctx, *p.CategoryID); err != nil {
			return nil, err
		}
		t.CategoryID = *p.CategoryID
	}
	if !p.OccurredAt.IsZero() {
		t.OccurredAt = p.OccurredAt
	}
	if err := s.repo.UpdateTransaction(ctx, t); err != nil {
		return nil, err
	}
	s.bump(ctx, uid)
	return t, nil
}

func (s *Service) DeleteTransaction(ctx context.Context, id string) error {
	uid, err := auth.UserID(ctx)
	if err != nil {
		return err
	}
	if err := s.repo.DeleteTransaction(ctx, uid, id); err != nil {
		return err
	}
	s.bump(ctx, uid)
	return nil
}

func (s *Service) ImportStatement(ctx context.Context, file []byte, format int16, bank, currency string) (*domain.StatementImport, error) {
	uid, home, err := s.home(ctx)
	if err != nil {
		return nil, err
	}
	isPDF := statement.IsPDF(file)
	switch statement.Format(format) {
	case statement.FormatAuto:
		if isPDF {
			format = int16(statement.FormatPDF)
		}
	case statement.FormatCSV, statement.FormatOFX:
	case statement.FormatPDF:
		if !isPDF {
			return nil, fmt.Errorf("%w: file is not a PDF", domain.ErrInvalidArgument)
		}
	default:
		return nil, fmt.Errorf("%w: unknown statement format %d", domain.ErrInvalidArgument, format)
	}
	limit, limitText := maxStatementBytes, "5 MB"
	if statement.Format(format) == statement.FormatPDF {
		limit, limitText = maxPDFStatementBytes, "15 MB"
	}
	if len(file) == 0 || len(file) > limit {
		return nil, fmt.Errorf("%w: file must be 1 byte..%s", domain.ErrInvalidArgument, limitText)
	}
	if currency, err = s.fx.Validate(ctx, currency, home); err != nil {
		return nil, err
	}
	imp := &domain.StatementImport{UserID: uid, Format: format, Bank: bank, Currency: currency, File: file}
	// Import row and event commit together (outbox), so an import is never stuck without its event.
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.repo.CreateImport(ctx, imp); err != nil {
			return err
		}
		ev := domain.StatementUploadedEvent{ImportID: imp.ID, UserID: uid}
		return s.pub.Publish(ctx, domain.TopicStatementUploaded, uid, ev)
	})
	if err != nil {
		return nil, err
	}
	imp.File = nil
	return imp, nil
}

const (
	defaultImportsLimit = 20
	maxImportsLimit     = 100
)

func (s *Service) ListImports(ctx context.Context, limit int) ([]domain.StatementImport, error) {
	uid, err := auth.UserID(ctx)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = defaultImportsLimit
	}
	limit = min(limit, maxImportsLimit)
	imps, err := s.repo.ListImports(ctx, uid, limit)
	if err != nil {
		return nil, err
	}
	for i := range imps {
		imps[i].File = nil
	}
	return imps, nil
}

func (s *Service) GetImport(ctx context.Context, id string) (*domain.StatementImport, error) {
	uid, err := auth.UserID(ctx)
	if err != nil {
		return nil, err
	}
	imp, err := s.repo.GetImport(ctx, id)
	if err != nil {
		return nil, err
	}
	if imp.UserID != uid {
		return nil, domain.ErrNotFound
	}
	imp.File = nil
	return imp, nil
}

func (s *Service) SetBudget(ctx context.Context, categoryID string, limit int64) (*domain.Budget, error) {
	uid, err := auth.UserID(ctx)
	if err != nil {
		return nil, err
	}
	if limit < 0 {
		return nil, fmt.Errorf("%w: limit must be >= 0", domain.ErrInvalidArgument)
	}
	if err := s.checkCategory(ctx, categoryID); err != nil {
		return nil, err
	}
	b := &domain.Budget{CategoryID: categoryID, LimitCents: limit}
	if limit == 0 { // 0 removes the budget
		if err := s.repo.DeleteBudget(ctx, uid, categoryID); err != nil {
			return nil, err
		}
		s.bump(ctx, uid)
		return b, nil
	}
	if err := s.repo.UpsertBudget(ctx, uid, b); err != nil {
		return nil, err
	}
	s.bump(ctx, uid)
	return b, nil
}

func (s *Service) DeleteBudget(ctx context.Context, categoryID string) error {
	uid, err := auth.UserID(ctx)
	if err != nil {
		return err
	}
	if categoryID == "" {
		return fmt.Errorf("%w: category_id is required", domain.ErrInvalidArgument)
	}
	if err := s.repo.DeleteBudget(ctx, uid, categoryID); err != nil {
		return err
	}
	s.bump(ctx, uid)
	return nil
}

func (s *Service) ListBudgets(ctx context.Context) ([]domain.Budget, error) {
	uid, err := auth.UserID(ctx)
	if err != nil {
		return nil, err
	}
	return s.repo.ListBudgets(ctx, uid)
}

func (s *Service) SetGoal(ctx context.Context, g *domain.SavingsGoal) (*GoalView, error) {
	uid, err := auth.UserID(ctx)
	if err != nil {
		return nil, err
	}
	g.UserID = uid
	g.Name = strings.TrimSpace(g.Name)
	if g.Name == "" || len(g.Name) > 100 || g.TargetCents <= 0 || g.SavedCents < 0 {
		return nil, fmt.Errorf("%w: name and positive target are required", domain.ErrInvalidArgument)
	}
	if g.InvestmentID != "" {
		invs, err := s.repo.ListInvestments(ctx, uid)
		if err != nil {
			return nil, err
		}
		if !slices.ContainsFunc(invs, func(i domain.Investment) bool { return i.ID == g.InvestmentID }) {
			return nil, fmt.Errorf("%w: investment not found", domain.ErrInvalidArgument)
		}
	}
	if err := s.repo.UpsertGoal(ctx, g); err != nil {
		return nil, err
	}
	vs, err := s.goalsView(ctx, []domain.SavingsGoal{*g})
	if err != nil {
		return nil, err
	}
	return &vs[0], nil
}

func (s *Service) ListGoals(ctx context.Context) ([]GoalView, error) {
	uid, err := auth.UserID(ctx)
	if err != nil {
		return nil, err
	}
	gs, err := s.repo.ListGoals(ctx, uid)
	if err != nil {
		return nil, err
	}
	return s.goalsView(ctx, gs)
}

// DeleteGoal removes a savings goal; goals of other users look like "not found".
func (s *Service) DeleteGoal(ctx context.Context, id string) error {
	uid, err := auth.UserID(ctx)
	if err != nil {
		return err
	}
	if id == "" {
		return fmt.Errorf("%w: id is required", domain.ErrInvalidArgument)
	}
	return s.repo.DeleteGoal(ctx, uid, id)
}

// MonthlyNeeded is how much to save per month to hit the goal by its deadline.
func MonthlyNeeded(g domain.SavingsGoal, now time.Time) int64 {
	left := g.TargetCents - g.SavedCents
	if left <= 0 || g.Deadline == nil {
		return 0
	}
	months := int64((g.Deadline.Year()-now.Year())*12 + int(g.Deadline.Month()-now.Month()))
	if months < 1 {
		return left
	}
	return (left + months - 1) / months
}

func (s *Service) checkCategory(ctx context.Context, id string) error {
	ok, err := s.repo.CategoryExists(ctx, id)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%w: unknown category %q", domain.ErrInvalidArgument, id)
	}
	return nil
}

// Dashboard cache uses a per-user version: any write bumps it, so stale entries are never read.
func dashVersionKey(uid string) string { return "finance:dash:ver:" + uid }

func (s *Service) bump(ctx context.Context, uid string) {
	if _, err := s.cache.Incr(ctx, dashVersionKey(uid)); err != nil {
		logger.FromContext(ctx).Warn("dashboard cache bump failed", "err", err)
	}
}
