package usecases

import (
	"context"
	"fmt"

	"keepstar/internal/domain"
	"keepstar/internal/ports"
	"keepstar/internal/presets"
	"keepstar/internal/tools"
)

// BackRequest is the request for going back to previous view
type BackRequest struct {
	SessionID string
	TurnID    string // Turn ID for delta grouping
}

// BackResponse is the response from back operation
type BackResponse struct {
	Success   bool
	Formation *domain.FormationWithData
	ViewMode  domain.ViewMode
	Focused   *domain.EntityRef
	StackSize int
	CanGoBack bool
}

// BackUseCase handles going back to previous view
type BackUseCase struct {
	statePort      ports.StatePort
	presetRegistry *presets.PresetRegistry
}

// NewBackUseCase creates a new BackUseCase
func NewBackUseCase(statePort ports.StatePort, presetRegistry *presets.PresetRegistry) *BackUseCase {
	return &BackUseCase{
		statePort:      statePort,
		presetRegistry: presetRegistry,
	}
}

// Execute goes back to the previous view
func (uc *BackUseCase) Execute(ctx context.Context, req BackRequest) (*BackResponse, error) {
	if sc := domain.SpanFromContext(ctx); sc != nil {
		endSpan := sc.Start("usecase.back")
		defer endSpan()
	}

	// 1. Pop from stack
	snapshot, err := uc.statePort.PopView(ctx, req.SessionID)
	if err != nil {
		return nil, fmt.Errorf("pop view: %w", err)
	}
	if snapshot == nil {
		return &BackResponse{Success: true, CanGoBack: false}, nil
	}

	// 2. Get current state
	state, err := uc.statePort.GetState(ctx, req.SessionID)
	if err != nil {
		return nil, fmt.Errorf("get state: %w", err)
	}

	// 3. Rebuild formation from state data using grid preset
	formation := uc.rebuildFormationFromState(state)

	// 4. Zone-write: UpdateView (view zone — restore previous)
	stack, _ := uc.statePort.GetViewStack(ctx, req.SessionID)
	restoredView := domain.ViewState{
		Mode:    snapshot.Mode,
		Focused: snapshot.Focused,
	}
	viewInfo := domain.DeltaInfo{
		TurnID:    req.TurnID,
		Trigger:   domain.TriggerWidgetAction,
		Source:    domain.SourceUser,
		ActorID:   "user_back",
		DeltaType: domain.DeltaTypePop,
		Path:      "view",
	}
	if _, err := uc.statePort.UpdateView(ctx, req.SessionID, restoredView, stack, viewInfo); err != nil {
		return nil, fmt.Errorf("update view: %w", err)
	}

	// 5. Zone-write: UpdateTemplate (template zone)
	template := map[string]interface{}{
		"formation": formation,
	}
	templateInfo := domain.DeltaInfo{
		TurnID:    req.TurnID,
		Trigger:   domain.TriggerWidgetAction,
		Source:    domain.SourceUser,
		ActorID:   "user_back",
		DeltaType: domain.DeltaTypeUpdate,
		Path:      "template",
	}
	if _, err := uc.statePort.UpdateTemplate(ctx, req.SessionID, template, templateInfo); err != nil {
		return nil, fmt.Errorf("update template: %w", err)
	}

	return &BackResponse{
		Success:   true,
		Formation: formation,
		ViewMode:  restoredView.Mode,
		Focused:   restoredView.Focused,
		StackSize: len(stack),
		CanGoBack: len(stack) > 0,
	}, nil
}

// rebuildFormationFromState rebuilds formation from current state data using grid preset
func (uc *BackUseCase) rebuildFormationFromState(state *domain.SessionState) *domain.FormationWithData {
	products := state.Current.Data.Products
	services := state.Current.Data.Services

	// If we have products, use product_grid preset
	if len(products) > 0 {
		preset, _ := uc.presetRegistry.Get(domain.PresetProductGrid)
		return tools.BuildFormation(preset, len(products), func(i int) (tools.FieldGetter, tools.CurrencyGetter, tools.IDGetter) {
			p := products[i]
			return productFieldGetter(p), func() string { return p.Currency }, func() string { return p.ID }
		})
	}

	// If we have services, use service_card preset
	if len(services) > 0 {
		preset, _ := uc.presetRegistry.Get(domain.PresetServiceCard)
		return tools.BuildFormation(preset, len(services), func(i int) (tools.FieldGetter, tools.CurrencyGetter, tools.IDGetter) {
			s := services[i]
			return serviceFieldGetter(s), func() string { return s.Currency }, func() string { return s.ID }
		})
	}

	// Empty formation if no data
	return &domain.FormationWithData{
		Mode:    domain.FormationTypeGrid,
		Widgets: []domain.Widget{},
	}
}
