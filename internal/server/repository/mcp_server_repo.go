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

// MCPServerRecord is a row from the mcp_servers table.
// It describes an external Model Context Protocol server registered for an org.
type MCPServerRecord struct {
	// ID is the UUID primary key.
	ID string `json:"id"`
	// OrgID is the UUID of the owning organization.
	OrgID string `json:"org_id"`
	// Name is the human-readable MCP server name, unique within the org.
	Name string `json:"name"`
	// URL is the base URL of the MCP server endpoint.
	URL string `json:"url"`
	// AuthType describes the authentication mechanism: none | bearer | apikey.
	AuthType string `json:"auth_type"`
	// CredentialsEnc holds the encrypted credentials blob.
	// This field is never serialised to JSON responses (json:"-").
	CredentialsEnc string `json:"-"`
	// Tools is the JSONB array of tool descriptors advertised by the MCP server.
	Tools json.RawMessage `json:"tools"`
	// Status is the current reachability state: active | unreachable | disabled.
	Status string `json:"status"`
	// CreatedAt is the timestamp when the record was created.
	CreatedAt time.Time `json:"created_at"`
}

// MCPServerRepository handles all CRUD data access for the mcp_servers table.
//
// Thread-safety: safe for concurrent use; all operations use the shared Pool.
type MCPServerRepository struct {
	// Pool is the underlying pgx connection pool.
	Pool *pgxpool.Pool
}

// NewMCPServerRepository constructs an MCPServerRepository backed by the given DB.
func NewMCPServerRepository(db *DB) *MCPServerRepository {
	return &MCPServerRepository{Pool: db.Pool}
}

// ListByOrg returns all MCPServerRecord rows for the given organization, ordered by name.
// CredentialsEnc is populated but callers must never return it to API clients.
//
// ctx controls deadline and cancellation for the database query.
func (r *MCPServerRepository) ListByOrg(ctx context.Context, orgID string) ([]MCPServerRecord, error) {
	const q = `
		SELECT id, org_id, name, url, auth_type, credentials_enc, tools, status, created_at
		FROM mcp_servers
		WHERE org_id = $1
		ORDER BY name
	`

	rows, err := r.Pool.Query(ctx, q, orgID)
	if err != nil {
		return nil, fmt.Errorf("MCPServerRepository.ListByOrg: query: %w", err)
	}
	defer rows.Close()

	var servers []MCPServerRecord
	for rows.Next() {
		s, err := scanMCPServer(rows)
		if err != nil {
			return nil, fmt.Errorf("MCPServerRepository.ListByOrg: scan: %w", err)
		}
		servers = append(servers, *s)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("MCPServerRepository.ListByOrg: rows: %w", err)
	}

	if servers == nil {
		servers = []MCPServerRecord{}
	}
	return servers, nil
}

// FindByID retrieves a single MCPServerRecord by its UUID.
//
// ctx controls deadline and cancellation for the database query.
// Returns model.ErrNotFound if no server exists with the given id.
func (r *MCPServerRepository) FindByID(ctx context.Context, id string) (*MCPServerRecord, error) {
	const q = `
		SELECT id, org_id, name, url, auth_type, credentials_enc, tools, status, created_at
		FROM mcp_servers
		WHERE id = $1
	`

	row := r.Pool.QueryRow(ctx, q, id)
	s, err := scanMCPServer(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("MCPServerRepository.FindByID: %w", model.ErrNotFound)
		}
		return nil, fmt.Errorf("MCPServerRepository.FindByID: scan: %w", err)
	}
	return s, nil
}

// Create inserts a new MCPServerRecord and returns the persisted row.
//
// ctx controls deadline and cancellation for the database operation.
func (r *MCPServerRepository) Create(ctx context.Context, s *MCPServerRecord) (*MCPServerRecord, error) {
	s.ID = uuid.New().String()

	const q = `
		INSERT INTO mcp_servers (id, org_id, name, url, auth_type, credentials_enc, tools, status, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())
		RETURNING id, org_id, name, url, auth_type, credentials_enc, tools, status, created_at
	`

	row := r.Pool.QueryRow(ctx, q,
		s.ID, s.OrgID, s.Name, s.URL, s.AuthType,
		s.CredentialsEnc, []byte(s.Tools), s.Status,
	)
	created, err := scanMCPServer(row)
	if err != nil {
		return nil, fmt.Errorf("MCPServerRepository.Create: scan: %w", err)
	}
	return created, nil
}

// Update replaces the mutable fields of the MCPServerRecord identified by id.
// Returns the updated record.
//
// ctx controls deadline and cancellation for the database operation.
// Returns model.ErrNotFound if no server exists with the given id.
func (r *MCPServerRepository) Update(ctx context.Context, id string, s *MCPServerRecord) (*MCPServerRecord, error) {
	const q = `
		UPDATE mcp_servers
		SET name = $2, url = $3, auth_type = $4,
		    credentials_enc = $5, tools = $6, status = $7
		WHERE id = $1
		RETURNING id, org_id, name, url, auth_type, credentials_enc, tools, status, created_at
	`

	row := r.Pool.QueryRow(ctx, q,
		id, s.Name, s.URL, s.AuthType,
		s.CredentialsEnc, []byte(s.Tools), s.Status,
	)
	updated, err := scanMCPServer(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("MCPServerRepository.Update: %w", model.ErrNotFound)
		}
		return nil, fmt.Errorf("MCPServerRepository.Update: scan: %w", err)
	}
	return updated, nil
}

// Delete removes the MCPServerRecord identified by id.
//
// ctx controls deadline and cancellation for the database operation.
// Returns model.ErrNotFound if no server exists with the given id.
func (r *MCPServerRepository) Delete(ctx context.Context, id string) error {
	const q = `DELETE FROM mcp_servers WHERE id = $1`

	tag, err := r.Pool.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("MCPServerRepository.Delete: exec: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("MCPServerRepository.Delete: %w", model.ErrNotFound)
	}
	return nil
}

// scanMCPServer maps a pgx row into an MCPServerRecord struct.
func scanMCPServer(row pgx.Row) (*MCPServerRecord, error) {
	var s MCPServerRecord
	var toolsRaw []byte
	err := row.Scan(
		&s.ID, &s.OrgID, &s.Name, &s.URL, &s.AuthType,
		&s.CredentialsEnc, &toolsRaw, &s.Status, &s.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	if len(toolsRaw) > 0 {
		s.Tools = json.RawMessage(toolsRaw)
	}
	return &s, nil
}
