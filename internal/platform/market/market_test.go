package market

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
	b, _ := json.Marshal(v)
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

// Shape of Yahoo's v8 chart response (values invented). The last bar is the
// live session duplicating the previous date; one close is null (holiday).
const chartJSON = `{"chart":{"result":[{"meta":{"currency":"GBp","symbol":"VUSA.L","longName":"Vanguard S&P 500",
 "regularMarketPrice":9150,"gmtoffset":0},
 "timestamp":[1767225600,1767312000,1767398400,1767484800,1767488400],
 "indicators":{"quote":[{"close":[9000,null,9100,9120,9150]}]}}],"error":null}}`

func TestChartParsesAndConvertsSubunits(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.Header.Get("User-Agent") == "" {
			t.Error("no User-Agent")
		}
		if r.URL.Path == "/v8/finance/chart/NOPE" {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"chart":{"result":null,"error":{"code":"Not Found"}}}`))
			return
		}
		_, _ = w.Write([]byte(chartJSON))
	}))
	defer srv.Close()
	c := New(srv.URL, &memCache{m: map[string][]byte{}})

	ch, err := c.Chart(context.Background(), " vusa.l ", "1mo")
	if err != nil {
		t.Fatal(err)
	}
	if ch.Currency != "GBP" || ch.Price != 91.5 || ch.Name != "Vanguard S&P 500" {
		t.Fatalf("meta: %+v", ch)
	}
	if len(ch.Points) != 3 || ch.Points[2].Close != 91.5 || ch.Points[0].Date != "2026-01-01" {
		t.Fatalf("points: %+v", ch.Points)
	}
	if ch.PrevClose != 91 || ch.CloseAt("2026-01-02") != 90 || ch.CloseAt("2025-12-31") != 0 {
		t.Fatalf("prev/closeAt: %+v", ch)
	}
	if _, err := c.Chart(context.Background(), "VUSA.L", "1mo"); err != nil || hits.Load() != 1 {
		t.Fatalf("second call should be cached: hits=%d err=%v", hits.Load(), err)
	}
	if _, err := c.Chart(context.Background(), "NOPE", "1mo"); err != ErrUnknownSymbol {
		t.Fatalf("unknown symbol err = %v", err)
	}
	if _, err := c.Chart(context.Background(), "AA PL", "1mo"); err != ErrUnknownSymbol {
		t.Fatalf("bad symbol err = %v", err)
	}
}
