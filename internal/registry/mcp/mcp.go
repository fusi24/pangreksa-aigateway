// Package mcp provides the MCP (Model Context Protocol) registry. It enforces a
// per-tool whitelist, flags destructive tools as requiring human-in-the-loop (HITL)
// approval, and includes a stub for future Cedar/OPA policy evaluation.
package mcp

import (
	"context"
	"fmt"
	"sync"

	"github.com/rs/zerolog"

	"github.com/pangreksa/ai-gateway-engine/pkg/model"
)

// ErrToolNotWhitelisted is returned when a tool is not in the whitelist for its MCP server.
var ErrToolNotWhitelisted = fmt.Errorf("tool not whitelisted")

// ErrMCPNotFound is returned when the named MCP server is not in the registry.
var ErrMCPNotFound = fmt.Errorf("MCP server not found")

// ErrHITLRequired is returned when a destructive tool requires human-in-the-loop approval.
var ErrHITLRequired = fmt.Errorf("human-in-the-loop approval required for destructive tool")

// AllowResult carries the outcome of an Allow check.
type AllowResult struct {
	// Allowed is true when the tool call is permitted.
	Allowed bool
	// HITLRequired is true when the tool is destructive and requires user confirmation
	// before execution. The caller must gate on HITL approval before invoking the tool.
	HITLRequired bool
	// Tool is the whitelisted MCPTool definition for the requested tool.
	Tool model.MCPTool
}

// Registry maintains the MCP server catalog and enforces tool-level access control.
//
// Thread-safety: safe for concurrent use; hot-reload via Update uses a write lock.
type Registry struct {
	// mu guards the servers map during hot-reload.
	mu sync.RWMutex
	// servers maps MCP server name → MCPConfig.
	servers map[string]model.MCPConfig
	// log is the structured logger.
	log zerolog.Logger
}

// NewRegistry constructs an empty MCP Registry.
// Call Update with the initial GatewayConfig before serving requests.
func NewRegistry(log zerolog.Logger) *Registry {
	return &Registry{
		servers: make(map[string]model.MCPConfig),
		log:     log.With().Str("component", "mcp-registry").Logger(),
	}
}

// Update hot-reloads the MCP registry from a new GatewayConfig.
// Safe to call concurrently with Allow.
func (r *Registry) Update(cfg model.GatewayConfig) {
	index := make(map[string]model.MCPConfig, len(cfg.Registry.MCPs))
	for _, m := range cfg.Registry.MCPs {
		index[m.Name] = m
	}

	r.mu.Lock()
	r.servers = index
	r.mu.Unlock()

	r.log.Info().Int("count", len(index)).Msg("MCP registry updated")
}

// Allow checks whether the named tool on the named MCP server is permitted for
// the given user entitlement. Returns an AllowResult describing the decision.
//
// mcpName is the MCP server name from MCPConfig.Name.
// toolName is the tool identifier from MCPTool.Name.
// ent is the caller's entitlement; ent.AllowedMCPs is checked against mcpName.
//
// Returns ErrMCPNotFound, ErrToolNotWhitelisted, or ErrHITLRequired as appropriate.
func (r *Registry) Allow(ctx context.Context, mcpName, toolName string, ent *model.UserEntitlement) (*AllowResult, error) {
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("mcp.Allow: %w", ctx.Err())
	default:
	}

	// check user entitlement for the MCP server.
	if !isMCPAllowed(mcpName, ent.AllowedMCPs) {
		return &AllowResult{Allowed: false}, fmt.Errorf("mcp.Allow: access denied to MCP server %q", mcpName)
	}

	r.mu.RLock()
	server, ok := r.servers[mcpName]
	r.mu.RUnlock()

	if !ok {
		return &AllowResult{Allowed: false}, fmt.Errorf("mcp.Allow: %w: %q", ErrMCPNotFound, mcpName)
	}

	// find the tool in the whitelist.
	tool, found := findTool(server.Tools, toolName)
	if !found {
		return &AllowResult{Allowed: false},
			fmt.Errorf("mcp.Allow: %w: server=%q tool=%q", ErrToolNotWhitelisted, mcpName, toolName)
	}

	// stub OPA/Cedar evaluation — always passes in this implementation.
	// TODO: replace with real Cedar/OPA policy evaluation when the policy engine is ready.
	if err := r.stubOPACheck(ctx, mcpName, toolName, ent); err != nil {
		return &AllowResult{Allowed: false}, fmt.Errorf("mcp.Allow: policy check: %w", err)
	}

	result := &AllowResult{
		Allowed:      true,
		HITLRequired: tool.Destructive,
		Tool:         tool,
	}

	if tool.Destructive {
		r.log.Warn().
			Str("mcp", mcpName).
			Str("tool", toolName).
			Str("user_id", ent.UserID).
			Msg("destructive tool call flagged for HITL")
	}

	return result, nil
}

// stubOPACheck is a placeholder for Cedar/OPA policy evaluation.
// It always returns nil (allow) until a real policy engine is integrated.
//
// TODO: replace with actual Cedar/OPA evaluation.
func (r *Registry) stubOPACheck(_ context.Context, _, _ string, _ *model.UserEntitlement) error {
	return nil
}

// findTool looks up a tool by name in the whitelist slice.
func findTool(tools []model.MCPTool, name string) (model.MCPTool, bool) {
	for _, t := range tools {
		if t.Name == name {
			return t, true
		}
	}
	return model.MCPTool{}, false
}

// isMCPAllowed checks whether mcpName matches at least one of the allowed patterns.
func isMCPAllowed(mcpName string, allowedPatterns []string) bool {
	for _, p := range allowedPatterns {
		if p == "*" || p == mcpName {
			return true
		}
		if len(p) > 1 && p[len(p)-1] == '*' {
			prefix := p[:len(p)-1]
			if len(mcpName) >= len(prefix) && mcpName[:len(prefix)] == prefix {
				return true
			}
		}
	}
	return false
}
