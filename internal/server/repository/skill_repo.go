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

// SkillRecord is a row from the skills table.
// A skill encapsulates a named, versioned, reusable prompt/tool definition
// that can be attached to user entitlements.
type SkillRecord struct {
	// ID is the UUID primary key.
	ID string `json:"id"`
	// OrgID is the UUID of the owning organization.
	OrgID string `json:"org_id"`
	// Name is the unique skill name within the org.
	Name string `json:"name"`
	// Description is a human-readable description of the skill's purpose.
	Description string `json:"description"`
	// Version is the current version number of the skill.
	Version int `json:"version"`
	// Content is the skill definition (prompt text, tool spec, etc.).
	Content string `json:"content"`
	// Frontmatter is the optional JSONB metadata blob for the skill.
	Frontmatter json.RawMessage `json:"frontmatter"`
	// Preload indicates whether the skill should be loaded into context automatically.
	Preload bool `json:"preload"`
	// CreatedAt is the timestamp when the skill record was created.
	CreatedAt time.Time `json:"created_at"`
}

// SkillRepository handles all CRUD data access for the skills table.
//
// Thread-safety: safe for concurrent use; all operations use the shared Pool.
type SkillRepository struct {
	// Pool is the underlying pgx connection pool.
	Pool *pgxpool.Pool
}

// NewSkillRepository constructs a SkillRepository backed by the given DB.
func NewSkillRepository(db *DB) *SkillRepository {
	return &SkillRepository{Pool: db.Pool}
}

// ListByOrg returns all SkillRecord rows for the given organization, ordered by name.
//
// ctx controls deadline and cancellation for the database query.
func (r *SkillRepository) ListByOrg(ctx context.Context, orgID string) ([]SkillRecord, error) {
	const q = `
		SELECT id, org_id, name, description, version, content, frontmatter, preload, created_at
		FROM skills
		WHERE org_id = $1
		ORDER BY name
	`

	rows, err := r.Pool.Query(ctx, q, orgID)
	if err != nil {
		return nil, fmt.Errorf("SkillRepository.ListByOrg: query: %w", err)
	}
	defer rows.Close()

	var skills []SkillRecord
	for rows.Next() {
		s, err := scanSkill(rows)
		if err != nil {
			return nil, fmt.Errorf("SkillRepository.ListByOrg: scan: %w", err)
		}
		skills = append(skills, *s)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("SkillRepository.ListByOrg: rows: %w", err)
	}

	if skills == nil {
		skills = []SkillRecord{}
	}
	return skills, nil
}

// FindByID retrieves a single SkillRecord by its UUID.
//
// ctx controls deadline and cancellation for the database query.
// Returns model.ErrNotFound if no skill exists with the given id.
func (r *SkillRepository) FindByID(ctx context.Context, id string) (*SkillRecord, error) {
	const q = `
		SELECT id, org_id, name, description, version, content, frontmatter, preload, created_at
		FROM skills
		WHERE id = $1
	`

	row := r.Pool.QueryRow(ctx, q, id)
	s, err := scanSkill(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("SkillRepository.FindByID: %w", model.ErrNotFound)
		}
		return nil, fmt.Errorf("SkillRepository.FindByID: scan: %w", err)
	}
	return s, nil
}

// Create inserts a new SkillRecord and returns the persisted row with its generated
// ID and CreatedAt timestamp.
//
// ctx controls deadline and cancellation for the database operation.
func (r *SkillRepository) Create(ctx context.Context, s *SkillRecord) (*SkillRecord, error) {
	s.ID = uuid.New().String()

	const q = `
		INSERT INTO skills (id, org_id, name, description, version, content, frontmatter, preload, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())
		RETURNING id, org_id, name, description, version, content, frontmatter, preload, created_at
	`

	row := r.Pool.QueryRow(ctx, q,
		s.ID, s.OrgID, s.Name, s.Description, s.Version,
		s.Content, []byte(s.Frontmatter), s.Preload,
	)
	created, err := scanSkill(row)
	if err != nil {
		return nil, fmt.Errorf("SkillRepository.Create: scan: %w", err)
	}
	return created, nil
}

// Update replaces the mutable fields of the SkillRecord identified by id.
// Returns the updated record.
//
// ctx controls deadline and cancellation for the database operation.
// Returns model.ErrNotFound if no skill exists with the given id.
func (r *SkillRepository) Update(ctx context.Context, id string, s *SkillRecord) (*SkillRecord, error) {
	const q = `
		UPDATE skills
		SET name = $2, description = $3, version = $4,
		    content = $5, frontmatter = $6, preload = $7
		WHERE id = $1
		RETURNING id, org_id, name, description, version, content, frontmatter, preload, created_at
	`

	row := r.Pool.QueryRow(ctx, q,
		id, s.Name, s.Description, s.Version,
		s.Content, []byte(s.Frontmatter), s.Preload,
	)
	updated, err := scanSkill(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("SkillRepository.Update: %w", model.ErrNotFound)
		}
		return nil, fmt.Errorf("SkillRepository.Update: scan: %w", err)
	}
	return updated, nil
}

// Delete removes the SkillRecord identified by id.
//
// ctx controls deadline and cancellation for the database operation.
// Returns model.ErrNotFound if no skill exists with the given id.
func (r *SkillRepository) Delete(ctx context.Context, id string) error {
	const q = `DELETE FROM skills WHERE id = $1`

	tag, err := r.Pool.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("SkillRepository.Delete: exec: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("SkillRepository.Delete: %w", model.ErrNotFound)
	}
	return nil
}

// scanSkill maps a pgx row into a SkillRecord struct.
// It centralises field scanning to avoid duplication across query methods.
func scanSkill(row pgx.Row) (*SkillRecord, error) {
	var s SkillRecord
	var frontmatterRaw []byte
	err := row.Scan(
		&s.ID, &s.OrgID, &s.Name, &s.Description, &s.Version,
		&s.Content, &frontmatterRaw, &s.Preload, &s.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	if len(frontmatterRaw) > 0 {
		s.Frontmatter = json.RawMessage(frontmatterRaw)
	}
	return &s, nil
}
