package finance

import (
	"math"
	"testing"
)

func near(a, b float64) bool { return math.Abs(a-b) < 0.01 }

func TestForecastPaceWithoutHistory(t *testing.T) {
	cur := map[int]dayFlow{1: {in: 1000, out: 100}, 5: {out: 400}}
	r := forecastMonth(cur, nil, 10, 30)
	// 500 spent in 10 days → 50/day for the remaining 20 days.
	if !near(r.expectedOut, 1000) || r.expectedIncome != 0 || r.historyMonths != 0 {
		t.Fatalf("pace: %+v", r)
	}
	if got := r.cumulative[29]; !near(got, 1000-500-1000) {
		t.Fatalf("month end = %v", got)
	}
}

func TestForecastSalaryFromHistory(t *testing.T) {
	// Salary 3000 on the 25th, rent 1000 on the 1st, 20/day otherwise.
	month := func() map[int]dayFlow {
		m := map[int]dayFlow{}
		for d := 1; d <= 30; d++ {
			m[d] = dayFlow{out: 20}
		}
		m[1] = dayFlow{out: 1020}
		m[25] = dayFlow{in: 3000, out: 20}
		return m
	}
	cur := map[int]dayFlow{}
	for d := 1; d <= 10; d++ {
		cur[d] = month()[d]
	}
	r := forecastMonth(cur, []map[int]dayFlow{month(), month(), {}}, 10, 30)
	if r.historyMonths != 2 {
		t.Fatalf("history months = %d", r.historyMonths)
	}
	if !near(r.expectedIncome, 3000) {
		t.Fatalf("expected income = %v", r.expectedIncome)
	}
	if !near(r.expectedOut, 20*20) {
		t.Fatalf("expected spending = %v", r.expectedOut)
	}
	// The jump happens on the 25th.
	if r.cumulative[24]-r.cumulative[23] < 2900 {
		t.Fatalf("no salary jump: %v", r.cumulative[20:26])
	}
}

func TestForecastSalaryAlreadyPaid(t *testing.T) {
	hist := map[int]dayFlow{25: {in: 3000}}
	cur := map[int]dayFlow{20: {in: 3000}} // paid early this month
	r := forecastMonth(cur, []map[int]dayFlow{hist}, 22, 30)
	if r.expectedIncome != 0 {
		t.Fatalf("salary counted twice: %v", r.expectedIncome)
	}
}

func TestForecastScalesSpending(t *testing.T) {
	hist := map[int]dayFlow{}
	for d := 1; d <= 30; d++ {
		hist[d] = dayFlow{out: 10}
	}
	cur := map[int]dayFlow{}
	for d := 1; d <= 10; d++ {
		cur[d] = dayFlow{out: 15} // 1.5x the usual pace
	}
	r := forecastMonth(cur, []map[int]dayFlow{hist}, 10, 30)
	if !near(r.expectedOut, 20*10*1.5) {
		t.Fatalf("expected spending = %v", r.expectedOut)
	}
}
