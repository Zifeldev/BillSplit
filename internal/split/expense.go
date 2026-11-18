package split

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/Zifeldev/Billsplit/internal/domain"
	"github.com/Zifeldev/Billsplit/internal/platform/fx"
)

// SplitType is how an expense is divided (values match pb.SplitType).
type SplitType int32

const (
	SplitEqual   SplitType = 1 // value ignored
	SplitPercent SplitType = 2 // value = basis points, sum 10000
	SplitExact   SplitType = 3 // value = minor units, sum == amount
	SplitShares  SplitType = 4 // value = weight
)

const (
	maxExpenseCents        = 1_000_000_000_000 // sanity cap: 10^12 minor units
	maxExpenseParticipants = 200
	maxShareWeight         = 1_000_000
)

type Participant struct {
	UserID string
	Value  int64
}

// expenseWeights validates participants for the split type and returns the
// weights whose largest-remainder allocation (allocate) is the split:
// EQUAL -> 1 each, PERCENT -> basis points, SHARES -> weights, EXACT -> the
// amounts themselves (allocate reproduces them exactly since sum == amount).
func expenseWeights(amount int64, t SplitType, ps []Participant) (map[string]int64, error) {
	if amount <= 0 || amount > maxExpenseCents {
		return nil, fmt.Errorf("%w: amount must be positive", domain.ErrInvalidArgument)
	}
	if len(ps) == 0 {
		return nil, fmt.Errorf("%w: participants are required", domain.ErrInvalidArgument)
	}
	if len(ps) > maxExpenseParticipants {
		return nil, fmt.Errorf("%w: too many participants", domain.ErrInvalidArgument)
	}
	w := make(map[string]int64, len(ps))
	var sum int64
	for _, p := range ps {
		if p.UserID == "" {
			return nil, fmt.Errorf("%w: participant user_id is required", domain.ErrInvalidArgument)
		}
		if _, dup := w[p.UserID]; dup {
			return nil, fmt.Errorf("%w: duplicate participant %s", domain.ErrInvalidArgument, p.UserID)
		}
		v := p.Value
		switch t {
		case SplitEqual:
			v = 1
		case SplitPercent:
			if v <= 0 || v > 10000 {
				return nil, fmt.Errorf("%w: percent must be 1..10000 basis points", domain.ErrInvalidArgument)
			}
		case SplitExact:
			if v <= 0 || v > amount {
				return nil, fmt.Errorf("%w: exact amounts must be positive and not exceed the total", domain.ErrInvalidArgument)
			}
		case SplitShares:
			if v <= 0 || v > maxShareWeight {
				return nil, fmt.Errorf("%w: shares must be 1..%d", domain.ErrInvalidArgument, maxShareWeight)
			}
		default:
			return nil, fmt.Errorf("%w: unknown split type", domain.ErrInvalidArgument)
		}
		w[p.UserID] = v
		sum += v
	}
	switch {
	case t == SplitPercent && sum != 10000:
		return nil, fmt.Errorf("%w: percents must add up to 100%% (10000 basis points), got %d", domain.ErrInvalidArgument, sum)
	case t == SplitExact && sum != amount:
		return nil, fmt.Errorf("%w: exact amounts add up to %d, expected %d", domain.ErrInvalidArgument, sum, amount)
	}
	return w, nil
}

// expenseItems turns an expense into receipt items whose claims reproduce
// allocate(amount, weights) exactly through ComputeSplit. Normally it is one
// item claimed by everybody (weights reduced by their GCD to fit claim
// shares); if the weights still don't fit int32 (huge exact amounts), each
// participant gets their own line with their exact part.
func expenseItems(name string, amount int64, weights map[string]int64) []domain.ReceiptItem {
	uids := make([]string, 0, len(weights))
	var g int64
	for uid, w := range weights {
		uids = append(uids, uid)
		g = gcd(g, w)
	}
	sort.Strings(uids)
	fits := true
	for _, uid := range uids {
		if weights[uid]/g > math.MaxInt32 {
			fits = false
			break
		}
	}
	if fits {
		it := domain.ReceiptItem{Name: name, Quantity: 1, PriceCents: amount}
		for _, uid := range uids {
			it.Claims = append(it.Claims, domain.ItemClaim{UserID: uid, Shares: int32(weights[uid] / g)})
		}
		return []domain.ReceiptItem{it}
	}
	parts := allocate(amount, weights)
	items := make([]domain.ReceiptItem, 0, len(uids))
	for _, uid := range uids {
		if parts[uid] > 0 {
			items = append(items, domain.ReceiptItem{Name: name, Quantity: 1, PriceCents: parts[uid],
				Claims: []domain.ItemClaim{{UserID: uid, Shares: 1}}})
		}
	}
	return items
}

func gcd(a, b int64) int64 {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

var currencySymbols = map[string]string{
	"USD": "$", "EUR": "€", "GBP": "£", "JPY": "¥", "CNY": "¥", "KRW": "₩", "INR": "₹",
	"CAD": "CA$", "AUD": "A$", "NZD": "NZ$", "HKD": "HK$", "MXN": "MX$", "BRL": "R$",
	"ILS": "₪", "TRY": "₺", "PHP": "₱", "THB": "฿", "PLN": "zł ",
}

// formatMoney renders minor units with the currency's real number of decimals:
// formatMoney(884, "USD") = "$8.84", formatMoney(1500, "JPY") = "¥1,500",
// formatMoney(1250, "CHF") = "CHF 12.50".
func formatMoney(cents int64, currency string) string {
	sign := ""
	if cents < 0 {
		sign, cents = "-", -cents
	}
	d := fx.MinorUnits(currency)
	pow := int64(math.Pow10(d))
	whole := fmt.Sprint(cents / pow)
	var b strings.Builder
	for i, c := range whole {
		if i > 0 && (len(whole)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	num := b.String()
	if d > 0 {
		num = fmt.Sprintf("%s.%0*d", num, d, cents%pow)
	}
	sym, ok := currencySymbols[currency]
	if !ok {
		sym = currency + " "
	}
	return sign + sym + num
}
