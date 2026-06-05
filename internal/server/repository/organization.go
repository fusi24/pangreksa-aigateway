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

// Organization is the database model for the organizations table.
// It represents a top-level tenant; all other data is scoped per org.
type Organization struct {
	// ID is the UUID primary key of the organization.
	ID string `db:"id"`
	// Name is the human-readable display name of the organization.
	Name string `db:"name"`
	// Slug is a URL-safe unique identifier for the organization.
	Slug string `db:"slug"`
	// CreatedAt is the timestamp when the organization record was created.
	CreatedAt time.Time `db:"created_at"`
}

// OrganizationRepository handles all data access for the organizations table.
//
// Thread-safety: safe for concurrent use; all operations use the shared Pool.
type OrganizationRepository struct {
	// Pool is the underlying pgx connection pool.
	Pool *pgxpool.Pool
}

// NewOrganizationRepository constructs an OrganizationRepository backed by the
// given DB connection pool.
func NewOrganizationRepository(db *DB) *OrganizationRepository {
	return &OrganizationRepository{Pool: db.Pool}
}

// FindByID retrieves an organization by its UUID string.
//
// ctx controls deadline and cancellation for the database query.
// Returns model.ErrNotFound if no organization exists with the given id.
func (r *OrganizationRepository) FindByID(ctx context.Context, id string) (*Organization, error) {
	const q = `
		SELECT id, name, slug, created_at
		FROM organizations
		WHERE id = $1
	`

	row := r.Pool.QueryRow(ctx, q, id)

	var org Organization
	if err := row.Scan(&org.ID, &org.Name, &org.Slug, &org.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("OrganizationRepository.FindByID: %w", model.ErrNotFound)
		}
		return nil, fmt.Errorf("OrganizationRepository.FindByID: scan: %w", err)
	}

	return &org, nil
}
