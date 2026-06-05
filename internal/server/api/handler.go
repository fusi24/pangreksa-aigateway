// Package api implements the Central Server HTTP API.
// All endpoints use POST method and JSON request/response bodies.
// Authentication is Bearer token-based with constant-time comparison.
package api

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/rs/zerolog"

	"github.com/pangreksa/ai-gateway-engine/internal/server/configstore"
	"github.com/pangreksa/ai-gateway-engine/internal/server/entitlement"
	"github.com/pangreksa/ai-gateway-engine/internal/server/publisher"
	"github.com/pangreksa/ai-gateway-engine/pkg/model"
)

// Handler implements all Central Server HTTP endpoints.
// It is wired with configstore, entitlement service, and publisher at construction.
//
// Thread-safety: safe for concurrent use; all state is read-only after construction.
type Handler struct {
	// cfgStore is the GatewayConfig management layer.
	cfgStore *configstore.Store
	// entService is the entitlement business logic layer.
	entService *entitlement.Service
	// pub is the Redis publisher for ad-hoc invalidation.
	pub *publisher.Publisher
	// serverID is the auto-generated instance identifier returned in /health.
	serverID string
	// adminAPIKey is the Bearer token required for /admin/* endpoints.
	adminAPIKey string
	// daemonAPIKey is the Bearer token required for daemon-facing endpoints.
	daemonAPIKey string
	// log is the structured logger for HTTP request events.
	log zerolog.Logger
}

// NewHandler constructs a Handler with all required dependencies.
// All parameters must be non-nil/non-empty.
func NewHandler(
	cfgStore *configstore.Store,
	entService *entitlement.Service,
	pub *publisher.Publisher,
	serverID string,
	adminAPIKey string,
	daemonAPIKey string,
	log zerolog.Logger,
) *Handler {
	return &Handler{
		cfgStore:     cfgStore,
		entService:   entService,
		pub:          pub,
		serverID:     serverID,
		adminAPIKey:  adminAPIKey,
		daemonAPIKey: daemonAPIKey,
		log:          log.With().Str("component", "api.handler").Logger(),
	}
}

// Register mounts all Central Server routes onto the given mux.
// Route layout:
//
//	POST /health              — liveness probe (no auth)
//	POST /config              — config poll (daemon auth)
//	POST /entitlement         — entitlement resolution (daemon auth)
//	POST /admin/config        — store new config (admin auth)
//	POST /admin/entitlement   — update entitlement (admin auth)
//	POST /admin/invalidate    — publish Redis invalidation (admin auth)
//	GET  /metrics             — Prometheus metrics stub (no auth)
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /health", h.handleHealth)
	mux.HandleFunc("POST /config", h.handleConfig)
	mux.HandleFunc("POST /entitlement", h.handleEntitlement)
	mux.HandleFunc("POST /admin/config", h.handleAdminConfig)
	mux.HandleFunc("POST /admin/entitlement", h.handleAdminEntitlement)
	mux.HandleFunc("POST /admin/invalidate", h.handleAdminInvalidate)
	mux.HandleFunc("GET /metrics", h.handleMetrics)
	// fallback to handle non-pattern routes on older Go routing implementations.
	mux.HandleFunc("/health", h.handleHealth)
	mux.HandleFunc("/config", h.withAuth(h.daemonAPIKey, h.handleConfig))
	mux.HandleFunc("/entitlement", h.withAuth(h.daemonAPIKey, h.handleEntitlement))
	mux.HandleFunc("/admin/config", h.withAuth(h.adminAPIKey, h.handleAdminConfig))
	mux.HandleFunc("/admin/entitlement", h.withAuth(h.adminAPIKey, h.handleAdminEntitlement))
	mux.HandleFunc("/admin/invalidate", h.withAuth(h.adminAPIKey, h.handleAdminInvalidate))
	mux.HandleFunc("/metrics", h.handleMetrics)
}

// ── Health ───────────────────────────────────────────────────────────────────

// healthResponse is the JSON body returned by POST /health.
type healthResponse struct {
	Status    string `json:"status"`
	ServerID  string `json:"server_id"`
	Timestamp string `json:"timestamp"`
}

