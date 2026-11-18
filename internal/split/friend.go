package split

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/Zifeldev/Billsplit/internal/clients"
	"github.com/Zifeldev/Billsplit/internal/domain"
	"github.com/Zifeldev/Billsplit/internal/platform/auth"
	"github.com/Zifeldev/Billsplit/internal/platform/logger"
)

// EntryKind values match pb.LedgerEntryKind.
type EntryKind int32

const (
	EntryReceipt    EntryKind = 1
	EntryExpense    EntryKind = 2
	EntrySettlement EntryKind = 3
)

// LedgerEntry is one event between the caller and a friend, from the caller's
// side: Amount > 0 moves the balance toward the friend owing the caller.
type LedgerEntry struct {
	Kind       EntryKind
	ReceiptID  string
	Title      string
	Amount     int64 // group currency
	OccurredAt time.Time
}

// LedgerGroup is the pairwise history in one group (one currency).
// Net == sum of Entries amounts; > 0 means the friend owes the caller.
type LedgerGroup struct {
	GroupID   string
	GroupName string
	Currency  string
	Net       int64
	NetHome   int64
	Entries   []LedgerEntry // newest first
}

// FriendLedger is the caller's cross-group account with one friend.
type FriendLedger struct {
	Friend   clients.UserInfo
	Currency string // caller's home currency
	Net      int64  // home currency; > 0 means the friend owes the caller
	Groups   []LedgerGroup
}

// pairLedger folds the raw pair history (newest first) into per-group ledgers
// from me's perspective. Debts of one receipt are merged into a single entry.
// Groups are ordered by most recent activity. Rows of one group in different
// currencies (group currency changed) yield separate ledgers so sums stay exact.
func pairLedger(events []PairEvent, me string) []LedgerGroup {
	type gkey struct{ group, currency string }
	type rkey struct {
		gkey
		receipt string
	}
	var out []LedgerGroup
	groupIdx := map[gkey]int{}
	entryIdx := map[rkey]int{}
	for _, e := range events {
		gk := gkey{e.GroupID, e.Currency}
		gi, ok := groupIdx[gk]
		if !ok {
			gi = len(out)
			groupIdx[gk] = gi
			out = append(out, LedgerGroup{GroupID: e.GroupID, Currency: e.Currency})
		}
		g := &out[gi]

		var amount int64
		var entry LedgerEntry
		if e.Settlement {
			// Paying the friend reduces what I owe: positive for me.
			amount = e.Amount
			if e.FromUser != me {
				amount = -amount
			}
			entry = LedgerEntry{Kind: EntrySettlement, Title: "Payment", Amount: amount, OccurredAt: e.OccurredAt}
		} else {
			// A debt toward me means the friend owes me more.
			amount = e.Amount
			if e.ToUser != me {
				amount = -amount
			}
			kind, title := EntryReceipt, titleOr(e.Title, "Receipt")
			if e.Kind == KindExpense {
				kind, title = EntryExpense, titleOr(e.Title, "Expense")
			}
			if e.ReceiptID != "" {
				rk := rkey{gk, e.ReceiptID}
				if ei, ok := entryIdx[rk]; ok {
					g.Entries[ei].Amount += amount
					g.Net += amount
					continue
				}
				entryIdx[rk] = len(g.Entries)
			}
			entry = LedgerEntry{Kind: kind, ReceiptID: e.ReceiptID, Title: title, Amount: amount, OccurredAt: e.OccurredAt}
		}
		g.Entries = append(g.Entries, entry)
		g.Net += amount
	}
	return out
}

// friendCtx validates the friend id and returns the caller id and their profile.
func (s *Service) friendCtx(ctx context.Context, friendID string) (string, clients.UserInfo, error) {
	uid, err := auth.UserID(ctx)
	if err != nil {
		return "", clients.UserInfo{}, err
	}
	if uuid.Validate(friendID) != nil || len(friendID) != 36 {
		return "", clients.UserInfo{}, fmt.Errorf("%w: user_id must be a UUID", domain.ErrInvalidArgument)
	}
	if friendID == uid {
		return "", clients.UserInfo{}, fmt.Errorf("%w: user_id must be someone else", domain.ErrInvalidArgument)
	}
	me, err := s.users.GetUser(ctx, uid)
	if err != nil {
		return "", clients.UserInfo{}, err
	}
	return uid, me, nil
}

