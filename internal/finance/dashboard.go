package finance

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/Zifeldev/Billsplit/internal/domain"
	"github.com/Zifeldev/Billsplit/internal/platform/fx"
	"github.com/Zifeldev/Billsplit/internal/platform/logger"
)

type Dashboard struct {
	Currency                  string
	IncomeCents, ExpenseCents int64
	NetCents                  int64
	SavingsRate               float64
	ByCategory                []CategorySpend
	Daily                     []domain.DailyTotal
	TopMerchants              []domain.MerchantTotal
	Needs, Wants, Savings     float64 // share of income, 50/30/20 rule
	Last6Months               []domain.MonthTotal
	ProjectedExpenseCents     int64
	Insights                  []string
	OwedToMeCents, IOweCents  int64
}

type CategorySpend struct {
	domain.CategoryTotal
	BudgetCents int64
	BudgetUsed  float64
}

// converter converts amounts to the home currency at one fixed date (end of the month).
type converter struct {
	ctx   context.Context
	fx    FX
	home  string
	date  time.Time
	rates map[string]float64
}

func (c *converter) do(cents int64, cur string) (int64, error) {
	if cur == c.home {
		return cents, nil
	}
	r, ok := c.rates[cur]
	if !ok {
		var err error
		if r, err = c.fx.Rate(c.ctx, cur, c.home, c.date); err != nil {
			return 0, fmt.Errorf("rate %s->%s: %w", cur, c.home, err)
		}
		c.rates[cur] = r
	}
	return fx.Apply(cents, r, cur, c.home), nil
}

