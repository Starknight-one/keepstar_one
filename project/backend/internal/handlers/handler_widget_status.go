package handlers

import (
	"net/http"

	"keepstar/internal/ports"
)

// WidgetStatusHandler handles GET /api/v1/widget/status
type WidgetStatusHandler struct {
	catalogPort ports.CatalogPort
}

func NewWidgetStatusHandler(catalogPort ports.CatalogPort) *WidgetStatusHandler {
	return &WidgetStatusHandler{catalogPort: catalogPort}
}

func (h *WidgetStatusHandler) HandleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "GET only"})
		return
	}

	slug := r.URL.Query().Get("tenant")
	if slug == "" {
		slug = r.Header.Get("X-Tenant-Slug")
	}
	if slug == "" {
		writeJSON(w, http.StatusOK, map[string]bool{"enabled": false})
		return
	}

	tenant, err := h.catalogPort.GetTenantBySlug(r.Context(), slug)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]bool{"enabled": false})
		return
	}

	enabled := false
	if tenant.Settings != nil {
		if v, ok := tenant.Settings["widgetEnabled"]; ok {
			if b, ok := v.(bool); ok {
				enabled = b
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]bool{"enabled": enabled})
}
