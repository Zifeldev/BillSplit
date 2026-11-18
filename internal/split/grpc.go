package split

import (
	"context"
	"sort"

	"github.com/Zifeldev/Billsplit/internal/clients"
	"github.com/Zifeldev/Billsplit/internal/domain"
	"github.com/Zifeldev/Billsplit/internal/platform/grpcx"
	ipb "github.com/Zifeldev/Billsplit/pkg/pb/billsplit/internalapi/v1"
	pb "github.com/Zifeldev/Billsplit/pkg/pb/billsplit/v1"
)

// Handler implements ReceiptService, BalanceService and the internal SplitInternal.
type Handler struct {
	pb.UnimplementedReceiptServiceServer
	pb.UnimplementedBalanceServiceServer
	ipb.UnimplementedSplitInternalServer
	svc *Service
}

func NewHandler(s *Service) *Handler { return &Handler{svc: s} }

func (h *Handler) UploadReceipt(ctx context.Context, req *pb.UploadReceiptRequest) (*pb.Receipt, error) {
	r, err := h.svc.Upload(ctx, UploadInput{GroupID: req.GetGroupId(), Image: req.GetImage(), RawText: req.GetRawText(),
		Currency: req.GetCurrency(), TipCents: req.GetTipCents()})
	if err != nil {
		return nil, grpcx.ToStatus(ctx, err)
	}
	return h.receipt(ctx, r), nil
}

func (h *Handler) CreateReceipt(ctx context.Context, req *pb.CreateReceiptRequest) (*pb.Receipt, error) {
	in := CreateReceiptInput{GroupID: req.GetGroupId(), ItemsUpdate: ItemsUpdate{Items: itemsIn(req.GetItems()),
		Store: req.GetStore(), TaxCents: req.GetTaxCents(), TipCents: req.GetTipCents(), Currency: req.GetCurrency()}}
	if req.GetPurchasedAt() != nil {
		t := req.GetPurchasedAt().AsTime()
		in.PurchasedAt = &t
	}
	r, err := h.svc.CreateReceipt(ctx, in)
	if err != nil {
		return nil, grpcx.ToStatus(ctx, err)
	}
	return h.receipt(ctx, r), nil
}

func (h *Handler) CreateExpense(ctx context.Context, req *pb.CreateExpenseRequest) (*pb.Split, error) {
	in := ExpenseInput{GroupID: req.GetGroupId(), Description: req.GetDescription(), AmountCents: req.GetAmountCents(),
		Currency: req.GetCurrency(), PaidBy: req.GetPaidBy(), SplitType: SplitType(req.GetSplitType())}
	for _, p := range req.GetParticipants() {
		in.Participants = append(in.Participants, Participant{UserID: p.GetUserId(), Value: p.GetValue()})
	}
	if req.GetOccurredAt() != nil {
		t := req.GetOccurredAt().AsTime()
		in.OccurredAt = &t
	}
	v, err := h.svc.CreateExpense(ctx, in)
	if err != nil {
		return nil, grpcx.ToStatus(ctx, err)
	}
	return h.split(ctx, v), nil
}

func (h *Handler) GetReceipt(ctx context.Context, req *pb.GetReceiptRequest) (*pb.Receipt, error) {
	r, _, err := h.svc.Get(ctx, req.GetReceiptId())
	if err != nil {
		return nil, grpcx.ToStatus(ctx, err)
	}
	return h.receipt(ctx, r), nil
}

func (h *Handler) ListReceipts(ctx context.Context, req *pb.ListReceiptsRequest) (*pb.ListReceiptsResponse, error) {
	rs, err := h.svc.List(ctx, req.GetGroupId(), int(req.GetLimit()), int(req.GetOffset()))
	if err != nil {
		return nil, grpcx.ToStatus(ctx, err)
	}
	payers := make([]string, len(rs))
	for i := range rs {
		payers[i] = rs[i].PayerID
	}
	users := h.svc.Users(ctx, payers)
	out := &pb.ListReceiptsResponse{Receipts: make([]*pb.Receipt, len(rs))}
	for i := range rs {
		out.Receipts[i] = receiptPB(&rs[i], users)
	}
	return out, nil
}

