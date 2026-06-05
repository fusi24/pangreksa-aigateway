package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pangreksa/ai-gateway-engine/pkg/model"
)

// RateLimitRuleRecord is a row from the rate_limit_rules table.
// Each record defines a prioritised YAML-encoded rate limit rule for an org.
type RateLimitRuleRecord struct {
	// ID is the UUID primary key.
	ID string `json:"id"`
	// OrgID is the UUID of the owning organization.
	OrgID string `json:"org_id"`
	// Priority determines evaluation order; lower numbers are evaluated first.
	Priority int `json:"priority"`
	// RuleYAML is the YAML-encoded rule definition.
	RuleYAML string `json:"rule_yaml"`
	// CreatedAt is the timestamp when the record was created.
	CreatedAt time.Time `json:"created_at"`
}

// RateLimitRuleRepository handles all CRUD data access for the rate_limit_rules table.
//
// Thread-safety: safe for concurrent use; all operations use the shared Pool.
type RateLimitRuleRepository struct {
	// Pool is the underlying pgx connection pool.
	Pool *pgxpool.Pool
}

// NewRateLimitRuleRepository constructs a RateLimitRuleRepository backed by the given DB.
func NewRateLimitRuleRepository(db *DB) *RateLimitRuleRepository {
	return &RateLimitRuleRepository{Pool: db.Pool}
}

// ListByOrg returns all RateLimitRuleRecord rows for the given organization,
// ordered by priority ascending.
//
// ctx controls deadline and cancellation for the database query.
func (r *RateLimitRuleRepository) ListByOrg(ctx context.Context, orgID string) ([]RateLimitRuleRecord, error) {
	const q = `
		SELECT id, org_id, priority, rule_yaml, created_at
		FROM rate_limit_rules
		WHERE org_id = $1
		ORDER BY priority ASC
	`

	rows, err := r.Pool.Query(ctx, q, orgID)
	if err != nil {
		return nil, fmt.Errorf("RateLimitRuleRepository.ListByOrg: query: %w", err)
	}
	defer rows.Close()

	var rules []RateLimitRuleRecord
	for rows.Next() {
		var rr RateLimitRuleRecord
		if err = rows.Scan(&rr.ID, &rr.OrgID, &rr.Priority, &rr.RuleYAML, &rr.CreatedAt); err != nil {
			return nil, fmt.Errorf("RateLimitRuleRepository.ListByOrg: scan: %w", err)
		}
		rules = append(rules, rr)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("RateLimitRuleRepository.ListByOrg: rows: %w", err)
	}

	if rules == nil {
		rules = []RateLimitRuleRecord{}
	}
	return rules, nil
}

// FindByID retrieves a single RateLimitRuleRecord by its UUID.
//
// ctx controls deadline and cancellation for the database query.
// Returns model.ErrNotFound if no record exists with the given id.
func (r *RateLimitRuleRepository) FindByID(ctx context.Context, id string) (*RateLimitRuleRecord, error) {
	const q = `
		SELECT id, org_id, priority, rule_yaml, created_at
		FROM rate_limit_rules
		WHERE id = $1
	`

	var rr RateLimitRuleRecord
	err := r.Pool.QueryRow(ctx, q, id).Scan(&rr.ID, &rr.OrgID, &rr.Priority, &rr.RuleYAML, &rr.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("RateLimitRuleRepository.FindByID: %w", model.ErrNotFound)
		}
		return nil, fmt.Errorf("RateLimitRuleRepository.FindByID: scan: %w", err)
	}
	return &rr, nil
}

// Create inserts a new RateLimitRuleRecord and returns the persisted row.
//
// ctx controls deadline and cancellation for the database operation.
func (r *RateLimitRuleRepository) Create(ctx context.Context, rule *RateLimitRuleRecord) (*RateLimitRuleRecord, error) {
	rule.ID = uuid.New().String()

	const q = `
		INSERT INTO rate_limit_rules (id, org_id, priority, rule_yaml, created_at)
		VALUES ($1, $2, $3, $4, NOW())
		RETURNING id, org_id, priority, rule_yaml, created_at
	`

	var rr RateLimitRuleRecord
	err := r.Pool.QueryRow(ctx, q, rule.ID, rule.OrgID, rule.Priority, rule.RuleYAML).
		Scan(&rr.ID, &rr.OrgID, &rr.Priority, &rr.RuleYAML, &rr.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("RateLimitRuleRepository.Create: scan: %w", err)
	}
	return &rr, nil
}

// Update replaces the mutable fields of the RateLimitRuleRecord identified by id.
// Returns the updated record.
//
// ctx controls deadline and cancellation for the database operation.
// Returns model.ErrNotFound if no record exists with the given id.
func (r *RateLimitRuleRepository) Update(ctx context.Context, id string, rule *RateLimitRuleRecord) (*RateLimitRuleRecord, error) {
	const q = `
		UPDATE rate_limit_rules
		SET priority = $2, rule_yaml = $3
		WHERE id = $1
		RETURNING id, org_id, priority, rule_yaml, created_at
	`

	var rr RateLimitRuleRecord
	err := r.Pool.QueryRow(ctx, q, id, rule.Priority, rule.RuleYAML).
		Scan(&rr.ID, &rr.OrgID, &rr.Priority, &rr.RuleYAML, &rr.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("RateLimitRuleRepository.Update: %w", model.ErrNotFound)
		}
		return nil, fmt.Errorf("RateLimitRuleRepository.Update: scan: %w", err)
	}
	return &rr, nil
}

// Delete removes the RateLimitRuleRecord identified by id.
//
// ctx controls deadline and cancellation for the database operation.
// Returns model.ErrNotFound if no record exists with the given id.
func (r *RateLimitRuleRepository) Delete(ctx context.Context, id string) error {
	const q = `DELETE FROM rate_limit_rules WHERE id = $1`

	tag, err := r.Pool.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("RateLimitRuleRepository.Delete: exec: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("RateLimitRuleRepository.Delete: %w", model.ErrNotFound)
	}
	return nil
}
