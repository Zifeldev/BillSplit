package finance

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/Zifeldev/Billsplit/internal/domain"
	"github.com/Zifeldev/Billsplit/internal/platform/auth"
	"github.com/Zifeldev/Billsplit/internal/platform/logger"
)

const (
	maxAccounts     = 50
	maxBalanceCents = 1e15 // sanity cap (10 trillion in a 2-decimal currency)
)

func (s *Service) SetAccount(ctx context.Context, a *domain.Account) (*AccountView, error) {
	uid, home, err := s.home(ctx)
	if err != nil {
		return nil, err
	}
	a.UserID = uid
	a.Name = strings.TrimSpace(a.Name)
	if a.Name == "" || len(a.Name) > 100 {
		return nil, fmt.Errorf("%w: name is required (up to 100 characters)", domain.ErrInvalidArgument)
	}
	if a.Kind < domain.AccountCash || a.Kind > domain.AccountProperty {
		return nil, fmt.Errorf("%w: unknown account kind", domain.ErrInvalidArgument)
	}
	if a.BalanceCents < 0 || a.BalanceCents > maxBalanceCents {
		return nil, fmt.Errorf("%w: balance must be 0 or more (for a credit card or loan enter the amount owed)", domain.ErrInvalidArgument)
	}
	if a.Currency, err = s.fx.Validate(ctx, a.Currency, home); err != nil {
		return nil, err
	}
	if a.ID == "" {
		existing, err := s.repo.ListAccounts(ctx, uid)
		if err != nil {
			return nil, err
		}
		if len(existing) >= maxAccounts {
			return nil, fmt.Errorf("%w: up to %d accounts", domain.ErrFailedPrecondition, maxAccounts)
		}
	}
	if err := s.repo.UpsertAccount(ctx, a); err != nil {
		return nil, err
	}
	cv := &converter{ctx: ctx, fx: s.fx, home: home, date: time.Now().UTC(), rates: map[string]float64{}}
	v, err := cv.do(a.BalanceCents, a.Currency)
	if err != nil {
		return nil, err
	}
	return &AccountView{Account: *a, BalanceHomeCents: v}, nil
}

func (s *Service) DeleteAccount(ctx context.Context, id string) error {
	uid, err := auth.UserID(ctx)
	if err != nil {
		return err
	}
	if id == "" {
		return fmt.Errorf("%w: id is required", domain.ErrInvalidArgument)
	}
	return s.repo.DeleteAccount(ctx, uid, id)
}

// AccountView is an account with its balance in the home currency.
type AccountView struct {
	domain.Account
	BalanceHomeCents int64
}

func (s *Service) ListAccounts(ctx context.Context) ([]AccountView, error) {
	uid, home, err := s.home(ctx)
	if err != nil {
		return nil, err
	}
	return s.accounts(ctx, uid, home, time.Now().UTC())
}

func (s *Service) accounts(ctx context.Context, uid, home string, now time.Time) ([]AccountView, error) {
	as, err := s.repo.ListAccounts(ctx, uid)
	if err != nil {
		return nil, err
	}
	cv := &converter{ctx: ctx, fx: s.fx, home: home, date: now, rates: map[string]float64{}}
	out := make([]AccountView, 0, len(as))
	for _, a := range as {
		v, err := cv.do(a.BalanceCents, a.Currency)
		if err != nil {
			return nil, err
		}
		out = append(out, AccountView{Account: a, BalanceHomeCents: v})
	}
	return out, nil
}

type NetWorth struct {
	Currency                      string
	NetCents                      int64
	AssetsCents, LiabilitiesCents int64
	CashCents, PropertyCents      int64
	InvestmentsCents              int64
	OwedToMeCents                 int64
	CreditCents, IOweCents        int64
	Accounts                      []AccountView
	History                       []domain.NetWorthSnapshot
	Change30dCents                int64
	HasChange30d                  bool
}

const netWorthHistoryDays = 365

