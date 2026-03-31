package usecases

import (
	"context"
	"fmt"

	"keepstar/internal/domain"
	"keepstar/internal/engine"
	"keepstar/internal/ports"
	"keepstar/internal/presets"
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
	statePort        ports.StatePort
	fieldDefPort     ports.FieldDefinitionPort
	presetV2Registry *presets.PresetV2Registry
}

// NewBackUseCase creates a new BackUseCase
func NewBackUseCase(statePort ports.StatePort, fieldDefPort ports.FieldDefinitionPort, presetV2Registry *presets.PresetV2Registry) *BackUseCase {
	return &BackUseCase{
		statePort:        statePort,
		fieldDefPort:     fieldDefPort,
		presetV2Registry: presetV2Registry,
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

	// 4. Zone-write: UpdateView (view zone -- restore previous)
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

	eng := engine.NewEngineV2()

	if len(products) > 0 {
		output := eng.Execute(engine.EngineV2Input{
			EntityType: domain.EntityTypeProduct,
			Products:   products,
		})
		return output.Formation
	}

	if len(services) > 0 {
		output := eng.Execute(engine.EngineV2Input{
			EntityType: domain.EntityTypeService,
			Services:   services,
		})
		return output.Formation
	}

	return &domain.FormationWithData{
		Mode:    domain.FormationTypeGrid,
		Widgets: []domain.Widget{},
	}
}
