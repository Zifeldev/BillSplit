// Package clients wraps internal gRPC APIs of other services behind small
// Go interfaces and converts their errors back to domain errors.
package clients

import (
	"context"

	"google.golang.org/grpc"

	"github.com/Zifeldev/Billsplit/internal/domain"
	"github.com/Zifeldev/Billsplit/internal/platform/grpcx"
	ipb "github.com/Zifeldev/Billsplit/pkg/pb/billsplit/internalapi/v1"
)

type UserInfo struct {
	ID, Email, Name, Currency string
	Venmo, PayPal, Zelle      string // payment handles for "pay back" links
}

func userInfo(x *ipb.UserInfo) UserInfo {
	return UserInfo{ID: x.GetId(), Email: x.GetEmail(), Name: x.GetName(), Currency: x.GetCurrency(),
		Venmo: x.GetVenmo(), PayPal: x.GetPaypal(), Zelle: x.GetZelle()}
}

// Users is the auth service directory.
type Users interface {
	GetUsers(ctx context.Context, ids []string) (map[string]UserInfo, error)
	GetUser(ctx context.Context, id string) (UserInfo, error)
	GetUserByEmail(ctx context.Context, email string) (UserInfo, error)
}

type usersClient struct{ c ipb.UserDirectoryClient }

func NewUsers(cc *grpc.ClientConn) Users { return usersClient{ipb.NewUserDirectoryClient(cc)} }

func (u usersClient) GetUsers(ctx context.Context, ids []string) (map[string]UserInfo, error) {
	out := make(map[string]UserInfo, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	resp, err := u.c.GetUsers(ctx, &ipb.GetUsersRequest{Ids: ids})
	if err != nil {
		return nil, grpcx.FromStatus(err)
	}
	for _, x := range resp.GetUsers() {
		out[x.GetId()] = userInfo(x)
	}
	return out, nil
}

func (u usersClient) GetUser(ctx context.Context, id string) (UserInfo, error) {
	m, err := u.GetUsers(ctx, []string{id})
	if err != nil {
		return UserInfo{}, err
	}
	x, ok := m[id]
	if !ok {
		return UserInfo{}, domain.ErrNotFound
	}
	return x, nil
}

func (u usersClient) GetUserByEmail(ctx context.Context, email string) (UserInfo, error) {
	x, err := u.c.GetUserByEmail(ctx, &ipb.GetUserByEmailRequest{Email: email})
	if err != nil {
		return UserInfo{}, grpcx.FromStatus(err)
	}
	return userInfo(x), nil
}

type GroupInfo struct {
	ID, Name, OwnerID, Currency string
	MemberIDs                   []string
}

func (g GroupInfo) IsMember(uid string) bool {
	for _, m := range g.MemberIDs {
		if m == uid {
			return true
		}
	}
	return false
}

// Groups is the social service directory.
type Groups interface {
	GetGroup(ctx context.Context, id string) (GroupInfo, error)
}

type groupsClient struct{ c ipb.GroupDirectoryClient }

func NewGroups(cc *grpc.ClientConn) Groups { return groupsClient{ipb.NewGroupDirectoryClient(cc)} }

func (g groupsClient) GetGroup(ctx context.Context, id string) (GroupInfo, error) {
	x, err := g.c.GetGroup(ctx, &ipb.GetGroupRequest{GroupId: id})
	if err != nil {
		return GroupInfo{}, grpcx.FromStatus(err)
	}
	return GroupInfo{ID: x.GetId(), Name: x.GetName(), OwnerID: x.GetOwnerId(), Currency: x.GetCurrency(), MemberIDs: x.GetMemberIds()}, nil
}

// Split is the split service internal API.
type Split interface {
	MemberBalance(ctx context.Context, groupID, userID string) (int64, error)
	GroupHasActivity(ctx context.Context, groupID string) (bool, error)
	UserTotals(ctx context.Context, userID, currency string) (owedToMe, iOwe int64, err error)
	ReceiptImage(ctx context.Context, receiptID string) (*ipb.ReceiptImage, error)
}

type splitClient struct{ c ipb.SplitInternalClient }

func NewSplit(cc *grpc.ClientConn) Split { return splitClient{ipb.NewSplitInternalClient(cc)} }

func (s splitClient) MemberBalance(ctx context.Context, groupID, userID string) (int64, error) {
	x, err := s.c.GetMemberBalance(ctx, &ipb.GetMemberBalanceRequest{GroupId: groupID, UserId: userID})
	if err != nil {
		return 0, grpcx.FromStatus(err)
	}
	return x.GetNetCents(), nil
}

func (s splitClient) GroupHasActivity(ctx context.Context, groupID string) (bool, error) {
	x, err := s.c.GroupHasActivity(ctx, &ipb.GroupHasActivityRequest{GroupId: groupID})
	if err != nil {
		return false, grpcx.FromStatus(err)
	}
	return x.GetHasActivity(), nil
}

func (s splitClient) UserTotals(ctx context.Context, userID, currency string) (int64, int64, error) {
	x, err := s.c.GetUserTotals(ctx, &ipb.GetUserTotalsRequest{UserId: userID, Currency: currency})
	if err != nil {
		return 0, 0, grpcx.FromStatus(err)
	}
	return x.GetOwedToMeCents(), x.GetIOweCents(), nil
}

func (s splitClient) ReceiptImage(ctx context.Context, receiptID string) (*ipb.ReceiptImage, error) {
	x, err := s.c.GetReceiptImage(ctx, &ipb.GetReceiptImageRequest{ReceiptId: receiptID})
	return x, grpcx.FromStatus(err)
}
