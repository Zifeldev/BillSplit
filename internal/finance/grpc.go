package finance

import (
	"context"
	"time"

	"github.com/Zifeldev/Billsplit/internal/domain"
	"github.com/Zifeldev/Billsplit/internal/platform/grpcx"
	pb "github.com/Zifeldev/Billsplit/pkg/pb/billsplit/v1"
)

type Handler struct {
	pb.UnimplementedFinanceServiceServer
	svc *Service
}

func NewHandler(s *Service) *Handler { return &Handler{svc: s} }

// ListCurrencies is public: it never reads the caller identity.
func (h *Handler) ListCurrencies(ctx context.Context, _ *pb.ListCurrenciesRequest) (*pb.ListCurrenciesResponse, error) {
	cs, err := h.svc.Currencies(ctx)
	if err != nil {
		return nil, grpcx.ToStatus(ctx, err)
	}
	out := &pb.ListCurrenciesResponse{}
	for _, c := range cs {
		out.Currencies = append(out.Currencies, &pb.Currency{Code: c.Code, Name: c.Name, MinorUnits: int32(c.MinorUnits)})
	}
	return out, nil
}

func (h *Handler) ListCategories(ctx context.Context, _ *pb.ListCategoriesRequest) (*pb.ListCategoriesResponse, error) {
	cats, err := h.svc.Categories(ctx)
	if err != nil {
		return nil, grpcx.ToStatus(ctx, err)
	}
	out := &pb.ListCategoriesResponse{}
	for _, c := range cats {
		out.Categories = append(out.Categories, &pb.Category{Id: c.ID, Name: c.Name, Kind: pb.CategoryKind(c.Kind), Icon: c.Icon})
	}
	return out, nil
}

func (h *Handler) CreateTransaction(ctx context.Context, req *pb.CreateTransactionRequest) (*pb.Transaction, error) {
	t, err := h.svc.CreateTransaction(ctx, TxInput{
		AmountCents: req.GetAmountCents(), Currency: req.GetCurrency(), Description: req.GetDescription(), Merchant: req.GetMerchant(),
		CategoryID: req.GetCategoryId(), OccurredAt: grpcx.FromTS(req.GetOccurredAt()),
	})
	if err != nil {
		return nil, grpcx.ToStatus(ctx, err)
	}
	return txPB(t), nil
}

func (h *Handler) ListTransactions(ctx context.Context, req *pb.ListTransactionsRequest) (*pb.ListTransactionsResponse, error) {
	txs, err := h.svc.ListTransactions(ctx, domain.TransactionFilter{
		From: grpcx.FromTS(req.GetFrom()), To: grpcx.FromTS(req.GetTo()), CategoryID: req.GetCategoryId(),
		Limit: int(req.GetLimit()), Offset: int(req.GetOffset()),
	})
	if err != nil {
		return nil, grpcx.ToStatus(ctx, err)
	}
	out := &pb.ListTransactionsResponse{Transactions: make([]*pb.Transaction, len(txs))}
	for i := range txs {
		out.Transactions[i] = txPB(&txs[i])
	}
	return out, nil
}

func (h *Handler) UpdateTransactionCategory(ctx context.Context, req *pb.UpdateTransactionCategoryRequest) (*pb.Transaction, error) {
	t, err := h.svc.UpdateCategory(ctx, req.GetTransactionId(), req.GetCategoryId(), req.GetRememberRule())
	if err != nil {
		return nil, grpcx.ToStatus(ctx, err)
	}
	return txPB(t), nil
}

func (h *Handler) UpdateTransaction(ctx context.Context, req *pb.UpdateTransactionRequest) (*pb.Transaction, error) {
	p := TxPatch{AmountCents: req.AmountCents, Currency: req.Currency, Description: req.Description,
		Merchant: req.Merchant, CategoryID: req.CategoryId}
	if req.GetOccurredAt() != nil {
		p.OccurredAt = req.GetOccurredAt().AsTime()
	}
	t, err := h.svc.UpdateTransaction(ctx, req.GetTransactionId(), p)
	if err != nil {
		return nil, grpcx.ToStatus(ctx, err)
	}
	return txPB(t), nil
}

func (h *Handler) DeleteTransaction(ctx context.Context, req *pb.DeleteTransactionRequest) (*pb.DeleteTransactionResponse, error) {
	if err := h.svc.DeleteTransaction(ctx, req.GetTransactionId()); err != nil {
		return nil, grpcx.ToStatus(ctx, err)
	}
	return &pb.DeleteTransactionResponse{}, nil
}

