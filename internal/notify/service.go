package notify

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/segmentio/kafka-go"

	"github.com/Zifeldev/Billsplit/internal/domain"
	"github.com/Zifeldev/Billsplit/internal/platform/auth"
	"github.com/Zifeldev/Billsplit/internal/platform/logger"
)

var (
	pushSent = promauto.NewCounter(prometheus.CounterOpts{
		Name: "billsplit_push_sent_total",
		Help: "Expo push messages accepted by Expo (ticket status ok).",
	})
	pushFailed = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "billsplit_push_failed_total",
		Help: "Expo push messages that failed, by reason (Expo error code or transport).",
	}, []string{"reason"})
	notificationsCreated = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "billsplit_notifications_created_total",
		Help: "Inbox notifications stored, by type.",
	}, []string{"type"})
)

const (
	defaultListLimit = 30
	maxListLimit     = 100
	maxMarkIDs       = 500
	maxTokenLen      = 256
)

var expoTokenRe = regexp.MustCompile(`^(ExponentPushToken|ExpoPushToken)\[[A-Za-z0-9_\-:.]+\]$`)

var platforms = map[string]bool{"ios": true, "android": true, "web": true}

type Service struct {
	repo   Repository
	pusher Pusher
}

func NewService(repo Repository, pusher Pusher) *Service {
	return &Service{repo: repo, pusher: pusher}
}

// ValidToken reports whether token looks like an Expo push token.
func ValidToken(token string) bool {
	return len(token) <= maxTokenLen && expoTokenRe.MatchString(token)
}

func (s *Service) RegisterDevice(ctx context.Context, token, platform string) error {
	uid, err := auth.UserID(ctx)
	if err != nil {
		return err
	}
	if !ValidToken(token) {
		return fmt.Errorf("%w: token must be ExponentPushToken[...]", domain.ErrInvalidArgument)
	}
	if !platforms[platform] {
		return fmt.Errorf("%w: platform must be ios, android or web", domain.ErrInvalidArgument)
	}
	return s.repo.UpsertDevice(ctx, Device{Token: token, UserID: uid, Platform: platform})
}

// UnregisterDevice removes the caller's token. Unknown or foreign tokens are a no-op.
func (s *Service) UnregisterDevice(ctx context.Context, token string) error {
	uid, err := auth.UserID(ctx)
	if err != nil {
		return err
	}
	if token == "" {
		return fmt.Errorf("%w: token is required", domain.ErrInvalidArgument)
	}
	return s.repo.DeleteDevice(ctx, uid, token)
}

func (s *Service) List(ctx context.Context, limit int, unreadOnly bool) ([]Notification, int, error) {
	uid, err := auth.UserID(ctx)
	if err != nil {
		return nil, 0, err
	}
	if limit <= 0 {
		limit = defaultListLimit
	}
	limit = min(limit, maxListLimit)
	items, err := s.repo.List(ctx, uid, limit, unreadOnly)
	if err != nil {
		return nil, 0, err
	}
	unread, err := s.repo.UnreadCount(ctx, uid)
	if err != nil {
		return nil, 0, err
	}
	return items, unread, nil
}

func (s *Service) MarkRead(ctx context.Context, ids []string) error {
	uid, err := auth.UserID(ctx)
	if err != nil {
		return err
	}
	if len(ids) > maxMarkIDs {
		return fmt.Errorf("%w: at most %d ids", domain.ErrInvalidArgument, maxMarkIDs)
	}
	for _, id := range ids {
		if _, err := uuid.Parse(id); err != nil {
			return fmt.Errorf("%w: bad notification id %q", domain.ErrInvalidArgument, id)
		}
	}
	return s.repo.MarkRead(ctx, uid, ids)
}

