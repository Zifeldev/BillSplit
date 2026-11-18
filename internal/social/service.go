package social

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/Zifeldev/Billsplit/internal/clients"
	"github.com/Zifeldev/Billsplit/internal/domain"
	"github.com/Zifeldev/Billsplit/internal/platform/auth"
	"github.com/Zifeldev/Billsplit/internal/platform/broker"
	"github.com/Zifeldev/Billsplit/internal/platform/cache"
	"github.com/Zifeldev/Billsplit/internal/platform/logger"
)

type TxManager interface {
	WithinTx(ctx context.Context, fn func(ctx context.Context) error) error
}

type CurrencyValidator interface {
	Validate(ctx context.Context, code, def string) (string, error)
}

const (
	friendInviteTTL     = 7 * 24 * time.Hour
	defaultGroupInvite  = 72 * time.Hour
	maxGroupInviteHours = 720
	maxGroupMembers     = 50
)

type Service struct {
	friends FriendRepository
	groups  GroupRepository
	invites InviteRepository
	tx      TxManager
	users   clients.Users
	split   clients.Split
	fx      CurrencyValidator
	cache   cache.Cache
	ttl     time.Duration
	links   LinkBuilder
	pub     broker.Publisher // transactional outbox: call inside tx.WithinTx
}

// LinkBuilder renders invite URLs for web and the mobile app (deep link).
type LinkBuilder struct {
	WebURL, Scheme string
}

func (l LinkBuilder) Web(token string) string {
	return strings.TrimRight(l.WebURL, "/") + "/invite/" + token
}
func (l LinkBuilder) App(token string) string { return l.Scheme + "://invite/" + token }

func NewService(f FriendRepository, g GroupRepository, i InviteRepository, tx TxManager, users clients.Users,
	split clients.Split, fx CurrencyValidator, c cache.Cache, ttl time.Duration, links LinkBuilder, pub broker.Publisher) *Service {
	return &Service{friends: f, groups: g, invites: i, tx: tx, users: users, split: split, fx: fx, cache: c, ttl: ttl, links: links, pub: pub}
}

// Payment are the user's "pay back" handles from the auth directory.
type Payment struct{ Venmo, PayPal, Zelle string }

func paymentOf(u clients.UserInfo) *Payment {
	return &Payment{Venmo: u.Venmo, PayPal: u.PayPal, Zelle: u.Zelle}
}

// Friend is domain.Friend plus payment handles (set for accepted friends only).
type Friend struct {
	domain.Friend
	Payment *Payment
}

func friendOf(u clients.UserInfo, st domain.FriendStatus, since time.Time) *Friend {
	f := &Friend{Friend: domain.Friend{UserID: u.ID, Name: u.Name, Email: u.Email, Status: st, Since: since}}
	if st == domain.FriendAccepted {
		f.Payment = paymentOf(u)
	}
	return f
}

// GroupView is a group with member names and payment handles filled in.
type GroupView struct {
	*domain.Group
	Payments map[string]*Payment // by user id
}

// ================= Friends =================

