package authsvc

import (
	"context"

	"github.com/Zifeldev/Billsplit/internal/platform/grpcx"
	ipb "github.com/Zifeldev/Billsplit/pkg/pb/billsplit/internalapi/v1"
	pb "github.com/Zifeldev/Billsplit/pkg/pb/billsplit/v1"
)

// Handler implements the public AuthService and the internal UserDirectory.
type Handler struct {
	pb.UnimplementedAuthServiceServer
	ipb.UnimplementedUserDirectoryServer
	svc *Service
}

func NewHandler(s *Service) *Handler { return &Handler{svc: s} }

func (h *Handler) Register(ctx context.Context, req *pb.RegisterRequest) (*pb.AuthResponse, error) {
	res, err := h.svc.Register(ctx, req.GetEmail(), req.GetPassword(), req.GetName(), req.GetCurrency())
	if err != nil {
		return nil, grpcx.ToStatus(ctx, err)
	}
	return authResp(res), nil
}

func (h *Handler) Login(ctx context.Context, req *pb.LoginRequest) (*pb.AuthResponse, error) {
	res, err := h.svc.Login(ctx, req.GetEmail(), req.GetPassword())
	if err != nil {
		return nil, grpcx.ToStatus(ctx, err)
	}
	return authResp(res), nil
}

func (h *Handler) Refresh(ctx context.Context, req *pb.RefreshRequest) (*pb.AuthResponse, error) {
	res, err := h.svc.Refresh(ctx, req.GetRefreshToken())
	if err != nil {
		return nil, grpcx.ToStatus(ctx, err)
	}
	return authResp(res), nil
}

func (h *Handler) Me(ctx context.Context, _ *pb.MeRequest) (*pb.User, error) {
	u, err := h.svc.Me(ctx)
	if err != nil {
		return nil, grpcx.ToStatus(ctx, err)
	}
	return userPB(u), nil
}

func (h *Handler) UpdateProfile(ctx context.Context, req *pb.UpdateProfileRequest) (*pb.User, error) {
	u, err := h.svc.UpdateProfile(ctx, req.GetName(), req.GetCurrency(),
		PaymentUpdate{Venmo: req.Venmo, PayPal: req.Paypal, Zelle: req.Zelle})
	if err != nil {
		return nil, grpcx.ToStatus(ctx, err)
	}
	return userPB(u), nil
}

func (h *Handler) Logout(ctx context.Context, req *pb.LogoutRequest) (*pb.LogoutResponse, error) {
	if err := h.svc.Logout(ctx, req.GetRefreshToken()); err != nil {
		return nil, grpcx.ToStatus(ctx, err)
	}
	return &pb.LogoutResponse{}, nil
}

func (h *Handler) GetUsers(ctx context.Context, req *ipb.GetUsersRequest) (*ipb.GetUsersResponse, error) {
	us, err := h.svc.GetUsers(ctx, req.GetIds())
	if err != nil {
		return nil, grpcx.ToStatus(ctx, err)
	}
	out := &ipb.GetUsersResponse{Users: make([]*ipb.UserInfo, len(us))}
	for i := range us {
		out.Users[i] = infoPB(&us[i])
	}
	return out, nil
}

func (h *Handler) GetUserByEmail(ctx context.Context, req *ipb.GetUserByEmailRequest) (*ipb.UserInfo, error) {
	u, err := h.svc.GetUserByEmail(ctx, req.GetEmail())
	if err != nil {
		return nil, grpcx.ToStatus(ctx, err)
	}
	return infoPB(u), nil
}

func authResp(r *AuthResult) *pb.AuthResponse {
	return &pb.AuthResponse{
		AccessToken:     r.Tokens.Access,
		RefreshToken:    r.Tokens.Refresh,
		AccessExpiresAt: grpcx.TS(r.Tokens.AccessExpiresAt),
		User:            userPB(r.User),
	}
}

func userPB(u *User) *pb.User {
	return &pb.User{Id: u.ID, Email: u.Email, Name: u.Name, Currency: u.Currency, CreatedAt: grpcx.TS(u.CreatedAt),
		Payment: &pb.PaymentHandles{Venmo: u.Payment.Venmo, Paypal: u.Payment.PayPal, Zelle: u.Payment.Zelle}}
}

func infoPB(u *User) *ipb.UserInfo {
	return &ipb.UserInfo{Id: u.ID, Email: u.Email, Name: u.Name, Currency: u.Currency,
		Venmo: u.Payment.Venmo, Paypal: u.Payment.PayPal, Zelle: u.Payment.Zelle}
}
