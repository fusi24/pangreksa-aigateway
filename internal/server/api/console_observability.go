package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog"

	"github.com/pangreksa/ai-gateway-engine/internal/server/repository"
	"github.com/pangreksa/ai-gateway-engine/internal/server/sse"
)

// ConsoleObservabilityHandler implements the console observability and monitoring endpoints.
// It serves the /admin/metrics, /admin/budget, /admin/pods, /admin/logs,
// /admin/traces, /admin/request/{id}, /admin/audit, and /sse/telemetry routes.
//
// Thread-safety: safe for concurrent use; all state is read-only after construction.
type ConsoleObservabilityHandler struct {
	// metricsRepo provides time-series metrics queries over request_logs.
	metricsRepo *repository.MetricsRepository
	// auditRepo provides paginated access to the audit_log table.
	auditRepo *repository.AuditRepository
	// daemonRepo provides daemon heartbeat registry queries.
	daemonRepo *repository.DaemonRegistrationRepository
	// logRepo is the request_logs repository; used for raw pool access.
	logRepo *repository.RequestLogRepository
	// broadcaster is the SSE event fan-out hub.
	broadcaster *sse.Broadcaster
	// adminAPIKey is the Bearer token required for all /admin/* endpoints.
	adminAPIKey string
	// jaegerURL is the base URL of the Jaeger UI, used to build trace deep-links.
	// Example: "http://192.168.32.161:16686"
	jaegerURL string
	// log is the structured logger for handler events.
	log zerolog.Logger
}

// NewConsoleObservabilityHandler constructs a ConsoleObservabilityHandler with
// all required dependencies injected.
func NewConsoleObservabilityHandler(
	metricsRepo *repository.MetricsRepository,
	auditRepo *repository.AuditRepository,
	daemonRepo *repository.DaemonRegistrationRepository,
	logRepo *repository.RequestLogRepository,
	broadcaster *sse.Broadcaster,
	adminAPIKey string,
	jaegerURL string,
	log zerolog.Logger,
) *ConsoleObservabilityHandler {
	return &ConsoleObservabilityHandler{
		metricsRepo: metricsRepo,
		auditRepo:   auditRepo,
		daemonRepo:  daemonRepo,
		logRepo:     logRepo,
		broadcaster: broadcaster,
		adminAPIKey: adminAPIKey,
		jaegerURL:   jaegerURL,
		log:         log.With().Str("component", "api.console_observability").Logger(),
	}
}

// Register mounts all observability routes onto mux.
// Route layout:
//
//	GET /admin/metrics          — aggregated time-series metrics
//	GET /admin/budget/summary   — per-rule budget utilisation
//	GET /admin/pods             — daemon pod health status
//	GET /admin/logs             — structured request log search
//	GET /admin/traces           — distributed trace index
//	GET /admin/request/{id}     — single request detail
//	GET /admin/audit            — audit log with keyset pagination
//	GET /sse/telemetry          — server-sent events stream
func (h *ConsoleObservabilityHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /admin/metrics", h.handleMetrics)
	mux.HandleFunc("GET /admin/budget/summary", h.handleBudgetSummary)
	mux.HandleFunc("GET /admin/pods", h.handlePods)
	mux.HandleFunc("GET /admin/logs", h.handleLogs)
	mux.HandleFunc("GET /admin/traces", h.handleTraces)
	mux.HandleFunc("GET /admin/request/", h.handleRequestDetail)
	mux.HandleFunc("GET /admin/audit", h.handleAudit)
	mux.HandleFunc("GET /sse/telemetry", h.handleSSETelemetry)
}

// ── CORS helpers ──────────────────────────────────────────────────────────────

// setCORSHeaders adds permissive CORS headers required by the Next.js BFF.
func setCORSHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
}

// ── Auth helper ───────────────────────────────────────────────────────────────

// checkBearerToken extracts and validates the Bearer token against the admin key.
// Returns false and writes a 401 response when authentication fails.
func (h *ConsoleObservabilityHandler) checkBearerToken(w http.ResponseWriter, r *http.Request) bool {
	const prefix = "Bearer "
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, prefix) || auth[len(prefix):] != h.adminAPIKey {
		setCORSHeaders(w)
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return false
	}
	return true
}

