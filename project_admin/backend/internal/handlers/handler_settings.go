package handlers

import (
	"encoding/json"
	"net/http"

	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/usecases"
)

type SettingsHandler struct {
	settings *usecases.SettingsUseCase
}

func NewSettingsHandler(settings *usecases.SettingsUseCase) *SettingsHandler {
	return &SettingsHandler{settings: settings}
}

func (h *SettingsHandler) HandleGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET only")
		return
	}

	tenantID := TenantID(r.Context())
	settings, err := h.settings.Get(r.Context(), tenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get settings")
		return
	}

	writeJSON(w, http.StatusOK, settings)
}

func (h *SettingsHandler) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		writeError(w, http.StatusMethodNotAllowed, "PUT only")
		return
	}

	tenantID := TenantID(r.Context())

	var settings domain.TenantSettings
	if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	if err := h.settings.Update(r.Context(), tenantID, settings); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update settings")
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
