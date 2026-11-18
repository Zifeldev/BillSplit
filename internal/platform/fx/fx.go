// Package fx converts money between currencies using free reference rates:
//
//   - Frankfurter (https://frankfurter.dev, ECB rates, no API key, ~30 currencies);
//   - the National Bank of Kazakhstan (nationalbank.kz RSS, official KZT rates for
//     ~45 currencies incl. KZT, AED, RUB, UZS, KGS, GEL... that ECB doesn't publish).
//
// Pairs Frankfurter knows go to Frankfurter; anything else is crossed through
// KZT with NBK rates. Rates are cached in Redis: historical rates never change,
// today's rate is cached for an hour, and the last known rate is the fallback
// when a provider is down.
package fx

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Zifeldev/Billsplit/internal/domain"
	"github.com/Zifeldev/Billsplit/internal/platform/cache"
)

// DefaultNBKURL is the National Bank of Kazakhstan daily rates feed (?fdate=DD.MM.YYYY).
const DefaultNBKURL = "https://nationalbank.kz/rss/get_rates.cfm"

// KZT is the base currency of NBK rates.
const KZT = "KZT"

type Client struct {
	base  string
	nbk   string
	http  *http.Client
	cache cache.Cache

	mu         sync.RWMutex
	currencies map[string]string // union of both providers
	frank      map[string]bool   // codes Frankfurter supports
}

func New(baseURL string, c cache.Cache) *Client {
	return &Client{base: strings.TrimRight(baseURL, "/"), nbk: DefaultNBKURL,
		http: &http.Client{Timeout: 4 * time.Second}, cache: c}
}

// WithNBK overrides the NBK feed URL (tests, mirrors).
func (c *Client) WithNBK(url string) *Client {
	c.nbk = url
	return c
}

type currencyLists struct {
	Frank map[string]string `json:"frank"`
	NBK   map[string]string `json:"nbk"`
}

// Currencies returns code -> name of supported currencies.
func (c *Client) Currencies(ctx context.Context) (map[string]string, error) {
	c.mu.RLock()
	if c.currencies != nil {
		defer c.mu.RUnlock()
		return c.currencies, nil
	}
	c.mu.RUnlock()

	const key = "fx:currencies:v2"
	var lists currencyLists
	complete := true
	if err := c.cache.Get(ctx, key, &lists); err != nil {
		ferr := c.get(ctx, "/currencies", &lists.Frank)
		var nerr error
		lists.NBK, nerr = c.nbkCurrencies(ctx)
		if ferr != nil && nerr != nil {
			return nil, ferr
		}
		if complete = ferr == nil && nerr == nil; complete {
			_ = c.cache.Set(ctx, key, lists, 24*time.Hour)
		}
	}
	out := make(map[string]string, len(lists.Frank)+len(lists.NBK)+1)
	for code, name := range lists.NBK {
		out[code] = name
	}
	frank := make(map[string]bool, len(lists.Frank))
	for code, name := range lists.Frank {
		out[code] = name // ECB names are English
		frank[code] = true
	}
	out[KZT] = englishNames[KZT]
	if complete {
		// A partial list (one provider down) is served but not memoized.
		c.mu.Lock()
		c.currencies, c.frank = out, frank
		c.mu.Unlock()
	}
	return out, nil
}

// Validate normalizes and checks a currency code. Empty input returns def.
func (c *Client) Validate(ctx context.Context, code, def string) (string, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "" {
		return def, nil
	}
	list, err := c.Currencies(ctx)
	if err != nil {
		// Rates provider down: accept syntactically valid codes rather than block users.
		if len(code) == 3 {
			return code, nil
		}
		return "", fmt.Errorf("%w: bad currency %q", domain.ErrInvalidArgument, code)
	}
	if _, ok := list[code]; !ok {
		return "", fmt.Errorf("%w: unsupported currency %q", domain.ErrInvalidArgument, code)
	}
	return code, nil
}

// Rate returns how many `to` units one `from` unit costs on date (zero date — latest).
func (c *Client) Rate(ctx context.Context, from, to string, date time.Time) (float64, error) {
	if from == to {
		return 1, nil
	}
	day := "latest"
	ttl := time.Hour
	if !date.IsZero() {
		if today := time.Now().UTC().Truncate(24 * time.Hour); !date.Before(today) {
			date = today
		} else {
			ttl = 30 * 24 * time.Hour // historical rates are immutable
		}
		day = date.UTC().Format(time.DateOnly)
	}
	key := fmt.Sprintf("fx:%s:%s:%s", day, from, to)
	var rate float64
	if err := c.cache.Get(ctx, key, &rate); err == nil {
		return rate, nil
	}

	lastKey := fmt.Sprintf("fx:last:%s:%s", from, to)
	var err error
	if c.viaFrankfurter(ctx, from, to) {
		rate, err = c.frankRate(ctx, day, from, to)
	} else {
		rate, err = c.nbkRate(ctx, date, day, from, to)
	}
	if err != nil {
		if errors.Is(err, domain.ErrInvalidArgument) {
			return 0, err
		}
		// Provider is down or slow: fall back to the last known rate instead of failing the request.
		if c.cache.Get(ctx, lastKey, &rate) == nil {
			return rate, nil
		}
		return 0, err
	}
	_ = c.cache.Set(ctx, key, rate, ttl)
	_ = c.cache.Set(ctx, lastKey, rate, 90*24*time.Hour)
	return rate, nil
}

