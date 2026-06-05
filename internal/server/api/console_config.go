package api

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/rs/zerolog"

	"github.com/pangreksa/ai-gateway-engine/internal/server/repository"
	"github.com/pangreksa/ai-gateway-engine/pkg/model"
)

// ConsoleConfigHandler implements config registry CRUD endpoints for the
// Pangreksa AI Gateway console.
//
// Thread-safety: safe for concurrent use; all state is read-only after construction.
type ConsoleConfigHandler struct {
	promptRepo    *repository.PromptRepo
	skillRepo     *repository.SkillRepository
	mcpRepo       *repository.MCPServerRepository
	guardrailRepo *repository.GuardrailRepository
	budgetRepo    *repository.BudgetRuleRepository
	rateRepo      *repository.RateLimitRuleRepository
	adminAPIKey   string
	log           zerolog.Logger
}

// NewConsoleConfigHandler constructs a ConsoleConfigHandler with all required dependencies.
func NewConsoleConfigHandler(
	promptRepo *repository.PromptRepo,
	skillRepo *repository.SkillRepository,
	mcpRepo *repository.MCPServerRepository,
	guardrailRepo *repository.GuardrailRepository,
	budgetRepo *repository.BudgetRuleRepository,
	rateRepo *repository.RateLimitRuleRepository,
	adminAPIKey string,
	log zerolog.Logger,
) *ConsoleConfigHandler {
	return &ConsoleConfigHandler{
		promptRepo:    promptRepo,
		skillRepo:     skillRepo,
		mcpRepo:       mcpRepo,
		guardrailRepo: guardrailRepo,
		budgetRepo:    budgetRepo,
		rateRepo:      rateRepo,
		adminAPIKey:   adminAPIKey,
		log:           log.With().Str("component", "api.console_config").Logger(),
	}
}

