package handlers

import (
	"net/http"
	"strings"

	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/usecases"
)

type IntegrationsHandler struct {
	integrations *usecases.IntegrationsUseCase
	wipe         *usecases.IntegrationsWipeUseCase
	log          *logger.Logger
}

func NewIntegrationsHandler(integrations *usecases.IntegrationsUseCase, wipe *usecases.IntegrationsWipeUseCase, log *logger.Logger) *IntegrationsHandler {
	return &IntegrationsHandler{integrations: integrations, wipe: wipe, log: log}
}

// HandleWipe — POST /admin/api/integrations/{id}/wipe
// Destructive: drops all tenant-scoped catalog rows for this integration
// (listings, raw imports, merge reports, categories, schema artifact) plus
// the OAuth row itself. master_products are intentionally preserved —
// orphan-safe (other tenants may link to them; cleanup separately).
func (h *IntegrationsHandler) HandleWipe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	tenantID := TenantID(r.Context())
	path := strings.TrimPrefix(r.URL.Path, "/admin/api/integrations/")
	id := strings.TrimSuffix(path, "/wipe")
	if id == "" || strings.Contains(id, "/") {
		writeError(w, http.StatusBadRequest, "invalid integration id")
		return
	}
	res, err := h.wipe.WipeAndDisconnect(r.Context(), tenantID, id)
	if err != nil {
		h.log.FromContext(r.Context()).Error("integrations_wipe_failed", "error", err, "id", id)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// HandleList — GET /admin/api/integrations
func (h *IntegrationsHandler) HandleList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET only")
		return
	}
	tenantID := TenantID(r.Context())
	items, err := h.integrations.ListForTenant(r.Context(), tenantID)
	if err != nil {
		h.log.FromContext(r.Context()).Error("integrations_list_failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list integrations")
		return
	}
	if items == nil {
		items = []domain.Integration{}
	}
	writeJSON(w, http.StatusOK, items)
}

// HandleByID — GET or DELETE /admin/api/integrations/{id}
func (h *IntegrationsHandler) HandleByID(w http.ResponseWriter, r *http.Request) {
	tenantID := TenantID(r.Context())
	id := strings.TrimPrefix(r.URL.Path, "/admin/api/integrations/")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing integration id")
		return
	}
	switch r.Method {
	case http.MethodDelete:
		if err := h.integrations.Disconnect(r.Context(), tenantID, id); err != nil {
			h.log.FromContext(r.Context()).Error("integrations_delete_failed", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to disconnect")
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	case http.MethodGet:
		item, err := h.integrations.Get(r.Context(), tenantID, id)
		if err != nil {
			writeError(w, http.StatusNotFound, "integration not found")
			return
		}
		item.Credentials = ""
		writeJSON(w, http.StatusOK, item)
	default:
		writeError(w, http.StatusMethodNotAllowed, "GET or DELETE only")
	}
}
