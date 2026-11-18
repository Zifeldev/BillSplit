// Package grpcx contains shared gRPC server/client plumbing for all services.
//
// Server chain: Recovery -> Logging (request id) -> Metrics -> Timeout -> Identity.
// Authentication happens once at the gateway; it forwards the user id in the
// x-user-id metadata. Backend services are reachable only from the internal
// network, so they trust that header.
package grpcx

import (
	"context"
	"crypto/subtle"
	"log/slog"
	"runtime/debug"
	"strings"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/Zifeldev/Billsplit/internal/platform/auth"
	"github.com/Zifeldev/Billsplit/internal/platform/logger"
	"github.com/Zifeldev/Billsplit/internal/platform/metrics"
)

const (
	HeaderRequestID     = "x-request-id"
	HeaderUserID        = "x-user-id"
	HeaderInternalToken = "x-internal-token"
)

// Recovery converts panics into codes.Internal instead of crashing the service.
func Recovery() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, h grpc.UnaryHandler) (resp any, err error) {
		defer func() {
			if r := recover(); r != nil {
				logger.FromContext(ctx).Error("panic recovered", "method", info.FullMethod, "panic", r, "stack", string(debug.Stack()))
				err = status.Error(codes.Internal, "internal error")
			}
		}()
		return h(ctx, req)
	}
}

// Logging attaches a request-scoped logger with request_id and logs every call.
func Logging(base *slog.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, h grpc.UnaryHandler) (any, error) {
		reqID := firstMD(ctx, HeaderRequestID)
		if reqID == "" || len(reqID) > 64 {
			reqID = uuid.NewString()
		}
		ctx = WithRequestID(ctx, reqID)
		l := base.With("request_id", reqID, "method", info.FullMethod)
		if uid := firstMD(ctx, HeaderUserID); uid != "" {
			l = l.With("user_id", uid)
		}
		ctx = logger.WithContext(ctx, l)

		start := time.Now()
		resp, err := h(ctx, req)
		code := status.Code(err)
		attrs := []any{"code", code.String(), "duration_ms", time.Since(start).Milliseconds()}
		switch code {
		case codes.OK:
			l.Info("grpc request", attrs...)
		case codes.Internal, codes.Unknown, codes.DataLoss, codes.Unavailable:
			l.Error("grpc request", append(attrs, "err", err)...)
		default:
			l.Warn("grpc request", append(attrs, "err", err)...)
		}
		return resp, err
	}
}

func Metrics() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, h grpc.UnaryHandler) (any, error) {
		start := time.Now()
		resp, err := h(ctx, req)
		metrics.GRPCDuration.WithLabelValues(info.FullMethod).Observe(time.Since(start).Seconds())
		metrics.GRPCRequests.WithLabelValues(info.FullMethod, status.Code(err).String()).Inc()
		return resp, err
	}
}

// Timeout bounds every request so slow DB/Redis calls can't pile up goroutines.
func Timeout(d time.Duration) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, h grpc.UnaryHandler) (any, error) {
		ctx, cancel := context.WithTimeout(ctx, d)
		defer cancel()
		return h(ctx, req)
	}
}

// ServiceAuth rejects calls that don't carry the shared internal token, so a
// process that merely reaches the network can't impersonate users via x-user-id.
// Health checks and reflection stay open for probes and tooling.
func ServiceAuth(token string) grpc.UnaryServerInterceptor {
	want := []byte(token)
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, h grpc.UnaryHandler) (any, error) {
		if strings.HasPrefix(info.FullMethod, "/grpc.health.v1.") || strings.HasPrefix(info.FullMethod, "/grpc.reflection.") {
			return h(ctx, req)
		}
		if subtle.ConstantTimeCompare([]byte(firstMD(ctx, HeaderInternalToken)), want) != 1 {
			return nil, status.Error(codes.Unauthenticated, "internal token required")
		}
		return h(ctx, req)
	}
}

// Identity puts the user id forwarded by the gateway into ctx.
func Identity() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, h grpc.UnaryHandler) (any, error) {
		if uid := firstMD(ctx, HeaderUserID); uid != "" {
			ctx = auth.WithUserID(ctx, uid)
		}
		return h(ctx, req)
	}
}

type requestIDKey struct{}

func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}

func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

// Propagate forwards the internal token, user id and request id on outgoing calls.
func Propagate(internalToken string) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		kv := []string{HeaderInternalToken, internalToken}
		if id := RequestID(ctx); id != "" {
			kv = append(kv, HeaderRequestID, id)
		}
		if uid, err := auth.UserID(ctx); err == nil {
			kv = append(kv, HeaderUserID, uid)
		}
		ctx = metadata.AppendToOutgoingContext(ctx, kv...)
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

func firstMD(ctx context.Context, key string) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	if v := md.Get(key); len(v) > 0 {
		return v[0]
	}
	return ""
}
