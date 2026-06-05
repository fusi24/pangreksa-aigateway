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

// PromptRepository is a prompt namespace row from the prompt_repositories table.
// It groups one or more named prompts under a single org-scoped repository.
type PromptRepository struct {
	// ID is the UUID primary key.
	ID string `json:"id"`
	// OrgID is the UUID of the owning organization.
	OrgID string `json:"org_id"`
	// Name is the human-readable repository name, unique within an org.
	Name string `json:"name"`
}

// PromptVersion is a single immutable versioned prompt row from prompt_versions.
// Once created, the Content field cannot be modified; a new version must be created instead.
type PromptVersion struct {
	// ID is the UUID primary key.
	ID string `json:"id"`
	// RepoID is the UUID of the owning PromptRepository.
	RepoID string `json:"repo_id"`
	// PromptName is the logical name of the prompt within the repository.
	PromptName string `json:"prompt_name"`
	// Version is the auto-incremented version number, starting at 1.
	Version int `json:"version"`
	// Content is the immutable prompt text.
	Content string `json:"content"`
	// Config is the optional JSONB configuration blob for the prompt.
	Config json.RawMessage `json:"config"`
	// CreatedBy is the identifier of the user or service that created this version.
	CreatedBy string `json:"created_by,omitempty"`
	// CreatedAt is the timestamp when this version was created.
	CreatedAt time.Time `json:"created_at"`
	// FQN is the fully-qualified name, computed as chat_prompt:{repo}/{name}:{version}.
	FQN string `json:"fqn"`
}

// PromptRepo wraps the prompt_repositories and prompt_versions tables.
//
// Thread-safety: safe for concurrent use; all operations use the shared Pool.
type PromptRepo struct {
	// Pool is the underlying pgx connection pool.
	Pool *pgxpool.Pool
}

// NewPromptRepo constructs a PromptRepo backed by the given DB.
func NewPromptRepo(db *DB) *PromptRepo {
	return &PromptRepo{Pool: db.Pool}
}

// ListReposByOrg returns all prompt repositories for the given org, along with
// their associated prompt versions.
//
// ctx controls deadline and cancellation for the database query.
func (r *PromptRepo) ListReposByOrg(ctx context.Context, orgID string) ([]PromptRepository, error) {
	const q = `
		SELECT id, org_id, name
		FROM prompt_repositories
		WHERE org_id = $1
		ORDER BY name
	`

	rows, err := r.Pool.Query(ctx, q, orgID)
	if err != nil {
		return nil, fmt.Errorf("PromptRepo.ListReposByOrg: query: %w", err)
	}
	defer rows.Close()

	var repos []PromptRepository
	for rows.Next() {
		var pr PromptRepository
		if err = rows.Scan(&pr.ID, &pr.OrgID, &pr.Name); err != nil {
			return nil, fmt.Errorf("PromptRepo.ListReposByOrg: scan: %w", err)
		}
		repos = append(repos, pr)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("PromptRepo.ListReposByOrg: rows: %w", err)
	}

	if repos == nil {
		repos = []PromptRepository{}
	}
	return repos, nil
}

// ListVersions returns all PromptVersion rows for the given repository ID and
// prompt name, ordered by version ascending.
//
// ctx controls deadline and cancellation for the database query.
func (r *PromptRepo) ListVersions(ctx context.Context, repoID, promptName string) ([]PromptVersion, error) {
	const q = `
		SELECT pv.id, pv.repo_id, pv.prompt_name, pv.version,
		       pv.content, pv.config, pv.created_by, pv.created_at,
		       pr.name AS repo_name
		FROM prompt_versions pv
		JOIN prompt_repositories pr ON pr.id = pv.repo_id
		WHERE pv.repo_id = $1 AND pv.prompt_name = $2
		ORDER BY pv.version ASC
	`

	rows, err := r.Pool.Query(ctx, q, repoID, promptName)
	if err != nil {
		return nil, fmt.Errorf("PromptRepo.ListVersions: query: %w", err)
	}
	defer rows.Close()

	var versions []PromptVersion
	for rows.Next() {
		var pv PromptVersion
		var configRaw []byte
		var repoName string
		if err = rows.Scan(
			&pv.ID, &pv.RepoID, &pv.PromptName, &pv.Version,
			&pv.Content, &configRaw, &pv.CreatedBy, &pv.CreatedAt,
			&repoName,
		); err != nil {
			return nil, fmt.Errorf("PromptRepo.ListVersions: scan: %w", err)
		}
		if len(configRaw) > 0 {
			pv.Config = json.RawMessage(configRaw)
		}
		pv.FQN = fmt.Sprintf("chat_prompt:%s/%s:%d", repoName, pv.PromptName, pv.Version)
		versions = append(versions, pv)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("PromptRepo.ListVersions: rows: %w", err)
	}

	if versions == nil {
		versions = []PromptVersion{}
	}
	return versions, nil
}

