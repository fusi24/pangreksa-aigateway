// Package budget implements ordered budget policy evaluation using Redis counters.
// Rules are evaluated top-to-bottom; the first matching rule controls allow/block.
// All matching rules (not just the first) are tracked via Redis INCRBYFLOAT for
// analytics even when they are not the controlling rule.
//
// Redis key format: budget:{rule_id}:{entity}:{period}
// where entity is org_id or user_id and period is the current day/month string.
package budget

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

// ErrBudgetExceeded is returned when the first-match budget rule's limit is reached.
var ErrBudgetExceeded = fmt.Errorf("budget limit exceeded")

// parsedRule is a budget rule after YAML parsing and priority sorting.
type parsedRule struct {
	// raw is the original rule delivered via GatewayConfig.
	raw model.BudgetRule
	// limitUSD is the maximum cost allowed for the period extracted from RuleYAML.
	limitUSD float64
	// period is the tracking window: "daily" or "monthly".
	period string
	// entityType controls which identifier is used as the Redis key entity: "user" or "org".
	entityType string
}

// Engine evaluates ordered budget rules against Redis counters.
//
// Thread-safety: safe for concurrent use; hot-reload via Update uses a write lock.
type Engine struct {
	// mu guards the rules slice during hot-reload.
	mu sync.RWMutex
	// rules is the priority-sorted slice of parsed budget rules.
	rules []parsedRule
	// redis is the cache used for INCRBYFLOAT operations.
	redis *rediscache.Cache
	// log is the structured logger.
	log zerolog.Logger
}

// NewEngine constructs a budget Engine with an empty rule set.
// Call Update with the initial rules before serving requests.
func NewEngine(redisCache *rediscache.Cache, log zerolog.Logger) *Engine {
	return &Engine{
		redis: redisCache,
		log:   log.With().Str("component", "budget-engine").Logger(),
	}
}

// Update hot-reloads the ordered budget rules from the latest GatewayConfig.
// Rules are sorted by Priority (ascending) so lower values are evaluated first.
func (e *Engine) Update(rules []model.BudgetRule) {
	parsed := make([]parsedRule, 0, len(rules))
	for _, r := range rules {
		pr, err := parseRule(r)
		if err != nil {
			e.log.Warn().Err(err).Str("rule_id", r.ID).Msg("failed to parse budget rule; skipping")
			continue
		}
		parsed = append(parsed, pr)
	}

	// stable sort by priority ascending.
	sort.SliceStable(parsed, func(i, j int) bool {
		return parsed[i].raw.Priority < parsed[j].raw.Priority
	})

	e.mu.Lock()
	e.rules = parsed
	e.mu.Unlock()

	e.log.Info().Int("count", len(parsed)).Msg("budget rules updated")
}

// Check evaluates the budget rules for the request. The first matching rule
// whose counter has reached its limit returns ErrBudgetExceeded. All matching
// rules are tracked even when not the first match.
//
// estimatedCostUSD is the pre-dispatch cost estimate; actual cost is recorded
// via Record after the LLM responds.
func (e *Engine) Check(ctx context.Context, req *model.InquiryRequest, ent *model.UserEntitlement, estimatedCostUSD float64) error {
	e.mu.RLock()
	rules := e.rules
	e.mu.RUnlock()

	firstMatch := true
	for _, rule := range rules {
		if !ruleMatches(rule, req, ent) {
			continue
		}

		key := budgetKey(rule, req, ent)
		current, err := e.redis.IncrByFloat(ctx, key, 0, periodTTL(rule.period))
		if err != nil {
			e.log.Warn().Err(err).Str("rule_id", rule.raw.ID).Msg("budget check: redis read failed; skipping rule")
			firstMatch = false
			continue
		}

		if firstMatch {
			firstMatch = false
			if current+estimatedCostUSD > rule.limitUSD {
				e.log.Warn().
					Str("rule_id", rule.raw.ID).
					Float64("current_usd", current).
					Float64("estimated_usd", estimatedCostUSD).
					Float64("limit_usd", rule.limitUSD).
					Msg("budget limit exceeded")
				return fmt.Errorf("%w: rule_id=%q current=%.4f limit=%.4f",
					ErrBudgetExceeded, rule.raw.ID, current, rule.limitUSD)
			}
		}
	}

	return nil
}

// Record atomically increments all matching budget counters by the actual cost.
// It is called after the LLM responds with actual token counts.
func (e *Engine) Record(ctx context.Context, req *model.InquiryRequest, ent *model.UserEntitlement, actualCostUSD float64) {
	e.mu.RLock()
	rules := e.rules
	e.mu.RUnlock()

	for _, rule := range rules {
		if !ruleMatches(rule, req, ent) {
			continue
		}

		key := budgetKey(rule, req, ent)
		if _, err := e.redis.IncrByFloat(ctx, key, actualCostUSD, periodTTL(rule.period)); err != nil {
			e.log.Warn().Err(err).Str("rule_id", rule.raw.ID).Msg("budget record: redis increment failed")
		}
	}
}

// parseRule extracts the limit, period, and entity type from a BudgetRule.
// The RuleYAML is expected to contain simple key:value pairs on separate lines.
// Supported keys: limit_usd, period (daily|monthly), entity (user|org).
func parseRule(r model.BudgetRule) (parsedRule, error) {
	pr := parsedRule{
		raw:        r,
		period:     "daily",
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
		case "limit_usd":
			v, err := strconv.ParseFloat(val, 64)
			if err != nil {
				return parsedRule{}, fmt.Errorf("budget.parseRule: rule_id=%q invalid limit_usd=%q: %w", r.ID, val, err)
			}
			pr.limitUSD = v
		case "period":
			pr.period = val
		case "entity":
			pr.entityType = val
		}
	}

	if pr.limitUSD <= 0 {
		return parsedRule{}, fmt.Errorf("budget.parseRule: rule_id=%q limit_usd must be > 0", r.ID)
	}

	return pr, nil
}

// ruleMatches returns true when the request/entitlement satisfies the rule's conditions.
// Currently all parsed rules match all requests; extend here for user/model/org filters.
func ruleMatches(_ parsedRule, _ *model.InquiryRequest, _ *model.UserEntitlement) bool {
	return true
}

// budgetKey builds the Redis key for a budget counter.
func budgetKey(rule parsedRule, req *model.InquiryRequest, ent *model.UserEntitlement) string {
	entity := ent.UserID
	if rule.entityType == "org" {
		entity = ent.OrgID
	}

	period := currentPeriod(rule.period)
	return fmt.Sprintf("budget:%s:%s:%s", rule.raw.ID, entity, period)
}

// currentPeriod returns a string representing the current period bucket.
func currentPeriod(period string) string {
	now := time.Now().UTC()
	switch period {
	case "monthly":
		return now.Format("2006-01")
	default: // daily
		return now.Format("2006-01-02")
	}
}

// periodTTL returns the Redis TTL for a given period type.
func periodTTL(period string) time.Duration {
	switch period {
	case "monthly":
		return 32 * 24 * time.Hour
	default: // daily
		return 25 * time.Hour
	}
}
