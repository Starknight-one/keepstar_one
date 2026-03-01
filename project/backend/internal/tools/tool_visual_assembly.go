package tools

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"

	"keepstar/internal/domain"
	"keepstar/internal/engine"
	"keepstar/internal/ports"
	"keepstar/internal/presets"
)

// layoutKeywords matches user requests that explicitly ask for layout change
var layoutKeywords = regexp.MustCompile(`(?i)(grid|грид|список|list|сравни|сравнение|compar|карусел|carousel|горизонтально|вертикально|horizontal|vertical|таблиц|table)`)

// VisualAssemblyTool renders entities using defaults engine + optional overrides
type VisualAssemblyTool struct {
	statePort      ports.StatePort
	presetRegistry *presets.PresetRegistry
}

// NewVisualAssemblyTool creates the visual assembly tool
func NewVisualAssemblyTool(statePort ports.StatePort, presetRegistry *presets.PresetRegistry) *VisualAssemblyTool {
	return &VisualAssemblyTool{
		statePort:      statePort,
		presetRegistry: presetRegistry,
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
					"description": "Optional shortcut: load a preset as base. If omitted, defaults engine decides.",
					"enum": []string{
						"product_card_grid", "product_card_detail", "product_row",
						"product_single_hero", "product_comparison",
						"search_empty", "category_overview", "attribute_picker",
						"cart_summary", "info_card",
						"product_grid", "product_card", "product_compact", "product_detail",
						"service_card", "service_list", "service_detail",
					},
				},
				"layout": map[string]interface{}{
					"type":        "string",
					"enum":        []string{"grid", "list", "single", "carousel", "comparison", "table"},
					"description": "Layout mode. Default: auto from entity count (1→single, 2+→grid).",
				},
				"show": map[string]interface{}{
					"type":        "array",
					"items":       map[string]interface{}{"type": "string"},
					"description": "Field names to display: images, name, price, rating, brand, category, description, tags, stockQuantity, attributes, duration, provider, availability.",
				},
				"hide": map[string]interface{}{
					"type":        "array",
					"items":       map[string]interface{}{"type": "string"},
					"description": "Field names to remove from defaults.",
				},
				"display": map[string]interface{}{
					"type":                 "object",
					"description":          "Field→display wrapper overrides. E.g. {\"brand\":\"badge\",\"price\":\"h2\"}. Display is the visual container (badge, tag, h1, body, etc.).",
					"additionalProperties": map[string]interface{}{"type": "string"},
				},
				"format": map[string]interface{}{
					"type":                 "object",
					"description":          "Field→format overrides. Auto-inferred from type+subtype — rarely needed. E.g. {\"rating\":\"stars-text\"}. Values: currency, stars, stars-text, stars-compact, percent, number, date, text.",
					"additionalProperties": map[string]interface{}{"type": "string"},
				},
				"order": map[string]interface{}{
					"type":        "array",
					"items":       map[string]interface{}{"type": "string"},
					"description": "Field render order. Fields not listed go after in default order.",
				},
				"size": map[string]interface{}{
					"description": "Widget size. String for uniform: \"large\". Object for per-field: {\"images\":\"xl\",\"price\":\"lg\"}.",
				},
				"color": map[string]interface{}{
					"type":                 "object",
					"description":          "Field→color map. Named colors: green, red, blue, orange, purple, gray. Or hex. E.g. {\"brand\":\"red\",\"price\":\"green\"}.",
					"additionalProperties": map[string]interface{}{"type": "string"},
				},
				"direction": map[string]interface{}{
					"type":        "string",
					"enum":        []string{"vertical", "horizontal"},
					"description": "Card direction: vertical (default) or horizontal (image left, content right).",
				},
				"shape": map[string]interface{}{
					"type":                 "object",
					"description":          "Field→shape map. E.g. {\"brand\":\"pill\",\"category\":\"rounded\"}. Values: pill, rounded, square, circle.",
					"additionalProperties": map[string]interface{}{"type": "string"},
				},
				"layer": map[string]interface{}{
					"type":                 "object",
					"description":          "Field→z-index layer map. E.g. {\"stockQuantity\":\"2\"}.",
					"additionalProperties": map[string]interface{}{"type": "string"},
				},
				"anchor": map[string]interface{}{
					"type":                 "object",
					"description":          "Field→anchor position map. E.g. {\"brand\":\"top-right\"}. Values: top-left, top-right, bottom-left, bottom-right, center.",
					"additionalProperties": map[string]interface{}{"type": "string"},
				},
				"place": map[string]interface{}{
					"type":        "string",
					"enum":        []string{"sticky", "floating", "default"},
					"description": "Widget placement mode: sticky (top), floating (bottom-right), default.",
				},
				"compose": map[string]interface{}{
					"type": "array",
					"items": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"mode":  map[string]interface{}{"type": "string"},
							"show":  map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
							"hide":  map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
							"count": map[string]interface{}{"type": "number"},
							"label": map[string]interface{}{"type": "string"},
						},
					},
					"description": "Multi-section formation. Each section has its own mode/show/hide/count.",
				},
				"conditional": map[string]interface{}{
					"type": "array",
					"items": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"field":   map[string]interface{}{"type": "string"},
							"op":      map[string]interface{}{"type": "string", "enum": []string{"eq", "gt", "lt", "gte", "lte"}},
							"value":   map[string]interface{}{},
							"display": map[string]interface{}{"type": "string"},
							"color":   map[string]interface{}{"type": "string"},
						},
					},
					"description": "Conditional styling rules. E.g. [{\"field\":\"stockQuantity\",\"op\":\"eq\",\"value\":0,\"display\":\"badge-error\",\"color\":\"red\"}].",
				},
				"limit": map[string]interface{}{
					"type":        "number",
					"description": "Max widgets to return (default 50). For pagination.",
				},
				"offset": map[string]interface{}{
					"type":        "number",
					"description": "Offset for pagination (default 0).",
				},
			},
		},
	}
}

