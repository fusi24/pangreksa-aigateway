// Package logger provides a thin zerolog-based structured logger with context helpers.
// It reads the LOG_LEVEL environment variable to set the global log level.
// The logger is configured for performance: caller info is disabled, and console
// writer is skipped in favour of JSON output suitable for log aggregation pipelines.
//
// Usage:
//
//	log := logger.New()
//	ctx  := logger.WithContext(ctx, log)
//	// ... in a handler:
//	l := logger.FromContext(ctx)
//	l.Info().Str("request_id", id).Msg("request received")
package logger

import (
	"context"
	"os"
	"strings"

	"github.com/rs/zerolog"
)

// contextKey is the unexported type used as the context key to avoid collisions
// with keys from other packages.
type contextKey struct{}

// New constructs a *zerolog.Logger configured from the LOG_LEVEL environment
// variable. If LOG_LEVEL is absent or unrecognised, the level defaults to "info".
//
// Caller info is intentionally disabled for hot-path performance.
// Output is JSON written to os.Stdout.
func New() *zerolog.Logger {
	level := parseLevel(os.Getenv("LOG_LEVEL"))
	zerolog.SetGlobalLevel(level)

	log := zerolog.New(os.Stdout).
		With().
		Timestamp().
		Logger()

	return &log
}

// WithContext stores the given logger in the context and returns a derived context.
// Downstream code retrieves it via FromContext.
//
// ctx must be non-nil; l must be non-nil.
func WithContext(ctx context.Context, l *zerolog.Logger) context.Context {
	return context.WithValue(ctx, contextKey{}, l)
}

// FromContext retrieves the *zerolog.Logger stored in ctx by WithContext.
// If no logger is found (e.g. in tests or early startup), a no-op logger
// at the Disabled level is returned so callers never need to nil-check.
func FromContext(ctx context.Context) *zerolog.Logger {
	if l, ok := ctx.Value(contextKey{}).(*zerolog.Logger); ok && l != nil {
		return l
	}
	// noop is returned so FromContext callers never need to guard against nil.
	noop := zerolog.Nop()
	return &noop
}

// parseLevel converts the LOG_LEVEL string to a zerolog.Level.
// Recognised values (case-insensitive): trace, debug, info, warn, error, fatal, panic.
// Any unrecognised value returns zerolog.InfoLevel.
func parseLevel(s string) zerolog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "trace":
		return zerolog.TraceLevel
	case "debug":
		return zerolog.DebugLevel
	case "info":
		return zerolog.InfoLevel
	case "warn", "warning":
		return zerolog.WarnLevel
	case "error":
		return zerolog.ErrorLevel
	case "fatal":
		return zerolog.FatalLevel
	case "panic":
		return zerolog.PanicLevel
	case "disabled", "off":
		return zerolog.Disabled
	default:
		return zerolog.InfoLevel
	}
}
