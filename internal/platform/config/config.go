// Package config loads settings from environment variables. Every service
// reads the same struct and uses only the parts it needs.
package config

import (
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/caarlos0/env/v11"
)

type Config struct {
	Env      string `env:"APP_ENV" envDefault:"local"` // local | prod
	LogLevel string `env:"LOG_LEVEL" envDefault:"info"`

	GRPC     GRPC
	Metrics  Metrics
	Postgres Postgres
	Redis    Redis
	Kafka    Kafka
	JWT      JWT
	Limits   Limits
	Services Services
	Gateway  Gateway
	OCR      OCR
	FX       FX
	Market   Market
	Security Security
	Push     Push
	Shutdown Shutdown
}

// Shutdown — graceful shutdown timings (see internal/platform/lifecycle).
// Keep Timeout below the orchestrator's kill grace period (compose stop_grace_period: 35s).
type Shutdown struct {
	Timeout    time.Duration `env:"SHUTDOWN_TIMEOUT" envDefault:"25s"`
	DrainDelay time.Duration `env:"SHUTDOWN_DRAIN_DELAY" envDefault:"2s"`
}

type Security struct {
	// Shared secret every internal gRPC call must carry (x-internal-token).
	// Backends reject calls without it, so only our services can talk to them.
	InternalToken string `env:"INTERNAL_TOKEN" envDefault:"local-internal-token-change-me"`
}

type Push struct {
	// Optional Expo access token (enhanced push security); empty works for development.
	ExpoAccessToken string `env:"EXPO_ACCESS_TOKEN"`
}

type GRPC struct {
	Addr           string        `env:"GRPC_ADDR" envDefault:":50051"`
	RequestTimeout time.Duration `env:"GRPC_REQUEST_TIMEOUT" envDefault:"15s"`
	MaxRecvMB      int           `env:"GRPC_MAX_RECV_MB" envDefault:"12"`
	Reflection     bool          `env:"GRPC_REFLECTION" envDefault:"true"`
}

type Metrics struct {
	Addr string `env:"METRICS_ADDR" envDefault:":9090"`
}

type Postgres struct {
	DSN string `env:"POSTGRES_DSN" envDefault:"postgres://billsplit:billsplit@localhost:5432/billsplit?sslmode=disable"`
	// Password overrides the one in the DSN — keeps the DSN free of secrets
	// (set POSTGRES_PASSWORD_FILE=/run/secrets/<service>_db_password).
	Password string `env:"POSTGRES_PASSWORD"`
	Schema   string `env:"POSTGRES_SCHEMA"` // each service owns one schema
	MaxConns int32  `env:"POSTGRES_MAX_CONNS" envDefault:"10"`
}

type Redis struct {
	Addr     string        `env:"REDIS_ADDR" envDefault:"localhost:6379"`
	Password string        `env:"REDIS_PASSWORD"`
	DB       int           `env:"REDIS_DB" envDefault:"0"`
	TTL      time.Duration `env:"CACHE_TTL" envDefault:"5m"`
}

type Kafka struct {
	Brokers []string `env:"KAFKA_BROKERS" envSeparator:"," envDefault:"localhost:29092"`
	GroupID string   `env:"KAFKA_GROUP_ID"` // defaults to service name
}

type JWT struct {
	Secret     string        `env:"JWT_SECRET" envDefault:"change-me-in-production-please"`
	AccessTTL  time.Duration `env:"JWT_ACCESS_TTL" envDefault:"15m"`
	RefreshTTL time.Duration `env:"JWT_REFRESH_TTL" envDefault:"720h"`
}

// Limits — rate limiting at the gateway (token bucket per client IP).
type Limits struct {
	RPS       float64 `env:"RATE_LIMIT_RPS" envDefault:"20"`
	Burst     int     `env:"RATE_LIMIT_BURST" envDefault:"40"`
	AuthRPS   float64 `env:"RATE_LIMIT_AUTH_RPS" envDefault:"1"` // brute-force protection for Login/Register
	AuthBurst int     `env:"RATE_LIMIT_AUTH_BURST" envDefault:"5"`
}

// Services — internal gRPC addresses.
type Services struct {
	Auth    string `env:"AUTH_ADDR" envDefault:"localhost:50051"`
	Social  string `env:"SOCIAL_ADDR" envDefault:"localhost:50052"`
	Split   string `env:"SPLIT_ADDR" envDefault:"localhost:50053"`
	Finance string `env:"FINANCE_ADDR" envDefault:"localhost:50054"`
	Notify  string `env:"NOTIFY_ADDR" envDefault:"localhost:50055"`
}

type Gateway struct {
	Addr        string   `env:"GATEWAY_ADDR" envDefault:":8088"`
	CORSOrigins []string `env:"CORS_ORIGINS" envSeparator:"," envDefault:"*"`
	// Public web app URL used to build invite links.
	WebURL    string `env:"WEB_URL" envDefault:"http://localhost:8081"`
	AppScheme string `env:"APP_SCHEME" envDefault:"billsplit"`
}

type OCR struct {
	URL            string `env:"OCR_URL" envDefault:"http://localhost:8000"` // RapidOCR service
	OCRSpaceAPIKey string `env:"OCRSPACE_API_KEY"`                           // optional free fallback
	// Optional Claude vision provider — most accurate on crumpled/long receipts.
	AnthropicAPIKey string `env:"ANTHROPIC_API_KEY"`
	AnthropicModel  string `env:"ANTHROPIC_MODEL"`
}

// Market data (Yahoo Finance chart/search API, no key).
type Market struct {
	BaseURL string `env:"MARKET_BASE_URL" envDefault:"https://query1.finance.yahoo.com"`
}

type FX struct {
	BaseURL string `env:"FX_BASE_URL" envDefault:"https://api.frankfurter.dev/v1"`
}

// secretVars may be provided as files (Docker secrets): NAME_FILE=/run/secrets/x
// takes precedence over NAME. Values never have to appear in the environment,
// `docker inspect` or compose files.
var secretVars = []string{
	"JWT_SECRET", "INTERNAL_TOKEN", "POSTGRES_PASSWORD", "REDIS_PASSWORD",
	"OCRSPACE_API_KEY", "ANTHROPIC_API_KEY", "EXPO_ACCESS_TOKEN",
}

func loadSecretFiles() error {
	for _, name := range secretVars {
		path := os.Getenv(name + "_FILE")
		if path == "" {
			continue
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s_FILE: %w", name, err)
		}
		if err := os.Setenv(name, strings.TrimSpace(string(b))); err != nil {
			return err
		}
	}
	return nil
}

func Load() (*Config, error) {
	if err := loadSecretFiles(); err != nil {
		return nil, err
	}
	var c Config
	if err := env.Parse(&c); err != nil {
		return nil, fmt.Errorf("parse env: %w", err)
	}
	if c.Postgres.Password != "" {
		u, err := url.Parse(c.Postgres.DSN)
		if err != nil {
			return nil, fmt.Errorf("POSTGRES_DSN: %w", err)
		}
		u.User = url.UserPassword(u.User.Username(), c.Postgres.Password)
		c.Postgres.DSN = u.String()
	}
	if c.Env == "prod" && len(c.JWT.Secret) < 32 {
		return nil, fmt.Errorf("JWT_SECRET must be at least 32 chars in prod")
	}
	if c.Env == "prod" && len(c.Security.InternalToken) < 32 {
		return nil, fmt.Errorf("INTERNAL_TOKEN must be at least 32 chars in prod")
	}
	return &c, nil
}