// viaFrankfurter: both codes are ECB currencies. If the currency lists are
// unavailable, codes known to be NBK-only go to NBK and the rest to Frankfurter.
func (c *Client) viaFrankfurter(ctx context.Context, from, to string) bool {
	if from == KZT || to == KZT {
		return false
	}
	_, _ = c.Currencies(ctx)
	c.mu.RLock()
	frank := c.frank
	c.mu.RUnlock()
	if frank == nil {
		return !nbkOnly[from] && !nbkOnly[to]
	}
	return frank[from] && frank[to]
}

func (c *Client) frankRate(ctx context.Context, day, from, to string) (float64, error) {
	var resp struct {
		Rates map[string]float64 `json:"rates"`
	}
	if err := c.get(ctx, fmt.Sprintf("/%s?base=%s&symbols=%s", day, from, to), &resp); err != nil {
		return 0, err
	}
	rate, ok := resp.Rates[to]
	if !ok || rate <= 0 {
		return 0, fmt.Errorf("%w: no rate %s->%s", domain.ErrInvalidArgument, from, to)
	}
	return rate, nil
}

// nbkRate crosses through KZT: rate = kztPer(from) / kztPer(to).
func (c *Client) nbkRate(ctx context.Context, date time.Time, day, from, to string) (float64, error) {
	t, err := c.nbkTable(ctx, date)
	if err != nil {
		return 0, err
	}
	kztPer := func(code string) (float64, error) {
		if code == KZT {
			return 1, nil
		}
		if v, ok := t.Rates[code]; ok {
			return v, nil
		}
		// Not published by NBK (e.g. ISK): ECB rate to USD, then NBK USD.
		usd, ok := t.Rates["USD"]
		if !ok {
			return 0, fmt.Errorf("%w: no rate %s->%s", domain.ErrInvalidArgument, from, to)
		}
		r, err := c.frankRate(ctx, day, code, "USD")
		if err != nil {
			return 0, err
		}
		return r * usd, nil
	}
	f, err := kztPer(from)
	if err != nil {
		return 0, err
	}
	d, err := kztPer(to)
	if err != nil {
		return 0, err
	}
	if f <= 0 || d <= 0 {
		return 0, fmt.Errorf("%w: no rate %s->%s", domain.ErrInvalidArgument, from, to)
	}
	return f / d, nil
}

// nbkDay is one NBK publication: KZT per 1 unit of each currency.
type nbkDay struct {
	Rates map[string]float64 `json:"rates"`
	Names map[string]string  `json:"names"`
}

// almaty is Kazakhstan's time zone (UTC+5, no DST): NBK dates are local.
var almaty = time.FixedZone("ALMT", 5*60*60)

// nbkTable returns the NBK rates for date (zero — today). Days without a
// publication fall back to the closest earlier one (up to a week).
func (c *Client) nbkTable(ctx context.Context, date time.Time) (*nbkDay, error) {
	today := time.Now().In(almaty)
	today = time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)
	if date.IsZero() || date.After(today) {
		date = today
	}
	date = time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
	var firstErr error
	for i := 0; i < 7; i++ {
		d := date.AddDate(0, 0, -i)
		key := "fx:nbk:" + d.Format(time.DateOnly)
		var t nbkDay
		if err := c.cache.Get(ctx, key, &t); err == nil && len(t.Rates) > 0 {
			return &t, nil
		}
		t, err := c.nbkFetch(ctx, d)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			if i == 0 {
				return nil, err // provider down: don't hammer it for every earlier day
			}
			continue
		}
		if len(t.Rates) == 0 {
			continue // weekend/holiday or not published yet
		}
		ttl := 30 * 24 * time.Hour
		if !d.Before(today) {
			ttl = time.Hour
		}
		_ = c.cache.Set(ctx, key, t, ttl)
		return &t, nil
	}
	if firstErr != nil {
		return nil, firstErr
	}
	return nil, fmt.Errorf("fx: nbk has no rates on or before %s", date.Format(time.DateOnly))
}

