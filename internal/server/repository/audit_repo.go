package repository

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// AuditLogEntry is a single audit_log row returned for console display.
type AuditLogEntry struct {
	// ID is the UUID primary key of the audit_log row.
	ID string `json:"id"`
	// OrgID is the owning organisation UUID.
	OrgID string `json:"org_id"`
	// UserID is the actor's UUID; empty when the event was system-generated.
	UserID string `json:"user_id,omitempty"`
	// IPAddress is the request source IP in text form.
	IPAddress string `json:"ip_address,omitempty"`
	// EventType classifies the mutation (e.g. "user.created", "config.updated").
	EventType string `json:"event_type"`
	// ResourceType is the kind of resource affected (e.g. "user", "gateway_config").
	ResourceType string `json:"resource_type,omitempty"`
	// ResourceID is the UUID of the affected resource.
	ResourceID string `json:"resource_id,omitempty"`
	// OldValue is the resource state before the mutation.
	OldValue json.RawMessage `json:"old_value,omitempty"`
	// NewValue is the resource state after the mutation.
	NewValue json.RawMessage `json:"new_value,omitempty"`
	// CreatedAt is when the audit event was recorded.
	CreatedAt time.Time `json:"created_at"`
}

// AuditQuery carries filters for audit log queries.
type AuditQuery struct {
	// OrgID scopes results to a single organisation (required for most callers).
	OrgID string
	// EventType filters by exact event_type when non-empty.
	EventType string
	// ResourceType filters by exact resource_type when non-empty.
	ResourceType string
	// From is the inclusive start of the time window.
	From time.Time
	// To is the exclusive end of the time window.
	To time.Time
	// Q is a free-text filter applied as a case-insensitive LIKE on event_type
	// and resource_type.
	Q string
	// Limit is the maximum number of entries to return. Defaults to 100.
	Limit int
	// Cursor is the opaque pagination token returned by a previous call.
	// It encodes "created_at|id" in base64 for keyset pagination.
	Cursor string
}

// AuditRepository queries the audit_log table.
//
// Thread-safety: safe for concurrent use; all operations use the shared Pool.
type AuditRepository struct {
	// Pool is the underlying pgx connection pool.
	Pool *pgxpool.Pool
}

// NewAuditRepository constructs an AuditRepository backed by the given DB.
func NewAuditRepository(db *DB) *AuditRepository {
	return &AuditRepository{Pool: db.Pool}
}

