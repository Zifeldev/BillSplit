package fx

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Zifeldev/Billsplit/internal/platform/cache"
)

type memCache struct {
	mu sync.Mutex
	m  map[string][]byte
}

func newMemCache() *memCache { return &memCache{m: map[string][]byte{}} }

func (c *memCache) Get(_ context.Context, key string, dst any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	b, ok := c.m[key]
	if !ok {
		return cache.ErrMiss
	}
	return json.Unmarshal(b, dst)
}

func (c *memCache) Set(_ context.Context, key string, v any, _ time.Duration) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[key] = b
	return nil
}

func (c *memCache) Delete(_ context.Context, keys ...string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, k := range keys {
		delete(c.m, k)
	}
	return nil
}

func (c *memCache) Incr(context.Context, string) (int64, error)   { return 0, nil }
func (c *memCache) GetInt(context.Context, string) (int64, error) { return 0, nil }

// NBK feed format as served by nationalbank.kz/rss/get_rates.cfm (values invented).
const nbkXML = `<?xml version="1.0" encoding="utf-8"?>
<rates>
    <generator>Alternate RSS Builder</generator>
    <title>Official exchange rates of National Bank of Republic Kazakhstan</title>
    <date>%s</date>
    <item><fullname>ДИРХАМ ОАЭ</fullname><title>AED</title><description>136.00</description><quant>1</quant><index></index><change>0.00</change></item>
    <item><fullname>АРМЯНСКИЙ ДРАМ</fullname><title>AMD</title><description>12.50</description><quant>10</quant><index></index><change>0.00</change></item>
    <item><fullname>ДОЛЛАР США</fullname><title>USD</title><description>500.00</description><quant>1</quant><index></index><change>0.00</change></item>
    <item><fullname>ЕВРО</fullname><title>EUR</title><description>550.00</description><quant>1</quant><index></index><change>0.00</change></item>
    <item><fullname>СДР</fullname><title>XDR</title><description>680.00</description><quant>1</quant><index></index><change>0.00</change></item>
</rates>`

const nbkEmpty = `<?xml version="1.0" encoding="utf-8"?>
<rates><date>%s</date><info>на выбранную дату информации нет.</info></rates>`

type providers struct {
	frank, nbk         *httptest.Server
	frankHits, nbkHits atomic.Int32
	down               atomic.Bool
	emptyDays          map[string]bool // DD.MM.YYYY without a publication
}

func newProviders(t *testing.T) *providers {
	p := &providers{emptyDays: map[string]bool{}}
	p.frank = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.frankHits.Add(1)
		if p.down.Load() {
			http.Error(w, "down", http.StatusBadGateway)
			return
		}
		if r.URL.Path == "/currencies" {
			fmt.Fprint(w, `{"USD":"United States Dollar","EUR":"Euro","ISK":"Icelandic Króna"}`)
			return
		}
		rates := map[string]map[string]float64{"EUR": {"USD": 1.1}, "ISK": {"USD": 0.008}}
		base, sym := r.URL.Query().Get("base"), r.URL.Query().Get("symbols")
		_ = json.NewEncoder(w).Encode(map[string]any{"rates": map[string]float64{sym: rates[base][sym]}})
	}))
	p.nbk = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.nbkHits.Add(1)
		if p.down.Load() {
			http.Error(w, "down", http.StatusServiceUnavailable)
			return
		}
		d := r.URL.Query().Get("fdate")
		if p.emptyDays[d] {
			fmt.Fprintf(w, nbkEmpty, d)
			return
		}
		fmt.Fprintf(w, nbkXML, d)
	}))
	t.Cleanup(p.frank.Close)
	t.Cleanup(p.nbk.Close)
	return p
}

