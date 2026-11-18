package split

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/Zifeldev/Billsplit/internal/clients"
	"github.com/Zifeldev/Billsplit/internal/domain"
	"github.com/Zifeldev/Billsplit/internal/platform/auth"
	"github.com/Zifeldev/Billsplit/internal/platform/broker"
	"github.com/Zifeldev/Billsplit/internal/platform/fx"
	"github.com/Zifeldev/Billsplit/internal/platform/logger"
	"github.com/Zifeldev/Billsplit/internal/platform/metrics"
	"github.com/Zifeldev/Billsplit/internal/platform/realtime"
)

const maxImageBytes = 8 << 20

type TxManager interface {
	WithinTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// FX is satisfied by *fx.Client.
type FX interface {
	Validate(ctx context.Context, code, def string) (string, error)
	Rate(ctx context.Context, from, to string, date time.Time) (float64, error)
}

type Service struct {
	receipts ReceiptRepository
	ledger   LedgerRepository
	tx       TxManager
	groups   clients.Groups
	users    clients.Users
	pub      broker.Publisher
	fx       FX
	rt       realtime.Publisher
}

func NewService(r ReceiptRepository, l LedgerRepository, tx TxManager, groups clients.Groups, users clients.Users,
	pub broker.Publisher, fx FX) *Service {
	return &Service{receipts: r, ledger: l, tx: tx, groups: groups, users: users, pub: pub, fx: fx, rt: realtime.Nop{}}
}

// WithRealtime pushes receipt changes to clients that have the receipt open.
func (s *Service) WithRealtime(p realtime.Publisher) *Service {
	s.rt = p
	return s
}

// live tells everyone viewing the receipt that it changed (after commit).
func (s *Service) live(ctx context.Context, receiptID, typ, action string, claims []ClaimInput) {
	uid, _ := auth.UserID(ctx)
	ev := realtime.Event{Type: typ, ReceiptID: receiptID, Action: action, UserID: uid}
	for _, c := range claims {
		ev.ItemIDs = append(ev.ItemIDs, c.ItemID)
		ev.Shares = append(ev.Shares, c.Shares)
	}
	s.rt.Publish(ctx, realtime.ReceiptChannel(receiptID), ev)
}

// group loads the group from social and checks that the caller is a member.
func (s *Service) group(ctx context.Context, id string) (clients.GroupInfo, string, error) {
	uid, err := auth.UserID(ctx)
	if err != nil {
		return clients.GroupInfo{}, "", err
	}
	g, err := s.groups.GetGroup(ctx, id)
	if err != nil {
		return g, "", err
	}
	if !g.IsMember(uid) {
		return g, "", domain.ErrNotFound
	}
	return g, uid, nil
}

// ================= Receipts =================

type UploadInput struct {
	GroupID  string
	Image    []byte
	RawText  string
	Currency string
	TipCents int64
}

func (s *Service) Upload(ctx context.Context, in UploadInput) (*Receipt, error) {
	g, uid, err := s.group(ctx, in.GroupID)
	if err != nil {
		return nil, err
	}
	if len(in.Image) == 0 && strings.TrimSpace(in.RawText) == "" {
		return nil, fmt.Errorf("%w: image or raw_text is required", domain.ErrInvalidArgument)
	}
	if len(in.Image) > maxImageBytes {
		return nil, fmt.Errorf("%w: image is larger than 8 MB", domain.ErrInvalidArgument)
	}
	if len(in.RawText) > 64<<10 {
		return nil, fmt.Errorf("%w: raw_text is too long", domain.ErrInvalidArgument)
	}
	var contentType string
	if len(in.Image) > 0 {
		// Trust the bytes, not the client.
		contentType = http.DetectContentType(in.Image)
		switch contentType {
		case "image/jpeg", "image/png", "image/webp", "application/pdf": // PDF: e-receipts and scans
		default:
			return nil, fmt.Errorf("%w: unsupported file type %s (use a photo or a PDF)", domain.ErrInvalidArgument, contentType)
		}
	}
	if in.TipCents < 0 {
		return nil, fmt.Errorf("%w: tip must be >= 0", domain.ErrInvalidArgument)
	}
	currency, err := s.fx.Validate(ctx, in.Currency, g.Currency)
	if err != nil {
		return nil, err
	}

	r := &Receipt{Kind: KindReceipt, Receipt: domain.Receipt{GroupID: g.ID, PayerID: uid, Status: domain.ReceiptPending,
		TipCents: in.TipCents, TotalCents: in.TipCents, Currency: currency}}
	file := &domain.ReceiptFile{ContentType: contentType, Data: in.Image, RawText: in.RawText}
	// The event goes through the outbox in the same tx: no receipt is left PENDING without a queued OCR job.
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.receipts.Create(ctx, r, file); err != nil {
			return err
		}
		return s.pub.Publish(ctx, domain.TopicReceiptUploaded, r.ID, domain.ReceiptUploadedEvent{ReceiptID: r.ID})
	})
	if err != nil {
		return nil, err
	}
	logger.FromContext(ctx).Info("receipt uploaded", "receipt_id", r.ID, "group_id", g.ID)
	return r, nil
}