// Query returns paginated audit log entries matching the filter.
// It uses keyset pagination: the cursor is a base64-encoded string of the form
// "<created_at RFC3339Nano>|<id>". When a cursor is provided, only rows strictly
// before the cursor position (created_at DESC, id DESC) are returned.
//
// Returns (entries, next_cursor, error). next_cursor is an empty string when no
// further pages exist.
//
// ctx controls deadline and cancellation for the database query.
func (r *AuditRepository) Query(ctx context.Context, q AuditQuery) ([]AuditLogEntry, string, error) {
	limit := q.Limit
	if limit <= 0 {
		limit = 100
	}

	// Decode cursor if provided.
	var cursorTS time.Time
	var cursorID string
	if q.Cursor != "" {
		ts, id, err := decodeCursor(q.Cursor)
		if err != nil {
			return nil, "", fmt.Errorf("AuditRepository.Query: decode cursor: %w", err)
		}
		cursorTS = ts
		cursorID = id
	}

	// Build WHERE clause dynamically.
	var clauses []string
	var args []interface{}
	argIdx := 1

	if q.OrgID != "" {
		clauses = append(clauses, fmt.Sprintf("org_id = $%d", argIdx))
		args = append(args, q.OrgID)
		argIdx++
	}
	if !q.From.IsZero() {
		clauses = append(clauses, fmt.Sprintf("created_at >= $%d", argIdx))
		args = append(args, q.From)
		argIdx++
	}
	if !q.To.IsZero() {
		clauses = append(clauses, fmt.Sprintf("created_at < $%d", argIdx))
		args = append(args, q.To)
		argIdx++
	}
	if q.EventType != "" {
		clauses = append(clauses, fmt.Sprintf("event_type = $%d", argIdx))
		args = append(args, q.EventType)
		argIdx++
	}
	if q.ResourceType != "" {
		clauses = append(clauses, fmt.Sprintf("resource_type = $%d", argIdx))
		args = append(args, q.ResourceType)
		argIdx++
	}
	if q.Q != "" {
		pattern := "%" + strings.ReplaceAll(q.Q, "%", "\\%") + "%"
		clauses = append(clauses, fmt.Sprintf(
			"(event_type ILIKE $%d OR resource_type ILIKE $%d)",
			argIdx, argIdx,
		))
		args = append(args, pattern)
		argIdx++
	}

	// Keyset pagination: (created_at, id) < (cursorTS, cursorID) for DESC order.
	if !cursorTS.IsZero() && cursorID != "" {
		clauses = append(clauses, fmt.Sprintf(
			"(created_at, id::text) < ($%d, $%d)",
			argIdx, argIdx+1,
		))
		args = append(args, cursorTS, cursorID)
		argIdx += 2
	}

	where := ""
	if len(clauses) > 0 {
		where = "WHERE " + strings.Join(clauses, " AND ")
	}

	// Fetch limit+1 rows so we can detect whether a next page exists.
	sql := fmt.Sprintf(`
		SELECT id, org_id,
		       COALESCE(user_id::text, '')     AS user_id,
		       COALESCE(ip_address::text, '')  AS ip_address,
		       event_type,
		       COALESCE(resource_type, '')     AS resource_type,
		       COALESCE(resource_id::text, '') AS resource_id,
		       old_value,
		       new_value,
		       created_at
		FROM audit_log
		%s
		ORDER BY created_at DESC, id DESC
		LIMIT $%d
	`, where, argIdx)
	args = append(args, limit+1)

	rows, err := r.Pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, "", fmt.Errorf("AuditRepository.Query: query: %w", err)
	}
	defer rows.Close()

	var entries []AuditLogEntry
	for rows.Next() {
		var e AuditLogEntry
		var rawOld, rawNew []byte

		if scanErr := rows.Scan(
			&e.ID,
			&e.OrgID,
			&e.UserID,
			&e.IPAddress,
			&e.EventType,
			&e.ResourceType,
			&e.ResourceID,
			&rawOld,
			&rawNew,
			&e.CreatedAt,
		); scanErr != nil {
			return nil, "", fmt.Errorf("AuditRepository.Query: scan: %w", scanErr)
		}

		if len(rawOld) > 0 {
			e.OldValue = json.RawMessage(rawOld)
		}
		if len(rawNew) > 0 {
			e.NewValue = json.RawMessage(rawNew)
		}

		entries = append(entries, e)
	}
	if err = rows.Err(); err != nil {
		return nil, "", fmt.Errorf("AuditRepository.Query: rows: %w", err)
	}

	// Determine next cursor.
	var nextCursor string
	if len(entries) > limit {
		entries = entries[:limit]
		last := entries[len(entries)-1]
		nextCursor = encodeCursor(last.CreatedAt, last.ID)
	}

	return entries, nextCursor, nil
}

// encodeCursor encodes a (created_at, id) pair as a base64 opaque cursor token.
// Format: base64("<created_at RFC3339Nano>|<id>").
func encodeCursor(ts time.Time, id string) string {
	raw := ts.UTC().Format(time.RFC3339Nano) + "|" + id
	return base64.StdEncoding.EncodeToString([]byte(raw))
}

// decodeCursor decodes an opaque cursor token into its constituent (ts, id) parts.
// Returns an error if the token is malformed.
func decodeCursor(cursor string) (time.Time, string, error) {
	b, err := base64.StdEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, "", fmt.Errorf("decodeCursor: base64 decode: %w", err)
	}

	parts := strings.SplitN(string(b), "|", 2)
	if len(parts) != 2 {
		return time.Time{}, "", fmt.Errorf("decodeCursor: invalid format %q", string(b))
	}

	ts, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return time.Time{}, "", fmt.Errorf("decodeCursor: parse timestamp: %w", err)
	}

	return ts, parts[1], nil
}
