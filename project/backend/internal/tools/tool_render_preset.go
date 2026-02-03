package tools

import (
	"context"
	"fmt"
	"sort"

	"github.com/google/uuid"
	"keepstar/internal/domain"
	"keepstar/internal/ports"
	"keepstar/internal/presets"
)

// RenderProductPresetTool renders products using a preset
type RenderProductPresetTool struct {
	statePort      ports.StatePort
	presetRegistry *presets.PresetRegistry
}

// NewRenderProductPresetTool creates the tool
func NewRenderProductPresetTool(statePort ports.StatePort, presetRegistry *presets.PresetRegistry) *RenderProductPresetTool {
	return &RenderProductPresetTool{
		statePort:      statePort,
		presetRegistry: presetRegistry,
	}
}

// Definition returns the tool definition for LLM
func (t *RenderProductPresetTool) Definition() domain.ToolDefinition {
	return domain.ToolDefinition{
		Name:        "render_product_preset",
		Description: "Render products from state using a preset template. Call this after search_products.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"preset": map[string]interface{}{
					"type":        "string",
					"enum":        []string{"product_grid", "product_card", "product_compact"},
					"description": "Preset to use: product_grid (for multiple items), product_card (for single detail), product_compact (for list)",
				},
			},
			"required": []string{"preset"},
		},
	}
}

// Execute renders products with preset and writes formation to state
func (t *RenderProductPresetTool) Execute(ctx context.Context, sessionID string, input map[string]interface{}) (*domain.ToolResult, error) {
	presetName, _ := input["preset"].(string)

	preset, ok := t.presetRegistry.Get(domain.PresetName(presetName))
	if !ok {
		return &domain.ToolResult{Content: "error: unknown preset", IsError: true}, nil
	}

	state, err := t.statePort.GetState(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("get state: %w", err)
	}

	products := state.Current.Data.Products
	if len(products) == 0 {
		return &domain.ToolResult{Content: "error: no products in state"}, nil
	}

	// Build formation from preset
	formation := buildFormationFromPreset(preset, products)

	// Store formation in state template
	state.Current.Template = map[string]interface{}{
		"formation": formation,
	}

	if err := t.statePort.UpdateState(ctx, state); err != nil {
		return nil, fmt.Errorf("update state: %w", err)
	}

	return &domain.ToolResult{
		Content: fmt.Sprintf("ok: rendered %d products with %s", len(products), presetName),
	}, nil
}

// RenderServicePresetTool renders services using a preset
type RenderServicePresetTool struct {
	statePort      ports.StatePort
	presetRegistry *presets.PresetRegistry
}

// NewRenderServicePresetTool creates the tool
func NewRenderServicePresetTool(statePort ports.StatePort, presetRegistry *presets.PresetRegistry) *RenderServicePresetTool {
	return &RenderServicePresetTool{
		statePort:      statePort,
		presetRegistry: presetRegistry,
	}
}

// Definition returns the tool definition for LLM
func (t *RenderServicePresetTool) Definition() domain.ToolDefinition {
	return domain.ToolDefinition{
		Name:        "render_service_preset",
		Description: "Render services from state using a preset template. Call this after search_services.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"preset": map[string]interface{}{
					"type":        "string",
					"enum":        []string{"service_card", "service_list"},
					"description": "Preset to use: service_card (for grid), service_list (for compact list)",
				},
			},
			"required": []string{"preset"},
		},
	}
}

// Execute renders services with preset and writes formation to state
func (t *RenderServicePresetTool) Execute(ctx context.Context, sessionID string, input map[string]interface{}) (*domain.ToolResult, error) {
	presetName, _ := input["preset"].(string)

	preset, ok := t.presetRegistry.Get(domain.PresetName(presetName))
	if !ok {
		return &domain.ToolResult{Content: "error: unknown preset", IsError: true}, nil
	}

	state, err := t.statePort.GetState(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("get state: %w", err)
	}

	services := state.Current.Data.Services
	if len(services) == 0 {
		return &domain.ToolResult{Content: "error: no services in state"}, nil
	}

	// Build formation from preset
	formation := buildServiceFormationFromPreset(preset, services)

	// Merge with existing formation if products already rendered
	if existing, ok := state.Current.Template["formation"].(*domain.FormationWithData); ok && existing != nil {
		formation.Widgets = append(existing.Widgets, formation.Widgets...)
	}

	state.Current.Template = map[string]interface{}{
		"formation": formation,
	}

	if err := t.statePort.UpdateState(ctx, state); err != nil {
		return nil, fmt.Errorf("update state: %w", err)
	}

	return &domain.ToolResult{
		Content: fmt.Sprintf("ok: rendered %d services with %s", len(services), presetName),
	}, nil
}

