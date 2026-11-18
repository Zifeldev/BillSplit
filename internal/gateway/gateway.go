// Package gateway is the single public entry point for web and mobile clients.
//
// It translates HTTP/JSON (POST /billsplit.v1.<Service>/<Method>) into gRPC
// calls to the backend services (grpc-gateway), and owns cross-cutting edge
// concerns: CORS, request ids, access logs, metrics, body limits, rate limiting
// and JWT authentication. The authenticated user id is forwarded to backends
// as x-user-id metadata; client-supplied metadata is never forwarded, so it
// can't be spoofed.
package gateway

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/Zifeldev/Billsplit/internal/platform/auth"
	"github.com/Zifeldev/Billsplit/internal/platform/config"
	"github.com/Zifeldev/Billsplit/internal/platform/grpcx"
	"github.com/Zifeldev/Billsplit/internal/platform/lifecycle"
	"github.com/Zifeldev/Billsplit/internal/platform/logger"
	"github.com/Zifeldev/Billsplit/internal/platform/metrics"
	pb "github.com/Zifeldev/Billsplit/pkg/pb/billsplit/v1"
)

// Requests that don't need a JWT. If a valid Bearer token is present anyway,
// the user id is still forwarded (e.g. GetInvite reports already_member).
var publicPaths = map[string]bool{
	pb.AuthService_Register_FullMethodName:          true,
	pb.AuthService_Login_FullMethodName:             true,
	pb.AuthService_Refresh_FullMethodName:           true,
	pb.GroupService_GetInvite_FullMethodName:        true,
	pb.FinanceService_ListCurrencies_FullMethodName: true,
}

// Requests rate-limited strictly (credential brute force).
var strictPaths = map[string]bool{
	pb.AuthService_Register_FullMethodName: true,
	pb.AuthService_Login_FullMethodName:    true,
	pb.AuthService_Refresh_FullMethodName:  true,
}

// Limiter is satisfied by ratelimit.Limiter (in-memory) and ratelimit.Redis (shared).
type Limiter interface {
	Allow(key string, strict bool) bool
	Cleanup(ctx context.Context, idle time.Duration)
}

// 8 MB image becomes ~11 MB as base64 JSON.
const maxBodyBytes = 16 << 20

type Conns struct {
	Auth, Social, Split, Finance, Notify *grpc.ClientConn
}