func (s *Service) Get(ctx context.Context, id string) (*Receipt, clients.GroupInfo, error) {
	r, err := s.receipts.GetByID(ctx, id)
	if err != nil {
		return nil, clients.GroupInfo{}, err
	}
	g, _, err := s.group(ctx, r.GroupID)
	if err != nil {
		return nil, g, err
	}
	return r, g, nil
}

func (s *Service) List(ctx context.Context, groupID string, limit, offset int) ([]Receipt, error) {
	if _, _, err := s.group(ctx, groupID); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	return s.receipts.ListByGroup(ctx, groupID, limit, max(offset, 0))
}

type ItemsUpdate struct {
	Items    []domain.ReceiptItem
	Store    string
	TaxCents int64
	TipCents int64
	Currency string
}

// validateItems normalizes items in place and returns their subtotal.
func validateItems(upd *ItemsUpdate) (int64, error) {
	if len(upd.Items) == 0 || len(upd.Items) > 500 {
		return 0, fmt.Errorf("%w: 1..500 items required", domain.ErrInvalidArgument)
	}
	if upd.TaxCents < 0 || upd.TipCents < 0 {
		return 0, fmt.Errorf("%w: tax/tip must be >= 0", domain.ErrInvalidArgument)
	}
	var subtotal int64
	for i := range upd.Items {
		it := &upd.Items[i]
		it.Name = strings.TrimSpace(it.Name)
		if it.Name == "" || len(it.Name) > 200 || it.PriceCents <= 0 || it.PriceCents > maxExpenseCents {
			return 0, fmt.Errorf("%w: item %d needs a name and a positive price", domain.ErrInvalidArgument, i+1)
		}
		if it.Quantity <= 0 {
			it.Quantity = 1
		}
		it.Claims = nil
		subtotal += it.PriceCents
	}
	upd.Store = strings.TrimSpace(upd.Store)
	if len(upd.Store) > 200 {
		return 0, fmt.Errorf("%w: store name is too long", domain.ErrInvalidArgument)
	}
	return subtotal, nil
}

// UpdateItems lets the payer fix OCR mistakes or enter items manually. Claims are reset.
func (s *Service) UpdateItems(ctx context.Context, id string, upd ItemsUpdate) (*Receipt, error) {
	uid, _ := auth.UserID(ctx)
	r, _, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if r.PayerID != uid {
		return nil, fmt.Errorf("%w: only the payer can edit items", domain.ErrForbidden)
	}
	if r.Status == domain.ReceiptFinalized {
		return nil, fmt.Errorf("%w: receipt is finalized", domain.ErrFailedPrecondition)
	}
	subtotal, err := validateItems(&upd)
	if err != nil {
		return nil, err
	}
	if r.Currency, err = s.fx.Validate(ctx, upd.Currency, r.Currency); err != nil {
		return nil, err
	}
	r.Items = upd.Items
	if upd.Store != "" {
		r.Store = upd.Store
	}
	r.SubtotalCents, r.TaxCents, r.TipCents = subtotal, upd.TaxCents, upd.TipCents
	r.TotalCents = subtotal + upd.TaxCents + upd.TipCents
	r.Status, r.ParseError = domain.ReceiptParsed, ""
	if r.OCRProvider == "" {
		r.OCRProvider = "manual"
	}
	if err := s.receipts.ReplaceItems(ctx, r); err != nil {
		return nil, err
	}
	s.live(ctx, id, realtime.ReceiptUpdated, realtime.ActionItems, nil)
	return s.receipts.GetByID(ctx, id)
}

