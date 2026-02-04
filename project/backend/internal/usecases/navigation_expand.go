package usecases

import (
	"context"
	"fmt"
	"time"

	"keepstar/internal/domain"
	"keepstar/internal/ports"
	"keepstar/internal/presets"
	"keepstar/internal/tools"
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
	statePort      ports.StatePort
	presetRegistry *presets.PresetRegistry
}

// NewExpandUseCase creates a new ExpandUseCase
func NewExpandUseCase(statePort ports.StatePort, presetRegistry *presets.PresetRegistry) *ExpandUseCase {
	return &ExpandUseCase{
		statePort:      statePort,
		presetRegistry: presetRegistry,
	}
}

// Execute expands a widget to detail view
func (uc *ExpandUseCase) Execute(ctx context.Context, req ExpandRequest) (*ExpandResponse, error) {
	// 1. Get current state
	state, err := uc.statePort.GetState(ctx, req.SessionID)
	if err != nil {
		return nil, fmt.Errorf("get state: %w", err)
	}

	// 2. Find entity by ID and get preset
	var entity interface{}
	var preset domain.Preset
	var found bool

	if req.EntityType == domain.EntityTypeProduct {
		for _, p := range state.Current.Data.Products {
			if p.ID == req.EntityID {
				entity = p
				break
			}
		}
		preset, found = uc.presetRegistry.Get(domain.PresetProductDetail)
	} else {
		for _, s := range state.Current.Data.Services {
			if s.ID == req.EntityID {
				entity = s
				break
			}
		}
		preset, found = uc.presetRegistry.Get(domain.PresetServiceDetail)
	}

	if entity == nil {
		return nil, fmt.Errorf("entity not found: %s", req.EntityID)
	}
	if !found {
		return nil, fmt.Errorf("detail preset not found for entity type: %s", req.EntityType)
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

	// 5. Build detail formation
	formation := uc.buildDetailFormation(preset, entity, req.EntityType)

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

// buildDetailFormation creates a formation for a single entity in detail view
func (uc *ExpandUseCase) buildDetailFormation(preset domain.Preset, entity interface{}, entityType domain.EntityType) *domain.FormationWithData {
	if entityType == domain.EntityTypeProduct {
		p := entity.(domain.Product)
		return tools.BuildFormation(preset, 1, func(i int) (tools.FieldGetter, tools.CurrencyGetter, tools.IDGetter) {
			return productFieldGetter(p), func() string { return p.Currency }, func() string { return p.ID }
		})
	}
	s := entity.(domain.Service)
	return tools.BuildFormation(preset, 1, func(i int) (tools.FieldGetter, tools.CurrencyGetter, tools.IDGetter) {
		return serviceFieldGetter(s), func() string { return s.Currency }, func() string { return s.ID }
	})
}

// productFieldGetter returns a FieldGetter for Product
func productFieldGetter(p domain.Product) tools.FieldGetter {
	return func(fieldName string) interface{} {
		switch fieldName {
		case "id":
			return p.ID
		case "name":
			return nonEmpty(p.Name)
		case "description":
			return nonEmpty(p.Description)
		case "price":
			return p.Price
		case "images":
			if len(p.Images) == 0 {
				return nil
			}
			return p.Images
		case "rating":
			if p.Rating == 0 {
				return nil
			}
			return p.Rating
		case "brand":
			return nonEmpty(p.Brand)
		case "category":
			return nonEmpty(p.Category)
		case "stockQuantity":
			if p.StockQuantity == 0 {
				return nil
			}
			return p.StockQuantity
		case "tags":
			if len(p.Tags) == 0 {
				return nil
			}
			return p.Tags
		case "attributes":
			if len(p.Attributes) == 0 {
				return nil
			}
			return p.Attributes
		default:
			return nil
		}
	}
}

// serviceFieldGetter returns a FieldGetter for Service
func serviceFieldGetter(s domain.Service) tools.FieldGetter {
	return func(fieldName string) interface{} {
		switch fieldName {
		case "id":
			return s.ID
		case "name":
			return nonEmpty(s.Name)
		case "description":
			return nonEmpty(s.Description)
		case "price":
			return s.Price
		case "images":
			if len(s.Images) == 0 {
				return nil
			}
			return s.Images
		case "rating":
			if s.Rating == 0 {
				return nil
			}
			return s.Rating
		case "duration":
			return nonEmpty(s.Duration)
		case "provider":
			return nonEmpty(s.Provider)
		case "availability":
			return nonEmpty(s.Availability)
		case "attributes":
			if len(s.Attributes) == 0 {
				return nil
			}
			return s.Attributes
		default:
			return nil
		}
	}
}

// nonEmpty returns nil if string is empty, otherwise returns the string
func nonEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
