package statement

// Layout detection and dispatch. Order: Freedom Bank → Bank CenterCredit →
// Kaspi Bank → (CSV only: generic US CSV; OFX is detected from the file) →
// generic heuristics. Among the bank layouts the one whose marker appears
// first in the text wins (ties: Freedom, BCC, Kaspi).

import (
	"bytes"
	"encoding/csv"
	"io"
	"strings"
)

// Bank is a recognized statement layout.
type Bank string

const (
	BankGeneric Bank = "generic"
	BankFreedom Bank = "freedom"
	BankBCC     Bank = "bcc"
	BankKaspi   Bank = "kaspi"
)

type layout struct {
	bank   Bank
	marker func([][]string) int // line index of the first marker, -1 if none
	parse  func([][]string) ([]Row, error)
}

var layouts = []layout{
	{BankFreedom, freedomMarker, ParseFreedomPages},
	{BankBCC, bccMarker, ParseBCCPages},
	{BankKaspi, kaspiMarker, ParseKaspiPages},
}

// detectLayout returns the layout whose marker (bank name, site, BIC or table
// header) appears first in the text; on the same line the order above wins.
func detectLayout(pages [][]string) *layout {
	var best *layout
	bestPos := -1
	for i := range layouts {
		if p := layouts[i].marker(pages); p >= 0 && (bestPos < 0 || p < bestPos) {
			best, bestPos = &layouts[i], p
		}
	}
	return best
}

// DetectBank tells which bank-specific layout the statement text matches;
// BankGeneric when none does.
func DetectBank(pages [][]string) Bank {
	if l := detectLayout(pages); l != nil {
		return l.bank
	}
	return BankGeneric
}

// ParseLines parses the text of a statement given as one stream of lines.
func ParseLines(lines []string) ([]Row, error) { return ParsePages([][]string{lines}) }

// ParsePages parses the text of a PDF statement, one slice of visual lines per
// page (as returned by the OCR service). A bank-specific layout is tried first;
// if it is not recognized or yields nothing, the generic heuristics are used.
// When nothing is found the error is ErrUnrecognized.
func ParsePages(pages [][]string) ([]Row, error) {
	if l := detectLayout(pages); l != nil {
		if rows, err := l.parse(pages); err == nil && len(rows) > 0 {
			return rows, nil
		}
	}
	return ParseGenericPages(pages)
}

// parseCSVAny: Freedom CSV → BCC/Kaspi table dumped to CSV → US-style CSV → generic.
func parseCSVAny(data []byte) ([]Row, error) {
	if isFreedomCSV(data) {
		return ParseFreedomCSV(data)
	}
	pages := [][]string{csvTextLines(data, " ")}
	if l := detectLayout(pages); l != nil && l.bank != BankFreedom {
		if rows, err := l.parse(pages); err == nil && len(rows) > 0 {
			return rows, nil
		}
	}
	if rows, err := ParseCSV(data); err == nil && len(rows) > 0 {
		return rows, nil
	}
	return ParseGenericPages([][]string{csvTextLines(data, " | ")})
}

// csvTextLines turns a CSV/TSV/semicolon file into text lines, cells joined
// with sep, for the text parsers. The delimiter is sniffed from the file head.
func csvTextLines(data []byte, sep string) []string {
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	head := string(data[:min(len(data), 2048)])
	delim := ','
	best := strings.Count(head, ",")
	for _, d := range []rune{';', '\t'} {
		if n := strings.Count(head, string(d)); n > best {
			delim, best = d, n
		}
	}
	r := csv.NewReader(bytes.NewReader(data))
	r.Comma = delim
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	var lines []string
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}
		var cells []string
		for _, c := range rec {
			if c = strings.TrimSpace(c); c != "" {
				cells = append(cells, c)
			}
		}
		lines = append(lines, strings.Join(cells, sep))
	}
	return lines
}
