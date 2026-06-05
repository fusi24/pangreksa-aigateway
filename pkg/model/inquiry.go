package model

import "time"

// InquiryRequest is the normalized internal representation of an LLM request
// after proxy adapter normalization. The proxy adapter translates the provider-native
// wire format (OpenAI, Anthropic, etc.) into this unified struct before passing it
// through the entitlement, policy, and dispatch pipeline.
type InquiryRequest struct {
	// RequestID is a UUID assigned by the proxy adapter on ingress.
	// It becomes the X-Gateway-Request-ID response header.
	RequestID string `json:"request_id"`
	// UserID is the resolved user identifier from the bearer token.
	UserID string `json:"user_id"`
	// OrgID is the resolved organization identifier from the bearer token.
	OrgID string `json:"org_id"`
	// Provider identifies the LLM backend: openai | claude | moonshot | ollama.
	Provider string `json:"provider"`
	// Model is the specific model name requested by the client (e.g. "gpt-4o").
	Model string `json:"model"`
	// Messages is the ordered conversation history.
	Messages []Message `json:"messages"`
	// Tools is the list of function-call tools made available to the LLM.
	Tools []Tool `json:"tools"`
	// Stream indicates whether the client wants a streaming (SSE) response.
	Stream bool `json:"stream"`
	// Temperature controls response randomness (0.0–2.0).
	Temperature float64 `json:"temperature"`
	// MaxTokens is the maximum number of completion tokens to generate.
	MaxTokens int `json:"max_tokens"`
	// PromptFQN is the fully-qualified name of the system prompt template, if any.
	// Empty when the client provides its own system message directly.
	PromptFQN string `json:"prompt_fqn,omitempty"`
	// Variables holds template variable substitutions for PromptFQN rendering.
	Variables map[string]string `json:"variables,omitempty"`
	// Metadata carries caller-supplied key-value annotations forwarded to
	// TransactionEvent for downstream analytics and policy matching.
	Metadata map[string]string `json:"metadata,omitempty"`
	// ReceivedAt is the timestamp when the proxy adapter accepted the request.
	// Used to compute total gateway latency for the X-Gateway-Latency-Ms header.
	ReceivedAt time.Time `json:"received_at"`
}

// Message is a single chat message in a conversation.
// The Role field follows the OpenAI convention used across all providers.
type Message struct {
	// Role identifies the speaker: system | user | assistant | tool.
	Role string `json:"role"`
	// Content is the message body.
	Content string `json:"content"`
	// Name is an optional display name for the author of the message,
	// used when Role is "tool" to identify which tool produced the result.
	Name string `json:"name,omitempty"`
}

// Tool represents an LLM-callable tool definition passed to the provider API.
type Tool struct {
	// Type is always "function" for function-call tools per the OpenAI spec.
	Type string `json:"type"`
	// Function holds the name, description, and parameter schema.
	Function ToolFunction `json:"function"`
}

// ToolFunction holds the schema for a callable function.
// The Parameters field accepts any valid JSON Schema object.
type ToolFunction struct {
	// Name is the function identifier the LLM uses to invoke this tool.
	Name string `json:"name"`
	// Description helps the LLM decide when and why to call this function.
	Description string `json:"description"`
	// Parameters is a JSON Schema object describing the function's argument shape.
	Parameters interface{} `json:"parameters"`
}

// TransactionEvent is the Kafka message published to the llm.transactions topic
// after each LLM request completes (success or error). It is published
// fire-and-forget by the daemon; the Central Server consumer persists it to
// request_logs and cost_records.
//
// The EventID field is the idempotency key — the Central Server consumer uses
// ON CONFLICT DO NOTHING so duplicate deliveries are safe.
type TransactionEvent struct {
	// EventID is a UUID used as the Kafka message idempotency key.
	EventID string `json:"event_id"`
	// RequestID matches InquiryRequest.RequestID for cross-system correlation.
	RequestID string `json:"request_id"`
	// GatewayID identifies which daemon instance processed the request.
	GatewayID string `json:"gateway_id"`
	// OrgID is the organization that owns the request.
	OrgID string `json:"org_id"`
	// UserID is the user who made the request.
	UserID string `json:"user_id"`
	// Provider is the LLM provider used: openai | claude | moonshot | ollama.
	Provider string `json:"provider"`
	// Model is the actual model name used (after virtual model resolution).
	Model string `json:"model"`
	// Status is the outcome: success | error.
	Status string `json:"status"`
	// LatencyMS is the total round-trip time in milliseconds, excluding time spent
	// waiting for this event to be published.
	LatencyMS int `json:"latency_ms"`
	// TTFTMS is the time-to-first-token in milliseconds for streaming responses.
	// Zero for non-streaming responses.
	TTFTMS int `json:"ttft_ms"`
	// InputTokens is the number of prompt tokens consumed.
	InputTokens int `json:"input_tokens"`
	// OutputTokens is the number of completion tokens generated.
	OutputTokens int `json:"output_tokens"`
	// CostUSD is the estimated cost of this request in US dollars.
	CostUSD float64 `json:"cost_usd"`
	// CacheStatus indicates whether a cached response was served: hit | miss | none.
	CacheStatus string `json:"cache_status"`
	// PromptFQN is the fully-qualified prompt name if a managed prompt was used.
	PromptFQN string `json:"prompt_fqn,omitempty"`
	// SkillsUsed lists the skill names activated during this request.
	SkillsUsed []string `json:"skills_used"`
	// MCPsCalled lists the MCP server names whose tools were invoked.
	MCPsCalled []string `json:"mcps_called"`
	// GuardrailsHit lists the guardrail IDs that triggered (block, mutate, or audit).
	GuardrailsHit []string `json:"guardrails_hit"`
	// Metadata carries the caller-supplied annotations from InquiryRequest.
	Metadata map[string]string `json:"metadata,omitempty"`
	// CreatedAt is the ISO 8601 timestamp when the event was created.
	CreatedAt string `json:"created_at"`
}
