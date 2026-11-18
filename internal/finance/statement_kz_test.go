package finance

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Zifeldev/Billsplit/internal/domain"
	"github.com/Zifeldev/Billsplit/internal/parser/statement"
)

func fixturePages(t *testing.T, name string) [][]string {
	t.Helper()
	b, err := os.ReadFile("../parser/statement/testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	var pages [][]string
	for _, p := range strings.Split(string(b), "\f\n") {
		pages = append(pages, strings.Split(strings.TrimSuffix(p, "\n"), "\n"))
	}
	return pages
}

func TestHandleStatementUploadedBCC(t *testing.T) {
	repo := &stubRepo{imp: pdfImport(time.Now()), rules: kzRules}
	svc := newStubService(repo, &fakeOCR{pages: fixturePages(t, "bcc_ru_pages.txt")})
	if err := svc.HandleStatementUploaded(context.Background(), domain.StatementUploadedEvent{ImportID: "imp-1"}); err != nil {
		t.Fatal(err)
	}
	if repo.status != domain.ImportDone || len(repo.inserted) != 9 {
		t.Fatalf("status %q (%s), inserted %d", repo.status, repo.errMsg, len(repo.inserted))
	}
	var total int64
	cats := map[string]int{}
	for _, tx := range repo.inserted {
		if tx.Currency != "KZT" { // account currency, not the import's USD
			t.Errorf("currency %q", tx.Currency)
		}
		total += tx.AmountCents
		cats[tx.CategoryID]++
	}
	if total != 25000000-6807050 {
		t.Errorf("total %d", total)
	}
	// Transfers and top-ups hinted by the parser, MAGNUM by the global rule.
	if cats["transfer"] != 4 || cats["groceries"] != 2 {
		t.Errorf("categories %v", cats)
	}
}

func TestHandleStatementUploadedUnknownLayoutMessage(t *testing.T) {
	repo := &stubRepo{imp: pdfImport(time.Now())}
	svc := newStubService(repo, &fakeOCR{pages: [][]string{{"Other Bank", "hello"}}})
	if err := svc.HandleStatementUploaded(context.Background(), domain.StatementUploadedEvent{ImportID: "imp-1"}); err != nil {
		t.Fatal(err)
	}
	if repo.status != domain.ImportFailed || repo.errMsg != statement.ErrUnrecognized.Error() {
		t.Errorf("status %q, message %q", repo.status, repo.errMsg)
	}
}

func TestHandleStatementUploadedGenericCSV(t *testing.T) {
	imp := pdfImport(time.Now())
	imp.Format, imp.Currency = int16(statement.FormatAuto), "KZT"
	imp.File = []byte("Дата;Описание;Сумма\n27.10.2025;MAGNUM ALMATY;-1 500,00\n28.10.2025;Зарплата;250 000,00\n")
	repo := &stubRepo{imp: imp, rules: kzRules}
	if err := newStubService(repo, nil).HandleStatementUploaded(context.Background(), domain.StatementUploadedEvent{ImportID: "imp-1"}); err != nil {
		t.Fatal(err)
	}
	if repo.status != domain.ImportDone || len(repo.inserted) != 2 {
		t.Fatalf("status %q (%s), inserted %d", repo.status, repo.errMsg, len(repo.inserted))
	}
	if tx := repo.inserted[0]; tx.AmountCents != -150000 || tx.CategoryID != "groceries" || tx.Currency != "KZT" {
		t.Errorf("tx %+v", tx)
	}
}
