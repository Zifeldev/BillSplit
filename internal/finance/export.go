package finance

import (
	"context"
	"fmt"
	"time"

	"github.com/Zifeldev/Billsplit/internal/domain"
	"github.com/Zifeldev/Billsplit/internal/finance/report"
)

type ExportKind int

const (
	ExportMonth ExportKind = iota + 1
	ExportPortfolio
)

type ExportFormat int

const (
	FormatCSV ExportFormat = iota + 1
	FormatPDF
)

type ExportFile struct {
	Filename, ContentType string
	Data                  []byte
}

// maxExportTx caps one month's export (a month of statements is a few hundred rows).
const maxExportTx = 5000

func (s *Service) Export(ctx context.Context, kind ExportKind, format ExportFormat, year, month int) (*ExportFile, error) {
	if format != FormatCSV && format != FormatPDF {
		return nil, fmt.Errorf("%w: format must be CSV or PDF", domain.ErrInvalidArgument)
	}
	now := time.Now().UTC()
	var (
		data []byte
		name string
		err  error
	)
	switch kind {
	case ExportMonth:
		var m *report.Month
		if m, err = s.monthReport(ctx, year, month, now); err != nil {
			return nil, err
		}
		name = "billsplit-" + m.Month.Format("2006-01")
		if format == FormatCSV {
			data, err = report.MonthCSV(*m)
		} else {
			data, err = report.MonthPDF(*m)
		}
	case ExportPortfolio:
		var p *report.Portfolio
		if p, err = s.portfolioReport(ctx, now); err != nil {
			return nil, err
		}
		name = "billsplit-net-worth-" + now.Format("2006-01-02")
		if format == FormatCSV {
			data, err = report.PortfolioCSV(*p)
		} else {
			data, err = report.PortfolioPDF(*p)
		}
	default:
		return nil, fmt.Errorf("%w: unknown report", domain.ErrInvalidArgument)
	}
	if err != nil {
		return nil, fmt.Errorf("render report: %w", err)
	}
	if format == FormatCSV {
		return &ExportFile{Filename: name + ".csv", ContentType: "text/csv; charset=utf-8", Data: data}, nil
	}
	return &ExportFile{Filename: name + ".pdf", ContentType: "application/pdf", Data: data}, nil
}

func (s *Service) monthReport(ctx context.Context, year, month int, now time.Time) (*report.Month, error) {
	if year == 0 || month == 0 {
		year, month = now.Year(), int(now.Month())
	}
	d, err := s.Dashboard(ctx, year, month) // validates year/month
	if err != nil {
		return nil, err
	}
	uid, _, err := s.home(ctx)
	if err != nil {
		return nil, err
	}
	from := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
	to := from.AddDate(0, 1, 0)
	txs, err := s.repo.ListTransactions(ctx, uid, domain.TransactionFilter{From: from, To: to, Limit: maxExportTx})
	if err != nil {
		return nil, err
	}
	cats, err := s.repo.ListCategories(ctx)
	if err != nil {
		return nil, err
	}
	catName := make(map[string]string, len(cats))
	for _, c := range cats {
		catName[c.ID] = c.Name
	}
	rateDate := to.AddDate(0, 0, -1)
	if rateDate.After(now) {
		rateDate = now
	}
	cv := &converter{ctx: ctx, fx: s.fx, home: d.Currency, date: rateDate, rates: map[string]float64{}}

	m := &report.Month{Month: from, Currency: d.Currency, GeneratedAt: now,
		IncomeCents: d.IncomeCents, ExpenseCents: d.ExpenseCents, NetCents: d.NetCents, SavingsRate: d.SavingsRate}
	for _, c := range d.ByCategory {
		if c.CategoryID == "transfer" {
			continue
		}
		m.Categories = append(m.Categories, report.Category{Name: c.Name, SpentCents: c.SpentCents, BudgetCents: c.BudgetCents})
	}
	for _, x := range d.TopMerchants {
		m.Merchants = append(m.Merchants, report.Merchant{Name: x.Merchant, SpentCents: x.SpentCents, Count: x.Count})
	}
	last := to.AddDate(0, 0, -1)
	if last.After(now) {
		last = day(now)
	}
	byDay := map[string]int64{}
	for _, x := range d.Daily {
		byDay[x.Date.Format(time.DateOnly)] = x.SpentCents
	}
	for at := from; !at.After(last); at = at.AddDate(0, 0, 1) {
		m.DailySpend = append(m.DailySpend, byDay[at.Format(time.DateOnly)])
	}
	// Oldest first reads like a statement.
	for i := len(txs) - 1; i >= 0; i-- {
		t := txs[i]
		home, err := cv.do(t.AmountCents, t.Currency)
		if err != nil {
			return nil, err
		}
		cat := catName[t.CategoryID]
		if cat == "" {
			cat = t.CategoryID
		}
		m.Transactions = append(m.Transactions, report.Tx{Date: t.OccurredAt, Description: t.Description, Merchant: t.Merchant,
			Category: cat, AmountCents: t.AmountCents, Currency: t.Currency, HomeCents: home})
	}
	if from.Year() == now.Year() && from.Month() == now.Month() {
		if f, err := s.MonthForecast(ctx); err == nil {
			m.HasForecast, m.ProjectedNetCents = true, f.ProjectedNetCents
		}
	}
	return m, nil
}

