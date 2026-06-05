// Package skill provides the skill registry with progressive disclosure.
//
// Progressive disclosure is a two-step pattern that minimises token usage:
//
//  1. Step 1 — the LLM receives a compact SkillSummary list (name + description only)
//     so it can decide which skill to invoke without loading full SKILL.md content.
//  2. Step 2 — once the LLM selects a skill, the full SKILL.md Content is injected
//     into the context as a system message.
//
// Skills marked Preload:true skip step 1 and have their content injected eagerly.
package skill

import (
	"context"
	"fmt"
	"sync"

	"github.com/rs/zerolog"

	"github.com/pangreksa/ai-gateway-engine/pkg/model"
)

// ErrSkillNotFound is returned when the requested skill name is not in the registry.
var ErrSkillNotFound = fmt.Errorf("skill not found")

// SkillSummary is the compact representation sent to the LLM in step 1.
// It contains only the information needed for the LLM to choose a skill.
type SkillSummary struct {
	// Name is the unique skill identifier.
	Name string `json:"name"`
	// Description is the short human-readable summary (<= 200 characters).
	Description string `json:"description"`
}

// Registry maintains a hot-reloadable catalog of available skills and supports
// progressive disclosure by returning summaries on step 1 and full content on step 2.
//
// Thread-safety: safe for concurrent use; hot-reload via Update uses a write lock.
type Registry struct {
	// mu guards skills during hot-reload.
	mu sync.RWMutex
	// skills maps skill name → SkillConfig.
	skills map[string]model.SkillConfig
	// log is the structured logger.
	log zerolog.Logger
}

// NewRegistry constructs an empty skill Registry.
// Call Update with the initial GatewayConfig before serving requests.
func NewRegistry(log zerolog.Logger) *Registry {
	return &Registry{
		skills: make(map[string]model.SkillConfig),
		log:    log.With().Str("component", "skill-registry").Logger(),
	}
}

// Update hot-reloads the skill catalog from a new GatewayConfig.
// Safe to call concurrently with Summaries and Content.
func (r *Registry) Update(cfg model.GatewayConfig) {
	index := make(map[string]model.SkillConfig, len(cfg.Registry.Skills))
	for _, s := range cfg.Registry.Skills {
		index[s.Name] = s
	}

	r.mu.Lock()
	r.skills = index
	r.mu.Unlock()

	r.log.Info().Int("count", len(index)).Msg("skill registry updated")
}

// Summaries returns the step-1 compact list of all skills the user is entitled to
// access. Preloaded skills are included in the list but their Content will also
// be available via Content without a round-trip.
//
// allowedPatterns is ent.AllowedSkills from the user's entitlement.
func (r *Registry) Summaries(ctx context.Context, allowedPatterns []string) ([]SkillSummary, error) {
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("skill.Summaries: %w", ctx.Err())
	default:
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	var out []SkillSummary
	for name, s := range r.skills {
		if isAllowed(name, allowedPatterns) {
			out = append(out, SkillSummary{Name: s.Name, Description: s.Description})
		}
	}

	return out, nil
}

// Content returns the full SKILL.md body for the named skill (step 2).
// Returns ErrSkillNotFound if the name is not in the registry.
//
// name is the exact skill identifier chosen by the LLM.
// allowedPatterns is ent.AllowedSkills from the user's entitlement.
func (r *Registry) Content(ctx context.Context, name string, allowedPatterns []string) (string, error) {
	select {
	case <-ctx.Done():
		return "", fmt.Errorf("skill.Content: %w", ctx.Err())
	default:
	}

	if !isAllowed(name, allowedPatterns) {
		return "", fmt.Errorf("skill.Content: access denied for skill %q", name)
	}

	r.mu.RLock()
	s, ok := r.skills[name]
	r.mu.RUnlock()

	if !ok {
		return "", fmt.Errorf("skill.Content: %w: %q", ErrSkillNotFound, name)
	}

	return s.Content, nil
}

// PreloadedContent returns the content of all skills that have Preload:true and
// are accessible to the user. Used to eagerly inject frequently-used skills.
func (r *Registry) PreloadedContent(ctx context.Context, allowedPatterns []string) ([]model.SkillConfig, error) {
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("skill.PreloadedContent: %w", ctx.Err())
	default:
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	var out []model.SkillConfig
	for name, s := range r.skills {
		if s.Preload && isAllowed(name, allowedPatterns) {
			out = append(out, s)
		}
	}

	return out, nil
}

// isAllowed checks whether skillName matches at least one pattern in allowedPatterns.
// Patterns are simple glob strings matched with a prefix/suffix wildcard check.
func isAllowed(skillName string, allowedPatterns []string) bool {
	for _, p := range allowedPatterns {
		if p == "*" || p == skillName {
			return true
		}
		// handle trailing wildcard: "sales/*" matches "sales/anything"
		if len(p) > 1 && p[len(p)-1] == '*' {
			prefix := p[:len(p)-1]
			if len(skillName) >= len(prefix) && skillName[:len(prefix)] == prefix {
				return true
			}
		}
	}
	return false
}