func (p *providers) client() *Client {
	return New(p.frank.URL, newMemCache()).WithNBK(p.nbk.URL + "/rss/get_rates.cfm")
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestCurrenciesUnion(t *testing.T) {
	p := newProviders(t)
	c := p.client()
	list, err := c.Currencies(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, code := range []string{"USD", "EUR", "ISK", "KZT", "AED", "AMD"} {
		if list[code] == "" {
			t.Errorf("missing %s in %v", code, list)
		}
	}
	if _, ok := list["XDR"]; ok {
		t.Error("XDR must be excluded")
	}
	if list["EUR"] != "Euro" || list["AED"] != "UAE Dirham" {
		t.Errorf("names: %v", list)
	}
	if code, err := c.Validate(context.Background(), "kzt", "USD"); err != nil || code != "KZT" {
		t.Errorf("Validate(kzt) = %q, %v", code, err)
	}
}

func TestRateViaKZT(t *testing.T) {
	p := newProviders(t)
	c := p.client()
	ctx := context.Background()
	day := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		from, to string
		want     float64
	}{
		{"USD", "KZT", 500},
		{"KZT", "USD", 1.0 / 500},
		{"AED", "USD", 136.0 / 500},
		{"AMD", "KZT", 1.25}, // quant 10
		{"KZT", "AED", 1.0 / 136},
		{"ISK", "KZT", 0.008 * 500}, // not in NBK: ECB to USD, then NBK USD
		{"EUR", "USD", 1.1},         // both ECB: Frankfurter, not the NBK cross (1.1 vs 550/500)
	}
	for _, tc := range cases {
		got, err := c.Rate(ctx, tc.from, tc.to, day)
		if err != nil || !near(got, tc.want) {
			t.Errorf("Rate(%s->%s) = %v, %v; want %v", tc.from, tc.to, got, err, tc.want)
		}
	}
	if got, _ := c.Convert(ctx, 100000, "KZT", "USD", day); got != 200 { // 1000.00 ₸ -> $2.00
		t.Errorf("Convert = %d", got)
	}

	// Cached: no new provider calls; the NBK table is fetched once per day.
	hits := p.nbkHits.Load() + p.frankHits.Load()
	if _, err := c.Rate(ctx, "USD", "KZT", day); err != nil {
		t.Fatal(err)
	}
	if p.nbkHits.Load()+p.frankHits.Load() != hits {
		t.Error("cached rate hit the providers")
	}
}

func TestRateNBKFallsBackToEarlierDay(t *testing.T) {
	p := newProviders(t)
	p.emptyDays["17.01.2026"] = true // a holiday
	c := p.client()
	got, err := c.Rate(context.Background(), "AED", "KZT", time.Date(2026, 1, 17, 0, 0, 0, 0, time.UTC))
	if err != nil || !near(got, 136) {
		t.Fatalf("rate = %v, %v", got, err)
	}
}

func TestRateLastKnownWhenNBKDown(t *testing.T) {
	p := newProviders(t)
	c := p.client()
	ctx := context.Background()
	if _, err := c.Rate(ctx, "KZT", "USD", time.Time{}); err != nil {
		t.Fatal(err)
	}
	p.down.Store(true)
	got, err := c.Rate(ctx, "KZT", "USD", time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC))
	if err != nil || !near(got, 1.0/500) {
		t.Fatalf("fallback rate = %v, %v", got, err)
	}
	if _, err := c.Rate(ctx, "AED", "RUB", time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)); err == nil {
		t.Fatal("no rate ever seen: want error")
	}
}

func TestRateRoutingWithoutLists(t *testing.T) {
	// Both providers down at start: routing still sends NBK-only codes to NBK.
	c := New("http://127.0.0.1:1", newMemCache()).WithNBK("http://127.0.0.1:1")
	if c.viaFrankfurter(context.Background(), "AED", "USD") || !c.viaFrankfurter(context.Background(), "EUR", "USD") {
		t.Error("routing")
	}
	if !strings.HasPrefix(DefaultNBKURL, "https://nationalbank.kz/") {
		t.Error("default NBK url")
	}
}
