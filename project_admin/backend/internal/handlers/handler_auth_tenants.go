package handlers

import (
	"encoding/json"
	"net/http"

	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/ports"
	"keepstar-admin/internal/usecases"
)

type TenantsHandler struct {
	tenants *usecases.TenantsUseCase
	log     *logger.Logger
}

func NewTenantsHandler(t *usecases.TenantsUseCase, log *logger.Logger) *TenantsHandler {
	return &TenantsHandler{tenants: t, log: log}
}

// HandleList returns all workspaces the current user belongs to.
// Bearer-protected — lives behind authMW.
func (h *TenantsHandler) HandleList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET only")
		return
	}
	uid := UserID(r.Context())
	list, err := h.tenants.List(r.Context(), uid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list failed")
		return
	}
	if list == nil {
		list = []ports.UserTenantMembership{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"tenants": list})
}

// HandleSelect mints a new token pair scoped to the chosen tenant. The old
// refresh token is NOT revoked here — clients drop the old pair on their own
// and the 30-day TTL will sweep it.
func (h *TenantsHandler) HandleSelect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	var req struct {
		TenantID string `json:"tenant_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.TenantID == "" {
		writeError(w, http.StatusBadRequest, "tenant_id required")
		return
	}
	uid := UserID(r.Context())
	pair, err := h.tenants.Select(r.Context(), uid, req.TenantID, r.Header.Get("User-Agent"), clientIP(r))
	if err != nil {
		writeError(w, http.StatusForbidden, "not a member of that workspace")
		return
	}
	writeJSON(w, http.StatusOK, pair)
}
