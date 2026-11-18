package finance

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/Zifeldev/Billsplit/internal/domain"
	"github.com/Zifeldev/Billsplit/internal/platform/logger"
)

// Recurring is the ListRecurring result in the user's home currency.
type Recurring struct {
	Currency          string
	Items             []RecurringCharge
	MonthlyTotalCents int64
}

// ListRecurring detects subscriptions and other repeating charges of the current user.
func (s *Service) ListRecurring(ctx context.Context) (*Recurring, error) {
	uid, home, err := s.home(ctx)
	if err != nil {
		return nil, err
	}
	return s.recurring(ctx, uid, home, time.Now().UTC())
}

// recurring is cached under the dashboard version key, so any transaction write invalidates it.
func (s *Service) recurring(ctx context.Context, uid, home string, now time.Time) (*Recurring, error) {
	ver, _ := s.cache.GetInt(ctx, dashVersionKey(uid))
	key := fmt.Sprintf("finance:recurring:%s:%s:%d:%s", uid, home, ver, now.Format(time.DateOnly))
	var r Recurring
	if err := s.cache.Get(ctx, key, &r); err == nil {
		return &r, nil
	}

	txs, err := s.repo.Expenses(ctx, uid, now.Add(-RecurringLookback))
	if err != nil {
		return nil, err
	}
	// Convert at today's rate: the question is "what will it cost me now".
	cv := &converter{ctx: ctx, fx: s.fx, home: home, date: now, rates: map[string]float64{}}
	for i := range txs {
		if txs[i].AmountCents, err = cv.do(txs[i].AmountCents, txs[i].Currency); err != nil {
			return nil, err
		}
		txs[i].Currency = home
	}
	items := DetectRecurring(txs, now)
	rp := &Recurring{Currency: home, Items: items, MonthlyTotalCents: MonthlyTotal(items)}
	if err := s.cache.Set(ctx, key, rp, s.ttl); err != nil {
		logger.FromContext(ctx).Warn("cache set failed", "err", err)
	}
	return rp, nil
}

const (
	reminderInterval  = time.Hour
	reminderBatch     = 200
	reminderActiveFor = 40 * 24 * time.Hour
	reminderDedupTTL  = 3 * 24 * time.Hour
)

// RunRecurringReminders notifies users about recurring charges expected tomorrow,
// once per hour until ctx is cancelled. Safe to run on several replicas: each
// (user, merchant, date) is claimed with SETNX before publishing.
func (s *Service) RunRecurringReminders(ctx context.Context, log *slog.Logger) error {
	log = log.With("component", "recurring-reminders")
	ctx = logger.WithContext(ctx, log)
	t := time.NewTicker(reminderInterval)
	defer t.Stop()
	for {
		if n, err := s.SendRecurringReminders(ctx, time.Now().UTC()); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			log.Warn("recurring reminders failed", "err", err)
		} else if n > 0 {
			log.Info("recurring reminders sent", "count", n)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
		}
	}
}

// SendRecurringReminders runs one pass and returns how many notifications were queued.
func (s *Service) SendRecurringReminders(ctx context.Context, now time.Time) (int, error) {
	log := logger.FromContext(ctx)
	sent := 0
	after := ""
	for {
		uids, err := s.repo.UsersWithExpenses(ctx, now.Add(-reminderActiveFor), after, reminderBatch)
		if err != nil {
			return sent, err
		}
		for _, uid := range uids {
			if ctx.Err() != nil {
				return sent, ctx.Err()
			}
			n, err := s.remindUser(ctx, uid, now)
			if err != nil {
				log.Warn("recurring reminder for user failed", "user_id", uid, "err", err)
				continue
			}
			sent += n
		}
		if len(uids) < reminderBatch {
			return sent, nil
		}
		after = uids[len(uids)-1]
	}
}

func (s *Service) remindUser(ctx context.Context, uid string, now time.Time) (int, error) {
	u, err := s.users.GetUser(ctx, uid)
	if err != nil {
		return 0, err
	}
	rec, err := s.recurring(ctx, uid, u.Currency, now)
	if err != nil {
		return 0, err
	}
	tomorrow := now.AddDate(0, 0, 1).Format(time.DateOnly)
	sent := 0
	for _, it := range rec.Items {
		if it.NextExpectedAt.UTC().Format(time.DateOnly) != tomorrow {
			continue
		}
		key := fmt.Sprintf("finance:recurring:notified:%s:%s:%s", uid, NormalizeMerchant(it.Merchant), tomorrow)
		ok, err := s.locker.SetNX(ctx, key, reminderDedupTTL)
		if err != nil {
			return sent, err
		}
		if !ok {
			continue // already notified (another replica or before a restart)
		}
		ev := domain.NotificationEvent{
			UserIDs: []string{uid},
			Type:    domain.NotifyRecurringUpcoming,
			Title:   "Upcoming charge",
			Body:    fmt.Sprintf("%s will charge ~%s tomorrow", displayMerchant(it.Merchant), FormatMoney(it.AverageCents, rec.Currency)),
			Data: map[string]string{
				"merchant":    it.Merchant,
				"category_id": it.CategoryID,
				"date":        tomorrow,
			},
		}
		if err := s.pub.Publish(ctx, domain.TopicNotification, uid, ev); err != nil {
			_ = s.cache.Delete(ctx, key) // let the next pass retry
			return sent, err
		}
		sent++
	}
	return sent, nil
}

// displayMerchant turns "NETFLIX.COM" into "Netflix" for notification text.
func displayMerchant(m string) string {
	m = strings.TrimSpace(m)
	if m == "" || strings.ToUpper(m) != m {
		return m
	}
	words := strings.Fields(domainTLD.ReplaceAllString(strings.ToLower(m), ""))
	if len(words) == 0 {
		return m
	}
	for i, w := range words {
		r := []rune(w)
		words[i] = strings.ToUpper(string(r[0])) + string(r[1:])
	}
	return strings.Join(words, " ")
}

// RedisLocker implements Locker with SET NX.
type RedisLocker struct{ rdb *redis.Client }

func NewRedisLocker(rdb *redis.Client) *RedisLocker { return &RedisLocker{rdb: rdb} }

func (l *RedisLocker) SetNX(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	return l.rdb.SetNX(ctx, key, 1, ttl).Result()
}
