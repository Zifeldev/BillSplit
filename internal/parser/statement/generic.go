package statement

// Generic fallback for statements of unknown banks (PDF text layer or CSV turned
// into text lines). It is line based:
//
//   - A transaction row starts with a date: dd.mm.yyyy, dd.mm.yy, yyyy-mm-dd,
//     dd/mm/yyyy or mm/dd/yyyy (the order is decided per document: a part > 12
//     settles it; otherwise day-first, unless the document uses US "1,234.56"
//     amounts), "27 окт. 2025", "27 Oct 2025", "Oct 27, 2025". An optional time
//     and a second (posting) date after it are skipped, as is a leading row number.
//   - Amounts: "1 234,56", "1,234.56", "1.234,56", "1234.5", signs + - − –,
//     parentheses and a trailing minus as negative, currency symbols/codes
//     (₸ KZT $ USD € EUR ₽ RUB ...) before or after, "Dr"/"Cr" markers. A bare
//     integer is not an amount (store numbers, card digits) unless it has a sign
//     or a currency.
//   - When the date line has no amount, the amounts of the next lines are used
//     (cells wrapped below the date). Lines without a date continue the
//     description of the row above (at most 4, never across a page break).
//   - Skipped: lines with balance/total/summary words (остаток, баланс, сальдо,
//     итого, обороты, total, balance, opening/closing ...), page numbers, and
//     footer lines (signatures, web links, "statement generated" ...).
//
// Choosing the transaction amount among several on a row:
//   - When a table header was seen, its amount columns are ordered by position
//     ("Debit | Credit | Balance", "Сумма | Остаток", "Приход | Расход"...). The
//     balance column is never the transaction amount: with N columns and N amounts
//     the one under the balance is dropped, with fewer amounts the last one is
//     taken for the balance when the header ends with it. Separate debit and
//     credit columns give credit − debit when both are printed.
//   - Otherwise (no header, or a single amount column): a signed amount wins, else
//     the first amount; a trailing extra amount is taken for the running balance
//     only if the header names one.
//
// The sign of an unsigned amount is decided, in order, by the running balance
// (consecutive balances differ by exactly the amount, in either row order), by
// the document convention (only "-" amounts printed → unsigned ones are income;
// only "+" → unsigned ones are expenses), then by income keywords
// (пополнение, зачисление, refund, salary...); otherwise it is an expense.

import (
	"regexp"
	"sort"
	"strings"
	"time"
)

// ErrUnrecognized is returned when no known layout matches and the generic
// heuristics find no transactions. Its message is shown to the user as is.
var ErrUnrecognized = &unrecognizedError{}

type unrecognizedError struct{}

func (*unrecognizedError) Error() string {
	return "Не удалось распознать формат выписки: пришлите пример файла / " +
		"Could not recognize the statement format: please send us a sample file"
}

func (*unrecognizedError) Is(target error) bool { return target == ErrUnsupported }

var (
	skipWordsRe = regexp.MustCompile(`(?i)(остат|баланс|сальдо|итог|оборот|входящ\S* остат|исходящ\S* остат|` +
		`всего|\btotal|\bbalance|opening|closing|brought forward|carried forward|\bsubtotal|summary)`)
	footerRe = regexp.MustCompile(`(?i)(www\.|https?://|@|подпис|signature|печат|\bseal\b|сформирован|generated|` +
		`страниц|\bpage\b|телефон|president|президент|director|директор|manager|менеджер|уполномоченн|authori[sz]ed|\bphone\b|(?:^|\s)тел\.|не требует|does not require|qr)`)
	incomeWordsRe = regexp.MustCompile(`(?i)(пополнени|зачислени|поступлени|возврат|refund|deposit|salary|зарплат|` +
		`заработн|cashback|кешб[еэ]к|replenish|top-?up|incoming|входящ|interest|вознагражден|начисление процент)`)
	ownTransferRe = regexp.MustCompile(`(?i)(между своими|own account|own accounts|собственн\S* сч|конвертац|обмен валют|currency exchange)`)
	rowIndexRe    = regexp.MustCompile(`^\d{1,4}[.)]?\s+`)
	dateKeyRe     = regexp.MustCompile(`(?i)(?:^|[^\pL])(?:дата|date|күні)(?:$|[^\pL])`)
	usAmountRe    = regexp.MustCompile(`\d,\d{3}\.\d{2}\b`)
	slashDatesRe  = regexp.MustCompile(`\b(\d{1,2})/(\d{1,2})/(\d{2,4})\b`)
)

