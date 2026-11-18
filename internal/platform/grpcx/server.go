package grpcx

import (
	"context"
	"log/slog"
	"net"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/reflection"

	"github.com/Zifeldev/Billsplit/internal/platform/config"
	"github.com/Zifeldev/Billsplit/internal/platform/lifecycle"
)

// NewServer builds a gRPC server with the standard interceptor chain, limits and health checks.
func NewServer(cfg *config.Config, log *slog.Logger) (*grpc.Server, *health.Server) {
	srv := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			Recovery(),
			Logging(log),
			Metrics(),
			ServiceAuth(cfg.Security.InternalToken),
			Timeout(cfg.GRPC.RequestTimeout),
			Identity(),
		),
		grpc.MaxRecvMsgSize(cfg.GRPC.MaxRecvMB<<20),
		grpc.MaxConcurrentStreams(1000),
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionIdle: 5 * time.Minute,
			Time:              2 * time.Minute,
			Timeout:           20 * time.Second,
			// Recycle client connections every minute (GOAWAY, in-flight calls finish).
			// Clients then re-resolve DNS: after containers are recreated and IPs
			// reshuffled, a client stuck on an address that now belongs to ANOTHER
			// service ("unknown service ...") heals itself within a minute.
			MaxConnectionAge:      time.Minute,
			MaxConnectionAgeGrace: 15 * time.Second,
		}),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{MinTime: 30 * time.Second, PermitWithoutStream: true}),
	)
	hs := health.NewServer()
	healthpb.RegisterHealthServer(srv, hs)
	if cfg.GRPC.Reflection {
		reflection.Register(srv)
	}
	return srv, hs
}

// Serve runs srv until ctx is cancelled, then stops gracefully (bounded by 15s).
func Serve(ctx context.Context, srv *grpc.Server, hs *health.Server, addr string, log *slog.Logger) error {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	go func() {
		<-ctx.Done()
		// 1) readiness: health checks report NOT_SERVING so clients/LBs move away;
		// 2) keep serving for the drain delay; 3) stop accepting and wait for in-flight RPCs.
		hs.Shutdown()
		lifecycle.WaitDrainDelay()
		log.Info("grpc: stopping, waiting for in-flight requests")
		done := make(chan struct{})
		go func() { srv.GracefulStop(); close(done) }()
		select {
		case <-done:
			log.Info("grpc: stopped gracefully")
		case <-time.After(lifecycle.Timeout() - lifecycle.DrainDelay()):
			log.Warn("grpc: graceful stop timed out, closing connections")
			srv.Stop()
		}
	}()
	hs.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	log.Info("grpc listening", "addr", addr)
	return srv.Serve(lis)
}

// Dial opens a client connection to another internal service. The address is
// always resolved via DNS so reconnects pick up containers' new IPs.
func Dial(addr, internalToken string) (*grpc.ClientConn, error) {
	if !strings.Contains(addr, "://") {
		addr = "dns:///" + addr
	}
	return grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithChainUnaryInterceptor(Propagate(internalToken)),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(16<<20)),
		// Retry transient failures of idempotent reads.
		grpc.WithDefaultServiceConfig(`{"methodConfig":[{"name":[{}],"timeout":"10s",
			"retryPolicy":{"maxAttempts":3,"initialBackoff":"0.1s","maxBackoff":"1s","backoffMultiplier":2,
			"retryableStatusCodes":["UNAVAILABLE"]}}]}`),
	)
}
