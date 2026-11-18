// Package market fetches quotes and price history from Yahoo Finance's public
// chart and search endpoints (no API key). One symbol space covers stocks and
// ETFs (AAPL, KSPI, VOO), indices (^GSPC), crypto (BTC-USD), FX (USDKZT=X) and
// futures (GC=F gold).
//
// Responses are cached in Redis (daily ranges for 15 minutes, longer ranges for
// an hour); the last good response is kept for a day as a fallback when Yahoo
// is down or rate limits us.
package market

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Zifeldev/Billsplit/internal/platform/cache"
)

const DefaultBaseURL = "https://query1.finance.yahoo.com"

var ErrUnknownSymbol = errors.New("unknown symbol")

// Ranges supported by Chart, mapped to the bar interval we request.
var ranges = map[string]string{
	"1mo": "1d",
	"3mo": "1d",
	"6mo": "1d",
	"1y":  "1d",
	"5y":  "1wk",
}

func ValidRange(r string) bool {
	_, ok := ranges[r]
	return ok
}

type Point struct {
	Date  string  `json:"d"` // YYYY-MM-DD (exchange time zone)
	Close float64 `json:"c"`
}

type Chart struct {
	Symbol    string  `json:"symbol"`
	Name      string  `json:"name"`
	Currency  string  `json:"currency"`
	Price     float64 `json:"price"`
	PrevClose float64 `json:"prev_close"`
	Points    []Point `json:"points"`
}

// DayChangePercent compares the last price with the previous session close.
func (c *Chart) DayChangePercent() float64 {
	if c.PrevClose <= 0 || c.Price <= 0 {
		return 0
	}
	return (c.Price/c.PrevClose - 1) * 100
}

// CloseAt returns the last close on or before date (YYYY-MM-DD); 0 if none.
func (c *Chart) CloseAt(date string) float64 {
	v := 0.0
	for _, p := range c.Points {
		if p.Date > date {
			break
		}
		v = p.Close
	}
	return v
}

type Match struct {
	Symbol, Name, Exchange, Type string
}

type Client struct {
	base  string
	http  *http.Client
	cache cache.Cache
}

func New(baseURL string, c cache.Cache) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return &Client{base: strings.TrimRight(baseURL, "/"), http: &http.Client{Timeout: 6 * time.Second}, cache: c}
}

// NormalizeSymbol upper-cases and validates a ticker (letters, digits, . - = ^).
func NormalizeSymbol(s string) (string, error) {
	s = strings.ToUpper(strings.TrimSpace(s))
	if s == "" || len(s) > 20 {
		return "", ErrUnknownSymbol
	}
	for _, r := range s {
		if !(r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune(".-=^", r)) {
			return "", ErrUnknownSymbol
		}
	}
	return s, nil
}

// Chart returns price history for symbol over rng (see ranges).
func (c *Client) Chart(ctx context.Context, symbol, rng string) (*Chart, error) {
	sym, err := NormalizeSymbol(symbol)
	if err != nil {
		return nil, err
	}
	interval, ok := ranges[rng]
	if !ok {
		return nil, fmt.Errorf("bad range %q", rng)
	}
	key := "market:chart:v1:" + sym + ":" + rng
	stale := key + ":stale"
	var ch Chart
	if err := c.cache.Get(ctx, key, &ch); err == nil {
		return &ch, nil
	}
	fresh, err := c.fetchChart(ctx, sym, rng, interval)
	if err != nil {
		if errors.Is(err, ErrUnknownSymbol) {
			return nil, err
		}
		if c.cache.Get(ctx, stale, &ch) == nil {
			return &ch, nil
		}
		return nil, err
	}
	ttl := time.Hour
	if interval == "1d" && (rng == "1mo" || rng == "3mo") {
		ttl = 15 * time.Minute
	}
	_ = c.cache.Set(ctx, key, fresh, ttl)
	_ = c.cache.Set(ctx, stale, fresh, 24*time.Hour)
	return fresh, nil
}

type chartResp struct {
	Chart struct {
		Result []struct {
			Meta struct {
				Currency           string  `json:"currency"`
				Symbol             string  `json:"symbol"`
				LongName           string  `json:"longName"`
				ShortName          string  `json:"shortName"`
				RegularMarketPrice float64 `json:"regularMarketPrice"`
				ChartPreviousClose float64 `json:"chartPreviousClose"`
				PreviousClose      float64 `json:"previousClose"`
				GMTOffset          int64   `json:"gmtoffset"`
			} `json:"meta"`
			Timestamp  []int64 `json:"timestamp"`
			Indicators struct {
				Quote []struct {
					Close []*float64 `json:"close"`
				} `json:"quote"`
			} `json:"indicators"`
		} `json:"result"`
		Error *struct {
			Code        string `json:"code"`
			Description string `json:"description"`
		} `json:"error"`
	} `json:"chart"`
}

