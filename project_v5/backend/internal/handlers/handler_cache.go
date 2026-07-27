package handlers

import (
	"log/slog"
	"net/http"
)

// tenantCacheInvalidator is the slice of a prompt cache the handler needs —
// both usecases.Agent1PromptCache and usecases.PromptCache satisfy it.
type tenantCacheInvalidator interface {
	Invalidate(tenantSlug string)
}

// registrySpecInvalidator is the slice of operations.Registry the handler
// needs: dropping the per-tenant materialized operation specs.
type registrySpecInvalidator interface {
	InvalidateTenant(tenantSlug string)
}

// CacheHandler owns the ONE scoped cache-invalidation endpoint (R15):
//
//	POST /api/v1/internal/cache/invalidate?tenant=<slug>&scope=agent1|agent2|all
//
// gated by X-Internal-Key (503 when V5_INTERNAL_KEY unset, 403 mismatch);
// exempt from WithTenant like every /api/v1/internal/ route.
//
// Scopes (§6.1 cache matrix):
//   - agent1: Agent1 prompt cache AND the operation registry's per-tenant
//     spec cache — always invalidated together (C1).
//   - agent2: Agent2 prompt cache (<fields> + <tenant_design_context>, C2).
//   - all:    both.
//
// The legacy POST /api/v1/internal/presets/cache-invalidate route stays
// mounted as an alias ≡ scope=agent2 so the deployed admin binary keeps
// working (admin v5_client.go). Invalidation is best-effort by contract:
// the §6.1 TTL safety net heals anything a caller misses.
type CacheHandler struct {
	agent1   tenantCacheInvalidator
	agent2   tenantCacheInvalidator
	registry registrySpecInvalidator
	log      *slog.Logger
}

// NewCacheHandler constructs the handler over both prompt caches and the
// operation registry. All three are required — nil means a wiring bug, and
// failing at boot beats silently skipping an invalidation in production.
func NewCacheHandler(agent1, agent2 tenantCacheInvalidator, registry registrySpecInvalidator, log *slog.Logger) *CacheHandler {
	if agent1 == nil || agent2 == nil || registry == nil {
		panic("NewCacheHandler: nil cache or registry")
	}
	return &CacheHandler{agent1: agent1, agent2: agent2, registry: registry, log: log}
}

// Invalidate handles POST /api/v1/internal/cache/invalidate?tenant=&scope=.
func (h *CacheHandler) Invalidate(w http.ResponseWriter, r *http.Request) {
	if !checkInternalKey(w, r) {
		return
	}
	tenant := r.URL.Query().Get("tenant")
	if tenant == "" {
		http.Error(w, "tenant query param missing", http.StatusBadRequest)
		return
	}
	scope := r.URL.Query().Get("scope")
	switch scope {
	case "agent1", "agent2", "all":
	default:
		http.Error(w, "scope must be agent1|agent2|all", http.StatusBadRequest)
		return
	}
	h.fire(tenant, scope)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"tenant":      tenant,
		"scope":       scope,
		"invalidated": true,
	})
}

// InvalidateLegacy handles POST /api/v1/internal/presets/cache-invalidate
// ?tenant=<slug> — the pre-R15 route, kept as an alias ≡ scope=agent2. The
// response keeps the legacy {tenant, invalidated} keys.
func (h *CacheHandler) InvalidateLegacy(w http.ResponseWriter, r *http.Request) {
	if !checkInternalKey(w, r) {
		return
	}
	tenant := r.URL.Query().Get("tenant")
	if tenant == "" {
		http.Error(w, "tenant query param missing", http.StatusBadRequest)
		return
	}
	h.fire(tenant, "agent2")
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"tenant":      tenant,
		"scope":       "agent2",
		"invalidated": true,
	})
}

// fire dispatches one validated invalidation. scope=agent1 drops the Agent1
// prompt cache AND the registry spec cache together (R15).
func (h *CacheHandler) fire(tenant, scope string) {
	if scope == "agent1" || scope == "all" {
		h.agent1.Invalidate(tenant)
		h.registry.InvalidateTenant(tenant)
	}
	if scope == "agent2" || scope == "all" {
		h.agent2.Invalidate(tenant)
	}
	if h.log != nil {
		h.log.Info("cache_invalidated", "tenant", tenant, "scope", scope)
	}
}
