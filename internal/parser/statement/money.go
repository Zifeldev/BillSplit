package statement

// Locale-tolerant money and date helpers shared by the BCC, Kaspi and generic
// text parsers. Amounts are returned as 2-decimal cents without floats.

import (
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Currency symbols and codes recognized next to amounts.
var currencySymbols = map[string]string{
	"₸": "KZT", "тг": "KZT", "тг.": "KZT", "тенге": "KZT",
	"$": "USD", "€": "EUR", "₽": "RUB", "руб": "RUB", "руб.": "RUB", "£": "GBP",
}

var isoCodes = map[string]bool{}

func init() {
	for _, c := range strings.Fields("KZT USD EUR RUB GBP CNY KGS UZS TRY AED GEL AMD AZN BYN CHF JPY KRW THB PLN CZK CAD AUD") {
		isoCodes[c] = true
	}
}

// moneyRe finds amount candidates: optional sign/parenthesis/currency before,
// digits with space, comma or dot thousands groups and 1-2 decimals, then an
// optional currency and Dr/Cr marker or trailing minus. Boundaries and
// plausibility are checked in findMoney.
var moneyRe = regexp.MustCompile(`\(?` +
	`(?:[+\-−–]\s?)?` +
	`(?:(?:[$€₽₸£])\s?)?` +
	`(?:[+\-−–])?` +
	`(?:\d{1,3}(?:(?:[ \x{00a0}\x{202f}]\d{3})+|(?:,\d{3})+|(?:\.\d{3})+)|\d+)` +
	`(?:[.,]\d{1,2})?` +
	`(?:\s?(?:[$€₽₸£]|KZT|USD|EUR|RUB|GBP|CNY|KGS|UZS|TRY|AED|тг\.?|тенге|руб\.?))?` +
	`\)?` +
	`(?:\s?(?:CR|DR|Cr|Dr)\b|-)?`)

type money struct {
	cents    int64 // absolute value
	sign     int   // -1, +1 or 0 when printed unsigned
	currency string
	dc       bool // sign comes from a Dr/Cr marker
	start    int  // byte offsets in the scanned string
	end      int
}

func isWordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '*' }

// findMoney returns the plausible amounts of s in order. An amount needs
// decimals, a currency marker or an attached sign; bare integers (store
// numbers, card digits) are ignored.
func findMoney(s string) []money {
	var out []money
	for pos := 0; pos < len(s); {
		loc := moneyRe.FindStringIndex(s[pos:])
		if loc == nil {
			break
		}
		start, end := pos+loc[0], pos+loc[1]
		if m, ok := moneyAt(s, start, end); ok {
			out = append(out, m)
			pos = end
			continue
		}
		// Rejected: retry right after its first rune, so "123 $4.50" still finds "$4.50".
		_, size := utf8.DecodeRuneInString(s[start:])
		pos = start + size
	}
	return out
}

func moneyAt(s string, start, end int) (money, bool) {
	tok := s[start:end]
	if start > 0 {
		r, _ := utf8.DecodeLastRuneInString(s[:start])
		if isWordRune(r) || r == '.' || r == ',' || r == '/' || r == ':' {
			return money{}, false
		}
	}
	if end < len(s) {
		r, _ := utf8.DecodeRuneInString(s[end:])
		if isWordRune(r) || r == '/' || r == ':' {
			return money{}, false
		}
		if (r == '.' || r == ',') && end+1 < len(s) && s[end+1] >= '0' && s[end+1] <= '9' {
			return money{}, false
		}
	}
	m, ok := parseMoneyToken(tok)
	if !ok {
		return money{}, false
	}
	m.start, m.end = start, end
	return m, true
}

var (
	digitsRe   = regexp.MustCompile(`\d[\d \x{00a0}\x{202f}.,]*`)
	trailingDC = regexp.MustCompile(`(?i)\s?(cr|dr)$`)
)

