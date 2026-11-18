// Package receipt turns a receipt photo/text into structured line items.
//
// OCR happens in the ocr-worker (RapidOCR / OCR.space); ParseText is a
// heuristic parser of the recognized text tuned for US grocery receipts
// (Walmart, Target, Costco, Kroger...).
package receipt

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Item struct {
	Name       string // readable name; starts equal to RawName and may be enriched later
	RawName    string // as printed on the receipt
	Code       string // UPC/EAN printed next to the item (Walmart, Target...)
	Quantity   int32
	PriceCents int64 // line total

	lowConfidence bool // price had one decimal digit ("4.8"): OCR likely dropped a digit
}

type Parsed struct {
	Store         string
	Items         []Item
	SubtotalCents int64
	TaxCents      int64
	TotalCents    int64
	PurchasedAt   *time.Time
	Warnings      []string
}

var ErrNoItems = errors.New("no items recognized")

var (
	// "GV MILK 007874235187 F 3.48 N", "TIDE PODS 9.97 X", "COUPON 1.00-"
	itemRe = regexp.MustCompile(`^(.+?)\s+(?:(\d{6,14})\s+)?(?:[A-Z]{1,2}\s+)?(-?\$?\d{1,5}[.,]\d{2})(-)?(?:\s+[A-Z0]{1,2})?$`)
	// Same line where OCR dropped the last cent digit ("BRM OATS 039978004780 4.8").
	// Only trusted when a product code is present, to avoid reading "7.5 OZ" as a price.
	itemShortRe = regexp.MustCompile(`^(.+?)\s+(\d{6,14})\s+(?:[A-Z]{1,2}\s+)?(\$?\d{1,5}[.,]\d)(?:\s+[A-Z0]{1,2})?$`)
	// "2 AT 1 FOR 0.98", "3 @ 1.25"
	qtyRe = regexp.MustCompile(`^(\d{1,3})\s*(?:AT|@)\s*(?:1\s+FOR\s+)?\$?\d+[.,]\d{2}`)
	// "1.32 lb @ 0.58 /lb" — weighted item, the price comes on the item line
	weightRe      = regexp.MustCompile(`(?i)^\d+[.,]\d+\s*(lb|kg|oz)\s*@`)
	gluedFlagRe   = regexp.MustCompile(`(\d{6,14}\s+(?:[A-Z]\s+)?)(\d{1,5}[.,]\d{2})[0O]$`)
	gluedCodeRe   = regexp.MustCompile(`\b(\d{12})(\d{1,4}[.,]\d{2})\b`)
	leadingCodeRe = regexp.MustCompile(`^(\d{6,14})\s+(.+)$`)
	// "TAX1 8.7000 %", "T = AZ TAX 8.7000 on $8.99", "STATE TAX" — but not "TAXI" or "SYNTAX".
	taxWordRe = regexp.MustCompile(`\bTAX\d?\b`)
	amountRe  = regexp.MustCompile(`-?\$?\d{1,6}[.,]\d{2}`)
	dateRe    = regexp.MustCompile(`\b(\d{1,2})/(\d{1,2})/(\d{2}|\d{4})\b`)
	spacesRe  = regexp.MustCompile(`\s+`)
)

var knownStores = []string{"walmart", "target", "costco", "kroger", "safeway", "whole foods", "trader joe",
	"aldi", "publix", "walgreens", "cvs", "home depot", "sam's club", "h-e-b", "meijer"}

// Lines that contain an amount but are not items.
var skipWords = []string{"TEND", "CHANGE", "CASH", "VISA", "MASTERCARD", "AMEX", "DISCOVER", "DEBIT", "CREDIT",
	"BALANCE", "APPROVAL", "AUTH", "REF #", "ACCOUNT", "CARD", "PAYMENT", "SAVINGS", "YOU SAVED", "ITEMS SOLD",
	"TC#", "ST#", "OP#", "TE#", "TR#", "PRICE", "REWARD", "CASHBACK", "EBT"}