type CreateReceiptInput struct {
	GroupID string
	ItemsUpdate
	PurchasedAt *time.Time
}

// CreateReceipt stores a manually entered itemized receipt (no OCR), ready for claims.
func (s *Service) CreateReceipt(ctx context.Context, in CreateReceiptInput) (*Receipt, error) {
	g, uid, err := s.group(ctx, in.GroupID)
	if err != nil {
		return nil, err
	}
	subtotal, err := validateItems(&in.ItemsUpdate)
	if err != nil {
		return nil, err
	}
	currency, err := s.fx.Validate(ctx, in.Currency, g.Currency)
	if err != nil {
		return nil, err
	}
	r := &Receipt{Kind: KindReceipt, Receipt: domain.Receipt{GroupID: g.ID, PayerID: uid, Store: in.Store,
		Status: domain.ReceiptParsed, Items: in.Items, SubtotalCents: subtotal, TaxCents: in.TaxCents, TipCents: in.TipCents,
		TotalCents: subtotal + in.TaxCents + in.TipCents, Currency: currency, OCRProvider: "manual", PurchasedAt: in.PurchasedAt}}
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.receipts.Create(ctx, r, nil); err != nil {
			return err
		}
		return s.receipts.ReplaceItems(ctx, r)
	})
	if err != nil {
		return nil, err
	}
	logger.FromContext(ctx).Info("manual receipt created", "receipt_id", r.ID, "group_id", g.ID, "items", len(r.Items))
	return s.receipts.GetByID(ctx, r.ID)
}

type ExpenseInput struct {
	GroupID      string
	Description  string
	AmountCents  int64
	Currency     string
	PaidBy       string
	SplitType    SplitType
	Participants []Participant
	OccurredAt   *time.Time
}