func (s *Service) ListFriends(ctx context.Context) (accepted, incoming, outgoing []Friend, err error) {
	uid, err := auth.UserID(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	rows, err := s.friends.List(ctx, uid)
	if err != nil {
		return nil, nil, nil, err
	}
	ids := make([]string, len(rows))
	for i, r := range rows {
		ids[i] = r.Other
	}
	info, err := s.users.GetUsers(ctx, ids)
	if err != nil {
		return nil, nil, nil, err
	}
	for _, r := range rows {
		f := Friend{Friend: domain.Friend{UserID: r.Other, Name: info[r.Other].Name, Email: info[r.Other].Email, Since: r.CreatedAt}}
		switch {
		case r.Accepted:
			f.Status = domain.FriendAccepted
			if r.AcceptedAt != nil {
				f.Since = *r.AcceptedAt
			}
			if u, ok := info[r.Other]; ok {
				f.Payment = paymentOf(u)
			}
			accepted = append(accepted, f)
		case r.RequestedBy == uid:
			f.Status = domain.FriendPendingOutgoing
			outgoing = append(outgoing, f)
		default:
			f.Status = domain.FriendPendingIncoming
			incoming = append(incoming, f)
		}
	}
	sort.Slice(accepted, func(i, j int) bool { return strings.ToLower(accepted[i].Name) < strings.ToLower(accepted[j].Name) })
	return accepted, incoming, outgoing, nil
}

func (s *Service) SendFriendRequest(ctx context.Context, email string) (*Friend, error) {
	uid, err := auth.UserID(ctx)
	if err != nil {
		return nil, err
	}
	target, err := s.users.GetUserByEmail(ctx, strings.ToLower(strings.TrimSpace(email)))
	if errors.Is(err, domain.ErrNotFound) {
		return nil, fmt.Errorf("%w: no user with this email — share your invite link instead", domain.ErrNotFound)
	}
	if err != nil {
		return nil, err
	}
	if target.ID == uid {
		return nil, fmt.Errorf("%w: you can't befriend yourself", domain.ErrInvalidArgument)
	}

	existing, err := s.friends.Get(ctx, uid, target.ID)
	switch {
	case err == nil && existing.Accepted:
		return nil, fmt.Errorf("%w: already friends", domain.ErrAlreadyExists)
	case err == nil && existing.RequestedBy == target.ID:
		// They already asked us — sending back means accepting.
		me := s.nameOf(ctx, uid)
		err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
			if err := s.friends.Accept(ctx, uid, target.ID); err != nil {
				return err
			}
			return s.notifyFriendAccepted(ctx, uid, me, target.ID)
		})
		if err != nil {
			return nil, err
		}
		return friendOf(target, domain.FriendAccepted, time.Now()), nil
	case err == nil:
		return nil, fmt.Errorf("%w: request already sent", domain.ErrAlreadyExists)
	case !errors.Is(err, domain.ErrNotFound):
		return nil, err
	}
	me := s.nameOf(ctx, uid)
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.friends.CreateRequest(ctx, uid, target.ID); err != nil {
			return err
		}
		return s.notify(ctx, target.ID, domain.NotificationEvent{
			UserIDs: []string{target.ID}, Type: domain.NotifyFriendRequest,
			Title: "Friend request", Body: me + " wants to be friends",
			Data: map[string]string{"user_id": uid},
		})
	})
	if err != nil {
		return nil, err
	}
	logger.FromContext(ctx).Info("friend request sent", "to", target.ID)
	return friendOf(target, domain.FriendPendingOutgoing, time.Now()), nil
}

func (s *Service) RespondFriendRequest(ctx context.Context, fromUser string, accept bool) (*Friend, error) {
	uid, err := auth.UserID(ctx)
	if err != nil {
		return nil, err
	}
	f, err := s.friends.Get(ctx, uid, fromUser)
	if err != nil {
		return nil, err
	}
	if f.Accepted || f.RequestedBy != fromUser {
		return nil, fmt.Errorf("%w: no pending request from this user", domain.ErrFailedPrecondition)
	}
	if !accept {
		return nil, s.friends.Delete(ctx, uid, fromUser)
	}
	u, err := s.users.GetUser(ctx, fromUser)
	if err != nil {
		return nil, err
	}
	me := s.nameOf(ctx, uid)
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.friends.Accept(ctx, uid, fromUser); err != nil {
			return err
		}
		return s.notifyFriendAccepted(ctx, uid, me, fromUser)
	})
	if err != nil {
		return nil, err
	}
	return friendOf(u, domain.FriendAccepted, time.Now()), nil
}

func (s *Service) RemoveFriend(ctx context.Context, other string) error {
	uid, err := auth.UserID(ctx)
	if err != nil {
		return err
	}
	return s.friends.Delete(ctx, uid, other)
}

func (s *Service) CreateFriendInvite(ctx context.Context) (*domain.Invite, error) {
	uid, err := auth.UserID(ctx)
	if err != nil {
		return nil, err
	}
	inv := &domain.Invite{Token: newToken(), Kind: domain.InviteFriend, InviterID: uid, ExpiresAt: time.Now().Add(friendInviteTTL)}
	if err := s.invites.Create(ctx, inv); err != nil {
		return nil, err
	}
	return inv, nil
}

