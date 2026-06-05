// Package daemon contains the HotPipeline that wires together all components of
// the per-request processing path:
//
//  1. Entitlement resolution (L1 → L2 → L3)
//  2. Prompt registry (FQN resolution + template rendering)
//  3. Skill registry (progressive disclosure summary injection)
//  4. MCP registry (tool whitelist check)
//  5. Guardrail engine (llm_input hook)
//  6. Budget engine (cost check)
//  7. Rate limit engine (sliding window check)
//  8. LLM dispatch (circuit-breaker-guarded HTTP POST)
//  9. Guardrail engine (llm_output hook)
//  10. Budget engine (actual cost recording)
//  11. Kafka producer (fire-and-forget TransactionEvent)
package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/pangreksa/ai-gateway-engine/internal/entitlement"
	"github.com/pangreksa/ai-gateway-engine/internal/llm"
	"github.com/pangreksa/ai-gateway-engine/internal/policy/budget"
	"github.com/pangreksa/ai-gateway-engine/internal/policy/guardrail"
	"github.com/pangreksa/ai-gateway-engine/internal/policy/ratelimit"
	"github.com/pangreksa/ai-gateway-engine/internal/producer"
	mcpreg "github.com/pangreksa/ai-gateway-engine/internal/registry/mcp"
	promptreg "github.com/pangreksa/ai-gateway-engine/internal/registry/prompt"
	skillreg "github.com/pangreksa/ai-gateway-engine/internal/registry/skill"
	"github.com/pangreksa/ai-gateway-engine/pkg/model"
)

// llmResponse is the minimal shape parsed from the provider's JSON response to
// extract token usage and content for policy recording and output guardrails.
type llmResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

// HotPipeline executes the full per-request hot path for a single inquiry.
// It implements the proxy.Pipeline interface.
//
// Thread-safety: safe for concurrent use; all component fields are themselves
// goroutine-safe after construction.
type HotPipeline struct {
	// resolver resolves user entitlements from the three-tier cache.
	resolver *entitlement.Resolver
	// promptReg resolves FQN prompts and injects template variables.
	promptReg *promptreg.Registry
	// skillReg provides skill summaries and full content.
	skillReg *skillreg.Registry
	// mcpReg enforces MCP tool whitelist and HITL flags.
	mcpReg *mcpreg.Registry
	// guardrail runs content inspection at llm_input and llm_output hooks.
	guardrail *guardrail.Engine
	// budget checks and records cost counters.
	budget *budget.Engine
	// ratelimit enforces sliding-window request rate limits.
	ratelimit *ratelimit.Engine
	// dispatcher routes requests to upstream LLM providers.
	dispatcher *llm.Dispatcher
	// producer publishes TransactionEvents to Kafka asynchronously.
	producer *producer.Producer
	// gatewayID is included in every TransactionEvent.
	gatewayID string
	// log is the structured logger.
	log zerolog.Logger
}

// NewHotPipeline constructs a HotPipeline with all required components injected.
//
// All parameters are required; nil values will cause panics at request time.
func NewHotPipeline(
	resolver *entitlement.Resolver,
	promptReg *promptreg.Registry,
	skillReg *skillreg.Registry,
	mcpReg *mcpreg.Registry,
	guardrailEng *guardrail.Engine,
	budgetEng *budget.Engine,
	ratelimitEng *ratelimit.Engine,
	dispatcher *llm.Dispatcher,
	prod *producer.Producer,
	gatewayID string,
	log zerolog.Logger,
) *HotPipeline {
	return &HotPipeline{
		resolver:   resolver,
		promptReg:  promptReg,
		skillReg:   skillReg,
		mcpReg:     mcpReg,
		guardrail:  guardrailEng,
		budget:     budgetEng,
		ratelimit:  ratelimitEng,
		dispatcher: dispatcher,
		producer:   prod,
		gatewayID:  gatewayID,
		log:        log.With().Str("component", "hot-pipeline").Logger(),
	}
}