// validateInput sanitizes tool input values, stripping invalid entries
func validateInput(input map[string]interface{}) {
	// size: only valid values
	if sizeStr, ok := input["size"].(string); ok {
		switch sizeStr {
		case "tiny", "small", "medium", "large", "xl":
			// valid
		default:
			input["size"] = "medium"
		}
	}

	// color: named (6) or hex #xxx/#xxxxxx, strip invalid
	if colorRaw, ok := input["color"].(map[string]interface{}); ok {
		validColors := map[string]bool{"green": true, "red": true, "blue": true, "orange": true, "purple": true, "gray": true}
		for field, c := range colorRaw {
			cs, ok := c.(string)
			if !ok {
				delete(colorRaw, field)
				continue
			}
			if !validColors[cs] && !isValidHex(cs) {
				delete(colorRaw, field)
			}
		}
	}

	// shape: only valid values
	validShapes := map[string]bool{"pill": true, "rounded": true, "square": true, "circle": true}
	if shapeRaw, ok := input["shape"].(map[string]interface{}); ok {
		for field, s := range shapeRaw {
			sv, ok := s.(string)
			if !ok || !validShapes[sv] {
				delete(shapeRaw, field)
			}
		}
	}

	// anchor: only valid values
	validAnchors := map[string]bool{"top-left": true, "top-right": true, "bottom-left": true, "bottom-right": true, "center": true}
	if anchorRaw, ok := input["anchor"].(map[string]interface{}); ok {
		for field, a := range anchorRaw {
			av, ok := a.(string)
			if !ok || !validAnchors[av] {
				delete(anchorRaw, field)
			}
		}
	}

	// layer: must parse to int
	if layerRaw, ok := input["layer"].(map[string]interface{}); ok {
		for field, l := range layerRaw {
			lv, ok := l.(string)
			if !ok {
				delete(layerRaw, field)
				continue
			}
			if _, err := strconv.Atoi(lv); err != nil {
				delete(layerRaw, field)
			}
		}
	}
}

