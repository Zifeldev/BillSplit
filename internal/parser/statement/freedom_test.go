package statement

import (
	"errors"
	"os"
	"strings"
	"testing"
)

// Fixtures are synthetic: they mimic the Freedom Bank layout with invented data.

func loadPages(t *testing.T) [][]string {
	t.Helper()
	b, err := os.ReadFile("testdata/freedom_pdf_pages.txt")
	if err != nil {
		t.Fatal(err)
	}
	var pages [][]string
	for _, p := range strings.Split(string(b), "\f\n") {
		pages = append(pages, strings.Split(strings.TrimSuffix(p, "\n"), "\n"))
	}
	return pages
}

type wantRow struct {
	date     string
	cents    int64
	currency string
	category string
	merchant string
	desc     string
}

var freedomWant = []wantRow{
	{"2026-01-29", -1250, "USD", "", "COFFEE HOUSE 12 SEATTLE", "COFFEE HOUSE 12 SEATTLE US"},
	{"2026-01-28", 10000, "USD", "transfer", "МЕЖДУ СВОИМИ СЧЕТАМИ ПО", "Между своими счетами по курсу 500.5"},
	{"2026-01-28", -5005000, "KZT", "transfer", "МЕЖДУ СВОИМИ СЧЕТАМИ", "Между своими счетами"},
	{"2026-01-27", 2500000, "KZT", "", "ПОПОЛНЕНИЕ. ФИО: ИВАН И..", "Пополнение. ФИО: Иван И.. Референс: ABC123. Перевод по мобильному номеру."},
	{"2026-01-27", -350000, "KZT", "", `ТОО "MAGNUM CASH&CARRY" ЗА`, `ТОО "Magnum Cash&Carry" За оплату /товар заказ 42`},
	{"2026-01-26", -800, "EUR", "", "CAFE CENTRAL WIEN", "CAFE CENTRAL WIEN AT"},
	{"2026-01-26", -1500, "USD", "", "MOVIE PLACE 77 PORTLAND", "MOVIE PLACE 77 PORTLAND US"},
	// The head of this cell ("Иван И. .") ended page 2: it belongs here, not to the row above.
	{"2026-01-25", -200000, "KZT", "", "ИВАН И.", "Иван И. . Безвозмездный перевод"},
	{"2026-01-25", -1500, "USD", "", "MOVIE PLACE 77 PORTLAND", "MOVIE PLACE 77 PORTLAND US"},
	{"2026-01-24", -120000, "KZT", "", "YANDEX GO ALMATY", "YANDEX GO ALMATY KZ"},
	{"2026-01-24", -300, "USD", "", "BUS TICKET SEATTLE", "BUS TICKET SEATTLE US"},
	{"2026-01-24", -300, "USD", "", "BUS TICKET SEATTLE", "BUS TICKET SEATTLE US"},
}

func checkFreedom(t *testing.T, rows []Row) {
	t.Helper()
	if len(rows) != len(freedomWant) {
		for _, r := range rows {
			t.Logf("%+v", r)
		}
		t.Fatalf("rows = %d, want %d (pending row must be skipped)", len(rows), len(freedomWant))
	}
	keys := map[string]bool{}
	for i, w := range freedomWant {
		r := rows[i]
		if r.Date.Format("2006-01-02") != w.date || r.AmountCents != w.cents || r.Currency != w.currency ||
			r.Category != w.category || r.Merchant != w.merchant || r.Description != w.desc {
			t.Errorf("row %d = {%s %d %s %q %q %q}, want %+v", i, r.Date.Format("2006-01-02"), r.AmountCents,
				r.Currency, r.Category, r.Merchant, r.Description, w)
		}
		if r.DedupKey == "" || keys[r.DedupKey] {
			t.Errorf("row %d: empty or duplicate dedup key", i)
		}
		keys[r.DedupKey] = true
	}
}

func TestFreedomPDFPages(t *testing.T) {
	rows, err := ParsePages(loadPages(t))
	if err != nil {
		t.Fatal(err)
	}
	checkFreedom(t, rows)
}

func TestFreedomPDFSingleStream(t *testing.T) {
	// Page boundaries are also recognized from the repeated page header.
	var lines []string
	for _, p := range loadPages(t) {
		lines = append(lines, p...)
	}
	rows, err := ParseLines(lines)
	if err != nil {
		t.Fatal(err)
	}
	checkFreedom(t, rows)
}

func TestFreedomPDFWithoutSoftWraps(t *testing.T) {
	// OCR of a scanned statement loses trailing spaces: wrapped lines then
	// simply continue the row above (page-split cells can't be told apart).
	var lines []string
	for _, p := range loadPages(t) {
		for _, l := range p {
			lines = append(lines, strings.TrimRight(l, " "))
		}
	}
	rows, err := ParseLines(lines)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != len(freedomWant) {
		t.Fatalf("rows = %d", len(rows))
	}
	if rows[6].Description != "MOVIE PLACE 77 PORTLAND US Иван И. ." || rows[1].Description != freedomWant[1].desc {
		t.Errorf("rows = %+v / %+v", rows[6], rows[1])
	}
}

func TestFreedomCSV(t *testing.T) {
	data, err := os.ReadFile("testdata/freedom.csv")
	if err != nil {
		t.Fatal(err)
	}
	rows, err := Parse(data, FormatAuto)
	if err != nil {
		t.Fatal(err)
	}
	checkFreedom(t, rows)

	// The CSV export and the PDF of the same statement produce the same dedup keys.
	pdf, err := ParsePages(loadPages(t))
	if err != nil {
		t.Fatal(err)
	}
	for i := range rows {
		if rows[i].DedupKey != pdf[i].DedupKey {
			t.Errorf("row %d: csv key %s != pdf key %s", i, rows[i].DedupKey, pdf[i].DedupKey)
		}
	}
}

