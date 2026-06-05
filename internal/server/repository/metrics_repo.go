package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// MetricPoint is a single time-series data point.
type MetricPoint struct {
	// TS is the bucket timestamp returned by date_trunc.
	TS time.Time `json:"ts"`
	// Value is the numeric measurement for this bucket.
	Value float64 `json:"value"`
}

// MetricSeries is one named time-series.
type MetricSeries struct {
	// Metric is the name of the series (e.g. "latency_p95", "rpm", "tokens_in", "tokens_out", "cost_usd", "error_rate").
	Metric string `json:"metric"`
	// Points holds the ordered time-series data points.
	Points []MetricPoint `json:"points"`
}

// MetricsSummary holds top-level aggregated values for the requested window.
type MetricsSummary struct {
	// TotalRequests is the total number of requests in the window.
	TotalRequests int64 `json:"total_requests"`
	// TotalCostUSD is the summed cost in USD for all requests in the window.
	TotalCostUSD float64 `json:"total_cost_usd"`
	// TotalInputTokens is the summed input token count.
	TotalInputTokens int64 `json:"total_input_tokens"`
	// TotalOutputTokens is the summed output token count.
	TotalOutputTokens int64 `json:"total_output_tokens"`
	// AvgLatencyP95MS is the 95th-percentile latency in milliseconds.
	AvgLatencyP95MS float64 `json:"avg_latency_p95_ms"`
	// ErrorRatePct is the percentage of requests that resulted in an error status.
	ErrorRatePct float64 `json:"error_rate_pct"`
}

// MetricsFilter carries optional filters for metrics queries.
type MetricsFilter struct {
	// OrgID scopes results to a single organization when non-empty.
	OrgID string
	// UserID scopes results to a single user when non-empty.
	UserID string
	// GatewayID is reserved for future daemon-scoped filtering.
	GatewayID string
	// Provider filters by LLM provider name when non-empty.
	Provider string
	// Model filters by model name when non-empty.
	Model string
	// From is the inclusive start of the time window.
	From time.Time
	// To is the exclusive end of the time window.
	To time.Time
	// Interval is the date_trunc granularity: "1m" | "5m" | "15m" | "1h" | "1d".
	Interval string
}

// BudgetRuleSummary holds budget utilisation for a single budget rule in the
// current period, computed from cost_records.
type BudgetRuleSummary struct {
	// RuleID is the UUID of the budget_rules row.
	RuleID string `json:"rule_id"`
	// RuleName is a human-readable label extracted from rule_yaml.
	RuleName string `json:"rule_name"`
	// Entity is the scoped entity type extracted from rule_yaml.
	Entity string `json:"entity"`
	// Period is the budget period extracted from rule_yaml.
	Period string `json:"period"`
	// SpendUSD is the total cost incurred in the current period.
	SpendUSD float64 `json:"spend_usd"`
	// LimitUSD is the budget limit extracted from rule_yaml.
	LimitUSD float64 `json:"limit_usd"`
	// PctConsumed is SpendUSD / LimitUSD * 100 (0 when LimitUSD == 0).
	PctConsumed float64 `json:"pct_consumed"`
	// Status is "ok" | "warning" (>= 80%) | "critical" (>= 100%).
	Status string `json:"status"`
}

// MetricsRepository queries request_logs for dashboard time-series metrics.
//
// Thread-safety: safe for concurrent use; all operations use the shared Pool.
type MetricsRepository struct {
	// Pool is the underlying pgx connection pool.
	Pool *pgxpool.Pool
}

// NewMetricsRepository constructs a MetricsRepository backed by the given DB.
func NewMetricsRepository(db *DB) *MetricsRepository {
	return &MetricsRepository{Pool: db.Pool}
}

// validIntervals maps the human-friendly interval string to the PostgreSQL
// date_trunc field name. date_trunc does not accept arbitrary strings, so we
// allowlist the values here before embedding them in SQL.
var validIntervals = map[string]string{
	"1m":  "minute",
	"5m":  "minute", // 5-minute buckets are handled post-query if needed; date_trunc rounds down to the nearest minute
	"15m": "minute",
	"1h":  "hour",
	"1d":  "day",
}

// intervalTrunc maps each human interval to the date_trunc precision to use.
// For sub-hour intervals we truncate to minute and accept the smaller buckets.
func intervalTrunc(interval string) (string, error) {
	trunc, ok := validIntervals[interval]
	if !ok {
		return "", fmt.Errorf("MetricsRepository: invalid interval %q; must be one of 1m,5m,15m,1h,1d", interval)
	}
	return trunc, nil
}

