// Package ratelimit implements sliding-window rate limiting via Redis Lua scripts.
// The window is 60 seconds divided into 12 five-second buckets. Each bucket key
// has a 65-second TTL so stale buckets expire automatically.
//
// Redis key format: ratelimit:{rule_id}:{entity}:{bucket_ts}
package ratelimit

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"

	rediscache "github.com/pangreksa/ai-gateway-engine/internal/cache/redis"
	"github.com/pangreksa/ai-gateway-engine/pkg/model"
)

// ErrRateLimitExceeded is returned when a rate limit rule's threshold is reached.
var ErrRateLimitExceeded = fmt.Errorf("rate limit exceeded")

// slidingWindowLua is the Redis Lua script that increments the current time bucket
// and computes the total request count across all 12 five-second buckets in the
// 60-second sliding window. Returns 1 when under limit, 0 when exceeded.
const slidingWindowLua = `
local key_prefix = KEYS[1]
local now = tonumber(ARGV[1])
local limit = tonumber(ARGV[2])
local bucket_size = 5
local window = 60
local current_bucket = math.floor(now / bucket_size) * bucket_size
redis.call('INCR', key_prefix .. ':' .. current_bucket)
redis.call('EXPIRE', key_prefix .. ':' .. current_bucket, 65)
local total = 0
for i = 0, (window / bucket_size) - 1 do
  local b = current_bucket - (i * bucket_size)
  local val = redis.call('GET', key_prefix .. ':' .. b)
  if val then total = total + tonumber(val) end
end
if total > limit then return 0 end
return 1
`

// parsedRule is a rate limit rule after YAML parsing and priority sorting.
type parsedRule struct {
	// raw is the original RateLimitRule from GatewayConfig.
	raw model.RateLimitRule
	// limitRPM is the maximum requests-per-minute from RuleYAML.
	limitRPM int
	// entityType is "user" or "org"; controls the Redis key entity segment.
	entityType string
}

// Engine evaluates rate limit rules using Redis sliding window counters.
//
// Thread-safety: safe for concurrent use; hot-reload via Update uses a write lock.
type Engine struct {
	// mu guards the rules slice during hot-reload.
	mu sync.RWMutex
	// rules is the priority-sorted parsed rule slice.
	rules []parsedRule
	// redis is the cache used for Lua script execution.
	redis *rediscache.Cache
	// log is the structured logger.
	log zerolog.Logger
}

// NewEngine constructs a rate limit Engine with an empty rule set.
// Call Update with the initial rules before serving requests.
func NewEngine(redisCache *rediscache.Cache, log zerolog.Logger) *Engine {
	return &Engine{
		redis: redisCache,
		log:   log.With().Str("component", "ratelimit-engine").Logger(),
	}
}

// Update hot-reloads the ordered rate limit rules from the latest GatewayConfig.
// Rules are sorted by Priority (ascending).
func (e *Engine) Update(rules []model.RateLimitRule) {
	parsed := make([]parsedRule, 0, len(rules))
	for _, r := range rules {
		pr, err := parseRule(r)
		if err != nil {
			e.log.Warn().Err(err).Str("rule_id", r.ID).Msg("failed to parse rate limit rule; skipping")
			continue
		}
		parsed = append(parsed, pr)
	}

	sort.SliceStable(parsed, func(i, j int) bool {
		return parsed[i].raw.Priority < parsed[j].raw.Priority
	})

	e.mu.Lock()
	e.rules = parsed
	e.mu.Unlock()

	e.log.Info().Int("count", len(parsed)).Msg("rate limit rules updated")
}

// Check evaluates rate limit rules for the request. The first matching rule
// that reports the sliding window is exhausted returns ErrRateLimitExceeded.
//
// The Lua script atomically increments the current bucket and checks the total.
func (e *Engine) Check(ctx context.Context, req *model.InquiryRequest, ent *model.UserEntitlement) error {
	e.mu.RLock()
	rules := e.rules
	e.mu.RUnlock()

	nowSec := time.Now().Unix()

	for _, rule := range rules {
		if !ruleMatches(rule, req, ent) {
			continue
		}

		keyPrefix := rateLimitKeyPrefix(rule, req, ent)
		result, err := e.redis.RunScript(
			ctx,
			slidingWindowLua,
			[]string{keyPrefix},
			strconv.FormatInt(nowSec, 10),
			strconv.Itoa(rule.limitRPM),
		)
		if err != nil {
			e.log.Warn().Err(err).Str("rule_id", rule.raw.ID).Msg("rate limit check: redis script failed; skipping rule")
			continue
		}

		// script returns int64(0) when the limit is exceeded, int64(1) when under.
		allowed, _ := result.(int64)
		if allowed == 0 {
			e.log.Warn().
				Str("rule_id", rule.raw.ID).
				Str("user_id", req.UserID).
				Int("limit_rpm", rule.limitRPM).
				Msg("rate limit exceeded")
			return fmt.Errorf("%w: rule_id=%q limit_rpm=%d", ErrRateLimitExceeded, rule.raw.ID, rule.limitRPM)
		}

		// first matching rule controls; stop evaluating further rules.
		return nil
	}

	return nil
}

// parseRule extracts limit and entity settings from a RateLimitRule's YAML.
// Supported keys: requests_per_minute, entity (user|org).
func parseRule(r model.RateLimitRule) (parsedRule, error) {
	pr := parsedRule{
		raw:        r,
		entityType: "user",
	}

	for _, line := range strings.Split(r.RuleYAML, "\n") {
		parts := strings.SplitN(strings.TrimSpace(line), ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])

		switch key {
		case "requests_per_minute":
			v, err := strconv.Atoi(val)
			if err != nil {
				return parsedRule{}, fmt.Errorf("ratelimit.parseRule: rule_id=%q invalid requests_per_minute=%q: %w", r.ID, val, err)
			}
			pr.limitRPM = v
		case "entity":
			pr.entityType = val
		}
	}

	if pr.limitRPM <= 0 {
		return parsedRule{}, fmt.Errorf("ratelimit.parseRule: rule_id=%q requests_per_minute must be > 0", r.ID)
	}

	return pr, nil
}

// ruleMatches returns true when the request/entitlement satisfies the rule.
// Currently all rules match all requests; extend for per-model/user/org filtering.
func ruleMatches(_ parsedRule, _ *model.InquiryRequest, _ *model.UserEntitlement) bool {
	return true
}

// rateLimitKeyPrefix builds the Redis key prefix for the sliding window buckets.
func rateLimitKeyPrefix(rule parsedRule, req *model.InquiryRequest, ent *model.UserEntitlement) string {
	entity := ent.UserID
	if rule.entityType == "org" {
		entity = req.OrgID
	}
	return fmt.Sprintf("ratelimit:%s:%s", rule.raw.ID, entity)
}
