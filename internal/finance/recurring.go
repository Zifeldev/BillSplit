package finance

import (
	"math"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/Zifeldev/Billsplit/internal/domain"
)

type RecurringPeriod int

const (
	PeriodUnknown RecurringPeriod = iota
	PeriodWeekly
	PeriodMonthly
	PeriodYearly
)

// RecurringCharge is a repeating expense detected in the user's transactions.
// Amounts are positive, in the currency the input transactions were converted to.
type RecurringCharge struct {
	Merchant        string
	CategoryID      string
	AverageCents    int64
	Period          RecurringPeriod
	Occurrences     int
	LastChargedAt   time.Time
	NextExpectedAt  time.Time
	YearlyCostCents int64
	IsSubscription  bool
}

// periodRule is the accepted range of days between two charges.
type periodRule struct {
	period         RecurringPeriod
	minDays        float64
	maxDays        float64
	minOccurrences int
	perYear        int64
}

var periodRules = []periodRule{
	{PeriodWeekly, 6, 8, 3, 52},
	{PeriodMonthly, 27, 33, 3, 12},
	{PeriodYearly, 350, 380, 2, 1},
}

// RecurringLookback is how far back detection looks (13 months catches yearly charges).
const RecurringLookback = 396 * 24 * time.Hour

// amountTolerance: every charge must be within ±20% of the median (price changes are fine).
const amountTolerance = 0.20

// knownSaaS are merchants that are subscriptions even if categorized otherwise.
var knownSaaS = []string{
	"netflix", "spotify", "hulu", "disney", "hbo", "max.com", "youtube", "apple.com", "icloud", "itunes",
	"google", "microsoft", "xbox", "playstation", "nintendo", "adobe", "dropbox", "chatgpt", "openai",
	"anthropic", "claude", "github", "notion", "slack", "zoom", "canva", "figma", "audible", "kindle",
	"amazon prime", "prime video", "paramount", "peacock", "crunchyroll", "patreon", "onlyfans", "duolingo",
	"1password", "lastpass", "nordvpn", "expressvpn", "grammarly", "linkedin", "twitch", "tidal", "deezer",
	"yandex plus", "headspace", "calm", "strava", "peloton",
}

var (
	// Tokens with digits are store numbers, phone numbers, references: "NETFLIX.COM 866-579-7172".
	digitToken  = regexp.MustCompile(`\S*\d\S*`)
	nonWord     = regexp.MustCompile(`[^\p{L}\p{N}.&' ]+`)
	multiSpace  = regexp.MustCompile(`\s+`)
	paymentPfx  = regexp.MustCompile(`^(sq|tst|pp|paypal|sp|pos|recurring|debit)\s*\*?\s+`)
	legalSuffix = regexp.MustCompile(`\s+(inc|llc|ltd|co|corp|gmbh)\.?$`)
	domainTLD   = regexp.MustCompile(`\.(com|net|org|io|tv|co\.uk)\b`)
)

// NormalizeMerchant reduces bank noise so charges from one merchant group together:
// "PAYPAL *NETFLIX.COM 866-579", "Netflix.com" and "NETFLIX" all become "netflix".
func NormalizeMerchant(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "*", " * ")
	s = digitToken.ReplaceAllString(s, " ")
	s = nonWord.ReplaceAllString(s, " ")
	s = multiSpace.ReplaceAllString(strings.TrimSpace(s), " ")
	for {
		t := paymentPfx.ReplaceAllString(s, "")
		if t == s {
			break
		}
		s = t
	}
	s = domainTLD.ReplaceAllString(s, "")
	s = strings.TrimSpace(strings.Trim(s, ". "))
	return strings.TrimSpace(legalSuffix.ReplaceAllString(s, ""))
}

func isSubscription(categoryID, key string) bool {
	if categoryID == "subscriptions" {
		return true
	}
	for _, m := range knownSaaS {
		if strings.Contains(key, m) {
			return true
		}
	}
	return false
}

