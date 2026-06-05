// Package config provides runtime configuration loading for the Central Server.
// All values are sourced from environment variables. Required variables cause
// Load to return an error if absent; optional variables fall back to defaults.
package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/google/uuid"
)

const (
	// defaultServerPort is the TCP port used when SERVER_PORT is not set.
	defaultServerPort = 9000

	// defaultLogLevel is used when LOG_LEVEL is not set.
	defaultLogLevel = "info"
)

// Config holds all Central Server runtime configuration.
//
// Thread-safety: Config is read-only after construction; safe for concurrent reads.
type Config struct {
	// DatabaseURL is the full PostgreSQL DSN (required). Read from DATABASE_URL.
	DatabaseURL string

	// RedisURL is the Redis connection URL. Read from REDIS_URL.
	// If empty, Redis-backed features (pub/sub invalidation) are disabled.
	RedisURL string

	// KafkaBrokers is a comma-separated list of Kafka broker addresses.
	// Read from KAFKA_BROKERS. Example: "192.168.32.161:9094".
	KafkaBrokers string

	// ServerPort is the TCP port the HTTP server listens on.
	// Read from SERVER_PORT; defaults to 9000.
	ServerPort int

	// AdminAPIKey is the Bearer token required for /admin/* endpoints (required).
	// Read from ADMIN_API_KEY.
	AdminAPIKey string

	// DaemonAPIKey is the Bearer token required for /config and /entitlement
	// endpoints consumed by Gateway Daemons (required). Read from DAEMON_API_KEY.
	DaemonAPIKey string

	// EncryptKeyHex is the 64-character hex AES-256 encryption key used to
	// encrypt/decrypt provider API keys at rest. Read from ENCRYPT_KEY.
	EncryptKeyHex string

	// LogLevel controls zerolog's global log level. Read from LOG_LEVEL;
	// defaults to "info". Valid values: trace|debug|info|warn|error|fatal|panic.
	LogLevel string

	// OTELEndpoint is the OpenTelemetry collector OTLP gRPC endpoint.
	// Read from OTEL_EXPORTER_OTLP_ENDPOINT. Example: "192.168.32.161:4317".
	OTELEndpoint string

	// JWTSecret is the HMAC-SHA256 signing key for console JWT tokens.
	// Read from JWT_SECRET (min 32 chars recommended). Defaults to a random
	// value if not set — sessions will not survive server restarts in that case.
	JWTSecret string

	// JaegerURL is the Jaeger UI base URL used to build trace deep-links.
	// Read from JAEGER_URL. Example: "http://192.168.32.161:16686".
	JaegerURL string

	// ServerID is a UUID auto-generated at startup to uniquely identify this
	// Central Server instance in health responses and logs.
	ServerID string
}

// Load reads all configuration from environment variables, applies defaults for
// optional keys, and validates that required keys are non-empty.
//
// Returns a non-nil error if DATABASE_URL, ADMIN_API_KEY, or DAEMON_API_KEY
// are absent.
func Load() (*Config, error) {
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		// fallback to a random per-process secret; sessions won't survive restarts.
		jwtSecret = uuid.NewString() + uuid.NewString()
	}

	cfg := &Config{
		DatabaseURL:   os.Getenv("DATABASE_URL"),
		RedisURL:      os.Getenv("REDIS_URL"),
		KafkaBrokers:  os.Getenv("KAFKA_BROKERS"),
		AdminAPIKey:   os.Getenv("ADMIN_API_KEY"),
		DaemonAPIKey:  os.Getenv("DAEMON_API_KEY"),
		EncryptKeyHex: os.Getenv("ENCRYPT_KEY"),
		LogLevel:      os.Getenv("LOG_LEVEL"),
		OTELEndpoint:  os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"),
		JWTSecret:     jwtSecret,
		JaegerURL:     os.Getenv("JAEGER_URL"),
		ServerID:      uuid.NewString(),
	}

	// apply optional defaults.
	if cfg.LogLevel == "" {
		cfg.LogLevel = defaultLogLevel
	}

	// parse SERVER_PORT, falling back to default.
	if portStr := os.Getenv("SERVER_PORT"); portStr != "" {
		p, err := strconv.Atoi(portStr)
		if err != nil {
			return nil, fmt.Errorf("config.Load: invalid SERVER_PORT %q: %w", portStr, err)
		}
		cfg.ServerPort = p
	} else {
		cfg.ServerPort = defaultServerPort
	}

	// validate required variables.
	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("config.Load: DATABASE_URL is required but not set")
	}
	if cfg.AdminAPIKey == "" {
		return nil, fmt.Errorf("config.Load: ADMIN_API_KEY is required but not set")
	}
	if cfg.DaemonAPIKey == "" {
		return nil, fmt.Errorf("config.Load: DAEMON_API_KEY is required but not set")
	}

	return cfg, nil
}
