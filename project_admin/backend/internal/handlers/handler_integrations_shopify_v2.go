// handler_integrations_shopify_v2.go — routes for the M4 metadata-first
// import flow. Lives alongside the legacy ShopifyHandler during 4a-c; the
// legacy one is deleted in 4d as part of the cut-over.
package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/usecases"
)

// ShopifyV2Handler holds endpoints exposed by the new pipeline. Right now
// (4a + 4c) the dump-to-staging and discover routes are live; harvest
// endpoint lands in 4d and will hang off this same struct.
type ShopifyV2Handler struct {
	v2        *usecases.ShopifyV2UseCase
	discovery *usecases.DiscoveryUseCase
	log       *logger.Logger
}

func NewShopifyV2Handler(v2 *usecases.ShopifyV2UseCase, discovery *usecases.DiscoveryUseCase, log *logger.Logger) *ShopifyV2Handler {
	return &ShopifyV2Handler{v2: v2, discovery: discovery, log: log}
}

// HandleDumpToStaging — POST /admin/api/integrations/shopify/{id}/dump-to-staging
//
// Authenticated. Synchronous for small catalogs (the dev store with 17 products
// finishes in <30s); for larger catalogs the handler will eventually move to a
// 202 Accepted + status-polling pattern (4d). For 4a this is the dev-only
// trigger we use to manually exercise the new bulk pipeline against the
// existing dev store.
//
// On success returns the DumpToStagingResult so the dev can eyeball product
// counts, vendor counts, etc. without opening psql.
func (h *ShopifyV2Handler) HandleDumpToStaging(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	tenantID := TenantID(r.Context())
	if tenantID == "" {
		writeError(w, http.StatusUnauthorized, "missing tenant")
		return
	}
	id := extractIntegrationID(r.URL.Path, "/dump-to-staging")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing integration id")
		return
	}
	res, err := h.v2.DumpToStaging(r.Context(), tenantID, id)
	if err != nil {
		h.log.FromContext(r.Context()).Error("shopify_v2_dump_failed",
			"integration_id", id, "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	out, _ := json.Marshal(map[string]any{
		"operation_id":       res.OperationID,
		"product_count":      res.ProductCount,
		"collection_count":   res.CollectionCount,
		"metafield_defs":     res.MetafieldDefs,
		"navigation_items":   res.NavigationItems,
		"vendor_count":       res.VendorCount,
		"product_type_count": res.ProductTypeCount,
		"tag_count":          res.TagCount,
		"duration_ms":        res.Duration.Milliseconds(),
	})
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(out)
}

// extractIntegrationID pulls the {id} segment out of
// `/admin/api/integrations/shopify/{id}/<suffix>`. Returns "" if the path
// doesn't end in the expected suffix.
func extractIntegrationID(path, suffix string) string {
	const prefix = "/admin/api/integrations/shopify/"
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	rest := strings.TrimPrefix(path, prefix)
	if !strings.HasSuffix(rest, suffix) {
		return ""
	}
	return strings.TrimSuffix(rest, suffix)
}

// HandleDiscover — POST /admin/api/integrations/shopify/{id}/discover
//
// Authenticated. Runs the discovery pipeline (metadata harvest → Tier 1
// auto-map → Sonnet 4.6 agent loop → validation → persist artifact).
// Synchronous for the dev-store; production catalogs will move this to
// async background-job pattern in 4d. Returns the validation report and
// truncated transcript so the dev can confirm what the agent did.
func (h *ShopifyV2Handler) HandleDiscover(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	if h.discovery == nil {
		writeError(w, http.StatusServiceUnavailable, "discovery agent not configured (missing ANTHROPIC_API_KEY?)")
		return
	}
	tenantID := TenantID(r.Context())
	if tenantID == "" {
		writeError(w, http.StatusUnauthorized, "missing tenant")
		return
	}
	id := extractIntegrationID(r.URL.Path, "/discover")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing integration id")
		return
	}
	res, err := h.discovery.Run(r.Context(), tenantID)
	if err != nil {
		h.log.FromContext(r.Context()).Error("shopify_v2_discover_failed",
			"integration_id", id, "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	out, _ := json.Marshal(res)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(out)
}
