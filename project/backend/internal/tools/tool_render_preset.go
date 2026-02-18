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

// fieldTypeEntry maps a field name to its AtomType and Subtype for dynamic field construction
type fieldTypeEntry struct {
	Type    domain.AtomType
	Subtype domain.AtomSubtype
}

// fieldTypeMap resolves field name → AtomType/Subtype for fields[] construction
var fieldTypeMap = map[string]fieldTypeEntry{
	"name":          {domain.AtomTypeText, domain.SubtypeString},
	"description":   {domain.AtomTypeText, domain.SubtypeString},
	"brand":         {domain.AtomTypeText, domain.SubtypeString},
	"category":      {domain.AtomTypeText, domain.SubtypeString},
	"price":         {domain.AtomTypeNumber, domain.SubtypeCurrency},
	"rating":        {domain.AtomTypeNumber, domain.SubtypeRating},
	"images":        {domain.AtomTypeImage, domain.SubtypeImageURL},
	"stockQuantity": {domain.AtomTypeNumber, domain.SubtypeInt},
	"tags":          {domain.AtomTypeText, domain.SubtypeString},
	"attributes":    {domain.AtomTypeText, domain.SubtypeString},
	"duration":      {domain.AtomTypeText, domain.SubtypeString},
	"provider":      {domain.AtomTypeText, domain.SubtypeString},
	"availability":  {domain.AtomTypeText, domain.SubtypeString},
}

// parseFieldSpecs parses fields[] from tool input into []domain.FieldConfig
func parseFieldSpecs(rawFields interface{}) ([]domain.FieldConfig, []domain.FieldSpec) {
	fieldsArr, ok := rawFields.([]interface{})
	if !ok || len(fieldsArr) == 0 {
		return nil, nil
	}

	configs := make([]domain.FieldConfig, 0, len(fieldsArr))
	specs := make([]domain.FieldSpec, 0, len(fieldsArr))

	for i, f := range fieldsArr {
		fm, ok := f.(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := fm["name"].(string)
		slot, _ := fm["slot"].(string)
		display, _ := fm["display"].(string)

		if name == "" || slot == "" {
			continue
		}

		entry, known := fieldTypeMap[name]
		if !known {
			// Default to text/string for unknown fields
			entry = fieldTypeEntry{domain.AtomTypeText, domain.SubtypeString}
		}

		configs = append(configs, domain.FieldConfig{
			Name:     name,
			Slot:     domain.AtomSlot(slot),
			AtomType: entry.Type,
			Subtype:  entry.Subtype,
			Display:  domain.AtomDisplay(display),
			Priority: i,
		})
		specs = append(specs, domain.FieldSpec{
			Name:    name,
			Slot:    slot,
			Display: display,
		})
	}

	return configs, specs
}

// buildRenderConfig creates a RenderConfig from preset and optional field overrides
func buildRenderConfig(entityType string, preset domain.Preset, size domain.WidgetSize, fieldSpecs []domain.FieldSpec) *domain.RenderConfig {
	cfg := &domain.RenderConfig{
		EntityType: entityType,
		Preset:     preset.Name,
		Mode:       preset.DefaultMode,
		Size:       size,
	}

	if len(fieldSpecs) > 0 {
		cfg.Fields = fieldSpecs
	} else {
		// Build from preset defaults
		specs := make([]domain.FieldSpec, 0, len(preset.Fields))
		for _, f := range preset.Fields {
			specs = append(specs, domain.FieldSpec{
				Name:    f.Name,
				Slot:    string(f.Slot),
				Display: string(f.Display),
			})
		}
		cfg.Fields = specs
	}

	return cfg
}

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
				"fields": map[string]interface{}{
					"type":        "array",
					"description": "Optional: custom field configuration. Replaces default preset fields. Each item: {name, slot, display}.",
					"items": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"name":    map[string]interface{}{"type": "string", "description": "Field name: images, name, price, rating, brand, category, description, tags, stockQuantity, attributes"},
							"slot":    map[string]interface{}{"type": "string", "description": "Target slot: hero, badge, title, primary, price, secondary"},
							"display": map[string]interface{}{"type": "string", "description": "Display style: h1, h2, h3, body, price, price-lg, rating, image-cover, badge, tag, etc."},
						},
						"required": []string{"name", "slot", "display"},
					},
				},
				"size": map[string]interface{}{
					"type":        "string",
					"enum":        []string{"tiny", "small", "medium", "large"},
					"description": "Optional: override widget size from preset default",
				},
			},
			"required": []string{"preset"},
		},
	}
}

