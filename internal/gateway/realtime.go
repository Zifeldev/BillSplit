package gateway

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/Zifeldev/Billsplit/internal/platform/auth"
	"github.com/Zifeldev/Billsplit/internal/platform/realtime"
	pb "github.com/Zifeldev/Billsplit/pkg/pb/billsplit/v1"
)

// Live receipt updates over WebSocket: GET /v1/realtime/receipts/{id}?access_token=<jwt>.
//
// Browsers can't set headers on a WebSocket handshake, so the access token
// comes in the query string (the access log records only the path). Access is
// checked by asking the split service for the receipt as the caller — the
// same membership rule as GetReceipt. One Redis pattern subscription per
// gateway process feeds every socket; presence ("who has the receipt open")
// lives in a Redis sorted set so it works across gateway replicas.

const (
	presenceTTL     = 60 * time.Second
	heartbeat       = 25 * time.Second
	clientBuffer    = 16
	maxSocketsPerRx = 200
)

type ReceiptGetter interface {
	GetReceipt(ctx context.Context, req *pb.GetReceiptRequest, opts ...grpc.CallOption) (*pb.Receipt, error)
}

// Hub fans Redis pub/sub events out to local sockets.
type Hub struct {
	rdb *redis.Client
	pub realtime.Publisher
	log *slog.Logger

	mu   sync.Mutex
	subs map[string]map[*socket]struct{} // receipt id -> sockets
}

type socket struct {
	out chan []byte
}

func NewHub(rdb *redis.Client, log *slog.Logger) *Hub {
	return &Hub{rdb: rdb, pub: realtime.NewRedis(rdb), log: log, subs: map[string]map[*socket]struct{}{}}
}

// Run keeps the pattern subscription alive until ctx ends.
func (h *Hub) Run(ctx context.Context) error {
	for ctx.Err() == nil {
		ps := h.rdb.PSubscribe(ctx, realtime.ChannelPattern)
		ch := ps.Channel(redis.WithChannelHealthCheckInterval(30 * time.Second))
	loop:
		for {
			select {
			case <-ctx.Done():
				break loop
			case m, ok := <-ch:
				if !ok {
					break loop
				}
				if id, ok := realtime.ReceiptFromChannel(m.Channel); ok {
					h.broadcast(id, []byte(m.Payload))
				}
			}
		}
		_ = ps.Close()
		if ctx.Err() == nil {
			h.log.Warn("realtime: subscription closed, resubscribing")
			time.Sleep(time.Second)
		}
	}
	return nil
}

func (h *Hub) broadcast(receiptID string, msg []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for s := range h.subs[receiptID] {
		select {
		case s.out <- msg:
		default: // slow client: drop; it refetches on the next event anyway
		}
	}
}

func (h *Hub) add(receiptID string, s *socket) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	m := h.subs[receiptID]
	if m == nil {
		m = map[*socket]struct{}{}
		h.subs[receiptID] = m
	}
	if len(m) >= maxSocketsPerRx {
		return false
	}
	m[s] = struct{}{}
	return true
}

func (h *Hub) remove(receiptID string, s *socket) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.subs[receiptID], s)
	if len(h.subs[receiptID]) == 0 {
		delete(h.subs, receiptID)
	}
}

func presenceKey(receiptID string) string { return "rt:presence:" + receiptID }

// touch registers (or refreshes) one connection in the presence set.
func (h *Hub) touch(ctx context.Context, receiptID, member string) error {
	key := presenceKey(receiptID)
	p := h.rdb.TxPipeline()
	p.ZAdd(ctx, key, redis.Z{Score: float64(time.Now().Add(presenceTTL).Unix()), Member: member})
	p.Expire(ctx, key, 2*presenceTTL)
	_, err := p.Exec(ctx)
	return err
}