// DedupeKey identifies one (Kafka message, user) pair, so redelivery of the
// same message never creates a second inbox row. It includes the message
// coordinates and value, so identical payloads sent twice are still separate.
func DedupeKey(msg kafka.Message, userID string) string {
	h := sha256.New()
	h.Write([]byte(msg.Topic))
	h.Write([]byte{0})
	h.Write([]byte(strconv.Itoa(msg.Partition)))
	h.Write([]byte{0})
	h.Write([]byte(strconv.FormatInt(msg.Offset, 10)))
	h.Write([]byte{0})
	h.Write(msg.Value)
	h.Write([]byte{0})
	h.Write([]byte(userID))
	return hex.EncodeToString(h.Sum(nil))
}

// HandleMessage is the Kafka handler for domain.TopicNotification. It stores one
// inbox row per user (idempotently), then pushes to the users' devices.
// DB errors are returned (the consumer retries); push errors are only logged.
func (s *Service) HandleMessage(ctx context.Context, msg kafka.Message) error {
	log := logger.FromContext(ctx).With("topic", msg.Topic, "partition", msg.Partition, "offset", msg.Offset)
	var ev domain.NotificationEvent
	if err := json.Unmarshal(msg.Value, &ev); err != nil {
		log.Error("bad notification event, skipping", "err", err)
		return nil
	}
	if ev.Title == "" || ev.Type == "" {
		log.Warn("notification without type/title, skipping")
		return nil
	}

	var created []*Notification
	seen := map[string]bool{}
	for _, uid := range ev.UserIDs {
		if seen[uid] {
			continue
		}
		seen[uid] = true
		if _, err := uuid.Parse(uid); err != nil {
			log.Warn("bad user id in notification, skipping", "user_id", uid)
			continue
		}
		n := &Notification{UserID: uid, Type: ev.Type, Title: ev.Title, Body: ev.Body, Data: ev.Data}
		inserted, err := s.repo.Insert(ctx, n, DedupeKey(msg, uid))
		if err != nil {
			return fmt.Errorf("insert notification: %w", err)
		}
		if inserted {
			notificationsCreated.WithLabelValues(ev.Type).Inc()
			created = append(created, n)
		}
	}
	if len(created) == 0 || s.pusher == nil {
		return nil
	}
	s.push(logger.WithContext(ctx, log), created)
	return nil
}

// push is best effort: the inbox row already exists.
func (s *Service) push(ctx context.Context, ns []*Notification) {
	log := logger.FromContext(ctx)
	uids := make([]string, len(ns))
	for i, n := range ns {
		uids[i] = n.UserID
	}
	tokens, err := s.repo.TokensByUsers(ctx, uids)
	if err != nil {
		log.Warn("load push tokens", "err", err)
		pushFailed.WithLabelValues("db").Add(float64(len(ns)))
		return
	}
	var msgs []PushMessage
	for _, n := range ns {
		data := make(map[string]string, len(n.Data)+2)
		for k, v := range n.Data {
			data[k] = v
		}
		data["type"] = n.Type
		data["notification_id"] = n.ID
		for _, tok := range tokens[n.UserID] {
			msgs = append(msgs, PushMessage{To: tok, Title: n.Title, Body: n.Body, Data: data, Sound: "default"})
		}
	}
	if len(msgs) == 0 {
		return
	}

	var dead []string
	for _, r := range s.pusher.Send(ctx, msgs) {
		if r.OK {
			pushSent.Inc()
			continue
		}
		pushFailed.WithLabelValues(reasonLabel(r)).Inc()
		if r.Unregistered {
			dead = append(dead, r.Token)
		} else {
			log.Warn("push failed", "err", r.Error)
		}
	}
	if len(dead) > 0 {
		if err := s.repo.DeleteTokens(ctx, dead); err != nil {
			log.Warn("delete unregistered tokens", "err", err)
		} else {
			log.Info("removed unregistered push tokens", "count", len(dead))
		}
	}
}

// reasonLabel keeps the metric's label set small and bounded.
func reasonLabel(r PushResult) string {
	switch r.Error {
	case "DeviceNotRegistered", "MessageTooBig", "MessageRateExceeded", "MismatchSenderId", "InvalidCredentials":
		return r.Error
	}
	return "other"
}
