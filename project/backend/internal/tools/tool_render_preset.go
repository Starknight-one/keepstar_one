package tools

import (
	"context"
	"fmt"
	"sort"

	"keepstar/internal/domain"
	"keepstar/internal/engine"
	"keepstar/internal/ports"
	"keepstar/internal/presets"
)

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
		format, _ := fm["format"].(string)

		if name == "" || slot == "" {
			continue
		}

		entry, known := engine.FieldTypeMap[name]
		if !known {
			entry = engine.FieldTypeEntry{Type: domain.AtomTypeText, Subtype: domain.SubtypeString}
		}

		inferredFormat := engine.InferFormat(domain.AtomFormat(format), entry.Type, entry.Subtype)

		configs = append(configs, domain.FieldConfig{
			Name:     name,
			Slot:     domain.AtomSlot(slot),
			AtomType: entry.Type,
			Subtype:  entry.Subtype,
			Format:   inferredFormat,
			Display:  domain.AtomDisplay(display),
			Priority: i,
		})
		specs = append(specs, domain.FieldSpec{
			Name:    name,
			Slot:    slot,
			Format:  string(inferredFormat),
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
		specs := make([]domain.FieldSpec, 0, len(preset.Fields))
		for _, f := range preset.Fields {
			specs = append(specs, domain.FieldSpec{
				Name:    f.Name,
				Slot:    string(f.Slot),
				Format:  string(f.Format),
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
					"enum":        []string{"product_grid", "product_card", "product_compact", "product_detail", "product_comparison"},
					"description": "Preset to use: product_grid (for multiple items), product_card (for single detail), product_compact (for list), product_detail (for full detail view), product_comparison (for side-by-side comparison, max 4 items)",
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

	if presetName == "product_comparison" && len(products) > 4 {
		products = products[:4]
	}

	var fieldSpecs []domain.FieldSpec
	if rawFields, hasFields := input["fields"]; hasFields {
		customFields, specs := parseFieldSpecs(rawFields)
		if len(customFields) > 0 {
			preset.Fields = customFields
			fieldSpecs = specs
		}
	}

	if sizeStr, ok := input["size"].(string); ok && sizeStr != "" {
		preset.DefaultSize = domain.WidgetSize(sizeStr)
	}

	// Sort fields by priority for consistent ordering
	fields := make([]domain.FieldConfig, len(preset.Fields))
	copy(fields, preset.Fields)
	sort.Slice(fields, func(i, j int) bool {
		return fields[i].Priority < fields[j].Priority
	})
	preset.Fields = fields

	formation := engine.BuildFormation(preset, len(products), func(i int) (engine.FieldGetter, engine.CurrencyGetter, engine.IDGetter) {
		p := products[i]
		return engine.ProductFieldGetter(p), func() string { return p.Currency }, func() string { return p.ID }
	})

	formation.Config = buildRenderConfig("product", preset, preset.DefaultSize, fieldSpecs)

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

	var fieldSpecs []domain.FieldSpec
	if rawFields, hasFields := input["fields"]; hasFields {
		customFields, specs := parseFieldSpecs(rawFields)
		if len(customFields) > 0 {
			preset.Fields = customFields
			fieldSpecs = specs
		}
	}

	if sizeStr, ok := input["size"].(string); ok && sizeStr != "" {
		preset.DefaultSize = domain.WidgetSize(sizeStr)
	}

	// Sort fields by priority for consistent ordering
	fields := make([]domain.FieldConfig, len(preset.Fields))
	copy(fields, preset.Fields)
	sort.Slice(fields, func(i, j int) bool {
		return fields[i].Priority < fields[j].Priority
	})
	preset.Fields = fields

	formation := engine.BuildFormation(preset, len(services), func(i int) (engine.FieldGetter, engine.CurrencyGetter, engine.IDGetter) {
		s := services[i]
		return engine.ServiceFieldGetter(s), func() string { return s.Currency }, func() string { return s.ID }
	})

	formation.Config = buildRenderConfig("service", preset, preset.DefaultSize, fieldSpecs)

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