// Execute renders products with preset and writes formation to state
func (t *RenderProductPresetTool) Execute(ctx context.Context, toolCtx ToolContext, input map[string]interface{}) (*domain.ToolResult, error) {
	presetName, _ := input["preset"].(string)

	preset, ok := t.presetRegistry.Get(domain.PresetName(presetName))
	if !ok {
		return &domain.ToolResult{Content: "error: unknown preset", IsError: true}, nil
	}

	state, err := t.statePort.GetState(ctx, toolCtx.SessionID)
	if err != nil {
		return nil, fmt.Errorf("get state: %w", err)
	}

	products := state.Current.Data.Products
	if len(products) == 0 {
		return &domain.ToolResult{Content: "error: no products in state"}, nil
	}

	// Override fields if provided (user has display preferences)
	var fieldSpecs []domain.FieldSpec
	if rawFields, hasFields := input["fields"]; hasFields {
		customFields, specs := parseFieldSpecs(rawFields)
		if len(customFields) > 0 {
			preset.Fields = customFields
			fieldSpecs = specs
		}
	}

	// Override size if provided
	if sizeStr, ok := input["size"].(string); ok && sizeStr != "" {
		preset.DefaultSize = domain.WidgetSize(sizeStr)
	}

	// Build formation using generic builder
	formation := BuildFormation(preset, len(products), func(i int) (FieldGetter, CurrencyGetter, IDGetter) {
		p := products[i]
		return productFieldGetter(p), func() string { return p.Currency }, func() string { return p.ID }
	})

	// Write RenderConfig for next-turn context
	formation.Config = buildRenderConfig("product", preset, preset.DefaultSize, fieldSpecs)

	// Store formation in state template via zone-write
	template := map[string]interface{}{
		"formation": formation,
	}

	info := domain.DeltaInfo{
		TurnID:    toolCtx.TurnID,
		Trigger:   domain.TriggerUserQuery,
		Source:    domain.SourceLLM,
		ActorID:   toolCtx.ActorID,
		DeltaType: domain.DeltaTypeUpdate,
		Path:      "template",
		Action:    domain.Action{Type: domain.ActionLayout, Tool: "render_product_preset"},
	}
	if _, err := t.statePort.UpdateTemplate(ctx, toolCtx.SessionID, template, info); err != nil {
		return nil, fmt.Errorf("update template: %w", err)
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
				"fields": map[string]interface{}{
					"type":        "array",
					"description": "Optional: custom field configuration. Replaces default preset fields. Each item: {name, slot, display}.",
					"items": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"name":    map[string]interface{}{"type": "string", "description": "Field name: images, name, price, rating, duration, provider, availability, description, attributes"},
							"slot":    map[string]interface{}{"type": "string", "description": "Target slot: hero, badge, title, primary, price, secondary"},
							"display": map[string]interface{}{"type": "string", "description": "Display style: h1, h2, h3, body, price, price-lg, rating, image-cover, badge, tag, etc."},
						},
						"required": []string{"name", "slot", "display"},
					},
				},
				"size": map[string]interface{}{
					"type":        "string",
					"enum":        []string{"tiny", "small", "medium", "large"},
					"description": "Optional: override widget size from preset default",
				},
			},
			"required": []string{"preset"},
		},
	}
}

