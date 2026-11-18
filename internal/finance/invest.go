package finance

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/Zifeldev/Billsplit/internal/domain"
	"github.com/Zifeldev/Billsplit/internal/platform/auth"
	"github.com/Zifeldev/Billsplit/internal/platform/fx"
	"github.com/Zifeldev/Billsplit/internal/platform/logger"
	"github.com/Zifeldev/Billsplit/internal/platform/market"
)

// Market is satisfied by *market.Client.
type Market interface {
	Chart(ctx context.Context, symbol, rng string) (*market.Chart, error)
	Search(ctx context.Context, query string) ([]market.Match, error)
}

// WithMarket enables market-priced investments and market data.
func (s *Service) WithMarket(m Market) *Service {
	s.market = m
	return s
}

const maxInvestments = 100

// SetInvestment validates and stores an investment. Market assets take their
// currency (and a default name) from the quote.
func (s *Service) SetInvestment(ctx context.Context, inv *domain.Investment) (*domain.Investment, error) {
	uid, home, err := s.home(ctx)
	if err != nil {
		return nil, err
	}
	inv.UserID = uid
	inv.Name = strings.TrimSpace(inv.Name)
	if inv.Kind < domain.InvestDeposit || inv.Kind > domain.InvestOther {
		return nil, fmt.Errorf("%w: unknown investment kind", domain.ErrInvalidArgument)
	}
	if inv.Kind == domain.InvestDeposit {
		inv.Symbol = ""
	}
	if inv.Quantity < 0 || inv.InvestedCents < 0 || inv.MonthlyContributionCents < 0 || inv.TermMonths < 0 ||
		inv.TermMonths > 600 || inv.RatePercent < -100 || inv.RatePercent > 1000 || math.IsNaN(inv.Quantity) {
		return nil, fmt.Errorf("%w: amounts, rate and term must be valid", domain.ErrInvalidArgument)
	}
	if inv.Capitalization < domain.CapMonthly || inv.Capitalization > domain.CapNone {
		inv.Capitalization = domain.CapMonthly
	}
	if inv.OpenedAt.IsZero() {
		inv.OpenedAt = time.Now()
	}
	inv.OpenedAt = day(inv.OpenedAt)
	if inv.OpenedAt.After(time.Now().AddDate(0, 0, 1)) {
		return nil, fmt.Errorf("%w: opening date is in the future", domain.ErrInvalidArgument)
	}

	if inv.Symbol != "" {
		if s.market == nil {
			return nil, fmt.Errorf("%w: market data is not configured", domain.ErrInvalidArgument)
		}
		sym, err := market.NormalizeSymbol(inv.Symbol)
		if err != nil {
			return nil, fmt.Errorf("%w: bad ticker %q", domain.ErrInvalidArgument, inv.Symbol)
		}
		ch, err := s.market.Chart(ctx, sym, "1mo")
		if errors.Is(err, market.ErrUnknownSymbol) {
			return nil, fmt.Errorf("%w: ticker %s not found", domain.ErrInvalidArgument, sym)
		}
		if err != nil {
			return nil, fmt.Errorf("%w: quotes are unavailable right now, try again later", domain.ErrUnavailable)
		}
		inv.Symbol, inv.Currency = sym, ch.Currency
		if inv.Name == "" {
			inv.Name = ch.Name
		}
		if inv.Quantity <= 0 {
			return nil, fmt.Errorf("%w: quantity must be positive", domain.ErrInvalidArgument)
		}
	} else {
		if inv.Currency, err = s.fx.Validate(ctx, inv.Currency, home); err != nil {
			return nil, err
		}
		if inv.InvestedCents <= 0 {
			return nil, fmt.Errorf("%w: amount must be positive", domain.ErrInvalidArgument)
		}
	}
	if inv.Name == "" || len(inv.Name) > 100 {
		return nil, fmt.Errorf("%w: name is required (up to 100 characters)", domain.ErrInvalidArgument)
	}
	if inv.ID == "" {
		existing, err := s.repo.ListInvestments(ctx, uid)
		if err != nil {
			return nil, err
		}
		if len(existing) >= maxInvestments {
			return nil, fmt.Errorf("%w: at most %d investments", domain.ErrInvalidArgument, maxInvestments)
		}
	}
	if err := s.repo.UpsertInvestment(ctx, inv); err != nil {
		return nil, err
	}
	return inv, nil
}

