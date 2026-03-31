package tools

import (
	"context"
	"fmt"

	"keepstar/internal/domain"
	"keepstar/internal/engine"
	"keepstar/internal/ports"
	"keepstar/internal/presets"
)

// VisualAssemblyTool renders entities using defaults engine + optional overrides
type VisualAssemblyTool struct {
	statePort        ports.StatePort
	presetV2Registry *presets.PresetV2Registry
	fieldDefPort     ports.FieldDefinitionPort
}

// NewVisualAssemblyTool creates the visual assembly tool with field definitions support.
func NewVisualAssemblyTool(statePort ports.StatePort, presetV2Registry *presets.PresetV2Registry, fieldDefPort ports.FieldDefinitionPort) *VisualAssemblyTool {
	return &VisualAssemblyTool{
		statePort:        statePort,
		presetV2Registry: presetV2Registry,
		fieldDefPort:     fieldDefPort,
	}
}

// Definition returns the tool definition for LLM
func (t *VisualAssemblyTool) Definition() domain.ToolDefinition {
	return domain.ToolDefinition{
		Name:        "visual_assembly",
		Description: "Render entities from state with smart defaults. All parameters optional — defaults engine auto-resolves layout, size, and fields. Use parameters only to override defaults.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"preset": map[string]interface{}{
					"type":        "string",
					"description": "Preset name to use as base configuration.",
					"enum":        []string{"product_card_grid", "product_card_detail", "product_row", "service_card", "service_detail"},
				},
				"layout": map[string]interface{}{
					"type":        "string",
					"enum":        []string{"grid", "list", "single", "carousel", "comparison", "table"},
					"description": "Layout mode. Default: auto from entity count (1→single, 2+→grid).",
				},
				"size": map[string]interface{}{
					"type":        "string",
					"enum":        []string{"tiny", "small", "medium", "large"},
					"description": "Widget size. Default: auto from entity count.",
				},
				"direction": map[string]interface{}{
					"type":        "string",
					"enum":        []string{"vertical", "horizontal"},
					"description": "Card direction: vertical (default) or horizontal (image left, content right).",
				},
				"show": map[string]interface{}{
					"type":        "array",
					"items":       map[string]interface{}{"type": "string"},
					"description": "Field names to ADD to defaults.",
				},
				"hide": map[string]interface{}{
					"type":        "array",
					"items":       map[string]interface{}{"type": "string"},
					"description": "Field names to REMOVE from defaults.",
				},
				"order": map[string]interface{}{
					"type":        "array",
					"items":       map[string]interface{}{"type": "string"},
					"description": "Field render order.",
				},
				"limit": map[string]interface{}{
					"type":        "number",
					"description": "Max widgets to return (default 50).",
				},
				"offset": map[string]interface{}{
					"type":        "number",
					"description": "Offset for pagination (default 0).",
				},
				"columns": map[string]interface{}{
					"type":        "number",
					"description": "Grid columns override (1-4). Overrides auto grid calculation.",
				},
				"gap": map[string]interface{}{
					"type":        "string",
					"enum":        []string{"xs", "sm", "md", "lg", "xl"},
					"description": "Gap between widgets/atoms. Default: sm.",
				},
				"widgetPadding": map[string]interface{}{
					"type":        "string",
					"enum":        []string{"xs", "sm", "md", "lg", "xl"},
					"description": "Widget internal padding.",
				},
				"widgetBackground": map[string]interface{}{
					"type":        "string",
					"description": "Widget background color (hex or semantic: 'surface', 'white', '#F3F4F6').",
				},
				"widgetBorderRadius": map[string]interface{}{
					"type":        "string",
					"enum":        []string{"none", "sm", "md", "lg", "xl", "full"},
					"description": "Widget border radius.",
				},
				"widgetShadow": map[string]interface{}{
					"type":        "string",
					"enum":        []string{"none", "sm", "md", "lg"},
					"description": "Widget shadow.",
				},
				"widgetBorder": map[string]interface{}{
					"type":        "string",
					"description": "Widget border (e.g. '1px solid #E5E7EB').",
				},
				"atoms": map[string]interface{}{
					"type":        "object",
					"description": "Per-field overrides keyed by field name. Each value has: textStyle, wrapper, mediaStyle, iconStyle, format, color, rigidity.",
					"additionalProperties": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"textStyle": map[string]interface{}{
								"type": "object",
								"properties": map[string]interface{}{
									"fontSize":       map[string]interface{}{"type": "string", "description": "Token: xs, sm, md, lg, xl, 2xl, 3xl"},
									"fontWeight":     map[string]interface{}{"type": "string", "description": "Token: light, normal, medium, semibold, bold"},
									"color":          map[string]interface{}{"type": "string", "description": "Color token or hex"},
									"textDecoration": map[string]interface{}{"type": "string"},
									"textTransform":  map[string]interface{}{"type": "string"},
									"lineClamp":      map[string]interface{}{"type": "number"},
									"lineHeight":     map[string]interface{}{"type": "string", "description": "Token: tight, normal, relaxed, loose"},
									"letterSpacing":  map[string]interface{}{"type": "string", "description": "Token: tight, normal, wide"},
									"truncate":       map[string]interface{}{"type": "number", "description": "Max chars (0 = unlimited)"},
								},
							},
							"wrapper": map[string]interface{}{
								"type": "object",
								"properties": map[string]interface{}{
									"type":         map[string]interface{}{"type": "string", "description": "Wrapper: none, badge, tag, pill, avatar, tooltip, alert, link, progress, button"},
									"variant":      map[string]interface{}{"type": "string", "description": "Variant: success, error, warning, primary, secondary, outline, active"},
									"background":   map[string]interface{}{"type": "string", "description": "Wrapper background color"},
									"borderRadius": map[string]interface{}{"type": "string", "description": "Wrapper border radius token"},
									"padding":      map[string]interface{}{"type": "string", "description": "Wrapper padding token"},
								},
							},
							"mediaStyle": map[string]interface{}{
								"type": "object",
								"properties": map[string]interface{}{
									"aspectRatio": map[string]interface{}{"type": "string", "description": "Aspect ratio: 1:1, 4:3, 16:9, auto"},
									"objectFit":   map[string]interface{}{"type": "string", "description": "Object fit: cover, contain, fill"},
									"controls":    map[string]interface{}{"type": "boolean", "description": "Show video/audio controls"},
									"autoplay":    map[string]interface{}{"type": "boolean", "description": "Autoplay video"},
									"muted":       map[string]interface{}{"type": "boolean", "description": "Mute video"},
									"poster":      map[string]interface{}{"type": "string", "description": "Video poster URL"},
								},
							},
							"iconStyle": map[string]interface{}{
								"type": "object",
								"properties": map[string]interface{}{
									"size":  map[string]interface{}{"type": "string", "description": "Icon size: xs, sm, md, lg, xl"},
									"color": map[string]interface{}{"type": "string", "description": "Icon color (semantic or hex)"},
									"style": map[string]interface{}{"type": "string", "description": "Icon style: stroke, fill"},
								},
							},
							"format":   map[string]interface{}{"type": "string", "description": "Value format: currency, stars, stars-text, stars-compact, percent, number, date, text"},
							"color":    map[string]interface{}{"type": "string", "description": "Color: green, red, blue, orange, purple, gray, or hex"},
							"rigidity": map[string]interface{}{"type": "string", "enum": []string{"locked", "preferred", "flexible"}, "description": "Override strength: locked (user explicit), preferred (preset), flexible (default)"},
						},
					},
				},
			},
		},
	}
}

