package statement

// Bank CenterCredit (BCC, bcc.kz) card account statements, English and Russian.
//
// After a header block (bank requisites, owner, "Account currency KZT" /
// "Валюта банковского счета KZT", period totals) comes the table
//
//	Operation date | Reflection date on an account | Operation description |
//	Amount in operation currency | Amount in KZT | Fee, KZT | Cashback | Bank exchange rate
//
// (Russian: Дата операции | Дата отражения на счете | Описание операции | Сумма в
// валюте операции | Сумма в KZT | Комиссия, KZT | Кешбэк, KZT | Курс конвертации банка).
// The PDF text layer flows each row band cell by cell with arbitrary wraps, even
// in the middle of a number ("-50 008." + "00 KZT"), and a description may
// continue after the amounts ("Transfer to 3 500.00 KZT -3 500.00 0.00 KZT 0.00KZT"
// + "Name A. KZT"). So a row is everything from a "date date" line to the next
// one; its text is glued back together and the amounts are taken by position:
// operation amount, account amount, fee, cashback and an optional unsuffixed rate.
//
// Semantics (they reconcile with the "Incomings/Expenses for the period" totals
// of the statement header):
//   - amount = signed "Amount in <account currency>" minus |fee|: the fee is part
//     of the expense (an annual service fee is a row with a zero amount and a fee);
//   - cashback is ignored: it is credited later as its own operation;
//   - the row currency is the account currency from the header;
//   - transfers, replenishments (top-ups) and cash withdrawals are hinted as
//     "transfer"; purchases are left to the category rules.

import (
	"errors"
	"regexp"
	"strings"
	"time"
)

