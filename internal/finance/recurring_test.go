package finance

import (
	"testing"
	"time"

	"github.com/Zifeldev/Billsplit/internal/domain"
)

var now = time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)

func tx(merchant, category string, cents int64, at time.Time) domain.Transaction {
	return domain.Transaction{Merchant: merchant, CategoryID: category, AmountCents: -cents, Currency: "USD", OccurredAt: at}
}

func TestDetectRecurringMonthlyNetflix(t *testing.T) {
	var txs []domain.Transaction
	start := time.Date(2026, 4, 5, 0, 0, 0, 0, time.UTC)
	for i := range 7 {
		txs = append(txs, tx("NETFLIX.COM 866-579-7172", "subscriptions", 1549, start.AddDate(0, i, 0)))
	}
	// Noise: same-day coffee purchases at random intervals.
	for _, d := range []int{1, 2, 9, 10, 30, 33, 70} {
		txs = append(txs, tx("STARBUCKS #1234", "dining", 550, start.AddDate(0, 0, d)))
	}

	got := DetectRecurring(txs, now)
	if len(got) != 1 {
		t.Fatalf("want 1 recurring, got %+v", got)
	}
	r := got[0]
	if r.Period != PeriodMonthly || r.Occurrences != 7 || r.AverageCents != 1549 {
		t.Fatalf("unexpected %+v", r)
	}
	if !r.IsSubscription || r.YearlyCostCents != 1549*12 {
		t.Fatalf("unexpected %+v", r)
	}
	last := start.AddDate(0, 6, 0)
	if !r.LastChargedAt.Equal(last) {
		t.Fatalf("last = %v", r.LastChargedAt)
	}
	if d := r.NextExpectedAt.Sub(last).Hours() / 24; d < 30 || d > 31 {
		t.Fatalf("next expected %v (+%.0f days)", r.NextExpectedAt, d)
	}
	if MonthlyTotal(got) != 1549 {
		t.Fatalf("monthly total %d", MonthlyTotal(got))
	}
}

func TestDetectRecurringWeekly(t *testing.T) {
	var txs []domain.Transaction
	start := now.AddDate(0, 0, -50)
	for i := range 7 {
		txs = append(txs, tx("", "groceries", 4000+int64(i%2)*300, start.AddDate(0, 0, 7*i)))
		txs[len(txs)-1].Description = "Farmers Market Box"
	}
	got := DetectRecurring(txs, now)
	if len(got) != 1 || got[0].Period != PeriodWeekly || got[0].Merchant != "Farmers Market Box" {
		t.Fatalf("got %+v", got)
	}
	if got[0].IsSubscription {
		t.Fatal("groceries box is not a SaaS subscription")
	}
	if got[0].YearlyCostCents != got[0].AverageCents*52 {
		t.Fatalf("yearly %d", got[0].YearlyCostCents)
	}
}

func TestDetectRecurringIrregular(t *testing.T) {
	var txs []domain.Transaction
	at := now.AddDate(0, -6, 0)
	for _, gap := range []int{3, 20, 45, 11, 60, 2} {
		at = at.AddDate(0, 0, gap)
		txs = append(txs, tx("AMAZON", "shopping", 2500, at))
	}
	if got := DetectRecurring(txs, now); len(got) != 0 {
		t.Fatalf("irregular charges must not be recurring: %+v", got)
	}
}

func TestDetectRecurringAmounts(t *testing.T) {
	start := now.AddDate(0, -5, 0)
	t.Run("price change within 20%", func(t *testing.T) {
		var txs []domain.Transaction
		for i, c := range []int64{1549, 1549, 1549, 1799, 1799} {
			txs = append(txs, tx("SPOTIFY USA", "other", c, start.AddDate(0, i, 0)))
		}
		got := DetectRecurring(txs, now)
		if len(got) != 1 {
			t.Fatalf("got %+v", got)
		}
		if got[0].AverageCents != (1549*3+1799*2)/5 || !got[0].IsSubscription {
			t.Fatalf("got %+v", got[0])
		}
	})
	t.Run("amounts vary too much", func(t *testing.T) {
		var txs []domain.Transaction
		for i, c := range []int64{12000, 4500, 9800, 15000} {
			txs = append(txs, tx("CON EDISON", "utilities", c, start.AddDate(0, i, 0)))
		}
		if got := DetectRecurring(txs, now); len(got) != 0 {
			t.Fatalf("got %+v", got)
		}
	})
}

func TestDetectRecurringYearlyAndRules(t *testing.T) {
	txs := []domain.Transaction{
		tx("ADOBE INC", "other", 23988, now.AddDate(-1, -1, 0)),
		tx("Adobe Inc.", "other", 23988, now.AddDate(0, -1, 0)),
		// Two monthly charges are not enough.
		tx("GYM", "health", 3000, now.AddDate(0, -2, 0)),
		tx("GYM", "health", 3000, now.AddDate(0, -1, 0)),
		// Income and transfers are ignored.
		{Merchant: "PAYROLL", CategoryID: "income", AmountCents: 500000, OccurredAt: now.AddDate(0, -2, 0)},
		{Merchant: "PAYROLL", CategoryID: "income", AmountCents: 500000, OccurredAt: now.AddDate(0, -1, 0)},
		{Merchant: "PAYROLL", CategoryID: "income", AmountCents: 500000, OccurredAt: now},
	}
	got := DetectRecurring(txs, now)
	if len(got) != 1 || got[0].Period != PeriodYearly || got[0].YearlyCostCents != 23988 || !got[0].IsSubscription {
		t.Fatalf("got %+v", got)
	}
}

func TestDetectRecurringCancelledAndSorted(t *testing.T) {
	var txs []domain.Transaction
	old := now.AddDate(0, -10, 0)
	for i := range 4 { // stopped 6 months ago
		txs = append(txs, tx("HULU", "subscriptions", 999, old.AddDate(0, i, 0)))
	}
	recent := now.AddDate(0, -4, 0)
	for i := range 4 {
		txs = append(txs, tx("RENT LLC", "housing", 150000, recent.AddDate(0, i, 0)))
		txs = append(txs, tx("CHATGPT", "subscriptions", 2000, recent.AddDate(0, i, 1)))
	}
	got := DetectRecurring(txs, now)
	if len(got) != 2 || got[0].Merchant != "RENT LLC" || got[1].Merchant != "CHATGPT" {
		t.Fatalf("got %+v", got)
	}
}

func TestNormalizeMerchant(t *testing.T) {
	for in, want := range map[string]string{
		"NETFLIX.COM 866-579-7172": "netflix",
		"PAYPAL *NETFLIX.COM":      "netflix",
		"Netflix":                  "netflix",
		"SQ *BLUE BOTTLE #123":     "blue bottle",
		"Adobe Inc.":               "adobe",
		"12345":                    "",
	} {
		if got := NormalizeMerchant(in); got != want {
			t.Errorf("NormalizeMerchant(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFormatMoney(t *testing.T) {
	for _, c := range []struct {
		cents int64
		cur   string
		want  string
	}{
		{1549, "USD", "$15.49"},
		{5, "EUR", "€0.05"},
		{1280, "JPY", "¥1280"},
		{-1280, "KRW", "-₩1280"},
		{1549, "CHF", "15.49 CHF"},
	} {
		if got := FormatMoney(c.cents, c.cur); got != c.want {
			t.Errorf("FormatMoney(%d, %s) = %q, want %q", c.cents, c.cur, got, c.want)
		}
	}
}