func (h *Handler) ImportStatement(ctx context.Context, req *pb.ImportStatementRequest) (*pb.StatementImport, error) {
	imp, err := h.svc.ImportStatement(ctx, req.GetFile(), int16(req.GetFormat()), req.GetBank(), req.GetCurrency())
	if err != nil {
		return nil, grpcx.ToStatus(ctx, err)
	}
	return importPB(imp), nil
}

func (h *Handler) GetStatementImport(ctx context.Context, req *pb.GetStatementImportRequest) (*pb.StatementImport, error) {
	imp, err := h.svc.GetImport(ctx, req.GetImportId())
	if err != nil {
		return nil, grpcx.ToStatus(ctx, err)
	}
	return importPB(imp), nil
}

func (h *Handler) ListStatementImports(ctx context.Context, req *pb.ListStatementImportsRequest) (*pb.ListStatementImportsResponse, error) {
	imps, err := h.svc.ListImports(ctx, int(req.GetLimit()))
	if err != nil {
		return nil, grpcx.ToStatus(ctx, err)
	}
	out := &pb.ListStatementImportsResponse{Imports: make([]*pb.StatementImport, len(imps))}
	for i := range imps {
		out.Imports[i] = importPB(&imps[i])
	}
	return out, nil
}

func (h *Handler) DeleteBudget(ctx context.Context, req *pb.DeleteBudgetRequest) (*pb.DeleteBudgetResponse, error) {
	if err := h.svc.DeleteBudget(ctx, req.GetCategoryId()); err != nil {
		return nil, grpcx.ToStatus(ctx, err)
	}
	return &pb.DeleteBudgetResponse{}, nil
}

func (h *Handler) DeleteSavingsGoal(ctx context.Context, req *pb.DeleteSavingsGoalRequest) (*pb.DeleteSavingsGoalResponse, error) {
	if err := h.svc.DeleteGoal(ctx, req.GetId()); err != nil {
		return nil, grpcx.ToStatus(ctx, err)
	}
	return &pb.DeleteSavingsGoalResponse{}, nil
}

func (h *Handler) ListRecurring(ctx context.Context, _ *pb.ListRecurringRequest) (*pb.ListRecurringResponse, error) {
	r, err := h.svc.ListRecurring(ctx)
	if err != nil {
		return nil, grpcx.ToStatus(ctx, err)
	}
	out := &pb.ListRecurringResponse{Currency: r.Currency, MonthlyTotalCents: r.MonthlyTotalCents}
	for _, it := range r.Items {
		out.Items = append(out.Items, &pb.RecurringExpense{
			Merchant: it.Merchant, CategoryId: it.CategoryID, AverageCents: it.AverageCents, Currency: r.Currency,
			Period: recurringPeriodPB(it.Period), Occurrences: int32(it.Occurrences),
			LastChargedAt: grpcx.TS(it.LastChargedAt), NextExpectedAt: grpcx.TS(it.NextExpectedAt),
			YearlyCostCents: it.YearlyCostCents, IsSubscription: it.IsSubscription,
		})
	}
	return out, nil
}

func recurringPeriodPB(p RecurringPeriod) pb.RecurringPeriod {
	switch p {
	case PeriodWeekly:
		return pb.RecurringPeriod_RECURRING_PERIOD_WEEKLY
	case PeriodMonthly:
		return pb.RecurringPeriod_RECURRING_PERIOD_MONTHLY
	case PeriodYearly:
		return pb.RecurringPeriod_RECURRING_PERIOD_YEARLY
	}
	return pb.RecurringPeriod_RECURRING_PERIOD_UNSPECIFIED
}

func (h *Handler) SetBudget(ctx context.Context, req *pb.SetBudgetRequest) (*pb.Budget, error) {
	b, err := h.svc.SetBudget(ctx, req.GetCategoryId(), req.GetLimitCents())
	if err != nil {
		return nil, grpcx.ToStatus(ctx, err)
	}
	return &pb.Budget{Id: b.ID, CategoryId: b.CategoryID, LimitCents: b.LimitCents}, nil
}

func (h *Handler) ListBudgets(ctx context.Context, _ *pb.ListBudgetsRequest) (*pb.ListBudgetsResponse, error) {
	bs, err := h.svc.ListBudgets(ctx)
	if err != nil {
		return nil, grpcx.ToStatus(ctx, err)
	}
	out := &pb.ListBudgetsResponse{}
	for _, b := range bs {
		out.Budgets = append(out.Budgets, &pb.Budget{Id: b.ID, CategoryId: b.CategoryID, LimitCents: b.LimitCents})
	}
	return out, nil
}