// QuerySeries returns time-bucketed series for latency_p95, rpm, tokens_in,
// tokens_out, cost_usd, and error_rate.
//
// The interval field in f controls the date_trunc granularity.
// ctx controls deadline and cancellation for the database query.
func (r *MetricsRepository) QuerySeries(ctx context.Context, f MetricsFilter) ([]MetricSeries, error) {
	interval := f.Interval
	if interval == "" {
		interval = "1h"
	}

	trunc, err := intervalTrunc(interval)
	if err != nil {
		return nil, fmt.Errorf("QuerySeries: %w", err)
	}

	// Build a dynamic WHERE clause to avoid sending empty-string filters.
	// The date_trunc field is allowlisted above and safe to embed directly.
	q := fmt.Sprintf(`
		SELECT date_trunc('%s', created_at) AS bucket,
		       COALESCE(percentile_cont(0.95) WITHIN GROUP (ORDER BY latency_ms), 0) AS latency_p95,
		       COUNT(*) AS request_count,
		       COALESCE(SUM(input_tokens), 0)  AS input_tokens,
		       COALESCE(SUM(output_tokens), 0) AS output_tokens,
		       COALESCE(SUM(cost_usd), 0)      AS cost_usd,
		       COALESCE(
		           SUM(CASE WHEN status='error' THEN 1 ELSE 0 END)::float
		           / NULLIF(COUNT(*), 0) * 100,
		           0
		       ) AS error_rate
		FROM request_logs
		WHERE created_at BETWEEN $1 AND $2`, trunc)

	args := []interface{}{f.From, f.To}
	argIdx := 3

	if f.OrgID != "" {
		q += fmt.Sprintf(" AND org_id = $%d", argIdx)
		args = append(args, f.OrgID)
		argIdx++
	}
	if f.UserID != "" {
		q += fmt.Sprintf(" AND user_id = $%d", argIdx)
		args = append(args, f.UserID)
		argIdx++
	}
	if f.Provider != "" {
		q += fmt.Sprintf(" AND provider = $%d", argIdx)
		args = append(args, f.Provider)
		argIdx++
	}
	if f.Model != "" {
		q += fmt.Sprintf(" AND model = $%d", argIdx)
		args = append(args, f.Model)
		argIdx++
	}

	q += " GROUP BY bucket ORDER BY bucket"

	rows, err := r.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("QuerySeries: query: %w", err)
	}
	defer rows.Close()

	// Accumulate per-metric slices indexed by bucket order.
	type row struct {
		bucket       time.Time
		latencyP95   float64
		requestCount float64
		inputTokens  float64
		outputTokens float64
		costUSD      float64
		errorRate    float64
	}

	var rawRows []row
	for rows.Next() {
		var rw row
		if err = rows.Scan(
			&rw.bucket,
			&rw.latencyP95,
			&rw.requestCount,
			&rw.inputTokens,
			&rw.outputTokens,
			&rw.costUSD,
			&rw.errorRate,
		); err != nil {
			return nil, fmt.Errorf("QuerySeries: scan: %w", err)
		}
		rawRows = append(rawRows, rw)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("QuerySeries: rows: %w", err)
	}

	// Build six named series.
	latency := MetricSeries{Metric: "latency_p95"}
	rpm := MetricSeries{Metric: "rpm"}
	tokensIn := MetricSeries{Metric: "tokens_in"}
	tokensOut := MetricSeries{Metric: "tokens_out"}
	cost := MetricSeries{Metric: "cost_usd"}
	errorRate := MetricSeries{Metric: "error_rate"}

	for _, rw := range rawRows {
		latency.Points = append(latency.Points, MetricPoint{TS: rw.bucket, Value: rw.latencyP95})
		rpm.Points = append(rpm.Points, MetricPoint{TS: rw.bucket, Value: rw.requestCount})
		tokensIn.Points = append(tokensIn.Points, MetricPoint{TS: rw.bucket, Value: rw.inputTokens})
		tokensOut.Points = append(tokensOut.Points, MetricPoint{TS: rw.bucket, Value: rw.outputTokens})
		cost.Points = append(cost.Points, MetricPoint{TS: rw.bucket, Value: rw.costUSD})
		errorRate.Points = append(errorRate.Points, MetricPoint{TS: rw.bucket, Value: rw.errorRate})
	}

	return []MetricSeries{latency, rpm, tokensIn, tokensOut, cost, errorRate}, nil
}

