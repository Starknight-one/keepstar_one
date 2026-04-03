package usecases

import (
	"context"
	"fmt"

	"keepstar_v4/internal/domain"
	"keepstar_v4/internal/ports"
)

// ActionRequest is the input for an action operation
type ActionRequest struct {
	SessionID  string
	Action     string // "toggle_like" | "add_to_cart" | "remove_from_cart" | "update_cart_qty"
	EntityType domain.EntityType
	EntityId   string
	Quantity   int
}

// ActionResponse is the output from an action operation
type ActionResponse struct {
	Success bool
	Actions domain.StateActions
}

// ActionUseCase handles user actions (like, cart)
type ActionUseCase struct {
	statePort ports.StatePort
}

// NewActionUseCase creates a new ActionUseCase
func NewActionUseCase(statePort ports.StatePort) *ActionUseCase {
	return &ActionUseCase{
		statePort: statePort,
	}
}

// Execute processes an action (toggle like, add/remove/update cart)
func (uc *ActionUseCase) Execute(ctx context.Context, req ActionRequest) (*ActionResponse, error) {
	if sc := domain.SpanFromContext(ctx); sc != nil {
		endSpan := sc.Start("usecase.action")
		defer endSpan()
	}

	// 1. Get current state
	state, err := uc.statePort.GetState(ctx, req.SessionID)
	if err != nil {
		return nil, fmt.Errorf("get state: %w", err)
	}

	// 2. Copy actions and apply mutation
	actions := state.Actions
	var actionType domain.ActionType

	switch req.Action {
	case "toggle_like":
		actions, actionType = uc.toggleLike(actions, req.EntityId)
	case "add_to_cart":
		actions, actionType = uc.addToCart(actions, req.EntityType, req.EntityId)
	case "remove_from_cart":
		actions, actionType = uc.removeFromCart(actions, req.EntityId)
	case "update_cart_qty":
		actions, actionType = uc.updateCartQty(actions, req.EntityId, req.Quantity)
	default:
		return nil, fmt.Errorf("unknown action: %s", req.Action)
	}

	// 3. Zone-write with delta
	info := domain.DeltaInfo{
		Trigger:   domain.TriggerWidgetAction,
		Source:    domain.SourceUser,
		ActorID:   "user_action",
		DeltaType: domain.DeltaTypeUpdate,
		Path:      "actions",
		Action: domain.Action{
			Type:   actionType,
			Params: map[string]interface{}{"entityId": req.EntityId, "entityType": string(req.EntityType)},
		},
		Result: domain.ResultMeta{
			Count: len(actions.LikedIds) + len(actions.CartItems),
		},
	}

	if _, err := uc.statePort.UpdateActions(ctx, req.SessionID, actions, info); err != nil {
		return nil, fmt.Errorf("update actions: %w", err)
	}

	return &ActionResponse{
		Success: true,
		Actions: actions,
	}, nil
}

func (uc *ActionUseCase) toggleLike(actions domain.StateActions, entityId string) (domain.StateActions, domain.ActionType) {
	for i, id := range actions.LikedIds {
		if id == entityId {
			// Remove (unlike)
			actions.LikedIds = append(actions.LikedIds[:i], actions.LikedIds[i+1:]...)
			return actions, domain.ActionUnlike
		}
	}
	// Add (like)
	actions.LikedIds = append(actions.LikedIds, entityId)
	return actions, domain.ActionLike
}

func (uc *ActionUseCase) addToCart(actions domain.StateActions, entityType domain.EntityType, entityId string) (domain.StateActions, domain.ActionType) {
	for i, item := range actions.CartItems {
		if item.EntityId == entityId {
			actions.CartItems[i].Quantity++
			return actions, domain.ActionCartUpdate
		}
	}
	actions.CartItems = append(actions.CartItems, domain.CartItem{
		EntityType: entityType,
		EntityId:   entityId,
		Quantity:   1,
	})
	return actions, domain.ActionCartAdd
}

func (uc *ActionUseCase) removeFromCart(actions domain.StateActions, entityId string) (domain.StateActions, domain.ActionType) {
	for i, item := range actions.CartItems {
		if item.EntityId == entityId {
			actions.CartItems = append(actions.CartItems[:i], actions.CartItems[i+1:]...)
			return actions, domain.ActionCartRemove
		}
	}
	return actions, domain.ActionCartRemove
}

func (uc *ActionUseCase) updateCartQty(actions domain.StateActions, entityId string, quantity int) (domain.StateActions, domain.ActionType) {
	for i, item := range actions.CartItems {
		if item.EntityId == entityId {
			if quantity <= 0 {
				actions.CartItems = append(actions.CartItems[:i], actions.CartItems[i+1:]...)
			} else {
				actions.CartItems[i].Quantity = quantity
			}
			return actions, domain.ActionCartUpdate
		}
	}
	return actions, domain.ActionCartUpdate
}
