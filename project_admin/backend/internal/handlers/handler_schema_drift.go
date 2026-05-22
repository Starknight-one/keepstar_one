// Package handlers — schema drift read + decide endpoints (B2).
//
// GET  /admin/api/catalog/drift?tenant_id=&status=  — list findings
// POST /admin/api/catalog/drift/{id}/apply           — accept (triggers patch-discovery)
// POST /admin/api/catalog/drift/{id}/dismiss         — reject finding
package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/ports"
	"keepstar-admin/internal/usecases"
)

type SchemaDriftHandler struct {
	findings  ports.SchemaDriftFindingsPort
	discovery *usecases.DiscoveryV2
	log       *logger.Logger
}

func NewSchemaDriftHandler(findings ports.SchemaDriftFindingsPort, discovery *usecases.DiscoveryV2, log *logger.Logger) *SchemaDriftHandler {
	return &SchemaDriftHandler{findings: findings, discovery: discovery, log: log}
}

func (h *SchemaDriftHandler) List(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET only")
		return
	}
	// Tenant comes from session by default (active workspace). Allow an
	// explicit override via query string for ops / scripting access.
	tenantID := r.URL.Query().Get("tenant_id")
	if tenantID == "" {
		tenantID = TenantID(r.Context())
	}
	if tenantID == "" {
		writeError(w, http.StatusBadRequest, "tenant_id required")
		return
	}
	status := domain.SchemaDriftStatus(r.URL.Query().Get("status"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	rows, err := h.findings.List(r.Context(), tenantID, status, limit)
	if err != nil {
		h.log.FromContext(r.Context()).Error("drift_list_failed", "tenant", tenantID, "error", err)
		writeError(w, http.StatusInternalServerError, "list drift findings failed")
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, f := range rows {
		out = append(out, map[string]any{
			"id":                f.ID,
			"tenant_id":         f.TenantID,
			"apply_run_id":      f.ApplyRunID,
			"field_name":        f.FieldName,
			"type_guess":        f.TypeGuess,
			"stats":             json.RawMessage(f.Stats),
			"decision":          string(f.Decision),
			"confidence":        f.Confidence,
			"suggested_action":  json.RawMessage(f.SuggestedAction),
			"status":            string(f.Status),
			"created_at":        f.CreatedAt,
			"classified_at":     f.ClassifiedAt,
			"decided_at":        f.DecidedAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"findings": out})
}

// Apply accepts a finding. We mark it applied immediately and trigger a
// discovery_v2 patch with `trigger=manual_drift_apply` carrying the
// suggested_action JSON. Discovery's prompt then knows to interpret the
// payload as a patch instruction (B1's pre-load path makes this cheap).
func (h *SchemaDriftHandler) Apply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	id := extractIDFromDriftPath(r.URL.Path, "/apply")
	if id == "" {
		writeError(w, http.StatusBadRequest, "id required in path")
		return
	}
	finding, err := h.findings.Get(r.Context(), id)
	if err != nil {
		h.log.FromContext(r.Context()).Error("drift_get_failed", "id", id, "error", err)
		writeError(w, http.StatusNotFound, "finding not found")
		return
	}
	// Fire patch-discovery. Best-effort — if it fails, finding is still
	// marked applied so curator can re-trigger or fix manually.
	if h.discovery != nil {
		payload, _ := json.Marshal(map[string]any{
			"finding_id":       finding.ID,
			"field":            finding.FieldName,
			"decision":         string(finding.Decision),
			"suggested_action": json.RawMessage(finding.SuggestedAction),
		})
		_, dErr := h.discovery.Discover(r.Context(), finding.TenantID, "manual_drift_apply", payload)
		if dErr != nil {
			h.log.FromContext(r.Context()).Warn("drift_apply_discovery_failed",
				"id", id, "tenant", finding.TenantID, "error", dErr)
		}
	}
	if err := h.findings.MarkApplied(r.Context(), id); err != nil {
		h.log.FromContext(r.Context()).Error("drift_mark_applied_failed", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "mark applied failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "status": "applied"})
}

func (h *SchemaDriftHandler) Dismiss(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	id := extractIDFromDriftPath(r.URL.Path, "/dismiss")
	if id == "" {
		writeError(w, http.StatusBadRequest, "id required in path")
		return
	}
	if err := h.findings.MarkDismissed(r.Context(), id); err != nil {
		h.log.FromContext(r.Context()).Error("drift_mark_dismissed_failed", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "mark dismissed failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "status": "dismissed"})
}

// extractIDFromDriftPath pulls the {id} segment from
//
//	/admin/api/catalog/drift/{id}/apply
//	/admin/api/catalog/drift/{id}/dismiss
//
// Returns "" if the URL shape is off.
func extractIDFromDriftPath(path, suffix string) string {
	const prefix = "/admin/api/catalog/drift/"
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	rest := strings.TrimPrefix(path, prefix)
	rest = strings.TrimSuffix(rest, suffix)
	if rest == "" || strings.Contains(rest, "/") {
		return ""
	}
	return rest
}
