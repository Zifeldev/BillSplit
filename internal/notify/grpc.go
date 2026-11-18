package notify

import (
	"context"

	"github.com/Zifeldev/Billsplit/internal/platform/grpcx"
	pb "github.com/Zifeldev/Billsplit/pkg/pb/billsplit/v1"
)

// Handler implements NotificationService.
type Handler struct {
	pb.UnimplementedNotificationServiceServer
	svc *Service
}

func NewHandler(s *Service) *Handler { return &Handler{svc: s} }

func (h *Handler) RegisterDevice(ctx context.Context, req *pb.RegisterDeviceRequest) (*pb.RegisterDeviceResponse, error) {
	if err := h.svc.RegisterDevice(ctx, req.GetToken(), req.GetPlatform()); err != nil {
		return nil, grpcx.ToStatus(ctx, err)
	}
	return &pb.RegisterDeviceResponse{}, nil
}

func (h *Handler) UnregisterDevice(ctx context.Context, req *pb.UnregisterDeviceRequest) (*pb.UnregisterDeviceResponse, error) {
	if err := h.svc.UnregisterDevice(ctx, req.GetToken()); err != nil {
		return nil, grpcx.ToStatus(ctx, err)
	}
	return &pb.UnregisterDeviceResponse{}, nil
}

func (h *Handler) ListNotifications(ctx context.Context, req *pb.ListNotificationsRequest) (*pb.ListNotificationsResponse, error) {
	items, unread, err := h.svc.List(ctx, int(req.GetLimit()), req.GetUnreadOnly())
	if err != nil {
		return nil, grpcx.ToStatus(ctx, err)
	}
	out := &pb.ListNotificationsResponse{Notifications: make([]*pb.Notification, len(items)), UnreadCount: int32(unread)}
	for i, n := range items {
		out.Notifications[i] = &pb.Notification{
			Id: n.ID, Type: n.Type, Title: n.Title, Body: n.Body, Data: n.Data,
			Read: n.Read, CreatedAt: grpcx.TS(n.CreatedAt),
		}
	}
	return out, nil
}

func (h *Handler) MarkNotificationsRead(ctx context.Context, req *pb.MarkNotificationsReadRequest) (*pb.MarkNotificationsReadResponse, error) {
	if err := h.svc.MarkRead(ctx, req.GetIds()); err != nil {
		return nil, grpcx.ToStatus(ctx, err)
	}
	return &pb.MarkNotificationsReadResponse{}, nil
}
