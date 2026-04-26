// Package handlers — audit log read endpoint (M12).
//
// Writes happen as a side effect inside other handlers (productsHandler,
// categoriesHandler, junkHandler, apiKeysHandler) — the writer pattern is in
// each of those handlers, not here.
package handlers

import (
	"net/http"
	"strconv"

	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/ports"
)

type AuditHandler struct {
	audit ports.AuditPort
	log   *logger.Logger
}

func NewAuditHandler(audit ports.AuditPort, log *logger.Logger) *AuditHandler {
	return &AuditHandler{audit: audit, log: log}
}

// GET /admin/api/audit?entity_kind=&entity_id=&limit=&offset=
func (h *AuditHandler) HandleList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET only")
		return
	}
	q := r.URL.Query()
	entityKind := q.Get("entity_kind")
	entityID := q.Get("entity_id")
	if entityKind == "" || entityID == "" {
		writeError(w, http.StatusBadRequest, "entity_kind and entity_id required")
		return
	}
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset, _ := strconv.Atoi(q.Get("offset"))
	if offset < 0 {
		offset = 0
	}
	entries, err := h.audit.ListAuditEntries(r.Context(), domain.EntityKind(entityKind), entityID, limit, offset)
	if err != nil {
		h.log.FromContext(r.Context()).Error("list_audit_failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list audit")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": entries})
}
