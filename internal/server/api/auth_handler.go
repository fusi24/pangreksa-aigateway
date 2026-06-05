package api

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/rs/zerolog"

	"github.com/pangreksa/ai-gateway-engine/internal/server/auth"
	"github.com/pangreksa/ai-gateway-engine/internal/server/repository"
	"github.com/pangreksa/ai-gateway-engine/pkg/model"
)

// ConsoleAuthHandler implements the console authentication endpoints:
//
//	POST /auth/login        — password login, issues JWT + refresh token
//	POST /auth/refresh      — exchange refresh token for new JWT
//	POST /auth/logout       — revoke refresh token
//	POST /health/daemon     — daemon heartbeat registration (DAEMON_API_KEY auth)
//
// Thread-safety: safe for concurrent use; all state is read-only after construction.
type ConsoleAuthHandler struct {
	// userRepo provides access to the users table.
	userRepo *repository.UserRepository
	// entRepo provides access to user_entitlements; used to read the
	// permissions JSONB column and embed it in the issued JWT.
	entRepo *repository.EntitlementRepository
	// sessionRepo provides access to the console_sessions table.
	sessionRepo *repository.ConsoleSessionRepository
	// daemonRegistrationRepo provides access to the daemon_registrations table.
	daemonRegistrationRepo *repository.DaemonRegistrationRepository
	// jwtSvc signs and validates HS256 JWTs.
	jwtSvc *auth.JWTService
	// daemonAPIKey is the Bearer token required for POST /health/daemon.
	daemonAPIKey string
	// log is the structured logger for authentication events.
	log zerolog.Logger
}

// NewConsoleAuthHandler constructs a ConsoleAuthHandler with all required
// dependencies.
func NewConsoleAuthHandler(
	userRepo *repository.UserRepository,
	entRepo *repository.EntitlementRepository,
	sessionRepo *repository.ConsoleSessionRepository,
	daemonRegistrationRepo *repository.DaemonRegistrationRepository,
	jwtSvc *auth.JWTService,
	daemonAPIKey string,
	log zerolog.Logger,
) *ConsoleAuthHandler {
	return &ConsoleAuthHandler{
		userRepo:               userRepo,
		entRepo:                entRepo,
		sessionRepo:            sessionRepo,
		daemonRegistrationRepo: daemonRegistrationRepo,
		jwtSvc:                 jwtSvc,
		daemonAPIKey:           daemonAPIKey,
		log:                    log.With().Str("component", "api.auth_handler").Logger(),
	}
}

// Register mounts authentication routes onto mux.
// Route layout:
//
//	POST /auth/login      — password login (no prior auth required)
//	POST /auth/refresh    — refresh JWT using HttpOnly cookie
//	POST /auth/logout     — revoke session (cookie-based)
//	POST /health/daemon   — daemon heartbeat (DAEMON_API_KEY auth)
func (h *ConsoleAuthHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /auth/login", h.handleLogin)
	mux.HandleFunc("POST /auth/refresh", h.handleRefresh)
	mux.HandleFunc("POST /auth/logout", h.handleLogout)
	mux.HandleFunc("POST /health/daemon", h.handleDaemonHeartbeat)

	// Fallback patterns for older Go router implementations.
	mux.HandleFunc("/auth/login", h.handleLogin)
	mux.HandleFunc("/auth/refresh", h.handleRefresh)
	mux.HandleFunc("/auth/logout", h.handleLogout)
	mux.HandleFunc("/health/daemon", h.checkDaemonBearer(h.handleDaemonHeartbeat))
}

// ── Auth: Login ───────────────────────────────────────────────────────────────

// loginRequest is the JSON body for POST /auth/login.
type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// tokenResponse is the JSON body returned by POST /auth/login and POST /auth/refresh.
type tokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