type colRole int

const (
	roleAmount colRole = iota
	roleDebit
	roleCredit
	roleBalance
	roleFee
)

var roleKeywords = []struct {
	role colRole
	re   *regexp.Regexp
}{
	{roleBalance, regexp.MustCompile(`(?i)(balance|остаток|баланс|сальдо)`)},
	{roleDebit, regexp.MustCompile(`(?i)(debit|withdrawal|paid out|money out|outflow|расход|списани|дебет)`)},
	{roleCredit, regexp.MustCompile(`(?i)(credit|deposit|paid in|money in|inflow|приход|поступлени|зачислени|кредит)`)},
	{roleFee, regexp.MustCompile(`(?i)(\bfee\b|комисси)`)},
	{roleAmount, regexp.MustCompile(`(?i)(amount|сумма|сомасы)`)},
}

// headerRoles returns the amount columns named by a table header line, in order,
// or nil if the line is not a header.
func headerRoles(line string) []colRole {
	if !dateKeyRe.MatchString(line) || len(findMoney(line)) > 0 {
		return nil
	}
	type hit struct {
		pos, end int
		role     colRole
	}
	var hits []hit
	taken := func(role colRole, a, b int) bool {
		margin := 0
		if role == roleAmount {
			margin = 16 // "Debit amount", "Сумма прихода": one column
		}
		for _, h := range hits {
			if a < h.end+margin && b > h.pos-margin {
				return true
			}
		}
		return false
	}
	for _, k := range roleKeywords {
		for _, loc := range k.re.FindAllStringIndex(line, -1) {
			if !taken(k.role, loc[0], loc[1]) {
				hits = append(hits, hit{loc[0], loc[1], k.role})
			}
		}
	}
	if len(hits) == 0 {
		return nil
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].pos < hits[j].pos })
	roles := make([]colRole, len(hits))
	for i, h := range hits {
		roles[i] = h.role
	}
	return roles
}

type genEntry struct {
	date    time.Time
	amounts []money
	desc    []string
	roles   []colRole
	balance *money
	amount  int64 // absolute
	sign    int
	cur     string
}

// ParseGenericPages parses statement text of an unknown bank with heuristics.
func ParseGenericPages(pages [][]string) ([]Row, error) {
	order := genericDateOrder(pages)
	var (
		entries []*genEntry
		cur     *genEntry
		roles   []colRole
		cont    int
	)
	for _, page := range pages {
		cur = nil
		for _, raw := range glueSplitNumbers(page) {
			line := strings.Join(strings.Fields(raw), " ")
			if line == "" {
				continue
			}
			e := genericRowStart(line, order)
			if e == nil {
				if r := headerRoles(line); r != nil {
					roles, cur = r, nil
					continue
				}
			}
			if e != nil {
				if skipWordsRe.MatchString(line) {
					cur = nil
					continue
				}
				e.roles = roles
				entries = append(entries, e)
				cur, cont = e, 0
				continue
			}
			if cur == nil {
				continue
			}
			if skipWordsRe.MatchString(line) || footerRe.MatchString(line) {
				cur = nil
				continue
			}
			if pageNumRe.MatchString(strings.ToLower(line)) {
				continue
			}
			text, ms := stripMoney(line)
			if len(ms) > 0 {
				// Cells wrapped under the date line may carry the (rest of the)
				// amounts; a line mixing text and amounts under a complete row is a
				// summary, not a description.
				if len(cur.amounts) > 0 && (text != "" || len(cur.amounts) >= 6) {
					cur = nil
					continue
				}
				cur.amounts = append(cur.amounts, ms...)
			}
			if text != "" && cont < 4 {
				cur.desc = append(cur.desc, text)
				cont++
			}
		}
	}

	var picked []*genEntry
	for _, e := range entries {
		if e.pick() {
			picked = append(picked, e)
		}
	}
	if len(picked) == 0 {
		return nil, ErrUnrecognized
	}
	resolveSigns(picked)

	rows := make([]Row, 0, len(picked))
	seen := map[string]int{}
	for _, e := range picked {
		amount := e.amount
		if e.sign < 0 {
			amount = -amount
		}
		if amount == 0 {
			continue
		}
		desc := strings.Join(strings.Fields(strings.Join(e.desc, " ")), " ")
		row := Row{Date: e.date, AmountCents: amount, Description: desc, Currency: e.cur,
			Merchant: NormalizeMerchant(desc)}
		if ownTransferRe.MatchString(desc) {
			row.Category = "transfer"
		}
		id := "generic|" + e.cur + "|" + strings.ToLower(desc)
		base := dedupKey(row.Date, amount, id, 0)
		row.DedupKey = dedupKey(row.Date, amount, id, seen[base])
		seen[base]++
		rows = append(rows, row)
	}
	if len(rows) == 0 {
		return nil, ErrUnrecognized
	}
	return rows, nil
}