func TestFreedomPageSplitContinuation(t *testing.T) {
	// A long cell split by a page break: its head ends page 1, the rest is
	// top-aligned next to the dated row on page 2.
	csvData := "Date,The amount,Currency,Operation,Details\n" +
		"10.01.2026,-1.00 $,USD,Acquiring,SHOP ONE US\n" +
		",,,,Пополнение. ФИО: Петр П..\n" +
		",,,,Мобильный: . Референс:\n" +
		",You can check the authenticity of the certificate,,,\n" +
		"Date,The amount,Currency,Operation,Details\n" +
		"09.01.2026,\"+5,000.00 ₸\",KZT,Replenishment,XYZ1. Перевод по\n" +
		",,,,мобильному номеру на счет\n" +
		",,,,C2C.\n" +
		",,,,Анна А. .\n" +
		"09.01.2026,\"+1,000.00 ₸\",KZT,Replenishment,\n" +
		",,,,Безвозмездный перевод\n" +
		"08.01.2026,-2.00 $,USD,Acquiring,SHOP TWO US\n"
	pdfPages := [][]string{
		{"Date The amount Currency Operation Details",
			"10.01.2026 -1.00 $ USD Acquiring SHOP ONE US",
			"Пополнение. ФИО: Петр П.. ",
			"Мобильный: . Референс: "},
		{"You can check the authenticity of the certificate",
			"https://bankffin.kz/en/check-receipt",
			"Date The amount Currency Operation Details",
			"09.01.2026 +5,000.00 ₸ KZT Replenishment XYZ1. Перевод по ",
			"мобильному номеру на счет ",
			"C2C.",
			"09.01.2026 +1,000.00 ₸ KZT Replenishment Анна А. . ",
			"Безвозмездный перевод",
			"08.01.2026 -2.00 $ USD Acquiring SHOP TWO US"},
	}
	want := []string{"SHOP ONE US",
		"Пополнение. ФИО: Петр П.. Мобильный: . Референс: XYZ1. Перевод по мобильному номеру на счет C2C.",
		"Анна А. . Безвозмездный перевод", "SHOP TWO US"}

	csvRows, err := ParseCSV([]byte(csvData))
	if err != nil {
		t.Fatal(err)
	}
	pdfRows, err := ParsePages(pdfPages)
	if err != nil {
		t.Fatal(err)
	}
	for name, rows := range map[string][]Row{"csv": csvRows, "pdf": pdfRows} {
		if len(rows) != len(want) {
			t.Fatalf("%s: rows = %d", name, len(rows))
		}
		for i, w := range want {
			if rows[i].Description != w {
				t.Errorf("%s row %d = %q, want %q", name, i, rows[i].Description, w)
			}
			if rows[i].DedupKey != csvRows[i].DedupKey {
				t.Errorf("%s row %d: dedup key differs from csv", name, i)
			}
		}
	}
	if pdfRows[1].AmountCents != 500000 || pdfRows[1].Currency != "KZT" {
		t.Errorf("row 1 = %+v", pdfRows[1])
	}
}

func TestFreedomDedupStableAcrossOverlappingExports(t *testing.T) {
	full, err := ParsePages(loadPages(t))
	if err != nil {
		t.Fatal(err)
	}
	// A later export covering only the last page sees the same rows with the same keys.
	pages := loadPages(t)
	part, err := ParsePages([][]string{pages[0], pages[2]})
	if err != nil {
		t.Fatal(err)
	}
	if len(part) != 5 {
		t.Fatalf("rows = %d", len(part))
	}
	// Except the first row, whose split head was on the dropped page.
	for i := 1; i < len(part); i++ {
		if part[i].DedupKey != full[len(full)-len(part)+i].DedupKey {
			t.Errorf("row %d key changed", i)
		}
	}
	// Two identical purchases on the same day stay distinct.
	if part[3].DedupKey == part[4].DedupKey {
		t.Error("identical rows share a key")
	}
}

func TestFreedomUnknownPDF(t *testing.T) {
	// Not a Freedom statement: the generic parser takes over.
	rows, err := ParseLines([]string{"Some Other Bank", "01/02/2026 coffee 3.00"})
	if err != nil || len(rows) != 1 || rows[0].AmountCents != -300 {
		t.Fatalf("rows = %+v, err = %v", rows, err)
	}
	if _, err := ParseLines([]string{"Some Other Bank", "hello"}); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("expected unsupported layout error, got %v", err)
	}
}

func TestDetectPDF(t *testing.T) {
	if Detect([]byte("%PDF-1.7\n...")) != FormatPDF || Detect([]byte("Date,Amount\n")) != FormatCSV {
		t.Fatal("detect")
	}
	if _, err := Parse([]byte("%PDF-1.7"), FormatAuto); err == nil {
		t.Fatal("raw pdf bytes must not be parsed")
	}
}

func TestSplitFBOp(t *testing.T) {
	cases := []struct{ in, op, det, tail string }{
		{"Acquiring CHIPOTLE 1 US", "Acquiring", "CHIPOTLE 1 US", ""},
		{"Pending", "Pending amount", "", "amount"},
		{"Pending amount SHOP", "Pending amount", "SHOP", ""},
		{"Credit payment", "Credit payment", "", ""},
		{"Payment", "Payment", "", ""},
		{"Unknown thing", "", "Unknown thing", ""},
	}
	for _, c := range cases {
		op, det, tail := splitFBOp(c.in)
		if op != c.op || det != c.det || tail != c.tail {
			t.Errorf("splitFBOp(%q) = %q,%q,%q", c.in, op, det, tail)
		}
	}
}
