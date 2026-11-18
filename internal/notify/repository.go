// Package notify is the notification microservice: Expo push devices and the
// in-app inbox. It consumes domain.TopicNotification from Kafka.
package notify

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Zifeldev/Billsplit/internal/platform/postgres"
)

// Notification is one inbox entry.
type Notification struct {
	ID        string
	UserID    string
	Type      string
	Title     string
	Body      string
	Data      map[string]string
	Read      bool
	CreatedAt time.Time
}

// Device is a registered push token.
type Device struct {
	Token    string
	UserID   string
	Platform string
}

type Repository interface {
	UpsertDevice(ctx context.Context, d Device) error
	// DeleteDevice removes the token only if it belongs to userID.
	DeleteDevice(ctx context.Context, userID, token string) error
	// DeleteTokens removes tokens regardless of owner (Expo said they are dead).
	DeleteTokens(ctx context.Context, tokens []string) error
	TokensByUsers(ctx context.Context, userIDs []string) (map[string][]string, error)

	// Insert stores n with the dedupe key; returns false if it was already stored.
	Insert(ctx context.Context, n *Notification, dedupeKey string) (bool, error)
	List(ctx context.Context, userID string, limit int, unreadOnly bool) ([]Notification, error)
	UnreadCount(ctx context.Context, userID string) (int, error)
	// MarkRead marks the given ids (or all, when ids is empty) of userID as read.
	MarkRead(ctx context.Context, userID string, ids []string) error
}

type PGRepo struct{ postgres.DB }

func NewRepo(p *pgxpool.Pool) *PGRepo { return &PGRepo{postgres.DB{Pool: p}} }

func (r *PGRepo) UpsertDevice(ctx context.Context, d Device) error {
	_, err := r.Q(ctx).Exec(ctx,
		`INSERT INTO devices (token, user_id, platform) VALUES ($1, $2, $3)
		 ON CONFLICT (token) DO UPDATE
		 SET user_id = EXCLUDED.user_id, platform = EXCLUDED.platform, last_seen_at = now()`,
		d.Token, d.UserID, d.Platform)
	return postgres.MapErr(err)
}

func (r *PGRepo) DeleteDevice(ctx context.Context, userID, token string) error {
	_, err := r.Q(ctx).Exec(ctx, `DELETE FROM devices WHERE token = $1 AND user_id = $2`, token, userID)
	return postgres.MapErr(err)
}

func (r *PGRepo) DeleteTokens(ctx context.Context, tokens []string) error {
	if len(tokens) == 0 {
		return nil
	}
	_, err := r.Q(ctx).Exec(ctx, `DELETE FROM devices WHERE token = ANY($1)`, tokens)
	return postgres.MapErr(err)
}

func (r *PGRepo) TokensByUsers(ctx context.Context, userIDs []string) (map[string][]string, error) {
	out := map[string][]string{}
	if len(userIDs) == 0 {
		return out, nil
	}
	rows, err := r.Q(ctx).Query(ctx,
		`SELECT user_id::text, token FROM devices WHERE user_id = ANY($1::uuid[]) ORDER BY user_id, created_at`, userIDs)
	if err != nil {
		return nil, postgres.MapErr(err)
	}
	defer rows.Close()
	for rows.Next() {
		var uid, tok string
		if err := rows.Scan(&uid, &tok); err != nil {
			return nil, err
		}
		out[uid] = append(out[uid], tok)
	}
	return out, rows.Err()
}

func (r *PGRepo) Insert(ctx context.Context, n *Notification, dedupeKey string) (bool, error) {
	data := n.Data
	if data == nil {
		data = map[string]string{}
	}
	b, err := json.Marshal(data)
	if err != nil {
		return false, err
	}
	err = r.Q(ctx).QueryRow(ctx,
		`INSERT INTO notifications (user_id, type, title, body, data, dedupe_key)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (dedupe_key) DO NOTHING
		 RETURNING id::text, created_at`,
		n.UserID, n.Type, n.Title, n.Body, b, dedupeKey).Scan(&n.ID, &n.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, postgres.MapErr(err)
	}
	return true, nil
}

func (r *PGRepo) List(ctx context.Context, userID string, limit int, unreadOnly bool) ([]Notification, error) {
	rows, err := r.Q(ctx).Query(ctx,
		`SELECT id::text, user_id::text, type, title, body, data, read, created_at
		 FROM notifications
		 WHERE user_id = $1 AND (NOT $2 OR NOT read)
		 ORDER BY created_at DESC, id DESC
		 LIMIT $3`, userID, unreadOnly, limit)
	if err != nil {
		return nil, postgres.MapErr(err)
	}
	defer rows.Close()
	var out []Notification
	for rows.Next() {
		var n Notification
		var raw []byte
		if err := rows.Scan(&n.ID, &n.UserID, &n.Type, &n.Title, &n.Body, &raw, &n.Read, &n.CreatedAt); err != nil {
			return nil, err
		}
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &n.Data) // non-string values are dropped
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (r *PGRepo) UnreadCount(ctx context.Context, userID string) (int, error) {
	var n int
	err := r.Q(ctx).QueryRow(ctx, `SELECT count(*) FROM notifications WHERE user_id = $1 AND NOT read`, userID).Scan(&n)
	return n, postgres.MapErr(err)
}

func (r *PGRepo) MarkRead(ctx context.Context, userID string, ids []string) error {
	var err error
	if len(ids) == 0 {
		_, err = r.Q(ctx).Exec(ctx, `UPDATE notifications SET read = true WHERE user_id = $1 AND NOT read`, userID)
	} else {
		_, err = r.Q(ctx).Exec(ctx,
			`UPDATE notifications SET read = true WHERE user_id = $1 AND id = ANY($2::uuid[]) AND NOT read`, userID, ids)
	}
	return postgres.MapErr(err)
}