// glueSplitNumbers rejoins a number wrapped in the middle ("-50 008." +
// "00 KZT", "2 000.3" + "2 KZT"): a line ending in a digit and a decimal mark
// (and at most one decimal) without a trailing space, followed by a line that
// starts with a digit.
func glueSplitNumbers(page []string) []string {
	out := make([]string, 0, len(page))
	for _, l := range page {
		l = strings.TrimRight(l, "\r\n")
		if n := len(out); n > 0 && !hasSoftWrap(out[n-1]) && bccSplitNum.MatchString(out[n-1]) &&
			l != "" && l[0] >= '0' && l[0] <= '9' && !startsWithDate(l) {
			out[n-1] += l
			continue
		}
		out = append(out, l)
	}
	return out
}

func startsWithDate(l string) bool {
	_, _, ok := leadingDate(strings.TrimSpace(l), dayFirst)
	return ok
}

// genericDateOrder decides between dd/mm and mm/dd for slash dates.
func genericDateOrder(pages [][]string) dateOrder {
	us := 0
	for _, page := range pages {
		for _, l := range page {
			for _, m := range slashDatesRe.FindAllStringSubmatch(l, -1) {
				a, b := atoiSafe(m[1]), atoiSafe(m[2])
				if a > 12 && b <= 12 {
					return dayFirst
				}
				if b > 12 && a <= 12 {
					return monthFirst
				}
			}
			if usAmountRe.MatchString(l) {
				us++
			}
		}
	}
	if us > 0 {
		return monthFirst
	}
	return dayFirst
}

func atoiSafe(s string) int {
	n := 0
	for _, c := range s {
		n = n*10 + int(c-'0')
	}
	return n
}

// genericRowStart parses a line that starts with a date.
func genericRowStart(line string, order dateOrder) *genEntry {
	s := line
	d, n, ok := leadingDate(s, order)
	if !ok {
		// "12 27.10.2025 ...": a row number before the date.
		if loc := rowIndexRe.FindStringIndex(s); loc != nil {
			if d, n, ok = leadingDate(s[loc[1]:], order); ok {
				n += loc[1]
			}
		}
		if !ok {
			return nil
		}
	}
	rest := strings.TrimSpace(s[n:])
	if _, n2, ok := leadingDate(rest, order); ok { // posting date
		rest = strings.TrimSpace(rest[n2:])
	}
	e := &genEntry{date: d}
	text, ms := stripMoney(rest)
	e.amounts = ms
	if text != "" {
		e.desc = []string{text}
	}
	return e
}

// stripMoney returns the line without dates and amounts, and the amounts.
func stripMoney(s string) (string, []money) {
	s = anyDateRe.ReplaceAllStringFunc(s, func(m string) string { return strings.Repeat(" ", len(m)) })
	ms := findMoney(s)
	var b strings.Builder
	last := 0
	for _, m := range ms {
		b.WriteString(s[last:m.start])
		b.WriteByte(' ')
		last = m.end
	}
	b.WriteString(s[last:])
	words := strings.Fields(b.String())
	out := words[:0]
	for _, w := range words {
		if isoCodes[w] || currencySymbols[strings.ToLower(w)] != "" || strings.Trim(w, "|;:-–—") == "" {
			continue
		}
		out = append(out, w)
	}
	return strings.Join(out, " "), ms
}

