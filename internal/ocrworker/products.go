package ocrworker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Zifeldev/Billsplit/internal/parser/receipt"
	"github.com/Zifeldev/Billsplit/internal/platform/cache"
	"github.com/Zifeldev/Billsplit/internal/platform/logger"
	"github.com/Zifeldev/Billsplit/internal/platform/metrics"
)

// Receipts from US chains print cryptic abbreviations ("GV HH 320Z") but also
// the product code. Names turns codes into readable names using the free Open
// Food Facts / Open Products Facts databases, cached in Redis, and falls back to
// expanding well-known store-brand prefixes.
type Names struct {
	http  *http.Client
	cache cache.Cache
	hosts []string
}

func NewNames(c cache.Cache) *Names {
	return &Names{
		http:  &http.Client{Timeout: 4 * time.Second},
		cache: c,
		hosts: []string{"https://world.openfoodfacts.org", "https://world.openproductsfacts.org"},
	}
}

const (
	namesBudget     = 8 * time.Second // per receipt: never delay OCR results much
	namesParallel   = 4               // OFF asks for ≤ 100 product reads/minute
	nameTTL         = 30 * 24 * time.Hour
	negativeNameTTL = 7 * 24 * time.Hour
	maxNameLen      = 60
)

var errRateLimited = errors.New("product database rate limit")

// Enrich sets readable names in place. Raw names stay in RawName.
func (n *Names) Enrich(ctx context.Context, items []receipt.Item) {
	ctx, cancel := context.WithTimeout(ctx, namesBudget)
	defer cancel()

	var (
		wg      sync.WaitGroup
		sem     = make(chan struct{}, namesParallel)
		mu      sync.Mutex
		limited bool
	)
	for i := range items {
		it := &items[i]
		if it.RawName == "" {
			it.RawName = it.Name
		}
		upc := normalizeUPC(it.Code)
		if upc == "" {
			it.Name = expandAbbreviations(it.RawName)
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			mu.Lock()
			skip := limited
			mu.Unlock()
			name := ""
			if !skip {
				var err error
				name, err = n.lookup(ctx, upc)
				if errors.Is(err, errRateLimited) {
					mu.Lock()
					limited = true
					mu.Unlock()
				}
			}
			if name == "" {
				name = expandAbbreviations(it.RawName)
			}
			it.Name = name
		}()
	}
	wg.Wait()
}

func (n *Names) lookup(ctx context.Context, upc string) (string, error) {
	key := "product:" + upc
	var cached string
	if err := n.cache.Get(ctx, key, &cached); err == nil {
		metrics.OCRRequests.WithLabelValues("product_db", "cache").Inc()
		return cached, nil
	}
	for _, host := range n.hosts {
		name, err := n.fetch(ctx, host, upc)
		if err != nil {
			metrics.OCRRequests.WithLabelValues("product_db", "error").Inc()
			logger.FromContext(ctx).Debug("product lookup failed", "upc", upc, "err", err)
			return "", err // transient: don't cache
		}
		if name != "" {
			metrics.OCRRequests.WithLabelValues("product_db", "found").Inc()
			_ = n.cache.Set(ctx, key, name, nameTTL)
			return name, nil
		}
	}
	metrics.OCRRequests.WithLabelValues("product_db", "not_found").Inc()
	_ = n.cache.Set(ctx, key, "", negativeNameTTL)
	return "", nil
}

func (n *Names) fetch(ctx context.Context, host, upc string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("%s/api/v2/product/%s.json?fields=product_name,brands", host, upc), nil)
	if err != nil {
		return "", err
	}
	// Open Food Facts asks API clients to identify themselves.
	req.Header.Set("User-Agent", "Billsplit/1.0 (receipt item names; github.com/Zifeldev/Billsplit)")
	resp, err := n.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return "", nil
	case resp.StatusCode == http.StatusTooManyRequests:
		return "", errRateLimited
	case resp.StatusCode != http.StatusOK:
		return "", fmt.Errorf("%s: status %d", host, resp.StatusCode)
	}
	var out struct {
		Status  int `json:"status"`
		Product struct {
			Name   string `json:"product_name"`
			Brands string `json:"brands"`
		} `json:"product"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if out.Status != 1 {
		return "", nil
	}
	return productName(out.Product.Brands, out.Product.Name), nil
}

// productName builds "Brand Product", without repeating the brand, ≤ maxNameLen runes.
func productName(brands, name string) string {
	name = strings.Join(strings.Fields(name), " ")
	if name == "" {
		return ""
	}
	brand := strings.TrimSpace(strings.Split(brands, ",")[0])
	if brand != "" && !strings.HasPrefix(strings.ToLower(name), strings.ToLower(brand)) {
		name = brand + " " + name
	}
	if r := []rune(name); len(r) > maxNameLen {
		cut := string(r[:maxNameLen])
		if i := strings.LastIndex(cut, " "); i > maxNameLen/2 {
			cut = cut[:i]
		}
		name = strings.TrimRight(cut, " ,-&") + "…"
	}
	return name
}

// normalizeUPC turns a printed receipt code into a GTIN for lookup.
// Walmart prints 12 digits: the first 11 digits of the UPC-A followed by a
// non-check digit, so the check digit is recomputed. Codes starting with 2
// are in-store random-weight/PLU codes with no product behind them.
func normalizeUPC(code string) string {
	// Produce PLUs are printed zero-padded ("000000004011"): nothing to look up.
	if len(code) < 11 || !allDigits(code) || strings.HasPrefix(code, "0000000") {
		return ""
	}
	switch len(code) {
	case 12:
		if code[0] == '2' || code[1] == '2' && code[0] == '0' {
			return ""
		}
		if validCheckDigit(code) {
			return code
		}
		return code[:11] + checkDigit(code[:11])
	case 13:
		if validCheckDigit(code) {
			return code
		}
	}
	return ""
}

func allDigits(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// checkDigit computes the GTIN check digit for the given body (UPC-A: 11 digits, EAN-13: 12).
func checkDigit(body string) string {
	sum := 0
	for i := range len(body) {
		d := int(body[len(body)-1-i] - '0')
		if i%2 == 0 {
			d *= 3
		}
		sum += d
	}
	return string(rune('0' + (10-sum%10)%10))
}

func validCheckDigit(code string) bool {
	return checkDigit(code[:len(code)-1]) == code[len(code)-1:]
}

// Store-brand prefixes common on US receipts. Only whole leading tokens are
// expanded, so "4PC" or "GVX" stay untouched.
var brandPrefixes = map[string]string{
	"GV": "Great Value",
	"EQ": "Equate",
	"MS": "Mainstays",
	"PC": "Parent's Choice",
	"OT": "Ozark Trail",
	"HT": "Hyper Tough",
	"SB": "Sam's Choice",
	"MM": "Member's Mark",
	"KS": "Kirkland Signature",
	"UP": "Up&Up",
}

var unitRe = regexp.MustCompile(`\b(\d+(?:\.\d+)?)\s*(OZ|LB|CT|PK|ML|L|G|KG|GAL|QT|FZ)\b`)

func expandAbbreviations(raw string) string {
	fields := strings.Fields(raw)
	if len(fields) == 0 {
		return raw
	}
	if full, ok := brandPrefixes[strings.ToUpper(fields[0])]; ok {
		fields[0] = full
	}
	s := strings.Join(fields, " ")
	return unitRe.ReplaceAllStringFunc(s, func(m string) string {
		sm := unitRe.FindStringSubmatch(m)
		return sm[1] + " " + strings.ToLower(sm[2])
	})
}
