package handlers

import (
	"encoding/json"
	"net/http"

	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/usecases"
)

type SettingsHandler struct {
	settings *usecases.SettingsUseCase
	log      *logger.Logger
}

func NewSettingsHandler(settings *usecases.SettingsUseCase, log *logger.Logger) *SettingsHandler {
	return &SettingsHandler{settings: settings, log: log}
}

func (h *SettingsHandler) HandleGet(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if sc := domain.SpanFromContext(ctx); sc != nil {
		endSpan := sc.Start("handler.settings_get")
		defer endSpan()
	}

	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET only")
		return
	}

	tenantID := TenantID(ctx)
	settings, err := h.settings.Get(ctx, tenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get settings")
		return
	}

	writeJSON(w, http.StatusOK, settings)
}

func (h *SettingsHandler) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if sc := domain.SpanFromContext(ctx); sc != nil {
		endSpan := sc.Start("handler.settings_update")
		defer endSpan()
	}

	if r.Method != http.MethodPut {
		writeError(w, http.StatusMethodNotAllowed, "PUT only")
		return
	}

	tenantID := TenantID(ctx)

	var settings domain.TenantSettings
	if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	if err := h.settings.Update(ctx, tenantID, settings); err != nil {
		h.log.FromContext(ctx).Error("settings_update_failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to update settings")
		return
	}

	h.log.FromContext(ctx).Info("settings_updated")
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
