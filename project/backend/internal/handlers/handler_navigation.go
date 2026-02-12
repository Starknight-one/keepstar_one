package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"keepstar/internal/domain"
	"keepstar/internal/logger"
	"keepstar/internal/usecases"
)

// NavigationHandler handles navigation requests (expand/back)
type NavigationHandler struct {
	expandUC *usecases.ExpandUseCase
	backUC   *usecases.BackUseCase
	log      *logger.Logger
}

// NewNavigationHandler creates a navigation handler
func NewNavigationHandler(expandUC *usecases.ExpandUseCase, backUC *usecases.BackUseCase, log *logger.Logger) *NavigationHandler {
	return &NavigationHandler{
		expandUC: expandUC,
		backUC:   backUC,
		log:      log,
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
	ctx := r.Context()
	if sc := domain.SpanFromContext(ctx); sc != nil {
		endSpan := sc.Start("handler.expand")
		defer endSpan()
	}

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

	ctx = logger.WithSessionID(ctx, req.SessionID)
	r = r.WithContext(ctx)

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

	// sync=true: frontend already has the formation, just sync backend state
	if r.URL.Query().Get("sync") == "true" {
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
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
	ctx := r.Context()
	if sc := domain.SpanFromContext(ctx); sc != nil {
		endSpan := sc.Start("handler.back")
		defer endSpan()
	}

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

	ctx = logger.WithSessionID(ctx, req.SessionID)
	r = r.WithContext(ctx)

	backTurnID := uuid.New().String()
	result, err := h.backUC.Execute(r.Context(), usecases.BackRequest{
		SessionID: req.SessionID,
		TurnID:    backTurnID,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// sync=true: frontend already has the formation from stack, just sync backend state
	if r.URL.Query().Get("sync") == "true" {
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
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