func (h *Handler) UpdateItems(ctx context.Context, req *pb.UpdateItemsRequest) (*pb.Receipt, error) {
	upd := ItemsUpdate{Items: itemsIn(req.GetItems()), Store: req.GetStore(), TaxCents: req.GetTaxCents(),
		TipCents: req.GetTipCents(), Currency: req.GetCurrency()}
	r, err := h.svc.UpdateItems(ctx, req.GetReceiptId(), upd)
	if err != nil {
		return nil, grpcx.ToStatus(ctx, err)
	}
	return h.receipt(ctx, r), nil
}

func (h *Handler) ClaimItems(ctx context.Context, req *pb.ClaimItemsRequest) (*pb.Receipt, error) {
	claims := make([]ClaimInput, 0, len(req.GetClaims()))
	for _, c := range req.GetClaims() {
		claims = append(claims, ClaimInput{ItemID: c.GetItemId(), Shares: c.GetShares()})
	}
	r, err := h.svc.Claim(ctx, req.GetReceiptId(), claims)
	if err != nil {
		return nil, grpcx.ToStatus(ctx, err)
	}
	return h.receipt(ctx, r), nil
}

func (h *Handler) GetSplit(ctx context.Context, req *pb.GetSplitRequest) (*pb.Split, error) {
	v, err := h.svc.Split(ctx, req.GetReceiptId())
	if err != nil {
		return nil, grpcx.ToStatus(ctx, err)
	}
	return h.split(ctx, v), nil
}

func (h *Handler) FinalizeReceipt(ctx context.Context, req *pb.FinalizeReceiptRequest) (*pb.Split, error) {
	v, err := h.svc.Finalize(ctx, req.GetReceiptId())
	if err != nil {
		return nil, grpcx.ToStatus(ctx, err)
	}
	return h.split(ctx, v), nil
}

func (h *Handler) DeleteReceipt(ctx context.Context, req *pb.DeleteReceiptRequest) (*pb.DeleteReceiptResponse, error) {
	if err := h.svc.Delete(ctx, req.GetReceiptId()); err != nil {
		return nil, grpcx.ToStatus(ctx, err)
	}
	return &pb.DeleteReceiptResponse{}, nil
}

// ---------- balances ----------

func (h *Handler) GetBalances(ctx context.Context, req *pb.GetBalancesRequest) (*pb.GetBalancesResponse, error) {
	b, err := h.svc.Balances(ctx, req.GetGroupId())
	if err != nil {
		return nil, grpcx.ToStatus(ctx, err)
	}
	ids := make([]string, 0, len(b.Net))
	for uid := range b.Net {
		ids = append(ids, uid)
	}
	sort.Strings(ids)
	users := h.svc.Users(ctx, ids)
	out := &pb.GetBalancesResponse{Currency: b.Currency}
	for _, uid := range ids {
		out.Balances = append(out.Balances, &pb.MemberBalance{UserId: uid, NetCents: b.Net[uid], Name: users[uid].Name})
	}
	for _, t := range b.Transfers {
		out.SuggestedTransfers = append(out.SuggestedTransfers, &pb.Transfer{FromUserId: t.From, ToUserId: t.To,
			AmountCents: t.Amount, FromName: users[t.From].Name, ToName: users[t.To].Name, ToPayment: paymentPB(users[t.To])})
	}
	return out, nil
}

func (h *Handler) SettleUp(ctx context.Context, req *pb.SettleUpRequest) (*pb.Settlement, error) {
	s, err := h.svc.SettleUp(ctx, req.GetGroupId(), req.GetFromUserId(), req.GetToUserId(), req.GetAmountCents())
	if err != nil {
		return nil, grpcx.ToStatus(ctx, err)
	}
	return settlementPB(s), nil
}

