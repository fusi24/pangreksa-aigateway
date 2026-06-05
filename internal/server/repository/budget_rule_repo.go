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

// BudgetRuleRecord is a row from the budget_rules table.
// Each record defines a prioritised YAML-encoded spending limit rule for an org.
type BudgetRuleRecord struct {
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

// BudgetRuleRepository handles all CRUD data access for the budget_rules table.
//
// Thread-safety: safe for concurrent use; all operations use the shared Pool.
type BudgetRuleRepository struct {
	// Pool is the underlying pgx connection pool.
	Pool *pgxpool.Pool
}

// NewBudgetRuleRepository constructs a BudgetRuleRepository backed by the given DB.
func NewBudgetRuleRepository(db *DB) *BudgetRuleRepository {
	return &BudgetRuleRepository{Pool: db.Pool}
}

// ListByOrg returns all BudgetRuleRecord rows for the given organization,
// ordered by priority ascending.
//
// ctx controls deadline and cancellation for the database query.
func (r *BudgetRuleRepository) ListByOrg(ctx context.Context, orgID string) ([]BudgetRuleRecord, error) {
	const q = `
		SELECT id, org_id, priority, rule_yaml, created_at
		FROM budget_rules
		WHERE org_id = $1
		ORDER BY priority ASC
	`

	rows, err := r.Pool.Query(ctx, q, orgID)
	if err != nil {
		return nil, fmt.Errorf("BudgetRuleRepository.ListByOrg: query: %w", err)
	}
	defer rows.Close()

	var rules []BudgetRuleRecord
	for rows.Next() {
		var br BudgetRuleRecord
		if err = rows.Scan(&br.ID, &br.OrgID, &br.Priority, &br.RuleYAML, &br.CreatedAt); err != nil {
			return nil, fmt.Errorf("BudgetRuleRepository.ListByOrg: scan: %w", err)
		}
		rules = append(rules, br)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("BudgetRuleRepository.ListByOrg: rows: %w", err)
	}

	if rules == nil {
		rules = []BudgetRuleRecord{}
	}
	return rules, nil
}

// FindByID retrieves a single BudgetRuleRecord by its UUID.
//
// ctx controls deadline and cancellation for the database query.
// Returns model.ErrNotFound if no record exists with the given id.
func (r *BudgetRuleRepository) FindByID(ctx context.Context, id string) (*BudgetRuleRecord, error) {
	const q = `
		SELECT id, org_id, priority, rule_yaml, created_at
		FROM budget_rules
		WHERE id = $1
	`

	var br BudgetRuleRecord
	err := r.Pool.QueryRow(ctx, q, id).Scan(&br.ID, &br.OrgID, &br.Priority, &br.RuleYAML, &br.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("BudgetRuleRepository.FindByID: %w", model.ErrNotFound)
		}
		return nil, fmt.Errorf("BudgetRuleRepository.FindByID: scan: %w", err)
	}
	return &br, nil
}

// Create inserts a new BudgetRuleRecord and returns the persisted row.
//
// ctx controls deadline and cancellation for the database operation.
func (r *BudgetRuleRepository) Create(ctx context.Context, rule *BudgetRuleRecord) (*BudgetRuleRecord, error) {
	rule.ID = uuid.New().String()

	const q = `
		INSERT INTO budget_rules (id, org_id, priority, rule_yaml, created_at)
		VALUES ($1, $2, $3, $4, NOW())
		RETURNING id, org_id, priority, rule_yaml, created_at
	`

	var br BudgetRuleRecord
	err := r.Pool.QueryRow(ctx, q, rule.ID, rule.OrgID, rule.Priority, rule.RuleYAML).
		Scan(&br.ID, &br.OrgID, &br.Priority, &br.RuleYAML, &br.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("BudgetRuleRepository.Create: scan: %w", err)
	}
	return &br, nil
}

// Update replaces the mutable fields of the BudgetRuleRecord identified by id.
// Returns the updated record.
//
// ctx controls deadline and cancellation for the database operation.
// Returns model.ErrNotFound if no record exists with the given id.
func (r *BudgetRuleRepository) Update(ctx context.Context, id string, rule *BudgetRuleRecord) (*BudgetRuleRecord, error) {
	const q = `
		UPDATE budget_rules
		SET priority = $2, rule_yaml = $3
		WHERE id = $1
		RETURNING id, org_id, priority, rule_yaml, created_at
	`

	var br BudgetRuleRecord
	err := r.Pool.QueryRow(ctx, q, id, rule.Priority, rule.RuleYAML).
		Scan(&br.ID, &br.OrgID, &br.Priority, &br.RuleYAML, &br.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("BudgetRuleRepository.Update: %w", model.ErrNotFound)
		}
		return nil, fmt.Errorf("BudgetRuleRepository.Update: scan: %w", err)
	}
	return &br, nil
}

// Delete removes the BudgetRuleRecord identified by id.
//
// ctx controls deadline and cancellation for the database operation.
// Returns model.ErrNotFound if no record exists with the given id.
func (r *BudgetRuleRepository) Delete(ctx context.Context, id string) error {
	const q = `DELETE FROM budget_rules WHERE id = $1`

	tag, err := r.Pool.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("BudgetRuleRepository.Delete: exec: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("BudgetRuleRepository.Delete: %w", model.ErrNotFound)
	}
	return nil
}
