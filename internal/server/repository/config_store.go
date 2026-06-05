package repository

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pangreksa/ai-gateway-engine/pkg/model"
)

// ConfigStoreRepository persists and retrieves the current GatewayConfig.
// Configs are stored as JSON blobs with a SHA-256 version hash in the
// gateway_configs table. The table is created inline on startup if absent.
//
// Thread-safety: safe for concurrent use; all operations use the shared Pool.
type ConfigStoreRepository struct {
	// Pool is the underlying pgx connection pool.
	Pool *pgxpool.Pool
}

// NewConfigStoreRepository constructs a ConfigStoreRepository backed by the given DB.
func NewConfigStoreRepository(db *DB) *ConfigStoreRepository {
	return &ConfigStoreRepository{Pool: db.Pool}
}

// EnsureTable creates the gateway_configs table if it does not already exist.
// This is called once at startup before any read/write operations.
//
// ctx controls deadline and cancellation for the DDL statement.
func (r *ConfigStoreRepository) EnsureTable(ctx context.Context) error {
	const ddl = `
		CREATE TABLE IF NOT EXISTS gateway_configs (
			id         BIGSERIAL    PRIMARY KEY,
			version    VARCHAR(64)  NOT NULL,
			config     JSONB        NOT NULL,
			created_at TIMESTAMPTZ  NOT NULL DEFAULT NOW()
		)
	`
	if _, err := r.Pool.Exec(ctx, ddl); err != nil {
		return fmt.Errorf("ConfigStoreRepository.EnsureTable: %w", err)
	}
	return nil
}

// GetCurrent returns the most recently stored GatewayConfig.
//
// ctx controls deadline and cancellation for the database query.
// Returns model.ErrNotFound if no configuration has been stored yet.
func (r *ConfigStoreRepository) GetCurrent(ctx context.Context) (*model.GatewayConfig, error) {
	const q = `
		SELECT config
		FROM gateway_configs
		ORDER BY id DESC
		LIMIT 1
	`

	row := r.Pool.QueryRow(ctx, q)

	var rawConfig []byte
	if err := row.Scan(&rawConfig); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("ConfigStoreRepository.GetCurrent: %w", model.ErrNotFound)
		}
		return nil, fmt.Errorf("ConfigStoreRepository.GetCurrent: scan: %w", err)
	}

	var cfg model.GatewayConfig
	if err := json.Unmarshal(rawConfig, &cfg); err != nil {
		return nil, fmt.Errorf("ConfigStoreRepository.GetCurrent: unmarshal: %w", err)
	}

	return &cfg, nil
}

// Upsert stores a new GatewayConfig version.
// The version hash is computed as SHA-256 of the JSON-serialised config.
// The GatewayConfig.Version field is set to the new hash before persisting.
//
// ctx controls deadline and cancellation for the database operation.
// Returns the new version hash string on success.
func (r *ConfigStoreRepository) Upsert(ctx context.Context, cfg model.GatewayConfig) (string, error) {
	version, err := computeConfigVersion(cfg)
	if err != nil {
		return "", fmt.Errorf("ConfigStoreRepository.Upsert: compute version: %w", err)
	}

	// embed the version into the config before persisting so readers always
	// have a self-describing document.
	cfg.Version = version

	b, err := json.Marshal(cfg)
	if err != nil {
		return "", fmt.Errorf("ConfigStoreRepository.Upsert: marshal config: %w", err)
	}

	const q = `
		INSERT INTO gateway_configs (version, config)
		VALUES ($1, $2)
	`
	if _, err = r.Pool.Exec(ctx, q, version, b); err != nil {
		return "", fmt.Errorf("ConfigStoreRepository.Upsert: exec: %w", err)
	}

	return version, nil
}

// computeConfigVersion returns the lowercase hex SHA-256 digest of the
// JSON-serialised GatewayConfig. The Version field is zeroed before hashing
// so it does not contribute to the digest, making the hash content-addressable.
func computeConfigVersion(cfg model.GatewayConfig) (string, error) {
	// zero the Version field so it is excluded from the hash input.
	cfg.Version = ""

	b, err := json.Marshal(cfg)
	if err != nil {
		return "", fmt.Errorf("computeConfigVersion: marshal: %w", err)
	}

	h := sha256.Sum256(b)
	return fmt.Sprintf("%x", h), nil
}
