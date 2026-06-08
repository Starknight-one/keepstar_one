package handlers

import (
	"net/http"

	"keepstar_v5/internal/usecases"
)

// PresetHandler owns the internal preset-control endpoints. Today that is
// just cache invalidation: when a tenant publishes a preset (via the
// future v9-canvas write side / curator cockpit), the caller pokes this
// endpoint so V5 rebuilds the <tenant_design_context> block on the next
// Agent2 turn instead of serving a stale cached system prompt.
type PresetHandler struct {
	promptCache *usecases.PromptCache
}

// NewPresetHandler constructs the handler over the shared Agent2 prompt
// cache (the one whose GetOrBuild assembles <tenant_design_context>).
func NewPresetHandler(promptCache *usecases.PromptCache) *PresetHandler {
	return &PresetHandler{promptCache: promptCache}
}

// CacheInvalidate handles POST /api/v1/internal/presets/cache-invalidate?tenant=<slug>.
// Drops the cached Agent2 system prompt for the tenant so the next turn
// re-reads the tenant's published presets. Gated by X-Internal-Key (same
// guard as the kill switch); exempt from WithTenant (middleware_tenant.go).
func (h *PresetHandler) CacheInvalidate(w http.ResponseWriter, r *http.Request) {
	if !checkInternalKey(w, r) {
		return
	}
	tenant := r.URL.Query().Get("tenant")
	if tenant == "" {
		http.Error(w, "tenant query param missing", http.StatusBadRequest)
		return
	}
	h.promptCache.Invalidate(tenant)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"tenant":      tenant,
		"invalidated": true,
	})
}
