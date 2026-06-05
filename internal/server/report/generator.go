// Package report provides report generation for the Pangreksa AI Gateway console.
// The current implementation returns JSON; a Go PDF library (fpdf/gopdf) will be
// integrated once the D-03 library decision is finalised (see SRS §13.2).
package report

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

// ReportType identifies the kind of report to generate.
type ReportType string

const (
	// ReportUsageSummary aggregates request counts, token usage, and latency.
	ReportUsageSummary ReportType = "usage_summary"
	// ReportBudgetReport shows cost breakdowns and budget utilisation.
	ReportBudgetReport ReportType = "budget_report"
	// ReportGuardrailAudit lists guardrail events and violation counts.
	ReportGuardrailAudit ReportType = "guardrail_audit"
	// ReportRequestDetail exports individual request log rows.
	ReportRequestDetail ReportType = "request_detail"
	// ReportCostBreakdown shows per-model and per-provider cost attribution.
	ReportCostBreakdown ReportType = "cost_breakdown"
)

// validReportTypes is the set of accepted ReportType values.
var validReportTypes = map[ReportType]bool{
	ReportUsageSummary:   true,
	ReportBudgetReport:   true,
	ReportGuardrailAudit: true,
	ReportRequestDetail:  true,
	ReportCostBreakdown:  true,
}

const (
	// maxReportDays is the maximum allowed date range for a single report request.
	maxReportDays = 90
	// maxReportRows is the maximum number of rows fetched per report.
	maxReportRows = 100_000
)

// ReportParams defines the parameters for a report generation request.
type ReportParams struct {
	// Type is the kind of report to produce.
	Type ReportType `json:"type"`
	// From is the inclusive start of the report date range.
	From time.Time `json:"from"`
	// To is the inclusive end of the report date range.
	To time.Time `json:"to"`
	// Scope narrows the report to a specific dimension: org | user | provider | model.
	Scope string `json:"scope"`
	// ScopeID is the UUID or string value for the chosen Scope. May be empty.
	ScopeID string `json:"scope_id"`
	// OrgID is the organization the report is scoped to; injected from the JWT.
	OrgID string `json:"org_id"`
}

// reportRow is a single aggregated data row returned by a report query.
// Fields are populated based on the ReportType; unused fields are zero-valued.
type reportRow struct {
	RequestID     string    `json:"request_id,omitempty"`
	OrgID         string    `json:"org_id,omitempty"`
	UserID        string    `json:"user_id,omitempty"`
	Model         string    `json:"model,omitempty"`
	Provider      string    `json:"provider,omitempty"`
	Status        string    `json:"status,omitempty"`
	LatencyMS     int64     `json:"latency_ms,omitempty"`
	InputTokens   int64     `json:"input_tokens,omitempty"`
	OutputTokens  int64     `json:"output_tokens,omitempty"`
	CostUSD       float64   `json:"cost_usd,omitempty"`
	CacheStatus   string    `json:"cache_status,omitempty"`
	PromptFQN     string    `json:"prompt_fqn,omitempty"`
	GuardrailsHit []string  `json:"guardrails_hit,omitempty"`
	CreatedAt     time.Time `json:"created_at,omitempty"`
}

// reportEnvelope is the top-level JSON structure returned by GeneratePDF.
type reportEnvelope struct {
	Type      ReportType   `json:"type"`
	OrgID     string       `json:"org_id"`
	From      time.Time    `json:"from"`
	To        time.Time    `json:"to"`
	Scope     string       `json:"scope,omitempty"`
	ScopeID   string       `json:"scope_id,omitempty"`
	RowCount  int          `json:"row_count"`
	Rows      []reportRow  `json:"rows"`
	GeneratedAt time.Time  `json:"generated_at"`
}

// Generator produces report files from aggregated request_logs data.
//
// Thread-safety: safe for concurrent use; all state is read-only after construction.
type Generator struct {
	pool *pgxpool.Pool
	log  zerolog.Logger
}

// NewGenerator constructs a Generator backed by the given connection pool.
// pool must be non-nil.
func NewGenerator(pool *pgxpool.Pool, log zerolog.Logger) *Generator {
	return &Generator{
		pool: pool,
		log:  log.With().Str("component", "report.generator").Logger(),
	}
}

