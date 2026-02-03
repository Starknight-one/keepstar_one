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

// FieldGetter extracts field value from an entity
type FieldGetter func(fieldName string) interface{}

// CurrencyGetter extracts currency from an entity
type CurrencyGetter func() string

// IDGetter extracts entity ID
type IDGetter func() string

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
					"enum":        []string{"product_grid", "product_card", "product_compact", "product_detail"},
					"description": "Preset to use: product_grid (for multiple items), product_card (for single detail), product_compact (for list), product_detail (for full detail view)",
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

	// Build formation using generic builder
	formation := BuildFormation(preset, len(products), func(i int) (FieldGetter, CurrencyGetter, IDGetter) {
		p := products[i]
		return productFieldGetter(p), func() string { return p.Currency }, func() string { return p.ID }
	})

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
					"enum":        []string{"service_card", "service_list", "service_detail"},
					"description": "Preset to use: service_card (for grid), service_list (for compact list), service_detail (for full detail view)",
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

	// Build formation using generic builder
	formation := BuildFormation(preset, len(services), func(i int) (FieldGetter, CurrencyGetter, IDGetter) {
		s := services[i]
		return serviceFieldGetter(s), func() string { return s.Currency }, func() string { return s.ID }
	})

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

// =============================================================================
// Generic Formation Builder
// =============================================================================

// EntityGetterFunc returns field getter, currency getter, and ID getter for entity at index i
type EntityGetterFunc func(i int) (FieldGetter, CurrencyGetter, IDGetter)

// BuildFormation creates formation from preset and entities (exported for use by navigation usecases)
func BuildFormation(preset domain.Preset, count int, getEntity EntityGetterFunc) *domain.FormationWithData {
	widgets := make([]domain.Widget, 0, count)

	// Sort fields by priority
	fields := make([]domain.FieldConfig, len(preset.Fields))
	copy(fields, preset.Fields)
	sort.Slice(fields, func(i, j int) bool {
		return fields[i].Priority < fields[j].Priority
	})

	for i := 0; i < count; i++ {
		fieldGetter, currencyGetter, idGetter := getEntity(i)
		atoms := buildAtoms(fields, fieldGetter, currencyGetter)
		widget := domain.Widget{
			ID:       uuid.New().String(),
			Template: preset.Template,
			Size:     preset.DefaultSize,
			Priority: i,
			Atoms:    atoms,
			EntityRef: &domain.EntityRef{
				Type: preset.EntityType,
				ID:   idGetter(),
			},
		}
		widgets = append(widgets, widget)
	}

	return &domain.FormationWithData{
		Mode:    preset.DefaultMode,
		Widgets: widgets,
	}
}

// buildAtoms creates atoms from fields using generic field getter
func buildAtoms(fields []domain.FieldConfig, getField FieldGetter, getCurrency CurrencyGetter) []domain.Atom {
	atoms := make([]domain.Atom, 0)

	for _, field := range fields {
		value := getField(field.Name)
		if value == nil {
			continue
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
			switch field.Slot {
			case domain.AtomSlotTitle:
				atom.Meta = map[string]interface{}{"style": "heading"}
			case domain.AtomSlotPrimary:
				atom.Meta = map[string]interface{}{"display": "chip"}
			case domain.AtomSlotSecondary:
				atom.Meta = map[string]interface{}{"label": field.Name}
			}
		case domain.AtomTypePrice:
			currency := getCurrency()
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

// =============================================================================
// Field Getters for Entity Types
// =============================================================================

// productFieldGetter returns a FieldGetter for Product
func productFieldGetter(p domain.Product) FieldGetter {
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
func serviceFieldGetter(s domain.Service) FieldGetter {
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
