package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pangreksa/ai-gateway-engine/pkg/model"
)

// CostRecordRepository handles inserts into the cost_records table.
// Inserts are idempotent via ON CONFLICT (request_log_id) DO NOTHING.
//
// Thread-safety: safe for concurrent use; all operations use the shared Pool.
type CostRecordRepository struct {
	// Pool is the underlying pgx connection pool.
	Pool *pgxpool.Pool
}

// NewCostRecordRepository constructs a CostRecordRepository backed by the given DB.
func NewCostRecordRepository(db *DB) *CostRecordRepository {
	return &CostRecordRepository{Pool: db.Pool}
}

// Insert persists a cost record derived from a TransactionEvent.
//
// The insert is idempotent: if a row with the same request_log_id already exists
// the statement silently does nothing, satisfying at-least-once Kafka delivery.
//
// ctx controls deadline and cancellation for the database operation.
func (r *CostRecordRepository) Insert(ctx context.Context, event *model.TransactionEvent) error {
	// derive period fields from the event's CreatedAt string.
	// CreatedAt is in ISO 8601 format: "2026-06-04T12:00:00Z".
	createdAt, err := time.Parse(time.RFC3339, event.CreatedAt)
	if err != nil {
		// fallback to current time if parsing fails; the record should still be inserted.
		createdAt = time.Now().UTC()
	}

	// period_day is the calendar date, period_month is "YYYY-MM".
	periodDay := createdAt.Format("2006-01-02")
	periodMonth := createdAt.Format("2006-01")

	const q = `
		INSERT INTO cost_records (
			org_id, request_log_id, user_id, model,
			input_tokens, output_tokens, cost_usd,
			period_day, period_month
		) VALUES (
			$1, $2, $3, $4,
			$5, $6, $7,
			$8, $9
		)
		ON CONFLICT (request_log_id) DO NOTHING
	`

	_, err = r.Pool.Exec(ctx, q,
		event.OrgID,        // $1 org_id
		event.RequestID,    // $2 request_log_id  (same UUID as request_logs.id)
		event.UserID,       // $3 user_id
		event.Model,        // $4 model
		event.InputTokens,  // $5 input_tokens
		event.OutputTokens, // $6 output_tokens
		event.CostUSD,      // $7 cost_usd
		periodDay,          // $8 period_day
		periodMonth,        // $9 period_month
	)
	if err != nil {
		return fmt.Errorf("CostRecordRepository.Insert: exec: %w", err)
	}
	return nil
}
