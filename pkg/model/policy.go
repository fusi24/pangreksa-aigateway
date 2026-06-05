package model

// GuardrailConfig defines a single guardrail rule applied at a specific hook
// position in the request pipeline. Rules are evaluated in the order they appear
// in PolicyConfig.Guardrails.
type GuardrailConfig struct {
	// ID is the unique identifier for this guardrail rule, used in audit logs
	// and the GuardrailsHit field of TransactionEvent.
	ID string `json:"id"`
	// Type classifies the inspection technology:
	//   pii       — personally identifiable information detection
	//   injection — prompt injection detection
	//   content   — content moderation (hate, violence, etc.)
	//   secrets   — API key / credential leakage detection
	//   sql       — SQL injection pattern detection
	//   code      — code safety linting
	//   regex     — custom regular expression matching
	Type string `json:"type"`
	// Hook identifies the pipeline position where this rule executes:
	//   llm_input  — before the request is forwarded to the LLM provider
	//   llm_output — after the LLM response is received
	//   mcp_pre    — before an MCP tool is invoked
	//   mcp_post   — after an MCP tool returns its result
	Hook string `json:"hook"`
	// Mode controls what the guardrail does when it triggers:
	//   validate — inspect content and report a violation without modifying
	//   mutate   — rewrite or redact content and continue the request
	//   block    — reject the request or response with an error
	Mode string `json:"mode"`
	// Enforcement controls whether a violation actually stops the request:
	//   enforce  — violations block / mutate the request immediately
	//   audit    — violations are logged only; request proceeds unmodified
	//   degrade  — enforce if the guardrail provider is available; pass-through otherwise
	Enforcement string `json:"enforcement"`
	// Config holds guardrail-type-specific configuration key-value pairs.
	// Example for regex type: {"pattern": "\\b\\d{16}\\b", "description": "credit card"}
	Config map[string]interface{} `json:"config"`
}

// BudgetRule is a single entry in the ordered budget policy.
// Rules are evaluated top-to-bottom; the first matching rule controls allow/block.
// All matching rules below the first are tracked in Redis for analytics but do not
// gate the request.
type BudgetRule struct {
	// ID uniquely identifies this budget rule for Redis key construction and audit logs.
	ID string `json:"id"`
	// Priority is the sort key; lower values are evaluated first.
	Priority int `json:"priority"`
	// RuleYAML is the YAML-encoded rule body specifying the match condition
	// (user, team, model, etc.) and the limit (cost_per_day, cost_per_month, etc.).
	RuleYAML string `json:"rule_yaml"`
}

// RateLimitRule is a single entry in the ordered rate limit policy.
// Rate limiting uses a sliding window counter implemented via Redis Lua scripts.
// Rules are evaluated top-to-bottom; the first matching rule controls allow/reject.
type RateLimitRule struct {
	// ID uniquely identifies this rate limit rule for Redis key construction.
	ID string `json:"id"`
	// Priority is the sort key; lower values are evaluated first.
	Priority int `json:"priority"`
	// RuleYAML is the YAML-encoded rule body specifying the match condition
	// and the limit (requests_per_minute, tokens_per_minute, etc.).
	RuleYAML string `json:"rule_yaml"`
}