// Register mounts all config registry CRUD routes onto mux.
// All routes require Bearer ADMIN_API_KEY authentication.
//
// Route layout:
//
//	GET    /admin/config/prompts                   — list prompt repos + versions by org_id
//	POST   /admin/config/prompts/{repo}/{name}     — create a new prompt version
//	GET    /admin/config/skills                    — list skills by org_id
//	POST   /admin/config/skills                    — create skill
//	PUT    /admin/config/skills/{id}               — update skill
//	DELETE /admin/config/skills/{id}               — delete skill (204)
//	GET    /admin/config/mcp                       — list MCP servers by org_id
//	POST   /admin/config/mcp                       — create MCP server
//	PUT    /admin/config/mcp/{id}                  — update MCP server
//	DELETE /admin/config/mcp/{id}                  — delete MCP server (204)
//	POST   /admin/config/mcp/{id}/test             — connectivity stub
//	GET    /admin/config/guardrails                — list guardrails by org_id
//	POST   /admin/config/guardrails                — create guardrail (201)
//	PUT    /admin/config/guardrails/{id}           — update guardrail
//	DELETE /admin/config/guardrails/{id}           — delete guardrail (204)
//	GET    /admin/config/budget-rules              — list budget rules by org_id
//	POST   /admin/config/budget-rules              — create budget rule
//	PUT    /admin/config/budget-rules/{id}         — update budget rule
//	DELETE /admin/config/budget-rules/{id}         — delete budget rule (204)
//	GET    /admin/config/rate-rules                — list rate limit rules by org_id
//	POST   /admin/config/rate-rules                — create rate limit rule
//	PUT    /admin/config/rate-rules/{id}           — update rate limit rule
//	DELETE /admin/config/rate-rules/{id}           — delete rate limit rule (204)
func (h *ConsoleConfigHandler) Register(mux *http.ServeMux) {
	// ── Prompts ───────────────────────────────────────────────────────────────
	mux.HandleFunc("GET /admin/config/prompts", h.authMiddleware(h.handleListPrompts))
	// Go 1.22 pattern routing: {repo} and {name} are path parameters.
	mux.HandleFunc("POST /admin/config/prompts/{repo}/{name}", h.authMiddleware(h.handleCreatePromptVersion))

	// ── Skills ────────────────────────────────────────────────────────────────
	mux.HandleFunc("GET /admin/config/skills", h.authMiddleware(h.handleListSkills))
	mux.HandleFunc("POST /admin/config/skills", h.authMiddleware(h.handleCreateSkill))
	mux.HandleFunc("PUT /admin/config/skills/{id}", h.authMiddleware(h.handleUpdateSkill))
	mux.HandleFunc("DELETE /admin/config/skills/{id}", h.authMiddleware(h.handleDeleteSkill))

	// ── MCP Servers ───────────────────────────────────────────────────────────
	mux.HandleFunc("GET /admin/config/mcp", h.authMiddleware(h.handleListMCP))
	mux.HandleFunc("POST /admin/config/mcp", h.authMiddleware(h.handleCreateMCP))
	mux.HandleFunc("PUT /admin/config/mcp/{id}", h.authMiddleware(h.handleUpdateMCP))
	mux.HandleFunc("DELETE /admin/config/mcp/{id}", h.authMiddleware(h.handleDeleteMCP))
	mux.HandleFunc("POST /admin/config/mcp/{id}/test", h.authMiddleware(h.handleTestMCP))

	// ── Guardrails ────────────────────────────────────────────────────────────
	mux.HandleFunc("GET /admin/config/guardrails", h.authMiddleware(h.handleListGuardrails))
	mux.HandleFunc("POST /admin/config/guardrails", h.authMiddleware(h.handleCreateGuardrail))
	mux.HandleFunc("PUT /admin/config/guardrails/{id}", h.authMiddleware(h.handleUpdateGuardrail))
	mux.HandleFunc("DELETE /admin/config/guardrails/{id}", h.authMiddleware(h.handleDeleteGuardrail))

	// ── Budget Rules ──────────────────────────────────────────────────────────
	mux.HandleFunc("GET /admin/config/budget-rules", h.authMiddleware(h.handleListBudgetRules))
	mux.HandleFunc("POST /admin/config/budget-rules", h.authMiddleware(h.handleCreateBudgetRule))
	mux.HandleFunc("PUT /admin/config/budget-rules/{id}", h.authMiddleware(h.handleUpdateBudgetRule))
	mux.HandleFunc("DELETE /admin/config/budget-rules/{id}", h.authMiddleware(h.handleDeleteBudgetRule))

	// ── Rate Limit Rules ──────────────────────────────────────────────────────
	mux.HandleFunc("GET /admin/config/rate-rules", h.authMiddleware(h.handleListRateRules))
	mux.HandleFunc("POST /admin/config/rate-rules", h.authMiddleware(h.handleCreateRateRule))
	mux.HandleFunc("PUT /admin/config/rate-rules/{id}", h.authMiddleware(h.handleUpdateRateRule))
	mux.HandleFunc("DELETE /admin/config/rate-rules/{id}", h.authMiddleware(h.handleDeleteRateRule))
}

// ── Auth middleware ───────────────────────────────────────────────────────────

// authMiddleware wraps a handler with Bearer ADMIN_API_KEY authentication.
func (h *ConsoleConfigHandler) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.checkBearer(r) {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next(w, r)
	}
}

// checkBearer performs constant-time comparison of the Authorization Bearer token.
func (h *ConsoleConfigHandler) checkBearer(r *http.Request) bool {
	auth := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if !strings.HasPrefix(auth, prefix) {
		return false
	}
	token := auth[len(prefix):]
	return subtle.ConstantTimeCompare([]byte(token), []byte(h.adminAPIKey)) == 1
}

// ── Prompts ───────────────────────────────────────────────────────────────────

// handleListPrompts handles GET /admin/config/prompts.
// Query param: org_id (required).
func (h *ConsoleConfigHandler) handleListPrompts(w http.ResponseWriter, r *http.Request) {
	orgID := r.URL.Query().Get("org_id")
	if orgID == "" {
		writeError(w, http.StatusBadRequest, "org_id is required")
		return
	}

	repos, err := h.promptRepo.ListReposByOrg(r.Context(), orgID)
	if err != nil {
		h.log.Error().Err(err).Str("org_id", orgID).Msg("handleListPrompts")
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, repos)
}

// createPromptVersionRequest is the JSON body for POST /admin/config/prompts/{repo}/{name}.
type createPromptVersionRequest struct {
	Content   string          `json:"content"`
	Config    interface{}     `json:"config"`
	CreatedBy string          `json:"created_by"`
	OrgID     string          `json:"org_id"`
}

