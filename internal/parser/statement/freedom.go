package statement

// Freedom Bank Kazakhstan (bankffin.kz) "Super Card" statements.
//
// The statement is a table "Date | The amount | Currency | Operation | Details"
// repeated on every page under a page header. A row looks like
//
//	23.10.2026 -102,375.63 ₸ KZT Transfer Между своими счетами
//
// and the Details cell may wrap over several visual lines. Two sources are supported:
//
//   - PDF text layer (one string per visual line, as returned by the OCR service).
//     A wrapped line keeps a trailing space ("soft wrap"), the last line of a cell
//     does not; that tells continuation lines from the head of a cell that a page
//     break has split (those lines end the page and belong to the next page's first row).
//   - The bank's CSV export: a PDF-to-table dump where each cell is vertically
//     centered around the dated row, so wrapped lines appear both above and below
//     it as rows with empty Date/Amount/Currency columns.

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"
)

const freedomCSVHeader = "date,the amount,currency,operation,details"

var (
	fbRowRe    = regexp.MustCompile(`^(\d{2}\.\d{2}\.\d{4})\s+([+-]?(?:\d{1,3}(?:,\d{3})+|\d+)(?:\.\d{1,2})?)\s*(?:([^\s\d]{1,4})\s+)?([A-Z]{3})\b\s*(.*)$`)
	fbAmountRe = regexp.MustCompile(`^[+-]?(?:\d{1,3}(?:,\d{3})+|\d+)(?:\.\d{1,2})?`)
	fbCodeRe   = regexp.MustCompile(`^[A-Z]{3}$`)
)

// Operation types as printed by the bank, multi-word ones first.
var fbOps = []string{"Pending amount", "Credit payment", "Replenishment", "Acquiring", "Withdrawal",
	"Repayment", "Overdraft", "Transfer", "Payment", "Others"}

// Expense-like operations: an unsigned amount of these is money going out.
var fbExpenseOps = map[string]bool{"acquiring": true, "payment": true, "transfer": true, "withdrawal": true,
	"credit payment": true, "pending amount": true, "overdraft": true}

type fbEntry struct {
	date     time.Time
	amount   int64 // 2-decimal cents
	signed   bool
	currency string
	op       string
	lines    []string // Details cell, one element per visual line
}

// IsPDF reports whether data looks like a PDF file.
func IsPDF(data []byte) bool {
	return bytes.Contains(data[:min(len(data), 1024)], []byte("%PDF-"))
}

func isFreedomCSV(data []byte) bool {
	return bytes.Contains(bytes.ToLower(data), []byte(freedomCSVHeader))
}

func freedomMarker(pages [][]string) int {
	return markerPos(pages, func(ll string) bool {
		return containsAny(ll, "bankffin.kz", "freedom bank") || isFBHeader(ll)
	})
}

func isFBHeader(line string) bool {
	return strings.EqualFold(strings.Join(strings.Fields(line), " "), "Date The amount Currency Operation Details")
}

// Page header block: everything from it to the next table header (or dated row) is skipped.
func isFBPageHeader(line string) bool {
	return strings.HasPrefix(line, "You can check the authenticity")
}

// Footnotes that may appear right under the table.
func isFBFootnote(line string) bool {
	return strings.HasPrefix(line, "Pending amount. We") || strings.HasPrefix(line, "Based on the Bank")
}

func hasSoftWrap(raw string) bool {
	return strings.TrimRight(raw, " \t ") != raw
}

// ParseFreedomLines parses the PDF text layer of a Freedom Bank statement given
// as one stream of lines (page boundaries are found by the page header).
func ParseFreedomLines(lines []string) ([]Row, error) {
	return ParseFreedomPages([][]string{lines})
}

