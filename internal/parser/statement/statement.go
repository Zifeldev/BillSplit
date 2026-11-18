// Package statement parses bank statement exports into normalized transactions:
//   - CSV and OFX/QFX of US banks (Chase, BofA, Wells Fargo, Capital One, Amex...);
//   - Kazakh banks: Freedom Bank (PDF text + CSV, freedom.go), Bank CenterCredit
//     (PDF text, EN/RU, bcc.go) and Kaspi Gold (PDF text, kaspi.go — built
//     without a real sample);
//   - anything else through line-based heuristics (generic.go), for PDF text
//     and for CSV/TSV/semicolon files the US CSV parser can't read.
//
// See detect.go for the detection order. When nothing is found the error is
// ErrUnrecognized, whose message is meant for the user.
package statement

import (
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Format int16

const (
	FormatAuto Format = 0
	FormatCSV  Format = 1
	FormatOFX  Format = 2
	// FormatPDF is a bank PDF statement. PDFs are turned into text lines by the
	// OCR service first and then parsed with ParsePages.
	FormatPDF Format = 3
)

type Row struct {
	Date        time.Time
	AmountCents int64 // <0 expense, >0 income
	Description string
	Merchant    string
	DedupKey    string // stable across re-imports of the same file
	Currency    string // ISO code of the row; empty — use the import's currency
	Category    string // category hint from the bank's own data (e.g. "transfer"); empty — use rules
}

var ErrUnsupported = errors.New("unsupported statement format")

func Parse(data []byte, f Format) ([]Row, error) {
	if f == FormatAuto {
		f = detect(data)
	}
	switch f {
	case FormatCSV:
		return parseCSVAny(data)
	case FormatOFX:
		return ParseOFX(data)
	case FormatPDF:
		return nil, fmt.Errorf("%w: pdf must be converted to text lines first (ParsePages)", ErrUnsupported)
	default:
		return nil, ErrUnsupported
	}
}

// Detect guesses the format of a statement file.
func Detect(data []byte) Format { return detect(data) }

func detect(data []byte) Format {
	if IsPDF(data) {
		return FormatPDF
	}
	head := strings.ToUpper(string(data[:min(len(data), 512)]))
	if strings.Contains(head, "OFXHEADER") || strings.Contains(head, "<OFX>") {
		return FormatOFX
	}
	return FormatCSV
}

// ---------- CSV ----------

// Header aliases in priority order.
var (
	dateCols   = []string{"posting date", "transaction date", "date", "posted date", "trans. date"}
	descCols   = []string{"description", "payee", "name", "merchant", "memo", "details"}
	amountCols = []string{"amount", "transaction amount"}
	debitCols  = []string{"debit", "withdrawal", "withdrawals", "money out"}
	creditCols = []string{"credit", "deposit", "deposits", "money in"}
)

func ParseCSV(data []byte) ([]Row, error) {
	if isFreedomCSV(data) {
		return ParseFreedomCSV(data)
	}
	r := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(data, []byte("\xef\xbb\xbf")))) // strip BOM
	r.FieldsPerRecord = -1
	r.TrimLeadingSpace = true
	records, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("csv: %w", err)
	}
	if len(records) == 0 {
		return nil, errors.New("csv: empty file")
	}

	idx := mapHeader(records[0])
	body := records[1:]
	if idx.date < 0 || (idx.amount < 0 && idx.debit < 0 && idx.credit < 0) {
		// No recognizable header (e.g. Wells Fargo): guess columns from the first row.
		idx = guessColumns(records[0])
		body = records
		if idx.date < 0 || idx.amount < 0 {
			return nil, errors.New("csv: cannot detect date/amount columns")
		}
	}

	rows := make([]Row, 0, len(body))
	seen := map[string]int{}
	for n, rec := range body {
		if len(rec) == 0 || (len(rec) == 1 && strings.TrimSpace(rec[0]) == "") {
			continue
		}
		row, err := idx.row(rec)
		if err != nil {
			return nil, fmt.Errorf("csv line %d: %w", n+2, err)
		}
		if row.AmountCents == 0 {
			continue
		}
		row.Merchant = NormalizeMerchant(row.Description)
		// Ordinal among identical rows keeps two same-day coffees apart but stays stable across overlapping exports.
		base := dedupKey(row.Date, row.AmountCents, row.Description, 0)
		row.DedupKey = dedupKey(row.Date, row.AmountCents, row.Description, seen[base])
		seen[base]++
		rows = append(rows, row)
	}
	return rows, nil
}

type colIndex struct{ date, desc, amount, debit, credit int }

func mapHeader(h []string) colIndex {
	norm := make([]string, len(h))
	for i, c := range h {
		norm[i] = strings.ToLower(strings.TrimSpace(c))
	}
	find := func(aliases []string) int {
		for _, a := range aliases {
			for i, c := range norm {
				if c == a {
					return i
				}
			}
		}
		return -1
	}
	return colIndex{date: find(dateCols), desc: find(descCols), amount: find(amountCols),
		debit: find(debitCols), credit: find(creditCols)}
}

func guessColumns(rec []string) colIndex {
	idx := colIndex{-1, -1, -1, -1, -1}
	longest := 0
	for i, v := range rec {
		v = strings.TrimSpace(v)
		switch {
		case idx.date < 0 && parseDateOK(v):
			idx.date = i
		case idx.amount < 0 && amountOK(v):
			idx.amount = i
		case len(v) > longest && !amountOK(v):
			idx.desc, longest = i, len(v)
		}
	}
	return idx
}