func (s *Service) AcceptFriendInvite(ctx context.Context, token string) (*Friend, error) {
	uid, err := auth.UserID(ctx)
	if err != nil {
		return nil, err
	}
	inv, err := s.usableInvite(ctx, token, domain.InviteFriend)
	if err != nil {
		return nil, err
	}
	if inv.InviterID == uid {
		return nil, fmt.Errorf("%w: this is your own invite link", domain.ErrInvalidArgument)
	}
	u, err := s.users.GetUser(ctx, inv.InviterID)
	if err != nil {
		return nil, err
	}
	already, err := s.areFriends(ctx, uid, inv.InviterID)
	if err != nil {
		return nil, err
	}
	me := s.nameOf(ctx, uid)
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.friends.Accept(ctx, uid, inv.InviterID); err != nil {
			return err
		}
		if err := s.invites.IncUses(ctx, token); err != nil {
			return err
		}
		if already {
			return nil
		}
		return s.notifyFriendAccepted(ctx, uid, me, inv.InviterID)
	})
	if err != nil {
		return nil, err
	}
	logger.FromContext(ctx).Info("friend invite accepted", "inviter", inv.InviterID)
	return friendOf(u, domain.FriendAccepted, time.Now()), nil
}

func (s *Service) areFriends(ctx context.Context, a, b string) (bool, error) {
	f, err := s.friends.Get(ctx, a, b)
	if errors.Is(err, domain.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return f.Accepted, nil
}

// ================= Groups =================

func groupKey(id string) string { return "social:group:" + id }

func (s *Service) CreateGroup(ctx context.Context, name, currency string, friendIDs []string) (*GroupView, error) {
	uid, err := auth.UserID(ctx)
	if err != nil {
		return nil, err
	}
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 100 {
		return nil, fmt.Errorf("%w: name is required (max 100)", domain.ErrInvalidArgument)
	}
	me, err := s.users.GetUser(ctx, uid)
	if err != nil {
		return nil, err
	}
	if currency, err = s.fx.Validate(ctx, currency, me.Currency); err != nil {
		return nil, err
	}
	if len(friendIDs) >= maxGroupMembers {
		return nil, fmt.Errorf("%w: too many members", domain.ErrInvalidArgument)
	}
	for _, fid := range friendIDs {
		ok, err := s.areFriends(ctx, uid, fid)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("%w: user %s is not your friend", domain.ErrForbidden, fid)
		}
	}

	g := &domain.Group{Name: name, OwnerID: uid, Currency: currency}
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.groups.Create(ctx, g); err != nil {
			return err
		}
		if err := s.groups.AddMember(ctx, g.ID, uid, domain.RoleOwner); err != nil {
			return err
		}
		seen := map[string]bool{uid: true}
		var added []string
		for _, fid := range friendIDs {
			if seen[fid] {
				continue
			}
			seen[fid] = true
			if err := s.groups.AddMember(ctx, g.ID, fid, domain.RoleMember); err != nil {
				return err
			}
			added = append(added, fid)
		}
		return s.notifyGroup(ctx, g, added, me.Name+" added you to "+g.Name)
	})
	if err != nil {
		return nil, err
	}
	logger.FromContext(ctx).Info("group created", "group_id", g.ID)
	return s.GetGroup(ctx, g.ID)
}

// load returns the group (no membership check) using cache-aside.
func (s *Service) load(ctx context.Context, id string) (*domain.Group, error) {
	var g domain.Group
	if err := s.cache.Get(ctx, groupKey(id), &g); err == nil {
		return &g, nil
	}
	gp, err := s.groups.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	_ = s.cache.Set(ctx, groupKey(id), gp, s.ttl)
	return gp, nil
}

func (s *Service) invalidate(ctx context.Context, id string) {
	if err := s.cache.Delete(ctx, groupKey(id)); err != nil {
		logger.FromContext(ctx).Warn("cache invalidate failed", "err", err)
	}
}

// GetGroup returns the group with member names if the caller is a member.
func (s *Service) GetGroup(ctx context.Context, id string) (*GroupView, error) {
	uid, err := auth.UserID(ctx)
	if err != nil {
		return nil, err
	}
	g, err := s.load(ctx, id)
	if err != nil {
		return nil, err
	}
	if !g.IsMember(uid) {
		return nil, domain.ErrNotFound // don't reveal existence
	}
	vs, err := s.fillNames(ctx, []*domain.Group{g})
	if err != nil {
		return nil, err
	}
	return &vs[0], nil
}

