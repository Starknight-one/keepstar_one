package usecases

import (
	"context"
	"fmt"

	"keepstar/internal/domain"
	"keepstar/internal/engine"
)

// ActionViewResponse is the output from building an action view
type ActionViewResponse struct {
	Formation *domain.FormationWithData
	Actions   domain.StateActions
}

// BuildView builds a formation view for liked or cart items
func (uc *ActionUseCase) BuildView(ctx context.Context, sessionID string, view string) (*ActionViewResponse, error) {
	if sc := domain.SpanFromContext(ctx); sc != nil {
		endSpan := sc.Start("usecase.action_view")
		defer endSpan()
	}

	state, err := uc.statePort.GetState(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("get state: %w", err)
	}

	switch view {
	case "liked":
		return uc.buildLikedView(state)
	case "cart":
		return uc.buildCartView(state)
	default:
		return nil, fmt.Errorf("unknown view: %s", view)
	}
}

func (uc *ActionUseCase) buildLikedView(state *domain.SessionState) (*ActionViewResponse, error) {
	if len(state.Actions.LikedIds) == 0 {
		return &ActionViewResponse{Actions: state.Actions}, nil
	}

	// Build a set for fast lookup
	likedSet := make(map[string]bool, len(state.Actions.LikedIds))
	for _, id := range state.Actions.LikedIds {
		likedSet[id] = true
	}

	// Filter products by liked IDs
	var products []domain.Product
	for _, p := range state.Current.Data.Products {
		if likedSet[p.ID] {
			products = append(products, p)
		}
	}

	if len(products) == 0 {
		return &ActionViewResponse{Actions: state.Actions}, nil
	}

	// Use V2 engine for grid view
	eng := engine.NewEngineV2()
	output := eng.Execute(engine.EngineV2Input{
		EntityType: domain.EntityTypeProduct,
		Products:   products,
	})
	formation := output.Formation

	return &ActionViewResponse{
		Formation: formation,
		Actions:   state.Actions,
	}, nil
}

func (uc *ActionUseCase) buildCartView(state *domain.SessionState) (*ActionViewResponse, error) {
	if len(state.Actions.CartItems) == 0 {
		return &ActionViewResponse{Actions: state.Actions}, nil
	}

	// Build a map for fast lookup
	cartMap := make(map[string]domain.CartItem, len(state.Actions.CartItems))
	for _, item := range state.Actions.CartItems {
		cartMap[item.EntityId] = item
	}

	// Filter products by cart IDs
	var products []domain.Product
	for _, p := range state.Current.Data.Products {
		if _, ok := cartMap[p.ID]; ok {
			products = append(products, p)
		}
	}

	if len(products) == 0 {
		return &ActionViewResponse{Actions: state.Actions}, nil
	}

	// Use V2 engine for grid view
	eng := engine.NewEngineV2()
	output := eng.Execute(engine.EngineV2Input{
		EntityType: domain.EntityTypeProduct,
		Products:   products,
	})
	formation := output.Formation

	// Add quantity meta to each widget
	if formation != nil {
		for i, w := range formation.Widgets {
			if w.EntityRef != nil {
				if cartItem, ok := cartMap[w.EntityRef.ID]; ok {
					if formation.Widgets[i].Meta == nil {
						formation.Widgets[i].Meta = make(map[string]interface{})
					}
					formation.Widgets[i].Meta["quantity"] = cartItem.Quantity
				}
			}
		}
	}

	return &ActionViewResponse{
		Formation: formation,
		Actions:   state.Actions,
	}, nil
}