// pick chooses the transaction amount (and the balance) of a row.
func (e *genEntry) pick() bool {
	ams := e.amounts
	if len(ams) == 0 {
		return false
	}
	roles := e.roles
	balIdx := -1
	for i, r := range roles {
		if r == roleBalance {
			balIdx = i
		}
	}
	// Drop the running balance.
	if balIdx >= 0 && len(ams) >= 2 {
		var bi int
		switch {
		case len(ams) == len(roles):
			bi = balIdx
		case balIdx == len(roles)-1:
			bi = len(ams) - 1
		default:
			bi = -1
		}
		if bi >= 0 {
			b := ams[bi]
			e.balance = &b
			ams = append(append([]money(nil), ams[:bi]...), ams[bi+1:]...)
			roles = append(append([]colRole(nil), roles[:balIdx]...), roles[balIdx+1:]...)
		}
	}
	var debitIdx, creditIdx = -1, -1
	for i, r := range roles {
		switch r {
		case roleDebit:
			debitIdx = i
		case roleCredit:
			creditIdx = i
		}
	}
	var chosen money
	switch {
	case debitIdx >= 0 && creditIdx >= 0 && len(ams) == len(roles):
		deb, cred := ams[debitIdx], ams[creditIdx]
		net := cred.cents - deb.cents
		chosen = cred
		chosen.cents, chosen.sign = abs(net), 1
		if net < 0 {
			chosen.sign = -1
		}
		if chosen.currency == "" {
			chosen.currency = deb.currency
		}
	case len(ams) == 1 && len(roles) == 1 && (roles[0] == roleDebit || roles[0] == roleCredit):
		chosen = ams[0]
		if chosen.sign == 0 {
			chosen.sign = -1
			if roles[0] == roleCredit {
				chosen.sign = 1
			}
		}
	default:
		chosen = ams[0]
		for _, m := range ams {
			if m.sign != 0 {
				chosen = m
				break
			}
		}
	}
	e.amount, e.sign, e.cur = chosen.cents, chosen.sign, chosen.currency
	return e.amount != 0
}

// resolveSigns gives unsigned amounts a sign (see the package comment above).
func resolveSigns(es []*genEntry) {
	// 1. Running balance, trying both row orders.
	matches := func(asc bool) int {
		n := 0
		for i := 1; i < len(es); i++ {
			a, b := es[i-1], es[i]
			if a.balance == nil || b.balance == nil {
				continue
			}
			delta := b.balance.signed(1) - a.balance.signed(1)
			target := b
			if !asc {
				delta, target = -delta, a
			}
			if abs(delta) == target.amount {
				n++
			}
		}
		return n
	}
	asc, desc := matches(true), matches(false)
	if asc > 0 || desc > 0 {
		isAsc := asc >= desc
		for i := 1; i < len(es); i++ {
			a, b := es[i-1], es[i]
			if a.balance == nil || b.balance == nil {
				continue
			}
			delta := b.balance.signed(1) - a.balance.signed(1)
			target := b
			if !isAsc {
				delta, target = -delta, a
			}
			if target.sign == 0 && abs(delta) == target.amount && delta != 0 {
				target.sign = 1
				if delta < 0 {
					target.sign = -1
				}
			}
		}
	}
	// 2. Document convention.
	var plus, minus bool
	for _, e := range es {
		for _, m := range e.amounts {
			if m.dc {
				continue
			}
			if m.sign > 0 {
				plus = true
			} else if m.sign < 0 {
				minus = true
			}
		}
	}
	for _, e := range es {
		if e.sign != 0 {
			continue
		}
		desc := strings.Join(e.desc, " ")
		switch {
		case minus && !plus:
			e.sign = 1
		case plus && !minus:
			e.sign = -1
		case incomeWordsRe.MatchString(desc):
			e.sign = 1
		default:
			e.sign = -1
		}
	}
}