// CreateExpense records a single-amount expense split equally / by percent /
// exactly / by shares. It is stored as a receipt of kind EXPENSE with claims
// reproducing the split and finalized right away in the same transaction.
func (s *Service) CreateExpense(ctx context.Context, in ExpenseInput) (*SplitView, error) {
	g, uid, err := s.group(ctx, in.GroupID)
	if err != nil {
		return nil, err
	}
	desc := strings.TrimSpace(in.Description)
	if desc == "" {
		desc = "Expense"
	}
	if len(desc) > 200 {
		return nil, fmt.Errorf("%w: description is too long", domain.ErrInvalidArgument)
	}
	payer := in.PaidBy
	if payer == "" {
		payer = uid
	}
	if !g.IsMember(payer) {
		return nil, fmt.Errorf("%w: paid_by must be a group member", domain.ErrInvalidArgument)
	}
	if in.SplitType == 0 {
		in.SplitType = SplitEqual
	}
	ps := in.Participants
	if len(ps) == 0 && in.SplitType == SplitEqual {
		for _, m := range g.MemberIDs {
			ps = append(ps, Participant{UserID: m})
		}
	}
	for _, p := range ps {
		if p.UserID != "" && !g.IsMember(p.UserID) {
			return nil, fmt.Errorf("%w: participant %s is not a group member", domain.ErrInvalidArgument, p.UserID)
		}
	}
	weights, err := expenseWeights(in.AmountCents, in.SplitType, ps)
	if err != nil {
		return nil, err
	}
	currency, err := s.fx.Validate(ctx, in.Currency, g.Currency)
	if err != nil {
		return nil, err
	}
	when := time.Now().UTC()
	if in.OccurredAt != nil && !in.OccurredAt.IsZero() {
		when = *in.OccurredAt
	}

	r := &Receipt{Kind: KindExpense, Receipt: domain.Receipt{GroupID: g.ID, PayerID: payer, Store: desc,
		Status: domain.ReceiptFinalized, Items: expenseItems(desc, in.AmountCents, weights),
		SubtotalCents: in.AmountCents, TotalCents: in.AmountCents, Currency: currency, OCRProvider: "manual",
		PurchasedAt: &when, CreatedAt: when}}
	v, err := s.view(ctx, r, g.Currency) // fx lookup before the tx
	if err != nil {
		return nil, err
	}
	debts := s.debts(v, g.Currency)
	names := s.Users(ctx, append([]string{uid, payer}, debtors(debts)...))

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.receipts.Create(ctx, r, nil); err != nil {
			return err
		}
		if err := s.receipts.ReplaceItems(ctx, r); err != nil {
			return err
		}
		for _, it := range r.Items {
			for _, c := range it.Claims {
				if err := s.receipts.SetClaim(ctx, it.ID, c.UserID, c.Shares); err != nil {
					return err
				}
			}
		}
		if len(debts) > 0 {
			for i := range debts {
				debts[i].ReceiptID = r.ID
			}
			if err := s.ledger.AddDebts(ctx, debts); err != nil {
				return err
			}
		}
		if err := s.pub.Publish(ctx, domain.TopicReceiptFinalized, r.ID, finalizedEvent(v)); err != nil {
			return err
		}
		return s.notifyExpense(ctx, g, r, v, uid, names)
	})
	if err != nil {
		return nil, err
	}
	logger.FromContext(ctx).Info("expense created", "receipt_id", r.ID, "group_id", g.ID, "split", in.SplitType, "debts", len(debts))
	return v, nil
}

type ClaimInput struct {
	ItemID string
	Shares int32
}