// ParseFreedomPages parses the PDF text layer of a Freedom Bank statement, one
// slice of visual lines per page.
func ParseFreedomPages(pages [][]string) ([]Row, error) {
	softWrap := false
	for _, page := range pages {
		for _, l := range page {
			l = strings.TrimRight(l, "\r\n")
			if strings.TrimSpace(l) != "" && hasSoftWrap(l) {
				softWrap = true
			}
		}
	}
	var (
		entries  []*fbEntry
		cur      *fbEntry
		closedAt = -1     // len(cur.lines) right after the last line that ended a cell
		opTail   string   // rest of a wrapped operation name ("amount" of "Pending amount")
		orphans  []string // head of a cell split by a page break, for the next row
		skipping bool
	)
	// pageEnd: lines after cur's closed cell that soft-wrap into the next page
	// are the head of the next page's first row, not a tail of cur.
	pageEnd := func() {
		if cur == nil || !softWrap || closedAt < 0 || closedAt >= len(cur.lines) {
			return
		}
		orphans = append(orphans, cur.lines[closedAt:]...)
		cur.lines = cur.lines[:closedAt]
	}
	add := func(line, raw string) {
		cur.lines = append(cur.lines, line)
		if !hasSoftWrap(raw) {
			closedAt = len(cur.lines)
		}
	}
	for _, page := range pages {
		for _, raw := range page {
			raw = strings.TrimRight(raw, "\r\n")
			line := strings.TrimSpace(raw)
			if line == "" {
				continue
			}
			if isFBHeader(line) {
				skipping = false
				continue
			}
			if isFBPageHeader(line) {
				pageEnd()
				skipping = true
				continue
			}
			if m := fbRowRe.FindStringSubmatch(line); m != nil {
				if e, ok := newFBEntry(m[1], m[2], m[4]); ok {
					skipping = false
					op, details, tail := splitFBOp(m[5])
					e.op, e.lines, orphans = op, orphans, nil
					cur, opTail, closedAt = e, tail, -1
					if details != "" {
						add(details, raw)
					}
					entries = append(entries, e)
					continue
				}
			}
			if skipping || cur == nil || isFBFootnote(line) {
				continue
			}
			if opTail != "" {
				t := opTail
				opTail = ""
				if rest, ok := cutPrefixFold(line, t); ok {
					if rest = strings.TrimSpace(rest); rest != "" {
						add(rest, raw)
					}
					continue
				}
			}
			add(line, raw)
		}
		pageEnd()
	}
	if len(entries) == 0 {
		return nil, errors.New("freedom: no transactions found")
	}
	cur.lines = append(cur.lines, orphans...)
	return freedomRows(entries), nil
}

func cutPrefixFold(s, prefix string) (string, bool) {
	if len(s) < len(prefix) || !strings.EqualFold(s[:len(prefix)], prefix) {
		return "", false
	}
	rest := s[len(prefix):]
	if rest != "" && rest[0] != ' ' {
		return "", false
	}
	return rest, true
}

// splitFBOp splits "Acquiring COFFEE HOUSE 12" into the operation and details.
// For a wrapped multi-word operation ("Pending" + next line "amount") it
// returns the full name and the missing words as tail.
func splitFBOp(rest string) (op, details, tail string) {
	rest = strings.TrimSpace(rest)
	for _, o := range fbOps {
		if r, ok := cutPrefixFold(rest, o); ok {
			return o, strings.TrimSpace(r), ""
		}
	}
	for _, o := range fbOps {
		first, more, multi := strings.Cut(o, " ")
		if !multi {
			continue
		}
		if r, ok := cutPrefixFold(rest, first); ok {
			return o, strings.TrimSpace(r), more
		}
	}
	return "", rest, ""
}

func newFBEntry(date, amount, code string) (*fbEntry, bool) {
	d, err := time.Parse("02.01.2006", date)
	if err != nil || !fbCodeRe.MatchString(code) {
		return nil, false
	}
	a, err := parseAmount(amount)
	if err != nil {
		return nil, false
	}
	return &fbEntry{date: d, amount: a, currency: code,
		signed: strings.HasPrefix(amount, "-") || strings.HasPrefix(amount, "+")}, true
}

