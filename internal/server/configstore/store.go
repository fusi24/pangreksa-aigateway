// Package configstore manages the authoritative GatewayConfig lifecycle on the
// Central Server. It wraps the ConfigStoreRepository with domain logic for
// version computation and provides a clean API used by the HTTP handler.
package configstore

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/rs/zerolog"

	"github.com/pangreksa/ai-gateway-engine/internal/server/repository"
	"github.com/pangreksa/ai-gateway-engine/pkg/model"
)

// Store manages the current GatewayConfig, including version computation and
// persistence. It is the single source of truth for the config that daemon
// instances poll via POST /config.
//
// Thread-safety: safe for concurrent use; all state is held in the repository.
type Store struct {
	// repo is the underlying persistence layer for gateway configs.
	repo *repository.ConfigStoreRepository
	// log is the structured logger for config operations.
	log zerolog.Logger
}

// NewStore constructs a Store backed by the given ConfigStoreRepository.
func NewStore(repo *repository.ConfigStoreRepository, log zerolog.Logger) *Store {
	return &Store{
		repo: repo,
		log:  log.With().Str("component", "configstore").Logger(),
	}
}

// GetCurrent returns the most recently stored GatewayConfig.
//
// ctx controls deadline and cancellation for the underlying DB query.
// Returns model.ErrNotFound if no config has been stored yet.
func (s *Store) GetCurrent(ctx context.Context) (*model.GatewayConfig, error) {
	cfg, err := s.repo.GetCurrent(ctx)
	if err != nil {
		return nil, fmt.Errorf("Store.GetCurrent: %w", err)
	}
	return cfg, nil
}

// Update stores a new GatewayConfig, computing and embedding its SHA-256 version.
// The config is persisted as a new row so the full history is retained.
//
// ctx controls deadline and cancellation for the underlying DB operation.
// Returns the new version hash on success.
func (s *Store) Update(ctx context.Context, cfg model.GatewayConfig) (string, error) {
	version, err := s.repo.Upsert(ctx, cfg)
	if err != nil {
		return "", fmt.Errorf("Store.Update: %w", err)
	}

	s.log.Info().
		Str("version", version).
		Msg("gateway config updated")

	return version, nil
}

// ComputeVersion returns the lowercase hex SHA-256 digest of the JSON-serialised cfg.
// The Version field of cfg is zeroed before hashing so only the content contributes
// to the digest, making the hash content-addressable.
//
// This function is exported so the HTTP handler can compare versions without a DB
// round-trip.
func ComputeVersion(cfg model.GatewayConfig) (string, error) {
	cfg.Version = ""
	b, err := json.Marshal(cfg)
	if err != nil {
		return "", fmt.Errorf("configstore.ComputeVersion: marshal: %w", err)
	}
	h := sha256.Sum256(b)
	return fmt.Sprintf("%x", h), nil
}