// ── GET /admin/metrics ────────────────────────────────────────────────────────

// metricsResponse is the JSON envelope for GET /admin/metrics.
type metricsResponse struct {
	Series  []repository.MetricSeries  `json:"series"`
	Summary *repository.MetricsSummary `json:"summary"`
}

// handleMetrics serves GET /admin/metrics.
// Required query params: from, to (ISO8601). Optional: interval, org_id, user_id,
// gateway_id, provider, model.
// Returns 400 if from/to are missing or the time range exceeds 90 days.
func (h *ConsoleObservabilityHandler) handleMetrics(w http.ResponseWriter, r *http.Request) {
	setCORSHeaders(w)
	if !h.checkBearerToken(w, r) {
		return
	}

	q := r.URL.Query()

	fromStr := q.Get("from")
	toStr := q.Get("to")
	if fromStr == "" || toStr == "" {
		writeError(w, http.StatusBadRequest, "from and to query parameters are required")
		return
	}

	from, err := time.Parse(time.RFC3339, fromStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid from: %v", err))
		return
	}
	to, err := time.Parse(time.RFC3339, toStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid to: %v", err))
		return
	}

	if to.Sub(from) > 90*24*time.Hour {
		writeError(w, http.StatusBadRequest, "time range must not exceed 90 days")
		return
	}

	interval := q.Get("interval")
	if interval == "" {
		interval = "1h"
	}

	f := repository.MetricsFilter{
		OrgID:     q.Get("org_id"),
		UserID:    q.Get("user_id"),
		GatewayID: q.Get("gateway_id"),
		Provider:  q.Get("provider"),
		Model:     q.Get("model"),
		From:      from,
		To:        to,
		Interval:  interval,
	}

	series, err := h.metricsRepo.QuerySeries(r.Context(), f)
	if err != nil {
		h.log.Error().Err(err).Msg("handleMetrics: QuerySeries")
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	summary, err := h.metricsRepo.QuerySummary(r.Context(), f)
	if err != nil {
		h.log.Error().Err(err).Msg("handleMetrics: QuerySummary")
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, metricsResponse{Series: series, Summary: summary})
}

// ── GET /admin/budget/summary ─────────────────────────────────────────────────

// budgetSummaryResponse is the JSON envelope for GET /admin/budget/summary.
type budgetSummaryResponse struct {
	Rules []repository.BudgetRuleSummary `json:"rules"`
}

// handleBudgetSummary serves GET /admin/budget/summary.
// Required query param: org_id.
func (h *ConsoleObservabilityHandler) handleBudgetSummary(w http.ResponseWriter, r *http.Request) {
	setCORSHeaders(w)
	if !h.checkBearerToken(w, r) {
		return
	}

	orgID := r.URL.Query().Get("org_id")
	if orgID == "" {
		writeError(w, http.StatusBadRequest, "org_id query parameter is required")
		return
	}

	rules, err := h.metricsRepo.QueryBudgetSummary(r.Context(), orgID)
	if err != nil {
		h.log.Error().Err(err).Str("org_id", orgID).Msg("handleBudgetSummary: QueryBudgetSummary")
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, budgetSummaryResponse{Rules: rules})
}

// ── GET /admin/pods ───────────────────────────────────────────────────────────

// podsResponse is the JSON envelope for GET /admin/pods.
type podsResponse struct {
	Pods     []repository.DaemonRegistration `json:"pods"`
	Total    int                             `json:"total"`
	Healthy  int                             `json:"healthy"`
	Degraded int                             `json:"degraded"`
	Dead     int                             `json:"dead"`
}

// handlePods serves GET /admin/pods.
// Optional query param: org_id.
func (h *ConsoleObservabilityHandler) handlePods(w http.ResponseWriter, r *http.Request) {
	setCORSHeaders(w)
	if !h.checkBearerToken(w, r) {
		return
	}

	orgID := r.URL.Query().Get("org_id")

	pods, err := h.daemonRepo.ListByOrg(r.Context(), orgID)
	if err != nil {
		h.log.Error().Err(err).Str("org_id", orgID).Msg("handlePods: ListByOrg")
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	resp := podsResponse{Pods: pods, Total: len(pods)}
	for _, p := range pods {
		switch p.Status {
		case "healthy":
			resp.Healthy++
		case "degraded":
			resp.Degraded++
		default:
			resp.Dead++
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// ── GET /admin/logs ───────────────────────────────────────────────────────────

// logEntry represents a request_log row shaped for log-viewer display.
type logEntry struct {
	ID         string    `json:"id"`
	OrgID      string    `json:"org_id"`
	TraceID    string    `json:"trace_id,omitempty"`
	UserID     string    `json:"user_id,omitempty"`
	Provider   string    `json:"provider"`
	Model      string    `json:"model"`
	Status     string    `json:"status"`
	LatencyMS  int       `json:"latency_ms"`
	CostUSD    float64   `json:"cost_usd"`
	GatewayID  string    `json:"gateway_id,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// logsResponse is the JSON envelope for GET /admin/logs.
type logsResponse struct {
	Logs         []logEntry `json:"logs"`
	NextCursor   string     `json:"next_cursor"`
	TotalMatched int        `json:"total_matched"`
}

// handleLogs serves GET /admin/logs.
// Required query params: from, to (ISO8601).
// Optional: q, level, gateway_id, trace_id, request_id, limit (default 100 max 1000), cursor.
func (h *ConsoleObservabilityHandler) handleLogs(w http.ResponseWriter, r *http.Request) {
	setCORSHeaders(w)
	if !h.checkBearerToken(w, r) {
		return
	}

	q := r.URL.Query()

	fromStr := q.Get("from")
	toStr := q.Get("to")
	if fromStr == "" || toStr == "" {
		writeError(w, http.StatusBadRequest, "from and to query parameters are required")
		return
	}

	from, err := time.Parse(time.RFC3339, fromStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid from: %v", err))
		return
	}
	to, err := time.Parse(time.RFC3339, toStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid to: %v", err))
		return
	}

	limit := 100
	if limStr := q.Get("limit"); limStr != "" {
		if lim, parseErr := strconv.Atoi(limStr); parseErr == nil && lim > 0 {
			limit = lim
		}
	}
	if limit > 1000 {
		limit = 1000
	}

	entries, nextCursor, total, err := h.queryLogs(r.Context(), logQuery{
		From:      from,
		To:        to,
		Q:         q.Get("q"),
		Level:     q.Get("level"),
		GatewayID: q.Get("gateway_id"),
		TraceID:   q.Get("trace_id"),
		RequestID: q.Get("request_id"),
		Limit:     limit,
		Cursor:    q.Get("cursor"),
	})
	if err != nil {
		h.log.Error().Err(err).Msg("handleLogs: queryLogs")
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, logsResponse{
		Logs:         entries,
		NextCursor:   nextCursor,
		TotalMatched: total,
	})
}

// logQuery carries the filters for queryLogs.
type logQuery struct {
	From      time.Time
	To        time.Time
	Q         string
	Level     string
	GatewayID string
	TraceID   string
	RequestID string
	Limit     int
	Cursor    string
}

// queryLogs queries request_logs and returns log entries with keyset pagination.
// The cursor encodes (created_at|id) identically to the audit cursor scheme.
func (h *ConsoleObservabilityHandler) queryLogs(ctx context.Context, lq logQuery) ([]logEntry, string, int, error) {
	var whereClauses []string
	var args []interface{}
	argIdx := 1

	whereClauses = append(whereClauses, fmt.Sprintf("created_at BETWEEN $%d AND $%d", argIdx, argIdx+1))
	args = append(args, lq.From, lq.To)
	argIdx += 2

	if lq.TraceID != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("trace_id = $%d", argIdx))
		args = append(args, lq.TraceID)
		argIdx++
	}
	if lq.RequestID != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("id::text = $%d", argIdx))
		args = append(args, lq.RequestID)
		argIdx++
	}
	if lq.Level != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, lq.Level)
		argIdx++
	}
	if lq.Q != "" {
		pattern := "%" + strings.ReplaceAll(lq.Q, "%", "\\%") + "%"
		whereClauses = append(whereClauses, fmt.Sprintf(
			"(model ILIKE $%d OR provider ILIKE $%d OR trace_id ILIKE $%d)",
			argIdx, argIdx, argIdx,
		))
		args = append(args, pattern)
		argIdx++
	}

	// Keyset pagination.
	if lq.Cursor != "" {
		cursorTS, cursorID, decErr := decodeCursorLocal(lq.Cursor)
		if decErr == nil && cursorID != "" {
			whereClauses = append(whereClauses, fmt.Sprintf(
				"(created_at, id::text) < ($%d, $%d)",
				argIdx, argIdx+1,
			))
			args = append(args, cursorTS, cursorID)
			argIdx += 2
		}
	}

	where := ""
	if len(whereClauses) > 0 {
		where = "WHERE " + strings.Join(whereClauses, " AND ")
	}

	sql := fmt.Sprintf(`
		SELECT id::text, org_id::text,
		       COALESCE(trace_id, '')   AS trace_id,
		       COALESCE(user_id::text, '') AS user_id,
		       provider, model, status,
		       COALESCE(latency_ms, 0) AS latency_ms,
		       COALESCE(cost_usd, 0)   AS cost_usd,
		       created_at
		FROM request_logs
		%s
		ORDER BY created_at DESC, id DESC
		LIMIT $%d
	`, where, argIdx)
	args = append(args, lq.Limit+1)

	rows, err := h.logRepo.Pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, "", 0, fmt.Errorf("queryLogs: query: %w", err)
	}
	defer rows.Close()

	var entries []logEntry
	for rows.Next() {
		var e logEntry
		if scanErr := rows.Scan(
			&e.ID,
			&e.OrgID,
			&e.TraceID,
			&e.UserID,
			&e.Provider,
			&e.Model,
			&e.Status,
			&e.LatencyMS,
			&e.CostUSD,
			&e.CreatedAt,
		); scanErr != nil {
			return nil, "", 0, fmt.Errorf("queryLogs: scan: %w", scanErr)
		}
		entries = append(entries, e)
	}
	if err = rows.Err(); err != nil {
		return nil, "", 0, fmt.Errorf("queryLogs: rows: %w", err)
	}

	var nextCursor string
	if len(entries) > lq.Limit {
		entries = entries[:lq.Limit]
		last := entries[len(entries)-1]
		nextCursor = encodeCursorLocal(last.CreatedAt, last.ID)
	}

	return entries, nextCursor, len(entries), nil
}

// ── GET /admin/traces ─────────────────────────────────────────────────────────

// traceEntry represents one row in the trace index response.
type traceEntry struct {
	RequestID string    `json:"request_id"`
	TraceID   string    `json:"trace_id"`
	Status    string    `json:"status"`
	LatencyMS int       `json:"latency_ms"`
	Provider  string    `json:"provider"`
	Model     string    `json:"model"`
	CreatedAt time.Time `json:"created_at"`
	JaegerURL string    `json:"jaeger_url,omitempty"`
}

// tracesResponse is the JSON envelope for GET /admin/traces.
type tracesResponse struct {
	Traces []traceEntry `json:"traces"`
}

// handleTraces serves GET /admin/traces.
// Required query params: from, to (ISO8601).
// Optional: request_id, trace_id, limit (default 50).
func (h *ConsoleObservabilityHandler) handleTraces(w http.ResponseWriter, r *http.Request) {
	setCORSHeaders(w)
	if !h.checkBearerToken(w, r) {
		return
	}

	q := r.URL.Query()

	fromStr := q.Get("from")
	toStr := q.Get("to")
	if fromStr == "" || toStr == "" {
		writeError(w, http.StatusBadRequest, "from and to query parameters are required")
		return
	}

	from, err := time.Parse(time.RFC3339, fromStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid from: %v", err))
		return
	}
	to, err := time.Parse(time.RFC3339, toStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid to: %v", err))
		return
	}

	limit := 50
	if limStr := q.Get("limit"); limStr != "" {
		if lim, parseErr := strconv.Atoi(limStr); parseErr == nil && lim > 0 {
			limit = lim
		}
	}

	var whereClauses []string
	var args []interface{}
	argIdx := 1

	whereClauses = append(whereClauses, fmt.Sprintf("created_at BETWEEN $%d AND $%d", argIdx, argIdx+1))
	args = append(args, from, to)
	argIdx += 2

	if reqID := q.Get("request_id"); reqID != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("id::text = $%d", argIdx))
		args = append(args, reqID)
		argIdx++
	}
	if traceID := q.Get("trace_id"); traceID != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("trace_id = $%d", argIdx))
		args = append(args, traceID)
		argIdx++
	}

	where := ""
	if len(whereClauses) > 0 {
		where = "WHERE " + strings.Join(whereClauses, " AND ")
	}

	sql := fmt.Sprintf(`
		SELECT id::text,
		       COALESCE(trace_id, '') AS trace_id,
		       status,
		       COALESCE(latency_ms, 0) AS latency_ms,
		       provider, model, created_at
		FROM request_logs
		%s
		ORDER BY created_at DESC
		LIMIT $%d
	`, where, argIdx)
	args = append(args, limit)

	rows, err := h.logRepo.Pool.Query(r.Context(), sql, args...)
	if err != nil {
		h.log.Error().Err(err).Msg("handleTraces: query")
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	defer rows.Close()

	var traces []traceEntry
	for rows.Next() {
		var e traceEntry
		if scanErr := rows.Scan(
			&e.RequestID,
			&e.TraceID,
			&e.Status,
			&e.LatencyMS,
			&e.Provider,
			&e.Model,
			&e.CreatedAt,
		); scanErr != nil {
			h.log.Error().Err(scanErr).Msg("handleTraces: scan")
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if e.TraceID != "" && h.jaegerURL != "" {
			e.JaegerURL = h.jaegerURL + "/trace/" + e.TraceID
		}
		traces = append(traces, e)
	}
	if err = rows.Err(); err != nil {
		h.log.Error().Err(err).Msg("handleTraces: rows")
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, tracesResponse{Traces: traces})
}

// ── GET /admin/request/{id} ───────────────────────────────────────────────────

// requestDetail is the combined view of a request_logs row joined with its cost_records row.
type requestDetail struct {
	ID           string          `json:"id"`
	OrgID        string          `json:"org_id"`
	TraceID      string          `json:"trace_id,omitempty"`
	SpanID       string          `json:"span_id,omitempty"`
	UserID       string          `json:"user_id,omitempty"`
	Model        string          `json:"model"`
	Provider     string          `json:"provider"`
	Status       string          `json:"status"`
	LatencyMS    int             `json:"latency_ms"`
	TTFTMS       int             `json:"ttft_ms"`
	InputTokens  int             `json:"input_tokens"`
	OutputTokens int             `json:"output_tokens"`
	CostUSD      float64         `json:"cost_usd"`
	CacheStatus  string          `json:"cache_status,omitempty"`
	PromptFQN    string          `json:"prompt_fqn,omitempty"`
	SkillsUsed   json.RawMessage `json:"skills_used,omitempty"`
	MCPsCalled   json.RawMessage `json:"mcps_called,omitempty"`
	GuardrailsHit json.RawMessage `json:"guardrails_hit,omitempty"`
	Metadata     json.RawMessage `json:"metadata,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
	// Cost record fields.
	CostRecordID  string  `json:"cost_record_id,omitempty"`
	PeriodDay     string  `json:"period_day,omitempty"`
	PeriodMonth   string  `json:"period_month,omitempty"`
}

// handleRequestDetail serves GET /admin/request/{id}.
// The id path segment is the request_logs UUID.
// Returns 404 if no matching row exists.
func (h *ConsoleObservabilityHandler) handleRequestDetail(w http.ResponseWriter, r *http.Request) {
	setCORSHeaders(w)
	if !h.checkBearerToken(w, r) {
		return
	}

	// Extract the last path segment as the request ID.
	path := strings.TrimPrefix(r.URL.Path, "/admin/request/")
	id := strings.TrimSuffix(path, "/")
	if id == "" {
		writeError(w, http.StatusBadRequest, "request id is required")
		return
	}

	const sql = `
		SELECT rl.id::text,
		       rl.org_id::text,
		       COALESCE(rl.trace_id, '')    AS trace_id,
		       COALESCE(rl.span_id, '')     AS span_id,
		       COALESCE(rl.user_id::text, '') AS user_id,
		       rl.model, rl.provider, rl.status,
		       COALESCE(rl.latency_ms, 0)  AS latency_ms,
		       COALESCE(rl.ttft_ms, 0)     AS ttft_ms,
		       rl.input_tokens, rl.output_tokens,
		       rl.cost_usd,
		       COALESCE(rl.cache_status, '') AS cache_status,
		       COALESCE(rl.prompt_fqn, '')  AS prompt_fqn,
		       rl.skills_used, rl.mcps_called, rl.guardrails_hit, rl.metadata,
		       rl.created_at,
		       COALESCE(cr.id::text, '')         AS cost_record_id,
		       COALESCE(cr.period_day::text, '')  AS period_day,
		       COALESCE(cr.period_month, '')       AS period_month
		FROM request_logs rl
		LEFT JOIN cost_records cr ON cr.request_log_id = rl.id
		WHERE rl.id::text = $1
		LIMIT 1
	`

	row := h.logRepo.Pool.QueryRow(r.Context(), sql, id)

	var d requestDetail
	var rawSkills, rawMCPs, rawGuardrails, rawMeta []byte

	err := row.Scan(
		&d.ID,
		&d.OrgID,
		&d.TraceID,
		&d.SpanID,
		&d.UserID,
		&d.Model,
		&d.Provider,
		&d.Status,
		&d.LatencyMS,
		&d.TTFTMS,
		&d.InputTokens,
		&d.OutputTokens,
		&d.CostUSD,
		&d.CacheStatus,
		&d.PromptFQN,
		&rawSkills,
		&rawMCPs,
		&rawGuardrails,
		&rawMeta,
		&d.CreatedAt,
		&d.CostRecordID,
		&d.PeriodDay,
		&d.PeriodMonth,
	)
	if err != nil {
		// pgx.ErrNoRows — treat as 404.
		if strings.Contains(err.Error(), "no rows") {
			writeError(w, http.StatusNotFound, "request not found")
			return
		}
		h.log.Error().Err(err).Str("id", id).Msg("handleRequestDetail: scan")
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if len(rawSkills) > 0 {
		d.SkillsUsed = json.RawMessage(rawSkills)
	}
	if len(rawMCPs) > 0 {
		d.MCPsCalled = json.RawMessage(rawMCPs)
	}
	if len(rawGuardrails) > 0 {
		d.GuardrailsHit = json.RawMessage(rawGuardrails)
	}
	if len(rawMeta) > 0 {
		d.Metadata = json.RawMessage(rawMeta)
	}

	writeJSON(w, http.StatusOK, d)
}

// ── GET /admin/audit ──────────────────────────────────────────────────────────

// auditResponse is the JSON envelope for GET /admin/audit.
type auditResponse struct {
	Entries    []repository.AuditLogEntry `json:"entries"`
	NextCursor string                     `json:"next_cursor"`
}

// handleAudit serves GET /admin/audit.
// Optional query params: event_type, resource_type, from, to, q, limit, cursor, org_id.
func (h *ConsoleObservabilityHandler) handleAudit(w http.ResponseWriter, r *http.Request) {
	setCORSHeaders(w)
	if !h.checkBearerToken(w, r) {
		return
	}

	q := r.URL.Query()

	limit := 100
	if limStr := q.Get("limit"); limStr != "" {
		if lim, parseErr := strconv.Atoi(limStr); parseErr == nil && lim > 0 {
			limit = lim
		}
	}

	aq := repository.AuditQuery{
		OrgID:        q.Get("org_id"),
		EventType:    q.Get("event_type"),
		ResourceType: q.Get("resource_type"),
		Q:            q.Get("q"),
		Limit:        limit,
		Cursor:       q.Get("cursor"),
	}

	if fromStr := q.Get("from"); fromStr != "" {
		if t, err := time.Parse(time.RFC3339, fromStr); err == nil {
			aq.From = t
		}
	}
	if toStr := q.Get("to"); toStr != "" {
		if t, err := time.Parse(time.RFC3339, toStr); err == nil {
			aq.To = t
		}
	}

	entries, nextCursor, err := h.auditRepo.Query(r.Context(), aq)
	if err != nil {
		h.log.Error().Err(err).Msg("handleAudit: Query")
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, auditResponse{Entries: entries, NextCursor: nextCursor})
}

// ── GET /sse/telemetry ────────────────────────────────────────────────────────

// handleSSETelemetry serves GET /sse/telemetry as a Server-Sent Events stream.
// Auth: Bearer ADMIN_API_KEY. Each connected client receives all events that
// are broadcast to the shared Broadcaster.
// A keepalive comment (": keepalive") is sent every 15 seconds to prevent
// proxy and load-balancer timeouts.
func (h *ConsoleObservabilityHandler) handleSSETelemetry(w http.ResponseWriter, r *http.Request) {
	setCORSHeaders(w)
	if !h.checkBearerToken(w, r) {
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ch := h.broadcaster.Subscribe()
	defer h.broadcaster.Unsubscribe(ch)

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			// Client disconnected.
			return

		case <-ticker.C:
			// Send keepalive comment so the connection is not severed by proxies.
			fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()

		case event, open := <-ch:
			if !open {
				// Channel was closed by Unsubscribe; exit cleanly.
				return
			}
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Type, event.Data)
			flusher.Flush()
		}
	}
}

// ── Cursor helpers (local copies to avoid cross-package import) ───────────────

// encodeCursorLocal encodes (ts, id) as a base64 opaque cursor for use in log
// pagination responses. Mirrors the scheme used in AuditRepository.
func encodeCursorLocal(ts time.Time, id string) string {
	raw := ts.UTC().Format(time.RFC3339Nano) + "|" + id
	// base64 via encoding/json is not available here; use a simple format that
	// the audit repo's base64 package would also decode correctly.
	// We import encoding/base64 indirectly through the repository package's
	// decodeCursor, but since we are in the api package we inline the encoding.
	return encodeBase64(raw)
}

// decodeCursorLocal decodes an opaque cursor produced by encodeCursorLocal.
func decodeCursorLocal(cursor string) (time.Time, string, error) {
	raw, err := decodeBase64(cursor)
	if err != nil {
		return time.Time{}, "", fmt.Errorf("decodeCursorLocal: %w", err)
	}
	parts := strings.SplitN(raw, "|", 2)
	if len(parts) != 2 {
		return time.Time{}, "", fmt.Errorf("decodeCursorLocal: invalid format")
	}
	ts, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return time.Time{}, "", fmt.Errorf("decodeCursorLocal: parse ts: %w", err)
	}
	return ts, parts[1], nil
}

// encodeBase64 returns the standard base64 encoding of s.
func encodeBase64(s string) string {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	src := []byte(s)
	encoded := make([]byte, (len(src)+2)/3*4)
	j := 0
	for i := 0; i < len(src); i += 3 {
		b0 := src[i]
		var b1, b2 byte
		if i+1 < len(src) {
			b1 = src[i+1]
		}
		if i+2 < len(src) {
			b2 = src[i+2]
		}
		encoded[j] = alphabet[b0>>2]
		encoded[j+1] = alphabet[((b0&0x3)<<4)|(b1>>4)]
		if i+1 < len(src) {
			encoded[j+2] = alphabet[((b1&0xF)<<2)|(b2>>6)]
		} else {
			encoded[j+2] = '='
		}
		if i+2 < len(src) {
			encoded[j+3] = alphabet[b2&0x3F]
		} else {
			encoded[j+3] = '='
		}
		j += 4
	}
	return string(encoded)
}

// decodeBase64 decodes a standard base64-encoded string.
func decodeBase64(s string) (string, error) {
	lookup := [256]int{}
	for i := range lookup {
		lookup[i] = -1
	}
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	for i, c := range alphabet {
		lookup[c] = i
	}

	src := []byte(s)
	// strip trailing padding.
	for len(src) > 0 && src[len(src)-1] == '=' {
		src = src[:len(src)-1]
	}

	dst := make([]byte, 0, len(src)*3/4+1)
	var buf uint32
	bits := 0
	for _, c := range src {
		v := lookup[c]
		if v < 0 {
			return "", fmt.Errorf("decodeBase64: invalid character %q", c)
		}
		buf = (buf << 6) | uint32(v)
		bits += 6
		if bits >= 8 {
			bits -= 8
			dst = append(dst, byte(buf>>uint(bits)))
			buf &= (1 << uint(bits)) - 1
		}
	}
	return string(dst), nil
}
