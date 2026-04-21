package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/usecases"
)

type BillingHandler struct {
	billing *usecases.BillingUseCase
	log     *logger.Logger
}

func NewBillingHandler(billing *usecases.BillingUseCase, log *logger.Logger) *BillingHandler {
	return &BillingHandler{billing: billing, log: log}
}

func (h *BillingHandler) HandleOverview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET only")
		return
	}
	ctx := r.Context()
	tenantID := TenantID(ctx)
	if tenantID == "" {
		writeError(w, http.StatusUnauthorized, "no tenant")
		return
	}
	ov, err := h.billing.Overview(ctx, tenantID)
	if err != nil {
		h.log.FromContext(ctx).Error("billing_overview_failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to load billing")
		return
	}
	writeJSON(w, http.StatusOK, ov)
}

func (h *BillingHandler) HandleInvoices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET only")
		return
	}
	ctx := r.Context()
	tenantID := TenantID(ctx)
	if tenantID == "" {
		writeError(w, http.StatusUnauthorized, "no tenant")
		return
	}
	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil {
			limit = n
		}
	}
	invs, err := h.billing.ListInvoices(ctx, tenantID, limit)
	if err != nil {
		h.log.FromContext(ctx).Error("billing_invoices_failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list invoices")
		return
	}
	if invs == nil {
		invs = []domain.Invoice{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": invs})
}

func (h *BillingHandler) HandleUpdatePlan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	ctx := r.Context()
	tenantID := TenantID(ctx)
	if tenantID == "" {
		writeError(w, http.StatusUnauthorized, "no tenant")
		return
	}
	var body struct {
		Plan string `json:"plan"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if !domain.IsValidPlan(body.Plan) {
		writeError(w, http.StatusBadRequest, "invalid plan")
		return
	}
	if err := h.billing.UpdatePlan(ctx, tenantID, body.Plan); err != nil {
		h.log.FromContext(ctx).Error("billing_update_plan_failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to update plan")
		return
	}
	h.log.FromContext(ctx).Info("billing_plan_updated", "plan", body.Plan)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *BillingHandler) HandleUpdatePreferences(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	ctx := r.Context()
	tenantID := TenantID(ctx)
	if tenantID == "" {
		writeError(w, http.StatusUnauthorized, "no tenant")
		return
	}
	var prefs domain.Preferences
	if err := json.NewDecoder(r.Body).Decode(&prefs); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if err := h.billing.UpdatePreferences(ctx, tenantID, prefs); err != nil {
		h.log.FromContext(ctx).Error("billing_update_prefs_failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to update preferences")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