func parseMoneyToken(tok string) (money, bool) {
	var m money
	t := strings.TrimSpace(tok)
	if mm := trailingDC.FindStringSubmatch(t); mm != nil {
		m.dc = true
		if strings.EqualFold(mm[1], "dr") {
			m.sign = -1
		} else {
			m.sign = 1
		}
		t = strings.TrimSpace(t[:len(t)-len(mm[0])])
	}
	if strings.HasPrefix(t, "(") && strings.HasSuffix(t, ")") {
		m.sign = -1
		t = t[1 : len(t)-1]
	} else if strings.HasSuffix(t, "-") && !m.dc {
		m.sign = -1
		t = strings.TrimSpace(t[:len(t)-1])
	}
	t = strings.Trim(t, "()")
	for _, r := range []string{"-", "−", "–"} {
		if strings.Contains(t, r) {
			m.sign = -1
		}
	}
	if strings.Contains(t, "+") {
		m.sign = 1
	}
	num := digitsRe.FindString(t)
	if num == "" {
		return m, false
	}
	rest := strings.TrimSpace(strings.Replace(t, num, " ", 1))
	for _, f := range strings.Fields(strings.NewReplacer("+", " ", "-", " ", "−", " ", "–", " ").Replace(rest)) {
		if c, ok := currencySymbols[strings.ToLower(f)]; ok {
			m.currency = c
		} else if isoCodes[f] {
			m.currency = f
		}
	}
	num = strings.TrimRight(strings.TrimSpace(num), ".,")
	cents, hasDec, ok := parseNumber(num)
	if !ok {
		return m, false
	}
	// Plausibility: decimals, a currency or an explicit sign/marker.
	if !hasDec && m.currency == "" && m.sign == 0 {
		return m, false
	}
	m.cents = cents
	return m, true
}

// parseNumber parses "1 234,56", "1,234.56", "1.234,56", "1234.5", "1 500"
// into cents. The last '.'/',' followed by 1-2 digits is the decimal mark;
// one followed by exactly 3 digits is a thousands separator.
func parseNumber(s string) (cents int64, hasDec bool, ok bool) {
	s = strings.NewReplacer(" ", "", " ", "", " ", "").Replace(s)
	if s == "" {
		return 0, false, false
	}
	whole, frac := s, ""
	if i := strings.LastIndexAny(s, ".,"); i >= 0 {
		sep, after := s[i:i+1], s[i+1:]
		other := "."
		if sep == "." {
			other = ","
		}
		switch {
		case strings.Contains(s[:i], other): // "1.234,56", "1,234.56"
			whole, frac, hasDec = s[:i], after, true
		case strings.Count(s, sep) > 1 || len(after) == 3: // "1.234.567", "1,234"
		default:
			whole, frac, hasDec = s[:i], after, true
		}
	}
	whole = strings.NewReplacer(",", "", ".", "").Replace(whole)
	if len(frac) > 2 {
		frac = frac[:2]
	}
	for len(frac) < 2 {
		frac += "0"
	}
	w, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return 0, false, false
	}
	f, err := strconv.ParseInt(frac, 10, 64)
	if err != nil {
		return 0, false, false
	}
	return w*100 + f, hasDec, true
}

func (m money) signed(def int) int64 {
	switch {
	case m.sign < 0:
		return -m.cents
	case m.sign > 0:
		return m.cents
	case def < 0:
		return -m.cents
	default:
		return m.cents
	}
}

// ---------- dates ----------

var ruMonths = map[string]time.Month{"янв": 1, "фев": 2, "мар": 3, "апр": 4, "мая": 5, "май": 5, "июн": 6,
	"июл": 7, "авг": 8, "сен": 9, "окт": 10, "ноя": 11, "дек": 12}
var enMonths = map[string]time.Month{"jan": 1, "feb": 2, "mar": 3, "apr": 4, "may": 5, "jun": 6, "jul": 7,
	"aug": 8, "sep": 9, "oct": 10, "nov": 11, "dec": 12}

const (
	ruMonRe = `(янв|фев|мар|апр|ма[йя]|июн|июл|авг|сен|окт|ноя|дек)[а-я]*\.?`
	enMonRe = `(jan|feb|mar|apr|may|jun|jul|aug|sep|oct|nov|dec)[a-z]*\.?`
)

