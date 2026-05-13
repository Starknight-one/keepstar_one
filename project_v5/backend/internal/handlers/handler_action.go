package handlers

import (
	"encoding/json"
	"net/http"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/ports"
)

// ActionHandler owns POST /api/v1/actions — the click-handler for the
// four backend-resolved user-action kinds (like / unlike / cart_add /
// cart_remove). The other five kinds in domain.UserActionKind
// (drill_detail / back / open_category / external_link / search) are
// frontend-only or routed through /navigation/{expand,back}; this
// endpoint rejects them with 400.
//
// Wire shape:
//
//	POST /api/v1/actions[?sync=true]
//	{"sessionId":"...","kind":"like","entity":{"type":"product","id":"..."},"params":{...}}
//
// On ?sync=true the response body is empty (V4-style fire-and-forget
// pattern). Default is the full body so the frontend can refresh its
// liked / cart cache from the canonical state in one round-trip.
type ActionHandler struct {
	state ports.StatePort
}

// NewActionHandler constructs the handler with its single dep.
func NewActionHandler(state ports.StatePort) *ActionHandler {
	return &ActionHandler{state: state}
}

type actionRequest struct {
	SessionID string                 `json:"sessionId"`
	Kind      domain.UserActionKind  `json:"kind"`
	Entity    *domain.EntityRef      `json:"entity,omitempty"`
	Params    map[string]interface{} `json:"params,omitempty"`
}

type actionResponse struct {
	Success bool                `json:"success"`
	Actions domain.StateActions `json:"actions"`
}

// Action handles POST /api/v1/actions.
func (h *ActionHandler) Action(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req actionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.SessionID == "" {
		http.Error(w, "sessionId is required", http.StatusBadRequest)
		return
	}
	if !req.Kind.IsKnown() {
		http.Error(w, "unknown action kind", http.StatusBadRequest)
		return
	}
	if !req.Kind.IsBackendHandled() {
		http.Error(w, "action kind handled by client, not /actions", http.StatusBadRequest)
		return
	}
	if req.Entity == nil || req.Entity.ID == "" {
		http.Error(w, "entity is required for like/cart actions", http.StatusBadRequest)
		return
	}

	// Load current actions zone, mutate, persist.
	state, err := h.state.GetState(r.Context(), req.SessionID)
	if err != nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	actions := applyUserAction(state.Actions, req.Kind, req.Entity, req.Params)

	deltaInfo := domain.DeltaInfo{
		Trigger:   domain.TriggerWidgetAction,
		Source:    domain.SourceUser,
		ActorID:   "user",
		DeltaType: domain.DeltaTypeUpdate,
		Path:      "actions",
		Action: domain.Action{
			Type:   userActionToDeltaActionType(req.Kind),
			Tool:   "user_action",
			Params: deltaParams(req),
		},
	}
	if _, err := h.state.UpdateActions(r.Context(), req.SessionID, actions, deltaInfo); err != nil {
		http.Error(w, "update actions failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if r.URL.Query().Get("sync") == "true" {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeJSON(w, http.StatusOK, actionResponse{
		Success: true,
		Actions: actions,
	})
}

// applyUserAction returns a new StateActions with the requested
// mutation applied. Pure function — does not touch storage.
//
// like      — ensures entity.ID is in LikedIds (no-op if already).
// unlike    — removes entity.ID from LikedIds.
// cart_add  — increments quantity for matching CartItem, or appends
//             a new one with quantity = max(1, params.quantity).
// cart_remove — drops matching CartItem entirely (quantity-decrement
//             semantics are out of scope for chunk 11; remove is the
//             V4-equivalent default).
func applyUserAction(in domain.StateActions, kind domain.UserActionKind, entity *domain.EntityRef, params map[string]interface{}) domain.StateActions {
	out := domain.StateActions{
		LikedIds:  append([]string(nil), in.LikedIds...),
		CartItems: append([]domain.CartItem(nil), in.CartItems...),
	}
	switch kind {
	case domain.UserActionLike:
		if !containsString(out.LikedIds, entity.ID) {
			out.LikedIds = append(out.LikedIds, entity.ID)
		}
	case domain.UserActionUnlike:
		out.LikedIds = removeString(out.LikedIds, entity.ID)
	case domain.UserActionCartAdd:
		qty := readQuantity(params)
		if idx := findCartItem(out.CartItems, entity); idx >= 0 {
			out.CartItems[idx].Quantity += qty
		} else {
			out.CartItems = append(out.CartItems, domain.CartItem{
				EntityType: entity.Type,
				EntityId:   entity.ID,
				Quantity:   qty,
			})
		}
	case domain.UserActionCartRemove:
		if idx := findCartItem(out.CartItems, entity); idx >= 0 {
			out.CartItems = append(out.CartItems[:idx], out.CartItems[idx+1:]...)
		}
	}
	return out
}

// userActionToDeltaActionType maps a user-facing kind to the existing
// domain.ActionType enum so the delta stream stays consistent with V4.
func userActionToDeltaActionType(kind domain.UserActionKind) domain.ActionType {
	switch kind {
	case domain.UserActionLike:
		return domain.ActionLike
	case domain.UserActionUnlike:
		return domain.ActionUnlike
	case domain.UserActionCartAdd:
		return domain.ActionCartAdd
	case domain.UserActionCartRemove:
		return domain.ActionCartRemove
	}
	return ""
}

func deltaParams(req actionRequest) map[string]interface{} {
	out := map[string]interface{}{
		"kind": string(req.Kind),
	}
	if req.Entity != nil {
		out["entityType"] = string(req.Entity.Type)
		out["entityId"] = req.Entity.ID
	}
	if len(req.Params) > 0 {
		out["params"] = req.Params
	}
	return out
}

func containsString(s []string, want string) bool {
	for _, v := range s {
		if v == want {
			return true
		}
	}
	return false
}

func removeString(s []string, want string) []string {
	out := s[:0]
	for _, v := range s {
		if v != want {
			out = append(out, v)
		}
	}
	return out
}

func findCartItem(items []domain.CartItem, entity *domain.EntityRef) int {
	for i, ci := range items {
		if ci.EntityType == entity.Type && ci.EntityId == entity.ID {
			return i
		}
	}
	return -1
}

func readQuantity(params map[string]interface{}) int {
	if params == nil {
		return 1
	}
	raw, ok := params["quantity"]
	if !ok {
		return 1
	}
	switch v := raw.(type) {
	case int:
		if v < 1 {
			return 1
		}
		return v
	case float64:
		if v < 1 {
			return 1
		}
		return int(v)
	}
	return 1
}
