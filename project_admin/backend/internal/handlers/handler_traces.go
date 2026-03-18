package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"keepstar-admin/internal/adapters/postgres"
	"keepstar-admin/internal/logger"
)

// TracesHandler handles trace-related HTTP requests.
type TracesHandler struct {
	traces *postgres.TraceAdapter
	log    *logger.Logger
}

// NewTracesHandler creates a new TracesHandler.
func NewTracesHandler(traces *postgres.TraceAdapter, log *logger.Logger) *TracesHandler {
	return &TracesHandler{traces: traces, log: log}
}

// HandleList returns paginated list of traces.
func (h *TracesHandler) HandleList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET only")
		return
	}

	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	if limit <= 0 {
		limit = 50
	}

	traces, total, err := h.traces.List(r.Context(), limit, offset)
	if err != nil {
		h.log.Error("traces_list_failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list traces")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"traces": traces,
		"total":  total,
	})
}

// HandleGet returns a single trace by ID.
func (h *TracesHandler) HandleGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET only")
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/admin/api/traces/")
	id = strings.TrimSuffix(id, "/")
	if id == "" {
		writeError(w, http.StatusBadRequest, "trace ID required")
		return
	}

	trace, err := h.traces.Get(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "trace not found")
		return
	}

	writeJSON(w, http.StatusOK, trace)
}

// HandleSessions returns list of chat sessions.
func (h *TracesHandler) HandleSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET only")
		return
	}

	sessions, err := h.traces.ListSessions(r.Context())
	if err != nil {
		h.log.Error("sessions_list_failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list sessions")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"sessions": sessions})
}

// HandleKillSession marks a chat session as closed.
func (h *TracesHandler) HandleKillSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}

	var body struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.SessionID == "" {
		writeError(w, http.StatusBadRequest, "sessionId required")
		return
	}

	if err := h.traces.KillSession(r.Context(), body.SessionID); err != nil {
		h.log.Error("kill_session_failed", "error", err, "session_id", body.SessionID)
		writeError(w, http.StatusInternalServerError, "failed to kill session")
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