func (s *Service) Claim(ctx context.Context, id string, claims []ClaimInput) (*Receipt, error) {
	uid, _ := auth.UserID(ctx)
	r, _, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if r.Status != domain.ReceiptParsed {
		return nil, fmt.Errorf("%w: receipt must be parsed and not finalized", domain.ErrFailedPrecondition)
	}
	itemIDs := make(map[string]bool, len(r.Items))
	for _, it := range r.Items {
		itemIDs[it.ID] = true
	}
	for _, c := range claims {
		if !itemIDs[c.ItemID] {
			return nil, fmt.Errorf("%w: item %s is not on this receipt", domain.ErrInvalidArgument, c.ItemID)
		}
		if c.Shares < 0 || c.Shares > 100 {
			return nil, fmt.Errorf("%w: shares must be 0..100", domain.ErrInvalidArgument)
		}
	}
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		for _, c := range claims {
			if err := s.receipts.SetClaim(ctx, c.ItemID, uid, c.Shares); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.live(ctx, id, realtime.ReceiptUpdated, realtime.ActionClaim, claims)
	return s.receipts.GetByID(ctx, id)
}

// SplitView is the split in receipt currency plus conversion to the group currency.
type SplitView struct {
	SplitResult
	Receipt       *Receipt
	GroupCurrency string
	Rate          float64
	GroupTotals   map[string]int64 // user -> total in group currency
}

func (s *Service) Split(ctx context.Context, id string) (*SplitView, error) {
	r, g, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.view(ctx, r, g.Currency)
}

func (s *Service) view(ctx context.Context, r *Receipt, groupCurrency string) (*SplitView, error) {
	when := r.CreatedAt
	if r.PurchasedAt != nil {
		when = *r.PurchasedAt
	}
	rate, err := s.fx.Rate(ctx, r.Currency, groupCurrency, when)
	if err != nil {
		return nil, fmt.Errorf("exchange rate %s->%s: %w", r.Currency, groupCurrency, err)
	}
	v := &SplitView{SplitResult: ComputeSplit(&r.Receipt), Receipt: r, GroupCurrency: groupCurrency, Rate: rate, GroupTotals: map[string]int64{}}
	for _, sh := range v.Shares {
		v.GroupTotals[sh.UserID] = fx.Apply(sh.TotalCents, rate, r.Currency, groupCurrency)
	}
	return v, nil
}

// Finalize turns shares into debts toward the payer (in group currency).
func (s *Service) Finalize(ctx context.Context, id string) (*SplitView, error) {
	uid, _ := auth.UserID(ctx)
	r, g, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if r.PayerID != uid {
		return nil, fmt.Errorf("%w: only the payer can finalize", domain.ErrForbidden)
	}
	if r.Status != domain.ReceiptParsed {
		return nil, fmt.Errorf("%w: receipt must be parsed and not finalized", domain.ErrFailedPrecondition)
	}
	v, err := s.view(ctx, r, g.Currency)
	if err != nil {
		return nil, err
	}
	if len(v.UnclaimedItemIDs) > 0 {
		return nil, fmt.Errorf("%w: %d items are not claimed yet", domain.ErrFailedPrecondition, len(v.UnclaimedItemIDs))
	}
	for _, sh := range v.Shares {
		if !g.IsMember(sh.UserID) {
			return nil, fmt.Errorf("%w: a claimant left the group; ask them to unclaim", domain.ErrFailedPrecondition)
		}
	}

	debts := s.debts(v, g.Currency)
	names := s.Users(ctx, []string{r.PayerID})
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.receipts.TransitionStatus(ctx, r.ID, domain.ReceiptParsed, domain.ReceiptFinalized); err != nil {
			return err
		}
		if len(debts) > 0 {
			if err := s.ledger.AddDebts(ctx, debts); err != nil {
				return err
			}
		}
		// Finance records each member's share as a personal expense (in receipt currency).
		if err := s.pub.Publish(ctx, domain.TopicReceiptFinalized, r.ID, finalizedEvent(v)); err != nil {
			return err
		}
		for _, d := range debts {
			body := fmt.Sprintf("You owe %s to %s", formatMoney(d.Amount, d.Currency), displayName(names, r.PayerID))
			if r.Store != "" {
				body += " for " + r.Store
			}
			if err := s.notify(ctx, r.ID, domain.NotificationEvent{UserIDs: []string{d.FromUser},
				Type: domain.NotifyReceiptFinalized, Title: titleOr(g.Name, "Receipt finalized"), Body: body,
				Data: map[string]string{"receipt_id": r.ID, "group_id": r.GroupID}}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	r.Status = domain.ReceiptFinalized
	s.live(ctx, r.ID, realtime.ReceiptUpdated, realtime.ActionFinalized, nil)
	logger.FromContext(ctx).Info("receipt finalized", "receipt_id", r.ID, "debts", len(debts))
	return v, nil
}

// debts turns shares into debts toward the payer (group currency).
func (s *Service) debts(v *SplitView, groupCurrency string) []domain.Debt {
	r := v.Receipt
	var out []domain.Debt
	for _, sh := range v.Shares {
		if amt := v.GroupTotals[sh.UserID]; sh.UserID != r.PayerID && amt > 0 {
			out = append(out, domain.Debt{GroupID: r.GroupID, ReceiptID: r.ID, FromUser: sh.UserID, ToUser: r.PayerID,
				Amount: amt, Currency: groupCurrency})
		}
	}
	return out
}

func debtors(debts []domain.Debt) []string {
	out := make([]string, len(debts))
	for i, d := range debts {
		out[i] = d.FromUser
	}
	return out
}

func finalizedEvent(v *SplitView) domain.ReceiptFinalizedEvent {
	r := v.Receipt
	ev := domain.ReceiptFinalizedEvent{ReceiptID: r.ID, Store: r.Store, Currency: r.Currency, OccurredAt: r.CreatedAt}
	if r.PurchasedAt != nil {
		ev.OccurredAt = *r.PurchasedAt
	}
	for _, sh := range v.Shares {
		ev.Shares = append(ev.Shares, domain.ShareEvent{UserID: sh.UserID, TotalCents: sh.TotalCents})
	}
	return ev
}

// notifyExpense tells everyone involved in a new expense except its author.
func (s *Service) notifyExpense(ctx context.Context, g clients.GroupInfo, r *Receipt, v *SplitView, author string,
	names map[string]clients.UserInfo) error {
	data := map[string]string{"receipt_id": r.ID, "group_id": r.GroupID}
	title := titleOr(g.Name, "New expense")
	total := formatMoney(r.TotalCents, r.Currency)
	if r.PayerID != author {
		body := fmt.Sprintf("%s added %s · %s paid by you", displayName(names, author), r.Store, total)
		if err := s.notify(ctx, r.ID, domain.NotificationEvent{UserIDs: []string{r.PayerID}, Type: domain.NotifyExpenseAdded,
			Title: title, Body: body, Data: data}); err != nil {
			return err
		}
	}
	for _, sh := range v.Shares {
		if sh.UserID == r.PayerID || sh.UserID == author {
			continue
		}
		body := fmt.Sprintf("%s · %s — you owe %s to %s", r.Store, total,
			formatMoney(v.GroupTotals[sh.UserID], v.GroupCurrency), displayName(names, r.PayerID))
		if err := s.notify(ctx, r.ID, domain.NotificationEvent{UserIDs: []string{sh.UserID}, Type: domain.NotifyExpenseAdded,
			Title: title, Body: body, Data: data}); err != nil {
			return err
		}
	}
	return nil
}

// notify queues a user-facing notification through the outbox (call inside a tx).
func (s *Service) notify(ctx context.Context, key string, ev domain.NotificationEvent) error {
	if len(ev.UserIDs) == 0 {
		return nil
	}
	return s.pub.Publish(ctx, domain.TopicNotification, key, ev)
}

// Users resolves names and payment handles in one batch call. Best effort:
// on failure it logs and returns an empty map (responses just lack names).
func (s *Service) Users(ctx context.Context, ids []string) map[string]clients.UserInfo {
	seen := make(map[string]bool, len(ids))
	uniq := make([]string, 0, len(ids))
	for _, id := range ids {
		if id != "" && !seen[id] {
			seen[id] = true
			uniq = append(uniq, id)
		}
	}
	if len(uniq) == 0 {
		return map[string]clients.UserInfo{}
	}
	m, err := s.users.GetUsers(ctx, uniq)
	if err != nil {
		logger.FromContext(ctx).Warn("resolve user names failed", "err", err)
		return map[string]clients.UserInfo{}
	}
	return m
}

func displayName(m map[string]clients.UserInfo, id string) string {
	if u, ok := m[id]; ok && u.Name != "" {
		return u.Name
	}
	return "a friend"
}

func titleOr(s, def string) string {
	if s != "" {
		return s
	}
	return def
}

func (s *Service) Delete(ctx context.Context, id string) error {
	uid, _ := auth.UserID(ctx)
	r, _, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if r.PayerID != uid {
		return fmt.Errorf("%w: only the payer can delete the receipt", domain.ErrForbidden)
	}
	if r.Status == domain.ReceiptFinalized {
		return fmt.Errorf("%w: finalized receipts can't be deleted", domain.ErrFailedPrecondition)
	}
	if err := s.receipts.Delete(ctx, id); err != nil {
		return err
	}
	s.live(ctx, id, realtime.ReceiptDeleted, "", nil)
	return nil
}

// HandleParsed stores OCR results (Kafka consumer). Idempotent.
func (s *Service) HandleParsed(ctx context.Context, ev domain.ReceiptParsedEvent) error {
	r, err := s.receipts.GetByID(ctx, ev.ReceiptID)
	if errors.Is(err, domain.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if r.Status != domain.ReceiptPending {
		return nil // redelivery or already edited manually
	}
	data := map[string]string{"receipt_id": r.ID, "group_id": r.GroupID}
	if !ev.OK {
		err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
			if err := s.receipts.SetStatus(ctx, r.ID, domain.ReceiptFailed, ev.Error); err != nil {
				return err
			}
			return s.notify(ctx, r.ID, domain.NotificationEvent{UserIDs: []string{r.PayerID}, Type: domain.NotifyReceiptParsed,
				Title: "Receipt not recognized", Body: "Couldn't read the receipt — enter items manually", Data: data})
		})
		if err != nil {
			return err
		}
		metrics.ReceiptsParsed.WithLabelValues("failed").Inc()
		s.live(ctx, r.ID, realtime.ReceiptUpdated, realtime.ActionParsed, nil)
		return nil
	}
	r.Store, r.OCRProvider = ev.Store, ev.Provider
	r.Items = r.Items[:0]
	for _, it := range ev.Items {
		r.Items = append(r.Items, domain.ReceiptItem{Name: it.Name, RawName: it.RawName, Code: it.Code, Quantity: it.Quantity, PriceCents: it.PriceCents})
	}
	r.SubtotalCents, r.TaxCents = ev.SubtotalCents, ev.TaxCents
	r.TotalCents = ev.TotalCents + r.TipCents
	r.PurchasedAt = ev.PurchasedAt
	r.Status, r.ParseError = domain.ReceiptParsed, ev.Warning
	store := titleOr(r.Store, "Receipt")
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.receipts.ReplaceItems(ctx, r); err != nil {
			return err
		}
		return s.notify(ctx, r.ID, domain.NotificationEvent{UserIDs: []string{r.PayerID}, Type: domain.NotifyReceiptParsed,
			Title: "Receipt recognized", Body: fmt.Sprintf("%s · %s — tap to claim yours", store, plural(len(r.Items), "item")),
			Data: data})
	})
	if err != nil {
		return err
	}
	metrics.ReceiptsParsed.WithLabelValues("parsed").Inc()
	s.live(ctx, r.ID, realtime.ReceiptUpdated, realtime.ActionParsed, nil)
	logger.FromContext(ctx).Info("receipt parsed", "receipt_id", r.ID, "items", len(r.Items), "provider", ev.Provider)
	return nil
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}