// ParseText extracts store, items and totals from OCR text.
func ParseText(text string) (*Parsed, error) {
	p := &Parsed{}
	lines := strings.Split(strings.ReplaceAll(text, "\r", ""), "\n")

	var taxes int64
	var haveSubtotal, haveTotal bool
	for i, raw := range lines {
		line := strings.TrimSpace(spacesRe.ReplaceAllString(raw, " "))
		// OCR sometimes glues a 12-digit product code to the price: "00370008775411.97".
		line = gluedCodeRe.ReplaceAllString(line, "$1 $2")
		// The tax flag "O" after a price is often read as "0" and glued: "0.990" -> "0.99 O".
		line = gluedFlagRe.ReplaceAllString(line, "$1 $2 O")
		if line == "" {
			continue
		}
		upper := strings.ToUpper(line)

		if p.Store == "" && i < 8 {
			if s := detectStore(line); s != "" {
				p.Store = s
			}
		}
		if p.PurchasedAt == nil {
			if t := parseDate(line); t != nil {
				p.PurchasedAt = t
			}
		}

		switch {
		case strings.HasPrefix(upper, "SUBTOTAL") || strings.HasPrefix(upper, "SUB TOTAL"):
			if v, ok := lastAmount(line); ok {
				p.SubtotalCents, haveSubtotal = v, true
			}
			continue
		case strings.HasPrefix(upper, "TAX") || taxWordRe.MatchString(upper):
			if v, ok := lastAmount(line); ok {
				taxes += v
			}
			continue
		case strings.HasPrefix(upper, "TOTAL") || strings.HasPrefix(upper, "BALANCE DUE") || strings.HasPrefix(upper, "AMOUNT DUE"):
			if v, ok := lastAmount(line); ok && !haveTotal {
				p.TotalCents, haveTotal = v, true
			}
			continue
		}
		if haveTotal {
			continue // everything after TOTAL is payment info
		}

		if m := qtyRe.FindStringSubmatch(upper); m != nil && len(p.Items) > 0 {
			q, _ := strconv.Atoi(m[1])
			p.Items[len(p.Items)-1].Quantity = int32(q)
			continue
		}
		if weightRe.MatchString(line) {
			continue
		}
		if containsAny(upper, skipWords) {
			continue
		}

		m := itemRe.FindStringSubmatch(line)
		low := false
		if m == nil {
			sm := itemShortRe.FindStringSubmatch(line)
			if sm == nil {
				continue
			}
			m = []string{sm[0], sm[1], sm[2], sm[3] + "0", ""}
			low = true
		}
		name := cleanName(m[1])
		code := m[2]
		// Target-style lines print the code first: "212000351 GOOD&GATHER MILK NF $3.49".
		if lc := leadingCodeRe.FindStringSubmatch(name); lc != nil {
			name = lc[2]
			if code == "" {
				code = lc[1]
			}
		}
		cents, err := toCents(m[3])
		if err != nil || name == "" {
			continue
		}
		if m[4] == "-" {
			cents = -cents
		}
		// Discounts/coupons reduce the previous item.
		if cents < 0 && len(p.Items) > 0 {
			p.Items[len(p.Items)-1].PriceCents += cents
			continue
		}
		if cents <= 0 {
			continue
		}
		p.Items = append(p.Items, Item{Name: name, RawName: name, Code: code, Quantity: 1, PriceCents: cents, lowConfidence: low})
	}

	if len(p.Items) == 0 {
		return nil, ErrNoItems
	}

	p.TaxCents = taxes
	var sum int64
	for _, it := range p.Items {
		sum += it.PriceCents
	}
	if haveSubtotal {
		sum = repairDroppedDigit(p.Items, p.SubtotalCents, sum)
		sum = repairFromSiblings(p.Items, p.SubtotalCents, sum)
	}
	if !haveSubtotal {
		p.SubtotalCents = sum
	} else if diff := p.SubtotalCents - sum; diff != 0 {
		p.Warnings = append(p.Warnings, fmt.Sprintf("items sum %.2f differs from subtotal %.2f — check items",
			float64(sum)/100, float64(p.SubtotalCents)/100))
	}
	if !haveTotal {
		p.TotalCents = p.SubtotalCents + p.TaxCents
	} else if p.TaxCents == 0 && p.TotalCents > p.SubtotalCents {
		p.TaxCents = p.TotalCents - p.SubtotalCents // tax line was not recognized
	}
	return p, nil
}

