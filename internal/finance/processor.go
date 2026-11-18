package finance

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Zifeldev/Billsplit/internal/domain"
	"github.com/Zifeldev/Billsplit/internal/parser/statement"
	"github.com/Zifeldev/Billsplit/internal/platform/fx"
	"github.com/Zifeldev/Billsplit/internal/platform/logger"
	"github.com/Zifeldev/Billsplit/internal/platform/metrics"
)

// Kafka consumers. Both are idempotent (Kafka delivers at-least-once).

// HandleStatementUploaded parses a bank statement and stores new transactions.
func (s *Service) HandleStatementUploaded(ctx context.Context, ev domain.StatementUploadedEvent) error {
	log := logger.FromContext(ctx).With("import_id", ev.ImportID)
	imp, err := s.repo.GetImport(ctx, ev.ImportID)
	if errors.Is(err, domain.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if imp.Status != domain.ImportPending {
		return nil
	}

	rows, err := s.parseStatement(ctx, imp)
	if err != nil {
		if errors.Is(err, errOCRUnavailable) && time.Since(imp.CreatedAt) < ocrRetryWindow {
			return err // transient: let Kafka redeliver; after the window the import fails instead of hanging
		}
		log.Warn("statement parse failed", "err", err)
		return s.repo.FinishImport(ctx, imp.ID, domain.ImportFailed, 0, 0, importErrorMessage(err))
	}
	rules, err := s.repo.Rules(ctx, imp.UserID)
	if err != nil {
		return err
	}
	ts := make([]domain.Transaction, 0, len(rows))
	for _, r := range rows {
		cur := imp.Currency
		if r.Currency != "" {
			cur = r.Currency // multi-currency statements (Freedom Bank) carry a currency per row
		}
		ts = append(ts, domain.Transaction{
			// The parser yields 2-decimal amounts; "1280.00" JPY is ¥1280, not 128000 minor units.
			UserID: imp.UserID, AmountCents: fx.FromTwoDecimals(r.AmountCents, cur), Currency: cur,
			Description: r.Description, Merchant: r.Merchant,
			CategoryID: categorizeRow(rules, r),
			Source:     domain.SourceStatement, DedupKey: r.DedupKey, OccurredAt: r.Date,
		})
	}

	var inserted int
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		var err error
		if inserted, err = s.repo.InsertTransactions(ctx, ts); err != nil {
			return err
		}
		return s.repo.FinishImport(ctx, imp.ID, domain.ImportDone, inserted, len(ts)-inserted, "")
	})
	if err != nil {
		return err
	}
	metrics.StatementRows.WithLabelValues("imported").Add(float64(inserted))
	metrics.StatementRows.WithLabelValues("duplicate").Add(float64(len(ts) - inserted))
	s.bump(ctx, imp.UserID)
	log.Info("statement imported", "rows", len(ts), "inserted", inserted)
	return nil
}

// errOCRUnavailable marks OCR failures worth retrying (service down, 5xx, timeout).
var errOCRUnavailable = errors.New("ocr service unavailable, please retry the import later")

// ocrRetryWindow bounds redeliveries of a PDF import while the OCR service is down.
const ocrRetryWindow = 2 * time.Minute

// parseStatement parses CSV/OFX directly; PDFs are turned into text by the OCR
// service first. Both paths try the bank layouts (Freedom → BCC → Kaspi), then
// (CSV) the US-style CSV, then the generic heuristics.
func (s *Service) parseStatement(ctx context.Context, imp *domain.StatementImport) ([]statement.Row, error) {
	format := statement.Format(imp.Format)
	if format == statement.FormatAuto && statement.IsPDF(imp.File) {
		format = statement.FormatPDF
	}
	if format != statement.FormatPDF {
		return statement.Parse(imp.File, format)
	}
	if s.ocr == nil {
		return nil, errors.New("pdf statements are not enabled")
	}
	pages, err := s.ocr.PDFText(ctx, imp.File)
	if err != nil {
		if errors.Is(err, ErrPDFRejected) {
			return nil, err
		}
		return nil, fmt.Errorf("%w: %v", errOCRUnavailable, err)
	}
	return statement.ParsePages(pages)
}

// importErrorMessage is the text the app shows for a failed import: an
// unrecognized layout gets the user-facing "send us a sample" message.
func importErrorMessage(err error) string {
	if errors.Is(err, statement.ErrUnsupported) {
		return statement.ErrUnrecognized.Error()
	}
	return err.Error()
}

// categorizeRow: the user's own rules win, then the bank's hint (own-account
// transfers, currency exchange), then global rules.
func categorizeRow(rules []domain.CategoryRule, r statement.Row) string {
	if r.Category != "" {
		text := strings.ToLower(r.Merchant + " " + r.Description)
		for _, rule := range rules {
			if rule.UserID != "" && strings.Contains(text, rule.Pattern) {
				return rule.CategoryID
			}
		}
		return r.Category
	}
	return Categorize(rules, r.Merchant, r.Description, r.AmountCents)
}

// HandleReceiptFinalized records each member's share of a split receipt as a personal expense.
func (s *Service) HandleReceiptFinalized(ctx context.Context, ev domain.ReceiptFinalizedEvent) error {
	rules, err := s.repo.Rules(ctx, "")
	if err != nil {
		return err
	}
	category := Categorize(rules, ev.Store, "", -1)
	ts := make([]domain.Transaction, 0, len(ev.Shares))
	for _, sh := range ev.Shares {
		if sh.TotalCents <= 0 {
			continue
		}
		ts = append(ts, domain.Transaction{
			UserID: sh.UserID, AmountCents: -sh.TotalCents, Currency: ev.Currency,
			Description: strings.TrimSpace(fmt.Sprintf("Split receipt %s", ev.Store)), Merchant: strings.ToUpper(ev.Store),
			CategoryID: category, Source: domain.SourceReceipt, DedupKey: "receipt:" + ev.ReceiptID, OccurredAt: ev.OccurredAt,
		})
	}
	if _, err := s.repo.InsertTransactions(ctx, ts); err != nil {
		return err
	}
	for _, t := range ts {
		s.bump(ctx, t.UserID)
	}
	return nil
}