// ================= Balances =================

type Balances struct {
	Currency  string
	Net       map[string]int64
	Transfers []Transfer
}

func (s *Service) Balances(ctx context.Context, groupID string) (*Balances, error) {
	g, _, err := s.group(ctx, groupID)
	if err != nil {
		return nil, err
	}
	net, err := s.ledger.NetBalances(ctx, groupID)
	if err != nil {
		return nil, err
	}
	return &Balances{Currency: g.Currency, Net: net, Transfers: SimplifyDebts(net)}, nil
}

// SettleUp records a payment made outside the app. The caller is either the
// debtor (from, default) or the creditor (to); the other side is notified.
func (s *Service) SettleUp(ctx context.Context, groupID, fromUser, toUser string, amount int64) (*domain.Settlement, error) {
	g, uid, err := s.group(ctx, groupID)
	if err != nil {
		return nil, err
	}
	if amount <= 0 || amount > maxExpenseCents {
		return nil, fmt.Errorf("%w: amount must be positive", domain.ErrInvalidArgument)
	}
	if fromUser == "" {
		fromUser = uid
	}
	if fromUser != uid && toUser != uid {
		return nil, fmt.Errorf("%w: you can only record payments you made or received", domain.ErrForbidden)
	}
	if fromUser == toUser || !g.IsMember(fromUser) || !g.IsMember(toUser) {
		return nil, fmt.Errorf("%w: payer and recipient must be two different group members", domain.ErrInvalidArgument)
	}
	other := toUser
	if uid == toUser {
		other = fromUser
	}
	names := s.Users(ctx, []string{uid})
	money := formatMoney(amount, g.Currency)
	body := fmt.Sprintf("%s paid you %s", displayName(names, uid), money)
	if uid == toUser {
		body = fmt.Sprintf("%s marked your %s payment as received", displayName(names, uid), money)
	}
	st := &domain.Settlement{GroupID: groupID, FromUser: fromUser, ToUser: toUser, Amount: amount, Currency: g.Currency}
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.ledger.CreateSettlement(ctx, st); err != nil {
			return err
		}
		return s.notify(ctx, groupID, domain.NotificationEvent{UserIDs: []string{other}, Type: domain.NotifySettlement,
			Title: titleOr(g.Name, "Payment recorded"), Body: body,
			Data: map[string]string{"group_id": groupID, "settlement_id": st.ID}})
	})
	if err != nil {
		return nil, err
	}
	logger.FromContext(ctx).Info("settlement recorded", "group_id", groupID, "amount", amount)
	return st, nil
}