// NewHandler builds the full HTTP handler.
// hub (optional) serves live receipt updates over WebSocket.
func NewHandler(ctx context.Context, cfg *config.Config, conns Conns, jwt *auth.JWTManager,
	rl Limiter, hub *Hub, log *slog.Logger) (http.Handler, error) {
	mux := runtime.NewServeMux(
		runtime.WithMarshalerOption(runtime.MIMEWildcard, &runtime.JSONPb{
			MarshalOptions:   protojson.MarshalOptions{EmitUnpopulated: true},
			UnmarshalOptions: protojson.UnmarshalOptions{DiscardUnknown: true},
		}),
		// Never forward client headers as gRPC metadata (prevents x-user-id spoofing).
		runtime.WithIncomingHeaderMatcher(func(string) (string, bool) { return "", false }),
		runtime.WithOutgoingHeaderMatcher(func(string) (string, bool) { return "", false }),
	)

	regs := []struct {
		conn *grpc.ClientConn
		fn   func(context.Context, *runtime.ServeMux, *grpc.ClientConn) error
	}{
		{conns.Auth, pb.RegisterAuthServiceHandler},
		{conns.Social, pb.RegisterFriendServiceHandler},
		{conns.Social, pb.RegisterGroupServiceHandler},
		{conns.Split, pb.RegisterReceiptServiceHandler},
		{conns.Split, pb.RegisterBalanceServiceHandler},
		{conns.Finance, pb.RegisterFinanceServiceHandler},
		{conns.Notify, pb.RegisterNotificationServiceHandler},
	}
	for _, r := range regs {
		if err := r.fn(ctx, mux, r.conn); err != nil {
			return nil, err
		}
	}

	root := http.NewServeMux()
	// Readiness: 503 while shutting down so load balancers stop routing here.
	root.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		if lifecycle.Draining() {
			http.Error(w, "shutting down", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	root.HandleFunc("GET /livez", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	if hub != nil {
		root.Handle("GET /v1/realtime/receipts/{id}", hub.Handler(jwt, pb.NewReceiptServiceClient(conns.Split), cfg.Gateway.CORSOrigins))
	}
	root.Handle("/", authenticate(jwt, mux))

	var h http.Handler = root
	h = rateLimit(rl, h)
	h = limitBody(h)
	h = accessLog(log, h)
	h = cors(cfg.Gateway.CORSOrigins, h)
	h = requestID(h)
	h = recoverer(log, h)
	return h, nil
}

// ---------------- middleware ----------------

func recoverer(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Error("panic recovered", "panic", rec, "stack", string(debug.Stack()))
				writeErr(w, http.StatusInternalServerError, 13, "internal error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-Id")
		if id == "" || len(id) > 64 {
			id = uuid.NewString()
		}
		w.Header().Set("X-Request-Id", id)
		next.ServeHTTP(w, r.WithContext(grpcx.WithRequestID(r.Context(), id)))
	})
}

func cors(origins []string, next http.Handler) http.Handler {
	allowAll := len(origins) == 1 && origins[0] == "*"
	allowed := map[string]bool{}
	for _, o := range origins {
		allowed[strings.TrimSpace(o)] = true
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin := r.Header.Get("Origin"); origin != "" && (allowAll || allowed[origin]) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Request-Id")
			w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
			w.Header().Set("Access-Control-Expose-Headers", "X-Request-Id")
			w.Header().Set("Access-Control-Max-Age", "600")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (s *statusWriter) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// Unwrap lets http.ResponseController reach Hijack/deadlines (WebSocket upgrade).
func (s *statusWriter) Unwrap() http.ResponseWriter { return s.ResponseWriter }

func accessLog(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		l := log.With("request_id", grpcx.RequestID(r.Context()), "path", r.URL.Path)
		next.ServeHTTP(sw, r.WithContext(logger.WithContext(r.Context(), l)))

		path := r.URL.Path
		if sw.status == http.StatusNotFound && !strings.HasPrefix(path, "/billsplit.v1.") || len(path) > 100 {
			path = "other" // keep metric cardinality bounded
		}
		metrics.HTTPRequests.WithLabelValues(path, strconv.Itoa(sw.status)).Inc()
		metrics.HTTPDuration.WithLabelValues(path).Observe(time.Since(start).Seconds())

		attrs := []any{"method", r.Method, "status", sw.status, "duration_ms", time.Since(start).Milliseconds(), "ip", clientIP(r)}
		switch {
		case sw.status >= 500:
			l.Error("http request", attrs...)
		case sw.status >= 400:
			l.Warn("http request", attrs...)
		default:
			l.Info("http request", attrs...)
		}
	})
}

func limitBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ContentLength > maxBodyBytes {
			writeErr(w, http.StatusRequestEntityTooLarge, 8, "request body too large")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
		next.ServeHTTP(w, r)
	})
}

func rateLimit(rl Limiter, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" || r.URL.Path == "/livez" {
			next.ServeHTTP(w, r)
			return
		}
		if !rl.Allow(clientIP(r), strictPaths[r.URL.Path]) {
			metrics.RateLimited.WithLabelValues(r.URL.Path).Inc()
			w.Header().Set("Retry-After", "1")
			writeErr(w, http.StatusTooManyRequests, 8, "rate limit exceeded, slow down")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func authenticate(jwt *auth.JWTManager, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		public := publicPaths[r.URL.Path]
		token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || token == "" {
			if public {
				next.ServeHTTP(w, r)
				return
			}
			writeErr(w, http.StatusUnauthorized, 16, "missing bearer token")
			return
		}
		uid, err := jwt.Parse(token, auth.TokenAccess)
		if err != nil {
			if public { // a stale token must not break public pages: continue anonymously
				next.ServeHTTP(w, r)
				return
			}
			writeErr(w, http.StatusUnauthorized, 16, "invalid or expired token")
			return
		}
		ctx := auth.WithUserID(r.Context(), uid) // forwarded as x-user-id by grpcx.Propagate
		ctx = logger.WithContext(ctx, logger.FromContext(ctx).With("user_id", uid))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// clientIP uses the TCP peer. If the peer is a private/loopback address (our
// own reverse proxy / load balancer) and X-Forwarded-For is set, the first
// XFF entry is the client. Public peers can't spoof their IP via XFF.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	xff := r.Header.Get("X-Forwarded-For")
	if xff == "" {
		return host
	}
	peer, err := netip.ParseAddr(host)
	if err != nil {
		return host
	}
	peer = peer.Unmap()
	if !peer.IsLoopback() && !peer.IsPrivate() {
		return host
	}
	first, _, _ := strings.Cut(xff, ",")
	ip, err := netip.ParseAddr(strings.TrimSpace(first))
	if err != nil {
		return host
	}
	return ip.Unmap().String()
}

// writeErr uses the same shape as grpc-gateway errors: {"code": <grpc code>, "message": "..."}.
func writeErr(w http.ResponseWriter, status, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"code": code, "message": msg, "details": []any{}})
}