// ParseFreedomCSV parses Freedom Bank's CSV export.
func ParseFreedomCSV(data []byte) ([]Row, error) {
	r := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))))
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	records, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("csv: %w", err)
	}

	// A section is the table part of one page: dated rows and fragment rows
	// (wrapped cell lines with empty Date/Amount/Currency).
	var (
		sections [][]fbItem
		sec      []fbItem
		inTable  bool
	)
	flush := func() {
		if len(sec) > 0 {
			sections = append(sections, sec)
		}
		sec = nil
	}
	for _, rec := range records {
		col := func(i int) string {
			if i < len(rec) {
				return strings.TrimSpace(rec[i])
			}
			return ""
		}
		if strings.EqualFold(col(0), "Date") && strings.EqualFold(col(1), "The amount") {
			flush()
			inTable = true
			continue
		}
		if !inTable {
			continue
		}
		empty := true
		for i := range rec {
			if col(i) != "" {
				empty = false
				break
			}
		}
		switch {
		case empty:
		case col(0) == "" && col(1) == "" && col(2) == "":
			sec = append(sec, fbItem{op: col(3), det: col(4)})
		default:
			if e := csvFBEntry(col(0), col(1), col(2)); e != nil {
				sec = append(sec, fbItem{entry: e, op: col(3), det: col(4)})
				continue
			}
			// Page footer/header text: the table of this page is over.
			flush()
			inTable = false
		}
	}
	flush()

	var entries []*fbEntry
	var carry []fbItem // head of a cell split by a page break
	for _, s := range sections {
		var got []*fbEntry
		got, carry = assignFBSection(s, carry)
		entries = append(entries, got...)
	}
	if len(entries) == 0 {
		return nil, errors.New("freedom csv: no transactions found")
	}
	last := entries[len(entries)-1]
	for _, it := range carry {
		addFBItem(last, it)
	}
	return freedomRows(entries), nil
}

type fbItem struct {
	entry   *fbEntry // nil for a fragment row
	op, det string
}

func csvFBEntry(date, amount, code string) *fbEntry {
	a := fbAmountRe.FindString(strings.TrimSpace(amount))
	if a == "" {
		return nil
	}
	e, ok := newFBEntry(date, a, code)
	if !ok {
		return nil
	}
	return e
}

func addFBItem(e *fbEntry, it fbItem) {
	if it.op != "" {
		e.op = strings.TrimSpace(e.op + " " + it.op)
	}
	if it.det != "" {
		e.lines = append(e.lines, it.det)
	}
}

// assignFBSection distributes fragment rows between dated rows. Cells are
// vertically centered, so a dated row has as many fragments above as below.
// The first row of a page may be the rest of a cell split by the page break:
// it is then top-aligned (nothing above, unknown count below), so every split
// is tried and the one that keeps all other rows symmetric wins.
func assignFBSection(items []fbItem, carry []fbItem) ([]*fbEntry, []fbItem) {
	var rows []fbItem
	gaps := [][]fbItem{nil}
	for _, it := range items {
		if it.entry == nil {
			gaps[len(gaps)-1] = append(gaps[len(gaps)-1], it)
			continue
		}
		rows = append(rows, it)
		gaps = append(gaps, nil)
	}
	m := len(rows)
	if m == 0 {
		return nil, append(carry, items...)
	}
	cont := len(carry) > 0 && len(gaps[0]) == 0

	type plan struct {
		above, below []int
		penalty      int
	}
	eval := func(b0 int) plan {
		p := plan{above: make([]int, m), below: make([]int, m)}
		p.above[0] = len(gaps[0])
		p.below[0] = b0
		if !cont {
			p.below[0] = p.above[0]
		}
		for i := 0; i < m; i++ {
			if i > 0 {
				p.above[i] = len(gaps[i]) - p.below[i-1]
				p.below[i] = p.above[i]
			}
			if p.below[i] > len(gaps[i+1]) {
				p.below[i] = len(gaps[i+1])
				p.penalty += 10
			}
			// An empty dated cell means the details were centered around it (even line count).
			if (i > 0 || !cont) && rows[i].det == "" && p.above[i] == 0 {
				p.penalty++
			}
		}
		return p
	}
	best := eval(0)
	if cont {
		for b := 1; b <= len(gaps[1]); b++ {
			if p := eval(b); p.penalty <= best.penalty {
				best = p // ties: prefer the longer continuation
			}
		}
	}

	out := make([]*fbEntry, m)
	for i, r := range rows {
		e := r.entry
		if i == 0 {
			for _, it := range carry {
				addFBItem(e, it)
			}
		}
		g := gaps[i]
		for _, it := range g[len(g)-best.above[i]:] {
			addFBItem(e, it)
		}
		addFBItem(e, fbItem{op: r.op, det: r.det})
		for _, it := range gaps[i+1][:best.below[i]] {
			addFBItem(e, it)
		}
		out[i] = e
	}
	tail := gaps[m][best.below[m-1]:]
	return out, append([]fbItem(nil), tail...)
}

