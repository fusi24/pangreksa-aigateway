// Package invalidation provides a Redis pub/sub subscriber that listens on the
// "user:invalidate" channel. When a message is received, the named user's
// entitlement entry is evicted from both L1 (sync.Map) and L2 (Redis) caches so
// that the next request triggers a fresh L3 fetch from the Central Server.
package invalidation

import (
	"context"
	"fmt"

	"github.com/rs/zerolog"

	"github.com/pangreksa/ai-gateway-engine/internal/cache/local"
	rediscache "github.com/pangreksa/ai-gateway-engine/internal/cache/redis"
)

const (
	// invalidateChannel is the Redis pub/sub channel the Central Server publishes
	// userID strings to when an entitlement must be evicted.
	invalidateChannel = "user:invalidate"
)

// Subscriber listens on the Redis "user:invalidate" pub/sub channel and evicts
// matching entries from both L1 and L2 caches on each message.
//
// Thread-safety: Run may only be called once per Subscriber instance.
type Subscriber struct {
	// redisCache is the L2 Redis cache used for remote eviction.
	redisCache *rediscache.Cache
	// localCache is the L1 in-process cache used for local eviction.
	localCache *local.Cache
	// log is the structured logger for invalidation events.
	log zerolog.Logger
}

// NewSubscriber constructs an invalidation Subscriber.
//
// redisCache is used both to subscribe to the pub/sub channel and to delete the
// L2 entitlement key. localCache provides the L1 in-process eviction.
func NewSubscriber(redisCache *rediscache.Cache, localCache *local.Cache, log zerolog.Logger) *Subscriber {
	return &Subscriber{
		redisCache: redisCache,
		localCache: localCache,
		log:        log.With().Str("component", "invalidation-sub").Logger(),
	}
}

// Run subscribes to Redis "user:invalidate" and evicts the named user's entries
// from both L1 and L2 on each message. Blocks until ctx is cancelled.
//
// Returns a non-nil error only if the internal subscription channel closes
// unexpectedly; ctx cancellation returns ctx.Err().
func (s *Subscriber) Run(ctx context.Context) error {
	s.log.Info().Str("channel", invalidateChannel).Msg("invalidation subscriber starting")

	msgCh := s.redisCache.Subscribe(ctx, invalidateChannel)

	for {
		select {
		case <-ctx.Done():
			s.log.Info().Msg("invalidation subscriber stopping")
			return ctx.Err()
		case userID, ok := <-msgCh:
			if !ok {
				return fmt.Errorf("invalidation.Run: subscription channel closed unexpectedly")
			}
			s.evict(ctx, userID)
		}
	}
}

// evict removes the entitlement for userID from both cache layers.
func (s *Subscriber) evict(ctx context.Context, userID string) {
	// L1 eviction is synchronous and cheap.
	s.localCache.Delete(userID)

	// L2 eviction; log but don't fail on Redis error.
	if err := s.redisCache.Delete(ctx, userID); err != nil {
		s.log.Warn().Err(err).Str("user_id", userID).Msg("failed to evict L2 entry")
		return
	}

	s.log.Debug().Str("user_id", userID).Msg("evicted entitlement from L1 and L2")
}
