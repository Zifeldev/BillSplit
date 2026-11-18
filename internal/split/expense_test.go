package split

import (
	"errors"
	"math"
	"testing"

	"github.com/Zifeldev/Billsplit/internal/domain"
)

// splitExpense runs the full expense path: weights -> items/claims -> ComputeSplit.
func splitExpense(t *testing.T, amount int64, st SplitType, ps []Participant) map[string]int64 {
	t.Helper()
	w, err := expenseWeights(amount, st, ps)
	if err != nil {
		t.Fatalf("weights: %v", err)
	}
	r := &domain.Receipt{SubtotalCents: amount, TotalCents: amount, Items: expenseItems("x", amount, w)}
	for i := range r.Items {
		r.Items[i].ID = string(rune('a' + i))
	}
	res := ComputeSplit(r)
	if res.UnclaimedCents != 0 {
		t.Fatalf("unclaimed %d", res.UnclaimedCents)
	}
	out := map[string]int64{}
	var sum int64
	for _, sh := range res.Shares {
		out[sh.UserID] = sh.TotalCents
		sum += sh.TotalCents
	}
	if sum != amount {
		t.Fatalf("sum %d != amount %d (%v)", sum, amount, out)
	}
	return out
}

func eq(t *testing.T, got, want map[string]int64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("got %v want %v", got, want)
		}
	}
}

func TestExpenseEqualRounding(t *testing.T) {
	// $100 / 3: one person absorbs the extra cent, deterministically.
	got := splitExpense(t, 10000, SplitEqual, []Participant{{UserID: "a"}, {UserID: "b"}, {UserID: "c"}})
	eq(t, got, map[string]int64{"a": 3334, "b": 3333, "c": 3333})

	// Values are ignored for EQUAL.
	got = splitExpense(t, 1001, SplitEqual, []Participant{{UserID: "a", Value: 99}, {UserID: "b", Value: 1}})
	eq(t, got, map[string]int64{"a": 501, "b": 500})

	// Zero-decimal currency: 1000 JPY / 3.
	got = splitExpense(t, 1000, SplitEqual, []Participant{{UserID: "a"}, {UserID: "b"}, {UserID: "c"}})
	eq(t, got, map[string]int64{"a": 334, "b": 333, "c": 333})
}

func TestExpensePercent(t *testing.T) {
	got := splitExpense(t, 10000, SplitPercent, []Participant{{"a", 5000}, {"b", 2500}, {"c", 2500}})
	eq(t, got, map[string]int64{"a": 5000, "b": 2500, "c": 2500})

	// 33.33/33.33/33.34% of $0.99 rounds by largest remainder.
	got = splitExpense(t, 99, SplitPercent, []Participant{{"a", 3333}, {"b", 3333}, {"c", 3334}})
	eq(t, got, map[string]int64{"a": 33, "b": 33, "c": 33})

	if _, err := expenseWeights(100, SplitPercent, []Participant{{"a", 5000}, {"b", 4999}}); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("percent sum != 10000 must fail, got %v", err)
	}
}

func TestExpenseExact(t *testing.T) {
	got := splitExpense(t, 10000, SplitExact, []Participant{{"a", 3333}, {"b", 3333}, {"c", 3334}})
	eq(t, got, map[string]int64{"a": 3333, "b": 3333, "c": 3334})

	// GCD reduction keeps claim shares small: 2000/6000 -> 1:3.
	w, _ := expenseWeights(8000, SplitExact, []Participant{{"a", 2000}, {"b", 6000}})
	items := expenseItems("x", 8000, w)
	if len(items) != 1 || items[0].Claims[0].Shares != 1 || items[0].Claims[1].Shares != 3 {
		t.Fatalf("items = %+v", items)
	}

	// Amounts that don't fit int32 claim shares fall back to one line per participant, still exact.
	big := int64(math.MaxInt32) * 3
	got = splitExpense(t, big+1, SplitExact, []Participant{{"a", big - 7}, {"b", 8}})
	eq(t, got, map[string]int64{"a": big - 7, "b": 8})

	if _, err := expenseWeights(100, SplitExact, []Participant{{"a", 50}, {"b", 49}}); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("exact sum != amount must fail, got %v", err)
	}
}

func TestExpenseShares(t *testing.T) {
	got := splitExpense(t, 1000, SplitShares, []Participant{{"a", 2}, {"b", 1}})
	eq(t, got, map[string]int64{"a": 667, "b": 333})

	if _, err := expenseWeights(100, SplitShares, []Participant{{"a", 0}}); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("zero weight must fail, got %v", err)
	}
}

func TestExpenseValidation(t *testing.T) {
	cases := []struct {
		name   string
		amount int64
		st     SplitType
		ps     []Participant
	}{
		{"zero amount", 0, SplitEqual, []Participant{{UserID: "a"}}},
		{"no participants", 100, SplitShares, nil},
		{"duplicate", 100, SplitEqual, []Participant{{UserID: "a"}, {UserID: "a"}}},
		{"unknown type", 100, SplitType(9), []Participant{{UserID: "a"}}},
		{"exact over total", 100, SplitExact, []Participant{{"a", 101}}},
	}
	for _, c := range cases {
		if _, err := expenseWeights(c.amount, c.st, c.ps); !errors.Is(err, domain.ErrInvalidArgument) {
			t.Errorf("%s: want invalid argument, got %v", c.name, err)
		}
	}
}

func TestAllocateLargeNoOverflow(t *testing.T) {
	total := int64(1_000_000_000_000)
	got := allocate(total, map[string]int64{"a": total / 3, "b": total - total/3})
	if got["a"]+got["b"] != total || got["a"] != total/3 {
		t.Fatalf("got %v", got)
	}
	neg := allocate(-100, map[string]int64{"a": 1, "b": 1, "c": 1})
	if neg["a"]+neg["b"]+neg["c"] != -100 {
		t.Fatalf("negative: %v", neg)
	}
}

func TestFormatMoney(t *testing.T) {
	cases := map[string]struct {
		cents int64
		cur   string
	}{
		"$8.84":         {884, "USD"},
		"$1,234,567.05": {123456705, "USD"},
		"¥1,500":        {1500, "JPY"},
		"CHF 12.50":     {1250, "CHF"},
		"-€0.07":        {-7, "EUR"},
		"₩0":            {0, "KRW"},
	}
	for want, c := range cases {
		if got := formatMoney(c.cents, c.cur); got != want {
			t.Errorf("formatMoney(%d, %s) = %q, want %q", c.cents, c.cur, got, want)
		}
	}
}
