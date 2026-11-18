package finance

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Zifeldev/Billsplit/internal/clients"
	"github.com/Zifeldev/Billsplit/internal/domain"
	"github.com/Zifeldev/Billsplit/internal/parser/statement"
	"github.com/Zifeldev/Billsplit/internal/platform/auth"
	"github.com/Zifeldev/Billsplit/internal/platform/cache"
)

// ---------- unit fakes (names differ from the integration-test fakes) ----------

type stubRepo struct {
	Repository
	imp      *domain.StatementImport
	created  *domain.StatementImport
	rules    []domain.CategoryRule
	inserted []domain.Transaction
	status   string
	errMsg   string
}

func (r *stubRepo) GetImport(context.Context, string) (*domain.StatementImport, error) {
	if r.imp == nil {
		return nil, domain.ErrNotFound
	}
	cp := *r.imp
	return &cp, nil
}

func (r *stubRepo) CreateImport(_ context.Context, imp *domain.StatementImport) error {
	imp.ID, imp.Status = "imp-1", domain.ImportPending
	r.created = imp
	return nil
}

func (r *stubRepo) Rules(context.Context, string) ([]domain.CategoryRule, error) { return r.rules, nil }

func (r *stubRepo) InsertTransactions(_ context.Context, ts []domain.Transaction) (int, error) {
	r.inserted = append(r.inserted, ts...)
	return len(ts), nil
}

func (r *stubRepo) FinishImport(_ context.Context, _, status string, _, _ int, errMsg string) error {
	r.status, r.errMsg = status, errMsg
	return nil
}

type stubTx struct{}

func (stubTx) WithinTx(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }

type stubPub struct{ n int }

func (p *stubPub) Publish(context.Context, string, string, any) error { p.n++; return nil }

type stubUsers struct{ clients.Users }

func (stubUsers) GetUser(_ context.Context, id string) (clients.UserInfo, error) {
	return clients.UserInfo{ID: id, Currency: "USD"}, nil
}

type stubFX struct{ FX }

func (stubFX) Validate(_ context.Context, code, def string) (string, error) {
	if code == "" {
		return def, nil
	}
	return strings.ToUpper(code), nil
}

type stubCache struct{ cache.Cache }

func (stubCache) Incr(context.Context, string) (int64, error) { return 1, nil }

type fakeOCR struct {
	pages [][]string
	err   error
	calls int
}

func (f *fakeOCR) PDFText(_ context.Context, pdf []byte) ([][]string, error) {
	f.calls++
	if !statement.IsPDF(pdf) {
		return nil, errors.New("not a pdf")
	}
	return f.pages, f.err
}

func freedomPages(t *testing.T) [][]string {
	t.Helper()
	b, err := os.ReadFile("../parser/statement/testdata/freedom_pdf_pages.txt")
	if err != nil {
		t.Fatal(err)
	}
	var pages [][]string
	for _, p := range strings.Split(string(b), "\f\n") {
		pages = append(pages, strings.Split(strings.TrimSuffix(p, "\n"), "\n"))
	}
	return pages
}

func newStubService(repo *stubRepo, ocr PDFTexter) *Service {
	s := NewService(repo, stubTx{}, stubUsers{}, nil, &stubPub{}, stubFX{}, stubCache{}, nil, time.Minute)
	if ocr != nil {
		s.WithOCR(ocr)
	}
	return s
}

var kzRules = []domain.CategoryRule{
	{Pattern: "безвозмездный перевод", CategoryID: "transfer"},
	{Pattern: "magnum", CategoryID: "groceries"},
	{Pattern: "yandex go", CategoryID: "transport"},
	{Pattern: "coffee", CategoryID: "dining"},
}

func pdfImport(created time.Time) *domain.StatementImport {
	return &domain.StatementImport{ID: "imp-1", UserID: "u1", Status: domain.ImportPending,
		Format: int16(statement.FormatPDF), Currency: "USD", File: []byte("%PDF-1.7 synthetic"), CreatedAt: created}
}

// ---------- tests ----------

