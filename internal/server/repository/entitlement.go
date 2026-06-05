package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pangreksa/ai-gateway-engine/pkg/model"
)

// Entitlement is the database model for the user_entitlements table.
// It holds the resolved access rights for a single user at the time of last update.
type Entitlement struct {
	// ID is the UUID primary key of this entitlement row.
	ID string `db:"id"`
	// UserID is the UUID of the owning user.
	UserID string `db:"user_id"`
	// OrgID is the UUID of the owning organization.
	OrgID string `db:"org_id"`
	// AllowedPrompts is the list of FQN glob patterns the user may invoke.
	// Stored as JSONB in PostgreSQL; unmarshalled on read.
	AllowedPrompts []string
	// AllowedSkills is the list of skill name patterns the user may use.
	// Stored as JSONB in PostgreSQL; unmarshalled on read.
	AllowedSkills []string
	// AllowedMCPs is the list of MCP server name patterns the user may call.
	// Stored as JSONB in PostgreSQL; unmarshalled on read.
	AllowedMCPs []string
	// Permissions is the list of RBAC permission tokens for this user
	// (e.g. "console.observability.read"). Stored as JSONB; used to populate
	// the JWT perms claim at console login.
	Permissions []string
	// BudgetLimitUSD is the maximum spend in USD allowed for this user.
	BudgetLimitUSD float64 `db:"budget_limit_usd"`
	// RateLimitRPM is the maximum number of requests per minute.
	RateLimitRPM int `db:"rate_limit_rpm"`
	// RateLimitTPM is the maximum number of tokens per minute.
	RateLimitTPM int `db:"rate_limit_tpm"`
	// DataScope controls data visibility: own_data | team_data | all_data.
	DataScope string `db:"data_scope"`
	// UpdatedAt is the last modification timestamp of the entitlement row.
	UpdatedAt time.Time `db:"updated_at"`
}

// EntitlementRepository handles all data access for the user_entitlements table.
//
// Thread-safety: safe for concurrent use; all operations use the shared Pool.
type EntitlementRepository struct {
	// Pool is the underlying pgx connection pool.
	Pool *pgxpool.Pool
}

// NewEntitlementRepository constructs an EntitlementRepository backed by the given DB.
func NewEntitlementRepository(db *DB) *EntitlementRepository {
	return &EntitlementRepository{Pool: db.Pool}
}

// FindByUserID returns the Entitlement row for the given userID.
//
// ctx controls deadline and cancellation for the database query.
// Returns model.ErrNotFound if no entitlement row exists for the user.
func (r *EntitlementRepository) FindByUserID(ctx context.Context, userID string) (*Entitlement, error) {
	const q = `
		SELECT id, user_id, org_id,
		       allowed_prompts, allowed_skills, allowed_mcps, permissions,
		       budget_limit_usd, rate_limit_rpm, rate_limit_tpm,
		       data_scope, updated_at
		FROM user_entitlements
		WHERE user_id = $1
	`

	row := r.Pool.QueryRow(ctx, q, userID)

	var e Entitlement
	// JSONB columns are scanned as raw []byte and then unmarshalled.
	var rawPrompts, rawSkills, rawMCPs, rawPerms []byte

	err := row.Scan(
		&e.ID,
		&e.UserID,
		&e.OrgID,
		&rawPrompts,
		&rawSkills,
		&rawMCPs,
		&rawPerms,
		&e.BudgetLimitUSD,
		&e.RateLimitRPM,
		&e.RateLimitTPM,
		&e.DataScope,
		&e.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("EntitlementRepository.FindByUserID: %w", model.ErrNotFound)
		}
		return nil, fmt.Errorf("EntitlementRepository.FindByUserID: scan: %w", err)
	}

	// unmarshal JSONB arrays.
	if err = unmarshalStringSlice(rawPrompts, &e.AllowedPrompts); err != nil {
		return nil, fmt.Errorf("EntitlementRepository.FindByUserID: unmarshal allowed_prompts: %w", err)
	}
	if err = unmarshalStringSlice(rawSkills, &e.AllowedSkills); err != nil {
		return nil, fmt.Errorf("EntitlementRepository.FindByUserID: unmarshal allowed_skills: %w", err)
	}
	if err = unmarshalStringSlice(rawMCPs, &e.AllowedMCPs); err != nil {
		return nil, fmt.Errorf("EntitlementRepository.FindByUserID: unmarshal allowed_mcps: %w", err)
	}
	if err = unmarshalStringSlice(rawPerms, &e.Permissions); err != nil {
		return nil, fmt.Errorf("EntitlementRepository.FindByUserID: unmarshal permissions: %w", err)
	}

	return &e, nil
}

// Update applies the given changes map to the user_entitlements row for userID.
// Only the keys present in changes are updated; unknown keys are silently ignored.
// The updated_at column is always refreshed to NOW() regardless of the changes map.
//
// ctx controls deadline and cancellation for the database query.
// Returns model.ErrNotFound if no entitlement row exists for userID.
func (r *EntitlementRepository) Update(ctx context.Context, userID string, changes map[string]interface{}) error {
	if len(changes) == 0 {
		return nil
	}

	// allowed updatable columns mapped to their SQL identifiers.
	allowed := map[string]bool{
		"allowed_prompts":  true,
		"allowed_skills":   true,
		"allowed_mcps":     true,
		"budget_limit_usd": true,
		"rate_limit_rpm":   true,
		"rate_limit_tpm":   true,
		"data_scope":       true,
	}

	setClauses := []string{"updated_at = NOW()"}
	args := []interface{}{}
	argIdx := 1

	for col, val := range changes {
		if !allowed[col] {
			continue
		}
		// JSONB columns need to be marshalled to JSON strings.
		if col == "allowed_prompts" || col == "allowed_skills" || col == "allowed_mcps" {
			b, err := json.Marshal(val)
			if err != nil {
				return fmt.Errorf("EntitlementRepository.Update: marshal %s: %w", col, err)
			}
			setClauses = append(setClauses, fmt.Sprintf("%s = $%d", col, argIdx))
			args = append(args, string(b))
		} else {
			setClauses = append(setClauses, fmt.Sprintf("%s = $%d", col, argIdx))
			args = append(args, val)
		}
		argIdx++
	}

	// if only updated_at is set (no valid columns in changes), skip the query.
	if len(setClauses) == 1 {
		return nil
	}

	// append userID as the final WHERE parameter.
	args = append(args, userID)
	query := fmt.Sprintf(
		"UPDATE user_entitlements SET %s WHERE user_id = $%d",
		strings.Join(setClauses, ", "),
		argIdx,
	)

	tag, err := r.Pool.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("EntitlementRepository.Update: exec: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("EntitlementRepository.Update: %w", model.ErrNotFound)
	}

	return nil
}

// unmarshalStringSlice decodes a JSONB []byte into a Go []string.
// If raw is nil or empty, dest is set to an empty slice.
func unmarshalStringSlice(raw []byte, dest *[]string) error {
	if len(raw) == 0 {
		*dest = []string{}
		return nil
	}
	return json.Unmarshal(raw, dest)
}