// Directory returns the group without membership check (internal API).
func (s *Service) Directory(ctx context.Context, id string) (*domain.Group, error) {
	return s.load(ctx, id)
}

func (s *Service) ListMyGroups(ctx context.Context) ([]GroupView, error) {
	uid, err := auth.UserID(ctx)
	if err != nil {
		return nil, err
	}
	gs, err := s.groups.ListByUser(ctx, uid)
	if err != nil {
		return nil, err
	}
	ptrs := make([]*domain.Group, len(gs))
	for i := range gs {
		ptrs[i] = &gs[i]
	}
	return s.fillNames(ctx, ptrs)
}

// fillNames adds member names/emails and payment handles from the auth directory.
func (s *Service) fillNames(ctx context.Context, gs []*domain.Group) ([]GroupView, error) {
	var ids []string
	for _, g := range gs {
		for _, m := range g.Members {
			ids = append(ids, m.UserID)
		}
	}
	info, err := s.users.GetUsers(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]GroupView, len(gs))
	for gi, g := range gs {
		out[gi] = GroupView{Group: g, Payments: make(map[string]*Payment, len(g.Members))}
		for i := range g.Members {
			u, ok := info[g.Members[i].UserID]
			if !ok {
				continue
			}
			g.Members[i].Name, g.Members[i].Email = u.Name, u.Email
			out[gi].Payments[u.ID] = paymentOf(u)
		}
	}
	return out, nil
}

func (s *Service) UpdateGroup(ctx context.Context, id, name, currency string) (*GroupView, error) {
	uid, _ := auth.UserID(ctx)
	g, err := s.GetGroup(ctx, id)
	if err != nil {
		return nil, err
	}
	if g.OwnerID != uid {
		return nil, fmt.Errorf("%w: only the owner can edit the group", domain.ErrForbidden)
	}
	if name = strings.TrimSpace(name); name != "" {
		if len(name) > 100 {
			return nil, fmt.Errorf("%w: name too long", domain.ErrInvalidArgument)
		}
		g.Name = name
	}
	newCur, err := s.fx.Validate(ctx, currency, g.Currency)
	if err != nil {
		return nil, err
	}
	if newCur != g.Currency {
		busy, err := s.split.GroupHasActivity(ctx, id)
		if err != nil {
			return nil, err
		}
		if busy {
			return nil, fmt.Errorf("%w: currency can't change after receipts were finalized", domain.ErrFailedPrecondition)
		}
		g.Currency = newCur
	}
	if err := s.groups.Update(ctx, g.Group); err != nil {
		return nil, err
	}
	s.invalidate(ctx, id)
	return s.GetGroup(ctx, id)
}

// AddMember adds the caller's friend to a group the caller belongs to.
func (s *Service) AddMember(ctx context.Context, groupID, userID string) (*GroupView, error) {
	uid, _ := auth.UserID(ctx)
	g, err := s.GetGroup(ctx, groupID)
	if err != nil {
		return nil, err
	}
	if g.IsMember(userID) {
		return nil, fmt.Errorf("%w: already a member", domain.ErrAlreadyExists)
	}
	if len(g.Members) >= maxGroupMembers {
		return nil, fmt.Errorf("%w: group is full", domain.ErrFailedPrecondition)
	}
	ok, err := s.areFriends(ctx, uid, userID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("%w: you can add only friends; send an invite link instead", domain.ErrForbidden)
	}
	me, added := s.nameOf(ctx, uid), s.nameOf(ctx, userID)
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.groups.AddMember(ctx, groupID, userID, domain.RoleMember); err != nil {
			return err
		}
		if err := s.notifyGroup(ctx, g.Group, []string{userID}, me+" added you to "+g.Name); err != nil {
			return err
		}
		return s.notifyGroup(ctx, g.Group, others(g.Group, uid), added+" joined "+g.Name)
	})
	if err != nil {
		return nil, err
	}
	s.invalidate(ctx, groupID)
	return s.GetGroup(ctx, groupID)
}