type MyBalances struct {
	Currency          string
	OwedToMe, IOwe    int64
	Friends           map[string]int64
	FriendsSortedKeys []string
}

// MyBalances converts per-friend nets from all groups into the caller's home currency.
func (s *Service) MyBalances(ctx context.Context) (*MyBalances, error) {
	uid, err := auth.UserID(ctx)
	if err != nil {
		return nil, err
	}
	me, err := s.users.GetUser(ctx, uid)
	if err != nil {
		return nil, err
	}
	pairs, err := s.ledger.PairBalances(ctx, uid)
	if err != nil {
		return nil, err
	}
	out := &MyBalances{Currency: me.Currency, Friends: map[string]int64{}}
	for _, p := range pairs {
		v, err := s.convert(ctx, p.Cents, p.Currency, me.Currency)
		if err != nil {
			return nil, err
		}
		out.Friends[p.Other] += v
	}
	for k, v := range out.Friends {
		if v > 0 {
			out.OwedToMe += v
		} else {
			out.IOwe -= v
		}
		out.FriendsSortedKeys = append(out.FriendsSortedKeys, k)
	}
	sort.Slice(out.FriendsSortedKeys, func(i, j int) bool {
		return abs(out.Friends[out.FriendsSortedKeys[i]]) > abs(out.Friends[out.FriendsSortedKeys[j]])
	})
	return out, nil
}

