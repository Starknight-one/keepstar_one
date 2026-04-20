package usecases

import (
	"context"
	"fmt"
	"time"

	"keepstar_v4/internal/domain"
	engine_v4 "keepstar_v4/internal/engine_v4"
	"keepstar_v4/internal/ports"
	"keepstar_v4/internal/tools"
)

// ExpandRequest is the request for expanding a widget to detail view
type ExpandRequest struct {
	SessionID  string
	EntityType domain.EntityType
	EntityID   string
	TurnID     string // Turn ID for delta grouping
}

// ExpandResponse is the response from expand operation
type ExpandResponse struct {
	Success   bool
	Formation *domain.FormationWithData
	ViewMode  domain.ViewMode
	Focused   *domain.EntityRef
	StackSize int
}

// ExpandUseCase handles expanding a widget to detail view
type ExpandUseCase struct {
	statePort ports.StatePort
	engine    *engine_v4.Engine
}

// NewExpandUseCase creates a new ExpandUseCase
func NewExpandUseCase(statePort ports.StatePort, eng *engine_v4.Engine) *ExpandUseCase {
	return &ExpandUseCase{
		statePort: statePort,
		engine:    eng,
	}
}

// Execute expands a widget to detail view
func (uc *ExpandUseCase) Execute(ctx context.Context, req ExpandRequest) (*ExpandResponse, error) {
	if sc := domain.SpanFromContext(ctx); sc != nil {
		endSpan := sc.Start("usecase.expand")
		defer endSpan()
	}

	// 1. Get current state
	state, err := uc.statePort.GetState(ctx, req.SessionID)
	if err != nil {
		return nil, fmt.Errorf("get state: %w", err)
	}

	// 2. Find entity by ID and convert to data map
	var dataMap map[string]interface{}

	if req.EntityType == domain.EntityTypeProduct {
		for _, p := range state.Current.Data.Products {
			if p.ID == req.EntityID {
				dataMap = tools.ProductToMap(p)
				break
			}
		}
	} else {
		for _, s := range state.Current.Data.Services {
			if s.ID == req.EntityID {
				dataMap = tools.ServiceToMap(s)
				break
			}
		}
	}

	if dataMap == nil {
		return nil, fmt.Errorf("entity not found: %s", req.EntityID)
	}

	// 3. Build refs from current data for snapshot
	refs := buildEntityRefs(state.Current.Data)

	// 4. Push current view to stack
	snapshot := &domain.ViewSnapshot{
		Mode:      state.View.Mode,
		Focused:   state.View.Focused,
		Refs:      refs,
		Step:      state.Step,
		CreatedAt: time.Now(),
	}
	if err := uc.statePort.PushView(ctx, req.SessionID, snapshot); err != nil {
		return nil, fmt.Errorf("push view: %w", err)
	}

	// 5. Build detail formation using V4 engine
	output := uc.engine.Execute(engine_v4.ExecuteInput{
		Ops:        engine_v4.ProductDetailOps(),
		Layout:     "single",
		EntityType: string(req.EntityType),
		Data:       []map[string]interface{}{dataMap},
	})
	formation := output.Formation

	// 6. Zone-write: UpdateView (view zone)
	stack, _ := uc.statePort.GetViewStack(ctx, req.SessionID)
	newView := domain.ViewState{
		Mode:    domain.ViewModeDetail,
		Focused: &domain.EntityRef{Type: req.EntityType, ID: req.EntityID},
	}
	viewInfo := domain.DeltaInfo{
		TurnID:    req.TurnID,
		Trigger:   domain.TriggerWidgetAction,
		Source:    domain.SourceUser,
		ActorID:   "user_expand",
		DeltaType: domain.DeltaTypePush,
		Path:      "view",
	}
	if _, err := uc.statePort.UpdateView(ctx, req.SessionID, newView, stack, viewInfo); err != nil {
		return nil, fmt.Errorf("update view: %w", err)
	}

	// 7. Zone-write: UpdateTemplate (template zone)
	template := map[string]interface{}{
		"formation": formation,
	}
	templateInfo := domain.DeltaInfo{
		TurnID:    req.TurnID,
		Trigger:   domain.TriggerWidgetAction,
		Source:    domain.SourceUser,
		ActorID:   "user_expand",
		DeltaType: domain.DeltaTypeUpdate,
		Path:      "template",
	}
	if _, err := uc.statePort.UpdateTemplate(ctx, req.SessionID, template, templateInfo); err != nil {
		return nil, fmt.Errorf("update template: %w", err)
	}

	return &ExpandResponse{
		Success:   true,
		Formation: formation,
		ViewMode:  newView.Mode,
		Focused:   newView.Focused,
		StackSize: len(stack),
	}, nil
}

// buildEntityRefs creates entity refs from state data
func buildEntityRefs(data domain.StateData) []domain.EntityRef {
	refs := make([]domain.EntityRef, 0, len(data.Products)+len(data.Services))
	for _, p := range data.Products {
		refs = append(refs, domain.EntityRef{Type: domain.EntityTypeProduct, ID: p.ID})
	}
	for _, s := range data.Services {
		refs = append(refs, domain.EntityRef{Type: domain.EntityTypeService, ID: s.ID})
	}
	return refs
}

