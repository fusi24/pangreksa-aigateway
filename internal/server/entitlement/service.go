// Package entitlement provides the Central Server's entitlement business logic.
// It resolves user entitlements from the database and publishes Redis invalidation
// signals whenever an entitlement is modified.
package entitlement

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog"

	"github.com/pangreksa/ai-gateway-engine/internal/server/publisher"
	"github.com/pangreksa/ai-gateway-engine/internal/server/repository"
	"github.com/pangreksa/ai-gateway-engine/pkg/model"
)

// entitlementTTL is the duration added to time.Now() to compute UserEntitlement.ExpiresAt.
// Daemon instances must re-fetch from Central Server after this period even if their
// Redis TTL has not expired.
const entitlementTTL = 5 * time.Minute

// Service handles entitlement resolution and updates.
// It coordinates between the repository layer (DB) and the publisher (Redis).
//
// Thread-safety: safe for concurrent use; no mutable state after construction.
type Service struct {
	// entRepo is the repository for user_entitlements data access.
	entRepo *repository.EntitlementRepository
	// userRepo is the repository for users data access.
	userRepo *repository.UserRepository
	// pub is the Redis publisher for cache invalidation broadcasts.
	pub *publisher.Publisher
	// log is the structured logger for entitlement operations.
	log zerolog.Logger
}

// NewService constructs an entitlement Service with all required dependencies.
// All parameters must be non-nil; passing nil will cause a panic on first use.
func NewService(
	entRepo *repository.EntitlementRepository,
	userRepo *repository.UserRepository,
	pub *publisher.Publisher,
	log zerolog.Logger,
) *Service {
	return &Service{
		entRepo:  entRepo,
		userRepo: userRepo,
		pub:      pub,
		log:      log.With().Str("component", "entitlement.service").Logger(),
	}
}

// Resolve returns the UserEntitlement for the given userID and orgID.
//
// It first verifies the user exists in the given org, then loads the entitlement
// row. The ExpiresAt field is set to time.Now() + 5 minutes so daemon instances
// know when to re-fetch.
//
// ctx controls deadline and cancellation for all database queries.
// Returns model.ErrNotFound if the user or entitlement does not exist.
func (s *Service) Resolve(ctx context.Context, userID, orgID string) (*model.UserEntitlement, error) {
	// verify the user exists and belongs to the given org.
	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("Service.Resolve: lookup user: %w", err)
	}
	// if orgID is provided, verify the user belongs to it; if empty, trust the user's own org.
	if orgID != "" && user.OrgID != orgID {
		return nil, fmt.Errorf("Service.Resolve: %w", model.ErrNotFound)
	}

	ent, err := s.entRepo.FindByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("Service.Resolve: lookup entitlement: %w", err)
	}

	result := &model.UserEntitlement{
		UserID:         ent.UserID,
		OrgID:          ent.OrgID,
		AllowedPrompts: ent.AllowedPrompts,
		AllowedSkills:  ent.AllowedSkills,
		AllowedMCPs:    ent.AllowedMCPs,
		BudgetLimitUSD: ent.BudgetLimitUSD,
		RateLimitRPM:   ent.RateLimitRPM,
		RateLimitTPM:   ent.RateLimitTPM,
		DataScope:      ent.DataScope,
		ExpiresAt:      time.Now().UTC().Add(entitlementTTL),
	}

	return result, nil
}

// Update applies the given changes map to the user's entitlement row and then
// publishes a Redis invalidation signal so all daemon instances evict their
// cached copy of this user's entitlement.
//
// ctx controls deadline and cancellation for the database update and Redis publish.
// Returns model.ErrNotFound if no entitlement row exists for userID.
// If the publisher is nil (Redis not configured), the update still succeeds and
// a warning is logged.
func (s *Service) Update(ctx context.Context, userID string, changes map[string]interface{}) error {
	if err := s.entRepo.Update(ctx, userID, changes); err != nil {
		return fmt.Errorf("Service.Update: db update: %w", err)
	}

	if s.pub == nil {
		s.log.Warn().
			Str("user_id", userID).
			Msg("entitlement updated but Redis publisher not configured; skipping invalidation")
	} else if err := s.pub.InvalidateUser(ctx, userID); err != nil {
		// log the error but do not fail the update — the daemon will re-fetch
		// from Central Server after its TTL expires even without invalidation.
		s.log.Warn().
			Str("user_id", userID).
			Err(err).
			Msg("entitlement updated but Redis invalidation failed")
	}

	s.log.Info().
		Str("user_id", userID).
		Int("change_count", len(changes)).
		Msg("entitlement updated")

	return nil
}