var (
	isoDateRe  = regexp.MustCompile(`^(\d{4})-(\d{2})-(\d{2})`)
	dotDateRe  = regexp.MustCompile(`^(\d{1,2})\.(\d{1,2})\.(\d{4}|\d{2})`)
	slashDate  = regexp.MustCompile(`^(\d{1,2})[/-](\d{1,2})[/-](\d{4}|\d{2})`)
	ruTextDate = regexp.MustCompile(`(?i)^(\d{1,2})\s+` + ruMonRe + `\s+(\d{4})(?:\s*г\.?)?`)
	enTextDate = regexp.MustCompile(`(?i)^(\d{1,2})\s+` + enMonRe + `,?\s+(\d{4})`)
	enUSDate   = regexp.MustCompile(`(?i)^` + enMonRe + `\s+(\d{1,2}),?\s+(\d{4})`)
	timeRe     = regexp.MustCompile(`^\s*\d{1,2}:\d{2}(?::\d{2})?`)
	// anyDateRe locates dates inside text so they are not taken for amounts.
	anyDateRe = regexp.MustCompile(`(?i)\b(?:\d{4}-\d{2}-\d{2}|\d{1,2}[./-]\d{1,2}[./-](?:\d{4}|\d{2})|\d{1,2}:\d{2}(?::\d{2})?)\b`)
)

type dateOrder int

const (
	dayFirst dateOrder = iota
	monthFirst
)

func year2(s string) int {
	y, _ := strconv.Atoi(s)
	if len(s) == 2 {
		y += 2000
	}
	return y
}

func mkDate(y, m, d int) (time.Time, bool) {
	if m < 1 || m > 12 || d < 1 || d > 31 || y < 1990 || y > 2100 {
		return time.Time{}, false
	}
	t := time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC)
	if t.Day() != d {
		return time.Time{}, false
	}
	return t, true
}

// leadingDate parses a date at the start of s and returns it with the length
// of the matched prefix (including a following time, if any).
func leadingDate(s string, order dateOrder) (time.Time, int, bool) {
	atoi := func(x string) int { v, _ := strconv.Atoi(x); return v }
	var (
		t  time.Time
		n  int
		ok bool
	)
	switch {
	case isoDateRe.MatchString(s):
		m := isoDateRe.FindStringSubmatch(s)
		t, ok = mkDate(atoi(m[1]), atoi(m[2]), atoi(m[3]))
		n = len(m[0])
	case dotDateRe.MatchString(s):
		m := dotDateRe.FindStringSubmatch(s)
		t, ok = mkDate(year2(m[3]), atoi(m[2]), atoi(m[1]))
		n = len(m[0])
	case slashDate.MatchString(s):
		m := slashDate.FindStringSubmatch(s)
		a, b := atoi(m[1]), atoi(m[2])
		if order == monthFirst && a <= 12 || b > 12 {
			a, b = b, a
		}
		t, ok = mkDate(year2(m[3]), b, a)
		n = len(m[0])
	case ruTextDate.MatchString(s):
		m := ruTextDate.FindStringSubmatch(s)
		mon := ruMonths[strings.ToLower(m[2])]
		t, ok = mkDate(atoi(m[3]), int(mon), atoi(m[1]))
		n = len(m[0])
	case enTextDate.MatchString(s):
		m := enTextDate.FindStringSubmatch(s)
		t, ok = mkDate(atoi(m[3]), int(enMonths[strings.ToLower(m[2])]), atoi(m[1]))
		n = len(m[0])
	case enUSDate.MatchString(s):
		m := enUSDate.FindStringSubmatch(s)
		t, ok = mkDate(atoi(m[3]), int(enMonths[strings.ToLower(m[1])]), atoi(m[2]))
		n = len(m[0])
	}
	if !ok {
		return time.Time{}, 0, false
	}
	// The date must end at a word boundary.
	if n < len(s) {
		if r, _ := utf8.DecodeRuneInString(s[n:]); unicode.IsLetter(r) || unicode.IsDigit(r) {
			return time.Time{}, 0, false
		}
	}
	if tm := timeRe.FindString(s[n:]); tm != "" {
		n += len(tm)
	}
	return t, n, true
}