func (h *Handler) SetSavingsGoal(ctx context.Context, req *pb.SetSavingsGoalRequest) (*pb.SavingsGoal, error) {
	g := &domain.SavingsGoal{ID: req.GetId(), Name: req.GetName(), TargetCents: req.GetTargetCents(),
		SavedCents: req.GetSavedCents(), InvestmentID: req.GetInvestmentId()}
	if req.GetDeadline() != nil {
		d := req.GetDeadline().AsTime()
		g.Deadline = &d
	}
	v, err := h.svc.SetGoal(ctx, g)
	if err != nil {
		return nil, grpcx.ToStatus(ctx, err)
	}
	return goalPB(*v), nil
}

func (h *Handler) ListSavingsGoals(ctx context.Context, _ *pb.ListSavingsGoalsRequest) (*pb.ListSavingsGoalsResponse, error) {
	gs, err := h.svc.ListGoals(ctx)
	if err != nil {
		return nil, grpcx.ToStatus(ctx, err)
	}
	out := &pb.ListSavingsGoalsResponse{}
	for _, g := range gs {
		out.Goals = append(out.Goals, goalPB(g))
	}
	return out, nil
}

func (h *Handler) GetDashboard(ctx context.Context, req *pb.GetDashboardRequest) (*pb.Dashboard, error) {
	d, err := h.svc.Dashboard(ctx, int(req.GetYear()), int(req.GetMonth()))
	if err != nil {
		return nil, grpcx.ToStatus(ctx, err)
	}
	out := &pb.Dashboard{
		Currency: d.Currency, IncomeCents: d.IncomeCents, ExpenseCents: d.ExpenseCents, NetCents: d.NetCents,
		SavingsRate: d.SavingsRate, ProjectedMonthExpenseCents: d.ProjectedExpenseCents, Insights: d.Insights,
		OwedToMeCents: d.OwedToMeCents, IOweCents: d.IOweCents,
		RuleBreakdown: &pb.RuleBreakdown{Needs: d.Needs, Wants: d.Wants, Savings: d.Savings},
	}
	for _, c := range d.ByCategory {
		out.ByCategory = append(out.ByCategory, &pb.CategorySpend{CategoryId: c.CategoryID, Name: c.Name,
			Kind: pb.CategoryKind(c.Kind), SpentCents: c.SpentCents, BudgetCents: c.BudgetCents, BudgetUsed: c.BudgetUsed})
	}
	for _, x := range d.Daily {
		out.Daily = append(out.Daily, &pb.DailySpend{Date: x.Date.Format(time.DateOnly), SpentCents: x.SpentCents})
	}
	for _, m := range d.TopMerchants {
		out.TopMerchants = append(out.TopMerchants, &pb.MerchantSpend{Merchant: m.Merchant, SpentCents: m.SpentCents, Count: int32(m.Count)})
	}
	for _, m := range d.Last6Months {
		out.LastSixMonths = append(out.LastSixMonths, &pb.MonthTotal{Month: m.Month.Format("2006-01"),
			IncomeCents: m.IncomeCents, ExpenseCents: m.ExpenseCents})
	}
	return out, nil
}

func txPB(t *domain.Transaction) *pb.Transaction {
	return &pb.Transaction{Id: t.ID, AmountCents: t.AmountCents, Currency: t.Currency, Description: t.Description,
		Merchant: t.Merchant, CategoryId: t.CategoryID, Source: t.Source, OccurredAt: grpcx.TS(t.OccurredAt)}
}

func importPB(i *domain.StatementImport) *pb.StatementImport {
	return &pb.StatementImport{Id: i.ID, Status: importStatusPB(i.Status), Imported: int32(i.Imported),
		Duplicates: int32(i.Duplicates), Error: i.Error, CreatedAt: grpcx.TS(i.CreatedAt)}
}

func goalPB(g GoalView) *pb.SavingsGoal {
	return &pb.SavingsGoal{Id: g.ID, Name: g.Name, TargetCents: g.TargetCents, SavedCents: g.SavedCents,
		Deadline: grpcx.TSPtr(g.Deadline), MonthlyNeededCents: MonthlyNeeded(g.SavingsGoal, time.Now()),
		InvestmentId: g.InvestmentID, InvestmentName: g.InvestmentName, ProjectedAt: grpcx.TSPtr(g.ProjectedAt)}
}

func importStatusPB(s string) pb.ImportStatus {
	switch s {
	case domain.ImportPending:
		return pb.ImportStatus_IMPORT_STATUS_PENDING
	case domain.ImportDone:
		return pb.ImportStatus_IMPORT_STATUS_DONE
	case domain.ImportFailed:
		return pb.ImportStatus_IMPORT_STATUS_FAILED
	}
	return pb.ImportStatus_IMPORT_STATUS_UNSPECIFIED
}
