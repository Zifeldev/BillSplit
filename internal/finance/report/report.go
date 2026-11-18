// Package report renders downloadable finance reports (CSV and PDF) from
// plain data. It knows nothing about storage: the finance service collects
// the numbers, this package only lays them out.
package report

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/Zifeldev/Billsplit/internal/platform/fx"
)

type Category struct {
	Name        string
	SpentCents  int64
	BudgetCents int64
}

type Merchant struct {
	Name       string
	SpentCents int64
	Count      int
}

type Tx struct {
	Date        time.Time
	Description string
	Merchant    string
	Category    string
	AmountCents int64 // <0 expense
	Currency    string
	HomeCents   int64
}

type Month struct {
	Month       time.Time
	Currency    string
	GeneratedAt time.Time

	IncomeCents, ExpenseCents, NetCents int64
	SavingsRate                         float64
	Categories                          []Category
	DailySpend                          []int64 // index 0 = day 1
	Merchants                           []Merchant
	Transactions                        []Tx
	// Current month only.
	HasForecast       bool
	ProjectedNetCents int64
}

type Holding struct {
	Name, Kind, Symbol   string
	Quantity, Price      float64
	Currency             string
	InvestedCents        int64
	ValueCents           int64
	ValueHomeCents       int64
	GainPercent          float64
	RatePercent          float64
	MaturityAt           *time.Time
	EffectiveRatePercent float64
}

type Account struct {
	Name, Kind       string
	Currency         string
	BalanceCents     int64
	BalanceHomeCents int64
	Liability        bool
}

type Point struct {
	Date  time.Time
	Cents int64
}

type Portfolio struct {
	Currency    string
	GeneratedAt time.Time

	NetCents, AssetsCents, LiabilitiesCents int64
	CashCents, PropertyCents                int64
	InvestmentsCents, OwedToMeCents         int64
	CreditCents, IOweCents                  int64
	GainCents                               int64
	GainPercent                             float64
	Holdings                                []Holding
	Accounts                                []Account
	NetHistory                              []Point
}

// ---------- money ----------

var symbols = map[string]string{"USD": "$", "EUR": "€", "GBP": "£", "JPY": "¥", "KZT": "₸", "RUB": "₽",
	"UAH": "₴", "TRY": "₺", "INR": "₹", "KRW": "₩", "CNY": "¥"}

// Money renders minor units with thousands separators: "₸1 234 567", "$1,234.56".
func Money(cents int64, cur string) string {
	return money(cents, cur, true)
}

func money(cents int64, cur string, symbol bool) string {
	sign := ""
	if cents < 0 {
		sign, cents = "−", -cents
	}
	units := fx.MinorUnits(cur)
	div := int64(math.Pow10(units))
	whole := group(cents/div, sepFor(cur))
	num := whole
	if units > 0 {
		num = fmt.Sprintf("%s.%0*d", whole, units, cents%div)
	}
	if !symbol {
		return sign + num
	}
	if s, ok := symbols[cur]; ok {
		return sign + s + num
	}
	return sign + num + " " + cur
}

func sepFor(cur string) string {
	if cur == "USD" || cur == "GBP" || cur == "JPY" || cur == "CNY" || cur == "INR" || cur == "KRW" {
		return ","
	}
	return " "
}

func group(n int64, sep string) string {
	s := strconv.FormatInt(n, 10)
	if len(s) <= 3 {
		return s
	}
	var b strings.Builder
	pre := len(s) % 3
	if pre > 0 {
		b.WriteString(s[:pre])
	}
	for i := pre; i < len(s); i += 3 {
		if b.Len() > 0 {
			b.WriteString(sep)
		}
		b.WriteString(s[i : i+3])
	}
	return b.String()
}

// decimal is a plain machine-readable amount for CSV: "-1234.56".
func decimal(cents int64, cur string) string {
	units := fx.MinorUnits(cur)
	if units == 0 {
		return strconv.FormatInt(cents, 10)
	}
	return strconv.FormatFloat(float64(cents)/math.Pow10(units), 'f', units, 64)
}

// ---------- CSV ----------

// UTF-8 BOM: Excel otherwise opens Cyrillic text as mojibake.
const bom = "\uFEFF"

func MonthCSV(m Month) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString(bom)
	w := csv.NewWriter(&buf)
	_ = w.Write([]string{"Date", "Description", "Merchant", "Category", "Amount", "Currency", "Amount (" + m.Currency + ")"})
	for _, t := range m.Transactions {
		_ = w.Write([]string{t.Date.Format(time.DateOnly), t.Description, t.Merchant, t.Category,
			decimal(t.AmountCents, t.Currency), t.Currency, decimal(t.HomeCents, m.Currency)})
	}
	w.Flush()
	return buf.Bytes(), w.Error()
}

func PortfolioCSV(p Portfolio) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString(bom)
	w := csv.NewWriter(&buf)
	_ = w.Write([]string{"Type", "Name", "Kind", "Symbol", "Quantity", "Price", "Currency", "Invested", "Value",
		"Value (" + p.Currency + ")", "Gain %", "Rate %", "Matures"})
	for _, h := range p.Holdings {
		mat := ""
		if h.MaturityAt != nil {
			mat = h.MaturityAt.Format(time.DateOnly)
		}
		qty, price, rate := "", "", ""
		if h.Symbol != "" {
			qty = strconv.FormatFloat(h.Quantity, 'f', -1, 64)
			price = strconv.FormatFloat(h.Price, 'f', -1, 64)
		} else {
			rate = strconv.FormatFloat(h.RatePercent, 'f', -1, 64)
		}
		_ = w.Write([]string{"investment", h.Name, h.Kind, h.Symbol, qty, price, h.Currency,
			decimal(h.InvestedCents, h.Currency), decimal(h.ValueCents, h.Currency), decimal(h.ValueHomeCents, p.Currency),
			strconv.FormatFloat(round2(h.GainPercent), 'f', 2, 64), rate, mat})
	}
	for _, a := range p.Accounts {
		bal, home := a.BalanceCents, a.BalanceHomeCents
		if a.Liability {
			bal, home = -bal, -home
		}
		_ = w.Write([]string{"account", a.Name, a.Kind, "", "", "", a.Currency, "", decimal(bal, a.Currency),
			decimal(home, p.Currency), "", "", ""})
	}
	w.Flush()
	return buf.Bytes(), w.Error()
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }

func minor(cur string) int { return fx.MinorUnits(cur) }