// NetWorth sums accounts, investments and bill-split balances in the home
// currency at today's rates, records today's snapshot and returns the history.
func (s *Service) NetWorth(ctx context.Context) (*NetWorth, error) {
	uid, home, err := s.home(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	nw, err := s.netWorthNow(ctx, uid, home, now)
	if err != nil {
		return nil, err
	}
	today := day(now)
	if err := s.repo.UpsertNetWorthSnapshot(ctx, uid, nw.snapshot(today)); err != nil {
		logger.FromContext(ctx).Warn("net worth snapshot", "err", err)
	}
	hist, err := s.repo.NetWorthHistory(ctx, uid, today.AddDate(0, 0, -netWorthHistoryDays))
	if err != nil {
		return nil, err
	}
	// Snapshots in another currency (home currency changed) would mix units.
	nw.History = hist[:0]
	for _, h := range hist {
		if h.Currency == home {
			nw.History = append(nw.History, h)
		}
	}
	// Change vs the latest snapshot at least 30 days old.
	for i := len(nw.History) - 1; i >= 0; i-- {
		if h := nw.History[i]; !h.Day.After(today.AddDate(0, 0, -30)) {
			nw.Change30dCents = nw.NetCents - (h.AssetsCents - h.LiabilitiesCents)
			nw.HasChange30d = true
			break
		}
	}
	return nw, nil
}

func (nw *NetWorth) snapshot(d time.Time) domain.NetWorthSnapshot {
	return domain.NetWorthSnapshot{Day: d, Currency: nw.Currency, AssetsCents: nw.AssetsCents, LiabilitiesCents: nw.LiabilitiesCents}
}

func (s *Service) netWorthNow(ctx context.Context, uid, home string, now time.Time) (*NetWorth, error) {
	nw := &NetWorth{Currency: home}
	var err error
	if nw.Accounts, err = s.accounts(ctx, uid, home, now); err != nil {
		return nil, err
	}
	for _, a := range nw.Accounts {
		switch {
		case a.Kind.Liability():
			nw.CreditCents += a.BalanceHomeCents
		case a.Kind == domain.AccountProperty:
			nw.PropertyCents += a.BalanceHomeCents
		default:
			nw.CashCents += a.BalanceHomeCents
		}
	}
	p, err := s.Portfolio(ctx)
	if err != nil {
		return nil, err
	}
	nw.InvestmentsCents = p.ValueCents
	if nw.OwedToMeCents, nw.IOweCents, err = s.split.UserTotals(ctx, uid, home); err != nil {
		logger.FromContext(ctx).Warn("split totals unavailable", "err", err)
	}
	nw.AssetsCents = nw.CashCents + nw.PropertyCents + nw.InvestmentsCents + nw.OwedToMeCents
	nw.LiabilitiesCents = nw.CreditCents + nw.IOweCents
	nw.NetCents = nw.AssetsCents - nw.LiabilitiesCents
	return nw, nil
}

const snapshotInterval = 6 * time.Hour

// RunNetWorthSnapshots records a daily net worth point for every user with
// accounts or investments, so the history chart has no gaps on days the app
// wasn't opened. Re-running on the same day just overwrites that day's point.
func (s *Service) RunNetWorthSnapshots(ctx context.Context, log *slog.Logger) error {
	log = log.With("component", "net-worth-snapshots")
	ctx = logger.WithContext(ctx, log)
	t := time.NewTicker(snapshotInterval)
	defer t.Stop()
	for {
		n, err := s.SnapshotAll(ctx, time.Now().UTC())
		switch {
		case ctx.Err() != nil:
			return nil
		case err != nil:
			log.Warn("snapshots failed", "err", err, "done", n)
		case n > 0:
			log.Info("snapshots recorded", "users", n)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
		}
	}
}

func (s *Service) SnapshotAll(ctx context.Context, now time.Time) (int, error) {
	// One replica per day-quarter does the work.
	if ok, err := s.locker.SetNX(ctx, "finance:networth:snap:"+now.Truncate(snapshotInterval).Format(time.RFC3339), snapshotInterval); err != nil || !ok {
		return 0, err
	}
	done, after := 0, ""
	for {
		ids, err := s.repo.UsersWithHoldings(ctx, after, 200)
		if err != nil {
			return done, err
		}
		for _, uid := range ids {
			if ctx.Err() != nil {
				return done, ctx.Err()
			}
			u, err := s.users.GetUser(ctx, uid)
			if err != nil {
				logger.FromContext(ctx).Warn("snapshot: user", "user_id", uid, "err", err)
				continue
			}
			uctx := auth.WithUserID(ctx, uid)
			nw, err := s.netWorthNow(uctx, uid, u.Currency, now)
			if err != nil {
				logger.FromContext(ctx).Warn("snapshot: net worth", "user_id", uid, "err", err)
				continue
			}
			if err := s.repo.UpsertNetWorthSnapshot(ctx, uid, nw.snapshot(day(now))); err != nil {
				return done, err
			}
			done++
		}
		if len(ids) < 200 {
			return done, nil
		}
		after = ids[len(ids)-1]
	}
}

// ---------- goals linked to investments ----------

// GoalView is a goal with progress resolved from its linked investment.
type GoalView struct {
	domain.SavingsGoal
	InvestmentName string
	ProjectedAt    *time.Time // linked deposits: when the target is reached
}

// goalsView resolves linked goals: saved = the investment's value in the home
// currency today; for deposits also the month the target is reached.
func (s *Service) goalsView(ctx context.Context, gs []domain.SavingsGoal) ([]GoalView, error) {
	out := make([]GoalView, len(gs))
	linked := false
	for i, g := range gs {
		out[i] = GoalView{SavingsGoal: g}
		linked = linked || g.InvestmentID != ""
	}
	if !linked {
		return out, nil
	}
	p, err := s.Portfolio(ctx)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]Holding, len(p.Items))
	for _, h := range p.Items {
		byID[h.ID] = h
	}
	now := time.Now().UTC()
	for i := range out {
		g := &out[i]
		h, ok := byID[g.InvestmentID]
		if g.InvestmentID == "" || !ok {
			g.InvestmentID = "" // investment deleted: falls back to the manual amount
			continue
		}
		g.InvestmentName = h.Name
		g.SavedCents = h.ValueHomeCents
		if h.Kind == domain.InvestDeposit && g.SavedCents < g.TargetCents && h.ValueCents > 0 {
			g.ProjectedAt = depositReaches(h.Investment, g.TargetCents, float64(h.ValueHomeCents)/float64(h.ValueCents), now)
		}
	}
	return out, nil
}

// depositReaches is the first month end when the deposit (converted with
// `rate` to the goal currency) is worth `target`; nil if not within 50 years
// or the deposit matures earlier.
func depositReaches(inv domain.Investment, target int64, rate float64, now time.Time) *time.Time {
	end := now.AddDate(50, 0, 0)
	if m := MaturityAt(inv); m != nil && m.Before(end) {
		end = *m
	}
	for at := now.AddDate(0, 1, 0); !at.After(end.AddDate(0, 0, 1)); at = at.AddDate(0, 1, 0) {
		if at.After(end) {
			at = end
		}
		if DepositAt(inv, at).Value*rate >= float64(target) {
			d := day(at)
			return &d
		}
		if at.Equal(end) {
			break
		}
	}
	return nil
}