func (s *Service) DeleteInvestment(ctx context.Context, id string) error {
	uid, err := auth.UserID(ctx)
	if err != nil {
		return err
	}
	if id == "" {
		return fmt.Errorf("%w: id is required", domain.ErrInvalidArgument)
	}
	return s.repo.DeleteInvestment(ctx, uid, id)
}

// Holding is an investment valued now.
type Holding struct {
	domain.Investment
	ValueCents, ValueHomeCents  int64
	ContributedCents, GainCents int64
	GainPercent                 float64
	Price, DayChangePercent     float64
	EffectiveRatePercent        float64
	MaturityValueCents          int64
	MaturityAt                  *time.Time
	QuoteError                  string
}

type AllocationSlice struct {
	Kind       domain.InvestmentKind
	ValueCents int64
	Share      float64
}

type PortfolioPoint struct {
	Date                    string
	ValueCents, Contributed int64
}

type Portfolio struct {
	Currency                     string
	ValueCents, ContributedCents int64
	GainCents                    int64
	GainPercent                  float64
	YearlyInterestCents          int64
	Items                        []Holding
	Allocation                   []AllocationSlice
	History                      []PortfolioPoint
}

const historyWeeks = 52

// Portfolio values every investment now (deposits by formula, market assets by
// the latest quote) and builds a weekly value history for the last year. All
// totals are in the home currency at today's rates.
func (s *Service) Portfolio(ctx context.Context) (*Portfolio, error) {
	uid, home, err := s.home(ctx)
	if err != nil {
		return nil, err
	}
	invs, err := s.repo.ListInvestments(ctx, uid)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	p := &Portfolio{Currency: home, Items: []Holding{}, Allocation: []AllocationSlice{}, History: []PortfolioPoint{}}
	if len(invs) == 0 {
		return p, nil
	}

	// Quotes (1y daily history) for every distinct ticker, fetched concurrently.
	charts := map[string]*market.Chart{}
	quoteErr := map[string]string{}
	if s.market != nil {
		var mu sync.Mutex
		g, gctx := errgroup.WithContext(ctx)
		g.SetLimit(6)
		seen := map[string]bool{}
		for _, inv := range invs {
			if !inv.Market() || seen[inv.Symbol] {
				continue
			}
			seen[inv.Symbol] = true
			g.Go(func() error {
				ch, err := s.market.Chart(gctx, inv.Symbol, "1y")
				mu.Lock()
				defer mu.Unlock()
				if err != nil {
					logger.FromContext(ctx).Warn("quote", "symbol", inv.Symbol, "err", err)
					quoteErr[inv.Symbol] = "Quote unavailable, showing the amount invested"
					return nil
				}
				charts[inv.Symbol] = ch
				return nil
			})
		}
		_ = g.Wait()
	}

	// Today's rate per currency -> home.
	rates := map[string]float64{home: 1}
	toHome := func(cents float64, cur string) (int64, error) {
		r, ok := rates[cur]
		if !ok {
			var err error
			if r, err = s.fx.Rate(ctx, cur, home, now); err != nil {
				return 0, fmt.Errorf("rate %s->%s: %w", cur, home, err)
			}
			rates[cur] = r
		}
		return fx.Apply(int64(math.Round(cents)), r, cur, home), nil
	}

	byKind := map[domain.InvestmentKind]int64{}
	for _, inv := range invs {
		h := Holding{Investment: inv, ContributedCents: inv.InvestedCents}
		value := float64(inv.InvestedCents)
		switch {
		case inv.Market():
			if ch := charts[inv.Symbol]; ch != nil {
				h.Price, h.DayChangePercent = ch.Price, ch.DayChangePercent()
				value = inv.Quantity * ch.Price * math.Pow10(fx.MinorUnits(inv.Currency))
			} else {
				h.QuoteError = quoteErr[inv.Symbol]
				if h.QuoteError == "" {
					h.QuoteError = "Quotes are not configured"
				}
			}
		default:
			st := DepositAt(inv, now)
			value, h.ContributedCents = st.Value, st.Contributed
			h.EffectiveRatePercent = EffectiveRate(inv.RatePercent, inv.Capitalization)
			if m := MaturityAt(inv); m != nil {
				h.MaturityAt = m
				h.MaturityValueCents = int64(math.Round(DepositAt(inv, *m).Value))
			}
			// Interest expected over the next 12 months (top-ups excluded).
			next := DepositAt(inv, now.AddDate(1, 0, 0))
			interest := (next.Value - value) - float64(next.Contributed-st.Contributed)
			if ih, err := toHome(interest, inv.Currency); err == nil && ih > 0 {
				p.YearlyInterestCents += ih
			}
		}
		h.ValueCents = int64(math.Round(value))
		h.GainCents = h.ValueCents - h.ContributedCents
		if h.ContributedCents > 0 {
			h.GainPercent = float64(h.GainCents) / float64(h.ContributedCents) * 100
		}
		if h.ValueHomeCents, err = toHome(value, inv.Currency); err != nil {
			return nil, err
		}
		contribHome, err := toHome(float64(h.ContributedCents), inv.Currency)
		if err != nil {
			return nil, err
		}
		p.ValueCents += h.ValueHomeCents
		p.ContributedCents += contribHome
		byKind[inv.Kind] += h.ValueHomeCents
		p.Items = append(p.Items, h)
	}
	p.GainCents = p.ValueCents - p.ContributedCents
	if p.ContributedCents > 0 {
		p.GainPercent = float64(p.GainCents) / float64(p.ContributedCents) * 100
	}
	for k, v := range byKind {
		sl := AllocationSlice{Kind: k, ValueCents: v}
		if p.ValueCents > 0 {
			sl.Share = float64(v) / float64(p.ValueCents)
		}
		p.Allocation = append(p.Allocation, sl)
	}
	sort.Slice(p.Allocation, func(i, j int) bool { return p.Allocation[i].ValueCents > p.Allocation[j].ValueCents })
	sort.SliceStable(p.Items, func(i, j int) bool { return p.Items[i].ValueHomeCents > p.Items[j].ValueHomeCents })

	// Weekly history. Market assets are assumed held in the current quantity
	// since their opening date; FX is today's rate for the whole series.
	for w := historyWeeks; w >= 0; w-- {
		at := day(now).AddDate(0, 0, -7*w)
		ds := at.Format("2006-01-02")
		var val, contrib float64
		for _, inv := range invs {
			if day(inv.OpenedAt).After(at) {
				continue
			}
			cur := inv.Currency
			var v, c float64
			if inv.Market() {
				c = float64(inv.InvestedCents)
				v = c
				if ch := charts[inv.Symbol]; ch != nil {
					px := ch.CloseAt(ds)
					if px == 0 && len(ch.Points) > 0 {
						px = ch.Points[0].Close
					}
					if w == 0 {
						px = ch.Price
					}
					v = inv.Quantity * px * math.Pow10(fx.MinorUnits(cur))
				}
			} else {
				st := DepositAt(inv, at)
				v, c = st.Value, float64(st.Contributed)
			}
			vh, err := toHome(v, cur)
			if err != nil {
				return nil, err
			}
			ch, err := toHome(c, cur)
			if err != nil {
				return nil, err
			}
			val += float64(vh)
			contrib += float64(ch)
		}
		p.History = append(p.History, PortfolioPoint{Date: ds, ValueCents: int64(val), Contributed: int64(contrib)})
	}
	return p, nil
}

