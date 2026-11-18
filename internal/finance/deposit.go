package finance

import (
	"math"
	"time"

	"github.com/Zifeldev/Billsplit/internal/domain"
)

// Deposit math. The model is month based, as banks quote it: interest accrues
// monthly at rate/12 on the current balance, joins the principal every
// capitalization period, and the monthly top-up arrives at the start of each
// following month. A partially elapsed month accrues pro rata by days. The
// mobile calculator (apps/mobile/src/lib/invest.ts) implements the same model.

// capEvery is the capitalization period in months; 0 — never (simple interest).
func capEvery(c domain.Capitalization) int {
	switch c {
	case domain.CapQuarterly:
		return 3
	case domain.CapYearly:
		return 12
	case domain.CapNone:
		return 0
	default:
		return 1
	}
}

// EffectiveRate is the effective annual yield (percent) of a nominal rate with capitalization.
func EffectiveRate(ratePercent float64, c domain.Capitalization) float64 {
	n := capEvery(c)
	if n == 0 || ratePercent <= 0 {
		return ratePercent
	}
	periods := 12 / float64(n)
	return (math.Pow(1+ratePercent/100/periods, periods) - 1) * 100
}

// MaturityAt is when a term deposit ends; nil for open-ended ones.
func MaturityAt(inv domain.Investment) *time.Time {
	if inv.TermMonths <= 0 {
		return nil
	}
	t := inv.OpenedAt.AddDate(0, inv.TermMonths, 0)
	return &t
}

// DepositState is a deposit's value and the money put into it, at some date.
type DepositState struct {
	Value       float64 // minor units, unrounded
	Contributed int64
}

// DepositAt values a rate-based investment at date `at` (truncated to days).
// Before opening it is zero; after maturity it stays at the maturity value.
func DepositAt(inv domain.Investment, at time.Time) DepositState {
	start := day(inv.OpenedAt)
	at = day(at)
	if at.Before(start) {
		return DepositState{}
	}
	if m := MaturityAt(inv); m != nil && at.After(*m) {
		at = *m
	}
	// Whole months elapsed since opening, then the fraction of the current one.
	months := (at.Year()-start.Year())*12 + int(at.Month()-start.Month())
	if start.AddDate(0, months, 0).After(at) {
		months--
	}
	periodStart := start.AddDate(0, months, 0)
	periodEnd := start.AddDate(0, months+1, 0)
	frac := at.Sub(periodStart).Hours() / periodEnd.Sub(periodStart).Hours()

	r := inv.RatePercent / 100 / 12
	every := capEvery(inv.Capitalization)
	balance := float64(inv.InvestedCents)
	contributed := inv.InvestedCents
	accrued := 0.0
	for k := 1; k <= months; k++ {
		accrued += balance * r
		if every > 0 && k%every == 0 {
			balance += accrued
			accrued = 0
		}
		if inv.TermMonths == 0 || k < inv.TermMonths {
			balance += float64(inv.MonthlyContributionCents)
			contributed += inv.MonthlyContributionCents
		}
	}
	accrued += balance * r * frac
	return DepositState{Value: balance + accrued, Contributed: contributed}
}

func day(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}