// UserTotals is the internal API for the finance dashboard.
func (s *Service) UserTotals(ctx context.Context, userID, currency string) (owedToMe, iOwe int64, err error) {
	nets, err := s.ledger.GroupNets(ctx, userID)
	if err != nil {
		return 0, 0, err
	}
	for _, n := range nets {
		v, err := s.convert(ctx, n.Cents, n.Currency, currency)
		if err != nil {
			return 0, 0, err
		}
		if v > 0 {
			owedToMe += v
		} else {
			iOwe -= v
		}
	}
	return owedToMe, iOwe, nil
}

func (s *Service) MemberBalance(ctx context.Context, groupID, userID string) (int64, error) {
	net, err := s.ledger.NetBalances(ctx, groupID)
	if err != nil {
		return 0, err
	}
	return net[userID], nil
}

func (s *Service) GroupHasActivity(ctx context.Context, groupID string) (bool, error) {
	return s.ledger.GroupHasActivity(ctx, groupID)
}

// ReceiptImage is the internal API for the OCR worker.
func (s *Service) ReceiptImage(ctx context.Context, id string) (*domain.ReceiptFile, domain.ReceiptStatus, error) {
	r, err := s.receipts.GetByID(ctx, id)
	if err != nil {
		return nil, 0, err
	}
	f, err := s.receipts.GetFile(ctx, id)
	return f, r.Status, err
}

func (s *Service) convert(ctx context.Context, cents int64, from, to string) (int64, error) {
	if from == to {
		return cents, nil
	}
	rate, err := s.fx.Rate(ctx, from, to, time.Time{})
	if err != nil {
		return 0, err
	}
	return fx.Apply(cents, rate, from, to), nil
}

func abs(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
