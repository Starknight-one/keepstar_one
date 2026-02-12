package handlers

import (
	"encoding/json"
	"net/http"

	"keepstar/internal/domain"
	"keepstar/internal/logger"
	"keepstar/internal/usecases"
)

// ChatHandler handles chat endpoints
type ChatHandler struct {
	sendMessage *usecases.SendMessageUseCase
	log         *logger.Logger
}

// NewChatHandler creates a new chat handler
func NewChatHandler(sendMessage *usecases.SendMessageUseCase, log *logger.Logger) *ChatHandler {
	return &ChatHandler{
		sendMessage: sendMessage,
		log:         log,
	}
}

// ChatRequest is the request body for POST /api/v1/chat
type ChatRequest struct {
	SessionID string `json:"sessionId,omitempty"`
	TenantID  string `json:"tenantId,omitempty"`
	Message   string `json:"message"`
}

// ChatResponse is the response body for POST /api/v1/chat
type ChatResponse struct {
	SessionID string `json:"sessionId"`
	Response  string `json:"response"`
	LatencyMs int    `json:"latencyMs,omitempty"`
}

// HandleChat handles POST /api/v1/chat
func (h *ChatHandler) HandleChat(w http.ResponseWriter, r *http.Request) {
	if sc := domain.SpanFromContext(r.Context()); sc != nil {
		endSpan := sc.Start("handler.chat")
		defer endSpan()
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Message == "" {
		http.Error(w, "Message is required", http.StatusBadRequest)
		return
	}

	// Default tenant if not provided
	if req.TenantID == "" {
		req.TenantID = "default"
	}

	result, err := h.sendMessage.Execute(r.Context(), usecases.SendMessageRequest{
		SessionID: req.SessionID,
		TenantID:  req.TenantID,
		Message:   req.Message,
	})
	if err != nil {
		h.log.Error("chat_error", "error", err)
		http.Error(w, "Failed to get AI response", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ChatResponse{
		SessionID: result.SessionID,
		Response:  result.Response,
		LatencyMs: result.LatencyMs,
	})
}
