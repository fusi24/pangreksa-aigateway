package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pangreksa/ai-gateway-engine/pkg/model"
)

// GuardrailRecord is a row from the guardrail_policies table.
// Each record defines a single guardrail policy applied to LLM interactions.
type GuardrailRecord struct {
	// ID is the UUID primary key.
	ID string `json:"id"`
	// OrgID is the UUID of the owning organization.
	OrgID string `json:"org_id"`
	// Type identifies the guardrail category, e.g. "content_filter" | "pii_redaction".
	Type string `json:"type"`
	// Hook indicates when the guardrail fires: "pre_request" | "post_response".
	Hook string `json:"hook"`
	// Mode controls the guardrail's operating mode: "active" | "audit".
	Mode string `json:"mode"`
	// Enforcement defines how violations are handled: "block" | "warn" | "log".
	Enforcement string `json:"enforcement"`
	// Config is the JSONB configuration blob specific to the guardrail type.
	Config json.RawMessage `json:"config"`
	// CreatedAt is the timestamp when the record was created.
	CreatedAt time.Time `json:"created_at"`
}

// GuardrailRepository handles all CRUD data access for the guardrail_policies table.
//
// Thread-safety: safe for concurrent use; all operations use the shared Pool.
type GuardrailRepository struct {
	// Pool is the underlying pgx connection pool.
	Pool *pgxpool.Pool
}

// NewGuardrailRepository constructs a GuardrailRepository backed by the given DB.
func NewGuardrailRepository(db *DB) *GuardrailRepository {
	return &GuardrailRepository{Pool: db.Pool}
}

// ListByOrg returns all GuardrailRecord rows for the given organization, ordered by type.
//
// ctx controls deadline and cancellation for the database query.
func (r *GuardrailRepository) ListByOrg(ctx context.Context, orgID string) ([]GuardrailRecord, error) {
	const q = `
		SELECT id, org_id, type, hook, mode, enforcement, config, created_at
		FROM guardrail_policies
		WHERE org_id = $1
		ORDER BY type
	`

	rows, err := r.Pool.Query(ctx, q, orgID)
	if err != nil {
		return nil, fmt.Errorf("GuardrailRepository.ListByOrg: query: %w", err)
	}
	defer rows.Close()

	var guardrails []GuardrailRecord
	for rows.Next() {
		g, err := scanGuardrail(rows)
		if err != nil {
			return nil, fmt.Errorf("GuardrailRepository.ListByOrg: scan: %w", err)
		}
		guardrails = append(guardrails, *g)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("GuardrailRepository.ListByOrg: rows: %w", err)
	}

	if guardrails == nil {
		guardrails = []GuardrailRecord{}
	}
	return guardrails, nil
}

// FindByID retrieves a single GuardrailRecord by its UUID.
//
// ctx controls deadline and cancellation for the database query.
// Returns model.ErrNotFound if no record exists with the given id.
func (r *GuardrailRepository) FindByID(ctx context.Context, id string) (*GuardrailRecord, error) {
	const q = `
		SELECT id, org_id, type, hook, mode, enforcement, config, created_at
		FROM guardrail_policies
		WHERE id = $1
	`

	row := r.Pool.QueryRow(ctx, q, id)
	g, err := scanGuardrail(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("GuardrailRepository.FindByID: %w", model.ErrNotFound)
		}
		return nil, fmt.Errorf("GuardrailRepository.FindByID: scan: %w", err)
	}
	return g, nil
}

// Create inserts a new GuardrailRecord and returns the persisted row with its
// generated ID and CreatedAt timestamp.
//
// ctx controls deadline and cancellation for the database operation.
func (r *GuardrailRepository) Create(ctx context.Context, g *GuardrailRecord) (*GuardrailRecord, error) {
	g.ID = uuid.New().String()

	const q = `
		INSERT INTO guardrail_policies (id, org_id, type, hook, mode, enforcement, config, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
		RETURNING id, org_id, type, hook, mode, enforcement, config, created_at
	`

	row := r.Pool.QueryRow(ctx, q,
		g.ID, g.OrgID, g.Type, g.Hook, g.Mode, g.Enforcement, []byte(g.Config),
	)
	created, err := scanGuardrail(row)
	if err != nil {
		return nil, fmt.Errorf("GuardrailRepository.Create: scan: %w", err)
	}
	return created, nil
}

// Update replaces the mutable fields of the GuardrailRecord identified by id.
// Returns the updated record.
//
// ctx controls deadline and cancellation for the database operation.
// Returns model.ErrNotFound if no record exists with the given id.
func (r *GuardrailRepository) Update(ctx context.Context, id string, g *GuardrailRecord) (*GuardrailRecord, error) {
	const q = `
		UPDATE guardrail_policies
		SET type = $2, hook = $3, mode = $4, enforcement = $5, config = $6
		WHERE id = $1
		RETURNING id, org_id, type, hook, mode, enforcement, config, created_at
	`

	row := r.Pool.QueryRow(ctx, q,
		id, g.Type, g.Hook, g.Mode, g.Enforcement, []byte(g.Config),
	)
	updated, err := scanGuardrail(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("GuardrailRepository.Update: %w", model.ErrNotFound)
		}
		return nil, fmt.Errorf("GuardrailRepository.Update: scan: %w", err)
	}
	return updated, nil
}

// Delete removes the GuardrailRecord identified by id.
//
// ctx controls deadline and cancellation for the database operation.
// Returns model.ErrNotFound if no record exists with the given id.
func (r *GuardrailRepository) Delete(ctx context.Context, id string) error {
	const q = `DELETE FROM guardrail_policies WHERE id = $1`

	tag, err := r.Pool.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("GuardrailRepository.Delete: exec: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("GuardrailRepository.Delete: %w", model.ErrNotFound)
	}
	return nil
}

// scanGuardrail maps a pgx row into a GuardrailRecord struct.
func scanGuardrail(row pgx.Row) (*GuardrailRecord, error) {
	var g GuardrailRecord
	var configRaw []byte
	err := row.Scan(
		&g.ID, &g.OrgID, &g.Type, &g.Hook, &g.Mode, &g.Enforcement,
		&configRaw, &g.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	if len(configRaw) > 0 {
		g.Config = json.RawMessage(configRaw)
	}
	return &g, nil
}
