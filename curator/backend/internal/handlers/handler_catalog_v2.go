// Package handlers — handler_catalog_v2.go surfaces the new 6-step flow
// data to curator's UI: inbox listing + detail, action log, agent runs
// (list + detail), current mapping artifact. Pure reads from adapters.
package handlers

import (
	"net/http"
	"strconv"
	"strings"
)

func (h *Handler) ListTenantInbox(w http.ResponseWriter, r *http.Request) {
	tenantID := extractTenantSegment(r.URL.Path, "/curator/tenants/", "/inbox")
	if tenantID == "" {
		http.Error(w, "missing tenant_id", http.StatusBadRequest)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	rows, total, err := h.Client.ListInbox(r.Context(), tenantID, limit, offset)
	if err != nil {
		http.Error(w, "list inbox: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": rows, "total": total})
}

func (h *Handler) GetTenantInboxItem(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 5 {
		http.Error(w, "bad path", http.StatusBadRequest)
		return
	}
	itemID := parts[4]
	row, err := h.Client.GetInboxItem(r.Context(), itemID)
	if err != nil {
		http.Error(w, "get inbox: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, row)
}

func (h *Handler) ListTenantActionLog(w http.ResponseWriter, r *http.Request) {
	tenantID := extractTenantSegment(r.URL.Path, "/curator/tenants/", "/action-log")
	if tenantID == "" {
		http.Error(w, "missing tenant_id", http.StatusBadRequest)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	rows, total, err := h.Client.ListActionLog(r.Context(), tenantID, limit, offset)
	if err != nil {
		http.Error(w, "list action log: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": rows, "total": total})
}

func (h *Handler) ListTenantAgentRuns(w http.ResponseWriter, r *http.Request) {
	tenantID := extractTenantSegment(r.URL.Path, "/curator/tenants/", "/agent-runs")
	if tenantID == "" {
		http.Error(w, "missing tenant_id", http.StatusBadRequest)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	rows, total, err := h.Client.ListAgentRuns(r.Context(), tenantID, limit, offset)
	if err != nil {
		http.Error(w, "list agent runs: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": rows, "total": total})
}

func (h *Handler) GetTenantAgentRun(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 5 {
		http.Error(w, "bad path", http.StatusBadRequest)
		return
	}
	runID := parts[4]
	run, err := h.Client.GetAgentRun(r.Context(), runID)
	if err != nil {
		http.Error(w, "get agent run: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func (h *Handler) GetTenantMapping(w http.ResponseWriter, r *http.Request) {
	tenantID := extractTenantSegment(r.URL.Path, "/curator/tenants/", "/mapping")
	if tenantID == "" {
		http.Error(w, "missing tenant_id", http.StatusBadRequest)
		return
	}
	v, err := h.Client.GetMappingArtifact(r.Context(), tenantID)
	if err != nil {
		http.Error(w, "get mapping: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if v == nil {
		writeJSON(w, http.StatusOK, map[string]any{"exists": false})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"exists": true, "artifact": v})
}

// extractTenantSegment pulls {tenant_id} out of /curator/tenants/{tenant_id}/<suffix>.
func extractTenantSegment(path, prefix, suffix string) string {
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	rest := strings.TrimPrefix(path, prefix)
	if suffix != "" && strings.Contains(rest, suffix) {
		idx := strings.Index(rest, suffix)
		return rest[:idx]
	}
	return strings.Trim(rest, "/")
}