func TestHandleStatementUploadedPDF(t *testing.T) {
	repo := &stubRepo{imp: pdfImport(time.Now()), rules: kzRules}
	ocr := &fakeOCR{pages: freedomPages(t)}
	svc := newStubService(repo, ocr)
	if err := svc.HandleStatementUploaded(context.Background(), domain.StatementUploadedEvent{ImportID: "imp-1", UserID: "u1"}); err != nil {
		t.Fatal(err)
	}
	if ocr.calls != 1 || repo.status != domain.ImportDone {
		t.Fatalf("ocr calls %d, status %q (%s)", ocr.calls, repo.status, repo.errMsg)
	}
	if len(repo.inserted) != 12 {
		t.Fatalf("inserted %d, want 12 (pending row skipped)", len(repo.inserted))
	}
	byDesc := map[string]domain.Transaction{}
	perCurrency := map[string]int64{}
	for _, tx := range repo.inserted {
		byDesc[tx.Description] = tx
		perCurrency[tx.Currency] += tx.AmountCents
		if tx.Source != domain.SourceStatement || tx.DedupKey == "" || tx.UserID != "u1" {
			t.Errorf("bad tx %+v", tx)
		}
	}
	// Each row keeps its own currency; KZT has 2 decimals.
	if perCurrency["USD"] != -1250+10000-1500-1500-300-300 || perCurrency["KZT"] != -5005000+2500000-350000-200000-120000 ||
		perCurrency["EUR"] != -800 {
		t.Errorf("per-currency totals %v", perCurrency)
	}
	checks := map[string]string{
		"Между своими счетами по курсу 500.5":               "transfer", // bank hint (own accounts / conversion)
		"Между своими счетами":                              "transfer",
		"Иван И. . Безвозмездный перевод":                   "transfer", // global rule
		`ТОО "Magnum Cash&Carry" За оплату /товар заказ 42`: "groceries",
		"YANDEX GO ALMATY KZ":        "transport",
		"COFFEE HOUSE 12 SEATTLE US": "dining",
		"CAFE CENTRAL WIEN AT":       "other",
		"Пополнение. ФИО: Иван И.. Референс: ABC123. Перевод по мобильному номеру.": "income",
	}
	for desc, cat := range checks {
		tx, ok := byDesc[desc]
		if !ok {
			t.Errorf("missing %q", desc)
			continue
		}
		if tx.CategoryID != cat {
			t.Errorf("%q: category %q, want %q", desc, tx.CategoryID, cat)
		}
	}
	if tx := byDesc["CAFE CENTRAL WIEN AT"]; tx.Currency != "EUR" || tx.AmountCents != -800 || tx.Merchant != "CAFE CENTRAL WIEN" {
		t.Errorf("eur row %+v", tx)
	}
}

func TestHandleStatementUploadedPDFAutoDetected(t *testing.T) {
	imp := pdfImport(time.Now())
	imp.Format = int16(statement.FormatAuto) // stored before detection existed
	repo := &stubRepo{imp: imp}
	svc := newStubService(repo, &fakeOCR{pages: freedomPages(t)})
	if err := svc.HandleStatementUploaded(context.Background(), domain.StatementUploadedEvent{ImportID: "imp-1"}); err != nil {
		t.Fatal(err)
	}
	if repo.status != domain.ImportDone || len(repo.inserted) != 12 {
		t.Fatalf("status %q, inserted %d", repo.status, len(repo.inserted))
	}
}

func TestHandleStatementUploadedOCRErrors(t *testing.T) {
	ctx := context.Background()
	ev := domain.StatementUploadedEvent{ImportID: "imp-1"}

	// Rejected PDF: permanent failure.
	repo := &stubRepo{imp: pdfImport(time.Now())}
	if err := newStubService(repo, &fakeOCR{err: ErrPDFRejected}).HandleStatementUploaded(ctx, ev); err != nil {
		t.Fatal(err)
	}
	if repo.status != domain.ImportFailed {
		t.Errorf("rejected: status %q", repo.status)
	}

	// OCR down on a fresh import: error so Kafka redelivers, import stays pending.
	repo = &stubRepo{imp: pdfImport(time.Now())}
	if err := newStubService(repo, &fakeOCR{err: errors.New("connection refused")}).HandleStatementUploaded(ctx, ev); err == nil {
		t.Error("transient OCR error must be retried")
	}
	if repo.status != "" {
		t.Errorf("transient: status %q", repo.status)
	}

	// OCR still down after the retry window: fail instead of hanging forever.
	repo = &stubRepo{imp: pdfImport(time.Now().Add(-time.Hour))}
	if err := newStubService(repo, &fakeOCR{err: errors.New("connection refused")}).HandleStatementUploaded(ctx, ev); err != nil {
		t.Fatal(err)
	}
	if repo.status != domain.ImportFailed || !strings.Contains(repo.errMsg, "ocr") {
		t.Errorf("expired: status %q, %q", repo.status, repo.errMsg)
	}

	// A PDF that isn't a Freedom Bank statement.
	repo = &stubRepo{imp: pdfImport(time.Now())}
	if err := newStubService(repo, &fakeOCR{pages: [][]string{{"Other Bank", "hello"}}}).HandleStatementUploaded(ctx, ev); err != nil {
		t.Fatal(err)
	}
	if repo.status != domain.ImportFailed {
		t.Errorf("unknown layout: status %q", repo.status)
	}

	// PDF support not configured.
	repo = &stubRepo{imp: pdfImport(time.Now())}
	if err := newStubService(repo, nil).HandleStatementUploaded(ctx, ev); err != nil {
		t.Fatal(err)
	}
	if repo.status != domain.ImportFailed {
		t.Errorf("no ocr: status %q", repo.status)
	}
}