func (h *Handler) GetFriendLedger(ctx context.Context, req *pb.GetFriendLedgerRequest) (*pb.FriendLedger, error) {
	l, err := h.svc.FriendLedger(ctx, req.GetUserId())
	if err != nil {
		return nil, grpcx.ToStatus(ctx, err)
	}
	out := &pb.FriendLedger{UserId: l.Friend.ID, Name: l.Friend.Name, Payment: paymentPB(l.Friend),
		Currency: l.Currency, NetCents: l.Net, Groups: make([]*pb.LedgerGroup, 0, len(l.Groups))}
	for _, g := range l.Groups {
		pg := &pb.LedgerGroup{GroupId: g.GroupID, GroupName: g.GroupName, Currency: g.Currency, NetCents: g.Net,
			NetHomeCents: g.NetHome, Entries: make([]*pb.LedgerEntry, 0, len(g.Entries))}
		for _, e := range g.Entries {
			pg.Entries = append(pg.Entries, &pb.LedgerEntry{Kind: pb.LedgerEntryKind(e.Kind), ReceiptId: e.ReceiptID,
				Title: e.Title, AmountCents: e.Amount, OccurredAt: grpcx.TS(e.OccurredAt)})
		}
		out.Groups = append(out.Groups, pg)
	}
	return out, nil
}

func (h *Handler) SettleWithFriend(ctx context.Context, req *pb.SettleWithFriendRequest) (*pb.SettleWithFriendResponse, error) {
	r, err := h.svc.SettleWithFriend(ctx, req.GetUserId())
	if err != nil {
		return nil, grpcx.ToStatus(ctx, err)
	}
	out := &pb.SettleWithFriendResponse{NetPaidCents: r.NetPaid, Currency: r.Currency,
		Settlements: make([]*pb.Settlement, 0, len(r.Settlements))}
	for i := range r.Settlements {
		out.Settlements = append(out.Settlements, settlementPB(&r.Settlements[i]))
	}
	return out, nil
}

func settlementPB(s *domain.Settlement) *pb.Settlement {
	return &pb.Settlement{Id: s.ID, GroupId: s.GroupID, FromUserId: s.FromUser, ToUserId: s.ToUser,
		AmountCents: s.Amount, Currency: s.Currency, CreatedAt: grpcx.TS(s.CreatedAt)}
}

func (h *Handler) GetMyBalances(ctx context.Context, _ *pb.GetMyBalancesRequest) (*pb.GetMyBalancesResponse, error) {
	b, err := h.svc.MyBalances(ctx)
	if err != nil {
		return nil, grpcx.ToStatus(ctx, err)
	}
	users := h.svc.Users(ctx, b.FriendsSortedKeys)
	out := &pb.GetMyBalancesResponse{Currency: b.Currency, OwedToMeCents: b.OwedToMe, IOweCents: b.IOwe}
	for _, uid := range b.FriendsSortedKeys {
		out.Friends = append(out.Friends, &pb.FriendBalance{UserId: uid, NetCents: b.Friends[uid], Name: users[uid].Name,
			Payment: paymentPB(users[uid])})
	}
	return out, nil
}

// ---------- internal ----------

func (h *Handler) GetReceiptImage(ctx context.Context, req *ipb.GetReceiptImageRequest) (*ipb.ReceiptImage, error) {
	f, st, err := h.svc.ReceiptImage(ctx, req.GetReceiptId())
	if err != nil {
		return nil, grpcx.ToStatus(ctx, err)
	}
	return &ipb.ReceiptImage{ReceiptId: f.ReceiptID, Image: f.Data, ContentType: f.ContentType, RawText: f.RawText,
		Status: pb.ReceiptStatus(st).String()}, nil
}

func (h *Handler) GetMemberBalance(ctx context.Context, req *ipb.GetMemberBalanceRequest) (*ipb.MemberBalance, error) {
	net, err := h.svc.MemberBalance(ctx, req.GetGroupId(), req.GetUserId())
	if err != nil {
		return nil, grpcx.ToStatus(ctx, err)
	}
	return &ipb.MemberBalance{NetCents: net}, nil
}

