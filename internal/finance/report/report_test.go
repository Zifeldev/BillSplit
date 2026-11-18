package report

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"
)

func sampleMonth() Month {
	m := Month{Month: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), Currency: "KZT",
		GeneratedAt: time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC),
		IncomeCents: 65000000, ExpenseCents: 41234550, NetCents: 23765450, SavingsRate: 0.366,
		HasForecast: true, ProjectedNetCents: 18000000,
		Categories: []Category{{"Groceries", 12000000, 15000000}, {"Cafés & restaurants", 8000000, 5000000}, {"Transport", 3000000, 0}},
		Merchants:  []Merchant{{"Magnum", 9000000, 12}, {"Yandex Go", 3000000, 20}},
	}
	for d := 0; d < 28; d++ {
		m.DailySpend = append(m.DailySpend, int64(500000+(d%7)*300000))
		m.Transactions = append(m.Transactions, Tx{Date: m.Month.AddDate(0, 0, d), Description: "Покупка в Magnum Cash&Carry, Алматы",
			Merchant: "Magnum", Category: "Groceries", AmountCents: -1234500, Currency: "KZT", HomeCents: -1234500})
	}
	m.Transactions = append(m.Transactions, Tx{Date: m.Month, Description: "Salary", Category: "Income",
		AmountCents: 65000000, Currency: "KZT", HomeCents: 65000000})
	return m
}

func TestMoney(t *testing.T) {
	for _, c := range []struct {
		cents int64
		cur   string
		want  string
	}{{123456789, "KZT", "₸1 234 567.89"}, {-150, "USD", "−$1.50"}, {1280, "JPY", "¥1,280"}, {100, "CHF", "1.00 CHF"}} {
		if got := Money(c.cents, c.cur); got != c.want {
			t.Errorf("Money(%d, %s) = %q, want %q", c.cents, c.cur, got, c.want)
		}
	}
}

func TestMonthCSV(t *testing.T) {
	b, err := MonthCSV(sampleMonth())
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.HasPrefix(s, bom+"Date,Description") || !strings.Contains(s, ",-12345.00,KZT,") ||
		!strings.Contains(s, "\"Покупка в Magnum Cash&Carry, Алматы\"") {
		t.Fatalf("unexpected csv:\n%s", s[:300])
	}
}

func TestPDFs(t *testing.T) {
	b, err := MonthPDF(sampleMonth())
	if err != nil || !bytes.HasPrefix(b, []byte("%PDF")) {
		t.Fatalf("month pdf: %v", err)
	}
	if dir := os.Getenv("REPORT_OUT"); dir != "" {
		_ = os.WriteFile(dir+"/month.pdf", b, 0o644)
	}
	p := Portfolio{Currency: "KZT", GeneratedAt: time.Now(), NetCents: 512345600, AssetsCents: 600000000, LiabilitiesCents: 87654400,
		CashCents: 150000000, InvestmentsCents: 450000000, CreditCents: 87654400,
		Holdings: []Holding{{Name: "Kaspi deposit", Kind: "Deposit", Currency: "KZT", InvestedCents: 300000000, ValueCents: 320000000,
			ValueHomeCents: 320000000, GainPercent: 6.67, RatePercent: 16, EffectiveRatePercent: 17.23},
			{Name: "Apple", Kind: "Stock", Symbol: "AAPL", Quantity: 3, Price: 231.5, Currency: "USD", InvestedCents: 60000,
				ValueCents: 69450, ValueHomeCents: 34000000, GainPercent: 15.75}},
		Accounts: []Account{{Name: "Kaspi Gold", Kind: "Card", Currency: "KZT", BalanceCents: 150000000, BalanceHomeCents: 150000000},
			{Name: "Kaspi Red", Kind: "Credit card", Currency: "KZT", BalanceCents: 87654400, BalanceHomeCents: 87654400, Liability: true}},
		NetHistory: []Point{{time.Now().AddDate(0, 0, -20), 480000000}, {time.Now().AddDate(0, 0, -10), 500000000}, {time.Now(), 512345600}}}
	b, err = PortfolioPDF(p)
	if err != nil || !bytes.HasPrefix(b, []byte("%PDF")) {
		t.Fatalf("portfolio pdf: %v", err)
	}
	if dir := os.Getenv("REPORT_OUT"); dir != "" {
		_ = os.WriteFile(dir+"/portfolio.pdf", b, 0o644)
	}
	if _, err := PortfolioCSV(p); err != nil {
		t.Fatal(err)
	}
}
