// Package handlers — curator-side merge agent endpoints (Phase D3,
// catalog completion 2026-04-28).
//
// These endpoints are NOT for tenants. They live behind an internal API key
// (X-Internal-Key header) and are called by the curator-backend on behalf of
// a curator user. Path layout deliberately starts with /admin/api/internal/
// so it's clearly out-of-band from the tenant-facing /admin/api/ surface.
package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/ports"
	"keepstar-admin/internal/usecases"
)

type CuratorMergeHandler struct {
	uc        *usecases.MergeApplyUseCase
	discovery *usecases.DiscoveryUseCase
	reports   ports.MergeReportsPort
	log       *logger.Logger
}

func NewCuratorMergeHandler(uc *usecases.MergeApplyUseCase, discovery *usecases.DiscoveryUseCase, reports ports.MergeReportsPort, log *logger.Logger) *CuratorMergeHandler {
	return &CuratorMergeHandler{uc: uc, discovery: discovery, reports: reports, log: log}
}

// POST /admin/api/internal/curator/tenants/{tenantId}/discover
// Triggers the M4c discovery agent for the tenant — produces / refreshes the
// MappingArtifact that the merge agent (run / apply) reads. ~$0.40 in LLM cost
// per call. Idempotent: re-running supersedes the prior artifact.
func (h *CuratorMergeHandler) HandleDiscover(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	tenantID := tenantIDFromMergePath(r.URL.Path)
	if tenantID == "" {
		writeError(w, http.StatusBadRequest, "tenant_id missing in path")
		return
	}
	if h.discovery == nil {
		writeError(w, http.StatusServiceUnavailable, "discovery agent not configured (missing ANTHROPIC_API_KEY?)")
		return
	}
	res, err := h.discovery.Run(r.Context(), tenantID)
	if err != nil {
		h.log.FromContext(r.Context()).Error("discovery_run_failed", "error", err, "tenant_id", tenantID)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// POST /admin/api/internal/curator/tenants/{tenantId}/merge/run
func (h *CuratorMergeHandler) HandleRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	tenantID := tenantIDFromMergePath(r.URL.Path)
	if tenantID == "" {
		writeError(w, http.StatusBadRequest, "tenant_id missing in path")
		return
	}
	res, err := h.uc.GenerateReport(r.Context(), tenantID)
	if err != nil {
		h.log.FromContext(r.Context()).Error("merge_run_failed", "error", err, "tenant_id", tenantID)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// GET /admin/api/internal/curator/tenants/{tenantId}/merge-reports?limit=20
func (h *CuratorMergeHandler) HandleListForTenant(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET only")
		return
	}
	tenantID := tenantIDFromMergePath(r.URL.Path)
	if tenantID == "" {
		writeError(w, http.StatusBadRequest, "tenant_id missing in path")
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 20
	}
	reports, err := h.reports.ListForTenant(r.Context(), tenantID, limit)
	if err != nil {
		h.log.FromContext(r.Context()).Error("merge_list_failed", "error", err, "tenant_id", tenantID)
		writeError(w, http.StatusInternalServerError, "failed to list reports")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"tenantId": tenantID,
		"reports":  reports,
	})
}

// GET /admin/api/internal/curator/merge-reports/{id}
func (h *CuratorMergeHandler) HandleGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET only")
		return
	}
	id, err := reportIDFromPath(r.URL.Path)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	report, err := h.reports.GetByID(r.Context(), id)
	if err != nil {
		h.log.FromContext(r.Context()).Error("merge_get_failed", "error", err, "id", id)
		writeError(w, http.StatusInternalServerError, "failed to load report")
		return
	}
	if report == nil {
		writeError(w, http.StatusNotFound, "report not found")
		return
	}
	writeJSON(w, http.StatusOK, report)
}

// POST /admin/api/internal/curator/merge-reports/{id}/apply
// Body: { "proposalIds": ["..."], "edits": { "<id>": {...} }, "actorId": "..." }
func (h *CuratorMergeHandler) HandleApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	id, err := reportIDFromPath(strings.TrimSuffix(r.URL.Path, "/apply"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var req usecases.ApplyProposalsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}
	req.ReportID = id
	if req.ActorID == "" {
		req.ActorID = r.Header.Get("X-Curator-User")
	}
	res, err := h.uc.ApplyProposals(r.Context(), req)
	if err != nil {
		h.log.FromContext(r.Context()).Error("merge_apply_failed", "error", err, "id", id)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// POST /admin/api/internal/curator/merge-reports/{id}/revert
// Body: { "actorId": "..." }
func (h *CuratorMergeHandler) HandleRevert(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	id, err := reportIDFromPath(strings.TrimSuffix(r.URL.Path, "/revert"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var body struct {
		ActorID string `json:"actorId"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body) // body optional
	actorID := body.ActorID
	if actorID == "" {
		actorID = r.Header.Get("X-Curator-User")
	}
	if err := h.uc.Revert(r.Context(), id, actorID); err != nil {
		h.log.FromContext(r.Context()).Error("merge_revert_failed", "error", err, "id", id)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"reportId": id,
		"status":   "reverted",
	})
}

// tenantIDFromMergePath extracts {tenantId} from
// /admin/api/internal/curator/tenants/{tenantId}/(merge/run|merge-reports).
func tenantIDFromMergePath(path string) string {
	const prefix = "/admin/api/internal/curator/tenants/"
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	rest := strings.TrimPrefix(path, prefix)
	idx := strings.Index(rest, "/")
	if idx < 0 {
		return rest
	}
	return rest[:idx]
}

// reportIDFromPath extracts {id} from /admin/api/internal/curator/merge-reports/{id}.
func reportIDFromPath(path string) (int64, error) {
	const prefix = "/admin/api/internal/curator/merge-reports/"
	if !strings.HasPrefix(path, prefix) {
		return 0, errors.New("invalid path")
	}
	rest := strings.TrimPrefix(path, prefix)
	if i := strings.Index(rest, "/"); i >= 0 {
		rest = rest[:i]
	}
	id, err := strconv.ParseInt(rest, 10, 64)
	if err != nil {
		return 0, errors.New("invalid report id")
	}
	return id, nil
}
