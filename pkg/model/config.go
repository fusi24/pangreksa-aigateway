// Package model contains the shared domain types used by both the Gateway Daemon
// and the Central Server. All types in this package are JSON-serializable and
// form the wire contract between the two binaries.
package model

// GatewayConfig is the full configuration delivered from Central Server to Daemon
// via POST /config. The Version field is the SHA-256 digest of the serialized config
// content and is used for change detection on each poll cycle.
type GatewayConfig struct {
	// Version is the SHA-256 hex digest of the config content, used as an ETag.
	Version string `json:"version"`
	// GatewayID is the unique identifier for the daemon instance.
	GatewayID string `json:"gateway_id"`
	// Proxies holds one entry per LLM provider listener.
	Proxies []ProxyConfig `json:"proxies"`
	// Registry holds prompts, skills, and MCP definitions.
	Registry RegistryConfig `json:"registry"`
	// Policies holds guardrails, budget rules, and rate limit rules.
	Policies PolicyConfig `json:"policies"`
}

// ProxyConfig defines one LLM provider proxy listener.
// Each provider runs on a dedicated port so clients can target a specific provider
// simply by choosing the port, without embedding the provider name in the URL.
type ProxyConfig struct {
	// Provider is the LLM provider name: openai | claude | moonshot | ollama.
	Provider string `json:"provider"`
	// Port is the TCP port the daemon listens on for this provider.
	Port int `json:"port"`
	// BaseURL is the upstream LLM provider API base URL.
	BaseURL string `json:"base_url"`
	// APIKeyEnc is the provider API key encrypted with AES-256-GCM.
	// It is decrypted in memory only; never logged or returned to callers.
	APIKeyEnc string `json:"api_key_enc"`
}

// RegistryConfig holds prompt, skill, and MCP definitions loaded from Central Server.
// The registry is refreshed on every successful config poll.
type RegistryConfig struct {
	// Prompts is the list of versioned prompt templates available to this gateway.
	Prompts []PromptConfig `json:"prompts"`
	// Skills is the catalog of SKILL.md entries available to this gateway.
	Skills []SkillConfig `json:"skills"`
	// MCPs is the list of MCP server definitions and their tool whitelists.
	MCPs []MCPConfig `json:"mcps"`
}

// PromptConfig represents a versioned prompt template.
// The FQN uniquely identifies the prompt across all orgs, repos, and versions.
type PromptConfig struct {
	// FQN is the fully-qualified name in the form: chat_prompt:{repo}/{name}:{version}
	FQN string `json:"fqn"`
	// Content is the prompt template body, which may contain {{.Variable}} placeholders.
	Content string `json:"content"`
	// Variables is the list of required template variable names that must be supplied
	// by the caller when rendering this prompt.
	Variables []string `json:"variables"`
}

// SkillConfig holds a SKILL.md catalog entry.
// Only name and description are sent to the LLM upfront (step 1); the full Content
// body is injected only after the LLM selects the skill (step 2), saving tokens.
type SkillConfig struct {
	// Name is the unique skill identifier within the org.
	Name string `json:"name"`
	// Description is a short human-readable summary shown to the LLM.
	// Must be <= 200 characters.
	Description string `json:"description"`
	// Content is the full SKILL.md body injected only after skill selection.
	Content string `json:"content"`
	// Preload controls whether the full Content is eagerly loaded into the LLM context.
	// Set to true only for skills used in nearly every request to avoid the round-trip.
	Preload bool `json:"preload"`
}

// MCPConfig defines an MCP server and its tool whitelist.
// Only tools explicitly listed are callable; all others are denied.
type MCPConfig struct {
	// Name is the unique MCP server identifier within the org.
	Name string `json:"name"`
	// URL is the MCP server endpoint (HTTP or WebSocket).
	URL string `json:"url"`
	// Tools is the whitelist of callable tools on this MCP server.
	Tools []MCPTool `json:"tools"`
}

// MCPTool is a single tool entry in an MCP server's whitelist.
type MCPTool struct {
	// Name is the tool identifier as exposed by the MCP server.
	Name string `json:"name"`
	// Destructive indicates that this tool makes irreversible changes
	// (e.g., delete file, create PR). Destructive tools require HITL approval.
	Destructive bool `json:"destructive"`
	// Description is shown to the LLM to help it decide when to invoke the tool.
	Description string `json:"description"`
}

// PolicyConfig bundles all policy rules delivered with the gateway config.
// Rules are evaluated in priority order; the first matching rule controls behavior.
type PolicyConfig struct {
	// Guardrails is the ordered list of content inspection / mutation rules.
	Guardrails []GuardrailConfig `json:"guardrails"`
	// BudgetRules is the ordered list of cost budget enforcement rules.
	BudgetRules []BudgetRule `json:"budget_rules"`
	// RateRules is the ordered list of request rate enforcement rules.
	RateRules []RateLimitRule `json:"rate_rules"`
}