// Execute renders entities with visual assembly and writes formation to state
func (t *VisualAssemblyTool) Execute(ctx context.Context, toolCtx ToolContext, input map[string]interface{}) (*domain.ToolResult, error) {
	state, err := t.statePort.GetState(ctx, toolCtx.SessionID)
	if err != nil {
		return nil, fmt.Errorf("get state: %w", err)
	}

	products := state.Current.Data.Products
	services := state.Current.Data.Services
	entityCount := len(products) + len(services)
	if entityCount == 0 {
		return &domain.ToolResult{Content: "error: no entities in state"}, nil
	}

	entityType := domain.EntityTypeProduct
	if len(products) == 0 && len(services) > 0 {
		entityType = domain.EntityTypeService
	}

	// Load field definitions from DB
	var fieldDefs []engine.FieldDefinitionEntry
	if t.fieldDefPort != nil && toolCtx.TenantSlug != "" {
		defs, err := t.fieldDefPort.ListFieldDefinitions(ctx, toolCtx.TenantSlug, entityType)
		if err == nil && len(defs) > 0 {
			fieldDefs = make([]engine.FieldDefinitionEntry, len(defs))
			for i, d := range defs {
				fieldDefs[i] = engine.FieldDefinitionEntry{
					FieldName:      d.FieldName,
					AtomType:       d.AtomType,
					AtomSubtype:    d.AtomSubtype,
					DefaultDisplay: d.DefaultDisplay,
					DefaultSlot:    d.DefaultSlot,
					Label:          d.Label,
					Priority:       d.Priority,
				}
			}
		}
	}

	// Parse input directly into AgentInstructions
	instructions := parseV2Input(input)

	// Load PresetV2 (explicit or auto-selected)
	var preset *domain.PresetV2
	presetNameV2, _ := input["preset"].(string)
	if t.presetV2Registry != nil {
		if presetNameV2 != "" {
			if p, ok := t.presetV2Registry.Get(presetNameV2); ok {
				preset = &p
			}
		}
		if preset == nil {
			// Auto-select preset based on entity type + instructions
			resolved := engine.AutoResolve(string(entityType), entityCount)
			if instructions != nil {
				if instructions.Layout != "" {
					resolved.Layout = instructions.Layout
				}
				if instructions.Size != "" {
					resolved.Size = domain.WidgetSize(instructions.Size)
				}
			}
			autoName := engine.AutoSelectPreset(entityType, resolved.Layout, resolved.Size)
			if p, ok := t.presetV2Registry.Get(autoName); ok {
				preset = &p
				presetNameV2 = autoName
			}
		}
	}

	// Extract current visible fields and mode from previous formation (for additive show/hide)
	var currentFields []string
	var currentMode string
	if state.Current.Template != nil {
		if fData, ok := state.Current.Template["formation"]; ok {
			if f, ok := fData.(*domain.FormationWithData); ok && f != nil {
				currentMode = string(f.Mode)
				if len(f.Widgets) > 0 {
					seen := make(map[string]bool)
					for _, a := range f.Widgets[0].AtomsV2 {
						if a.FieldName != "" && !seen[a.FieldName] {
							currentFields = append(currentFields, a.FieldName)
							seen[a.FieldName] = true
						}
					}
				}
			}
		}
	}

	// Run v2 engine
	eng := engine.NewEngineV2()
	output := eng.Execute(engine.EngineV2Input{
		EntityType:    entityType,
		Products:      products,
		Services:      services,
		FieldDefs:     fieldDefs,
		Instructions:  instructions,
		Preset:        preset,
		CurrentFields: currentFields,
		CurrentMode:   currentMode,
	})

	formation := output.Formation

	// Apply post-processing (direction + pagination; colors/wrappers handled by engine)
	direction := ""
	if instructions != nil {
		direction = instructions.Direction
	}
	paginationLimit := 50
	paginationOffset := 0
	if instructions != nil && instructions.Limit > 0 {
		paginationLimit = instructions.Limit
	}
	if instructions != nil && instructions.Offset > 0 {
		paginationOffset = instructions.Offset
	}
	emptyMap := map[string]string{}
	formation = engine.ApplyPostProcessing(formation, emptyMap, emptyMap, emptyMap, emptyMap, emptyMap, direction, "", paginationLimit, paginationOffset)

	// Build render config
	fieldSpecs := make([]domain.FieldSpec, 0)
	for _, w := range formation.Widgets {
		for _, a := range w.AtomsV2 {
			fieldSpecs = append(fieldSpecs, domain.FieldSpec{
				Name:    a.FieldName,
				Slot:    string(a.Slot),
				Format:  string(a.Format),
				Display: inferLegacyDisplayFromAtomV2(a),
			})
		}
		break // Use first widget's atoms as field spec
	}

	formation.Config = &domain.RenderConfig{
		EntityType: string(entityType),
		Preset:     presetNameV2,
		Mode:       formation.Mode,
		Size:       formation.Widgets[0].Size,
		Fields:     fieldSpecs,
	}

	// Write to state
	templateMap := map[string]interface{}{"formation": formation}
	info := domain.DeltaInfo{
		TurnID:    toolCtx.TurnID,
		Trigger:   domain.TriggerUserQuery,
		Source:    domain.SourceLLM,
		ActorID:   toolCtx.ActorID,
		DeltaType: domain.DeltaTypeUpdate,
		Path:      "template",
		Action:    domain.Action{Type: domain.ActionLayout, Tool: "visual_assembly"},
	}
	if _, err := t.statePort.UpdateTemplate(ctx, toolCtx.SessionID, templateMap, info); err != nil {
		return nil, fmt.Errorf("update template: %w", err)
	}

	msg := fmt.Sprintf("ok: rendered %d entities with visual_assembly layout=%s size=%s preset=%s", entityCount, formation.Mode, formation.Widgets[0].Size, presetNameV2)
	if len(output.Warnings) > 0 {
		msg += fmt.Sprintf(" warnings=%v", output.Warnings)
	}

	metadata := map[string]interface{}{
		"preset":      presetNameV2,
		"layout":      string(formation.Mode),
		"size":        string(formation.Widgets[0].Size),
		"entityType":  string(entityType),
		"entityCount": entityCount,
		"widgetCount": len(formation.Widgets),
		"fieldDefCount": len(fieldDefs),
		"warnings":    output.Warnings,
	}

	return &domain.ToolResult{Content: msg, Metadata: metadata}, nil
}