// handleLogin authenticates a user by email and password.
//
// On success it:
//  1. Verifies credentials against the users table.
//  2. Issues a 15-minute HS256 JWT (access token).
//  3. Generates a cryptographically random refresh token (64-byte hex).
//  4. Stores the SHA-256 hash of the refresh token in console_sessions (7-day TTL).
//  5. Sets an HttpOnly Secure SameSite=Strict cookie with the raw refresh token.
//  6. Returns {"access_token":"...","token_type":"Bearer","expires_in":900}.
func (h *ConsoleAuthHandler) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "email and password are required")
		return
	}

	// Look up the user by email across all orgs.
	user, err := h.findUserByEmail(r.Context(), req.Email)
	if err != nil {
		if errors.Is(err, model.ErrNotFound) {
			// Return generic error to avoid email enumeration.
			writeError(w, http.StatusUnauthorized, "invalid credentials")
			return
		}
		h.log.Error().Err(err).Str("email", req.Email).Msg("handleLogin: find user")
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if user.Status != "active" {
		writeError(w, http.StatusUnauthorized, "account is not active")
		return
	}

	if user.PasswordHash == nil {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	if err = auth.CheckPassword(*user.PasswordHash, req.Password); err != nil {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	perms := h.loadPerms(r.Context(), user.ID)

	accessToken, err := h.jwtSvc.Sign(auth.Claims{
		UserID: user.ID,
		OrgID:  user.OrgID,
		Email:  user.Email,
		Perms:  perms,
	})
	if err != nil {
		h.log.Error().Err(err).Str("user_id", user.ID).Msg("handleLogin: sign jwt")
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	rawRefresh, err := auth.GenerateRefreshToken()
	if err != nil {
		h.log.Error().Err(err).Str("user_id", user.ID).Msg("handleLogin: generate refresh token")
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	tokenHash := hashToken(rawRefresh)

	session := &repository.ConsoleSession{
		UserID:           user.ID,
		RefreshTokenHash: tokenHash,
		IPAddress:        remoteIP(r),
		UserAgent:        r.UserAgent(),
		ExpiresAt:        time.Now().UTC().Add(7 * 24 * time.Hour),
	}
	if err = h.sessionRepo.Create(r.Context(), session); err != nil {
		h.log.Error().Err(err).Str("user_id", user.ID).Msg("handleLogin: create session")
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	setRefreshCookie(w, rawRefresh, 7*24*60*60)

	writeJSON(w, http.StatusOK, tokenResponse{
		AccessToken: accessToken,
		TokenType:   "Bearer",
		ExpiresIn:   900,
	})
}

// ── Auth: Refresh ─────────────────────────────────────────────────────────────

// handleRefresh exchanges a valid refresh-token cookie for a new JWT.
//
// It reads the pangreksa_refresh cookie, hashes the value, looks up the
// session in console_sessions (respecting expiry), then issues a new JWT.
// Returns 401 when the cookie is missing, the session is not found, or expired.
func (h *ConsoleAuthHandler) handleRefresh(w http.ResponseWriter, r *http.Request) {
	rawRefresh, err := refreshCookieValue(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "missing refresh token")
		return
	}

	tokenHash := hashToken(rawRefresh)

	session, err := h.sessionRepo.FindByTokenHash(r.Context(), tokenHash)
	if err != nil {
		if errors.Is(err, model.ErrNotFound) {
			writeError(w, http.StatusUnauthorized, "invalid or expired refresh token")
			return
		}
		h.log.Error().Err(err).Msg("handleRefresh: find session")
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	user, err := h.userRepo.FindByID(r.Context(), session.UserID)
	if err != nil {
		if errors.Is(err, model.ErrNotFound) {
			writeError(w, http.StatusUnauthorized, "user not found")
			return
		}
		h.log.Error().Err(err).Str("user_id", session.UserID).Msg("handleRefresh: find user")
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	perms := h.loadPerms(r.Context(), user.ID)

	accessToken, err := h.jwtSvc.Sign(auth.Claims{
		UserID: user.ID,
		OrgID:  user.OrgID,
		Email:  user.Email,
		Perms:  perms,
	})
	if err != nil {
		h.log.Error().Err(err).Str("user_id", user.ID).Msg("handleRefresh: sign jwt")
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, tokenResponse{
		AccessToken: accessToken,
		TokenType:   "Bearer",
		ExpiresIn:   900,
	})
}

// ── Auth: Logout ──────────────────────────────────────────────────────────────

// handleLogout revokes the caller's refresh token session.
//
// It reads the pangreksa_refresh cookie, hashes the value, finds and deletes
// the corresponding console_sessions row, then instructs the browser to clear
// the cookie by setting an expired Set-Cookie header. Returns 204 No Content.
func (h *ConsoleAuthHandler) handleLogout(w http.ResponseWriter, r *http.Request) {
	rawRefresh, err := refreshCookieValue(r)
	if err != nil {
		// No cookie present; nothing to revoke.
		w.WriteHeader(http.StatusNoContent)
		return
	}

	tokenHash := hashToken(rawRefresh)

	session, err := h.sessionRepo.FindByTokenHash(r.Context(), tokenHash)
	if err != nil {
		// Session not found or already expired; still clear the cookie.
		clearRefreshCookie(w)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if err = h.sessionRepo.Delete(r.Context(), session.ID); err != nil {
		h.log.Error().Err(err).Str("session_id", session.ID).Msg("handleLogout: delete session")
		// Best-effort: still clear the cookie.
	}

	clearRefreshCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

// ── Daemon Heartbeat ──────────────────────────────────────────────────────────

// daemonHeartbeatRequest is the JSON body for POST /health/daemon.
type daemonHeartbeatRequest struct {
	GatewayID     string         `json:"gateway_id"`
	Version       string         `json:"version"`
	ConfigVersion string         `json:"config_version"`
	UptimeSeconds int64          `json:"uptime_seconds"`
	ProviderPorts map[string]int `json:"provider_ports"`
	OtelEndpoint  string         `json:"otel_endpoint"`
	OrgID         string         `json:"org_id"`
}

// handleDaemonHeartbeat registers or updates a daemon heartbeat.
//
// This supplements the existing POST /health endpoint defined in handler.go.
// Daemon clients should send their extended heartbeat body to POST /health/daemon.
//
// Auth: Bearer DAEMON_API_KEY.
// Returns 200 {"status":"ok"} on success.
func (h *ConsoleAuthHandler) handleDaemonHeartbeat(w http.ResponseWriter, r *http.Request) {
	if !h.checkDaemonBearerInline(r) {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req daemonHeartbeatRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.GatewayID == "" {
		writeError(w, http.StatusBadRequest, "gateway_id is required")
		return
	}

	ports := req.ProviderPorts
	if ports == nil {
		ports = make(map[string]int)
	}

	reg := &repository.DaemonRegistration{
		GatewayID:     req.GatewayID,
		OrgID:         req.OrgID,
		Version:       req.Version,
		ConfigVersion: req.ConfigVersion,
		UptimeSeconds: req.UptimeSeconds,
		ProviderPorts: ports,
		OtelEndpoint:  req.OtelEndpoint,
	}

	if err := h.daemonRegistrationRepo.Upsert(r.Context(), reg); err != nil {
		h.log.Error().Err(err).Str("gateway_id", req.GatewayID).Msg("handleDaemonHeartbeat: upsert")
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// checkDaemonBearer is a middleware that enforces DAEMON_API_KEY authentication
// on the fallback (non-method-prefixed) route pattern.
func (h *ConsoleAuthHandler) checkDaemonBearer(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.checkDaemonBearerInline(r) {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next(w, r)
	}
}

// checkDaemonBearerInline performs a constant-time Bearer token check against
// the configured DAEMON_API_KEY.
func (h *ConsoleAuthHandler) checkDaemonBearerInline(r *http.Request) bool {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return false
	}
	const prefix = "Bearer "
	if !strings.HasPrefix(authHeader, prefix) {
		return false
	}
	token := authHeader[len(prefix):]
	return subtle.ConstantTimeCompare([]byte(token), []byte(h.daemonAPIKey)) == 1
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// findUserByEmail looks up a user by email address across all organizations.
// It queries the database directly to avoid requiring an orgID at login time.
// Returns model.ErrNotFound when no user with the given email exists.
func (h *ConsoleAuthHandler) findUserByEmail(ctx context.Context, email string) (*repository.User, error) {
	const q = `
		SELECT id, org_id, email, password_hash, provider, status,
		       mfa_enabled, failed_login_count, locked_until, created_at
		FROM users
		WHERE email = $1
		LIMIT 1
	`
	row := h.userRepo.Pool.QueryRow(ctx, q, email)

	var u repository.User
	err := row.Scan(
		&u.ID,
		&u.OrgID,
		&u.Email,
		&u.PasswordHash,
		&u.Provider,
		&u.Status,
		&u.MFAEnabled,
		&u.FailedLoginCount,
		&u.LockedUntil,
		&u.CreatedAt,
	)
	if err != nil {
		if err.Error() == "no rows in result set" {
			return nil, fmt.Errorf("findUserByEmail: %w", model.ErrNotFound)
		}
		// pgx.ErrNoRows string check as a fallback.
		return nil, fmt.Errorf("findUserByEmail: scan: %w", err)
	}
	return &u, nil
}

// loadPerms fetches the RBAC permission tokens from user_entitlements for the
// given userID and returns them as a string slice suitable for JWT embedding.
// On any error (missing entitlement row, DB failure) it logs a warning and
// returns an empty slice so login still succeeds — RBAC simply grants nothing.
func (h *ConsoleAuthHandler) loadPerms(ctx context.Context, userID string) []string {
	ent, err := h.entRepo.FindByUserID(ctx, userID)
	if err != nil {
		h.log.Warn().Err(err).Str("user_id", userID).
			Msg("loadPerms: could not load entitlement; JWT perms will be empty")
		return []string{}
	}
	if len(ent.Permissions) == 0 {
		return []string{}
	}
	return ent.Permissions
}

// hashToken returns the hex-encoded SHA-256 digest of the raw token string.
// This digest is stored in the database; the raw token is only kept in cookies.
func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// setRefreshCookie writes the pangreksa_refresh HttpOnly cookie to the response.
// maxAge is expressed in seconds (e.g. 604800 for 7 days).
func setRefreshCookie(w http.ResponseWriter, token string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     "pangreksa_refresh",
		Value:    token,
		Path:     "/auth/refresh",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})
}

// clearRefreshCookie instructs the browser to delete the pangreksa_refresh cookie
// by setting MaxAge=-1 and an empty value.
func clearRefreshCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     "pangreksa_refresh",
		Value:    "",
		Path:     "/auth/refresh",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})
}

// refreshCookieValue reads the pangreksa_refresh cookie from the request.
// Returns an error when the cookie is absent or has an empty value.
func refreshCookieValue(r *http.Request) (string, error) {
	cookie, err := r.Cookie("pangreksa_refresh")
	if err != nil {
		return "", fmt.Errorf("refreshCookieValue: %w", err)
	}
	if cookie.Value == "" {
		return "", fmt.Errorf("refreshCookieValue: empty cookie value")
	}
	return cookie.Value, nil
}

// remoteIP extracts the client IP from the request.
// It respects X-Real-IP and X-Forwarded-For headers before falling back to
// RemoteAddr.
func remoteIP(r *http.Request) string {
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		// X-Forwarded-For may be a comma-separated list; take the first entry.
		parts := strings.SplitN(fwd, ",", 2)
		return strings.TrimSpace(parts[0])
	}
	// RemoteAddr has the form "IP:port"; strip the port.
	addr := r.RemoteAddr
	if idx := strings.LastIndex(addr, ":"); idx != -1 {
		return addr[:idx]
	}
	return addr
}