// DetectRecurring finds repeating expenses. All transactions must already be in
// one currency (the caller converts them). Income and transfers are ignored.
// Series that stopped (no charge for two periods) are dropped as cancelled.
// Result is sorted by yearly cost, most expensive first.
func DetectRecurring(txs []domain.Transaction, now time.Time) []RecurringCharge {
	groups := map[string][]domain.Transaction{}
	for _, t := range txs {
		if t.AmountCents >= 0 || t.CategoryID == "transfer" || t.CategoryID == "income" {
			continue
		}
		name := t.Merchant
		if strings.TrimSpace(name) == "" {
			name = t.Description
		}
		key := NormalizeMerchant(name)
		if key == "" {
			continue
		}
		groups[key] = append(groups[key], t)
	}

	var out []RecurringCharge
	for key, g := range groups {
		if rc, ok := detectSeries(key, g, now); ok {
			out = append(out, rc)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].YearlyCostCents != out[j].YearlyCostCents {
			return out[i].YearlyCostCents > out[j].YearlyCostCents
		}
		return out[i].Merchant < out[j].Merchant
	})
	return out
}

func detectSeries(key string, g []domain.Transaction, now time.Time) (RecurringCharge, bool) {
	if len(g) < 2 {
		return RecurringCharge{}, false
	}
	sort.Slice(g, func(i, j int) bool { return g[i].OccurredAt.Before(g[j].OccurredAt) })

	intervals := make([]float64, 0, len(g)-1)
	for i := 1; i < len(g); i++ {
		intervals = append(intervals, g[i].OccurredAt.Sub(g[i-1].OccurredAt).Hours()/24)
	}
	medInterval := median(intervals)

	var rule *periodRule
	for i := range periodRules {
		if medInterval >= periodRules[i].minDays && medInterval <= periodRules[i].maxDays {
			rule = &periodRules[i]
			break
		}
	}
	if rule == nil || len(g) < rule.minOccurrences {
		return RecurringCharge{}, false
	}
	for _, d := range intervals {
		if d < rule.minDays || d > rule.maxDays {
			return RecurringCharge{}, false
		}
	}

	amounts := make([]float64, len(g))
	var sum int64
	for i, t := range g {
		amounts[i] = float64(-t.AmountCents)
		sum += -t.AmountCents
	}
	medAmount := median(amounts)
	for _, a := range amounts {
		if math.Abs(a-medAmount) > medAmount*amountTolerance {
			return RecurringCharge{}, false
		}
	}

	last := g[len(g)-1]
	step := time.Duration(medInterval * float64(24*time.Hour)).Round(24 * time.Hour)
	next := last.OccurredAt.Add(step)
	// Cancelled: two expected charges missed.
	if now.Sub(last.OccurredAt) > 2*time.Duration(rule.maxDays*float64(24*time.Hour)) {
		return RecurringCharge{}, false
	}

	avg := int64(math.Round(float64(sum) / float64(len(g))))
	name := strings.TrimSpace(last.Merchant)
	if name == "" {
		name = strings.TrimSpace(last.Description)
	}
	return RecurringCharge{
		Merchant:        name,
		CategoryID:      last.CategoryID,
		AverageCents:    avg,
		Period:          rule.period,
		Occurrences:     len(g),
		LastChargedAt:   last.OccurredAt,
		NextExpectedAt:  next,
		YearlyCostCents: avg * rule.perYear,
		IsSubscription:  isSubscription(last.CategoryID, key+" "+strings.ToLower(name)),
	}, true
}

func median(xs []float64) float64 {
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	n := len(s)
	if n == 0 {
		return 0
	}
	if n%2 == 1 {
		return s[n/2]
	}
	return (s[n/2-1] + s[n/2]) / 2
}

// MonthlyTotal is the average monthly cost of all recurring charges.
func MonthlyTotal(rs []RecurringCharge) int64 {
	var yearly int64
	for _, r := range rs {
		yearly += r.YearlyCostCents
	}
	return int64(math.Round(float64(yearly) / 12))
}