// parseV2Input parses tool input directly into AgentInstructions.
func parseV2Input(input map[string]interface{}) *engine.AgentInstructions {
	instr := &engine.AgentInstructions{}
	hasAny := false

	if preset, ok := input["preset"].(string); ok && preset != "" {
		instr.Preset = preset
		hasAny = true
	}

	if showRaw, ok := input["show"].([]interface{}); ok && len(showRaw) > 0 {
		for _, s := range showRaw {
			if name, ok := s.(string); ok {
				instr.Show = append(instr.Show, name)
			}
		}
		hasAny = true
	}

	if hideRaw, ok := input["hide"].([]interface{}); ok && len(hideRaw) > 0 {
		for _, h := range hideRaw {
			if name, ok := h.(string); ok {
				instr.Hide = append(instr.Hide, name)
			}
		}
		hasAny = true
	}

	if orderRaw, ok := input["order"].([]interface{}); ok && len(orderRaw) > 0 {
		for _, o := range orderRaw {
			if name, ok := o.(string); ok {
				instr.Order = append(instr.Order, name)
			}
		}
		hasAny = true
	}

	if layout, ok := input["layout"].(string); ok && layout != "" {
		instr.Layout = layout
		hasAny = true
	}

	if sizeStr, ok := input["size"].(string); ok && sizeStr != "" {
		instr.Size = sizeStr
		hasAny = true
	}

	if dir, ok := input["direction"].(string); ok && dir != "" {
		instr.Direction = dir
		hasAny = true
	}

	if v, ok := input["limit"].(float64); ok && v > 0 {
		instr.Limit = int(v)
		hasAny = true
	}
	if v, ok := input["offset"].(float64); ok && v > 0 {
		instr.Offset = int(v)
		hasAny = true
	}

	// Widget/formation container overrides
	if v, ok := input["columns"].(float64); ok && v > 0 {
		instr.Columns = int(v)
		hasAny = true
	}
	if v, ok := input["gap"].(string); ok && v != "" {
		instr.Gap = v
		hasAny = true
	}
	if v, ok := input["widgetPadding"].(string); ok && v != "" {
		instr.WidgetPadding = v
		hasAny = true
	}
	if v, ok := input["widgetBackground"].(string); ok && v != "" {
		instr.WidgetBackground = v
		hasAny = true
	}
	if v, ok := input["widgetBorderRadius"].(string); ok && v != "" {
		instr.WidgetBorderRadius = v
		hasAny = true
	}
	if v, ok := input["widgetShadow"].(string); ok && v != "" {
		instr.WidgetShadow = v
		hasAny = true
	}
	if v, ok := input["widgetBorder"].(string); ok && v != "" {
		instr.WidgetBorder = v
		hasAny = true
	}

	// Parse atoms map directly
	if atomsRaw, ok := input["atoms"].(map[string]interface{}); ok && len(atomsRaw) > 0 {
		instr.Atoms = make(map[string]engine.AtomOverride, len(atomsRaw))
		for field, overrideRaw := range atomsRaw {
			instr.Atoms[field] = parseAtomOverride(overrideRaw)
		}
		hasAny = true
	}

	if !hasAny {
		return nil
	}
	return instr
}

