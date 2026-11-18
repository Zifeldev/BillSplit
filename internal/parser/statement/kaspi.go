package statement

// Kaspi Bank (kaspi.kz) "Kaspi Gold" card statements.
//
// NOTE: built WITHOUT a real sample, from the publicly known layout of the
// statement; it is deliberately tolerant and falls back to the generic parser
// when it finds nothing. The statement header names "АО «Kaspi Bank»" /
// "Kaspi Gold" / "www.kaspi.kz", then a summary per operation type (skipped:
// those lines don't start with a date) and the table
//
//	Дата | Сумма | Операция | Детали          (Date | Amount | Transaction | Details)
//	27.10.25  - 1 500,00 ₸  Покупки      MAGNUM CASH&CARRY
//	26.10.25  + 50 000,00 ₸ Пополнения   С Kaspi Депозита
//
// Amounts have space thousands, comma decimals, a sign separated by a space and
// a ₸ suffix. Operation types: Покупки/Purchases, Переводы/Transfers,
// Пополнения/Replenishment, Снятия/Withdrawals, Разное/Others (and their Kazakh
// names). Transfers, replenishments and cash withdrawals are hinted "transfer".

import (
	"errors"
	"regexp"
	"strings"
)

var kaspiRowRe = regexp.MustCompile(`^(\d{2}\.\d{2}\.(?:\d{4}|\d{2}))\s+([+\-−–])?\s*(\d{1,3}(?:[ \x{00a0}\x{202f}]\d{3})*(?:[.,]\d{1,2})?)\s*(₸|\$|€|₽|KZT|USD|EUR|RUB|тг\.?)?(?:\s+(.*))?$`)

type kaspiOp struct {
	names    []string
	sign     int // default sign of an unsigned amount
	category string
}

// Multi-word names first within each operation.
var kaspiOps = []kaspiOp{
	{[]string{"Покупки", "Покупка", "Purchases", "Purchase", "Сатып алулар", "Сатып алу"}, -1, ""},
	{[]string{"Переводы", "Перевод", "Transfers", "Transfer", "Аударымдар", "Аударым"}, -1, "transfer"},
	{[]string{"Пополнения", "Пополнение", "Replenishments", "Replenishment", "Top-up", "Толықтырулар", "Толықтыру"}, 1, "transfer"},
	{[]string{"Снятия", "Снятие", "Withdrawals", "Withdrawal", "Ақша алу"}, -1, "transfer"},
	{[]string{"Разное", "Others", "Other", "Әртүрлі"}, 0, ""},
}

func kaspiMarker(pages [][]string) int {
	return markerPos(pages, func(ll string) bool {
		return containsAny(ll, "kaspi bank", "kaspi.kz", "caspkzka", "kaspi gold") || isKaspiHeader(ll)
	})
}

func isKaspiHeader(l string) bool {
	h := strings.ToLower(strings.Join(strings.Fields(l), " "))
	return h == "дата сумма операция детали" || h == "date amount transaction details" || h == "күні сомасы операция толығырақ"
}

// Lines that are never part of a Details cell.
func isKaspiNoise(ll string) bool {
	return strings.Contains(ll, "kaspi bank") || strings.Contains(ll, "kaspi.kz") || pageNumRe.MatchString(ll) ||
		strings.HasPrefix(ll, "доступно на") || strings.HasPrefix(ll, "available on") ||
		strings.HasPrefix(ll, "выписка") || strings.HasPrefix(ll, "statement")
}

type kaspiEntry struct {
	row     Row
	op      *kaspiOp
	signed  bool
	details []string
}

// ParseKaspiPages parses the PDF text layer of a Kaspi Gold statement.
func ParseKaspiPages(pages [][]string) ([]Row, error) {
	var (
		entries []*kaspiEntry
		cur     *kaspiEntry
		cont    int
	)
	for _, page := range pages {
		cur = nil // details never continue on the next page: it starts with the bank header
		for _, raw := range page {
			line := strings.TrimSpace(raw)
			if line == "" {
				continue
			}
			if m := kaspiRowRe.FindStringSubmatch(line); m != nil {
				if e := newKaspiEntry(m); e != nil {
					entries = append(entries, e)
					cur, cont = e, 0
					continue
				}
			}
			ll := strings.ToLower(line)
			if isKaspiHeader(line) || isKaspiNoise(ll) || len(findMoney(line)) > 0 {
				cur = nil
				continue
			}
			if cur != nil && cont < 3 {
				cur.details = append(cur.details, line)
				cont++
			}
		}
	}
	rows := make([]Row, 0, len(entries))
	seen := map[string]int{}
	for _, e := range entries {
		r := e.row
		if !e.signed && e.op != nil && e.op.sign < 0 {
			r.AmountCents = -r.AmountCents
		}
		if r.AmountCents == 0 {
			continue
		}
		details := strings.Join(strings.Fields(strings.Join(e.details, " ")), " ")
		opName := ""
		if e.op != nil {
			opName = e.op.names[0]
			r.Category = e.op.category
		}
		r.Description = details
		if details == "" {
			r.Description = opName
		}
		if r.Category == "" {
			r.Category = freedomCategory(details)
		}
		r.Merchant = NormalizeMerchant(kaspiForeignRe.ReplaceAllString(details, ""))
		id := "kaspi|" + r.Currency + "|" + opName + "|" + details
		base := dedupKey(r.Date, r.AmountCents, id, 0)
		r.DedupKey = dedupKey(r.Date, r.AmountCents, id, seen[base])
		seen[base]++
		rows = append(rows, r)
	}
	if len(rows) == 0 {
		return nil, errors.New("kaspi: no transactions found")
	}
	return rows, nil
}

// "(- 9,99 $)": the original amount of a foreign-currency purchase.
var kaspiForeignRe = regexp.MustCompile(`\(\s*[+\-−–]?\s*[\d \x{00a0}]+(?:[.,]\d{1,2})?\s*\S{0,4}\s*\)`)

func newKaspiEntry(m []string) *kaspiEntry {
	d, _, ok := leadingDate(m[1], dayFirst)
	if !ok {
		return nil
	}
	cents, _, ok := parseNumber(m[3])
	if !ok {
		return nil
	}
	e := &kaspiEntry{signed: m[2] != ""}
	if m[2] != "" && m[2] != "+" {
		cents = -cents
	}
	cur := "KZT"
	if m[4] != "" {
		if c, ok := currencySymbols[strings.ToLower(m[4])]; ok {
			cur = c
		} else if isoCodes[m[4]] {
			cur = m[4]
		}
	} else if m[2] == "" {
		return nil // neither a sign nor a currency: not a Kaspi row
	}
	e.row = Row{Date: d, AmountCents: cents, Currency: cur}
	rest := strings.TrimSpace(m[5])
	for i := range kaspiOps {
		for _, name := range kaspiOps[i].names {
			if r, ok := cutPrefixFold(rest, name); ok {
				e.op = &kaspiOps[i]
				rest = strings.TrimSpace(r)
				break
			}
		}
		if e.op != nil {
			break
		}
	}
	if rest != "" {
		e.details = []string{rest}
	}
	return e
}
