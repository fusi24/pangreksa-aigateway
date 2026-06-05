// Package prompt implements the prompt registry that resolves FQNs from the
// locally-cached GatewayConfig. Template variable injection uses {{var_name}}
// syntax and missing required variables cause an error. Guardrail configs from
// the matching PromptConfig are returned alongside the rendered content.
package prompt

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/rs/zerolog"

	"github.com/pangreksa/ai-gateway-engine/pkg/model"
)

// ErrPromptNotFound is returned when the requested FQN does not exist in the registry.
var ErrPromptNotFound = fmt.Errorf("prompt not found")

// ErrEntitlementDenied is returned when the user's entitlement does not allow
// access to the requested prompt FQN.
var ErrEntitlementDenied = fmt.Errorf("prompt access denied by entitlement")

// ErrMissingVariable is returned when a required template variable is absent.
var ErrMissingVariable = fmt.Errorf("missing required template variable")

// Registry resolves prompt FQNs from the local GatewayConfig cache.
// It supports FQNs in the form: chat_prompt:{repo}/{name}:{version}
//
// Thread-safety: safe for concurrent use; hot-reload via Update uses a write lock.
type Registry struct {
	// mu guards the prompts map during hot-reload.
	mu sync.RWMutex
	// prompts maps FQN → PromptConfig for O(1) lookup.
	prompts map[string]model.PromptConfig
	// log is the structured logger.
	log zerolog.Logger
}

// NewRegistry constructs an empty prompt Registry.
// Call Update with the initial GatewayConfig before serving requests.
func NewRegistry(log zerolog.Logger) *Registry {
	return &Registry{
		prompts: make(map[string]model.PromptConfig),
		log:     log.With().Str("component", "prompt-registry").Logger(),
	}
}

// Update hot-reloads the prompt registry from a new GatewayConfig.
// It is safe to call concurrently with Resolve; existing in-flight resolutions
// complete against the old snapshot before the new one takes effect.
func (r *Registry) Update(cfg model.GatewayConfig) {
	index := make(map[string]model.PromptConfig, len(cfg.Registry.Prompts))
	for _, p := range cfg.Registry.Prompts {
		index[p.FQN] = p
	}

	r.mu.Lock()
	r.prompts = index
	r.mu.Unlock()

	r.log.Info().Int("count", len(index)).Msg("prompt registry updated")
}

// Resolve validates entitlement, resolves the FQN, injects template variables,
// and returns the rendered prompt content together with the guardrail configs
// associated with the prompt's PromptConfig (currently empty — guardrails are
// delivered via PolicyConfig; this slice is reserved for future per-prompt rules).
//
// fqn must follow the format: chat_prompt:{repo}/{name}:{version}
// vars provides the values for {{var_name}} placeholders in the template.
// ent is the caller's entitlement; access is denied if no pattern in
// ent.AllowedPrompts matches the fqn.
func (r *Registry) Resolve(ctx context.Context, fqn string, vars map[string]string, ent *model.UserEntitlement) (string, []model.GuardrailConfig, error) {
	select {
	case <-ctx.Done():
		return "", nil, fmt.Errorf("prompt.Resolve: %w", ctx.Err())
	default:
	}

	if err := checkEntitlement(fqn, ent.AllowedPrompts); err != nil {
		return "", nil, fmt.Errorf("prompt.Resolve: %w", err)
	}

	r.mu.RLock()
	pc, ok := r.prompts[fqn]
	r.mu.RUnlock()

	if !ok {
		return "", nil, fmt.Errorf("prompt.Resolve: fqn=%q: %w", fqn, ErrPromptNotFound)
	}

	content, err := renderTemplate(pc.Content, pc.Variables, vars)
	if err != nil {
		return "", nil, fmt.Errorf("prompt.Resolve: render: %w", err)
	}

	// guardrail union: per-prompt guardrail configs are reserved for future use;
	// policy-level guardrails are applied by the guardrail engine separately.
	return content, nil, nil
}

// checkEntitlement verifies that at least one pattern in allowedPatterns matches
// the given fqn using filepath.Match glob semantics.
func checkEntitlement(fqn string, allowedPatterns []string) error {
	for _, pattern := range allowedPatterns {
		matched, err := filepath.Match(pattern, fqn)
		if err != nil {
			continue // invalid pattern; skip
		}
		if matched {
			return nil
		}
	}
	return fmt.Errorf("%w: fqn=%q not in allowed patterns", ErrEntitlementDenied, fqn)
}

// renderTemplate substitutes {{var_name}} placeholders in tmpl with the values
// from vars. Returns ErrMissingVariable if a required variable has no value.
func renderTemplate(tmpl string, required []string, vars map[string]string) (string, error) {
	for _, name := range required {
		if _, ok := vars[name]; !ok {
			return "", fmt.Errorf("%w: %q", ErrMissingVariable, name)
		}
	}

	result := tmpl
	for k, v := range vars {
		placeholder := "{{" + k + "}}"
		result = strings.ReplaceAll(result, placeholder, v)
	}

	return result, nil
}
