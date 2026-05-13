package handlers

// V5 chat / trace inspection endpoints (chunk 13). Read-only — Curator
// reads v5_chat_sessions + v5_chat_session_traces directly from the
// shared Neon DB. All three routes wrapped by the same auth
// middleware as the rest of the protected mux.

import (
	"net/http"
	"strconv"
	"strings"

	"keepstar-curator/internal/adapters"
)

// ListChats handles GET /curator/chats?tenant=&status=&q=&limit=&offset=
func (h *Handler) ListChats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "GET only")
		return
	}
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))

	rows, total, err := h.Client.ListChats(r.Context(), adapters.ListChatsFilter{
		TenantSlugOrID: strings.TrimSpace(q.Get("tenant")),
		Status:         q.Get("status"),
		Search:         q.Get("q"),
		Limit:          limit,
		Offset:         offset,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "list chats: "+err.Error())
		return
	}
	hasMore := offset+len(rows) < total
	writeJSON(w, http.StatusOK, map[string]any{
		"sessions": rows,
		"total":    total,
		"hasMore":  hasMore,
	})
}

// GetChatTimeline handles GET /curator/chats/{sessionId}
func (h *Handler) GetChatTimeline(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "GET only")
		return
	}
	// Path: /curator/chats/{sessionId}
	rest := strings.TrimPrefix(r.URL.Path, "/curator/chats/")
	sessionID := strings.TrimSuffix(rest, "/")
	if sessionID == "" || strings.Contains(sessionID, "/") {
		writeErr(w, http.StatusBadRequest, "session id required")
		return
	}

	tl, err := h.Client.GetChatTimeline(r.Context(), sessionID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "chat not found: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, tl)
}

// GetChatTurn handles GET /curator/chats/{sessionId}/turns/{requestId}
func (h *Handler) GetChatTurn(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "GET only")
		return
	}
	// Path: /curator/chats/{sessionId}/turns/{requestId}
	rest := strings.TrimPrefix(r.URL.Path, "/curator/chats/")
	parts := strings.Split(rest, "/")
	if len(parts) < 3 || parts[1] != "turns" || parts[0] == "" || parts[2] == "" {
		writeErr(w, http.StatusBadRequest, "expected /curator/chats/{sessionId}/turns/{requestId}")
		return
	}
	sessionID, requestID := parts[0], parts[2]

	td, err := h.Client.GetChatTurn(r.Context(), sessionID, requestID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "turn not found: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, td)
}
