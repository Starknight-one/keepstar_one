package handlers

import (
	"encoding/json"
	"net/http"

	"keepstar_v4/internal/domain"
	"keepstar_v4/internal/logger"
	"keepstar_v4/internal/usecases"
)

// ActionHandler handles user action requests (like, cart)
type ActionHandler struct {
	actionUC *usecases.ActionUseCase
	log      *logger.Logger
}

// NewActionHandler creates an action handler
func NewActionHandler(actionUC *usecases.ActionUseCase, log *logger.Logger) *ActionHandler {
	return &ActionHandler{
		actionUC: actionUC,
		log:      log,
	}
}

// ActionRequest is the request body for action
type ActionRequest struct {
	SessionID  string `json:"sessionId"`
	Action     string `json:"action"`     // "toggle_like" | "add_to_cart" | "remove_from_cart" | "update_cart_qty"
	EntityType string `json:"entityType"` // "product" | "service"
	EntityId   string `json:"entityId"`
	Quantity   int    `json:"quantity,omitempty"` // for update_cart_qty
}

// ActionResponseBody is the response body for action
type ActionResponseBody struct {
	Success bool                `json:"success"`
	Actions domain.StateActions `json:"actions"`
}

// HandleAction handles POST /api/v1/actions
func (h *ActionHandler) HandleAction(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if sc := domain.SpanFromContext(ctx); sc != nil {
		endSpan := sc.Start("handler.action")
		defer endSpan()
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.SessionID == "" {
		http.Error(w, "sessionId is required", http.StatusBadRequest)
		return
	}
	if req.Action == "" {
		http.Error(w, "action is required", http.StatusBadRequest)
		return
	}
	if req.EntityId == "" {
		http.Error(w, "entityId is required", http.StatusBadRequest)
		return
	}

	ctx = logger.WithSessionID(ctx, req.SessionID)
	r = r.WithContext(ctx)

	result, err := h.actionUC.Execute(r.Context(), usecases.ActionRequest{
		SessionID:  req.SessionID,
		Action:     req.Action,
		EntityType: domain.EntityType(req.EntityType),
		EntityId:   req.EntityId,
		Quantity:   req.Quantity,
	})
	if err != nil {
		h.log.Error("action_failed", "error", err, "action", req.Action, "session_id", req.SessionID)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// sync=true: fire-and-forget, frontend doesn't need the response body
	if r.URL.Query().Get("sync") == "true" {
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}

	writeJSON(w, http.StatusOK, ActionResponseBody{
		Success: result.Success,
		Actions: result.Actions,
	})
}

// ActionViewRequest is the request body for action view
type ActionViewRequest struct {
	SessionID string `json:"sessionId"`
	View      string `json:"view"` // "liked" | "cart"
}

// ActionViewResponseBody is the response body for action view
type ActionViewResponseBody struct {
	Success   bool               `json:"success"`
	Formation *FormationResponse `json:"formation,omitempty"`
	Actions   domain.StateActions `json:"actions"`
}

// HandleActionView handles POST /api/v1/actions/view
func (h *ActionHandler) HandleActionView(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if sc := domain.SpanFromContext(ctx); sc != nil {
		endSpan := sc.Start("handler.action_view")
		defer endSpan()
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ActionViewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.SessionID == "" {
		http.Error(w, "sessionId is required", http.StatusBadRequest)
		return
	}
	if req.View == "" {
		http.Error(w, "view is required", http.StatusBadRequest)
		return
	}

	ctx = logger.WithSessionID(ctx, req.SessionID)
	r = r.WithContext(ctx)

	result, err := h.actionUC.BuildView(r.Context(), req.SessionID, req.View)
	if err != nil {
		h.log.Error("action_view_failed", "error", err, "view", req.View, "session_id", req.SessionID)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	resp := ActionViewResponseBody{
		Success: true,
		Actions: result.Actions,
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
