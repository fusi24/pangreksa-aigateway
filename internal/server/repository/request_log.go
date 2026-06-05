package repository

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pangreksa/ai-gateway-engine/pkg/model"
)

// RequestLogRepository handles inserts into the request_logs partitioned table.
// Inserts are idempotent via ON CONFLICT (id, created_at) DO NOTHING so
// duplicate Kafka deliveries are handled safely.
//
// Thread-safety: safe for concurrent use; all operations use the shared Pool.
type RequestLogRepository struct {
	// Pool is the underlying pgx connection pool.
	Pool *pgxpool.Pool
}

// NewRequestLogRepository constructs a RequestLogRepository backed by the given DB.
func NewRequestLogRepository(db *DB) *RequestLogRepository {
	return &RequestLogRepository{Pool: db.Pool}
}

// Insert persists a TransactionEvent as a request_log row.
//
// The insert is idempotent: if a row with the same (id, created_at) already
// exists the statement silently does nothing, satisfying at-least-once delivery
// from Kafka without risking duplicate rows.
//
// ctx controls deadline and cancellation for the database operation.
func (r *RequestLogRepository) Insert(ctx context.Context, event *model.TransactionEvent) error {
	// marshal JSONB slice columns.
	skillsJSON, err := json.Marshal(event.SkillsUsed)
	if err != nil {
		return fmt.Errorf("RequestLogRepository.Insert: marshal skills_used: %w", err)
	}
	mcpsJSON, err := json.Marshal(event.MCPsCalled)
	if err != nil {
		return fmt.Errorf("RequestLogRepository.Insert: marshal mcps_called: %w", err)
	}
	guardrailsJSON, err := json.Marshal(event.GuardrailsHit)
	if err != nil {
		return fmt.Errorf("RequestLogRepository.Insert: marshal guardrails_hit: %w", err)
	}
	metaJSON, err := json.Marshal(event.Metadata)
	if err != nil {
		return fmt.Errorf("RequestLogRepository.Insert: marshal metadata: %w", err)
	}

	const q = `
		INSERT INTO request_logs (
			id, org_id, user_id, model, provider, status,
			latency_ms, ttft_ms, input_tokens, output_tokens,
			cost_usd, cache_status, prompt_fqn,
			skills_used, mcps_called, guardrails_hit, metadata,
			created_at
		) VALUES (
			$1,  $2,  $3,  $4,  $5,  $6,
			$7,  $8,  $9,  $10,
			$11, $12, $13,
			$14, $15, $16, $17,
			$18
		)
		ON CONFLICT (id, created_at) DO NOTHING
	`

	_, err = r.Pool.Exec(ctx, q,
		event.RequestID,   // $1  id
		event.OrgID,       // $2  org_id
		event.UserID,      // $3  user_id
		event.Model,       // $4  model
		event.Provider,    // $5  provider
		event.Status,      // $6  status
		event.LatencyMS,   // $7  latency_ms
		event.TTFTMS,      // $8  ttft_ms
		event.InputTokens, // $9  input_tokens
		event.OutputTokens, // $10 output_tokens
		event.CostUSD,     // $11 cost_usd
		event.CacheStatus, // $12 cache_status
		event.PromptFQN,   // $13 prompt_fqn
		skillsJSON,        // $14 skills_used
		mcpsJSON,          // $15 mcps_called
		guardrailsJSON,    // $16 guardrails_hit
		metaJSON,          // $17 metadata
		event.CreatedAt,   // $18 created_at
	)
	if err != nil {
		return fmt.Errorf("RequestLogRepository.Insert: exec: %w", err)
	}
	return nil
}
