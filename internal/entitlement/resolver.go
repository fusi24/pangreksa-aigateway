// Package entitlement provides a three-layer entitlement resolver that progressively
// falls back to slower cache tiers on each miss:
//
//	L1: sync.Map in-process cache (~50 ns)
//	L2: Redis cache (~0.5 ms)
//	L3: Central Server HTTP POST /entitlement (~20 ms)
//
// On an L3 fetch the resolved entitlement is written through to both L1 and L2 so
// subsequent requests are served from the faster tiers.
package entitlement

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/rs/zerolog"

	"github.com/pangreksa/ai-gateway-engine/internal/cache/local"
	rediscache "github.com/pangreksa/ai-gateway-engine/internal/cache/redis"
	"github.com/pangreksa/ai-gateway-engine/pkg/model"
)

// entitlementRequest is the JSON body sent to POST /entitlement.
type entitlementRequest struct {
	// UserID is the user whose entitlement is being resolved.
	UserID string `json:"user_id"`
	// OrgID is the organisation the user belongs to.
	OrgID string `json:"org_id"`
}

// Resolver resolves UserEntitlement using a three-layer cache strategy.
// L1: sync.Map (in-process), L2: Redis, L3: Central Server HTTP.
//
// Thread-safety: safe for concurrent use.
type Resolver struct {
	// localCache is the L1 in-process cache.
	localCache *local.Cache
	// redisCache is the L2 Redis cache.
	redisCache *rediscache.Cache
	// serverURL is the base URL of the Central Server, without trailing slash.
	serverURL string
	// apiKey is the bearer token for Central Server authentication.
	apiKey string
	// log is the structured logger.
	log zerolog.Logger
	// httpClient is reused across requests for connection pooling.
	httpClient *http.Client
}

// NewResolver constructs an entitlement Resolver with the given cache and server details.
//
// local is the L1 in-process cache; redisCache is the L2 Redis cache.
// serverURL is the Central Server base URL without a trailing slash.
// apiKey authenticates L3 requests.
func NewResolver(local *local.Cache, redisCache *rediscache.Cache, serverURL, apiKey string, log zerolog.Logger) *Resolver {
	return &Resolver{
		localCache: local,
		redisCache: redisCache,
		serverURL:  serverURL,
		apiKey:     apiKey,
		log:        log.With().Str("component", "entitlement-resolver").Logger(),
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// Resolve returns the UserEntitlement for the given user. It consults cache layers
// from fastest to slowest, writing through to faster layers on an L3 fetch.
//
// Returns ErrEntitlementDenied if the Central Server explicitly denies the user.
// Returns a wrapped error on network or decode failures.
func (r *Resolver) Resolve(ctx context.Context, userID, orgID string) (*model.UserEntitlement, error) {
	// L1: in-process sync.Map lookup.
	if ent, ok := r.localCache.Get(userID); ok {
		r.log.Trace().Str("user_id", userID).Str("tier", "L1").Msg("entitlement cache hit")
		return ent, nil
	}

	// L2: Redis lookup.
	ent, err := r.redisCache.Get(ctx, userID)
	if err != nil {
		r.log.Warn().Err(err).Str("user_id", userID).Msg("L2 entitlement lookup failed; falling through to L3")
	}
	if ent != nil {
		// write-through to L1 so subsequent requests skip Redis.
		r.localCache.Set(ent)
		r.log.Trace().Str("user_id", userID).Str("tier", "L2").Msg("entitlement cache hit")
		return ent, nil
	}

	// L3: Central Server HTTP fetch.
	ent, err = r.fetchFromServer(ctx, userID, orgID)
	if err != nil {
		return nil, fmt.Errorf("entitlement.Resolve: L3 fetch: %w", err)
	}

	// write-through to L2 and L1.
	if setErr := r.redisCache.Set(ctx, ent); setErr != nil {
		r.log.Warn().Err(setErr).Str("user_id", userID).Msg("failed to populate L2 cache")
	}
	r.localCache.Set(ent)

	r.log.Debug().Str("user_id", userID).Str("tier", "L3").Msg("entitlement resolved from Central Server")
	return ent, nil
}

// fetchFromServer performs POST /entitlement to resolve the entitlement from the
// Central Server. Returns a descriptive error on HTTP errors or decode failures.
func (r *Resolver) fetchFromServer(ctx context.Context, userID, orgID string) (*model.UserEntitlement, error) {
	body, err := json.Marshal(entitlementRequest{UserID: userID, OrgID: orgID})
	if err != nil {
		return nil, fmt.Errorf("entitlement.fetchFromServer: marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.serverURL+"/entitlement", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("entitlement.fetchFromServer: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+r.apiKey)

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("entitlement.fetchFromServer: http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("entitlement.fetchFromServer: server denied access (HTTP %d)", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("entitlement.fetchFromServer: unexpected status %d", resp.StatusCode)
	}

	var ent model.UserEntitlement
	if err := json.NewDecoder(resp.Body).Decode(&ent); err != nil {
		return nil, fmt.Errorf("entitlement.fetchFromServer: decode: %w", err)
	}

	return &ent, nil
}
