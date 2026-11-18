package finance

import (
	"math"
	"testing"
	"time"

	"github.com/Zifeldev/Billsplit/internal/domain"
)

func date(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

func TestDepositMonthlyCapitalization(t *testing.T) {
	inv := domain.Investment{Kind: domain.InvestDeposit, InvestedCents: 1_000_000_00, RatePercent: 12,
		Capitalization: domain.CapMonthly, OpenedAt: date(2025, 1, 15)}
	got := DepositAt(inv, date(2026, 1, 15))
	want := 1_000_000_00 * math.Pow(1.01, 12) // 1 126 825.03
	if math.Abs(got.Value-want) > 1 {
		t.Fatalf("value = %.2f, want %.2f", got.Value, want)
	}
	if got.Contributed != 1_000_000_00 {
		t.Fatalf("contributed = %d", got.Contributed)
	}
	if e := EffectiveRate(12, domain.CapMonthly); math.Abs(e-12.6825) > 0.001 {
		t.Fatalf("effective = %v", e)
	}
}

func TestDepositSimpleAndQuarterly(t *testing.T) {
	base := domain.Investment{InvestedCents: 100_000, RatePercent: 10, OpenedAt: date(2025, 1, 1)}
	simple := base
	simple.Capitalization = domain.CapNone
	if v := DepositAt(simple, date(2026, 1, 1)).Value; math.Abs(v-110_000) > 0.01 {
		t.Fatalf("simple = %v", v)
	}
	q := base
	q.Capitalization = domain.CapQuarterly
	want := 100_000 * math.Pow(1+0.10/4, 4)
	if v := DepositAt(q, date(2026, 1, 1)).Value; math.Abs(v-want) > 0.01 {
		t.Fatalf("quarterly = %v, want %v", v, want)
	}
}

func TestDepositTopUpsTermAndBounds(t *testing.T) {
	inv := domain.Investment{InvestedCents: 1000_00, MonthlyContributionCents: 100_00, RatePercent: 0,
		Capitalization: domain.CapMonthly, OpenedAt: date(2025, 3, 10), TermMonths: 6}
	if s := DepositAt(inv, date(2025, 3, 1)); s.Value != 0 || s.Contributed != 0 {
		t.Fatalf("before opening: %+v", s)
	}
	// 0% rate: value == contributed. Top-ups after months 1..5 (none at maturity).
	s := DepositAt(inv, date(2027, 1, 1))
	if s.Contributed != 1500_00 || math.Abs(s.Value-1500_00) > 0.001 {
		t.Fatalf("after maturity: %+v", s)
	}
	mid := DepositAt(inv, date(2025, 5, 25)) // 2 whole months -> 2 top-ups
	if mid.Contributed != 1200_00 {
		t.Fatalf("mid contributed = %d", mid.Contributed)
	}
}

func TestDepositPartialMonthAccrues(t *testing.T) {
	inv := domain.Investment{InvestedCents: 1_200_00, RatePercent: 12, Capitalization: domain.CapMonthly,
		OpenedAt: date(2025, 4, 1)}
	// April has 30 days; on the 16th half the monthly interest (1% of 1200 = 12) has accrued.
	if v := DepositAt(inv, date(2025, 4, 16)).Value; math.Abs(v-1_206_00) > 0.5 {
		t.Fatalf("partial = %v", v)
	}
}
