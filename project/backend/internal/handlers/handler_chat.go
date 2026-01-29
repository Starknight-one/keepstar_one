package handlers

import (
	"net/http"

	"keepstar/internal/usecases"
)

// ChatHandler handles chat endpoints
type ChatHandler struct {
	analyzeQuery   *usecases.AnalyzeQueryUseCase
	composeWidgets *usecases.ComposeWidgetsUseCase
	executeSearch  *usecases.ExecuteSearchUseCase
}

// NewChatHandler creates a new chat handler
func NewChatHandler(
	analyzeQuery *usecases.AnalyzeQueryUseCase,
	composeWidgets *usecases.ComposeWidgetsUseCase,
	executeSearch *usecases.ExecuteSearchUseCase,
) *ChatHandler {
	return &ChatHandler{
		analyzeQuery:   analyzeQuery,
		composeWidgets: composeWidgets,
		executeSearch:  executeSearch,
	}
}

// ChatRequest is the request body for POST /api/v1/chat
type ChatRequest struct {
	SessionID string `json:"sessionId,omitempty"`
	Message   string `json:"message"`
}

// ChatResponse is the response body for POST /api/v1/chat
type ChatResponse struct {
	SessionID string      `json:"sessionId"`
	Response  interface{} `json:"response"`
}

// HandleChat handles POST /api/v1/chat
func (h *ChatHandler) HandleChat(w http.ResponseWriter, r *http.Request) {
	// TODO: implement
}