// announce publishes the current viewers (unique user ids) of the receipt.
func (h *Hub) announce(ctx context.Context, receiptID string) {
	key := presenceKey(receiptID)
	now := strconv.FormatInt(time.Now().Unix(), 10)
	h.rdb.ZRemRangeByScore(ctx, key, "-inf", "("+now)
	members, err := h.rdb.ZRange(ctx, key, 0, -1).Result()
	if err != nil {
		return
	}
	seen := map[string]bool{}
	viewers := []string{}
	for _, m := range members {
		uid, _, _ := strings.Cut(m, "|")
		if !seen[uid] {
			seen[uid] = true
			viewers = append(viewers, uid)
		}
	}
	h.pub.Publish(ctx, realtime.ReceiptChannel(receiptID), realtime.Event{
		Type: realtime.Presence, ReceiptID: receiptID, Viewers: viewers})
}

// Handler serves the WebSocket endpoint.
func (h *Hub) Handler(jwt *auth.JWTManager, receipts ReceiptGetter, origins []string) http.Handler {
	opts := &websocket.AcceptOptions{}
	if len(origins) == 1 && origins[0] == "*" {
		opts.InsecureSkipVerify = true
	} else {
		for _, o := range origins {
			o = strings.TrimSpace(o)
			_, host, ok := strings.Cut(o, "://")
			if !ok {
				host = o
			}
			opts.OriginPatterns = append(opts.OriginPatterns, host)
		}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receiptID := r.PathValue("id")
		if _, err := uuid.Parse(receiptID); err != nil {
			writeErr(w, http.StatusBadRequest, 3, "bad receipt id")
			return
		}
		uid, err := jwt.Parse(r.URL.Query().Get("access_token"), auth.TokenAccess)
		if err != nil {
			writeErr(w, http.StatusUnauthorized, 16, "invalid or expired token")
			return
		}
		ctx := auth.WithUserID(r.Context(), uid)
		cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		_, err = receipts.GetReceipt(cctx, &pb.GetReceiptRequest{ReceiptId: receiptID})
		cancel()
		if err != nil {
			code := http.StatusBadGateway
			if s, ok := status.FromError(err); ok && (s.Code() == codes.NotFound || s.Code() == codes.PermissionDenied) {
				code = http.StatusNotFound
			}
			writeErr(w, code, 5, "receipt not found")
			return
		}

		// The HTTP server's read/write timeouts must not kill a long-lived socket.
		rc := http.NewResponseController(w)
		_ = rc.SetReadDeadline(time.Time{})
		_ = rc.SetWriteDeadline(time.Time{})
		conn, err := websocket.Accept(w, r, opts)
		if err != nil {
			return // Accept already wrote the error response
		}
		defer conn.CloseNow()
		h.serve(ctx, conn, receiptID, uid)
	})
}

func (h *Hub) serve(ctx context.Context, conn *websocket.Conn, receiptID, uid string) {
	s := &socket{out: make(chan []byte, clientBuffer)}
	if !h.add(receiptID, s) {
		_ = conn.Close(websocket.StatusTryAgainLater, "too many viewers")
		return
	}
	defer h.remove(receiptID, s)

	// Clients only listen; CloseRead handles pings/close frames and cancels ctx on disconnect.
	ctx = conn.CloseRead(ctx)
	member := uid + "|" + uuid.NewString()
	if err := h.touch(ctx, receiptID, member); err == nil {
		h.announce(ctx, receiptID)
	}
	defer func() {
		bg, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer cancel()
		h.rdb.ZRem(bg, presenceKey(receiptID), member)
		h.announce(bg, receiptID)
	}()

	hello, _ := json.Marshal(realtime.Event{Type: "hello", ReceiptID: receiptID, UserID: uid})
	if err := write(ctx, conn, hello); err != nil {
		return
	}
	tick := time.NewTicker(heartbeat)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-s.out:
			if err := write(ctx, conn, msg); err != nil {
				return
			}
		case <-tick.C:
			_ = h.touch(ctx, receiptID, member)
			pctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			err := conn.Ping(pctx)
			cancel()
			if err != nil {
				return
			}
		}
	}
}

func write(ctx context.Context, conn *websocket.Conn, msg []byte) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return conn.Write(ctx, websocket.MessageText, msg)
}