// isValidHex checks if string is a valid hex color (#xxx or #xxxxxx)
func isValidHex(s string) bool {
	if len(s) == 0 || s[0] != '#' {
		return false
	}
	hex := s[1:]
	if len(hex) != 3 && len(hex) != 6 {
		return false
	}
	for _, c := range hex {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// Execute renders entities with visual assembly and writes formation to state
func (t *VisualAssemblyTool) Execute(ctx context.Context, toolCtx ToolContext, input map[string]interface{}) (*domain.ToolResult, error) {
	degraded := false

	// Step 0: Validate and sanitize input
	validateInput(input)

	state, err := t.statePort.GetState(ctx, toolCtx.SessionID)
	if err != nil {
		return nil, fmt.Errorf("get state: %w", err)
	}

	// Step 1: Auto-detect entity type and count
	entityType := "product"
	products := state.Current.Data.Products
	services := state.Current.Data.Services

	if len(products) == 0 && len(services) > 0 {
		entityType = "service"
	}

	entityCount := len(products) + len(services)
	if entityCount == 0 {
		return &domain.ToolResult{Content: "error: no entities in state"}, nil
	}

	// Step 2: Get base defaults (or patch from currentConfig if no data change)
	resolved := engine.AutoResolve(entityType, entityCount)
	fields := resolved.Fields
	displayOverrides := make(map[string]string)
	layout := resolved.Layout
	size := resolved.Size

	// Step 2.5: Formation diff/patch — if currentConfig exists, use it as base
	presetName, _ := input["preset"].(string)
	if presetName == "" && state.Current.Template != nil {
		if fData, ok := state.Current.Template["formation"]; ok {
			if f, ok := fData.(*domain.FormationWithData); ok && f != nil && f.Config != nil {
				prevConfig := f.Config
				if len(prevConfig.Fields) > 0 {
					fields = make([]string, 0, len(prevConfig.Fields))
					for _, fs := range prevConfig.Fields {
						fields = append(fields, fs.Name)
						if fs.Display != "" {
							displayOverrides[fs.Name] = fs.Display
						}
					}
				}
				layout = string(prevConfig.Mode)
				size = prevConfig.Size
			}
		}
	}

	// Step 3: If preset specified, load it as base (backward compat)
	if presetName != "" && t.presetRegistry != nil {
		if preset, ok := t.presetRegistry.Get(domain.PresetName(presetName)); ok {
			fields = make([]string, 0, len(preset.Fields))
			for _, f := range preset.Fields {
				fields = append(fields, f.Name)
				displayOverrides[f.Name] = string(f.Display)
			}
			layout = string(preset.DefaultMode)
			size = preset.DefaultSize
		}
	}

	// Step 4: Apply show/hide
	hasExplicitShow := false
	if showRaw, ok := input["show"].([]interface{}); ok && len(showRaw) > 0 {
		hasExplicitShow = true
		showFields := make([]string, 0, len(showRaw))
		for _, s := range showRaw {
			if name, ok := s.(string); ok {
				showFields = append(showFields, name)
			}
		}
		seen := make(map[string]bool, len(showFields)+len(fields))
		merged := make([]string, 0, len(showFields)+len(fields))
		for _, f := range fields {
			if !seen[f] {
				merged = append(merged, f)
				seen[f] = true
			}
		}
		for _, f := range showFields {
			if !seen[f] {
				merged = append(merged, f)
				seen[f] = true
			}
		}
		fields = merged
	}

	hasExplicitHide := false
	if hideRaw, ok := input["hide"].([]interface{}); ok && len(hideRaw) > 0 {
		hasExplicitHide = true
		hideSet := make(map[string]bool, len(hideRaw))
		for _, h := range hideRaw {
			if name, ok := h.(string); ok {
				hideSet[name] = true
			}
		}
		filtered := make([]string, 0, len(fields))
		for _, f := range fields {
			if !hideSet[f] {
				filtered = append(filtered, f)
			}
		}
		fields = filtered
	}

	// Step 5: Apply display overrides
	if displayRaw, ok := input["display"].(map[string]interface{}); ok {
		for field, disp := range displayRaw {
			if d, ok := disp.(string); ok {
				displayOverrides[field] = d
			}
		}
	}

	// Step 6: Apply order
	if orderRaw, ok := input["order"].([]interface{}); ok && len(orderRaw) > 0 {
		ordered := make([]string, 0, len(fields))
		fieldSet := make(map[string]bool, len(fields))
		for _, f := range fields {
			fieldSet[f] = true
		}
		for _, o := range orderRaw {
			if name, ok := o.(string); ok && fieldSet[name] {
				ordered = append(ordered, name)
				delete(fieldSet, name)
			}
		}
		for _, f := range fields {
			if fieldSet[f] {
				ordered = append(ordered, f)
			}
		}
		fields = ordered
	}

	// Step 7: Apply layout/size overrides with graceful degradation
	layoutExplicit := false
	if layoutStr, ok := input["layout"].(string); ok && layoutStr != "" {
		switch layoutStr {
		case "grid", "list", "single", "carousel", "comparison", "table":
			layout = layoutStr
			layoutExplicit = true
		default:
			degraded = true
		}
	}
	perAtomSize := make(map[string]string)
	if sizeStr, ok := input["size"].(string); ok && sizeStr != "" {
		size = domain.WidgetSize(sizeStr)
	} else if sizeObj, ok := input["size"].(map[string]interface{}); ok {
		for field, s := range sizeObj {
			if sv, ok := s.(string); ok {
				perAtomSize[field] = sv
			}
		}
	}

	// Step 7.1: Layout post-validate guard
	if layoutExplicit && toolCtx.UserQuery != "" {
		var currentConfig *domain.RenderConfig
		if state.Current.Template != nil {
			if fData, ok := state.Current.Template["formation"]; ok {
				if f, ok := fData.(*domain.FormationWithData); ok && f != nil && f.Config != nil {
					currentConfig = f.Config
				}
			}
		}
		if currentConfig != nil && !layoutKeywords.MatchString(toolCtx.UserQuery) {
			layout = string(currentConfig.Mode)
		}
	}

	// Step 7.5: Parse color, direction, shape, layer, anchor
	colorMap := make(map[string]string)
	if colorRaw, ok := input["color"].(map[string]interface{}); ok {
		for field, c := range colorRaw {
			if cs, ok := c.(string); ok {
				colorMap[field] = cs
			}
		}
	}
	direction, _ := input["direction"].(string)

	shapeMap := make(map[string]string)
	if shapeRaw, ok := input["shape"].(map[string]interface{}); ok {
		for field, s := range shapeRaw {
			if sv, ok := s.(string); ok {
				shapeMap[field] = sv
			}
		}
	}

	layerMap := make(map[string]string)
	if layerRaw, ok := input["layer"].(map[string]interface{}); ok {
		for field, l := range layerRaw {
			if lv, ok := l.(string); ok {
				layerMap[field] = lv
			}
		}
	}

	anchorMap := make(map[string]string)
	if anchorRaw, ok := input["anchor"].(map[string]interface{}); ok {
		for field, a := range anchorRaw {
			if av, ok := a.(string); ok {
				anchorMap[field] = av
			}
		}
	}

	place, _ := input["place"].(string)

	paginationLimit := 50
	paginationOffset := 0
	if v, ok := input["limit"].(float64); ok && v > 0 {
		paginationLimit = int(v)
	}
	if v, ok := input["offset"].(float64); ok {
		paginationOffset = int(v)
		if paginationOffset < 0 {
			paginationOffset = 0
		}
	}

	if layout == "comparison" && len(products) > 4 {
		products = products[:4]
	}

	// Step 7.6: Parse format overrides
	formatOverrides := make(map[string]string)
	if formatRaw, ok := input["format"].(map[string]interface{}); ok {
		for field, f := range formatRaw {
			if fs, ok := f.(string); ok {
				formatOverrides[field] = fs
			}
		}
	}

	// Step 8: Build FieldConfigs (with format inference)
	fieldConfigs := engine.BuildFieldConfigsWithFormat(fields, displayOverrides, formatOverrides)

	sort.Slice(fieldConfigs, func(i, j int) bool {
		return fieldConfigs[i].Priority < fieldConfigs[j].Priority
	})

	// Step 8.3: Apply slot constraints
	fieldConfigs = engine.ApplySlotConstraints(fieldConfigs)

	// Step 8.5: Apply max atoms constraint per size
	if !hasExplicitShow && !hasExplicitHide {
		if max, ok := engine.MaxAtomsPerSize[string(size)]; ok && len(fieldConfigs) > max {
			fieldConfigs = fieldConfigs[:max]
		}
	}

	// Step 9: Determine template and formation mode
	template := "GenericCard"
	formationMode := engine.ParseFormationType(layout)

	// Step 9.5: Check for compose (multi-section)
	if composeRaw, ok := input["compose"].([]interface{}); ok && len(composeRaw) > 0 {
		formation := engine.BuildComposedFormation(t.presetRegistry, composeRaw, products, services, displayOverrides, formatOverrides, template, size, entityType)
		formation = engine.ApplyPostProcessing(formation, colorMap, perAtomSize, shapeMap, layerMap, anchorMap, direction, place, paginationLimit, paginationOffset)
		return t.writeFormation(ctx, toolCtx, formation, entityType, presetName, formationMode, size, fieldConfigs, fields, layout, products, services, degraded)
	}

	// Step 10: Build formation (standard path)
	var formation *domain.FormationWithData

	if len(products) > 0 && len(services) > 0 {
		pWidgets := engine.BuildVisualWidgets(fieldConfigs, template, size, len(products), func(i int) (engine.FieldGetter, engine.CurrencyGetter, engine.IDGetter) {
			p := products[i]
			return engine.ProductFieldGetter(p), func() string { return p.Currency }, func() string { return p.ID }
		}, domain.EntityTypeProduct)

		sFieldConfigs := engine.BuildFieldConfigsWithFormat(engine.ResolveServiceFields(fields), displayOverrides, formatOverrides)
		sWidgets := engine.BuildVisualWidgets(sFieldConfigs, template, size, len(services), func(i int) (engine.FieldGetter, engine.CurrencyGetter, engine.IDGetter) {
			s := services[i]
			return engine.ServiceFieldGetter(s), func() string { return s.Currency }, func() string { return s.ID }
		}, domain.EntityTypeService)

		formation = &domain.FormationWithData{
			Mode:    formationMode,
			Widgets: append(pWidgets, sWidgets...),
		}
	} else if len(products) > 0 {
		widgets := engine.BuildVisualWidgets(fieldConfigs, template, size, len(products), func(i int) (engine.FieldGetter, engine.CurrencyGetter, engine.IDGetter) {
			p := products[i]
			return engine.ProductFieldGetter(p), func() string { return p.Currency }, func() string { return p.ID }
		}, domain.EntityTypeProduct)
		formation = &domain.FormationWithData{
			Mode:    formationMode,
			Widgets: widgets,
		}
	} else {
		widgets := engine.BuildVisualWidgets(fieldConfigs, template, size, len(services), func(i int) (engine.FieldGetter, engine.CurrencyGetter, engine.IDGetter) {
			s := services[i]
			return engine.ServiceFieldGetter(s), func() string { return s.Currency }, func() string { return s.ID }
		}, domain.EntityTypeService)
		formation = &domain.FormationWithData{
			Mode:    formationMode,
			Widgets: widgets,
		}
	}

	// Auto grid config for grid mode
	if formationMode == domain.FormationTypeGrid && formation.Grid == nil {
		formation.Grid = engine.CalcGridConfig(len(formation.Widgets), size)
	}

	// Apply constraints pipeline
	for i := range formation.Widgets {
		formation.Widgets[i].Atoms = engine.ApplyAtomConstraints(formation.Widgets[i].Atoms)
		engine.ApplyWidgetConstraints(&formation.Widgets[i])
	}
	engine.ApplyCrossWidgetConstraints(formation.Widgets, formationMode)

	// Parse and apply conditional styling
	if condRaw, ok := input["conditional"].([]interface{}); ok && len(condRaw) > 0 {
		rules := engine.ParseConditionalRules(condRaw)
		engine.ApplyConditionalStyling(formation.Widgets, rules)
	}

	// Calculate layout zones for each widget
	tokens := engine.DefaultDesignTokens()
	for i := range formation.Widgets {
		formation.Widgets[i].Zones = engine.CalculateZones(formation.Widgets[i].Atoms, tokens)
	}

	// Apply post-processing (meta, pagination)
	formation = engine.ApplyPostProcessing(formation, colorMap, perAtomSize, shapeMap, layerMap, anchorMap, direction, place, paginationLimit, paginationOffset)

	return t.writeFormation(ctx, toolCtx, formation, entityType, presetName, formationMode, size, fieldConfigs, fields, layout, products, services, degraded)
}

// writeFormation saves formation to state and returns result
func (t *VisualAssemblyTool) writeFormation(ctx context.Context, toolCtx ToolContext, formation *domain.FormationWithData, entityType, presetName string, formationMode domain.FormationType, size domain.WidgetSize, fieldConfigs []domain.FieldConfig, fields []string, layout string, products []domain.Product, services []domain.Service, degraded bool) (*domain.ToolResult, error) {
	fieldSpecs := make([]domain.FieldSpec, 0, len(fieldConfigs))
	for _, fc := range fieldConfigs {
		fieldSpecs = append(fieldSpecs, domain.FieldSpec{
			Name:    fc.Name,
			Slot:    string(fc.Slot),
			Format:  string(fc.Format),
			Display: string(fc.Display),
		})
	}
	formation.Config = &domain.RenderConfig{
		EntityType: entityType,
		Preset:     presetName,
		Mode:       formationMode,
		Size:       size,
		Fields:     fieldSpecs,
	}

	templateMap := map[string]interface{}{
		"formation": formation,
	}

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

	totalEntities := len(products) + len(services)
	msg := fmt.Sprintf("ok: rendered %d entities with visual_assembly layout=%s size=%s fields=%v", totalEntities, layout, size, fields)
	if degraded {
		msg += " (degraded: unsupported options ignored)"
	}
	return &domain.ToolResult{Content: msg}, nil
}