func (c *Client) nbkFetch(ctx context.Context, d time.Time) (nbkDay, error) {
	url := c.nbk + "?fdate=" + d.Format("02.01.2006")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nbkDay{}, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nbkDay{}, fmt.Errorf("fx nbk: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nbkDay{}, fmt.Errorf("fx nbk: %s returned %d", url, resp.StatusCode)
	}
	var doc struct {
		Items []struct {
			FullName    string `xml:"fullname"`
			Title       string `xml:"title"`
			Description string `xml:"description"`
			Quant       string `xml:"quant"`
		} `xml:"item"`
	}
	if err := xml.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return nbkDay{}, fmt.Errorf("fx nbk: %w", err)
	}
	out := nbkDay{Rates: map[string]float64{}, Names: map[string]string{}}
	for _, it := range doc.Items {
		code := strings.ToUpper(strings.TrimSpace(it.Title))
		if len(code) != 3 || code == "XDR" { // XDR is an IMF unit, not a currency
			continue
		}
		rate, err := strconv.ParseFloat(strings.TrimSpace(it.Description), 64)
		if err != nil || rate <= 0 {
			continue
		}
		quant := 1.0
		if q, err := strconv.ParseFloat(strings.TrimSpace(it.Quant), 64); err == nil && q > 0 {
			quant = q
		}
		out.Rates[code] = rate / quant
		out.Names[code] = strings.TrimSpace(it.FullName)
	}
	return out, nil
}

func (c *Client) nbkCurrencies(ctx context.Context) (map[string]string, error) {
	t, err := c.nbkTable(ctx, time.Time{})
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(t.Rates))
	for code := range t.Rates {
		name := englishNames[code]
		if name == "" {
			name = t.Names[code]
		}
		out[code] = name
	}
	return out, nil
}

// English names for NBK currencies that ECB (Frankfurter) doesn't list.
var englishNames = map[string]string{
	"KZT": "Kazakhstani Tenge", "AED": "UAE Dirham", "AMD": "Armenian Dram", "AZN": "Azerbaijani Manat",
	"BYN": "Belarusian Ruble", "EGP": "Egyptian Pound", "GEL": "Georgian Lari", "IRR": "Iranian Rial",
	"KGS": "Kyrgyzstani Som", "KWD": "Kuwaiti Dinar", "MDL": "Moldovan Leu", "MNT": "Mongolian Tugrik",
	"OMR": "Omani Rial", "PKR": "Pakistani Rupee", "QAR": "Qatari Riyal", "RUB": "Russian Ruble",
	"SAR": "Saudi Riyal", "TJS": "Tajikistani Somoni", "UAH": "Ukrainian Hryvnia", "UZS": "Uzbekistani Som",
	"VND": "Vietnamese Dong",
}

// nbkOnly: used to route rates when the currency lists can't be loaded.
var nbkOnly = func() map[string]bool {
	m := map[string]bool{}
	for code := range englishNames {
		m[code] = true
	}
	return m
}()

// Convert converts cents with banker-safe rounding to the nearest cent.
func (c *Client) Convert(ctx context.Context, cents int64, from, to string, date time.Time) (int64, error) {
	if from == to || cents == 0 {
		return cents, nil
	}
	r, err := c.Rate(ctx, from, to, date)
	if err != nil {
		return 0, err
	}
	return Apply(cents, r, from, to), nil
}

// Apply converts an amount in minor units of `from` into minor units of `to`,
// accounting for currencies without cents (JPY, KRW, ISK) or with 3 decimals (KWD).
func Apply(cents int64, rate float64, from, to string) int64 {
	scale := math.Pow10(MinorUnits(to) - MinorUnits(from))
	return int64(math.Round(float64(cents) * rate * scale))
}

// zeroDecimal lists ISO 4217 currencies with no minor units among the supported ones.
var zeroDecimal = map[string]bool{"JPY": true, "KRW": true, "ISK": true, "CLP": true, "VND": true}

// threeDecimal lists ISO 4217 currencies with 3 decimals among the supported ones.
var threeDecimal = map[string]bool{"KWD": true, "OMR": true, "BHD": true, "JOD": true, "TND": true}

// MinorUnits is the number of decimal digits of a currency (2 for USD and KZT, 0 for JPY, 3 for KWD).
func MinorUnits(code string) int {
	switch {
	case zeroDecimal[code]:
		return 0
	case threeDecimal[code]:
		return 3
	}
	return 2
}

// FromTwoDecimals rescales an amount parsed as if it had 2 decimals ("1280.00"
// from a statement) into the real minor units of the currency.
func FromTwoDecimals(cents int64, code string) int64 {
	switch MinorUnits(code) {
	case 0:
		return cents / 100
	case 3:
		return cents * 10
	}
	return cents
}

func (c *Client) get(ctx context.Context, path string, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("fx: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fx: %s returned %d", path, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(dst)
}
