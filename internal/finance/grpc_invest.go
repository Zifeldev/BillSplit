package finance

import (
	"context"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/Zifeldev/Billsplit/internal/domain"
	"github.com/Zifeldev/Billsplit/internal/platform/grpcx"
	pb "github.com/Zifeldev/Billsplit/pkg/pb/billsplit/v1"
)

func (h *Handler) ListInvestments(ctx context.Context, _ *pb.ListInvestmentsRequest) (*pb.Portfolio, error) {
	p, err := h.svc.Portfolio(ctx)
	if err != nil {
		return nil, grpcx.ToStatus(ctx, err)
	}
	out := &pb.Portfolio{Currency: p.Currency, ValueCents: p.ValueCents, ContributedCents: p.ContributedCents,
		GainCents: p.GainCents, GainPercent: p.GainPercent, YearlyInterestCents: p.YearlyInterestCents}
	for _, it := range p.Items {
		out.Items = append(out.Items, holdingPB(it))
	}
	for _, a := range p.Allocation {
		out.Allocation = append(out.Allocation, &pb.AllocationSlice{Kind: pb.InvestmentKind(a.Kind), ValueCents: a.ValueCents, Share: a.Share})
	}
	for _, pt := range p.History {
		out.History = append(out.History, &pb.PortfolioPoint{Date: pt.Date, ValueCents: pt.ValueCents, ContributedCents: pt.Contributed})
	}
	return out, nil
}

func (h *Handler) SetInvestment(ctx context.Context, req *pb.SetInvestmentRequest) (*pb.Investment, error) {
	inv := &domain.Investment{
		ID: req.GetId(), Kind: domain.InvestmentKind(req.GetKind()), Name: req.GetName(), Symbol: req.GetSymbol(),
		Currency: req.GetCurrency(), Quantity: req.GetQuantity(), InvestedCents: req.GetInvestedCents(),
		RatePercent: req.GetRatePercent(), Capitalization: domain.Capitalization(req.GetCapitalization()),
		MonthlyContributionCents: req.GetMonthlyContributionCents(), TermMonths: int(req.GetTermMonths()),
	}
	if req.GetOpenedAt() != nil {
		inv.OpenedAt = req.GetOpenedAt().AsTime()
	}
	inv, err := h.svc.SetInvestment(ctx, inv)
	if err != nil {
		return nil, grpcx.ToStatus(ctx, err)
	}
	return investmentPB(*inv), nil
}

func (h *Handler) DeleteInvestment(ctx context.Context, req *pb.DeleteInvestmentRequest) (*pb.DeleteInvestmentResponse, error) {
	if err := h.svc.DeleteInvestment(ctx, req.GetId()); err != nil {
		return nil, grpcx.ToStatus(ctx, err)
	}
	return &pb.DeleteInvestmentResponse{}, nil
}

func (h *Handler) GetMarketChart(ctx context.Context, req *pb.GetMarketChartRequest) (*pb.MarketChart, error) {
	rng := req.GetRange()
	if rng == "" {
		rng = "1y"
	}
	ch, err := h.svc.MarketChart(ctx, req.GetSymbol(), rng)
	if err != nil {
		return nil, grpcx.ToStatus(ctx, err)
	}
	out := &pb.MarketChart{Symbol: ch.Symbol, Name: ch.Name, Currency: ch.Currency, Price: ch.Price,
		DayChangePercent: ch.DayChangePercent(), Range: rng}
	for _, p := range ch.Points {
		out.Points = append(out.Points, &pb.PricePoint{Date: p.Date, Close: p.Close})
	}
	if len(ch.Points) > 0 && ch.Points[0].Close > 0 {
		out.RangeChangePercent = (ch.Price/ch.Points[0].Close - 1) * 100
	}
	return out, nil
}

func (h *Handler) SearchSymbols(ctx context.Context, req *pb.SearchSymbolsRequest) (*pb.SearchSymbolsResponse, error) {
	ms, err := h.svc.SearchSymbols(ctx, req.GetQuery())
	if err != nil {
		return nil, grpcx.ToStatus(ctx, err)
	}
	out := &pb.SearchSymbolsResponse{}
	for _, m := range ms {
		out.Matches = append(out.Matches, &pb.SymbolMatch{Symbol: m.Symbol, Name: m.Name, Exchange: m.Exchange, Type: m.Type})
	}
	return out, nil
}

func (h *Handler) GetMarketOverview(ctx context.Context, _ *pb.GetMarketOverviewRequest) (*pb.MarketOverview, error) {
	qs, err := h.svc.MarketOverview(ctx)
	if err != nil {
		return nil, grpcx.ToStatus(ctx, err)
	}
	out := &pb.MarketOverview{}
	for _, q := range qs {
		out.Quotes = append(out.Quotes, &pb.MarketQuote{Symbol: q.Symbol, Name: q.Name, Currency: q.Currency, Price: q.Price,
			DayChangePercent: q.DayChangePercent, MonthChangePercent: q.MonthChangePercent, Spark: q.Spark, Held: q.Held})
	}
	return out, nil
}

func investmentPB(i domain.Investment) *pb.Investment {
	return &pb.Investment{
		Id: i.ID, Kind: pb.InvestmentKind(i.Kind), Name: i.Name, Symbol: i.Symbol, Currency: i.Currency,
		Quantity: i.Quantity, InvestedCents: i.InvestedCents, RatePercent: i.RatePercent,
		Capitalization: pb.Capitalization(i.Capitalization), MonthlyContributionCents: i.MonthlyContributionCents,
		OpenedAt: timestamppb.New(i.OpenedAt), TermMonths: int32(i.TermMonths),
		ContributedCents: i.InvestedCents, ValueCents: i.InvestedCents,
	}
}

func holdingPB(h Holding) *pb.Investment {
	out := investmentPB(h.Investment)
	out.ValueCents, out.ValueHomeCents = h.ValueCents, h.ValueHomeCents
	out.ContributedCents, out.GainCents, out.GainPercent = h.ContributedCents, h.GainCents, h.GainPercent
	out.Price, out.DayChangePercent = h.Price, h.DayChangePercent
	out.EffectiveRatePercent, out.MaturityValueCents = h.EffectiveRatePercent, h.MaturityValueCents
	if h.MaturityAt != nil {
		out.MaturityAt = timestamppb.New(*h.MaturityAt)
	}
	out.QuoteError = h.QuoteError
	return out
}