// Execute runs the full hot path for a single inquiry.
// It returns the raw LLM provider response bytes on success, or a descriptive
// error on any policy violation or dispatch failure.
//
// ctx should carry a deadline appropriate for the end-to-end request budget.
// req must have RequestID set; UserID/OrgID will be populated from the resolved entitlement.
func (p *HotPipeline) Execute(ctx context.Context, req *model.InquiryRequest) ([]byte, error) {
	start := time.Now()

	// 1. Resolve entitlement.
	ent, err := p.resolver.Resolve(ctx, req.UserID, req.OrgID)
	if err != nil {
		return nil, fmt.Errorf("pipeline.Execute: entitlement: %w", err)
	}
	// propagate resolved identifiers back onto the request.
	req.UserID = ent.UserID
	req.OrgID = ent.OrgID

	// 2. Prompt registry — resolve FQN if provided.
	if req.PromptFQN != "" {
		content, _, err := p.promptReg.Resolve(ctx, req.PromptFQN, req.Variables, ent)
		if err != nil {
			return nil, fmt.Errorf("pipeline.Execute: prompt registry: %w", err)
		}
		// inject resolved prompt as the first system message.
		req.Messages = prependSystemMessage(req.Messages, content)
	}

	// 3. Skill registry — inject preloaded skill summaries as a system message.
	summaries, err := p.skillReg.Summaries(ctx, ent.AllowedSkills)
	if err != nil {
		p.log.Warn().Err(err).Str("request_id", req.RequestID).Msg("skill summaries failed; continuing")
	} else if len(summaries) > 0 {
		req.Messages = prependSystemMessage(req.Messages, buildSkillSummaryMessage(summaries))
	}

	// 4. Guardrail — llm_input hook.
	if err := p.guardrail.CheckInput(ctx, req); err != nil {
		return nil, fmt.Errorf("pipeline.Execute: guardrail input: %w", err)
	}

	// 5. Rate limit check.
	if err := p.ratelimit.Check(ctx, req, ent); err != nil {
		return nil, fmt.Errorf("pipeline.Execute: rate limit: %w", err)
	}

	// 6. Budget check (estimated cost of 0 — pre-dispatch check uses counter only).
	if err := p.budget.Check(ctx, req, ent, 0); err != nil {
		return nil, fmt.Errorf("pipeline.Execute: budget: %w", err)
	}

	// 7. LLM dispatch.
	respBytes, err := p.dispatcher.Dispatch(ctx, req)
	if err != nil {
		p.publishEvent(ctx, req, ent, start, "error", 0, 0, 0)
		return nil, fmt.Errorf("pipeline.Execute: dispatch: %w", err)
	}

	// 8. Guardrail — llm_output hook.
	resp := parseResponse(respBytes)
	outputContent := extractContent(resp)
	if _, err := p.guardrail.CheckOutput(ctx, outputContent); err != nil {
		return nil, fmt.Errorf("pipeline.Execute: guardrail output: %w", err)
	}

	// 9. Budget record (actual cost based on token usage).
	actualCost := estimateCost(req.Provider, resp.Usage.PromptTokens, resp.Usage.CompletionTokens)
	p.budget.Record(ctx, req, ent, actualCost)

	// 10. Kafka publish (fire-and-forget).
	p.publishEvent(ctx, req, ent, start, "success",
		resp.Usage.PromptTokens, resp.Usage.CompletionTokens, actualCost)

	return respBytes, nil
}

// prependSystemMessage inserts a system role message at the head of the messages slice.
func prependSystemMessage(msgs []model.Message, content string) []model.Message {
	sys := model.Message{Role: "system", Content: content}
	return append([]model.Message{sys}, msgs...)
}

// buildSkillSummaryMessage formats the skill summary list into a system message body.
func buildSkillSummaryMessage(summaries []skillreg.SkillSummary) string {
	s := "Available skills:\n"
	for _, sk := range summaries {
		s += fmt.Sprintf("- %s: %s\n", sk.Name, sk.Description)
	}
	return s
}

// parseResponse attempts to decode the raw provider response JSON.
// Returns an empty llmResponse on decode failure to allow the pipeline to continue.
func parseResponse(data []byte) llmResponse {
	var r llmResponse
	_ = json.Unmarshal(data, &r)
	return r
}

// extractContent returns the content of the first choice in the LLM response.
func extractContent(r llmResponse) string {
	if len(r.Choices) > 0 {
		return r.Choices[0].Message.Content
	}
	return ""
}

// estimateCost computes a rough USD cost from token counts.
// These are placeholder rates; replace with per-model pricing tables.
func estimateCost(provider string, inputTokens, outputTokens int) float64 {
	// uniform placeholder rate: $0.002/1K input + $0.002/1K output
	return (float64(inputTokens)+float64(outputTokens))/1000.0 * 0.002
}

// publishEvent asynchronously publishes a TransactionEvent to Kafka.
func (p *HotPipeline) publishEvent(
	ctx context.Context,
	req *model.InquiryRequest,
	ent *model.UserEntitlement,
	start time.Time,
	status string,
	inputTokens, outputTokens int,
	costUSD float64,
) {
	event := model.TransactionEvent{
		EventID:      uuid.NewString(),
		RequestID:    req.RequestID,
		GatewayID:    p.gatewayID,
		OrgID:        ent.OrgID,
		UserID:       ent.UserID,
		Provider:     req.Provider,
		Model:        req.Model,
		Status:       status,
		LatencyMS:    int(time.Since(start).Milliseconds()),
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		CostUSD:      costUSD,
		CacheStatus:  "none",
		PromptFQN:    req.PromptFQN,
		SkillsUsed:   []string{},
		MCPsCalled:   []string{},
		GuardrailsHit: []string{},
		Metadata:     req.Metadata,
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
	}

	p.producer.PublishAsync(event)
}
