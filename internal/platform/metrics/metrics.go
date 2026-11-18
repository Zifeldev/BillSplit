package metrics

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/Zifeldev/Billsplit/internal/platform/lifecycle"
)

var (
	GRPCRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "billsplit_grpc_requests_total",
		Help: "gRPC requests by method and status code.",
	}, []string{"method", "code"})

	GRPCDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "billsplit_grpc_request_duration_seconds",
		Help:    "gRPC request latency.",
		Buckets: prometheus.DefBuckets,
	}, []string{"method"})

	RateLimited = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "billsplit_rate_limited_total",
		Help: "Requests rejected by the rate limiter.",
	}, []string{"method"})

	HTTPRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "billsplit_http_requests_total",
		Help: "Gateway HTTP requests by RPC path and status.",
	}, []string{"path", "status"})

	HTTPDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "billsplit_http_request_duration_seconds",
		Help:    "Gateway HTTP latency.",
		Buckets: prometheus.DefBuckets,
	}, []string{"path"})

	OCRRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "billsplit_ocr_requests_total",
		Help: "OCR calls by provider and result.",
	}, []string{"provider", "result"})

	CacheRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "billsplit_cache_requests_total",
		Help: "Cache lookups by result (hit/miss/error).",
	}, []string{"result"})

	KafkaMessages = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "billsplit_kafka_messages_total",
		Help: "Kafka messages by topic, direction and status.",
	}, []string{"topic", "direction", "status"})

	KafkaHandleDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "billsplit_kafka_handle_duration_seconds",
		Help:    "Time to process a consumed message.",
		Buckets: []float64{.01, .05, .1, .5, 1, 2.5, 5, 10, 30},
	}, []string{"topic"})

	ReceiptsParsed = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "billsplit_receipts_parsed_total",
		Help: "Receipt parsing results.",
	}, []string{"status"})

	StatementRows = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "billsplit_statement_rows_total",
		Help: "Bank statement rows by result (imported/duplicate).",
	}, []string{"result"})
)

// Serve exposes /metrics, /livez (process alive) and /healthz (ready: 503 while
// shutting down) on addr until ctx is done. It stays up for the drain delay so
// probes can observe the 503.
func Serve(ctx context.Context, addr string) error {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/livez", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		if lifecycle.Draining() {
			http.Error(w, "shutting down", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}

	go func() {
		<-ctx.Done()
		lifecycle.WaitDrainDelay()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