func (s *Service) RemoveMember(ctx context.Context, groupID, userID string) (*GroupView, error) {
	uid, _ := auth.UserID(ctx)
	g, err := s.GetGroup(ctx, groupID)
	if err != nil {
		return nil, err
	}
	if uid != g.OwnerID && uid != userID {
		return nil, domain.ErrForbidden // members can only leave themselves
	}
	if userID == g.OwnerID {
		return nil, fmt.Errorf("%w: owner can't leave the group", domain.ErrFailedPrecondition)
	}
	net, err := s.split.MemberBalance(ctx, groupID, userID)
	if err != nil {
		return nil, err
	}
	if net != 0 {
		return nil, fmt.Errorf("%w: member has an unsettled balance", domain.ErrFailedPrecondition)
	}
	if err := s.groups.RemoveMember(ctx, groupID, userID); err != nil {
		return nil, err
	}
	s.invalidate(ctx, groupID)
	if uid == userID {
		return g, nil // the caller left and can't read the group anymore
	}
	return s.GetGroup(ctx, groupID)
}

func (s *Service) CreateGroupInvite(ctx context.Context, groupID string, ttlHours, maxUses int32) (*domain.Invite, error) {
	uid, _ := auth.UserID(ctx)
	if _, err := s.GetGroup(ctx, groupID); err != nil {
		return nil, err
	}
	ttl := defaultGroupInvite
	if ttlHours > 0 {
		if ttlHours > maxGroupInviteHours {
			ttlHours = maxGroupInviteHours
		}
		ttl = time.Duration(ttlHours) * time.Hour
	}
	if maxUses < 0 || maxUses > 1000 {
		return nil, fmt.Errorf("%w: max_uses must be 0..1000", domain.ErrInvalidArgument)
	}
	inv := &domain.Invite{Token: newToken(), Kind: domain.InviteGroup, InviterID: uid, GroupID: groupID,
		ExpiresAt: time.Now().Add(ttl), MaxUses: maxUses}
	if err := s.invites.Create(ctx, inv); err != nil {
		return nil, err
	}
	return inv, nil
}

// ListGroupInvites returns the group's active invite links (members only).
func (s *Service) ListGroupInvites(ctx context.Context, groupID string) ([]domain.Invite, error) {
	if _, err := s.GetGroup(ctx, groupID); err != nil {
		return nil, err
	}
	return s.invites.ListActiveByGroup(ctx, groupID)
}

func (s *Service) RevokeInvite(ctx context.Context, token string) error {
	uid, _ := auth.UserID(ctx)
	inv, err := s.invites.Get(ctx, token)
	if err != nil {
		return err
	}
	if inv.InviterID != uid {
		if inv.Kind != domain.InviteGroup {
			return domain.ErrForbidden
		}
		g, err := s.load(ctx, inv.GroupID)
		if err != nil || g.OwnerID != uid {
			return domain.ErrForbidden
		}
	}
	return s.invites.Revoke(ctx, token)
}

type InvitePreview struct {
	Kind          domain.InviteKind
	InviterName   string
	GroupID       string
	GroupName     string
	MembersCount  int
	Expired       bool
	AlreadyMember bool
}

// PreviewInvite works for anonymous callers too (public link preview):
// then AlreadyMember is always false. The token itself is the credential.
func (s *Service) PreviewInvite(ctx context.Context, token string) (*InvitePreview, error) {
	uid, _ := auth.UserID(ctx) // "" — anonymous
	inv, err := s.invites.Get(ctx, token)
	if err != nil {
		return nil, err
	}
	inviter, err := s.users.GetUser(ctx, inv.InviterID)
	if err != nil {
		return nil, err
	}
	p := &InvitePreview{Kind: inv.Kind, InviterName: inviter.Name, Expired: !inv.Usable(time.Now())}
	switch inv.Kind {
	case domain.InviteGroup:
		g, err := s.load(ctx, inv.GroupID)
		if err != nil {
			return nil, err
		}
		p.GroupID, p.GroupName, p.MembersCount = g.ID, g.Name, len(g.Members)
		p.AlreadyMember = uid != "" && g.IsMember(uid)
	case domain.InviteFriend:
		if uid == "" {
			break
		}
		p.AlreadyMember, err = s.areFriends(ctx, uid, inv.InviterID)
		if err != nil {
			return nil, err
		}
		p.AlreadyMember = p.AlreadyMember || inv.InviterID == uid
	}
	return p, nil
}