// Execute renders services with preset and writes formation to state
func (t *RenderServicePresetTool) Execute(ctx context.Context, toolCtx ToolContext, input map[string]interface{}) (*domain.ToolResult, error) {
	presetName, _ := input["preset"].(string)

	preset, ok := t.presetRegistry.Get(domain.PresetName(presetName))
	if !ok {
		return &domain.ToolResult{Content: "error: unknown preset", IsError: true}, nil
	}

	state, err := t.statePort.GetState(ctx, toolCtx.SessionID)
	if err != nil {
		return nil, fmt.Errorf("get state: %w", err)
	}

	services := state.Current.Data.Services
	if len(services) == 0 {
		return &domain.ToolResult{Content: "error: no services in state"}, nil
	}

	// Override fields if provided (user has display preferences)
	var fieldSpecs []domain.FieldSpec
	if rawFields, hasFields := input["fields"]; hasFields {
		customFields, specs := parseFieldSpecs(rawFields)
		if len(customFields) > 0 {
			preset.Fields = customFields
			fieldSpecs = specs
		}
	}

	// Override size if provided
	if sizeStr, ok := input["size"].(string); ok && sizeStr != "" {
		preset.DefaultSize = domain.WidgetSize(sizeStr)
	}

	// Build formation using generic builder
	formation := BuildFormation(preset, len(services), func(i int) (FieldGetter, CurrencyGetter, IDGetter) {
		s := services[i]
		return serviceFieldGetter(s), func() string { return s.Currency }, func() string { return s.ID }
	})

	// Write RenderConfig for next-turn context
	formation.Config = buildRenderConfig("service", preset, preset.DefaultSize, fieldSpecs)

	// Merge with existing formation if products already rendered
	if existing, ok := state.Current.Template["formation"].(*domain.FormationWithData); ok && existing != nil {
		formation.Widgets = append(existing.Widgets, formation.Widgets...)
	}

	template := map[string]interface{}{
		"formation": formation,
	}

	info := domain.DeltaInfo{
		TurnID:    toolCtx.TurnID,
		Trigger:   domain.TriggerUserQuery,
		Source:    domain.SourceLLM,
		ActorID:   toolCtx.ActorID,
		DeltaType: domain.DeltaTypeUpdate,
		Path:      "template",
		Action:    domain.Action{Type: domain.ActionLayout, Tool: "render_service_preset"},
	}
	if _, err := t.statePort.UpdateTemplate(ctx, toolCtx.SessionID, template, info); err != nil {
		return nil, fmt.Errorf("update template: %w", err)
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

// BuildTemplateFormation creates a template formation with all fields from the preset.
// Each atom has FieldName set and Value=nil — the frontend fills values from entity data at render time.
// This is used for adjacent templates: 1 template per entity type instead of N formations per entity.
func BuildTemplateFormation(preset domain.Preset) *domain.FormationWithData {
	fields := make([]domain.FieldConfig, len(preset.Fields))
	copy(fields, preset.Fields)
	sort.Slice(fields, func(i, j int) bool {
		return fields[i].Priority < fields[j].Priority
	})

	atoms := make([]domain.Atom, 0, len(fields))
	for _, field := range fields {
		atom := domain.Atom{
			Type:      field.AtomType,
			Subtype:   field.Subtype,
			Display:   string(field.Display),
			Value:     nil,
			FieldName: field.Name,
			Slot:      field.Slot,
		}
		// Add meta hints for special types
		switch field.AtomType {
		case domain.AtomTypeImage:
			atom.Meta = map[string]interface{}{"size": "large"}
		case domain.AtomTypeNumber:
			if field.Subtype == domain.SubtypeCurrency {
				// Sentinel — frontend replaces with entity.currency
				atom.Meta = map[string]interface{}{"currency": "__ENTITY_CURRENCY__"}
			}
		}
		atoms = append(atoms, atom)
	}

	widget := domain.Widget{
		ID:       "template",
		Template: preset.Template,
		Size:     preset.DefaultSize,
		Atoms:    atoms,
	}

	return &domain.FormationWithData{
		Mode:    preset.DefaultMode,
		Widgets: []domain.Widget{widget},
	}
}

// buildAtoms creates atoms from fields using generic field getter
// Now uses the new atom model with Type, Subtype, Display
func buildAtoms(fields []domain.FieldConfig, getField FieldGetter, getCurrency CurrencyGetter) []domain.Atom {
	atoms := make([]domain.Atom, 0)

	for _, field := range fields {
		value := getField(field.Name)
		if value == nil {
			continue
		}

		atom := domain.Atom{
			Type:    field.AtomType,
			Subtype: field.Subtype,
			Display: string(field.Display), // Use Display from FieldConfig
			Value:   value,
			Slot:    field.Slot,
		}

		// Add meta for backward compatibility and additional data
		switch field.AtomType {
		case domain.AtomTypeImage:
			atom.Meta = map[string]interface{}{"size": "large"}
		case domain.AtomTypeNumber:
			if field.Subtype == domain.SubtypeCurrency {
				currency := getCurrency()
				if currency == "" {
					currency = "$"
				}
				atom.Meta = map[string]interface{}{"currency": currency}
			}
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
