package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"keepstar/internal/domain"
	"keepstar/internal/usecases"
)

// NavigationHandler handles navigation requests (expand/back)
type NavigationHandler struct {
	expandUC *usecases.ExpandUseCase
	backUC   *usecases.BackUseCase
}

// NewNavigationHandler creates a navigation handler
func NewNavigationHandler(expandUC *usecases.ExpandUseCase, backUC *usecases.BackUseCase) *NavigationHandler {
	return &NavigationHandler{
		expandUC: expandUC,
		backUC:   backUC,
	}
}

// ExpandRequest is the request body for expand
type ExpandRequest struct {
	SessionID  string `json:"sessionId"`
	EntityType string `json:"entityType"`
	EntityID   string `json:"entityId"`
}

// ExpandResponse is the response body for expand
type NavigationResponse struct {
	Success   bool               `json:"success"`
	Formation *FormationResponse `json:"formation,omitempty"`
	ViewMode  string             `json:"viewMode"`
	Focused   *domain.EntityRef  `json:"focused,omitempty"`
	StackSize int                `json:"stackSize"`
	CanGoBack bool               `json:"canGoBack"`
}

// BackRequest is the request body for back
type BackRequest struct {
	SessionID string `json:"sessionId"`
}

// HandleExpand handles POST /api/v1/navigation/expand
func (h *NavigationHandler) HandleExpand(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ExpandRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.SessionID == "" {
		http.Error(w, "sessionId is required", http.StatusBadRequest)
		return
	}
	if req.EntityType == "" {
		http.Error(w, "entityType is required", http.StatusBadRequest)
		return
	}
	if req.EntityID == "" {
		http.Error(w, "entityId is required", http.StatusBadRequest)
		return
	}

	turnID := uuid.New().String()
	result, err := h.expandUC.Execute(r.Context(), usecases.ExpandRequest{
		SessionID:  req.SessionID,
		EntityType: domain.EntityType(req.EntityType),
		EntityID:   req.EntityID,
		TurnID:     turnID,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	resp := NavigationResponse{
		Success:   result.Success,
		ViewMode:  string(result.ViewMode),
		Focused:   result.Focused,
		StackSize: result.StackSize,
		CanGoBack: result.StackSize > 0,
	}

	if result.Formation != nil {
		resp.Formation = &FormationResponse{
			Mode:    string(result.Formation.Mode),
			Grid:    result.Formation.Grid,
			Widgets: result.Formation.Widgets,
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// HandleBack handles POST /api/v1/navigation/back
func (h *NavigationHandler) HandleBack(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req BackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.SessionID == "" {
		http.Error(w, "sessionId is required", http.StatusBadRequest)
		return
	}

	backTurnID := uuid.New().String()
	result, err := h.backUC.Execute(r.Context(), usecases.BackRequest{
		SessionID: req.SessionID,
		TurnID:    backTurnID,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	resp := NavigationResponse{
		Success:   result.Success,
		ViewMode:  string(result.ViewMode),
		Focused:   result.Focused,
		StackSize: result.StackSize,
		CanGoBack: result.CanGoBack,
	}

	if result.Formation != nil {
		resp.Formation = &FormationResponse{
			Mode:    string(result.Formation.Mode),
			Grid:    result.Formation.Grid,
			Widgets: result.Formation.Widgets,
		}
	}

	writeJSON(w, http.StatusOK, resp)
}
