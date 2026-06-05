// Package proxy provides the common HTTP server infrastructure for per-provider
// LLM proxy listeners. Each provider runs its own Server on a dedicated port.
// Incoming requests are normalized to InquiryRequest via the provider-specific
// Adapter, forwarded through the Pipeline, and the raw provider response is
// returned to the caller.
//
// Every response includes:
//
//	X-Gateway-Request-ID — the request UUID assigned at ingress
//	X-Gateway-Latency-Ms — total gateway latency in milliseconds
package proxy

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog"

	"github.com/pangreksa/ai-gateway-engine/pkg/model"
)

// Adapter normalizes provider-specific HTTP requests into an InquiryRequest
// and formats the raw provider response for the downstream client.
//
// Implementations are provided for openai, claude, moonshot, and ollama.
type Adapter interface {
	// Name returns the canonical provider name (e.g. "openai").
	Name() string
	// Normalize reads the HTTP request body and headers and returns the
	// unified InquiryRequest. The token field is populated from the
	// Authorization header by the Server before Normalize is called.
	Normalize(r *http.Request) (*model.InquiryRequest, error)
	// FormatResponse transforms the raw provider response bytes into the
	// wire format expected by the client. For most providers this is a
	// pass-through; adapters may rewrite the body if needed.
	FormatResponse(providerResp []byte, req *model.InquiryRequest) ([]byte, error)
}

// Pipeline executes the full hot-path for each request: entitlement resolution,
// registry lookups, policy checks, LLM dispatch, and Kafka publish.
type Pipeline interface {
	// Execute processes the inquiry and returns the raw LLM response bytes.
	// It is the caller's responsibility to pass an InquiryRequest with a
	// populated UserID, OrgID, Provider, and RequestID.
	Execute(ctx context.Context, req *model.InquiryRequest) ([]byte, error)
}

// Server is a single provider proxy HTTP server that listens on the port
// specified by cfg.Port and routes all requests through the given Pipeline.
//
// Thread-safety: Start may only be called once per Server instance.
type Server struct {
	// cfg holds the provider-specific proxy configuration.
	cfg model.ProxyConfig
	// adapter normalizes provider-specific request/response formats.
	adapter Adapter
	// pipeline executes the hot-path for each normalized request.
	pipeline Pipeline
	// log is the structured logger scoped to this server.
	log zerolog.Logger
}

// NewServer creates a proxy server for the given provider config.
//
// cfg.Port defines the listen port; cfg.Provider is used for logging.
// adapter handles provider-specific request/response normalization.
// pipeline executes the full per-request hot path.
func NewServer(cfg model.ProxyConfig, adapter Adapter, pipeline Pipeline, log zerolog.Logger) *Server {
	return &Server{
		cfg:      cfg,
		adapter:  adapter,
		pipeline: pipeline,
		log:      log.With().Str("provider", cfg.Provider).Int("port", cfg.Port).Logger(),
	}
}

// Start begins listening on cfg.Port and serves requests until ctx is cancelled.
// Returns an error if the listener cannot be bound or the server exits unexpectedly.
func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleRequest)

	addr := ":" + strconv.Itoa(s.cfg.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  120 * time.Second,
		WriteTimeout: 180 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// bind the listener before the goroutine so we can surface bind errors.
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("proxy.Server.Start [%s]: bind %s: %w", s.cfg.Provider, addr, err)
	}

	s.log.Info().Str("addr", addr).Msg("proxy server listening")

	// shutdown goroutine: close listener when ctx is cancelled.
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			s.log.Error().Err(err).Msg("proxy server shutdown error")
		}
	}()

	if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("proxy.Server.Start [%s]: serve: %w", s.cfg.Provider, err)
	}

	return nil
}

// handleRequest is the single HTTP handler for all requests received by this proxy.
// It extracts the bearer token, normalizes the request, runs the pipeline, and
// writes the provider response to the client.
func (s *Server) handleRequest(w http.ResponseWriter, r *http.Request) {
	received := time.Now()

	token := extractBearerToken(r)

	// inject the bearer token as a header so Normalize implementations can read it.
	r.Header.Set("X-Gateway-Token", token)

	req, err := s.adapter.Normalize(r)
	if err != nil {
		s.log.Warn().Err(err).Msg("request normalization failed")
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusBadRequest)
		return
	}

	// set response correlation headers early so they appear even on error.
	w.Header().Set("X-Gateway-Request-ID", req.RequestID)

	respBytes, err := s.pipeline.Execute(r.Context(), req)
	if err != nil {
		latency := time.Since(received).Milliseconds()
		w.Header().Set("X-Gateway-Latency-Ms", strconv.FormatInt(latency, 10))
		status, body := mapPipelineError(err)
		s.log.Warn().Err(err).Str("request_id", req.RequestID).Msg("pipeline error")
		http.Error(w, body, status)
		return
	}

	formatted, err := s.adapter.FormatResponse(respBytes, req)
	if err != nil {
		s.log.Warn().Err(err).Str("request_id", req.RequestID).Msg("response formatting failed")
		formatted = respBytes // fall back to raw bytes on formatting error
	}

	latency := time.Since(received).Milliseconds()
	w.Header().Set("X-Gateway-Latency-Ms", strconv.FormatInt(latency, 10))
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(formatted)
}

// extractBearerToken reads the Bearer token from the Authorization header.
// Returns empty string when the header is absent or has an unexpected format.
func extractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return ""
	}
	parts := strings.SplitN(auth, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

// mapPipelineError converts a pipeline error into an HTTP status code and body.
// Budget/rate errors become 429; policy blocks become 403; others become 502.
func mapPipelineError(err error) (int, string) {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "rate limit") || strings.Contains(msg, "too many requests"):
		return http.StatusTooManyRequests, `{"error":"rate limit exceeded"}`
	case strings.Contains(msg, "budget exceeded") || strings.Contains(msg, "budget limit"):
		return http.StatusTooManyRequests, `{"error":"budget limit exceeded"}`
	case strings.Contains(msg, "blocked") || strings.Contains(msg, "policy"):
		return http.StatusForbidden, `{"error":"request blocked by policy"}`
	case strings.Contains(msg, "entitlement") || strings.Contains(msg, "unauthorized"):
		return http.StatusUnauthorized, `{"error":"unauthorized"}`
	case strings.Contains(msg, "circuit breaker"):
		return http.StatusServiceUnavailable, `{"error":"provider unavailable"}`
	default:
		// Include the underlying error detail to aid debugging.
		detail := strings.ReplaceAll(msg, `"`, `'`)
		return http.StatusBadGateway, fmt.Sprintf(`{"error":"gateway error","detail":%q}`, detail)
	}
}