// JoinGroup redeems a group invite. Joiner and inviter also become friends.
func (s *Service) JoinGroup(ctx context.Context, token string) (*GroupView, error) {
	uid, err := auth.UserID(ctx)
	if err != nil {
		return nil, err
	}
	me := s.nameOf(ctx, uid)
	var groupID string
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		inv, err := s.invites.GetForUpdate(ctx, token)
		if err != nil {
			return err
		}
		if inv.Kind != domain.InviteGroup {
			return fmt.Errorf("%w: not a group invite", domain.ErrInvalidArgument)
		}
		if !inv.Usable(time.Now()) {
			return fmt.Errorf("%w: invite expired or used up", domain.ErrFailedPrecondition)
		}
		g, err := s.groups.GetByID(ctx, inv.GroupID)
		if err != nil {
			return err
		}
		groupID = g.ID
		if g.IsMember(uid) {
			return nil // idempotent
		}
		if len(g.Members) >= maxGroupMembers {
			return fmt.Errorf("%w: group is full", domain.ErrFailedPrecondition)
		}
		if err := s.groups.AddMember(ctx, g.ID, uid, domain.RoleMember); err != nil {
			return err
		}
		if inv.InviterID != uid {
			if err := s.friends.Accept(ctx, uid, inv.InviterID); err != nil {
				return err
			}
		}
		if err := s.invites.IncUses(ctx, token); err != nil {
			return err
		}
		return s.notifyGroup(ctx, g, others(g, uid), me+" joined "+g.Name)
	})
	if err != nil {
		return nil, err
	}
	s.invalidate(ctx, groupID)
	logger.FromContext(ctx).Info("joined group by invite", "group_id", groupID)
	return s.GetGroup(ctx, groupID)
}

func (s *Service) usableInvite(ctx context.Context, token string, kind domain.InviteKind) (*domain.Invite, error) {
	inv, err := s.invites.Get(ctx, token)
	if err != nil {
		return nil, err
	}
	if inv.Kind != kind {
		return nil, fmt.Errorf("%w: wrong invite type", domain.ErrInvalidArgument)
	}
	if !inv.Usable(time.Now()) {
		return nil, fmt.Errorf("%w: invite expired or used up", domain.ErrFailedPrecondition)
	}
	return inv, nil
}

// ================= Notifications =================

// nameOf returns the user's display name for notification texts (best effort).
func (s *Service) nameOf(ctx context.Context, uid string) string {
	u, err := s.users.GetUser(ctx, uid)
	if err != nil || u.Name == "" {
		return "Someone"
	}
	return u.Name
}

// notify writes the event to the outbox; call it inside s.tx.WithinTx so it
// commits atomically with the business change.
func (s *Service) notify(ctx context.Context, key string, ev domain.NotificationEvent) error {
	if len(ev.UserIDs) == 0 {
		return nil
	}
	return s.pub.Publish(ctx, domain.TopicNotification, key, ev)
}

func (s *Service) notifyFriendAccepted(ctx context.Context, uid, name, to string) error {
	return s.notify(ctx, to, domain.NotificationEvent{
		UserIDs: []string{to}, Type: domain.NotifyFriendAccepted,
		Title: "New friend", Body: name + " accepted your friend request",
		Data: map[string]string{"user_id": uid},
	})
}

func (s *Service) notifyGroup(ctx context.Context, g *domain.Group, to []string, body string) error {
	return s.notify(ctx, g.ID, domain.NotificationEvent{
		UserIDs: to, Type: domain.NotifyGroupJoined, Title: g.Name, Body: body,
		Data: map[string]string{"group_id": g.ID},
	})
}

// others returns the group's member ids except the given ones.
func others(g *domain.Group, except ...string) []string {
	var out []string
	for _, m := range g.Members {
		if !slices.Contains(except, m.UserID) {
			out = append(out, m.UserID)
		}
	}
	return out
}

// newToken returns a 128-bit URL-safe random token (unguessable).
func newToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}