func (c *Client) fetchChart(ctx context.Context, sym, rng, interval string) (*Chart, error) {
	q := url.Values{"range": {rng}, "interval": {interval}, "includePrePost": {"false"}}
	var r chartResp
	status, err := c.get(ctx, "/v8/finance/chart/"+url.PathEscape(sym)+"?"+q.Encode(), &r)
	if status == http.StatusNotFound || (r.Chart.Error != nil && r.Chart.Error.Code == "Not Found") {
		return nil, ErrUnknownSymbol
	}
	if err != nil {
		return nil, err
	}
	if len(r.Chart.Result) == 0 {
		return nil, ErrUnknownSymbol
	}
	res := r.Chart.Result[0]
	m := res.Meta
	out := &Chart{Symbol: sym, Name: m.LongName, Currency: strings.ToUpper(m.Currency), Price: m.RegularMarketPrice}
	if out.Name == "" {
		out.Name = m.ShortName
	}
	if out.Name == "" {
		out.Name = sym
	}
	var closes []*float64
	if len(res.Indicators.Quote) > 0 {
		closes = res.Indicators.Quote[0].Close
	}
	loc := time.FixedZone("exchange", int(m.GMTOffset))
	for i, ts := range res.Timestamp {
		if i >= len(closes) || closes[i] == nil || *closes[i] <= 0 {
			continue
		}
		d := time.Unix(ts, 0).In(loc).Format("2006-01-02")
		if n := len(out.Points); n > 0 && out.Points[n-1].Date == d {
			out.Points[n-1].Close = *closes[i] // live bar duplicates today's date
			continue
		}
		out.Points = append(out.Points, Point{Date: d, Close: *closes[i]})
	}
	if out.Price <= 0 && len(out.Points) > 0 {
		out.Price = out.Points[len(out.Points)-1].Close
	}
	// Previous session close: the bar before the last one (meta.previousClose is
	// only present for short ranges; chartPreviousClose is the range start).
	switch {
	case len(out.Points) >= 2:
		out.PrevClose = out.Points[len(out.Points)-2].Close
	case m.PreviousClose > 0:
		out.PrevClose = m.PreviousClose
	default:
		out.PrevClose = m.ChartPreviousClose
	}
	if out.Price <= 0 {
		return nil, ErrUnknownSymbol
	}
	// London (GBp), Johannesburg (ZAc) and Tel Aviv (ILA) quote in minor units.
	if major, ok := subunitCurrencies[m.Currency]; ok {
		out.Currency = major
		out.Price /= 100
		out.PrevClose /= 100
		for i := range out.Points {
			out.Points[i].Close /= 100
		}
	}
	return out, nil
}

var subunitCurrencies = map[string]string{"GBp": "GBP", "GBX": "GBP", "ZAc": "ZAR", "ILA": "ILS"}

type searchResp struct {
	Quotes []struct {
		Symbol    string `json:"symbol"`
		ShortName string `json:"shortname"`
		LongName  string `json:"longname"`
		ExchDisp  string `json:"exchDisp"`
		QuoteType string `json:"quoteType"`
	} `json:"quotes"`
}

// Search looks tickers up by name or symbol.
func (c *Client) Search(ctx context.Context, query string) ([]Match, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	if len(query) > 60 {
		query = query[:60]
	}
	key := "market:search:v1:" + strings.ToLower(query)
	var out []Match
	if err := c.cache.Get(ctx, key, &out); err == nil {
		return out, nil
	}
	q := url.Values{"q": {query}, "quotesCount": {"8"}, "newsCount": {"0"}, "listsCount": {"0"}}
	var r searchResp
	if _, err := c.get(ctx, "/v1/finance/search?"+q.Encode(), &r); err != nil {
		return nil, err
	}
	out = []Match{}
	for _, x := range r.Quotes {
		if x.Symbol == "" {
			continue
		}
		name := x.LongName
		if name == "" {
			name = x.ShortName
		}
		out = append(out, Match{Symbol: x.Symbol, Name: strings.TrimSpace(name), Exchange: x.ExchDisp, Type: x.QuoteType})
	}
	_ = c.cache.Set(ctx, key, out, 24*time.Hour)
	return out, nil
}

func (c *Client) get(ctx context.Context, path string, dst any) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return 0, err
	}
	// Yahoo rejects requests without a browser-like User-Agent.
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Billsplit/1.0)")
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, fmt.Errorf("market: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		_ = json.NewDecoder(resp.Body).Decode(dst)
		return resp.StatusCode, ErrUnknownSymbol
	}
	if resp.StatusCode != http.StatusOK {
		return resp.StatusCode, fmt.Errorf("market: %s returned %s", path, resp.Status)
	}
	if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
		return resp.StatusCode, fmt.Errorf("market: decode: %w", err)
	}
	return resp.StatusCode, nil
}
