// Package handlers — CatalogV2Handler exposes the manual triggers for the
// new 6-step catalog flow. Used by curator's "Sync now" button and for
// internal smoke testing on dev-stand. Routes:
//
//   POST /admin/api/catalog/v2/sync-now/{tenant_id}    — ManualSync (rate-limit bypass)
//   POST /admin/api/catalog/v2/discover/{tenant_id}    — force discovery_v2 run
//
// Both routes are auth'd via X-Internal-Key (curator → admin) OR a normal
// admin auth session — handler relies on the wrapping middleware. Inside,
// it just delegates to UpdateOrchestrator / DiscoveryV2.
package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/usecases"
)

type CatalogV2Handler struct {
	orch      *usecases.UpdateOrchestrator
	discovery *usecases.DiscoveryV2
	log       *logger.Logger
}

func NewCatalogV2Handler(orch *usecases.UpdateOrchestrator, discovery *usecases.DiscoveryV2, log *logger.Logger) *CatalogV2Handler {
	return &CatalogV2Handler{orch: orch, discovery: discovery, log: log}
}

// HandleSyncNow expects path /admin/api/catalog/v2/sync-now/{tenant_id}.
func (h *CatalogV2Handler) HandleSyncNow(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	tenantID := extractTenantSuffix(r.URL.Path, "/admin/api/catalog/v2/sync-now/")
	if tenantID == "" {
		http.Error(w, "missing tenant_id", http.StatusBadRequest)
		return
	}
	res, err := h.orch.ManualSync(r.Context(), tenantID)
	if err != nil {
		h.log.Warn("catalog_v2_sync_now_failed", "tenant", tenantID, "error", err)
		http.Error(w, "sync failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

// HandleDiscover expects path /admin/api/catalog/v2/discover/{tenant_id}.
// Body (optional): {"trigger":"manual|first_install|mapping_miss","payload":{...}}.
func (h *CatalogV2Handler) HandleDiscover(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	tenantID := extractTenantSuffix(r.URL.Path, "/admin/api/catalog/v2/discover/")
	if tenantID == "" {
		http.Error(w, "missing tenant_id", http.StatusBadRequest)
		return
	}
	var body struct {
		Trigger string          `json:"trigger"`
		Payload json.RawMessage `json:"payload"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body) // optional
	}
	if body.Trigger == "" {
		body.Trigger = "manual"
	}
	artifact, err := h.discovery.Discover(r.Context(), tenantID, body.Trigger, body.Payload)
	if err != nil {
		h.log.Warn("catalog_v2_discover_failed", "tenant", tenantID, "error", err)
		http.Error(w, "discover failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"tenant_id": tenantID,
		"committed": artifact != nil,
		"artifact":  artifact,
	})
}

func extractTenantSuffix(path, prefix string) string {
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	return strings.Trim(strings.TrimPrefix(path, prefix), "/")
}
