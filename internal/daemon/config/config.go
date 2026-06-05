// Package config loads all daemon runtime configuration exclusively from
// environment variables. No configuration files are used at runtime; the
// Central Server delivers the gateway-specific config via the poller.
//
// Required environment variables:
//   - GATEWAY_ID      — unique identifier for this daemon instance
//   - CONFIG_SERVER_URL — HTTP(S) base URL of the Central Server
//   - DAEMON_API_KEY  — bearer token for authenticating Central Server requests
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config holds all daemon runtime configuration loaded from environment variables.
// Fields marked "required" cause Load to return an error when absent or empty.
//
// Thread-safety: Config is read-only after construction; safe for concurrent use.
type Config struct {
	// GatewayID uniquely identifies this daemon instance. Loaded from GATEWAY_ID (required).
	GatewayID string
	// ConfigServerURL is the HTTP(S) base URL of the Central Server.
	// Loaded from CONFIG_SERVER_URL (required).
	ConfigServerURL string
	// DaemonAPIKey is the bearer token used to authenticate requests to the Central Server.
	// Loaded from DAEMON_API_KEY (required).
	DaemonAPIKey string
	// RedisURL is the Redis connection string (e.g. redis://:pass@host:6379/0).
	// Loaded from REDIS_URL. Defaults to redis://localhost:6379/0 when absent.
	RedisURL string
	// KafkaBrokers is a comma-separated list of Kafka bootstrap broker addresses.
	// Loaded from KAFKA_BROKERS. Defaults to localhost:9092 when absent.
	KafkaBrokers string
	// PollIntervalSec is the number of seconds between config poll cycles.
	// Loaded from POLL_INTERVAL_SEC. Defaults to 30 when absent or invalid.
	PollIntervalSec int
	// MaxLivenessRetries is the maximum number of liveness probe attempts before the
	// daemon exits. 0 means retry indefinitely. Loaded from MAX_LIVENESS_RETRIES.
	MaxLivenessRetries int
	// EncryptKeyHex is the 64-character hex-encoded AES-256 encryption key used to
	// decrypt provider API keys stored in GatewayConfig. Loaded from ENCRYPT_KEY.
	EncryptKeyHex string
	// LogLevel controls zerolog output verbosity (trace/debug/info/warn/error/fatal).
	// Loaded from LOG_LEVEL. Defaults to "info".
	LogLevel string
	// OTELEndpoint is the OpenTelemetry OTLP gRPC endpoint for traces and metrics.
	// Loaded from OTEL_EXPORTER_OTLP_ENDPOINT. Empty string disables OTEL.
	OTELEndpoint string
}

// Load reads all daemon config from environment variables and returns a validated
// Config. Returns a non-nil error if any required field is missing or any numeric
// field cannot be parsed.
func Load() (*Config, error) {
	cfg := &Config{
		GatewayID:       os.Getenv("GATEWAY_ID"),
		ConfigServerURL: trimSlash(os.Getenv("CONFIG_SERVER_URL")),
		DaemonAPIKey:    os.Getenv("DAEMON_API_KEY"),
		RedisURL:        envOrDefault("REDIS_URL", "redis://localhost:6379/0"),
		KafkaBrokers:    envOrDefault("KAFKA_BROKERS", "localhost:9092"),
		EncryptKeyHex:   os.Getenv("ENCRYPT_KEY"),
		LogLevel:        envOrDefault("LOG_LEVEL", "info"),
		OTELEndpoint:    os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"),
	}

	// parse PollIntervalSec with default 30.
	if raw := os.Getenv("POLL_INTERVAL_SEC"); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil {
			return nil, fmt.Errorf("config.Load: POLL_INTERVAL_SEC is not a valid integer: %w", err)
		}
		cfg.PollIntervalSec = v
	} else {
		cfg.PollIntervalSec = 30
	}

	// parse MaxLivenessRetries; 0 is the valid "infinite" sentinel.
	if raw := os.Getenv("MAX_LIVENESS_RETRIES"); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil {
			return nil, fmt.Errorf("config.Load: MAX_LIVENESS_RETRIES is not a valid integer: %w", err)
		}
		cfg.MaxLivenessRetries = v
	}

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("config.Load: %w", err)
	}

	return cfg, nil
}

// validate checks that all required fields are non-empty.
func (c *Config) validate() error {
	required := map[string]string{
		"GATEWAY_ID":       c.GatewayID,
		"CONFIG_SERVER_URL": c.ConfigServerURL,
		"DAEMON_API_KEY":   c.DaemonAPIKey,
	}
	var missing []string
	for name, val := range required {
		if strings.TrimSpace(val) == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("required environment variable(s) not set: %s", strings.Join(missing, ", "))
	}
	return nil
}

// envOrDefault returns the value of the environment variable named by key, or
// fallback when the variable is absent or empty.
func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// trimSlash removes a trailing "/" from s so URL paths can be appended without
// producing double-slashes.
func trimSlash(s string) string {
	return strings.TrimRight(s, "/")
}
