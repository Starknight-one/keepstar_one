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

// HandleSessions returns list of chat sessions with aggregated stats.
func (h *TracesHandler) HandleSessions(w http.ResponseWriter, r *http.Request) {
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

	sessions, total, err := h.traces.ListSessionsWithStats(r.Context(), limit, offset)
	if err != nil {
		h.log.Error("sessions_list_failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list sessions")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"sessions": sessions, "total": total})
}

// HandleSessionDetail returns session info with all traces and user actions.
func (h *TracesHandler) HandleSessionDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET only")
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/admin/api/sessions/")
	id = strings.TrimSuffix(id, "/")
	if id == "" || id == "kill" {
		writeError(w, http.StatusBadRequest, "session ID required")
		return
	}

	session, err := h.traces.GetSession(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}

	traces, err := h.traces.ListBySession(r.Context(), id)
	if err != nil {
		h.log.Error("session_traces_failed", "error", err, "session_id", id)
		writeError(w, http.StatusInternalServerError, "failed to get traces")
		return
	}

	actions, err := h.traces.ListUserActions(r.Context(), id)
	if err != nil {
		h.log.Error("session_actions_failed", "error", err, "session_id", id)
		writeError(w, http.StatusInternalServerError, "failed to get actions")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"session":     session,
		"traces":      traces,
		"userActions": actions,
	})
}

// HandleKillAllSessions closes all active sessions.
func (h *TracesHandler) HandleKillAllSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}

	killed, err := h.traces.KillAllActiveSessions(r.Context())
	if err != nil {
		h.log.Error("kill_all_sessions_failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to kill all sessions")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "killed": killed})
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
