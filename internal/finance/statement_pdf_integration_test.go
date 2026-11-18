//go:build integration

package finance

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/Zifeldev/Billsplit/internal/domain"
	"github.com/Zifeldev/Billsplit/internal/parser/statement"
	"github.com/Zifeldev/Billsplit/internal/platform/auth"
)

// End to end: a synthetic Freedom Bank PDF goes through ImportStatement, the
// statement-uploaded handler (with a fake OCR service) and real Postgres with
// all migrations, including the Kazakhstan category rules.
func TestFreedomPDFImportEndToEnd(t *testing.T) {
	p := testPool(t)
	svc := newTestService(p, newMemCache()).WithOCR(&fakeOCR{pages: freedomPages(t)})
	ctx := auth.WithUserID(context.Background(), userA)

	importAndProcess := func(file []byte, format statement.Format) *domain.StatementImport {
		t.Helper()
		imp, err := svc.ImportStatement(ctx, file, int16(format), "freedom", "")
		if err != nil {
			t.Fatal(err)
		}
		if err := svc.HandleStatementUploaded(ctx, domain.StatementUploadedEvent{ImportID: imp.ID, UserID: userA}); err != nil {
			t.Fatal(err)
		}
		got, err := svc.GetImport(ctx, imp.ID)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}

	imp := importAndProcess([]byte("%PDF-1.7\nsynthetic"), statement.FormatAuto)
	if imp.Status != domain.ImportDone || imp.Imported != 12 || imp.Duplicates != 0 || imp.Format != int16(statement.FormatPDF) {
		t.Fatalf("import = %+v", imp)
	}

	txs, err := svc.ListTransactions(ctx, domain.TransactionFilter{
		From: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), To: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), Limit: 200})
	if err != nil {
		t.Fatal(err)
	}
	if len(txs) != 12 {
		t.Fatalf("transactions = %d", len(txs))
	}
	cats := map[string]string{}
	currencies := map[string]int{}
	for _, tx := range txs {
		cats[tx.Description] = tx.CategoryID
		currencies[tx.Currency]++
	}
	if currencies["USD"] != 6 || currencies["KZT"] != 5 || currencies["EUR"] != 1 {
		t.Errorf("currencies = %v", currencies)
	}
	want := map[string]string{
		"Между своими счетами":                              "transfer",
		"Иван И. . Безвозмездный перевод":                   "transfer",  // 00004 rule
		`ТОО "Magnum Cash&Carry" За оплату /товар заказ 42`: "groceries", // 00004 rule
		"YANDEX GO ALMATY KZ":        "transport", // 00004 rule
		"MOVIE PLACE 77 PORTLAND US": "other",
	}
	for desc, cat := range want {
		if cats[desc] != cat {
			t.Errorf("%q: category %q, want %q", desc, cats[desc], cat)
		}
	}

	// Re-importing the same statement — as PDF or as the bank's CSV export — adds nothing.
	again := importAndProcess([]byte("%PDF-1.7\nsynthetic again"), statement.FormatPDF)
	if again.Imported != 0 || again.Duplicates != 12 {
		t.Errorf("pdf re-import = %+v", again)
	}
	csvData, err := os.ReadFile("../parser/statement/testdata/freedom.csv")
	if err != nil {
		t.Fatal(err)
	}
	fromCSV := importAndProcess(csvData, statement.FormatCSV)
	if fromCSV.Status != domain.ImportDone || fromCSV.Imported != 0 || fromCSV.Duplicates != 12 {
		t.Errorf("csv re-import = %+v", fromCSV)
	}
}