// freedomRows applies the statement semantics: pending holds are skipped
// (they reappear later as final operations), own-account transfers and currency
// conversions are hinted as "transfer", and dedup keys are stable across
// overlapping exports and across the PDF and CSV versions of a statement.
func freedomRows(entries []*fbEntry) []Row {
	rows := make([]Row, 0, len(entries))
	seen := map[string]int{}
	for _, e := range entries {
		op := strings.ToLower(e.op)
		if strings.HasPrefix(op, "pending") || e.amount == 0 {
			continue
		}
		amount := e.amount
		if !e.signed && fbExpenseOps[op] {
			amount = -amount
		}
		details := strings.Join(strings.Fields(strings.Join(e.lines, " ")), " ")
		desc := details
		if desc == "" {
			desc = e.op
		}
		row := Row{Date: e.date, AmountCents: amount, Description: desc, Currency: e.currency,
			Merchant: freedomMerchant(op, e.lines), Category: freedomCategory(details)}
		id := e.currency + "|" + details
		base := dedupKey(row.Date, amount, id, 0)
		row.DedupKey = dedupKey(row.Date, amount, id, seen[base])
		seen[base]++
		rows = append(rows, row)
	}
	return rows
}

// Phrases of own-account moves and currency exchange: not income nor spending.
var fbTransferHints = []string{"между своими счетами", "перевод валюты", "по курсу", "проданой валюты",
	"проданной валюты", "купленной валюты", "конвертац", "прием вклада", "выплата вклада"}

func freedomCategory(details string) string {
	d := strings.ToLower(details)
	for _, h := range fbTransferHints {
		if strings.Contains(d, h) {
			return "transfer"
		}
	}
	return ""
}

// Trailing country codes of card acquiring descriptors ("COFFEE HOUSE 12 SEATTLE US").
var fbCountries = map[string]bool{}

func init() {
	for _, c := range strings.Fields(`US GB IE KZ TR AE DE FR NL ES IT RU CN CA AU JP KR GE UZ KG AM AZ TH SG CH AT
		PL CZ LT LV EE PT SE NO FI DK BE LU CY MT HK IN ID MY VN MX BR IL EG SA QA`) {
		fbCountries[c] = true
	}
}

func freedomMerchant(op string, lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	first := lines[0]
	if op == "acquiring" || strings.HasPrefix(op, "pending") {
		if w := strings.Fields(first); len(w) > 1 && fbCountries[w[len(w)-1]] {
			first = strings.Join(w[:len(w)-1], " ")
		}
	}
	// NormalizeMerchant drops long numbers; drop what is left of them ("O*25-15134" -> "O 25- -").
	var words []string
	for _, w := range strings.Fields(NormalizeMerchant(first)) {
		if w = strings.TrimRight(w, "-"); strings.IndexFunc(w, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }) >= 0 {
			words = append(words, w)
		}
	}
	return strings.Join(words, " ")
}