var investmentKindNames = map[domain.InvestmentKind]string{
	domain.InvestDeposit: "Deposit", domain.InvestStock: "Stock", domain.InvestETF: "ETF / fund",
	domain.InvestCrypto: "Crypto", domain.InvestBond: "Bond", domain.InvestMetal: "Metal", domain.InvestOther: "Other",
}

var accountKindNames = map[domain.AccountKind]string{
	domain.AccountCash: "Cash", domain.AccountCard: "Card", domain.AccountSavings: "Savings",
	domain.AccountCreditCard: "Credit card", domain.AccountLoan: "Loan", domain.AccountProperty: "Property",
}

func (s *Service) portfolioReport(ctx context.Context, now time.Time) (*report.Portfolio, error) {
	nw, err := s.NetWorth(ctx)
	if err != nil {
		return nil, err
	}
	pf, err := s.Portfolio(ctx)
	if err != nil {
		return nil, err
	}
	p := &report.Portfolio{Currency: nw.Currency, GeneratedAt: now, NetCents: nw.NetCents, AssetsCents: nw.AssetsCents,
		LiabilitiesCents: nw.LiabilitiesCents, CashCents: nw.CashCents, PropertyCents: nw.PropertyCents,
		InvestmentsCents: nw.InvestmentsCents, OwedToMeCents: nw.OwedToMeCents, CreditCents: nw.CreditCents,
		IOweCents: nw.IOweCents, GainCents: pf.GainCents, GainPercent: pf.GainPercent}
	for _, h := range pf.Items {
		p.Holdings = append(p.Holdings, report.Holding{Name: h.Name, Kind: investmentKindNames[h.Kind], Symbol: marketSymbol(h),
			Quantity: h.Quantity, Price: h.Price, Currency: h.Currency, InvestedCents: h.ContributedCents, ValueCents: h.ValueCents,
			ValueHomeCents: h.ValueHomeCents, GainPercent: h.GainPercent, RatePercent: h.RatePercent,
			EffectiveRatePercent: h.EffectiveRatePercent, MaturityAt: h.MaturityAt})
	}
	for _, a := range nw.Accounts {
		p.Accounts = append(p.Accounts, report.Account{Name: a.Name, Kind: accountKindNames[a.Kind], Currency: a.Currency,
			BalanceCents: a.BalanceCents, BalanceHomeCents: a.BalanceHomeCents, Liability: a.Kind.Liability()})
	}
	for _, h := range nw.History {
		p.NetHistory = append(p.NetHistory, report.Point{Date: h.Day, Cents: h.AssetsCents - h.LiabilitiesCents})
	}
	return p, nil
}

func marketSymbol(h Holding) string {
	if h.Market() {
		return h.Symbol
	}
	return ""
}
