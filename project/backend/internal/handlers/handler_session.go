package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"keepstar/internal/domain"
	"keepstar/internal/ports"
)

// SessionHandler handles session endpoints
type SessionHandler struct {
	cache ports.CachePort
}

// NewSessionHandler creates a new session handler
func NewSessionHandler(cache ports.CachePort) *SessionHandler {
	return &SessionHandler{cache: cache}
}

// SessionResponse is the response for GET /api/v1/session/{id}
type SessionResponse struct {
	ID             string            `json:"id"`
	Status         string            `json:"status"`
	Messages       []MessageResponse `json:"messages"`
	StartedAt      time.Time         `json:"startedAt"`
	LastActivityAt time.Time         `json:"lastActivityAt"`
}

// MessageResponse represents a message in the session
type MessageResponse struct {
	ID        string     `json:"id"`
	Role      string     `json:"role"`
	Content   string     `json:"content"`
	SentAt    time.Time  `json:"sentAt"`
	LatencyMs int        `json:"latencyMs,omitempty"`
}

// HandleGetSession handles GET /api/v1/session/{id}
func (h *SessionHandler) HandleGetSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if h.cache == nil {
		http.Error(w, "Session storage not available", http.StatusServiceUnavailable)
		return
	}

	// Extract session ID from path: /api/v1/session/{id}
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/session/")
	sessionID := strings.TrimSuffix(path, "/")

	if sessionID == "" {
		http.Error(w, "Session ID is required", http.StatusBadRequest)
		return
	}

	session, err := h.cache.GetSession(r.Context(), sessionID)
	if err == domain.ErrSessionNotFound {
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "Failed to get session", http.StatusInternalServerError)
		return
	}

	// Convert to response
	messages := make([]MessageResponse, len(session.Messages))
	for i, msg := range session.Messages {
		messages[i] = MessageResponse{
			ID:        msg.ID,
			Role:      string(msg.Role),
			Content:   msg.Content,
			SentAt:    msg.SentAt,
			LatencyMs: msg.LatencyMs,
		}
	}

	resp := SessionResponse{
		ID:             session.ID,
		Status:         string(session.Status),
		Messages:       messages,
		StartedAt:      session.StartedAt,
		LastActivityAt: session.LastActivityAt,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
