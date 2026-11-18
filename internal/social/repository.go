// Package social is the social microservice: friends, groups and invite links.
package social

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Zifeldev/Billsplit/internal/domain"
	"github.com/Zifeldev/Billsplit/internal/platform/postgres"
)

// Friendship row as stored (pair-ordered).
type Friendship struct {
	Other       string // the other user from the caller's perspective
	RequestedBy string
	Accepted    bool
	CreatedAt   time.Time
	AcceptedAt  *time.Time
}

type FriendRepository interface {
	// Get returns the friendship between a and b or ErrNotFound.
	Get(ctx context.Context, a, b string) (*Friendship, error)
	List(ctx context.Context, userID string) ([]Friendship, error)
	CreateRequest(ctx context.Context, from, to string) error
	// Accept marks the pair as friends (creating the row if needed).
	Accept(ctx context.Context, a, b string) error
	Delete(ctx context.Context, a, b string) error
}

type GroupRepository interface {
	Create(ctx context.Context, g *domain.Group) error
	GetByID(ctx context.Context, id string) (*domain.Group, error) // members have UserID/Role only
	ListByUser(ctx context.Context, userID string) ([]domain.Group, error)
	Update(ctx context.Context, g *domain.Group) error
	AddMember(ctx context.Context, groupID, userID, role string) error
	RemoveMember(ctx context.Context, groupID, userID string) error
}

type InviteRepository interface {
	Create(ctx context.Context, inv *domain.Invite) error
	// GetForUpdate locks the row inside a transaction (safe max_uses counting).
	GetForUpdate(ctx context.Context, token string) (*domain.Invite, error)
	Get(ctx context.Context, token string) (*domain.Invite, error)
	IncUses(ctx context.Context, token string) error
	Revoke(ctx context.Context, token string) error
	// ListActiveByGroup returns the group's invites that can still be used, newest first.
	ListActiveByGroup(ctx context.Context, groupID string) ([]domain.Invite, error)
}

func pair(a, b string) (string, string) {
	if a < b {
		return a, b
	}
	return b, a
}

// ---------- postgres ----------

type PGFriendRepo struct{ postgres.DB }

func NewFriendRepo(p *pgxpool.Pool) *PGFriendRepo { return &PGFriendRepo{postgres.DB{Pool: p}} }

func (r *PGFriendRepo) Get(ctx context.Context, a, b string) (*Friendship, error) {
	lo, hi := pair(a, b)
	var f Friendship
	var status int16
	err := r.Q(ctx).QueryRow(ctx,
		`SELECT requested_by, status, created_at, accepted_at FROM friendships WHERE user_low = $1 AND user_high = $2`, lo, hi,
	).Scan(&f.RequestedBy, &status, &f.CreatedAt, &f.AcceptedAt)
	if err != nil {
		return nil, postgres.MapErr(err)
	}
	f.Other, f.Accepted = b, status == 2
	return &f, nil
}

func (r *PGFriendRepo) List(ctx context.Context, userID string) ([]Friendship, error) {
	rows, err := r.Q(ctx).Query(ctx,
		`SELECT CASE WHEN user_low = $1 THEN user_high ELSE user_low END, requested_by, status, created_at, accepted_at
		   FROM friendships WHERE user_low = $1 OR user_high = $1
		  ORDER BY COALESCE(accepted_at, created_at) DESC`, userID)
	if err != nil {
		return nil, postgres.MapErr(err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Friendship, error) {
		var f Friendship
		var status int16
		err := row.Scan(&f.Other, &f.RequestedBy, &status, &f.CreatedAt, &f.AcceptedAt)
		f.Accepted = status == 2
		return f, err
	})
}

func (r *PGFriendRepo) CreateRequest(ctx context.Context, from, to string) error {
	lo, hi := pair(from, to)
	_, err := r.Q(ctx).Exec(ctx,
		`INSERT INTO friendships (user_low, user_high, requested_by) VALUES ($1, $2, $3)`, lo, hi, from)
	return postgres.MapErr(err)
}