var (
	bccRowRe = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}|\d{2}\.\d{2}\.\d{4})\s+(\d{4}-\d{2}-\d{2}|\d{2}\.\d{2}\.\d{4})(?:\s+(.*))?$`)
	// Amounts of the table: space thousands, dot and exactly two decimals.
	bccAmountRe = regexp.MustCompile(`[-+]?\d{1,3}(?: \d{3})*\.\d{2}`)
	bccCodeRe   = regexp.MustCompile(`\b[A-Z]{3}\b`)
	bccSplitNum = regexp.MustCompile(`\d\.\d?$`)
)

// Lines that start the signature/QR footer: skipped up to the next row.
var bccFooters = []string{"vice president", "вице-президент", "the qr code", "qr-код", "qr код",
	"retail business division", "блок розничного бизнеса"}

// Operation names: the description starts with one of them.
var bccTransferOps = []string{"transfer", "перевод", "пополнение", "replenishment", "top-up", "top up",
	"cash withdrawal", "снятие наличных", "выдача наличных"}

var bccPurchaseOps = []string{"purchase", "покупка", "оплата", "payment"}

func bccMarker(pages [][]string) int {
	return markerPos(pages, func(ll string) bool {
		return containsAny(ll, "bank centercredit", "банк центркредит", "bcc.kz", "kcjbkzkx")
	})
}

// markerPos returns the global index of the first line (lower-cased, spaces
// collapsed) matching, or -1. Detection picks the layout whose marker comes
// first, so a bank named in a transaction description doesn't win over the
// statement's own header.
func markerPos(pages [][]string, match func(ll string) bool) int {
	n := 0
	for _, page := range pages {
		for _, l := range page {
			if match(strings.ToLower(strings.Join(strings.Fields(l), " "))) {
				return n
			}
			n++
		}
	}
	return -1
}

func containsAny(s string, subs ...string) bool {
	for _, x := range subs {
		if strings.Contains(s, x) {
			return true
		}
	}
	return false
}

type bccEntry struct {
	date  time.Time
	lines []string // raw lines of the row band, the first one without the dates
}

// ParseBCCPages parses the PDF text layer of a Bank CenterCredit statement.
func ParseBCCPages(pages [][]string) ([]Row, error) {
	currency := bccAccountCurrency(pages)
	var (
		entries  []*bccEntry
		cur      *bccEntry
		skipping bool
	)
	for _, page := range pages {
		for _, raw := range page {
			raw = strings.TrimRight(raw, "\r\n")
			line := strings.TrimSpace(raw)
			if line == "" {
				continue
			}
			if m := bccRowRe.FindStringSubmatch(line); m != nil {
				if d, ok := bccDate(m[1]); ok {
					rest := m[3]
					if hasSoftWrap(raw) {
						rest += " "
					}
					cur = &bccEntry{date: d, lines: []string{rest}}
					entries = append(entries, cur)
					skipping = false
					continue
				}
			}
			ll := strings.ToLower(line)
			if pageNumRe.MatchString(ll) {
				continue
			}
			if hasAnyPrefix(ll, bccFooters) || isBCCPageNoise(ll) || isBCCTableHeader(ll) {
				skipping = true
				continue
			}
			if skipping || cur == nil {
				continue
			}
			cur.lines = append(cur.lines, raw)
		}
	}
	rows := make([]Row, 0, len(entries))
	seen := map[string]int{}
	for _, e := range entries {
		row, ok := bccRow(e, currency)
		if !ok {
			continue
		}
		// Keyed by date, amount and currency only, so the English and the Russian
		// version of the same statement deduplicate against each other.
		base := dedupKey(row.Date, row.AmountCents, "bcc|"+currency, 0)
		row.DedupKey = dedupKey(row.Date, row.AmountCents, "bcc|"+currency, seen[base])
		seen[base]++
		rows = append(rows, row)
	}
	if len(rows) == 0 {
		return nil, errors.New("bcc: no transactions found")
	}
	return rows, nil
}

func hasAnyPrefix(s string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

// A bank header repeated on a later page.
func isBCCPageNoise(ll string) bool {
	return strings.HasPrefix(ll, "bank centercredit jsc") || strings.Contains(ll, "«банк центркредит»")
}

// The first line of a (repeated) table header.
func isBCCTableHeader(ll string) bool {
	return ll == "operation" || ll == "дата" || strings.HasPrefix(ll, "operation date") || strings.HasPrefix(ll, "дата операции")
}

var pageNumRe = regexp.MustCompile(`^(?:page|стр\.?|страница)?\s*\d{1,3}\s*(?:(?:of|из|/)\s*\d{1,3})?$`)

func bccDate(s string) (time.Time, bool) {
	for _, l := range []string{"2006-01-02", "02.01.2006"} {
		if t, err := time.Parse(l, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// bccAccountCurrency reads "Account currency KZT" / "Валюта банковского счета" + "KZT".
func bccAccountCurrency(pages [][]string) string {
	for _, page := range pages {
		for i, l := range page {
			ll := strings.ToLower(l)
			if !strings.Contains(ll, "account currency") && !strings.Contains(ll, "валюта банковского") && !strings.Contains(ll, "валюта счета") {
				continue
			}
			for j := i; j < len(page) && j <= i+3; j++ {
				for _, c := range bccCodeRe.FindAllString(page[j], -1) {
					if isoCodes[c] {
						return c
					}
				}
			}
		}
	}
	return "KZT"
}

// bccJoin glues the lines of a row band: a number split by a wrap ("008." +
// "00", "000.3" + "2") is rejoined, everything else is joined with a space.
func bccJoin(lines []string) string {
	var b strings.Builder
	for i, raw := range lines {
		t := strings.TrimSpace(raw)
		if t == "" {
			continue
		}
		if i > 0 && b.Len() > 0 {
			prev := lines[i-1]
			if !hasSoftWrap(prev) && bccSplitNum.MatchString(prev) && t[0] >= '0' && t[0] <= '9' {
				b.WriteString(t)
				continue
			}
			b.WriteByte(' ')
		}
		b.WriteString(t)
	}
	return b.String()
}

func bccRow(e *bccEntry, currency string) (Row, bool) {
	text := bccJoin(e.lines)
	locs := bccAmountRe.FindAllStringIndex(text, -1)
	// Drop matches glued to other digits ("3 500.001").
	var amts [][]int
	for _, l := range locs {
		if l[0] > 0 && (text[l[0]-1] >= '0' && text[l[0]-1] <= '9' || text[l[0]-1] == '.') {
			continue
		}
		if l[1] < len(text) && text[l[1]] >= '0' && text[l[1]] <= '9' {
			continue
		}
		amts = append(amts, l)
	}
	if len(amts) < 2 {
		return Row{}, false
	}
	// A trailing rate with two decimals has no currency after it.
	if len(amts) >= 5 && !bccCodeRe.MatchString(text[amts[len(amts)-1][1]:]) {
		amts = amts[:len(amts)-1]
	}
	val := func(l []int) int64 {
		v, err := parseAmount(strings.ReplaceAll(text[l[0]:l[1]], " ", ""))
		if err != nil {
			return 0
		}
		return v
	}
	var account, fee int64
	if len(amts) >= 4 {
		cols := amts[len(amts)-4:]
		account, fee = val(cols[1]), val(cols[2])
		amts = amts[:len(amts)-4+1] // the description ends before the operation amount
	} else {
		account = val(amts[1])
		if len(amts) == 3 {
			fee = val(amts[2])
		}
		amts = amts[:1]
	}
	amount := account - abs(fee)
	if amount == 0 {
		return Row{}, false
	}

	// Description: the text before the operation amount plus any words that
	// wrapped past the amounts, without currency codes and numbers.
	first := amts[len(amts)-1][0]
	desc := strings.TrimSpace(text[:first])
	var tail []string
	for _, w := range strings.Fields(bccAmountRe.ReplaceAllString(text[first:], " ")) {
		if isoCodes[w] || isNumberish(w) {
			continue
		}
		tail = append(tail, w)
	}
	if len(tail) > 0 {
		desc = strings.TrimSpace(desc + " " + strings.Join(tail, " "))
	}
	desc = strings.Join(strings.Fields(desc), " ")
	return Row{Date: e.date, AmountCents: amount, Description: desc, Currency: currency,
		Merchant: bccMerchant(desc), Category: bccCategory(desc)}, true
}

func isNumberish(w string) bool {
	return strings.Trim(w, "0123456789.,-+") == ""
}

func bccCategory(desc string) string {
	d := strings.ToLower(desc)
	if hasAnyPrefix(d, bccTransferOps) {
		return "transfer"
	}
	return freedomCategory(desc)
}

func bccMerchant(desc string) string {
	d := strings.ToLower(desc)
	rest := desc
	for _, op := range append(append([]string{"transfer to", "перевод на", "перевод"}, bccPurchaseOps...), bccTransferOps...) {
		if strings.HasPrefix(d, op+" ") {
			rest = desc[len(op)+1:]
			break
		}
	}
	return NormalizeMerchant(rest)
}
