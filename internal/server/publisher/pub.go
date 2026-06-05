// Package publisher provides a Redis pub/sub publisher used to broadcast
// user entitlement invalidation signals to all subscribed Gateway Daemon instances.
// When an admin updates a user's entitlement, a message is published to the
// "user:invalidate" channel so every daemon can immediately evict that user's
// cached entitlement from its L1 and L2 caches.
package publisher

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
)

const (
	// invalidateChannel is the Redis pub/sub channel all daemon instances subscribe to.
	invalidateChannel = "user:invalidate"
)

// Publisher sends cache invalidation messages to the Redis pub/sub channel.
// It wraps a redis.Client and provides a domain-specific API for invalidation events.
//
// Thread-safety: safe for concurrent use; redis.Client is goroutine-safe.
type Publisher struct {
	// client is the underlying Redis connection used for publish operations.
	client *redis.Client
	// log is the structured logger for publish events and errors.
	log zerolog.Logger
}

// NewPublisher constructs a Publisher by parsing redisURL and establishing a
// Redis connection. It verifies connectivity with a PING before returning.
//
// redisURL must be a valid Redis URL, e.g. "redis://192.168.32.161:6379".
// Returns an error if the URL is invalid or Redis is unreachable.
func NewPublisher(redisURL string, log zerolog.Logger) (*Publisher, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("publisher.NewPublisher: parse Redis URL: %w", err)
	}

	client := redis.NewClient(opts)

	// verify connectivity with a background PING.
	if err = client.Ping(context.Background()).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("publisher.NewPublisher: ping Redis: %w", err)
	}

	return &Publisher{
		client: client,
		log:    log.With().Str("component", "publisher").Logger(),
	}, nil
}

// InvalidateUser publishes the given userID to the "user:invalidate" Redis channel.
// All daemon instances subscribed to this channel will evict the user's entitlement
// from their L1 (sync.Map) and L2 (Redis) caches on receipt.
//
// ctx controls deadline and cancellation for the Redis PUBLISH command.
// Returns an error if the publish operation fails.
func (p *Publisher) InvalidateUser(ctx context.Context, userID string) error {
	if err := p.client.Publish(ctx, invalidateChannel, userID).Err(); err != nil {
		return fmt.Errorf("Publisher.InvalidateUser: publish to %s: %w", invalidateChannel, err)
	}

	p.log.Info().
		Str("channel", invalidateChannel).
		Str("user_id", userID).
		Msg("invalidation published")

	return nil
}

// Close shuts down the underlying Redis client connection.
// It is safe to call Close after the publisher is no longer needed.
func (p *Publisher) Close() error {
	if err := p.client.Close(); err != nil {
		return fmt.Errorf("Publisher.Close: %w", err)
	}
	return nil
}
