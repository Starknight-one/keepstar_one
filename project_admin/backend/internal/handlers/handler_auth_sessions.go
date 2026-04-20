package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/usecases"
)

type SessionsHandler struct {
	sessions *usecases.SessionsUseCase
	log      *logger.Logger
}

func NewSessionsHandler(s *usecases.SessionsUseCase, log *logger.Logger) *SessionsHandler {
	return &SessionsHandler{sessions: s, log: log}
}

// HandleRefresh is public — auth is the refresh token in the body.
func (h *SessionsHandler) HandleRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.RefreshToken == "" {
		writeError(w, http.StatusBadRequest, "refresh_token required")
		return
	}
	ua := r.Header.Get("User-Agent")
	ip := clientIP(r)
	pair, err := h.sessions.Refresh(r.Context(), req.RefreshToken, ua, ip)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid refresh token")
		return
	}
	writeJSON(w, http.StatusOK, pair)
}

func (h *SessionsHandler) HandleList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET only")
		return
	}
	uid := UserID(r.Context())
	sess, err := h.sessions.List(r.Context(), uid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list failed")
		return
	}
	// Strip token hash before returning.
	type view struct {
		ID        string `json:"id"`
		UserAgent string `json:"user_agent"`
		IP        string `json:"ip"`
		CreatedAt string `json:"created_at"`
		ExpiresAt string `json:"expires_at"`
	}
	out := make([]view, 0, len(sess))
	for _, s := range sess {
		out = append(out, view{
			ID: s.ID, UserAgent: s.UserAgent, IP: s.IP,
			CreatedAt: s.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			ExpiresAt: s.ExpiresAt.Format("2006-01-02T15:04:05Z07:00"),
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *SessionsHandler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeError(w, http.StatusMethodNotAllowed, "DELETE only")
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/admin/api/auth/sessions/")
	if id == "" {
		writeError(w, http.StatusBadRequest, "session id required")
		return
	}
	if err := h.sessions.RevokeByID(r.Context(), id, UserID(r.Context())); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// HandleLogout revokes the refresh token sent in the body (if any).
func (h *SessionsHandler) HandleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.RefreshToken != "" {
		_ = h.sessions.Revoke(r.Context(), req.RefreshToken)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func clientIP(r *http.Request) string {
	if v := r.Header.Get("X-Forwarded-For"); v != "" {
		if idx := strings.Index(v, ","); idx >= 0 {
			return strings.TrimSpace(v[:idx])
		}
		return strings.TrimSpace(v)
	}
	host := r.RemoteAddr
	if idx := strings.LastIndex(host, ":"); idx >= 0 {
		return host[:idx]
	}
	return host
}