// FriendLedger nets the caller's balance with a friend across every group
// where the two have history (debts or settlements between them).
func (s *Service) FriendLedger(ctx context.Context, friendID string) (*FriendLedger, error) {
	uid, me, err := s.friendCtx(ctx, friendID)
	if err != nil {
		return nil, err
	}
	events, err := s.ledger.PairHistory(ctx, uid, friendID)
	if err != nil {
		return nil, err
	}
	out := &FriendLedger{Currency: me.Currency, Groups: pairLedger(events, uid)}
	out.Friend = s.Users(ctx, []string{friendID})[friendID]
	out.Friend.ID = friendID

	names := map[string]string{}
	for i := range out.Groups {
		g := &out.Groups[i]
		name, ok := names[g.GroupID]
		if !ok {
			name = "Group"
			if info, err := s.groups.GetGroup(ctx, g.GroupID); err == nil && info.Name != "" {
				name = info.Name
			} else if err != nil {
				logger.FromContext(ctx).Warn("friend ledger: group lookup failed", "group_id", g.GroupID, "err", err)
			}
			names[g.GroupID] = name
		}
		g.GroupName = name
		if g.NetHome, err = s.convert(ctx, g.Net, g.Currency, me.Currency); err != nil {
			return nil, err
		}
		out.Net += g.NetHome
	}
	return out, nil
}

// FriendSettlement is the result of SettleWithFriend.
type FriendSettlement struct {
	Settlements []domain.Settlement
	NetPaid     int64 // home currency; > 0 the friend paid the caller, < 0 the caller paid
	Currency    string
}

// SettleWithFriend zeroes every pairwise balance with the friend: one
// settlement per group with a non-zero net, all in one transaction together
// with the friend's notification. Already settled → empty result.
func (s *Service) SettleWithFriend(ctx context.Context, friendID string) (*FriendSettlement, error) {
	uid, me, err := s.friendCtx(ctx, friendID)
	if err != nil {
		return nil, err
	}
	names := s.Users(ctx, []string{uid})
	var out *FriendSettlement
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		out = &FriendSettlement{Currency: me.Currency}
		if err := s.ledger.LockPair(ctx, uid, friendID); err != nil {
			return err
		}
		events, err := s.ledger.PairHistory(ctx, uid, friendID)
		if err != nil {
			return err
		}
		for _, g := range pairLedger(events, uid) {
			if g.Net == 0 {
				continue
			}
			st := domain.Settlement{GroupID: g.GroupID, FromUser: friendID, ToUser: uid, Amount: g.Net, Currency: g.Currency}
			if g.Net < 0 {
				st.FromUser, st.ToUser, st.Amount = uid, friendID, -g.Net
			}
			if err := s.ledger.CreateSettlement(ctx, &st); err != nil {
				return err
			}
			home, err := s.convert(ctx, g.Net, g.Currency, me.Currency)
			if err != nil {
				return err
			}
			out.NetPaid += home
			out.Settlements = append(out.Settlements, st)
		}
		if len(out.Settlements) == 0 {
			return nil
		}
		body := fmt.Sprintf("%s settled up with you: %s across %s", displayName(names, uid),
			formatMoney(abs(out.NetPaid), me.Currency), plural(len(out.Settlements), "group"))
		return s.notify(ctx, uid+":"+friendID, domain.NotificationEvent{UserIDs: []string{friendID},
			Type: domain.NotifySettlement, Title: "Settled up", Body: body,
			Data: map[string]string{"user_id": uid}})
	})
	if err != nil {
		return nil, err
	}
	logger.FromContext(ctx).Info("settled with friend", "friend_id", friendID, "groups", len(out.Settlements))
	return out, nil
}
