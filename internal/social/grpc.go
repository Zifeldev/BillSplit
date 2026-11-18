package social

import (
	"context"

	"github.com/Zifeldev/Billsplit/internal/domain"
	"github.com/Zifeldev/Billsplit/internal/platform/grpcx"
	ipb "github.com/Zifeldev/Billsplit/pkg/pb/billsplit/internalapi/v1"
	pb "github.com/Zifeldev/Billsplit/pkg/pb/billsplit/v1"
)

// Handler implements FriendService, GroupService and the internal GroupDirectory.
type Handler struct {
	pb.UnimplementedFriendServiceServer
	pb.UnimplementedGroupServiceServer
	svc *Service
}

func NewHandler(s *Service) *Handler { return &Handler{svc: s} }

// ---------- friends ----------

func (h *Handler) ListFriends(ctx context.Context, _ *pb.ListFriendsRequest) (*pb.ListFriendsResponse, error) {
	acc, _, _, err := h.svc.ListFriends(ctx)
	if err != nil {
		return nil, grpcx.ToStatus(ctx, err)
	}
	return &pb.ListFriendsResponse{Friends: friendsPB(acc)}, nil
}

func (h *Handler) ListFriendRequests(ctx context.Context, _ *pb.ListFriendRequestsRequest) (*pb.ListFriendRequestsResponse, error) {
	_, in, out, err := h.svc.ListFriends(ctx)
	if err != nil {
		return nil, grpcx.ToStatus(ctx, err)
	}
	return &pb.ListFriendRequestsResponse{Incoming: friendsPB(in), Outgoing: friendsPB(out)}, nil
}

func (h *Handler) SendFriendRequest(ctx context.Context, req *pb.SendFriendRequestRequest) (*pb.Friend, error) {
	f, err := h.svc.SendFriendRequest(ctx, req.GetEmail())
	if err != nil {
		return nil, grpcx.ToStatus(ctx, err)
	}
	return friendPB(*f), nil
}

func (h *Handler) RespondFriendRequest(ctx context.Context, req *pb.RespondFriendRequestRequest) (*pb.Friend, error) {
	f, err := h.svc.RespondFriendRequest(ctx, req.GetUserId(), req.GetAccept())
	if err != nil {
		return nil, grpcx.ToStatus(ctx, err)
	}
	if f == nil {
		return &pb.Friend{UserId: req.GetUserId()}, nil // declined
	}
	return friendPB(*f), nil
}

func (h *Handler) RemoveFriend(ctx context.Context, req *pb.RemoveFriendRequest) (*pb.RemoveFriendResponse, error) {
	if err := h.svc.RemoveFriend(ctx, req.GetUserId()); err != nil {
		return nil, grpcx.ToStatus(ctx, err)
	}
	return &pb.RemoveFriendResponse{}, nil
}

func (h *Handler) CreateFriendInvite(ctx context.Context, _ *pb.CreateFriendInviteRequest) (*pb.InviteLink, error) {
	inv, err := h.svc.CreateFriendInvite(ctx)
	if err != nil {
		return nil, grpcx.ToStatus(ctx, err)
	}
	return h.linkPB(inv), nil
}

func (h *Handler) AcceptFriendInvite(ctx context.Context, req *pb.AcceptFriendInviteRequest) (*pb.Friend, error) {
	f, err := h.svc.AcceptFriendInvite(ctx, req.GetToken())
	if err != nil {
		return nil, grpcx.ToStatus(ctx, err)
	}
	return friendPB(*f), nil
}

// ---------- groups ----------

func (h *Handler) CreateGroup(ctx context.Context, req *pb.CreateGroupRequest) (*pb.Group, error) {
	return h.group(ctx)(h.svc.CreateGroup(ctx, req.GetName(), req.GetCurrency(), req.GetFriendIds()))
}

func (h *Handler) GetGroup(ctx context.Context, req *pb.GetGroupRequest) (*pb.Group, error) {
	return h.group(ctx)(h.svc.GetGroup(ctx, req.GetGroupId()))
}

func (h *Handler) ListMyGroups(ctx context.Context, _ *pb.ListMyGroupsRequest) (*pb.ListMyGroupsResponse, error) {
	gs, err := h.svc.ListMyGroups(ctx)
	if err != nil {
		return nil, grpcx.ToStatus(ctx, err)
	}
	out := &pb.ListMyGroupsResponse{Groups: make([]*pb.Group, len(gs))}
	for i := range gs {
		out.Groups[i] = groupPB(&gs[i])
	}
	return out, nil
}

func (h *Handler) UpdateGroup(ctx context.Context, req *pb.UpdateGroupRequest) (*pb.Group, error) {
	return h.group(ctx)(h.svc.UpdateGroup(ctx, req.GetGroupId(), req.GetName(), req.GetCurrency()))
}

func (h *Handler) AddMember(ctx context.Context, req *pb.AddMemberRequest) (*pb.Group, error) {
	return h.group(ctx)(h.svc.AddMember(ctx, req.GetGroupId(), req.GetUserId()))
}

func (h *Handler) RemoveMember(ctx context.Context, req *pb.RemoveMemberRequest) (*pb.Group, error) {
	return h.group(ctx)(h.svc.RemoveMember(ctx, req.GetGroupId(), req.GetUserId()))
}

