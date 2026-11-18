package split

import (
	"math/bits"
	"sort"

	"github.com/Zifeldev/Billsplit/internal/domain"
)

type MemberShare struct {
	UserID     string
	ItemsCents int64
	TaxCents   int64
	TipCents   int64
	TotalCents int64
}

type SplitResult struct {
	Shares           []MemberShare // sorted by user ID
	UnclaimedCents   int64
	UnclaimedItemIDs []string
}

// ComputeSplit divides every item between its claimants proportionally to
// shares, then spreads tax and tip proportionally to each member's items
// amount. All math is integer; leftover cents go by the largest-remainder
// method so the parts always add up exactly to the whole.
func ComputeSplit(r *domain.Receipt) SplitResult {
	items := map[string]int64{}
	var res SplitResult

	for _, it := range r.Items {
		if len(it.Claims) == 0 {
			res.UnclaimedCents += it.PriceCents
			res.UnclaimedItemIDs = append(res.UnclaimedItemIDs, it.ID)
			continue
		}
		weights := make(map[string]int64, len(it.Claims))
		for _, c := range it.Claims {
			weights[c.UserID] = int64(c.Shares)
		}
		for uid, v := range allocate(it.PriceCents, weights) {
			items[uid] += v
		}
	}

	tax := allocate(r.TaxCents, items)
	tip := allocate(r.TipCents, items)
	for uid, v := range items {
		s := MemberShare{UserID: uid, ItemsCents: v, TaxCents: tax[uid], TipCents: tip[uid]}
		s.TotalCents = s.ItemsCents + s.TaxCents + s.TipCents
		res.Shares = append(res.Shares, s)
	}
	sort.Slice(res.Shares, func(i, j int) bool { return res.Shares[i].UserID < res.Shares[j].UserID })
	return res
}

// allocate splits total by weights; sum(result) == total exactly. Weights must
// be >= 0. The products total*weight are computed in 128 bits, so large
// weights (e.g. exact minor-unit amounts of an expense) can't overflow.
func allocate(total int64, weights map[string]int64) map[string]int64 {
	out := make(map[string]int64, len(weights))
	var sumW uint64
	for _, w := range weights {
		sumW += uint64(max(w, 0))
	}
	if sumW == 0 || total == 0 {
		return out
	}
	neg := total < 0
	t := uint64(total)
	if neg {
		t = uint64(-total)
	}

	type rem struct {
		uid string
		r   uint64
	}
	rems := make([]rem, 0, len(weights))
	var given uint64
	for uid, w := range weights {
		hi, lo := bits.Mul64(t, uint64(max(w, 0)))
		q, r := bits.Div64(hi, lo, sumW) // w <= sumW, so q <= t: no overflow
		out[uid] = int64(q)
		given += q
		rems = append(rems, rem{uid, r})
	}
	sort.Slice(rems, func(i, j int) bool {
		if rems[i].r != rems[j].r {
			return rems[i].r > rems[j].r
		}
		return rems[i].uid < rems[j].uid // deterministic
	})
	for i := uint64(0); i < t-given; i++ {
		out[rems[i%uint64(len(rems))].uid]++
	}
	if neg {
		for uid, v := range out {
			out[uid] = -v
		}
	}
	return out
}

type Transfer struct {
	From, To string
	Amount   int64
}

// SimplifyDebts turns net balances into a short list of transfers
// (greedy: biggest debtor pays biggest creditor). At most n-1 transfers.
func SimplifyDebts(net map[string]int64) []Transfer {
	type bal struct {
		uid string
		amt int64
	}
	var debtors, creditors []bal
	for uid, v := range net {
		switch {
		case v < 0:
			debtors = append(debtors, bal{uid, -v})
		case v > 0:
			creditors = append(creditors, bal{uid, v})
		}
	}
	byAmount := func(s []bal) func(i, j int) bool {
		return func(i, j int) bool {
			if s[i].amt != s[j].amt {
				return s[i].amt > s[j].amt
			}
			return s[i].uid < s[j].uid
		}
	}
	sort.Slice(debtors, byAmount(debtors))
	sort.Slice(creditors, byAmount(creditors))

	var out []Transfer
	i, j := 0, 0
	for i < len(debtors) && j < len(creditors) {
		amt := min(debtors[i].amt, creditors[j].amt)
		out = append(out, Transfer{From: debtors[i].uid, To: creditors[j].uid, Amount: amt})
		debtors[i].amt -= amt
		creditors[j].amt -= amt
		if debtors[i].amt == 0 {
			i++
		}
		if creditors[j].amt == 0 {
			j++
		}
	}
	return out
}
