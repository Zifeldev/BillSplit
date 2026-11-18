package finance

import (
	"context"
	"math"
	"sort"
	"time"
)

// Month-end forecast.
//
// Actual income/spending up to today comes from transactions. The rest of the
// month follows the user's own pattern: for each remaining day of the month
// the average of the same day in the last few months. Spending is scaled by
// how this month compares with the same days of those months (spending more
// than usual so far → expect more). Income already received this month is
// subtracted from the usual monthly income, so a salary paid early isn't
// counted twice. Without history the current daily pace is extended.

const forecastHistoryMonths = 3

// dayFlow is one day of income and spending (home currency, both >= 0).
type dayFlow struct{ in, out float64 }

type forecastResult struct {
	incomeSoFar, expenseSoFar   float64
	expectedIncome, expectedOut float64
	cumulative                  []float64 // index 0 = day 1
	historyMonths               int
}

// forecastMonth: cur and hist are keyed by day of month (1..31); today is the
// day of month already passed; days is the month length.
func forecastMonth(cur map[int]dayFlow, hist []map[int]dayFlow, today, days int) forecastResult {
	var r forecastResult
	for d := 1; d <= today; d++ {
		r.incomeSoFar += cur[d].in
		r.expenseSoFar += cur[d].out
	}

	// Usual pattern for the remaining days.
	restIn := make([]float64, days+1)
	restOut := make([]float64, days+1)
	var histSoFarOut, histRestIn, histTotalIn float64
	for _, m := range hist {
		if len(m) == 0 {
			continue
		}
		r.historyMonths++
		for d, f := range m {
			histTotalIn += f.in
			if d <= today {
				histSoFarOut += f.out
				continue
			}
			if d > days { // e.g. the 31st of a longer month: fold into the last day
				d = days
			}
			restIn[d] += f.in
			restOut[d] += f.out
			histRestIn += f.in
		}
	}

	if r.historyMonths > 0 {
		n := float64(r.historyMonths)
		histSoFarOut /= n
		histRestIn /= n
		histTotalIn /= n
		scale := 1.0
		if histSoFarOut > 0 && r.expenseSoFar > 0 {
			scale = math.Min(2, math.Max(0.5, r.expenseSoFar/histSoFarOut))
		}
		inScale := 0.0
		if histRestIn > 0 {
			want := math.Min(histRestIn, math.Max(0, histTotalIn-r.incomeSoFar))
			inScale = want / histRestIn
		}
		for d := today + 1; d <= days; d++ {
			restIn[d] = restIn[d] / n * inScale
			restOut[d] = restOut[d] / n * scale
		}
	} else if today > 0 {
		pace := r.expenseSoFar / float64(today)
		for d := today + 1; d <= days; d++ {
			restIn[d], restOut[d] = 0, pace
		}
	}

	var cum float64
	for d := 1; d <= days; d++ {
		if d <= today {
			cum += cur[d].in - cur[d].out
		} else {
			cum += restIn[d] - restOut[d]
			r.expectedIncome += restIn[d]
			r.expectedOut += restOut[d]
		}
		r.cumulative = append(r.cumulative, cum)
	}
	return r
}

type ForecastDay struct {
	Date     time.Time
	NetCents int64
	Actual   bool
}

type UpcomingCharge struct {
	Merchant    string
	AmountCents int64
	Date        time.Time
}

type MonthForecast struct {
	Currency                                  string
	Month                                     time.Time
	DaysLeft                                  int
	IncomeSoFarCents, ExpenseSoFarCents       int64
	ExpectedIncomeCents, ExpectedExpenseCents int64
	ProjectedNetCents                         int64
	HasBalance                                bool
	BalanceNowCents, ProjectedBalanceCents    int64
	Days                                      []ForecastDay
	Upcoming                                  []UpcomingCharge
	HistoryMonths                             int
}

func (s *Service) MonthForecast(ctx context.Context) (*MonthForecast, error) {
	uid, home, err := s.home(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	from := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	to := from.AddDate(0, 1, 0)
	days := to.AddDate(0, 0, -1).Day()
	today := now.Day()
	cv := &converter{ctx: ctx, fx: s.fx, home: home, date: now, rates: map[string]float64{}}

	load := func(f, t time.Time) (map[int]dayFlow, error) {
		rows, err := s.repo.DailyFlows(ctx, uid, f, t)
		if err != nil {
			return nil, err
		}
		out := map[int]dayFlow{}
		for _, x := range rows {
			in, err := cv.do(x.IncomeCents, x.Currency)
			if err != nil {
				return nil, err
			}
			ex, err := cv.do(x.ExpenseCents, x.Currency)
			if err != nil {
				return nil, err
			}
			d := x.Date.UTC().Day()
			v := out[d]
			v.in += float64(in)
			v.out += float64(ex)
			out[d] = v
		}
		return out, nil
	}
	cur, err := load(from, to)
	if err != nil {
		return nil, err
	}
	var hist []map[int]dayFlow
	for k := 1; k <= forecastHistoryMonths; k++ {
		m, err := load(from.AddDate(0, -k, 0), from.AddDate(0, -k+1, 0))
		if err != nil {
			return nil, err
		}
		hist = append(hist, m)
	}
	r := forecastMonth(cur, hist, today, days)

	f := &MonthForecast{
		Currency: home, Month: from, DaysLeft: days - today, HistoryMonths: r.historyMonths,
		IncomeSoFarCents: round(r.incomeSoFar), ExpenseSoFarCents: round(r.expenseSoFar),
		ExpectedIncomeCents: round(r.expectedIncome), ExpectedExpenseCents: round(r.expectedOut),
	}
	f.ProjectedNetCents = f.IncomeSoFarCents + f.ExpectedIncomeCents - f.ExpenseSoFarCents - f.ExpectedExpenseCents
	for i, v := range r.cumulative {
		f.Days = append(f.Days, ForecastDay{Date: from.AddDate(0, 0, i), NetCents: round(v), Actual: i+1 <= today})
	}

	accs, err := s.accounts(ctx, uid, home, now)
	if err != nil {
		return nil, err
	}
	for _, a := range accs {
		if a.Kind.Liquid() {
			f.HasBalance = true
			f.BalanceNowCents += a.BalanceHomeCents
		}
	}
	f.ProjectedBalanceCents = f.BalanceNowCents + f.ExpectedIncomeCents - f.ExpectedExpenseCents

	if rec, err := s.recurring(ctx, uid, home, now); err == nil {
		end := to.AddDate(0, 0, -1)
		for _, it := range rec.Items {
			next := day(it.NextExpectedAt)
			if next.After(day(now)) && !next.After(end) {
				f.Upcoming = append(f.Upcoming, UpcomingCharge{Merchant: it.Merchant, AmountCents: it.AverageCents, Date: next})
			}
		}
		sort.Slice(f.Upcoming, func(i, j int) bool { return f.Upcoming[i].Date.Before(f.Upcoming[j].Date) })
	}
	return f, nil
}

func round(v float64) int64 { return int64(math.Round(v)) }
