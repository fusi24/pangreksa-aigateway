// Package guardrail implements the four-hook guardrail engine:
//
//	llm_input  — runs before the request is forwarded to the LLM provider
//	llm_output — runs after the LLM response is received
//	mcp_pre    — runs before an MCP tool is invoked
//	mcp_post   — runs after an MCP tool returns its result
//
// Enforcement modes:
//
//	enforce  — violations block / mutate the request immediately
//	audit    — violations are logged only; request proceeds unmodified
//	degrade  — enforce when the checker is available; pass-through otherwise
//
// Operation modes:
//
//	validate — inspect and report without modifying content
//	mutate   — rewrite or redact content and continue
//	block    — reject the request/response with an error
//
// The stub checker always passes. Replace stubCheck with real implementations
// (regex, PII, LLM-based classification, etc.) without changing the interface.
package guardrail

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/rs/zerolog"

	"github.com/pangreksa/ai-gateway-engine/pkg/model"
)

// ErrBlocked is returned when a guardrail with mode=block triggers in enforce mode.
var ErrBlocked = fmt.Errorf("request blocked by guardrail")

// Engine runs guardrail checks across all 4 hooks.
//
// Thread-safety: safe for concurrent use; hot-reload via Update uses a write lock.
type Engine struct {
	// mu guards the rules slice during hot-reload.
	mu sync.RWMutex
	// rules is the ordered slice of active guardrail rules.
	rules []model.GuardrailConfig
	// log is the structured logger.
	log zerolog.Logger
}

// NewEngine constructs a guardrail Engine with an empty rule set.
// Call Update with the initial rules before serving requests.
func NewEngine(log zerolog.Logger) *Engine {
	return &Engine{
		log: log.With().Str("component", "guardrail-engine").Logger(),
	}
}

// Update hot-reloads the active guardrail rules. Safe to call concurrently with
// the Check* methods; in-flight checks complete against the old snapshot.
func (e *Engine) Update(rules []model.GuardrailConfig) {
	snapshot := make([]model.GuardrailConfig, len(rules))
	copy(snapshot, rules)

	e.mu.Lock()
	e.rules = snapshot
	e.mu.Unlock()

	e.log.Info().Int("count", len(snapshot)).Msg("guardrail rules updated")
}

// CheckInput runs llm_input_guardrails on the inquiry content.
// Returns ErrBlocked when a blocking guardrail triggers in enforce mode.
func (e *Engine) CheckInput(ctx context.Context, req *model.InquiryRequest) error {
	return e.runHook(ctx, "llm_input", contentFromMessages(req.Messages), false)
}

// CheckOutput runs llm_output_guardrails on the LLM response.
// Returns the (possibly mutated) content and ErrBlocked when applicable.
func (e *Engine) CheckOutput(ctx context.Context, content string) (string, error) {
	if err := e.runHook(ctx, "llm_output", content, false); err != nil {
		return "", err
	}
	return content, nil
}

// CheckMCPPre runs mcp_pre guardrail on a tool invocation before it is sent.
// Returns ErrBlocked when a blocking guardrail triggers in enforce mode.
func (e *Engine) CheckMCPPre(ctx context.Context, tool, args string) error {
	payload := fmt.Sprintf("tool=%s args=%s", tool, args)
	return e.runHook(ctx, "mcp_pre", payload, false)
}

// CheckMCPPost runs mcp_post guardrail on a tool result after it is received.
// Returns the (possibly mutated) result and ErrBlocked when applicable.
func (e *Engine) CheckMCPPost(ctx context.Context, result string) (string, error) {
	if err := e.runHook(ctx, "mcp_post", result, false); err != nil {
		return "", err
	}
	return result, nil
}

// runHook evaluates all rules whose Hook matches hookName against content.
// Enforcement logic is applied per the rule's Enforcement and Mode fields.
func (e *Engine) runHook(ctx context.Context, hookName, content string, isMutation bool) error {
	select {
	case <-ctx.Done():
		return fmt.Errorf("guardrail.runHook: %w", ctx.Err())
	default:
	}

	e.mu.RLock()
	rules := e.rules
	e.mu.RUnlock()

	for _, rule := range rules {
		if rule.Hook != hookName {
			continue
		}

		triggered, err := stubCheck(ctx, rule, content)
		if err != nil {
			// checker error: apply degrade semantics — pass through.
			e.log.Warn().
				Err(err).
				Str("rule_id", rule.ID).
				Str("hook", hookName).
				Msg("guardrail checker error; applying degrade pass-through")
			continue
		}

		if !triggered {
			continue
		}

		e.log.Info().
			Str("rule_id", rule.ID).
			Str("hook", hookName).
			Str("mode", rule.Mode).
			Str("enforcement", rule.Enforcement).
			Msg("guardrail triggered")

		switch rule.Enforcement {
		case "audit":
			// log only; do not block or mutate.
			continue
		case "degrade":
			// treat as enforce since the checker ran successfully.
			fallthrough
		case "enforce":
			if rule.Mode == "block" {
				return fmt.Errorf("%w: rule_id=%q hook=%q", ErrBlocked, rule.ID, hookName)
			}
			// validate and mutate modes: continue processing for now.
			// Full mutation logic requires a checker implementation.
		}
	}

	return nil
}

// stubCheck is a placeholder guardrail checker that always reports no violation.
// Replace with real PII, injection, content, or regex checkers per GuardrailConfig.Type.
//
// Returns (triggered bool, err error).
func stubCheck(_ context.Context, _ model.GuardrailConfig, _ string) (bool, error) {
	// TODO: implement per-type checkers:
	//   pii       → scan for PII patterns
	//   injection → detect prompt injection signatures
	//   content   → moderation classification
	//   secrets   → credential/API key pattern scan
	//   sql       → SQL injection heuristics
	//   code      → code safety linting
	//   regex     → rule.Config["pattern"] regexp match
	return false, nil
}

// contentFromMessages concatenates message contents for input guardrail scanning.
func contentFromMessages(msgs []model.Message) string {
	if len(msgs) == 0 {
		return ""
	}
	var sb strings.Builder
	for _, m := range msgs {
		sb.WriteString(m.Content)
		sb.WriteByte('\n')
	}
	return sb.String()
}