// Dashboard builds the monthly overview in the home currency. Cached in Redis per (user, version, month).
func (s *Service) Dashboard(ctx context.Context, year, month int) (*Dashboard, error) {
	uid, home, err := s.home(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	if year == 0 || month == 0 {
		year, month = now.Year(), int(now.Month())
	}
	if month < 1 || month > 12 || year < 2000 || year > 2100 {
		return nil, fmt.Errorf("%w: bad year/month", domain.ErrInvalidArgument)
	}

	ver, _ := s.cache.GetInt(ctx, dashVersionKey(uid))
	key := fmt.Sprintf("finance:dash:%s:%s:%d:%04d-%02d", uid, home, ver, year, month)
	var d Dashboard
	if err := s.cache.Get(ctx, key, &d); err == nil {
		return &d, nil
	}
	dp, err := s.buildDashboard(ctx, uid, home, year, month, now)
	if err != nil {
		return nil, err
	}
	// Short TTL: owed/owe comes from split and isn't covered by the version key.
	if err := s.cache.Set(ctx, key, dp, min(s.ttl, time.Minute)); err != nil {
		logger.FromContext(ctx).Warn("cache set failed", "err", err)
	}
	return dp, nil
}

func (s *Service) buildDashboard(ctx context.Context, uid, home string, year, month int, now time.Time) (*Dashboard, error) {
	from := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
	to := from.AddDate(0, 1, 0)
	rateDate := to.AddDate(0, 0, -1)
	if rateDate.After(now) {
		rateDate = now
	}
	cv := &converter{ctx: ctx, fx: s.fx, home: home, date: rateDate, rates: map[string]float64{}}
	d := &Dashboard{Currency: home}

	totals, err := s.repo.IncomeExpense(ctx, uid, from, to)
	if err != nil {
		return nil, err
	}
	for _, t := range totals {
		inc, err := cv.do(t.IncomeCents, t.Currency)
		if err != nil {
			return nil, err
		}
		exp, err := cv.do(t.ExpenseCents, t.Currency)
		if err != nil {
			return nil, err
		}
		d.IncomeCents += inc
		d.ExpenseCents += exp
	}
	d.NetCents = d.IncomeCents - d.ExpenseCents
	if d.IncomeCents > 0 {
		d.SavingsRate = float64(d.NetCents) / float64(d.IncomeCents)
	}

	// Categories (merge currencies).
	cats, err := s.repo.SpendByCategory(ctx, uid, from, to)
	if err != nil {
		return nil, err
	}
	merged := map[string]*domain.CategoryTotal{}
	for _, c := range cats {
		v, err := cv.do(c.SpentCents, c.Currency)
		if err != nil {
			return nil, err
		}
		if m, ok := merged[c.CategoryID]; ok {
			m.SpentCents += v
		} else {
			c.SpentCents, c.Currency = v, home
			merged[c.CategoryID] = &c
		}
	}
	budgets, err := s.repo.ListBudgets(ctx, uid)
	if err != nil {
		return nil, err
	}
	limits := make(map[string]int64, len(budgets))
	for _, b := range budgets {
		limits[b.CategoryID] = b.LimitCents
	}
	var needs, wants, savingsSpent int64
	for _, c := range merged {
		cs := CategorySpend{CategoryTotal: *c, BudgetCents: limits[c.CategoryID]}
		if cs.BudgetCents > 0 {
			cs.BudgetUsed = float64(c.SpentCents) / float64(cs.BudgetCents)
		}
		d.ByCategory = append(d.ByCategory, cs)
		switch c.Kind {
		case domain.KindNeeds:
			needs += c.SpentCents
		case domain.KindWants:
			if c.CategoryID != "transfer" {
				wants += c.SpentCents
			}
		case domain.KindSavings:
			savingsSpent += c.SpentCents
		}
	}
	sort.Slice(d.ByCategory, func(i, j int) bool { return d.ByCategory[i].SpentCents > d.ByCategory[j].SpentCents })
	if d.IncomeCents > 0 {
		inc := float64(d.IncomeCents)
		d.Needs, d.Wants = float64(needs)/inc, float64(wants)/inc
		d.Savings = float64(savingsSpent+max(d.NetCents, 0)) / inc
	}

	// Daily.
	days, err := s.repo.SpendByDay(ctx, uid, from, to)
	if err != nil {
		return nil, err
	}
	byDay := map[time.Time]int64{}
	for _, x := range days {
		v, err := cv.do(x.SpentCents, x.Currency)
		if err != nil {
			return nil, err
		}
		byDay[x.Date] += v
	}
	for day, v := range byDay {
		d.Daily = append(d.Daily, domain.DailyTotal{Currency: home, Date: day, SpentCents: v})
	}
	sort.Slice(d.Daily, func(i, j int) bool { return d.Daily[i].Date.Before(d.Daily[j].Date) })

	// Merchants.
	ms, err := s.repo.TopMerchants(ctx, uid, from, to)
	if err != nil {
		return nil, err
	}
	byM := map[string]*domain.MerchantTotal{}
	for _, m := range ms {
		v, err := cv.do(m.SpentCents, m.Currency)
		if err != nil {
			return nil, err
		}
		if x, ok := byM[m.Merchant]; ok {
			x.SpentCents += v
			x.Count += m.Count
		} else {
			byM[m.Merchant] = &domain.MerchantTotal{Currency: home, Merchant: m.Merchant, SpentCents: v, Count: m.Count}
		}
	}
	for _, m := range byM {
		d.TopMerchants = append(d.TopMerchants, *m)
	}
	sort.Slice(d.TopMerchants, func(i, j int) bool { return d.TopMerchants[i].SpentCents > d.TopMerchants[j].SpentCents })
	if len(d.TopMerchants) > 5 {
		d.TopMerchants = d.TopMerchants[:5]
	}

	// Last 6 months (same rate date — good enough for a trend chart).
	months, err := s.repo.MonthlyTotals(ctx, uid, from.AddDate(0, -5, 0), to)
	if err != nil {
		return nil, err
	}
	byMonth := map[time.Time]*domain.MonthTotal{}
	for _, m := range months {
		inc, err := cv.do(m.IncomeCents, m.Currency)
		if err != nil {
			return nil, err
		}
		exp, err := cv.do(m.ExpenseCents, m.Currency)
		if err != nil {
			return nil, err
		}
		if x, ok := byMonth[m.Month]; ok {
			x.IncomeCents += inc
			x.ExpenseCents += exp
		} else {
			byMonth[m.Month] = &domain.MonthTotal{Currency: home, Month: m.Month, IncomeCents: inc, ExpenseCents: exp}
		}
	}
	for _, m := range byMonth {
		d.Last6Months = append(d.Last6Months, *m)
	}
	sort.Slice(d.Last6Months, func(i, j int) bool { return d.Last6Months[i].Month.Before(d.Last6Months[j].Month) })

	// Bill-splitting balances (graceful degradation if split is down).
	if d.OwedToMeCents, d.IOweCents, err = s.split.UserTotals(ctx, uid, home); err != nil {
		logger.FromContext(ctx).Warn("split totals unavailable", "err", err)
	}

	// Linear projection for the current month.
	d.ProjectedExpenseCents = d.ExpenseCents
	if now.After(from) && now.Before(to) {
		d.ProjectedExpenseCents = d.ExpenseCents * int64(to.AddDate(0, 0, -1).Day()) / int64(now.Day())
	}

	var prevExpense int64
	if prev, err := s.repo.IncomeExpense(ctx, uid, from.AddDate(0, -1, 0), from); err == nil {
		for _, t := range prev {
			v, err := cv.do(t.ExpenseCents, t.Currency)
			if err == nil {
				prevExpense += v
			}
		}
	}
	d.Insights = insights(d, prevExpense)
	return d, nil
}

// insights produces simple financial-literacy tips from the numbers.
func insights(d *Dashboard, prevExpense int64) []string {
	var out []string
	money := func(c int64) string { return FormatMoney(c, d.Currency) }

	for _, c := range d.ByCategory {
		if c.BudgetCents > 0 && c.BudgetUsed >= 1 {
			out = append(out, fmt.Sprintf("Budget for %s exceeded: spent %s of %s.", c.Name, money(c.SpentCents), money(c.BudgetCents)))
		} else if c.BudgetCents > 0 && c.BudgetUsed >= 0.8 {
			out = append(out, fmt.Sprintf("%s budget is %.0f%% used.", c.Name, c.BudgetUsed*100))
		}
	}
	if prevExpense > 0 && d.ExpenseCents > prevExpense*12/10 {
		out = append(out, fmt.Sprintf("Spending is %.0f%% higher than last month.", (float64(d.ExpenseCents)/float64(prevExpense)-1)*100))
	}
	if d.IncomeCents > 0 {
		if d.SavingsRate < 0.2 {
			out = append(out, fmt.Sprintf("Savings rate is %.0f%%. The 50/30/20 rule suggests saving at least 20%% of income.", d.SavingsRate*100))
		}
		if d.Wants > 0.3 {
			out = append(out, fmt.Sprintf("Wants take %.0f%% of income (target ≤ 30%%). Check subscriptions and dining out.", d.Wants*100))
		}
		if d.Needs > 0.5 {
			out = append(out, fmt.Sprintf("Needs take %.0f%% of income (target ≤ 50%%).", d.Needs*100))
		}
		if d.ProjectedExpenseCents > d.IncomeCents {
			out = append(out, fmt.Sprintf("At this pace you'll spend %s this month — more than your income.", money(d.ProjectedExpenseCents)))
		}
	}
	for _, c := range d.ByCategory {
		if c.CategoryID == "subscriptions" && c.SpentCents > 0 {
			out = append(out, fmt.Sprintf("Subscriptions cost %s/month (%s/year). Cancel the ones you don't use.",
				money(c.SpentCents), money(c.SpentCents*12)))
		}
	}
	if d.IOweCents > 0 {
		out = append(out, fmt.Sprintf("You owe friends %s. Settle up to keep balances clean.", money(d.IOweCents)))
	}
	return out
}

var currencySymbols = map[string]string{"USD": "$", "EUR": "€", "GBP": "£", "JPY": "¥", "KRW": "₩", "INR": "₹"}

// FormatMoney renders minor units respecting the currency's decimals:
// 1549 USD -> "$15.49", 1280 JPY -> "¥1280", 1549 CHF -> "15.49 CHF".
func FormatMoney(cents int64, currency string) string {
	sign := ""
	if cents < 0 {
		sign, cents = "-", -cents
	}
	var num string
	if units := fx.MinorUnits(currency); units == 0 {
		num = fmt.Sprintf("%d", cents)
	} else {
		div := int64(math.Pow10(units))
		num = fmt.Sprintf("%d.%0*d", cents/div, units, cents%div)
	}
	if sym, ok := currencySymbols[currency]; ok {
		return sign + sym + num
	}
	return sign + num + " " + currency
}