// handleHealth responds to POST /health with the server's liveness status.
// No authentication is required.
func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{
		Status:    "ok",
		ServerID:  h.serverID,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}

// ── Config ───────────────────────────────────────────────────────────────────

// configPollRequest is the JSON body for POST /config.
type configPollRequest struct {
	GatewayID      string `json:"gateway_id"`
	CurrentVersion string `json:"current_version"`
}

// handleConfig serves the daemon config-poll endpoint (POST /config).
// Auth: Bearer DAEMON_API_KEY.
// If the daemon's current_version matches the stored version, 304 is returned.
// Otherwise the full GatewayConfig JSON is returned with 200.
func (h *Handler) handleConfig(w http.ResponseWriter, r *http.Request) {
	if !h.checkBearer(r, h.daemonAPIKey) {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req configPollRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	cfg, err := h.cfgStore.GetCurrent(r.Context())
	if err != nil {
		if errors.Is(err, model.ErrNotFound) {
			writeError(w, http.StatusNotFound, "no config stored")
			return
		}
		h.log.Error().Err(err).Msg("handleConfig: get current config")
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// if daemon is already on the latest version, return 304 with no body.
	if req.CurrentVersion != "" && req.CurrentVersion == cfg.Version {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	writeJSON(w, http.StatusOK, cfg)
}

// ── Entitlement ──────────────────────────────────────────────────────────────

// entitlementRequest is the JSON body for POST /entitlement.
type entitlementRequest struct {
	UserID string `json:"user_id"`
	OrgID  string `json:"org_id"`
}

// handleEntitlement resolves a user's entitlement (POST /entitlement).
// Auth: Bearer DAEMON_API_KEY.
// Returns 200 + UserEntitlement JSON, or 404 if the user is not found.
func (h *Handler) handleEntitlement(w http.ResponseWriter, r *http.Request) {
	if !h.checkBearer(r, h.daemonAPIKey) {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req entitlementRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	ent, err := h.entService.Resolve(r.Context(), req.UserID, req.OrgID)
	if err != nil {
		if errors.Is(err, model.ErrNotFound) {
			writeError(w, http.StatusNotFound, "user_not_found")
			return
		}
		h.log.Error().Err(err).Str("user_id", req.UserID).Msg("handleEntitlement: resolve")
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, ent)
}

// ── Admin: Config ─────────────────────────────────────────────────────────────

// adminConfigResponse is the JSON body returned after a successful config store.
type adminConfigResponse struct {
	Version   string `json:"version"`
	AppliedAt string `json:"applied_at"`
}

// handleAdminConfig stores a new GatewayConfig (POST /admin/config).
// Auth: Bearer ADMIN_API_KEY.
// Returns {"version":"<sha256>","applied_at":"<iso8601>"} on success.
func (h *Handler) handleAdminConfig(w http.ResponseWriter, r *http.Request) {
	if !h.checkBearer(r, h.adminAPIKey) {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var cfg model.GatewayConfig
	if err := decodeJSON(r, &cfg); err != nil {
		writeError(w, http.StatusBadRequest, "invalid config body")
		return
	}

	version, err := h.cfgStore.Update(r.Context(), cfg)
	if err != nil {
		h.log.Error().Err(err).Msg("handleAdminConfig: update")
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, adminConfigResponse{
		Version:   version,
		AppliedAt: time.Now().UTC().Format(time.RFC3339),
	})
}

// ── Admin: Entitlement ────────────────────────────────────────────────────────

// adminEntitlementRequest is the JSON body for POST /admin/entitlement.
type adminEntitlementRequest struct {
	UserID  string                 `json:"user_id"`
	Changes map[string]interface{} `json:"changes"`
}

// adminEntitlementResponse is the JSON body returned after a successful entitlement update.
type adminEntitlementResponse struct {
	Updated    bool `json:"updated"`
	Invalidated bool `json:"invalidated"`
}

// handleAdminEntitlement updates a user's entitlement and publishes invalidation
// (POST /admin/entitlement). Auth: Bearer ADMIN_API_KEY.
// Returns {"updated":true,"invalidated":true} on success.
func (h *Handler) handleAdminEntitlement(w http.ResponseWriter, r *http.Request) {
	if !h.checkBearer(r, h.adminAPIKey) {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req adminEntitlementRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	err := h.entService.Update(r.Context(), req.UserID, req.Changes)
	if err != nil {
		if errors.Is(err, model.ErrNotFound) {
			writeError(w, http.StatusNotFound, "user_not_found")
			return
		}
		h.log.Error().Err(err).Str("user_id", req.UserID).Msg("handleAdminEntitlement: update")
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, adminEntitlementResponse{
		Updated:    true,
		Invalidated: true,
	})
}

// ── Admin: Invalidate ─────────────────────────────────────────────────────────

// adminInvalidateRequest is the JSON body for POST /admin/invalidate.
type adminInvalidateRequest struct {
	UserID string `json:"user_id"`
}

// adminInvalidateResponse is the JSON body returned after a successful invalidation publish.
type adminInvalidateResponse struct {
	Published bool   `json:"published"`
	Channel   string `json:"channel"`
}

// handleAdminInvalidate publishes a Redis invalidation for a specific user
// (POST /admin/invalidate). Auth: Bearer ADMIN_API_KEY.
// Returns {"published":true,"channel":"user:invalidate"}.
// If the publisher is nil (Redis not configured), returns {"published":false,"channel":"user:invalidate"}.
func (h *Handler) handleAdminInvalidate(w http.ResponseWriter, r *http.Request) {
	if !h.checkBearer(r, h.adminAPIKey) {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req adminInvalidateRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if h.pub == nil {
		h.log.Warn().Str("user_id", req.UserID).Msg("handleAdminInvalidate: publisher not configured")
		writeJSON(w, http.StatusOK, adminInvalidateResponse{
			Published: false,
			Channel:   "user:invalidate",
		})
		return
	}

	if err := h.pub.InvalidateUser(r.Context(), req.UserID); err != nil {
		h.log.Error().Err(err).Str("user_id", req.UserID).Msg("handleAdminInvalidate: publish")
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, adminInvalidateResponse{
		Published: true,
		Channel:   "user:invalidate",
	})
}

// ── Metrics ───────────────────────────────────────────────────────────────────

// handleMetrics returns an empty Prometheus-format response (SRS-NFR-018 stub).
// A full Prometheus integration can replace this stub without changing the route.
func (h *Handler) handleMetrics(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.WriteHeader(http.StatusOK)
}

// ── Middleware ────────────────────────────────────────────────────────────────

// withAuth wraps an http.HandlerFunc with Bearer token authentication.
// It extracts the "Authorization: Bearer <token>" header and performs a
// constant-time comparison against the expected key.
// Returns 401 Unauthorized if the token is missing or incorrect.
func (h *Handler) withAuth(expectedKey string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.checkBearer(r, expectedKey) {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next(w, r)
	}
}

// checkBearer extracts the Bearer token from the Authorization header and
// performs a constant-time comparison against expectedKey.
// Returns false if the header is missing, malformed, or the token does not match.
func (h *Handler) checkBearer(r *http.Request, expectedKey string) bool {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return false
	}

	const prefix = "Bearer "
	if !strings.HasPrefix(authHeader, prefix) {
		return false
	}

	token := authHeader[len(prefix):]

	// constant-time comparison prevents timing attacks.
	return subtle.ConstantTimeCompare([]byte(token), []byte(expectedKey)) == 1
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// errorResponse is the JSON body returned for error responses.
type errorResponse struct {
	Error string `json:"error"`
}

// writeJSON serialises v as JSON and writes it to w with the given status code.
// Sets Content-Type to application/json.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError writes a JSON error response body.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorResponse{Error: msg})
}

// decodeJSON reads and decodes the request body into v.
// Returns an error if the body is missing or not valid JSON.
func decodeJSON(r *http.Request, v interface{}) error {
	if r.Body == nil {
		return errors.New("empty request body")
	}
	return json.NewDecoder(r.Body).Decode(v)
}