func (r *PGFriendRepo) Accept(ctx context.Context, a, b string) error {
	lo, hi := pair(a, b)
	_, err := r.Q(ctx).Exec(ctx,
		`INSERT INTO friendships (user_low, user_high, requested_by, status, accepted_at) VALUES ($1, $2, $3, 2, now())
		 ON CONFLICT (user_low, user_high) DO UPDATE SET status = 2, accepted_at = COALESCE(friendships.accepted_at, now())`,
		lo, hi, a)
	return postgres.MapErr(err)
}

func (r *PGFriendRepo) Delete(ctx context.Context, a, b string) error {
	lo, hi := pair(a, b)
	return postgres.Affected(r.Q(ctx).Exec(ctx, `DELETE FROM friendships WHERE user_low = $1 AND user_high = $2`, lo, hi))
}

type PGGroupRepo struct{ postgres.DB }

func NewGroupRepo(p *pgxpool.Pool) *PGGroupRepo { return &PGGroupRepo{postgres.DB{Pool: p}} }

func (r *PGGroupRepo) Create(ctx context.Context, g *domain.Group) error {
	return postgres.MapErr(r.Q(ctx).QueryRow(ctx,
		`INSERT INTO groups (name, owner_id, currency) VALUES ($1, $2, $3) RETURNING id, created_at`,
		g.Name, g.OwnerID, g.Currency).Scan(&g.ID, &g.CreatedAt))
}

func (r *PGGroupRepo) GetByID(ctx context.Context, id string) (*domain.Group, error) {
	var g domain.Group
	err := r.Q(ctx).QueryRow(ctx,
		`SELECT id, name, owner_id, currency, created_at FROM groups WHERE id = $1`, id,
	).Scan(&g.ID, &g.Name, &g.OwnerID, &g.Currency, &g.CreatedAt)
	if err != nil {
		return nil, postgres.MapErr(err)
	}
	m, err := r.members(ctx, []string{id})
	if err != nil {
		return nil, err
	}
	g.Members = m[id]
	return &g, nil
}

func (r *PGGroupRepo) ListByUser(ctx context.Context, userID string) ([]domain.Group, error) {
	rows, err := r.Q(ctx).Query(ctx,
		`SELECT g.id, g.name, g.owner_id, g.currency, g.created_at
		   FROM groups g JOIN group_members m ON m.group_id = g.id
		  WHERE m.user_id = $1 ORDER BY g.created_at DESC`, userID)
	if err != nil {
		return nil, postgres.MapErr(err)
	}
	gs, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.Group, error) {
		var g domain.Group
		err := row.Scan(&g.ID, &g.Name, &g.OwnerID, &g.Currency, &g.CreatedAt)
		return g, err
	})
	if err != nil {
		return nil, postgres.MapErr(err)
	}
	ids := make([]string, len(gs))
	for i := range gs {
		ids[i] = gs[i].ID
	}
	m, err := r.members(ctx, ids)
	if err != nil {
		return nil, err
	}
	for i := range gs {
		gs[i].Members = m[gs[i].ID]
	}
	return gs, nil
}