// parseAtomOverride parses a single atom override from raw JSON input.
func parseAtomOverride(raw interface{}) engine.AtomOverride {
	obj, ok := raw.(map[string]interface{})
	if !ok {
		return engine.AtomOverride{}
	}

	var ov engine.AtomOverride

	// textStyle: { fontSize, fontWeight, color, textDecoration, textTransform, lineClamp, lineHeight, letterSpacing, truncate }
	if tsRaw, ok := obj["textStyle"].(map[string]interface{}); ok {
		ts := &domain.TextStyle{}
		if v, ok := tsRaw["fontSize"].(string); ok {
			ts.FontSize = v
		}
		if v, ok := tsRaw["fontWeight"].(string); ok {
			ts.FontWeight = v
		}
		if v, ok := tsRaw["color"].(string); ok {
			ts.Color = v
		}
		if v, ok := tsRaw["textDecoration"].(string); ok {
			ts.TextDecoration = v
		}
		if v, ok := tsRaw["textTransform"].(string); ok {
			ts.TextTransform = v
		}
		if v, ok := tsRaw["lineClamp"].(float64); ok {
			ts.LineClamp = int(v)
		}
		if v, ok := tsRaw["lineHeight"].(string); ok {
			ts.LineHeight = v
		}
		if v, ok := tsRaw["letterSpacing"].(string); ok {
			ts.LetterSpacing = v
		}
		if v, ok := tsRaw["truncate"].(float64); ok {
			ts.Truncate = int(v)
		}
		ov.TextStyle = ts
	}

	// wrapper: { type, variant, background, borderRadius, padding }
	if wrRaw, ok := obj["wrapper"].(map[string]interface{}); ok {
		wr := &domain.WrapperConfig{}
		if v, ok := wrRaw["type"].(string); ok {
			wr.Type = v
		}
		if v, ok := wrRaw["variant"].(string); ok {
			wr.Variant = v
		}
		if v, ok := wrRaw["background"].(string); ok {
			wr.Background = v
		}
		if v, ok := wrRaw["borderRadius"].(string); ok {
			wr.BorderRadius = v
		}
		if v, ok := wrRaw["padding"].(string); ok {
			wr.Padding = v
		}
		ov.Wrapper = wr
	}

	// mediaStyle: { aspectRatio, objectFit, controls, autoplay, muted, poster }
	if msRaw, ok := obj["mediaStyle"].(map[string]interface{}); ok {
		ms := &domain.MediaStyle{}
		if v, ok := msRaw["aspectRatio"].(string); ok {
			ms.AspectRatio = v
		}
		if v, ok := msRaw["objectFit"].(string); ok {
			ms.ObjectFit = v
		}
		if v, ok := msRaw["controls"].(bool); ok {
			ms.Controls = v
		}
		if v, ok := msRaw["autoplay"].(bool); ok {
			ms.Autoplay = v
		}
		if v, ok := msRaw["muted"].(bool); ok {
			ms.Muted = v
		}
		if v, ok := msRaw["poster"].(string); ok {
			ms.Poster = v
		}
		ov.MediaStyle = ms
	}

	// iconStyle: { size, color, style }
	if isRaw, ok := obj["iconStyle"].(map[string]interface{}); ok {
		is := &domain.IconStyle{}
		if v, ok := isRaw["size"].(string); ok {
			is.Size = v
		}
		if v, ok := isRaw["color"].(string); ok {
			is.Color = v
		}
		if v, ok := isRaw["style"].(string); ok {
			is.Style = v
		}
		ov.IconStyle = is
	}

	// format
	if v, ok := obj["format"].(string); ok {
		ov.Format = v
	}

	// color
	if v, ok := obj["color"].(string); ok {
		ov.Color = v
	}

	// rigidity
	if v, ok := obj["rigidity"].(string); ok {
		ov.Rigidity = domain.Rigidity(v)
	}

	return ov
}

// inferLegacyDisplayFromAtomV2 converts v2 atom textStyle+wrapper back to legacy display string.
func inferLegacyDisplayFromAtomV2(a domain.AtomV2) string {
	return string(engine.AtomV2ToLegacy(a).Display)
}
