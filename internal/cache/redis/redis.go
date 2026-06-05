// Package redis provides an L2 entitlement cache backed by Redis. It also
// exposes low-level helpers (IncrByFloat, RunScript, Subscribe, Client) that
// are used by the budget and rate-limit policy engines.
//
// Entitlement entries are stored as JSON under the key user:entitlement:{user_id}
// with a 60-second TTL.
package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"

	"github.com/pangreksa/ai-gateway-engine/pkg/model"
)

const (
	// entitlementTTL is the Redis TTL applied to every entitlement key.
	entitlementTTL = 60 * time.Second
	// entitlementKeyPrefix is the Redis key prefix for entitlement records.
	entitlementKeyPrefix = "user:entitlement:"
)

// Cache provides L2 entitlement caching backed by Redis.
//
// Thread-safety: safe for concurrent use; the underlying go-redis client is
// itself goroutine-safe.
type Cache struct {
	// client is the Redis client used for all operations.
	client *goredis.Client
	// log is the structured logger for cache operations.
	log zerolog.Logger
}

// NewCache constructs a Cache from a Redis URL and verifies connectivity with a PING.
// Returns an error if the URL cannot be parsed or the initial PING fails.
//
// redisURL accepts the standard redis:// or rediss:// scheme
// (e.g. "redis://:password@192.168.32.161:6379/0").
func NewCache(redisURL string, log zerolog.Logger) (*Cache, error) {
	opts, err := goredis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("redis.NewCache: parse URL: %w", err)
	}

	client := goredis.NewClient(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("redis.NewCache: ping: %w", err)
	}

	return &Cache{
		client: client,
		log:    log.With().Str("component", "redis-cache").Logger(),
	}, nil
}

// Get retrieves the UserEntitlement for userID from Redis.
// Returns (nil, nil) when the key does not exist (cache miss).
// Returns a non-nil error only on a Redis communication failure.
func (c *Cache) Get(ctx context.Context, userID string) (*model.UserEntitlement, error) {
	raw, err := c.client.Get(ctx, entitlementKeyPrefix+userID).Bytes()
	if err == goredis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("redis.Cache.Get: %w", err)
	}

	var ent model.UserEntitlement
	if err := json.Unmarshal(raw, &ent); err != nil {
		return nil, fmt.Errorf("redis.Cache.Get: unmarshal: %w", err)
	}

	return &ent, nil
}

// Set stores ent in Redis with a 60-second TTL.
// Returns a non-nil error only on a Redis communication failure.
func (c *Cache) Set(ctx context.Context, ent *model.UserEntitlement) error {
	data, err := json.Marshal(ent)
	if err != nil {
		return fmt.Errorf("redis.Cache.Set: marshal: %w", err)
	}

	if err := c.client.Set(ctx, entitlementKeyPrefix+ent.UserID, data, entitlementTTL).Err(); err != nil {
		return fmt.Errorf("redis.Cache.Set: %w", err)
	}

	return nil
}

// Delete removes the entitlement for userID from Redis. It is a no-op if the
// key does not exist.
func (c *Cache) Delete(ctx context.Context, userID string) error {
	if err := c.client.Del(ctx, entitlementKeyPrefix+userID).Err(); err != nil {
		return fmt.Errorf("redis.Cache.Delete: %w", err)
	}
	return nil
}

// Close flushes pending commands and closes the Redis client connection.
func (c *Cache) Close() error {
	if err := c.client.Close(); err != nil {
		return fmt.Errorf("redis.Cache.Close: %w", err)
	}
	return nil
}

// IncrByFloat atomically increments the float64 value stored at key by amount.
// If the key does not exist it is created with value amount. The key TTL is set
// (or refreshed) to ttl after every increment.
//
// Returns the new value after the increment.
func (c *Cache) IncrByFloat(ctx context.Context, key string, amount float64, ttl time.Duration) (float64, error) {
	val, err := c.client.IncrByFloat(ctx, key, amount).Result()
	if err != nil {
		return 0, fmt.Errorf("redis.Cache.IncrByFloat: %w", err)
	}

	// refresh TTL so counters expire even when not incremented for a while.
	if err := c.client.Expire(ctx, key, ttl).Err(); err != nil {
		c.log.Warn().Err(err).Str("key", key).Msg("IncrByFloat: failed to refresh TTL")
	}

	return val, nil
}

// RunScript executes a Lua script via EVAL. keys and args follow the standard
// Redis EVAL convention. Returns the raw Redis reply which callers must type-assert.
func (c *Cache) RunScript(ctx context.Context, script string, keys []string, args ...interface{}) (interface{}, error) {
	val, err := c.client.Eval(ctx, script, keys, args...).Result()
	if err != nil {
		return nil, fmt.Errorf("redis.Cache.RunScript: %w", err)
	}
	return val, nil
}

// Subscribe subscribes to a Redis pub/sub channel and returns a string channel
// that emits each published message payload. The returned channel is closed when
// ctx is cancelled or the subscription fails.
func (c *Cache) Subscribe(ctx context.Context, channel string) <-chan string {
	out := make(chan string, 64)

	go func() {
		defer close(out)
		defer func() {
			if r := recover(); r != nil {
				c.log.Error().Interface("panic", r).Msg("Subscribe: recovered from panic")
			}
		}()

		pubsub := c.client.Subscribe(ctx, channel)
		defer pubsub.Close()

		ch := pubsub.Channel()
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-ch:
				if !ok {
					return
				}
				select {
				case out <- msg.Payload:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return out
}

// Client returns the underlying go-redis Client for callers that need direct access.
// Use sparingly — prefer the typed methods above for common operations.
func (c *Cache) Client() *goredis.Client {
	return c.client
}