// CreateVersion inserts a new prompt version. The version number is computed as
// MAX(version)+1 for the given repo+name combination (starting at 1 if none exist).
// Content is immutable after creation.
//
// ctx controls deadline and cancellation for the database operation.
// Returns the newly created PromptVersion including its computed FQN.
func (r *PromptRepo) CreateVersion(
	ctx context.Context,
	repoID, promptName, content string,
	config json.RawMessage,
	createdBy string,
) (*PromptVersion, error) {
	// Resolve the repository name for FQN construction.
	const repoQ = `SELECT name FROM prompt_repositories WHERE id = $1`
	var repoName string
	if err := r.Pool.QueryRow(ctx, repoQ, repoID).Scan(&repoName); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("PromptRepo.CreateVersion: %w", model.ErrNotFound)
		}
		return nil, fmt.Errorf("PromptRepo.CreateVersion: resolve repo name: %w", err)
	}

	const q = `
		INSERT INTO prompt_versions (id, repo_id, prompt_name, version, content, config, created_by, created_at)
		VALUES (
			$1, $2, $3,
			COALESCE((SELECT MAX(version) FROM prompt_versions WHERE repo_id = $2 AND prompt_name = $3), 0) + 1,
			$4, $5, $6, NOW()
		)
		RETURNING id, repo_id, prompt_name, version, content, config, created_by, created_at
	`

	newID := uuid.New().String()
	var pv PromptVersion
	var configRaw []byte

	row := r.Pool.QueryRow(ctx, q, newID, repoID, promptName, content, []byte(config), createdBy)
	if err := row.Scan(
		&pv.ID, &pv.RepoID, &pv.PromptName, &pv.Version,
		&pv.Content, &configRaw, &pv.CreatedBy, &pv.CreatedAt,
	); err != nil {
		return nil, fmt.Errorf("PromptRepo.CreateVersion: scan: %w", err)
	}
	if len(configRaw) > 0 {
		pv.Config = json.RawMessage(configRaw)
	}
	pv.FQN = fmt.Sprintf("chat_prompt:%s/%s:%d", repoName, pv.PromptName, pv.Version)
	return &pv, nil
}

// EnsureRepo creates a prompt_repository row for the given org+name if one does
// not already exist. Returns the repository ID whether it was newly created or
// already present.
//
// ctx controls deadline and cancellation for the database operation.
func (r *PromptRepo) EnsureRepo(ctx context.Context, orgID, repoName string) (string, error) {
	const q = `
		INSERT INTO prompt_repositories (id, org_id, name)
		VALUES ($1, $2, $3)
		ON CONFLICT (org_id, name) DO NOTHING
		RETURNING id
	`

	newID := uuid.New().String()
	var id string
	err := r.Pool.QueryRow(ctx, q, newID, orgID, repoName).Scan(&id)
	if err != nil {
		// ON CONFLICT DO NOTHING causes ErrNoRows when the row already exists.
		if errors.Is(err, pgx.ErrNoRows) {
			const selectQ = `SELECT id FROM prompt_repositories WHERE org_id = $1 AND name = $2`
			if err2 := r.Pool.QueryRow(ctx, selectQ, orgID, repoName).Scan(&id); err2 != nil {
				return "", fmt.Errorf("PromptRepo.EnsureRepo: lookup existing: %w", err2)
			}
			return id, nil
		}
		return "", fmt.Errorf("PromptRepo.EnsureRepo: insert: %w", err)
	}
	return id, nil
}
