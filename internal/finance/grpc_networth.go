package finance

import (
	"context"
	"time"

	"github.com/Zifeldev/Billsplit/internal/domain"
	"github.com/Zifeldev/Billsplit/internal/platform/grpcx"
	pb "github.com/Zifeldev/Billsplit/pkg/pb/billsplit/v1"
)

func accountPB(a AccountView) *pb.Account {
	return &pb.Account{Id: a.ID, Name: a.Name, Kind: pb.AccountKind(a.Kind), Currency: a.Currency,
		BalanceCents: a.BalanceCents, BalanceHomeCents: a.BalanceHomeCents, Liability: a.Kind.Liability(),
		Liquid: a.Kind.Liquid(), UpdatedAt: grpcx.TS(a.UpdatedAt)}
}

func (h *Handler) ListAccounts(ctx context.Context, _ *pb.ListAccountsRequest) (*pb.ListAccountsResponse, error) {
	as, err := h.svc.ListAccounts(ctx)
	if err != nil {
		return nil, grpcx.ToStatus(ctx, err)
	}
	out := &pb.ListAccountsResponse{Accounts: []*pb.Account{}}
	for _, a := range as {
		out.Accounts = append(out.Accounts, accountPB(a))
	}
	return out, nil
}

func (h *Handler) SetAccount(ctx context.Context, req *pb.SetAccountRequest) (*pb.Account, error) {
	a, err := h.svc.SetAccount(ctx, &domain.Account{ID: req.GetId(), Name: req.GetName(),
		Kind: domain.AccountKind(req.GetKind()), Currency: req.GetCurrency(), BalanceCents: req.GetBalanceCents()})
	if err != nil {
		return nil, grpcx.ToStatus(ctx, err)
	}
	return accountPB(*a), nil
}

func (h *Handler) DeleteAccount(ctx context.Context, req *pb.DeleteAccountRequest) (*pb.DeleteAccountResponse, error) {
	if err := h.svc.DeleteAccount(ctx, req.GetId()); err != nil {
		return nil, grpcx.ToStatus(ctx, err)
	}
	return &pb.DeleteAccountResponse{}, nil
}

func (h *Handler) GetNetWorth(ctx context.Context, _ *pb.GetNetWorthRequest) (*pb.NetWorth, error) {
	nw, err := h.svc.NetWorth(ctx)
	if err != nil {
		return nil, grpcx.ToStatus(ctx, err)
	}
	out := &pb.NetWorth{Currency: nw.Currency, NetCents: nw.NetCents, AssetsCents: nw.AssetsCents,
		LiabilitiesCents: nw.LiabilitiesCents, CashCents: nw.CashCents, PropertyCents: nw.PropertyCents,
		InvestmentsCents: nw.InvestmentsCents, OwedToMeCents: nw.OwedToMeCents, CreditCents: nw.CreditCents,
		IOweCents: nw.IOweCents, Change_30DCents: nw.Change30dCents, HasChange_30D: nw.HasChange30d,
		Accounts: []*pb.Account{}, History: []*pb.NetWorthPoint{}}
	for _, a := range nw.Accounts {
		out.Accounts = append(out.Accounts, accountPB(a))
	}
	for _, p := range nw.History {
		out.History = append(out.History, &pb.NetWorthPoint{Date: p.Day.Format(time.DateOnly),
			NetCents: p.AssetsCents - p.LiabilitiesCents, AssetsCents: p.AssetsCents, LiabilitiesCents: p.LiabilitiesCents})
	}
	return out, nil
}

func (h *Handler) GetMonthForecast(ctx context.Context, _ *pb.GetMonthForecastRequest) (*pb.MonthForecast, error) {
	f, err := h.svc.MonthForecast(ctx)
	if err != nil {
		return nil, grpcx.ToStatus(ctx, err)
	}
	out := &pb.MonthForecast{Currency: f.Currency, Month: f.Month.Format("2006-01"), DaysLeft: int32(f.DaysLeft),
		IncomeSoFarCents: f.IncomeSoFarCents, ExpenseSoFarCents: f.ExpenseSoFarCents,
		ExpectedIncomeCents: f.ExpectedIncomeCents, ExpectedExpenseCents: f.ExpectedExpenseCents,
		ProjectedNetCents: f.ProjectedNetCents, HasBalance: f.HasBalance, BalanceNowCents: f.BalanceNowCents,
		ProjectedBalanceCents: f.ProjectedBalanceCents, HistoryMonths: int32(f.HistoryMonths),
		Days: []*pb.ForecastDay{}, Upcoming: []*pb.UpcomingCharge{}}
	for _, d := range f.Days {
		out.Days = append(out.Days, &pb.ForecastDay{Date: d.Date.Format(time.DateOnly), NetCents: d.NetCents, Actual: d.Actual})
	}
	for _, u := range f.Upcoming {
		out.Upcoming = append(out.Upcoming, &pb.UpcomingCharge{Merchant: u.Merchant, AmountCents: u.AmountCents,
			Date: u.Date.Format(time.DateOnly)})
	}
	return out, nil
}

func (h *Handler) ExportReport(ctx context.Context, req *pb.ExportReportRequest) (*pb.ExportFile, error) {
	f, err := h.svc.Export(ctx, ExportKind(req.GetKind()), ExportFormat(req.GetFormat()), int(req.GetYear()), int(req.GetMonth()))
	if err != nil {
		return nil, grpcx.ToStatus(ctx, err)
	}
	return &pb.ExportFile{Filename: f.Filename, ContentType: f.ContentType, Data: f.Data}, nil
}