func (r *PGGroupRepo) members(ctx context.Context, ids []string) (map[string][]domain.GroupMember, error) {
	out := map[string][]domain.GroupMember{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := r.Q(ctx).Query(ctx,
		`SELECT group_id, user_id, role FROM group_members WHERE group_id = ANY($1::uuid[]) ORDER BY joined_at`, ids)
	if err != nil {
		return nil, postgres.MapErr(err)
	}
	defer rows.Close()
	for rows.Next() {
		var gid string
		var m domain.GroupMember
		if err := rows.Scan(&gid, &m.UserID, &m.Role); err != nil {
			return nil, err
		}
		out[gid] = append(out[gid], m)
	}
	return out, postgres.MapErr(rows.Err())
}

func (r *PGGroupRepo) Update(ctx context.Context, g *domain.Group) error {
	return postgres.Affected(r.Q(ctx).Exec(ctx,
		`UPDATE groups SET name = $2, currency = $3 WHERE id = $1`, g.ID, g.Name, g.Currency))
}

func (r *PGGroupRepo) AddMember(ctx context.Context, groupID, userID, role string) error {
	_, err := r.Q(ctx).Exec(ctx,
		`INSERT INTO group_members (group_id, user_id, role) VALUES ($1, $2, $3)`, groupID, userID, role)
	return postgres.MapErr(err)
}

func (r *PGGroupRepo) RemoveMember(ctx context.Context, groupID, userID string) error {
	return postgres.Affected(r.Q(ctx).Exec(ctx,
		`DELETE FROM group_members WHERE group_id = $1 AND user_id = $2`, groupID, userID))
}

type PGInviteRepo struct{ postgres.DB }

func NewInviteRepo(p *pgxpool.Pool) *PGInviteRepo { return &PGInviteRepo{postgres.DB{Pool: p}} }

const inviteCols = `token, kind, inviter_id, COALESCE(group_id::text, ''), expires_at, max_uses, uses, revoked, created_at`

func scanInvite(row pgx.Row) (*domain.Invite, error) {
	i, err := scanInviteRow(row)
	if err != nil {
		return nil, postgres.MapErr(err)
	}
	return &i, nil
}

func scanInviteRow(row pgx.Row) (domain.Invite, error) {
	var i domain.Invite
	err := row.Scan(&i.Token, &i.Kind, &i.InviterID, &i.GroupID, &i.ExpiresAt, &i.MaxUses, &i.Uses, &i.Revoked, &i.CreatedAt)
	return i, err
}

func (r *PGInviteRepo) Create(ctx context.Context, i *domain.Invite) error {
	return postgres.MapErr(r.Q(ctx).QueryRow(ctx,
		`INSERT INTO invites (token, kind, inviter_id, group_id, expires_at, max_uses) VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING created_at`,
		i.Token, i.Kind, i.InviterID, postgres.Nullable(i.GroupID), i.ExpiresAt, i.MaxUses).Scan(&i.CreatedAt))
}

func (r *PGInviteRepo) Get(ctx context.Context, token string) (*domain.Invite, error) {
	return scanInvite(r.Q(ctx).QueryRow(ctx, `SELECT `+inviteCols+` FROM invites WHERE token = $1`, token))
}

func (r *PGInviteRepo) GetForUpdate(ctx context.Context, token string) (*domain.Invite, error) {
	return scanInvite(r.Q(ctx).QueryRow(ctx, `SELECT `+inviteCols+` FROM invites WHERE token = $1 FOR UPDATE`, token))
}

func (r *PGInviteRepo) IncUses(ctx context.Context, token string) error {
	return postgres.Affected(r.Q(ctx).Exec(ctx, `UPDATE invites SET uses = uses + 1 WHERE token = $1`, token))
}

func (r *PGInviteRepo) Revoke(ctx context.Context, token string) error {
	return postgres.Affected(r.Q(ctx).Exec(ctx, `UPDATE invites SET revoked = true WHERE token = $1`, token))
}

func (r *PGInviteRepo) ListActiveByGroup(ctx context.Context, groupID string) ([]domain.Invite, error) {
	rows, err := r.Q(ctx).Query(ctx,
		`SELECT `+inviteCols+` FROM invites
		  WHERE group_id = $1 AND kind = $2 AND NOT revoked AND expires_at > now()
		    AND (max_uses = 0 OR uses < max_uses)
		  ORDER BY created_at DESC`, groupID, domain.InviteGroup)
	if err != nil {
		return nil, postgres.MapErr(err)
	}
	out, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.Invite, error) { return scanInviteRow(row) })
	return out, postgres.MapErr(err)
}
