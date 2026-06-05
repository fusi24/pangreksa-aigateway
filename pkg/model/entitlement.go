package model

import "time"

// UserEntitlement holds the resolved access rights for a single user.
// It is the authoritative record of what a user may do within the gateway.
//
// Caching strategy (three-tier):
//   - L1: sync.Map in-process cache (TTL: process-local, invalidated via Redis pub/sub)
//   - L2: Redis key with TTL 60s
//   - L3: Central Server POST /entitlement on cache miss
//
// Thread-safety: safe for concurrent read access. Mutations are handled by the
// entitlement resolver which replaces the entire struct atomically.
type UserEntitlement struct {
	// UserID is the unique identifier of the user (UUID string).
	UserID string `json:"user_id"`
	// OrgID is the organization the user belongs to.
	OrgID string `json:"org_id"`
	// AllowedPrompts is the list of FQN glob patterns the user may invoke.
	// Example: "sales/*" permits all prompts under the sales/ namespace.
	AllowedPrompts []string `json:"allowed_prompts"`
	// AllowedSkills is the list of skill name patterns the user may use.
	AllowedSkills []string `json:"allowed_skills"`
	// AllowedMCPs is the list of MCP server name patterns the user may call.
	AllowedMCPs []string `json:"allowed_mcps"`
	// Permissions is the set of RBAC token strings granted to this user.
	// Example: "gateway.mcp.read", "gateway.prompt.write".
	Permissions []string `json:"permissions"`
	// BudgetLimitUSD is the maximum spend in USD allowed for this user.
	// A value of 0 means no limit is enforced.
	BudgetLimitUSD float64 `json:"budget_limit_usd"`
	// RateLimitRPM is the maximum number of requests per minute.
	RateLimitRPM int `json:"rate_limit_rpm"`
	// RateLimitTPM is the maximum number of tokens per minute.
	RateLimitTPM int `json:"rate_limit_tpm"`
	// DataScope controls which data the user's agent may access.
	// Valid values: own_data | team_data | all_data.
	DataScope string `json:"data_scope"`
	// ExpiresAt is the time after which this entitlement record must be re-fetched
	// from the Central Server, even if the Redis TTL has not expired.
	ExpiresAt time.Time `json:"expires_at"`
}
