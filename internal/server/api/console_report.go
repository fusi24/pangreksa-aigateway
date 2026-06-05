package api

import (
	"crypto/subtle"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/rs/zerolog"

	"github.com/pangreksa/ai-gateway-engine/internal/server/report"
)

// ConsoleReportHandler implements the server-side report generation endpoint for
// the Pangreksa AI Gateway console.
//
// Thread-safety: safe for concurrent use; all state is read-only after construction.
type ConsoleReportHandler struct {
	generator   *report.Generator
	adminAPIKey string
	log         zerolog.Logger
}

// NewConsoleReportHandler constructs a ConsoleReportHandler.
// generator and adminAPIKey must be non-nil/non-empty.
func NewConsoleReportHandler(
	generator *report.Generator,
	adminAPIKey string,
	log zerolog.Logger,
) *ConsoleReportHandler {
	return &ConsoleReportHandler{
		generator:   generator,
		adminAPIKey: adminAPIKey,
		log:         log.With().Str("component", "api.console_report").Logger(),
	}
}

// Register mounts the report route onto mux.
//
// Route layout:
//
//	POST /admin/reports/pdf — generate a report and return bytes (admin auth)
func (h *ConsoleReportHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /admin/reports/pdf", h.handleGenerateReport)
}

// reportRequest is the JSON request body for POST /admin/reports/pdf.
type reportRequest struct {
	Type    report.ReportType `json:"type"`
	From    string            `json:"from"`
	To      string            `json:"to"`
	Scope   string            `json:"scope"`
	ScopeID string            `json:"scope_id"`
	OrgID   string            `json:"org_id"`
}

// validReportTypes mirrors the constants in the report package for validation here.
var consoleValidReportTypes = map[report.ReportType]bool{
	report.ReportUsageSummary:   true,
	report.ReportBudgetReport:   true,
	report.ReportGuardrailAudit: true,
	report.ReportRequestDetail:  true,
	report.ReportCostBreakdown:  true,
}

// handleGenerateReport handles POST /admin/reports/pdf.
// Auth: Bearer ADMIN_API_KEY.
// Body: {"type":"usage_summary","from":"ISO8601","to":"ISO8601","scope":"org","scope_id":"uuid","org_id":"uuid"}
// Response: binary report bytes with Content-Disposition attachment header.
func (h *ConsoleReportHandler) handleGenerateReport(w http.ResponseWriter, r *http.Request) {
	if !h.checkBearer(r) {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req reportRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// ── Validate type ─────────────────────────────────────────────────────────
	if !consoleValidReportTypes[req.Type] {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid report type %q; valid values: usage_summary, budget_report, guardrail_audit, request_detail, cost_breakdown", req.Type))
		return
	}

	// ── Parse date range ──────────────────────────────────────────────────────
	if req.From == "" || req.To == "" {
		writeError(w, http.StatusBadRequest, "from and to are required")
		return
	}

	from, err := time.Parse(time.RFC3339, req.From)
	if err != nil {
		writeError(w, http.StatusBadRequest, "from must be ISO 8601 (RFC 3339) format")
		return
	}
	to, err := time.Parse(time.RFC3339, req.To)
	if err != nil {
		writeError(w, http.StatusBadRequest, "to must be ISO 8601 (RFC 3339) format")
		return
	}
	if to.Before(from) {
		writeError(w, http.StatusBadRequest, "to must not be before from")
		return
	}
	rangeDays := to.Sub(from).Hours() / 24
	if rangeDays > 90 {
		writeError(w, http.StatusBadRequest, "date range must not exceed 90 days")
		return
	}

	// ── Build params ──────────────────────────────────────────────────────────
	params := report.ReportParams{
		Type:    req.Type,
		From:    from,
		To:      to,
		Scope:   req.Scope,
		ScopeID: req.ScopeID,
		OrgID:   req.OrgID,
	}

	// ── Generate ──────────────────────────────────────────────────────────────
	data, contentType, err := h.generator.GeneratePDF(r.Context(), params)
	if err != nil {
		if isValidationError(err) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		h.log.Error().Err(err).Str("type", string(req.Type)).Msg("handleGenerateReport")
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// ── Stream response ───────────────────────────────────────────────────────
	date := time.Now().UTC().Format("2006-01-02")
	ext := "json"
	if contentType == "application/pdf" {
		ext = "pdf"
	}
	filename := fmt.Sprintf("pangreksa-report-%s-%s.%s", req.Type, date, ext)

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// checkBearer performs constant-time comparison of the Authorization Bearer token.
func (h *ConsoleReportHandler) checkBearer(r *http.Request) bool {
	auth := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if !strings.HasPrefix(auth, prefix) {
		return false
	}
	token := auth[len(prefix):]
	return subtle.ConstantTimeCompare([]byte(token), []byte(h.adminAPIKey)) == 1
}

// isValidationError returns true if the error message originates from a
// parameter validation check in the generator, indicating a 400 response is
// more appropriate than a 500.
func isValidationError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "invalid report type") ||
		strings.Contains(msg, "from and to are required") ||
		strings.Contains(msg, "to must not be before from") ||
		strings.Contains(msg, "date range exceeds") ||
		strings.Contains(msg, "org_id is required")
}