func TestImportStatementFormatsAndLimits(t *testing.T) {
	ctx := auth.WithUserID(context.Background(), "u1")
	pdf := append([]byte("%PDF-1.7\n"), make([]byte, 6<<20)...) // 6 MB: over the CSV limit, fine for PDF
	cases := []struct {
		name    string
		file    []byte
		format  int16
		wantErr bool
		want    statement.Format
	}{
		{"pdf auto-detected", pdf, 0, false, statement.FormatPDF},
		{"pdf explicit", []byte("%PDF-1.4 x"), int16(statement.FormatPDF), false, statement.FormatPDF},
		{"pdf format but not a pdf", []byte("Date,Amount\n"), int16(statement.FormatPDF), true, 0},
		{"csv stays auto", []byte("Date,Description,Amount\n"), 0, false, statement.FormatAuto},
		{"csv too big", make([]byte, 6<<20), int16(statement.FormatCSV), true, 0},
		{"pdf too big", append([]byte("%PDF-1.7\n"), make([]byte, 16<<20)...), 0, true, 0},
		{"unknown format", []byte("x"), 9, true, 0},
	}
	for _, c := range cases {
		repo := &stubRepo{}
		imp, err := newStubService(repo, nil).ImportStatement(ctx, c.file, c.format, "freedom", "")
		if c.wantErr {
			if !errors.Is(err, domain.ErrInvalidArgument) {
				t.Errorf("%s: err = %v, want invalid argument", c.name, err)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if statement.Format(repo.created.Format) != c.want || imp.File != nil || imp.Currency != "USD" {
			t.Errorf("%s: stored format %d", c.name, repo.created.Format)
		}
	}
}

func TestCategorizeRowUserRuleBeatsHint(t *testing.T) {
	r := statement.Row{Merchant: "МЕЖДУ СВОИМИ СЧЕТАМИ", Description: "Между своими счетами", AmountCents: -100, Category: "transfer"}
	if got := categorizeRow(nil, r); got != "transfer" {
		t.Errorf("hint: %q", got)
	}
	global := []domain.CategoryRule{{Pattern: "между своими", CategoryID: "savings"}}
	if got := categorizeRow(global, r); got != "transfer" {
		t.Errorf("global rule must not beat the bank hint: %q", got)
	}
	user := []domain.CategoryRule{{Pattern: "между своими", CategoryID: "savings", UserID: "u1"}}
	if got := categorizeRow(user, r); got != "savings" {
		t.Errorf("user rule: %q", got)
	}
}

func TestOCRClient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		switch {
		case r.URL.Path != "/pdf/text" || r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/pdf":
			http.Error(w, "bad request", http.StatusBadRequest)
		case string(body) == "%PDF-broken":
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"detail":"cannot open pdf"}`))
		case string(body) == "%PDF-crash":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"detail":"boom"}`))
		default:
			_, _ = w.Write([]byte(`{"pages":[{"lines":["a ","b"]},{"lines":["c"]}],"text":"a \nb\nc"}`))
		}
	}))
	defer srv.Close()
	c := NewOCRClient(srv.URL + "/")
	ctx := context.Background()

	pages, err := c.PDFText(ctx, []byte("%PDF-ok"))
	if err != nil || len(pages) != 2 || pages[0][0] != "a " || pages[1][0] != "c" {
		t.Fatalf("pages = %q, %v", pages, err)
	}
	if _, err := c.PDFText(ctx, []byte("%PDF-broken")); !errors.Is(err, ErrPDFRejected) || !strings.Contains(err.Error(), "cannot open pdf") {
		t.Errorf("4xx: %v", err)
	}
	if _, err := c.PDFText(ctx, []byte("%PDF-crash")); err == nil || errors.Is(err, ErrPDFRejected) {
		t.Errorf("5xx must be transient: %v", err)
	}
}