// MarketChart returns price history for a ticker.
func (s *Service) MarketChart(ctx context.Context, symbol, rng string) (*market.Chart, error) {
	if _, err := auth.UserID(ctx); err != nil {
		return nil, err
	}
	if s.market == nil {
		return nil, fmt.Errorf("%w: market data is not configured", domain.ErrUnavailable)
	}
	if rng == "" {
		rng = "1y"
	}
	if !market.ValidRange(rng) {
		return nil, fmt.Errorf("%w: range must be one of 1mo, 3mo, 6mo, 1y, 5y", domain.ErrInvalidArgument)
	}
	ch, err := s.market.Chart(ctx, symbol, rng)
	if errors.Is(err, market.ErrUnknownSymbol) {
		return nil, fmt.Errorf("%w: ticker not found", domain.ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("%w: quotes are unavailable right now", domain.ErrUnavailable)
	}
	return ch, nil
}

func (s *Service) SearchSymbols(ctx context.Context, q string) ([]market.Match, error) {
	if _, err := auth.UserID(ctx); err != nil {
		return nil, err
	}
	if s.market == nil {
		return nil, fmt.Errorf("%w: market data is not configured", domain.ErrUnavailable)
	}
	out, err := s.market.Search(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("%w: search is unavailable right now", domain.ErrUnavailable)
	}
	return out, nil
}

// Watchlist shown on the investments screen, before the user's own tickers.
var watchlist = []struct{ Symbol, Name string }{
	{"^GSPC", "S&P 500"},
	{"^IXIC", "Nasdaq"},
	{"BTC-USD", "Bitcoin"},
	{"ETH-USD", "Ethereum"},
	{"GC=F", "Gold"},
	{"USDKZT=X", "USD / KZT"},
	{"EURUSD=X", "EUR / USD"},
}

type Quote struct {
	Symbol, Name, Currency                      string
	Price, DayChangePercent, MonthChangePercent float64
	Spark                                       []float64
	Held                                        bool
}

// MarketOverview returns 1-month trends for the watchlist plus held tickers.
// Tickers whose quote fails are skipped.
func (s *Service) MarketOverview(ctx context.Context) ([]Quote, error) {
	uid, err := auth.UserID(ctx)
	if err != nil {
		return nil, err
	}
	if s.market == nil {
		return nil, fmt.Errorf("%w: market data is not configured", domain.ErrUnavailable)
	}
	invs, err := s.repo.ListInvestments(ctx, uid)
	if err != nil {
		return nil, err
	}
	type item struct{ sym, name string }
	var list []item
	held := map[string]bool{}
	for _, inv := range invs {
		if inv.Market() && !held[inv.Symbol] {
			held[inv.Symbol] = true
			list = append(list, item{inv.Symbol, inv.Name})
		}
	}
	for _, w := range watchlist {
		if !held[w.Symbol] {
			list = append(list, item{w.Symbol, w.Name})
		}
	}

	out := make([]*Quote, len(list))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(6)
	for i, it := range list {
		g.Go(func() error {
			ch, err := s.market.Chart(gctx, it.sym, "1mo")
			if err != nil {
				logger.FromContext(ctx).Warn("overview quote", "symbol", it.sym, "err", err)
				return nil
			}
			q := &Quote{Symbol: it.sym, Name: it.name, Currency: ch.Currency, Price: ch.Price,
				DayChangePercent: ch.DayChangePercent(), Held: held[it.sym]}
			for _, pt := range ch.Points {
				q.Spark = append(q.Spark, pt.Close)
			}
			if len(ch.Points) > 0 && ch.Points[0].Close > 0 {
				q.MonthChangePercent = (ch.Price/ch.Points[0].Close - 1) * 100
			}
			out[i] = q
			return nil
		})
	}
	_ = g.Wait()
	res := make([]Quote, 0, len(out))
	for _, q := range out {
		if q != nil {
			res = append(res, *q)
		}
	}
	return res, nil
}