// buildFormationFromPreset creates formation from preset and products
func buildFormationFromPreset(preset domain.Preset, products []domain.Product) *domain.FormationWithData {
	widgets := make([]domain.Widget, 0, len(products))

	// Sort fields by priority
	fields := make([]domain.FieldConfig, len(preset.Fields))
	copy(fields, preset.Fields)
	sort.Slice(fields, func(i, j int) bool {
		return fields[i].Priority < fields[j].Priority
	})

	for i, product := range products {
		atoms := buildAtomsFromProduct(fields, product)
		widget := domain.Widget{
			ID:       uuid.New().String(),
			Template: preset.Template,
			Size:     preset.DefaultSize,
			Priority: i,
			Atoms:    atoms,
		}
		widgets = append(widgets, widget)
	}

	return &domain.FormationWithData{
		Mode:    preset.DefaultMode,
		Widgets: widgets,
	}
}

func buildAtomsFromProduct(fields []domain.FieldConfig, product domain.Product) []domain.Atom {
	atoms := make([]domain.Atom, 0)

	for _, field := range fields {
		value := getProductFieldValue(product, field.Name)
		if value == nil && !field.Required {
			continue
		}
		if value == nil {
			continue // Skip required fields that are nil
		}

		atom := domain.Atom{
			Type:  field.AtomType,
			Value: value,
			Slot:  field.Slot,
		}

		// Add meta based on atom type
		switch field.AtomType {
		case domain.AtomTypeImage:
			atom.Meta = map[string]interface{}{"size": "large"}
		case domain.AtomTypeText:
			if field.Slot == domain.AtomSlotTitle {
				atom.Meta = map[string]interface{}{"style": "heading"}
			} else if field.Slot == domain.AtomSlotPrimary {
				atom.Meta = map[string]interface{}{"display": "chip"}
			} else if field.Slot == domain.AtomSlotSecondary {
				atom.Meta = map[string]interface{}{"label": field.Name}
			}
		case domain.AtomTypePrice:
			currency := product.Currency
			if currency == "" {
				currency = "$"
			}
			atom.Meta = map[string]interface{}{"currency": currency}
		case domain.AtomTypeRating:
			atom.Meta = map[string]interface{}{"display": "chip"}
		}

		atoms = append(atoms, atom)
	}

	return atoms
}

func getProductFieldValue(p domain.Product, fieldName string) interface{} {
	switch fieldName {
	case "id":
		return p.ID
	case "name":
		if p.Name == "" {
			return nil
		}
		return p.Name
	case "description":
		if p.Description == "" {
			return nil
		}
		return p.Description
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
		if p.Brand == "" {
			return nil
		}
		return p.Brand
	case "category":
		if p.Category == "" {
			return nil
		}
		return p.Category
	default:
		return nil
	}
}

// buildServiceFormationFromPreset creates formation from preset and services
func buildServiceFormationFromPreset(preset domain.Preset, services []domain.Service) *domain.FormationWithData {
	widgets := make([]domain.Widget, 0, len(services))

	fields := make([]domain.FieldConfig, len(preset.Fields))
	copy(fields, preset.Fields)
	sort.Slice(fields, func(i, j int) bool {
		return fields[i].Priority < fields[j].Priority
	})

	for i, service := range services {
		atoms := buildAtomsFromService(fields, service)
		widget := domain.Widget{
			ID:       uuid.New().String(),
			Template: preset.Template,
			Size:     preset.DefaultSize,
			Priority: i,
			Atoms:    atoms,
		}
		widgets = append(widgets, widget)
	}

	return &domain.FormationWithData{
		Mode:    preset.DefaultMode,
		Widgets: widgets,
	}
}

func buildAtomsFromService(fields []domain.FieldConfig, service domain.Service) []domain.Atom {
	atoms := make([]domain.Atom, 0)

	for _, field := range fields {
		value := getServiceFieldValue(service, field.Name)
		if value == nil && !field.Required {
			continue
		}
		if value == nil {
			continue // Skip required fields that are nil
		}

		atom := domain.Atom{
			Type:  field.AtomType,
			Value: value,
			Slot:  field.Slot,
		}

		// Add meta based on atom type
		switch field.AtomType {
		case domain.AtomTypeImage:
			atom.Meta = map[string]interface{}{"size": "large"}
		case domain.AtomTypeText:
			if field.Slot == domain.AtomSlotTitle {
				atom.Meta = map[string]interface{}{"style": "heading"}
			} else if field.Slot == domain.AtomSlotPrimary {
				atom.Meta = map[string]interface{}{"display": "chip"}
			} else if field.Slot == domain.AtomSlotSecondary {
				atom.Meta = map[string]interface{}{"label": field.Name}
			}
		case domain.AtomTypePrice:
			currency := service.Currency
			if currency == "" {
				currency = "$"
			}
			atom.Meta = map[string]interface{}{"currency": currency}
		case domain.AtomTypeRating:
			atom.Meta = map[string]interface{}{"display": "chip"}
		}

		atoms = append(atoms, atom)
	}

	return atoms
}

func getServiceFieldValue(s domain.Service, fieldName string) interface{} {
	switch fieldName {
	case "id":
		return s.ID
	case "name":
		if s.Name == "" {
			return nil
		}
		return s.Name
	case "description":
		if s.Description == "" {
			return nil
		}
		return s.Description
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
		if s.Duration == "" {
			return nil
		}
		return s.Duration
	case "provider":
		if s.Provider == "" {
			return nil
		}
		return s.Provider
	default:
		return nil
	}
}