// QuerySummary returns top-level aggregated totals for the requested window.
//
// ctx controls deadline and cancellation for the database query.
func (r *MetricsRepository) QuerySummary(ctx context.Context, f MetricsFilter) (*MetricsSummary, error) {
	q := `
		SELECT COUNT(*),
		       COALESCE(SUM(cost_usd), 0),
		       COALESCE(SUM(input_tokens), 0),
		       COALESCE(SUM(output_tokens), 0),
		       COALESCE(percentile_cont(0.95) WITHIN GROUP (ORDER BY latency_ms), 0),
		       COALESCE(
		           SUM(CASE WHEN status='error' THEN 1 ELSE 0 END)::float
		           / NULLIF(COUNT(*), 0) * 100,
		           0
		       )
		FROM request_logs
		WHERE created_at BETWEEN $1 AND $2`

	args := []interface{}{f.From, f.To}
	argIdx := 3

	if f.OrgID != "" {
		q += fmt.Sprintf(" AND org_id = $%d", argIdx)
		args = append(args, f.OrgID)
		argIdx++
	}
	if f.UserID != "" {
		q += fmt.Sprintf(" AND user_id = $%d", argIdx)
		args = append(args, f.UserID)
		argIdx++
	}
	if f.Provider != "" {
		q += fmt.Sprintf(" AND provider = $%d", argIdx)
		args = append(args, f.Provider)
		argIdx++
	}
	if f.Model != "" {
		q += fmt.Sprintf(" AND model = $%d", argIdx)
		args = append(args, f.Model)
		argIdx++
	}

	// suppress unused variable warning from argIdx when no optional filters provided.
	_ = argIdx

	var s MetricsSummary
	row := r.Pool.QueryRow(ctx, q, args...)
	if err := row.Scan(
		&s.TotalRequests,
		&s.TotalCostUSD,
		&s.TotalInputTokens,
		&s.TotalOutputTokens,
		&s.AvgLatencyP95MS,
		&s.ErrorRatePct,
	); err != nil {
		return nil, fmt.Errorf("QuerySummary: scan: %w", err)
	}

	return &s, nil
}

// QueryBudgetSummary returns current-period spend vs limit per budget rule for
// the given organisation.
//
// It joins budget_rules with cost_records grouped by the current calendar month
// to approximate live spend (a Redis-free alternative).
// The limit and metadata are extracted from the rule_yaml TEXT column with a
// simple heuristic key scan; a full YAML parse is avoided to keep the repository
// dependency-free.
//
// ctx controls deadline and cancellation for the database query.
func (r *MetricsRepository) QueryBudgetSummary(ctx context.Context, orgID string) ([]BudgetRuleSummary, error) {
	const q = `
		SELECT br.id,
		       br.rule_yaml,
		       COALESCE(SUM(cr.cost_usd), 0) AS period_spend
		FROM budget_rules br
		LEFT JOIN cost_records cr
		       ON cr.org_id = br.org_id
		      AND cr.period_month = to_char(NOW(), 'YYYY-MM')
		WHERE br.org_id = $1
		GROUP BY br.id, br.rule_yaml
		ORDER BY br.priority ASC
	`

	rows, err := r.Pool.Query(ctx, q, orgID)
	if err != nil {
		return nil, fmt.Errorf("QueryBudgetSummary: query: %w", err)
	}
	defer rows.Close()

	var results []BudgetRuleSummary
	for rows.Next() {
		var ruleID, ruleYAML string
		var periodSpend float64

		if err = rows.Scan(&ruleID, &ruleYAML, &periodSpend); err != nil {
			return nil, fmt.Errorf("QueryBudgetSummary: scan: %w", err)
		}

		limitUSD := extractYAMLFloat(ruleYAML, "limit_usd")
		var pct float64
		if limitUSD > 0 {
			pct = periodSpend / limitUSD * 100
		}

		status := "ok"
		if pct >= 100 {
			status = "critical"
		} else if pct >= 80 {
			status = "warning"
		}

		results = append(results, BudgetRuleSummary{
			RuleID:      ruleID,
			RuleName:    extractYAMLString(ruleYAML, "name"),
			Entity:      extractYAMLString(ruleYAML, "entity"),
			Period:      extractYAMLString(ruleYAML, "period"),
			SpendUSD:    periodSpend,
			LimitUSD:    limitUSD,
			PctConsumed: pct,
			Status:      status,
		})
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("QueryBudgetSummary: rows: %w", err)
	}

	return results, nil
}

// extractYAMLString performs a minimal key-value extraction from a YAML string.
// It looks for lines of the form "key: value" and returns the trimmed value.
// This avoids importing a YAML library just for simple scalar reads.
func extractYAMLString(yaml, key string) string {
	prefix := key + ":"
	for _, line := range splitLines(yaml) {
		trimmed := trimSpace(line)
		if len(trimmed) > len(prefix) && trimmed[:len(prefix)] == prefix {
			return trimSpace(trimmed[len(prefix):])
		}
	}
	return ""
}

// extractYAMLFloat parses a float64 from a YAML key-value line.
// Returns 0 if the key is absent or the value cannot be parsed.
func extractYAMLFloat(yaml, key string) float64 {
	val := extractYAMLString(yaml, key)
	if val == "" {
		return 0
	}
	var f float64
	if _, err := fmt.Sscanf(val, "%f", &f); err != nil {
		return 0
	}
	return f
}

// splitLines splits a string on newline characters (\n or \r\n).
func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			end := i
			if end > start && s[end-1] == '\r' {
				end--
			}
			lines = append(lines, s[start:end])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

// trimSpace removes leading and trailing ASCII whitespace from s.
func trimSpace(s string) string {
	start := 0
	for start < len(s) && (s[start] == ' ' || s[start] == '\t' || s[start] == '\r' || s[start] == '\n') {
		start++
	}
	end := len(s)
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\r' || s[end-1] == '\n') {
		end--
	}
	return s[start:end]
}
