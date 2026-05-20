// Package handlers — pending approval surface for the curator UI.
// Wires GET /curator/pending/tree, /pending/category/{id},
// /pending/master/{id}, POST /pending/approve, /pending/reject.
//
// All endpoints sit behind the session middleware (same as the rest of
// curator) — decidedBy in the audit + master row is derived from
// session.Email(ctx).
package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"keepstar-curator/internal/session"
)

// ListPendingTree — GET /curator/pending/tree?vertical=cosmetics
func (h *Handler) ListPendingTree(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "GET only")
		return
	}
	vertical := r.URL.Query().Get("vertical")
	nodes, err := h.Client.ListPendingTree(r.Context(), vertical)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"vertical":   vertical,
		"categories": nodes,
		"count":      len(nodes),
	})
}

// ListPendingCategory — GET /curator/pending/category/{id}?limit=&offset=
func (h *Handler) ListPendingCategory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "GET only")
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/curator/pending/category/")
	if i := strings.Index(id, "/"); i >= 0 {
		id = id[:i]
	}
	if id == "" {
		writeErr(w, http.StatusBadRequest, "category id required")
		return
	}
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	cards, total, err := h.Client.ListPendingMastersInCategory(r.Context(), id, limit, offset)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"category_id": id,
		"cards":       cards,
		"total":       total,
	})
}

// GetPendingMaster — GET /curator/pending/master/{id}
func (h *Handler) GetPendingMaster(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "GET only")
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/curator/pending/master/")
	if i := strings.Index(id, "/"); i >= 0 {
		id = id[:i]
	}
	if id == "" {
		writeErr(w, http.StatusBadRequest, "master id required")
		return
	}
	d, err := h.Client.GetPendingMaster(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, d)
}

// approveRejectBody is the shared shape for POST /pending/approve and
// /pending/reject. Caller supplies one or both id lists; both granularities
// are processed in one request to support the UI bulk action bar.
type approveRejectBody struct {
	MasterIDs []string `json:"master_ids,omitempty"`
	ChangeIDs []string `json:"change_ids,omitempty"`
	Reason    string   `json:"reason,omitempty"`
}

// ApprovePending — POST /curator/pending/approve
func (h *Handler) ApprovePending(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	var body approveRejectBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	decidedBy := session.Email(r.Context())
	if decidedBy == "" {
		decidedBy = "curator"
	}

	mastersResult, err := h.Client.BulkApproveMasters(r.Context(), body.MasterIDs, decidedBy)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	changesResult, err := h.Client.BulkApprovePendingChanges(r.Context(), body.ChangeIDs, decidedBy)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"masters_updated": mastersResult.Updated,
		"changes_updated": changesResult.Updated,
		"decided_by":      decidedBy,
	})
}

// RejectPending — POST /curator/pending/reject
func (h *Handler) RejectPending(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	var body approveRejectBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	decidedBy := session.Email(r.Context())
	if decidedBy == "" {
		decidedBy = "curator"
	}

	mastersResult, err := h.Client.BulkRejectMasters(r.Context(), body.MasterIDs, decidedBy, body.Reason)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	changesResult, err := h.Client.BulkRejectPendingChanges(r.Context(), body.ChangeIDs, decidedBy, body.Reason)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"masters_updated": mastersResult.Updated,
		"changes_updated": changesResult.Updated,
		"decided_by":      decidedBy,
	})
}
