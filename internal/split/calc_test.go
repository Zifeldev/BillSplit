package split

import (
	"testing"

	"github.com/Zifeldev/Billsplit/internal/domain"
)

func TestComputeSplit(t *testing.T) {
	r := &domain.Receipt{
		TaxCents: 100,
		TipCents: 0,
		Items: []domain.ReceiptItem{
			{ID: "milk", PriceCents: 300, Claims: []domain.ItemClaim{{UserID: "a", Shares: 1}}},
			// pizza shared 2:1
			{ID: "pizza", PriceCents: 1000, Claims: []domain.ItemClaim{{UserID: "a", Shares: 2}, {UserID: "b", Shares: 1}}},
			{ID: "gum", PriceCents: 99},
		},
	}
	s := ComputeSplit(r)

	if s.UnclaimedCents != 99 || len(s.UnclaimedItemIDs) != 1 {
		t.Errorf("unclaimed = %d %v", s.UnclaimedCents, s.UnclaimedItemIDs)
	}
	var items, tax int64
	for _, sh := range s.Shares {
		items += sh.ItemsCents
		tax += sh.TaxCents
	}
	if items != 1300 || tax != 100 {
		t.Errorf("sum items=%d tax=%d", items, tax)
	}
	// a: 300 + 667 = 967, b: 333
	if s.Shares[0].UserID != "a" || s.Shares[0].ItemsCents != 967 || s.Shares[1].ItemsCents != 333 {
		t.Errorf("shares = %+v", s.Shares)
	}
}

func TestAllocateExact(t *testing.T) {
	got := allocate(100, map[string]int64{"a": 1, "b": 1, "c": 1})
	if got["a"]+got["b"]+got["c"] != 100 || got["a"] != 34 {
		t.Errorf("got %v", got)
	}
}

func TestSimplifyDebts(t *testing.T) {
	// a paid for everyone: b owes 30, c owes 20.
	tr := SimplifyDebts(map[string]int64{"a": 50, "b": -30, "c": -20})
	if len(tr) != 2 {
		t.Fatalf("transfers = %+v", tr)
	}
	if tr[0] != (Transfer{From: "b", To: "a", Amount: 30}) || tr[1] != (Transfer{From: "c", To: "a", Amount: 20}) {
		t.Errorf("transfers = %+v", tr)
	}
}