func (c colIndex) row(rec []string) (Row, error) {
	get := func(i int) string {
		if i < 0 || i >= len(rec) {
			return ""
		}
		return strings.TrimSpace(rec[i])
	}
	d, err := parseDate(get(c.date))
	if err != nil {
		return Row{}, err
	}
	var amount int64
	if c.amount >= 0 {
		if amount, err = parseAmount(get(c.amount)); err != nil {
			return Row{}, err
		}
	} else {
		if v := get(c.debit); v != "" {
			a, err := parseAmount(v)
			if err != nil {
				return Row{}, err
			}
			amount -= abs(a)
		}
		if v := get(c.credit); v != "" {
			a, err := parseAmount(v)
			if err != nil {
				return Row{}, err
			}
			amount += abs(a)
		}
	}
	return Row{Date: d, AmountCents: amount, Description: get(c.desc)}, nil
}

var dateLayouts = []string{"01/02/2006", "1/2/2006", "2006-01-02", "01/02/06", "1/2/06", "01-02-2006", "Jan 2, 2006", "02 Jan 2006"}

func parseDate(s string) (time.Time, error) {
	for _, l := range dateLayouts {
		if t, err := time.Parse(l, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("bad date %q", s)
}

func parseDateOK(s string) bool { _, err := parseDate(s); return err == nil }

var amountClean = strings.NewReplacer("$", "", ",", "", " ", "", "USD", "")

// parseAmount handles "-12.34", "$1,234.56", "(12.34)" and "12" without floats.
func parseAmount(s string) (int64, error) {
	s = amountClean.Replace(strings.TrimSpace(s))
	neg := false
	if strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")") {
		neg, s = true, s[1:len(s)-1]
	}
	if strings.HasPrefix(s, "-") {
		neg, s = !neg, s[1:]
	}
	s = strings.TrimPrefix(s, "+")
	whole, frac, _ := strings.Cut(s, ".")
	if len(frac) > 2 {
		frac = frac[:2]
	}
	for len(frac) < 2 {
		frac += "0"
	}
	w, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("bad amount %q", s)
	}
	f, err := strconv.ParseInt(frac, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("bad amount %q", s)
	}
	v := w*100 + f
	if neg {
		v = -v
	}
	return v, nil
}

func amountOK(s string) bool {
	if !strings.ContainsAny(s, ".0123456789") {
		return false
	}
	_, err := parseAmount(s)
	return err == nil
}

// ---------- OFX / QFX ----------

var (
	stmtTrnRe = regexp.MustCompile(`(?is)<STMTTRN>(.*?)(?:</STMTTRN>|<STMTTRN>|</BANKTRANLIST>)`)
	tagRe     = regexp.MustCompile(`(?i)<(TRNAMT|DTPOSTED|NAME|MEMO|FITID)>([^<\r\n]*)`)
)

func ParseOFX(data []byte) ([]Row, error) {
	blocks := stmtTrnRe.FindAllSubmatch(data, -1)
	if len(blocks) == 0 {
		return nil, errors.New("ofx: no transactions")
	}
	rows := make([]Row, 0, len(blocks))
	for n, b := range blocks {
		fields := map[string]string{}
		for _, m := range tagRe.FindAllSubmatch(b[1], -1) {
			fields[strings.ToUpper(string(m[1]))] = strings.TrimSpace(string(m[2]))
		}
		dt := fields["DTPOSTED"]
		if len(dt) < 8 {
			return nil, fmt.Errorf("ofx transaction %d: bad DTPOSTED", n)
		}
		d, err := time.Parse("20060102", dt[:8])
		if err != nil {
			return nil, fmt.Errorf("ofx transaction %d: %w", n, err)
		}
		amount, err := parseAmount(fields["TRNAMT"])
		if err != nil {
			return nil, fmt.Errorf("ofx transaction %d: %w", n, err)
		}
		desc := strings.TrimSpace(fields["NAME"] + " " + fields["MEMO"])
		key := fields["FITID"]
		if key == "" {
			key = dedupKey(d, amount, desc, 0)
		} else {
			key = "fitid:" + key
		}
		rows = append(rows, Row{Date: d, AmountCents: amount, Description: desc,
			Merchant: NormalizeMerchant(fields["NAME"]), DedupKey: key})
	}
	return rows, nil
}

// ---------- helpers ----------

// dedupKey identifies a row; ordinal distinguishes identical purchases on the same day.
func dedupKey(d time.Time, amount int64, desc string, ordinal int) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%s|%d|%s|%d", d.Format("2006-01-02"), amount, strings.ToLower(desc), ordinal)))
	return hex.EncodeToString(h[:16])
}

var (
	merchantPrefixes = regexp.MustCompile(`(?i)^(pos |debit card purchase |purchase |purchase authorized on \d\d/\d\d |card purchase |recurring payment |checkcard \d+ |ach debit |sq \*|tst\* |pp\*|paypal \*)+`)
	merchantNoise    = regexp.MustCompile(`(?i)(#\s*\d+|\b\d{4,}\b|\b\d\d/\d\d\b|x{2,}\d+|\*+\w*)`)
	multiSpace       = regexp.MustCompile(`\s+`)
)

// NormalizeMerchant turns "POS DEBIT CARD PURCHASE WAL-MART #1234 SPRINGFIELD IL" into "WAL-MART SPRINGFIELD IL".
func NormalizeMerchant(desc string) string {
	s := merchantPrefixes.ReplaceAllString(strings.TrimSpace(desc), "")
	s = merchantNoise.ReplaceAllString(s, " ")
	s = strings.TrimSpace(multiSpace.ReplaceAllString(s, " "))
	words := strings.Fields(s)
	if len(words) > 4 {
		words = words[:4]
	}
	return strings.ToUpper(strings.Join(words, " "))
}

func abs(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
