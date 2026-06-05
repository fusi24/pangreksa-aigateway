package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pangreksa/ai-gateway-engine/pkg/model"
)

// DaemonRegistration is the DB row for a registered daemon heartbeat.
//
// Each gateway daemon upserts its row on every heartbeat call.
// The Status field is derived from last_seen_at and is not stored in the database.
type DaemonRegistration struct {
	// GatewayID is the unique identifier for the gateway daemon instance.
	GatewayID string
	// OrgID is the UUID of the owning organization.
	OrgID string
	// Version is the daemon binary version string (e.g. "1.2.3").
	Version string
	// ConfigVersion is the SHA-256 digest of the currently loaded GatewayConfig.
	ConfigVersion string
	// UptimeSeconds is the number of seconds the daemon has been running.
	UptimeSeconds int64
	// ProviderPorts maps LLM provider name to its local proxy port number.
	ProviderPorts map[string]int
	// OtelEndpoint is the OTLP collector endpoint configured on this daemon.
	OtelEndpoint string
	// LastSeenAt is the timestamp of the most recent heartbeat.
	LastSeenAt time.Time
	// RegisteredAt is the timestamp when the daemon first registered.
	RegisteredAt time.Time
	// Status is the derived health status: healthy | degraded | dead.
	// healthy  — last heartbeat < 60 seconds ago.
	// degraded — last heartbeat 60–299 seconds ago.
	// dead     — last heartbeat ≥ 300 seconds ago or never received.
	Status string
}

// DaemonRegistrationRepository handles daemon_registrations CRUD.
//
// Thread-safety: safe for concurrent use; all operations use the shared Pool.
type DaemonRegistrationRepository struct {
	// Pool is the underlying pgx connection pool.
	Pool *pgxpool.Pool
}

// NewDaemonRegistrationRepository constructs a DaemonRegistrationRepository
// backed by the given DB connection pool.
func NewDaemonRegistrationRepository(db *DB) *DaemonRegistrationRepository {
	return &DaemonRegistrationRepository{Pool: db.Pool}
}

// Upsert inserts or updates a daemon heartbeat row in daemon_registrations.
//
// On conflict (gateway_id already exists), all mutable columns are updated and
// last_seen_at is refreshed to NOW(). registered_at is preserved on update.
func (r *DaemonRegistrationRepository) Upsert(ctx context.Context, reg *DaemonRegistration) error {
	portsJSON, err := json.Marshal(reg.ProviderPorts)
	if err != nil {
		return fmt.Errorf("DaemonRegistrationRepository.Upsert: marshal provider_ports: %w", err)
	}

	const q = `
		INSERT INTO daemon_registrations
			(gateway_id, org_id, version, config_version, uptime_seconds,
			 provider_ports, otel_endpoint, last_seen_at, registered_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), NOW())
		ON CONFLICT (gateway_id) DO UPDATE SET
			org_id         = EXCLUDED.org_id,
			version        = EXCLUDED.version,
			config_version = EXCLUDED.config_version,
			uptime_seconds = EXCLUDED.uptime_seconds,
			provider_ports = EXCLUDED.provider_ports,
			otel_endpoint  = EXCLUDED.otel_endpoint,
			last_seen_at   = NOW()
	`

	_, err = r.Pool.Exec(ctx, q,
		reg.GatewayID,
		reg.OrgID,
		reg.Version,
		reg.ConfigVersion,
		reg.UptimeSeconds,
		portsJSON,
		reg.OtelEndpoint,
	)
	if err != nil {
		return fmt.Errorf("DaemonRegistrationRepository.Upsert: exec: %w", err)
	}
	return nil
}

// ListByOrg returns all daemon registrations for an organization sorted by
// last_seen_at descending, with Status derived from the age of last_seen_at.
//
// Status derivation:
//   - healthy  — last_seen_at within the past 60 seconds
//   - degraded — last_seen_at within the past 300 seconds
//   - dead     — last_seen_at ≥ 300 seconds ago
func (r *DaemonRegistrationRepository) ListByOrg(ctx context.Context, orgID string) ([]DaemonRegistration, error) {
	const q = `
		SELECT gateway_id, org_id, version, config_version, uptime_seconds,
		       provider_ports, otel_endpoint, last_seen_at, registered_at,
		       CASE
		         WHEN last_seen_at > NOW() - INTERVAL '60 seconds'  THEN 'healthy'
		         WHEN last_seen_at > NOW() - INTERVAL '300 seconds' THEN 'degraded'
		         ELSE 'dead'
		       END AS status
		FROM daemon_registrations
		WHERE org_id = $1
		ORDER BY last_seen_at DESC
	`

	rows, err := r.Pool.Query(ctx, q, orgID)
	if err != nil {
		return nil, fmt.Errorf("DaemonRegistrationRepository.ListByOrg: query: %w", err)
	}
	defer rows.Close()

	var result []DaemonRegistration
	for rows.Next() {
		reg, err := scanDaemonRegistration(rows)
		if err != nil {
			return nil, fmt.Errorf("DaemonRegistrationRepository.ListByOrg: scan: %w", err)
		}
		result = append(result, *reg)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("DaemonRegistrationRepository.ListByOrg: rows iteration: %w", err)
	}

	return result, nil
}

// FindByID returns a single daemon registration identified by gatewayID.
//
// Returns model.ErrNotFound if no registration exists with the given ID.
func (r *DaemonRegistrationRepository) FindByID(ctx context.Context, gatewayID string) (*DaemonRegistration, error) {
	const q = `
		SELECT gateway_id, org_id, version, config_version, uptime_seconds,
		       provider_ports, otel_endpoint, last_seen_at, registered_at,
		       CASE
		         WHEN last_seen_at > NOW() - INTERVAL '60 seconds'  THEN 'healthy'
		         WHEN last_seen_at > NOW() - INTERVAL '300 seconds' THEN 'degraded'
		         ELSE 'dead'
		       END AS status
		FROM daemon_registrations
		WHERE gateway_id = $1
	`

	row := r.Pool.QueryRow(ctx, q, gatewayID)
	reg, err := scanDaemonRegistration(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("DaemonRegistrationRepository.FindByID: %w", model.ErrNotFound)
		}
		return nil, fmt.Errorf("DaemonRegistrationRepository.FindByID: scan: %w", err)
	}
	return reg, nil
}

// scanDaemonRegistration maps a pgx row into a DaemonRegistration struct.
// provider_ports JSONB is scanned into []byte then unmarshalled into map[string]int.
func scanDaemonRegistration(row pgx.Row) (*DaemonRegistration, error) {
	var reg DaemonRegistration
	var portsJSON []byte

	err := row.Scan(
		&reg.GatewayID,
		&reg.OrgID,
		&reg.Version,
		&reg.ConfigVersion,
		&reg.UptimeSeconds,
		&portsJSON,
		&reg.OtelEndpoint,
		&reg.LastSeenAt,
		&reg.RegisteredAt,
		&reg.Status,
	)
	if err != nil {
		return nil, err
	}

	if len(portsJSON) > 0 {
		if err = json.Unmarshal(portsJSON, &reg.ProviderPorts); err != nil {
			return nil, fmt.Errorf("scanDaemonRegistration: unmarshal provider_ports: %w", err)
		}
	}
	if reg.ProviderPorts == nil {
		reg.ProviderPorts = make(map[string]int)
	}

	return &reg, nil
}