func (h *Handler) GroupHasActivity(ctx context.Context, req *ipb.GroupHasActivityRequest) (*ipb.GroupHasActivityResponse, error) {
	ok, err := h.svc.GroupHasActivity(ctx, req.GetGroupId())
	if err != nil {
		return nil, grpcx.ToStatus(ctx, err)
	}
	return &ipb.GroupHasActivityResponse{HasActivity: ok}, nil
}

func (h *Handler) GetUserTotals(ctx context.Context, req *ipb.GetUserTotalsRequest) (*ipb.UserTotals, error) {
	owed, owe, err := h.svc.UserTotals(ctx, req.GetUserId(), req.GetCurrency())
	if err != nil {
		return nil, grpcx.ToStatus(ctx, err)
	}
	return &ipb.UserTotals{OwedToMeCents: owed, IOweCents: owe}, nil
}

// ---------- mapping ----------

func itemsIn(in []*pb.ItemInput) []domain.ReceiptItem {
	out := make([]domain.ReceiptItem, 0, len(in))
	for _, it := range in {
		out = append(out, domain.ReceiptItem{Name: it.GetName(), Quantity: it.GetQuantity(), PriceCents: it.GetPriceCents()})
	}
	return out
}

// paymentPB always returns an object (empty handles = no pay buttons) so clients don't null-check.
func paymentPB(u clients.UserInfo) *pb.PaymentHandles {
	return &pb.PaymentHandles{Venmo: u.Venmo, Paypal: u.PayPal, Zelle: u.Zelle}
}

// receipt maps one receipt, resolving the payer name.
func (h *Handler) receipt(ctx context.Context, r *Receipt) *pb.Receipt {
	return receiptPB(r, h.svc.Users(ctx, []string{r.PayerID}))
}

func receiptPB(r *Receipt, users map[string]clients.UserInfo) *pb.Receipt {
	out := &pb.Receipt{
		Id: r.ID, GroupId: r.GroupID, PayerId: r.PayerID, Store: r.Store, Status: pb.ReceiptStatus(r.Status),
		SubtotalCents: r.SubtotalCents, TaxCents: r.TaxCents, TipCents: r.TipCents, TotalCents: r.TotalCents,
		Currency: r.Currency, ParseError: r.ParseError, OcrProvider: r.OCRProvider,
		PurchasedAt: grpcx.TSPtr(r.PurchasedAt), CreatedAt: grpcx.TS(r.CreatedAt),
		Kind: pb.ReceiptKind(r.Kind), PayerName: users[r.PayerID].Name,
	}
	for _, it := range r.Items {
		item := &pb.ReceiptItem{Id: it.ID, Name: it.Name, RawName: it.RawName, Code: it.Code, Quantity: it.Quantity, PriceCents: it.PriceCents}
		for _, c := range it.Claims {
			item.Claims = append(item.Claims, &pb.ItemClaim{UserId: c.UserID, Shares: c.Shares})
		}
		out.Items = append(out.Items, item)
	}
	return out
}

// split maps a split view, resolving member names in one call.
func (h *Handler) split(ctx context.Context, v *SplitView) *pb.Split {
	ids := []string{v.Receipt.PayerID}
	for _, sh := range v.Shares {
		ids = append(ids, sh.UserID)
	}
	return splitPB(v, h.svc.Users(ctx, ids))
}

func splitPB(v *SplitView, users map[string]clients.UserInfo) *pb.Split {
	out := &pb.Split{ReceiptId: v.Receipt.ID, PayerId: v.Receipt.PayerID, UnclaimedCents: v.UnclaimedCents,
		UnclaimedItemIds: v.UnclaimedItemIDs, Currency: v.Receipt.Currency, GroupCurrency: v.GroupCurrency, FxRate: v.Rate}
	for _, sh := range v.Shares {
		out.Shares = append(out.Shares, &pb.MemberShare{UserId: sh.UserID, ItemsCents: sh.ItemsCents, TaxCents: sh.TaxCents,
			TipCents: sh.TipCents, TotalCents: sh.TotalCents, TotalGroupCents: v.GroupTotals[sh.UserID], Name: users[sh.UserID].Name})
	}
	return out
}