func (h *Handler) CreateGroupInvite(ctx context.Context, req *pb.CreateGroupInviteRequest) (*pb.InviteLink, error) {
	inv, err := h.svc.CreateGroupInvite(ctx, req.GetGroupId(), req.GetTtlHours(), req.GetMaxUses())
	if err != nil {
		return nil, grpcx.ToStatus(ctx, err)
	}
	return h.linkPB(inv), nil
}

func (h *Handler) ListGroupInvites(ctx context.Context, req *pb.ListGroupInvitesRequest) (*pb.ListGroupInvitesResponse, error) {
	invs, err := h.svc.ListGroupInvites(ctx, req.GetGroupId())
	if err != nil {
		return nil, grpcx.ToStatus(ctx, err)
	}
	out := &pb.ListGroupInvitesResponse{Invites: make([]*pb.InviteLink, len(invs))}
	for i := range invs {
		out.Invites[i] = h.linkPB(&invs[i])
	}
	return out, nil
}

func (h *Handler) RevokeGroupInvite(ctx context.Context, req *pb.RevokeGroupInviteRequest) (*pb.RevokeGroupInviteResponse, error) {
	if err := h.svc.RevokeInvite(ctx, req.GetToken()); err != nil {
		return nil, grpcx.ToStatus(ctx, err)
	}
	return &pb.RevokeGroupInviteResponse{}, nil
}

func (h *Handler) GetInvite(ctx context.Context, req *pb.GetInviteRequest) (*pb.InvitePreview, error) {
	p, err := h.svc.PreviewInvite(ctx, req.GetToken())
	if err != nil {
		return nil, grpcx.ToStatus(ctx, err)
	}
	return &pb.InvitePreview{Kind: pb.InviteKind(p.Kind), InviterName: p.InviterName, GroupId: p.GroupID,
		GroupName: p.GroupName, MembersCount: int32(p.MembersCount), Expired: p.Expired, AlreadyMember: p.AlreadyMember}, nil
}

func (h *Handler) JoinGroup(ctx context.Context, req *pb.JoinGroupRequest) (*pb.Group, error) {
	return h.group(ctx)(h.svc.JoinGroup(ctx, req.GetToken()))
}

// ---------- internal ----------

func (h *Handler) GetGroupInternal(ctx context.Context, id string) (*ipb.GroupInfo, error) {
	g, err := h.svc.Directory(ctx, id)
	if err != nil {
		return nil, grpcx.ToStatus(ctx, err)
	}
	out := &ipb.GroupInfo{Id: g.ID, Name: g.Name, OwnerId: g.OwnerID, Currency: g.Currency}
	for _, m := range g.Members {
		out.MemberIds = append(out.MemberIds, m.UserID)
	}
	return out, nil
}

// Directory adapts the internal GroupDirectory service (method name clashes with GroupService.GetGroup).
type Directory struct {
	ipb.UnimplementedGroupDirectoryServer
	h *Handler
}

func (h *Handler) Directory() *Directory { return &Directory{h: h} }

func (d *Directory) GetGroup(ctx context.Context, req *ipb.GetGroupRequest) (*ipb.GroupInfo, error) {
	return d.h.GetGroupInternal(ctx, req.GetGroupId())
}

// ---------- mapping ----------

func (h *Handler) group(ctx context.Context) func(*GroupView, error) (*pb.Group, error) {
	return func(g *GroupView, err error) (*pb.Group, error) {
		if err != nil {
			return nil, grpcx.ToStatus(ctx, err)
		}
		return groupPB(g), nil
	}
}

func groupPB(g *GroupView) *pb.Group {
	out := &pb.Group{Id: g.ID, Name: g.Name, OwnerId: g.OwnerID, Currency: g.Currency, CreatedAt: grpcx.TS(g.CreatedAt)}
	for _, m := range g.Members {
		out.Members = append(out.Members, &pb.GroupMember{UserId: m.UserID, Name: m.Name, Email: m.Email, Role: m.Role,
			Payment: paymentPB(g.Payments[m.UserID])})
	}
	return out
}

func paymentPB(p *Payment) *pb.PaymentHandles {
	if p == nil {
		return nil
	}
	return &pb.PaymentHandles{Venmo: p.Venmo, Paypal: p.PayPal, Zelle: p.Zelle}
}

func friendPB(f Friend) *pb.Friend {
	return &pb.Friend{UserId: f.UserID, Name: f.Name, Email: f.Email, Status: pb.FriendStatus(f.Status), Since: grpcx.TS(f.Since),
		Payment: paymentPB(f.Payment)}
}

func friendsPB(fs []Friend) []*pb.Friend {
	out := make([]*pb.Friend, len(fs))
	for i := range fs {
		out[i] = friendPB(fs[i])
	}
	return out
}

func (h *Handler) linkPB(inv *domain.Invite) *pb.InviteLink {
	return &pb.InviteLink{Token: inv.Token, Url: h.svc.links.Web(inv.Token), AppUrl: h.svc.links.App(inv.Token),
		Kind: pb.InviteKind(inv.Kind), ExpiresAt: grpcx.TS(inv.ExpiresAt), MaxUses: inv.MaxUses, Uses: inv.Uses}
}
