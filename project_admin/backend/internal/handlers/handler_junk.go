// Package handlers — junk variant triage (M9). Spec: §4.7 / §6.6.
//
// Junk candidates are detected by the harvester (M4d) when 2+ signals match
// the addon heuristic (axis_name pattern, no_identifiers, no_dimensions,
// small_price_delta). The admin UI shows pending records grouped by tenant
// and lets the user mark each as confirmed_addon or false_positive.
package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/ports"
)

type JunkHandler struct {
	candidates ports.CandidatesPort
	log        *logger.Logger
}

func NewJunkHandler(candidates ports.CandidatesPort, log *logger.Logger) *JunkHandler {
	return &JunkHandler{candidates: candidates, log: log}
}

// GET /admin/api/junk?status=pending — list candidates for the current tenant.
func (h *JunkHandler) HandleList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET only")
		return
	}
	statusParam := r.URL.Query().Get("status")
	if statusParam == "" {
		statusParam = string(domain.JunkClassificationPending)
	}
	cands, err := h.candidates.ListJunkCandidates(r.Context(),
		TenantID(r.Context()),
		domain.JunkClassification(statusParam),
	)
	if err != nil {
		h.log.FromContext(r.Context()).Error("list_junk_failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list junk candidates")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"candidates": cands})
}

// GET /admin/api/junk/count — count of pending records, used for sidebar badge.
func (h *JunkHandler) HandleCount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET only")
		return
	}
	n, err := h.candidates.CountJunkPending(r.Context(), TenantID(r.Context()))
	if err != nil {
		h.log.FromContext(r.Context()).Error("count_junk_failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to count junk")
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"count": n})
}

type junkClassifyPayload struct {
	Classification string `json:"classification"`
}

// POST /admin/api/junk/{id}/classify — mark as confirmed_addon | false_positive.
func (h *JunkHandler) HandleClassify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	id := extractJunkID(r.URL.Path)
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing id")
		return
	}
	var p junkClassifyPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	cls := domain.JunkClassification(p.Classification)
	switch cls {
	case domain.JunkClassificationConfirmedAddon, domain.JunkClassificationFalsePositive:
		// ok
	default:
		writeError(w, http.StatusBadRequest, "classification must be confirmed_addon or false_positive")
		return
	}
	if err := h.candidates.ClassifyJunkCandidate(r.Context(), id, cls, UserID(r.Context())); err != nil {
		h.log.FromContext(r.Context()).Error("classify_junk_failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to classify")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// extractJunkID parses /admin/api/junk/{id}/classify.
func extractJunkID(path string) string {
	rest := strings.TrimPrefix(path, "/admin/api/junk/")
	rest = strings.TrimSuffix(rest, "/classify")
	rest = strings.TrimSuffix(rest, "/")
	if strings.Contains(rest, "/") {
		return ""
	}
	return rest
}
