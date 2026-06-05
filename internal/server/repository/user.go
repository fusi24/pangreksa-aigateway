package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pangreksa/ai-gateway-engine/pkg/model"
)

// User is the database model for the users table.
// Users are scoped to an organization and may authenticate via local credentials
// or an external identity provider (Entra ID, Active Directory).
type User struct {
	// ID is the UUID primary key.
	ID string `db:"id"`
	// OrgID is the UUID of the owning organization.
	OrgID string `db:"org_id"`
	// Email is the user's email address, unique within an org.
	Email string `db:"email"`
	// PasswordHash is the bcrypt hash of the user's password.
	// Nil when the user authenticates via an external provider.
	PasswordHash *string `db:"password_hash"`
	// Provider is the authentication provider: local | entra | ad.
	Provider string `db:"provider"`
	// Status is the account state: active | suspended | deleted.
	Status string `db:"status"`
	// MFAEnabled indicates whether multi-factor authentication is required.
	MFAEnabled bool `db:"mfa_enabled"`
	// FailedLoginCount is the number of consecutive failed login attempts.
	FailedLoginCount int `db:"failed_login_count"`
	// LockedUntil is the time until which the account is locked after too many
	// failed login attempts. Nil when the account is not locked.
	LockedUntil *time.Time `db:"locked_until"`
	// CreatedAt is the timestamp when the user record was created.
	CreatedAt time.Time `db:"created_at"`
}

// UserRepository handles all data access for the users table.
//
// Thread-safety: safe for concurrent use; all operations use the shared Pool.
type UserRepository struct {
	// Pool is the underlying pgx connection pool.
	Pool *pgxpool.Pool
}

// NewUserRepository constructs a UserRepository backed by the given DB.
func NewUserRepository(db *DB) *UserRepository {
	return &UserRepository{Pool: db.Pool}
}

// FindByID retrieves a user by its UUID string.
//
// ctx controls deadline and cancellation for the database query.
// Returns model.ErrNotFound if no user exists with the given id.
func (r *UserRepository) FindByID(ctx context.Context, id string) (*User, error) {
	const q = `
		SELECT id, org_id, email, password_hash, provider, status,
		       mfa_enabled, failed_login_count, locked_until, created_at
		FROM users
		WHERE id = $1
	`

	row := r.Pool.QueryRow(ctx, q, id)
	u, err := scanUser(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("UserRepository.FindByID: %w", model.ErrNotFound)
		}
		return nil, fmt.Errorf("UserRepository.FindByID: scan: %w", err)
	}
	return u, nil
}

// FindByEmail retrieves a user by organization ID and email address.
//
// ctx controls deadline and cancellation for the database query.
// Returns model.ErrNotFound if no matching user exists.
func (r *UserRepository) FindByEmail(ctx context.Context, orgID, email string) (*User, error) {
	const q = `
		SELECT id, org_id, email, password_hash, provider, status,
		       mfa_enabled, failed_login_count, locked_until, created_at
		FROM users
		WHERE org_id = $1 AND email = $2
	`

	row := r.Pool.QueryRow(ctx, q, orgID, email)
	u, err := scanUser(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("UserRepository.FindByEmail: %w", model.ErrNotFound)
		}
		return nil, fmt.Errorf("UserRepository.FindByEmail: scan: %w", err)
	}
	return u, nil
}

// scanUser maps a pgx row into a User struct.
// It centralises field scanning to avoid duplication across FindByID/FindByEmail.
func scanUser(row pgx.Row) (*User, error) {
	var u User
	err := row.Scan(
		&u.ID,
		&u.OrgID,
		&u.Email,
		&u.PasswordHash,
		&u.Provider,
		&u.Status,
		&u.MFAEnabled,
		&u.FailedLoginCount,
		&u.LockedUntil,
		&u.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &u, nil
}
