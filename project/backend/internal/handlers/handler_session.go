package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"keepstar/internal/domain"
	"keepstar/internal/logger"
	"keepstar/internal/ports"
)

// SessionHandler handles session endpoints
type SessionHandler struct {
	cache       ports.CachePort
	statePort   ports.StatePort
	catalogPort ports.CatalogPort
	log         *logger.Logger
}

// NewSessionHandler creates a new session handler
func NewSessionHandler(cache ports.CachePort, statePort ports.StatePort, catalogPort ports.CatalogPort, log *logger.Logger) *SessionHandler {
	return &SessionHandler{cache: cache, statePort: statePort, catalogPort: catalogPort, log: log}
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
	if sc := domain.SpanFromContext(r.Context()); sc != nil {
		endSpan := sc.Start("handler.session_get")
		defer endSpan()
	}

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

	// Check TTL - if session expired, mark as closed
	if session.Status == domain.SessionStatusActive && time.Since(session.LastActivityAt) > domain.SessionTTL {
		session.Status = domain.SessionStatusClosed
		now := time.Now()
		session.EndedAt = &now
		session.UpdatedAt = now
		// Save updated status (best effort, don't fail the request)
		_ = h.cache.SaveSession(r.Context(), session)
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

// InitSessionResponse is the response for POST /api/v1/session/init
type InitSessionResponse struct {
	SessionID string              `json:"sessionId"`
	Tenant    *InitTenantResponse `json:"tenant"`
	Greeting  string              `json:"greeting"`
}

// InitTenantResponse is the tenant info in init response
type InitTenantResponse struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
}

// HandleInitSession handles POST /api/v1/session/init
// Creates a new session, seeds tenant in state, returns greeting.
func (h *SessionHandler) HandleInitSession(w http.ResponseWriter, r *http.Request) {
	if sc := domain.SpanFromContext(r.Context()); sc != nil {
		endSpan := sc.Start("handler.session_init")
		defer endSpan()
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if h.statePort == nil {
		http.Error(w, "State storage not available", http.StatusServiceUnavailable)
		return
	}

	// Resolve tenant from context (set by middleware)
	tenant := GetTenantFromContext(r.Context())

	// Generate session ID
	sessionID := uuid.New().String()

	// Create session record FIRST (FK: chat_session_state.session_id → chat_sessions.id)
	if h.cache != nil {
		now := time.Now()
		session := &domain.Session{
			ID:             sessionID,
			Status:         domain.SessionStatusActive,
			StartedAt:      now,
			LastActivityAt: now,
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		if err := h.cache.SaveSession(r.Context(), session); err != nil {
			http.Error(w, "Failed to create session", http.StatusInternalServerError)
			return
		}
	}

	// Create session state
	state, err := h.statePort.CreateState(r.Context(), sessionID)
	if err != nil {
		http.Error(w, "Failed to create session: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Seed tenant_slug in state aliases
	if tenant != nil {
		if state.Current.Meta.Aliases == nil {
			state.Current.Meta.Aliases = make(map[string]string)
		}
		state.Current.Meta.Aliases["tenant_slug"] = tenant.Slug
		if err := h.statePort.UpdateState(r.Context(), state); err != nil {
			http.Error(w, "Failed to save session state", http.StatusInternalServerError)
			return
		}

		// Seed catalog digest into conversation history (sent once, cached by Anthropic)
		if h.catalogPort != nil {
			if digest, err := h.catalogPort.GetCatalogDigest(r.Context(), tenant.ID); err == nil && digest != nil {
				digestText := digest.ToPromptText()
				if digestText != "" {
					initialHistory := []domain.LLMMessage{
						{Role: "user", Content: "<catalog>\n" + digestText + "</catalog>"},
						{Role: "assistant", Content: "ok"},
					}
					if err := h.statePort.AppendConversation(r.Context(), sessionID, initialHistory); err != nil {
						h.log.Warn("digest_seed_failed", "session_id", sessionID, "error", err)
					}
				}
			}
		}
	}

	resp := InitSessionResponse{
		SessionID: sessionID,
		Greeting:  "Привет! Чем могу помочь?",
	}
	if tenant != nil {
		resp.Tenant = &InitTenantResponse{
			Slug: tenant.Slug,
			Name: tenant.Name,
		}
	}

	writeJSON(w, http.StatusOK, resp)
}