// createPromptVersionResponse is the JSON body returned by the create-version endpoint.
type createPromptVersionResponse struct {
	FQN     string `json:"fqn"`
	Version int    `json:"version"`
}

// handleCreatePromptVersion handles POST /admin/config/prompts/{repo}/{name}.
func (h *ConsoleConfigHandler) handleCreatePromptVersion(w http.ResponseWriter, r *http.Request) {
	repoName := r.PathValue("repo")
	promptName := r.PathValue("name")
	if repoName == "" || promptName == "" {
		writeError(w, http.StatusBadRequest, "repo and name path params are required")
		return
	}

	var req createPromptVersionRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.OrgID == "" {
		writeError(w, http.StatusBadRequest, "org_id is required")
		return
	}
	if req.Content == "" {
		writeError(w, http.StatusBadRequest, "content is required")
		return
	}

	// Ensure the repository exists (idempotent upsert).
	repoID, err := h.promptRepo.EnsureRepo(r.Context(), req.OrgID, repoName)
	if err != nil {
		h.log.Error().Err(err).Str("repo", repoName).Msg("handleCreatePromptVersion: ensure repo")
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Marshal the config field back to JSON for storage.
	configJSON, marshalErr := marshalConfig(req.Config)
	if marshalErr != nil {
		writeError(w, http.StatusBadRequest, "config must be a valid JSON object")
		return
	}

	pv, err := h.promptRepo.CreateVersion(r.Context(), repoID, promptName, req.Content, configJSON, req.CreatedBy)
	if err != nil {
		h.log.Error().Err(err).Str("repo_id", repoID).Str("name", promptName).Msg("handleCreatePromptVersion")
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusCreated, createPromptVersionResponse{
		FQN:     pv.FQN,
		Version: pv.Version,
	})
}

// ── Skills ────────────────────────────────────────────────────────────────────

// handleListSkills handles GET /admin/config/skills.
func (h *ConsoleConfigHandler) handleListSkills(w http.ResponseWriter, r *http.Request) {
	orgID := r.URL.Query().Get("org_id")
	if orgID == "" {
		writeError(w, http.StatusBadRequest, "org_id is required")
		return
	}

	skills, err := h.skillRepo.ListByOrg(r.Context(), orgID)
	if err != nil {
		h.log.Error().Err(err).Str("org_id", orgID).Msg("handleListSkills")
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, skills)
}

// handleCreateSkill handles POST /admin/config/skills.
func (h *ConsoleConfigHandler) handleCreateSkill(w http.ResponseWriter, r *http.Request) {
	var s repository.SkillRecord
	if err := decodeJSON(r, &s); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	created, err := h.skillRepo.Create(r.Context(), &s)
	if err != nil {
		h.log.Error().Err(err).Msg("handleCreateSkill")
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

// handleUpdateSkill handles PUT /admin/config/skills/{id}.
func (h *ConsoleConfigHandler) handleUpdateSkill(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "id path param is required")
		return
	}

	var s repository.SkillRecord
	if err := decodeJSON(r, &s); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	updated, err := h.skillRepo.Update(r.Context(), id, &s)
	if err != nil {
		if errors.Is(err, model.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		h.log.Error().Err(err).Str("id", id).Msg("handleUpdateSkill")
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// handleDeleteSkill handles DELETE /admin/config/skills/{id}.
func (h *ConsoleConfigHandler) handleDeleteSkill(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "id path param is required")
		return
	}

	if err := h.skillRepo.Delete(r.Context(), id); err != nil {
		if errors.Is(err, model.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		h.log.Error().Err(err).Str("id", id).Msg("handleDeleteSkill")
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── MCP Servers ───────────────────────────────────────────────────────────────

// handleListMCP handles GET /admin/config/mcp.
func (h *ConsoleConfigHandler) handleListMCP(w http.ResponseWriter, r *http.Request) {
	orgID := r.URL.Query().Get("org_id")
	if orgID == "" {
		writeError(w, http.StatusBadRequest, "org_id is required")
		return
	}

	servers, err := h.mcpRepo.ListByOrg(r.Context(), orgID)
	if err != nil {
		h.log.Error().Err(err).Str("org_id", orgID).Msg("handleListMCP")
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, servers)
}

// handleCreateMCP handles POST /admin/config/mcp.
// CredentialsEnc is stored but never echoed back in the response.
func (h *ConsoleConfigHandler) handleCreateMCP(w http.ResponseWriter, r *http.Request) {
	var s repository.MCPServerRecord
	if err := decodeJSON(r, &s); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	created, err := h.mcpRepo.Create(r.Context(), &s)
	if err != nil {
		h.log.Error().Err(err).Msg("handleCreateMCP")
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	// Redact credentials before returning.
	created.CredentialsEnc = ""
	writeJSON(w, http.StatusCreated, created)
}

// handleUpdateMCP handles PUT /admin/config/mcp/{id}.
func (h *ConsoleConfigHandler) handleUpdateMCP(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "id path param is required")
		return
	}

	var s repository.MCPServerRecord
	if err := decodeJSON(r, &s); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	updated, err := h.mcpRepo.Update(r.Context(), id, &s)
	if err != nil {
		if errors.Is(err, model.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		h.log.Error().Err(err).Str("id", id).Msg("handleUpdateMCP")
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	updated.CredentialsEnc = ""
	writeJSON(w, http.StatusOK, updated)
}

// handleDeleteMCP handles DELETE /admin/config/mcp/{id}.
func (h *ConsoleConfigHandler) handleDeleteMCP(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "id path param is required")
		return
	}

	if err := h.mcpRepo.Delete(r.Context(), id); err != nil {
		if errors.Is(err, model.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		h.log.Error().Err(err).Str("id", id).Msg("handleDeleteMCP")
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// mcpTestResponse is the JSON stub returned by POST /admin/config/mcp/{id}/test.
type mcpTestResponse struct {
	Reachable bool        `json:"reachable"`
	Tools     interface{} `json:"tools"`
	Error     string      `json:"error"`
}

// handleTestMCP handles POST /admin/config/mcp/{id}/test.
// Actual HTTP ping to the MCP server is out of scope for v1; returns a stub response.
func (h *ConsoleConfigHandler) handleTestMCP(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, mcpTestResponse{
		Reachable: false,
		Tools:     []interface{}{},
		Error:     "not implemented",
	})
}

// ── Guardrails ────────────────────────────────────────────────────────────────

// handleListGuardrails handles GET /admin/config/guardrails.
func (h *ConsoleConfigHandler) handleListGuardrails(w http.ResponseWriter, r *http.Request) {
	orgID := r.URL.Query().Get("org_id")
	if orgID == "" {
		writeError(w, http.StatusBadRequest, "org_id is required")
		return
	}

	gs, err := h.guardrailRepo.ListByOrg(r.Context(), orgID)
	if err != nil {
		h.log.Error().Err(err).Str("org_id", orgID).Msg("handleListGuardrails")
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, gs)
}

// handleCreateGuardrail handles POST /admin/config/guardrails.
func (h *ConsoleConfigHandler) handleCreateGuardrail(w http.ResponseWriter, r *http.Request) {
	var g repository.GuardrailRecord
	if err := decodeJSON(r, &g); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	created, err := h.guardrailRepo.Create(r.Context(), &g)
	if err != nil {
		h.log.Error().Err(err).Msg("handleCreateGuardrail")
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": created.ID})
}

// handleUpdateGuardrail handles PUT /admin/config/guardrails/{id}.
func (h *ConsoleConfigHandler) handleUpdateGuardrail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "id path param is required")
		return
	}

	var g repository.GuardrailRecord
	if err := decodeJSON(r, &g); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	updated, err := h.guardrailRepo.Update(r.Context(), id, &g)
	if err != nil {
		if errors.Is(err, model.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		h.log.Error().Err(err).Str("id", id).Msg("handleUpdateGuardrail")
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// handleDeleteGuardrail handles DELETE /admin/config/guardrails/{id}.
func (h *ConsoleConfigHandler) handleDeleteGuardrail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "id path param is required")
		return
	}

	if err := h.guardrailRepo.Delete(r.Context(), id); err != nil {
		if errors.Is(err, model.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		h.log.Error().Err(err).Str("id", id).Msg("handleDeleteGuardrail")
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── Budget Rules ──────────────────────────────────────────────────────────────

// handleListBudgetRules handles GET /admin/config/budget-rules.
func (h *ConsoleConfigHandler) handleListBudgetRules(w http.ResponseWriter, r *http.Request) {
	orgID := r.URL.Query().Get("org_id")
	if orgID == "" {
		writeError(w, http.StatusBadRequest, "org_id is required")
		return
	}

	rules, err := h.budgetRepo.ListByOrg(r.Context(), orgID)
	if err != nil {
		h.log.Error().Err(err).Str("org_id", orgID).Msg("handleListBudgetRules")
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, rules)
}

// handleCreateBudgetRule handles POST /admin/config/budget-rules.
func (h *ConsoleConfigHandler) handleCreateBudgetRule(w http.ResponseWriter, r *http.Request) {
	var rule repository.BudgetRuleRecord
	if err := decodeJSON(r, &rule); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	created, err := h.budgetRepo.Create(r.Context(), &rule)
	if err != nil {
		h.log.Error().Err(err).Msg("handleCreateBudgetRule")
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

// handleUpdateBudgetRule handles PUT /admin/config/budget-rules/{id}.
func (h *ConsoleConfigHandler) handleUpdateBudgetRule(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "id path param is required")
		return
	}

	var rule repository.BudgetRuleRecord
	if err := decodeJSON(r, &rule); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	updated, err := h.budgetRepo.Update(r.Context(), id, &rule)
	if err != nil {
		if errors.Is(err, model.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		h.log.Error().Err(err).Str("id", id).Msg("handleUpdateBudgetRule")
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// handleDeleteBudgetRule handles DELETE /admin/config/budget-rules/{id}.
func (h *ConsoleConfigHandler) handleDeleteBudgetRule(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "id path param is required")
		return
	}

	if err := h.budgetRepo.Delete(r.Context(), id); err != nil {
		if errors.Is(err, model.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		h.log.Error().Err(err).Str("id", id).Msg("handleDeleteBudgetRule")
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── Rate Limit Rules ──────────────────────────────────────────────────────────

// handleListRateRules handles GET /admin/config/rate-rules.
func (h *ConsoleConfigHandler) handleListRateRules(w http.ResponseWriter, r *http.Request) {
	orgID := r.URL.Query().Get("org_id")
	if orgID == "" {
		writeError(w, http.StatusBadRequest, "org_id is required")
		return
	}

	rules, err := h.rateRepo.ListByOrg(r.Context(), orgID)
	if err != nil {
		h.log.Error().Err(err).Str("org_id", orgID).Msg("handleListRateRules")
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, rules)
}

// handleCreateRateRule handles POST /admin/config/rate-rules.
func (h *ConsoleConfigHandler) handleCreateRateRule(w http.ResponseWriter, r *http.Request) {
	var rule repository.RateLimitRuleRecord
	if err := decodeJSON(r, &rule); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	created, err := h.rateRepo.Create(r.Context(), &rule)
	if err != nil {
		h.log.Error().Err(err).Msg("handleCreateRateRule")
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

// handleUpdateRateRule handles PUT /admin/config/rate-rules/{id}.
func (h *ConsoleConfigHandler) handleUpdateRateRule(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "id path param is required")
		return
	}

	var rule repository.RateLimitRuleRecord
	if err := decodeJSON(r, &rule); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	updated, err := h.rateRepo.Update(r.Context(), id, &rule)
	if err != nil {
		if errors.Is(err, model.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		h.log.Error().Err(err).Str("id", id).Msg("handleUpdateRateRule")
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// handleDeleteRateRule handles DELETE /admin/config/rate-rules/{id}.
func (h *ConsoleConfigHandler) handleDeleteRateRule(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "id path param is required")
		return
	}

	if err := h.rateRepo.Delete(r.Context(), id); err != nil {
		if errors.Is(err, model.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		h.log.Error().Err(err).Str("id", id).Msg("handleDeleteRateRule")
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// marshalConfig marshals an arbitrary config value to JSON bytes.
// Returns an empty JSON object if config is nil.
func marshalConfig(config interface{}) ([]byte, error) {
	if config == nil {
		return []byte("{}"), nil
	}
	b, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}
	return b, nil
}
