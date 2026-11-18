package ocrworker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Zifeldev/Billsplit/internal/parser/receipt"
	"github.com/Zifeldev/Billsplit/internal/platform/cache"
)

func TestNormalizeUPC(t *testing.T) {
	cases := map[string]string{
		"078742133710":  "078742133713",  // Walmart: 11 UPC digits + non-check digit
		"039978004780":  "039978004789",  // recomputed check digit
		"078742148380":  "078742148380",  // already a valid UPC-A
		"299689000000":  "",              // random-weight store code
		"000000004011":  "",              // produce PLU
		"3017620422003": "3017620422003", // valid EAN-13
		"12345":         "",
		"ABC123456789":  "",
	}
	for in, want := range cases {
		if got := normalizeUPC(in); got != want {
			t.Errorf("normalizeUPC(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestExpandAbbreviations(t *testing.T) {
	cases := map[string]string{
		"GV HH 320Z":  "Great Value HH 320Z", // OCR read "32OZ" as "320Z": left alone, no guessing
		"GV MILK 2%":  "Great Value MILK 2%",
		"EQ MTT 240":  "Equate MTT 240",
		"HH BRSH 4PC": "HH BRSH 4PC",
		"BANANAS 3LB": "BANANAS 3 lb",
	}
	for in, want := range cases {
		if got := expandAbbreviations(in); got != want {
			t.Errorf("expand(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestProductName(t *testing.T) {
	if got := productName("Great Value, Walmart", "Half & Half"); got != "Great Value Half & Half" {
		t.Error(got)
	}
	if got := productName("Jennie-O", "Jennie-O fresh ground turkey"); got != "Jennie-O fresh ground turkey" {
		t.Error(got)
	}
	long := productName("Kinder", "Joy Egg DC Funko Limited Edition Treat + Toy Egg Sweet Cream Topped with Cocoa Wafer Bites")
	if len([]rune(long)) > maxNameLen+1 || !strings.HasSuffix(long, "…") {
		t.Errorf("long name not shortened: %q", long)
	}
}

// memCache is a tiny concurrency-safe in-memory cache.Cache for tests
// (Enrich looks names up from several goroutines).
type memCache struct {
	mu sync.Mutex
	m  map[string]string
}

func (c *memCache) Get(_ context.Context, k string, dst any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.m[k]
	if !ok {
		return cache.ErrMiss
	}
	*(dst.(*string)) = v
	return nil
}
func (c *memCache) Set(_ context.Context, k string, v any, _ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[k] = v.(string)
	return nil
}
func (c *memCache) Delete(context.Context, ...string) error       { return nil }
func (c *memCache) Incr(context.Context, string) (int64, error)   { return 0, nil }
func (c *memCache) GetInt(context.Context, string) (int64, error) { return 0, nil }

func TestEnrich(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		switch {
		case strings.Contains(r.URL.Path, "078742133713"):
			_, _ = w.Write([]byte(`{"status":1,"product":{"product_name":"Cooked Shrimp","brands":"Great Value"}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	n := &Names{http: srv.Client(), cache: &memCache{m: map[string]string{}}, hosts: []string{srv.URL}}

	items := []receipt.Item{
		{Name: "GV 41-60 CK", RawName: "GV 41-60 CK", Code: "078742133710"},
		{Name: "GV DC INT CF", RawName: "GV DC INT CF", Code: "194346623490"}, // not in DB
		{Name: "BRAT BUN", RawName: "BRAT BUN", Code: "299689000000"},         // store code: no lookup
	}
	n.Enrich(context.Background(), items)
	want := []string{"Great Value Cooked Shrimp", "Great Value DC INT CF", "BRAT BUN"}
	for i, w := range want {
		if items[i].Name != w {
			t.Errorf("item %d name = %q, want %q", i, items[i].Name, w)
		}
	}
	if items[0].RawName != "GV 41-60 CK" {
		t.Errorf("raw name lost: %q", items[0].RawName)
	}
	before := calls.Load()
	n.Enrich(context.Background(), items[:2]) // cached, including the negative result
	if calls.Load() != before {
		t.Errorf("expected cache hits, got %d extra calls", calls.Load()-before)
	}
}