// GeneratePDF fetches report data from PostgreSQL and returns the serialised report.
//
// TODO (D-03): integrate a Go PDF library (fpdf / gopdf) once the library decision
// is finalised. The current implementation returns a JSON response to unblock
// frontend development. content_type is "application/json" until a PDF library
// is integrated.
//
// Returns (content_bytes, content_type, error).
func (g *Generator) GeneratePDF(ctx context.Context, params ReportParams) ([]byte, string, error) {
	// ── Validation ────────────────────────────────────────────────────────────

	if !validReportTypes[params.Type] {
		return nil, "", fmt.Errorf("Generator.GeneratePDF: invalid report type %q", params.Type)
	}
	if params.From.IsZero() || params.To.IsZero() {
		return nil, "", fmt.Errorf("Generator.GeneratePDF: from and to are required")
	}
	if params.To.Before(params.From) {
		return nil, "", fmt.Errorf("Generator.GeneratePDF: to must not be before from")
	}
	rangeDays := params.To.Sub(params.From).Hours() / 24
	if rangeDays > maxReportDays {
		return nil, "", fmt.Errorf("Generator.GeneratePDF: date range exceeds %d-day limit (got %.0f days)",
			maxReportDays, rangeDays)
	}
	if params.OrgID == "" {
		return nil, "", fmt.Errorf("Generator.GeneratePDF: org_id is required")
	}

	// ── Query ─────────────────────────────────────────────────────────────────

	rows, err := g.queryRows(ctx, params)
	if err != nil {
		return nil, "", fmt.Errorf("Generator.GeneratePDF: query: %w", err)
	}

	// ── Serialise ─────────────────────────────────────────────────────────────

	envelope := reportEnvelope{
		Type:        params.Type,
		OrgID:       params.OrgID,
		From:        params.From,
		To:          params.To,
		Scope:       params.Scope,
		ScopeID:     params.ScopeID,
		RowCount:    len(rows),
		Rows:        rows,
		GeneratedAt: time.Now().UTC(),
	}

	b, err := json.Marshal(envelope)
	if err != nil {
		return nil, "", fmt.Errorf("Generator.GeneratePDF: marshal: %w", err)
	}

	g.log.Info().
		Str("type", string(params.Type)).
		Str("org_id", params.OrgID).
		Int("rows", len(rows)).
		Msg("report generated")

	return b, "application/json", nil
}

// queryRows executes the appropriate SELECT against request_logs based on params.
// It enforces the maxReportRows limit via LIMIT in SQL.
func (g *Generator) queryRows(ctx context.Context, params ReportParams) ([]reportRow, error) {
	// Build the WHERE clause incrementally.
	// All queries are parameterised; no string concatenation of user data.
	args := []interface{}{
		params.OrgID,    // $1
		params.From,     // $2
		params.To,       // $3
		maxReportRows,   // $4  LIMIT
	}

	// Optional scope filter appended as $5.
	scopeFilter := ""
	if params.ScopeID != "" {
		switch params.Scope {
		case "user":
			scopeFilter = "AND user_id = $5"
			args = append(args, params.ScopeID)
		case "provider":
			scopeFilter = "AND provider = $5"
			args = append(args, params.ScopeID)
		case "model":
			scopeFilter = "AND model = $5"
			args = append(args, params.ScopeID)
		}
	}

	q := fmt.Sprintf(`
		SELECT id, org_id, user_id, model, provider, status,
		       latency_ms, input_tokens, output_tokens,
		       cost_usd, cache_status, prompt_fqn,
		       guardrails_hit, created_at
		FROM request_logs
		WHERE org_id = $1
		  AND created_at >= $2
		  AND created_at <= $3
		  %s
		ORDER BY created_at ASC
		LIMIT $4
	`, scopeFilter)

	dbRows, err := g.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("queryRows: query: %w", err)
	}
	defer dbRows.Close()

	var result []reportRow
	for dbRows.Next() {
		var row reportRow
		var guardrailsRaw []byte
		if err = dbRows.Scan(
			&row.RequestID, &row.OrgID, &row.UserID,
			&row.Model, &row.Provider, &row.Status,
			&row.LatencyMS, &row.InputTokens, &row.OutputTokens,
			&row.CostUSD, &row.CacheStatus, &row.PromptFQN,
			&guardrailsRaw, &row.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("queryRows: scan: %w", err)
		}
		if len(guardrailsRaw) > 0 {
			if err = json.Unmarshal(guardrailsRaw, &row.GuardrailsHit); err != nil {
				return nil, fmt.Errorf("queryRows: unmarshal guardrails_hit: %w", err)
			}
		}
		result = append(result, row)
	}
	if err = dbRows.Err(); err != nil {
		return nil, fmt.Errorf("queryRows: rows: %w", err)
	}

	if result == nil {
		result = []reportRow{}
	}
	return result, nil
}