func detectStore(line string) string {
	l := strings.ToLower(line)
	for _, s := range knownStores {
		if strings.Contains(l, s) {
			return strings.Title(s) //nolint:staticcheck // ASCII store names only
		}
	}
	return ""
}

func parseDate(line string) *time.Time {
	m := dateRe.FindStringSubmatch(line)
	if m == nil {
		return nil
	}
	layout := "1/2/2006"
	if len(m[3]) == 2 {
		layout = "1/2/06"
	}
	t, err := time.Parse(layout, m[1]+"/"+m[2]+"/"+m[3])
	if err != nil || t.Year() < 2000 {
		return nil
	}
	return &t
}

func lastAmount(line string) (int64, bool) {
	all := amountRe.FindAllString(line, -1)
	if len(all) == 0 {
		return 0, false
	}
	v, err := toCents(all[len(all)-1])
	return v, err == nil
}

// toCents parses "12.34", "$1,234.56" or "12,34" into cents without floats.
func toCents(s string) (int64, error) {
	s = strings.TrimSpace(strings.TrimPrefix(strings.ReplaceAll(s, "$", ""), "+"))
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	if len(s) < 4 {
		return 0, fmt.Errorf("bad amount %q", s)
	}
	intPart := strings.NewReplacer(",", "", ".", "").Replace(s[:len(s)-3])
	frac := s[len(s)-2:]
	whole, err := strconv.ParseInt(intPart, 10, 64)
	if err != nil {
		return 0, err
	}
	f, err := strconv.ParseInt(frac, 10, 64)
	if err != nil {
		return 0, err
	}
	v := whole*100 + f
	if neg {
		v = -v
	}
	return v, nil
}

func cleanName(s string) string {
	s = strings.Trim(s, " -*#")
	return strings.TrimSpace(s)
}

func containsAny(s string, words []string) bool {
	for _, w := range words {
		if strings.Contains(s, w) {
			return true
		}
	}
	return false
}

// repairDroppedDigit fixes the common OCR slip where the last cent digit is lost
// ("4.88" read as "4.8"): if exactly one such low-confidence price exists and the
// printed subtotal differs by less than 10 cents, the difference is that digit.
func repairDroppedDigit(items []Item, subtotal, sum int64) int64 {
	diff := subtotal - sum
	if diff <= 0 || diff >= 10 {
		return sum
	}
	idx := -1
	for i := range items {
		if items[i].lowConfidence {
			if idx >= 0 {
				return sum // ambiguous: more than one candidate
			}
			idx = i
		}
	}
	if idx < 0 {
		return sum
	}
	items[idx].PriceCents += diff
	items[idx].lowConfidence = false
	return subtotal
}

// repairFromSiblings fixes one misread price among identical products: when the
// same product code appears several times and one line's price differs from the
// others by exactly the gap to the printed subtotal ("0.99" read as "0.90"),
// that line takes its siblings' price.
func repairFromSiblings(items []Item, subtotal, sum int64) int64 {
	diff := subtotal - sum
	if diff == 0 {
		return sum
	}
	prices := map[string]map[int64]int{} // code -> price -> count
	for _, it := range items {
		if it.Code == "" {
			continue
		}
		if prices[it.Code] == nil {
			prices[it.Code] = map[int64]int{}
		}
		prices[it.Code][it.PriceCents]++
	}
	for i := range items {
		it := &items[i]
		for p, n := range prices[it.Code] {
			if p != it.PriceCents && n >= 1 && p-it.PriceCents == diff && n >= prices[it.Code][it.PriceCents] {
				it.PriceCents = p
				return subtotal
			}
		}
	}
	return sum
}
