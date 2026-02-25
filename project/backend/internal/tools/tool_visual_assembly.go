package tools

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"keepstar/internal/domain"
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
					"description":          "Field→display style overrides. E.g. {\"brand\":\"badge\",\"price\":\"price-lg\"}.",
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
	resolved := AutoResolve(entityType, entityCount)
	fields := resolved.Fields
	displayOverrides := make(map[string]string)
	layout := resolved.Layout
	size := resolved.Size

	// Step 2.5: Formation diff/patch — if currentConfig exists, use it as base
	// This preserves previous settings (color, size overrides) when user tweaks style
	presetName, _ := input["preset"].(string)
	if presetName == "" && state.Current.Template != nil {
		if fData, ok := state.Current.Template["formation"]; ok {
			if f, ok := fData.(*domain.FormationWithData); ok && f != nil && f.Config != nil {
				prevConfig := f.Config
				// Use previous fields as base
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
			// Extract fields from preset
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
		// show = add semantics: keep base fields order, append new show-fields at end
		showFields := make([]string, 0, len(showRaw))
		for _, s := range showRaw {
			if name, ok := s.(string); ok {
				showFields = append(showFields, name)
			}
		}
		seen := make(map[string]bool, len(showFields)+len(fields))
		merged := make([]string, 0, len(showFields)+len(fields))
		// Base fields first (preserve order)
		for _, f := range fields {
			if !seen[f] {
				merged = append(merged, f)
				seen[f] = true
			}
		}
		// Show-fields appended at end (new additions)
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
		// First: ordered fields that exist in our field list
		for _, o := range orderRaw {
			if name, ok := o.(string); ok && fieldSet[name] {
				ordered = append(ordered, name)
				delete(fieldSet, name)
			}
		}
		// Then: remaining fields in original order
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
		// Validate layout — graceful degradation for unknown values
		switch layoutStr {
		case "grid", "list", "single", "carousel", "comparison", "table":
			layout = layoutStr
			layoutExplicit = true
		default:
			degraded = true
			// keep current layout
		}
	}
	// Per-atom size map (if size is object like {"images":"xl","price":"lg"})
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

	// Step 7.1: Layout post-validate guard — prevent LLM from changing layout without user intent
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
			// User didn't ask for layout change — preserve current layout
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

	// Parse shape map
	shapeMap := make(map[string]string)
	if shapeRaw, ok := input["shape"].(map[string]interface{}); ok {
		for field, s := range shapeRaw {
			if sv, ok := s.(string); ok {
				shapeMap[field] = sv
			}
		}
	}

	// Parse layer map (z-index)
	layerMap := make(map[string]string)
	if layerRaw, ok := input["layer"].(map[string]interface{}); ok {
		for field, l := range layerRaw {
			if lv, ok := l.(string); ok {
				layerMap[field] = lv
			}
		}
	}

	// Parse anchor map (position)
	anchorMap := make(map[string]string)
	if anchorRaw, ok := input["anchor"].(map[string]interface{}); ok {
		for field, a := range anchorRaw {
			if av, ok := a.(string); ok {
				anchorMap[field] = av
			}
		}
	}

	// Parse place (widget placement)
	place, _ := input["place"].(string)

	// Parse pagination
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

	// Limit comparison to max 4
	if layout == "comparison" && len(products) > 4 {
		products = products[:4]
	}

	// Step 8: Build FieldConfigs
	fieldConfigs := BuildFieldConfigs(fields, displayOverrides)

	// Sort by priority
	sort.Slice(fieldConfigs, func(i, j int) bool {
		return fieldConfigs[i].Priority < fieldConfigs[j].Priority
	})

	// Step 8.3: Apply slot constraints
	fieldConfigs = ApplySlotConstraints(fieldConfigs)

	// Step 8.5: Apply max atoms constraint per size
	// Skip when show/hide are explicit — Agent2 deliberately chose these fields
	if !hasExplicitShow && !hasExplicitHide {
		if max, ok := MaxAtomsPerSize[string(size)]; ok && len(fieldConfigs) > max {
			fieldConfigs = fieldConfigs[:max]
		}
	}

	// Step 9: Determine template and formation mode
	template := "GenericCard"
	formationMode := parseFormationType(layout)

	// Step 9.5: Check for compose (multi-section)
	if composeRaw, ok := input["compose"].([]interface{}); ok && len(composeRaw) > 0 {
		formation := t.buildComposedFormation(composeRaw, products, services, displayOverrides, template, size, entityType)
		formation = t.applyPostProcessing(formation, colorMap, perAtomSize, shapeMap, layerMap, anchorMap, direction, place, paginationLimit, paginationOffset)
		return t.writeFormation(ctx, toolCtx, formation, entityType, presetName, formationMode, size, fieldConfigs, fields, layout, products, services, degraded)
	}

	// Step 10: Build formation (standard path)
	var formation *domain.FormationWithData

	if len(products) > 0 && len(services) > 0 {
		// Both: build products first, then append services
		pWidgets := buildVisualWidgets(fieldConfigs, template, size, len(products), func(i int) (FieldGetter, CurrencyGetter, IDGetter) {
			p := products[i]
			return productFieldGetter(p), func() string { return p.Currency }, func() string { return p.ID }
		}, domain.EntityTypeProduct)

		sFieldConfigs := BuildFieldConfigs(resolveServiceFields(fields), displayOverrides)
		sWidgets := buildVisualWidgets(sFieldConfigs, template, size, len(services), func(i int) (FieldGetter, CurrencyGetter, IDGetter) {
			s := services[i]
			return serviceFieldGetter(s), func() string { return s.Currency }, func() string { return s.ID }
		}, domain.EntityTypeService)

		formation = &domain.FormationWithData{
			Mode:    formationMode,
			Widgets: append(pWidgets, sWidgets...),
		}
	} else if len(products) > 0 {
		widgets := buildVisualWidgets(fieldConfigs, template, size, len(products), func(i int) (FieldGetter, CurrencyGetter, IDGetter) {
			p := products[i]
			return productFieldGetter(p), func() string { return p.Currency }, func() string { return p.ID }
		}, domain.EntityTypeProduct)
		formation = &domain.FormationWithData{
			Mode:    formationMode,
			Widgets: widgets,
		}
	} else {
		widgets := buildVisualWidgets(fieldConfigs, template, size, len(services), func(i int) (FieldGetter, CurrencyGetter, IDGetter) {
			s := services[i]
			return serviceFieldGetter(s), func() string { return s.Currency }, func() string { return s.ID }
		}, domain.EntityTypeService)
		formation = &domain.FormationWithData{
			Mode:    formationMode,
			Widgets: widgets,
		}
	}

	// Auto grid config for grid mode
	if formationMode == domain.FormationTypeGrid && formation.Grid == nil {
		formation.Grid = CalcGridConfig(len(formation.Widgets), size)
	}

	// Apply constraints pipeline
	for i := range formation.Widgets {
		formation.Widgets[i].Atoms = ApplyAtomConstraints(formation.Widgets[i].Atoms)
		ApplyWidgetConstraints(&formation.Widgets[i])
	}
	ApplyCrossWidgetConstraints(formation.Widgets, formationMode)

	// Parse and apply conditional styling
	if condRaw, ok := input["conditional"].([]interface{}); ok && len(condRaw) > 0 {
		rules := parseConditionalRules(condRaw)
		applyConditionalStyling(formation.Widgets, rules)
	}

	// Apply post-processing (meta, pagination)
	formation = t.applyPostProcessing(formation, colorMap, perAtomSize, shapeMap, layerMap, anchorMap, direction, place, paginationLimit, paginationOffset)

	return t.writeFormation(ctx, toolCtx, formation, entityType, presetName, formationMode, size, fieldConfigs, fields, layout, products, services, degraded)
}

// applyPostProcessing applies color, size, shape, layer, anchor, direction, place, and pagination
func (t *VisualAssemblyTool) applyPostProcessing(formation *domain.FormationWithData, colorMap, perAtomSize, shapeMap, layerMap, anchorMap map[string]string, direction, place string, paginationLimit, paginationOffset int) *domain.FormationWithData {
	// Apply color, per-atom size, shape, layer, anchor, and direction to widgets
	for wi := range formation.Widgets {
		if len(colorMap) > 0 {
			applyAtomColors(formation.Widgets[wi].Atoms, colorMap)
		}
		if len(perAtomSize) > 0 {
			applyAtomMeta(formation.Widgets[wi].Atoms, perAtomSize, "size")
		}
		if len(shapeMap) > 0 {
			applyAtomMeta(formation.Widgets[wi].Atoms, shapeMap, "shape")
		}
		if len(layerMap) > 0 {
			applyAtomMeta(formation.Widgets[wi].Atoms, layerMap, "layer")
		}
		if len(anchorMap) > 0 {
			applyAtomMeta(formation.Widgets[wi].Atoms, anchorMap, "anchor")
		}
		if direction != "" {
			if formation.Widgets[wi].Meta == nil {
				formation.Widgets[wi].Meta = make(map[string]interface{})
			}
			formation.Widgets[wi].Meta["direction"] = direction
		}
		if place != "" && place != "default" {
			if formation.Widgets[wi].Meta == nil {
				formation.Widgets[wi].Meta = make(map[string]interface{})
			}
			formation.Widgets[wi].Meta["place"] = place
		}
	}

	// Apply pagination
	totalWidgets := len(formation.Widgets)
	if paginationOffset > 0 || paginationLimit < totalWidgets {
		start := paginationOffset
		if start < 0 {
			start = 0
		}
		if start > totalWidgets {
			start = totalWidgets
		}
		end := start + paginationLimit
		if end > totalWidgets {
			end = totalWidgets
		}
		formation.Widgets = formation.Widgets[start:end]
		formation.Pagination = &domain.PaginationMeta{
			Total:   totalWidgets,
			Offset:  paginationOffset,
			Limit:   paginationLimit,
			HasMore: end < totalWidgets,
		}
	}

	return formation
}

// writeFormation saves formation to state and returns result
func (t *VisualAssemblyTool) writeFormation(ctx context.Context, toolCtx ToolContext, formation *domain.FormationWithData, entityType, presetName string, formationMode domain.FormationType, size domain.WidgetSize, fieldConfigs []domain.FieldConfig, fields []string, layout string, products []domain.Product, services []domain.Service, degraded bool) (*domain.ToolResult, error) {
	// Build RenderConfig for next-turn context
	fieldSpecs := make([]domain.FieldSpec, 0, len(fieldConfigs))
	for _, fc := range fieldConfigs {
		fieldSpecs = append(fieldSpecs, domain.FieldSpec{
			Name:    fc.Name,
			Slot:    string(fc.Slot),
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

	// Write to state
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

// buildComposedFormation builds a multi-section formation from compose[] input
func (t *VisualAssemblyTool) buildComposedFormation(composeRaw []interface{}, products []domain.Product, services []domain.Service, displayOverrides map[string]string, template string, size domain.WidgetSize, entityType string) *domain.FormationWithData {
	sections := make([]domain.FormationSection, 0, len(composeRaw))

	// Track offsets across sections so each section gets unique entities
	productOffset := 0
	serviceOffset := 0

	for _, raw := range composeRaw {
		sectionInput, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}

		// Parse section config
		sectionMode := "grid"
		if m, ok := sectionInput["mode"].(string); ok && m != "" {
			sectionMode = m
		}
		sectionLabel, _ := sectionInput["label"].(string)

		// Resolve fields for section
		sectionFields := fieldRanking[entityType]
		if sectionFields == nil {
			sectionFields = fieldRanking["product"]
		}

		if showRaw, ok := sectionInput["show"].([]interface{}); ok && len(showRaw) > 0 {
			showFields := make([]string, 0, len(showRaw))
			for _, s := range showRaw {
				if name, ok := s.(string); ok {
					showFields = append(showFields, name)
				}
			}
			seen := make(map[string]bool)
			merged := make([]string, 0)
			for _, f := range showFields {
				if !seen[f] {
					merged = append(merged, f)
					seen[f] = true
				}
			}
			for _, f := range sectionFields {
				if !seen[f] {
					merged = append(merged, f)
					seen[f] = true
				}
			}
			sectionFields = merged
		}
		if hideRaw, ok := sectionInput["hide"].([]interface{}); ok && len(hideRaw) > 0 {
			hideSet := make(map[string]bool)
			for _, h := range hideRaw {
				if name, ok := h.(string); ok {
					hideSet[name] = true
				}
			}
			filtered := make([]string, 0)
			for _, f := range sectionFields {
				if !hideSet[f] {
					filtered = append(filtered, f)
				}
			}
			sectionFields = filtered
		}

		sectionFieldConfigs := BuildFieldConfigs(sectionFields, displayOverrides)
		sort.Slice(sectionFieldConfigs, func(i, j int) bool {
			return sectionFieldConfigs[i].Priority < sectionFieldConfigs[j].Priority
		})

		// Determine count for this section (from remaining entities)
		remainingProducts := len(products) - productOffset
		if remainingProducts < 0 {
			remainingProducts = 0
		}
		remainingServices := len(services) - serviceOffset
		if remainingServices < 0 {
			remainingServices = 0
		}
		sectionCount := remainingProducts + remainingServices
		if c, ok := sectionInput["count"].(float64); ok && int(c) > 0 && int(c) < sectionCount {
			sectionCount = int(c)
		}

		// Build widgets for section using offset-adjusted slices
		var sectionWidgets []domain.Widget
		pCount := remainingProducts
		if pCount > sectionCount {
			pCount = sectionCount
		}
		remaining := sectionCount - pCount
		sRemaining := remainingServices
		if remaining > sRemaining {
			remaining = sRemaining
		}

		if pCount > 0 {
			pOff := productOffset
			sectionWidgets = buildVisualWidgets(sectionFieldConfigs, template, size, pCount, func(i int) (FieldGetter, CurrencyGetter, IDGetter) {
				p := products[pOff+i]
				return productFieldGetter(p), func() string { return p.Currency }, func() string { return p.ID }
			}, domain.EntityTypeProduct)
			productOffset += pCount
		}
		if remaining > 0 {
			sOff := serviceOffset
			sw := buildVisualWidgets(sectionFieldConfigs, template, size, remaining, func(i int) (FieldGetter, CurrencyGetter, IDGetter) {
				s := services[sOff+i]
				return serviceFieldGetter(s), func() string { return s.Currency }, func() string { return s.ID }
			}, domain.EntityTypeService)
			sectionWidgets = append(sectionWidgets, sw...)
			serviceOffset += remaining
		}

		sMode := parseFormationType(sectionMode)
		section := domain.FormationSection{
			Mode:    sMode,
			Widgets: sectionWidgets,
			Label:   sectionLabel,
		}
		if sMode == domain.FormationTypeGrid {
			section.Grid = CalcGridConfig(len(sectionWidgets), size)
		}
		sections = append(sections, section)
	}

	// Merge all widgets into top-level for backward compat
	var allWidgets []domain.Widget
	for _, s := range sections {
		allWidgets = append(allWidgets, s.Widgets...)
	}

	return &domain.FormationWithData{
		Mode:     domain.FormationTypeGrid, // default top-level mode
		Widgets:  allWidgets,
		Sections: sections,
	}
}

// parseFormationType converts string to FormationType
func parseFormationType(mode string) domain.FormationType {
	switch mode {
	case "grid":
		return domain.FormationTypeGrid
	case "list":
		return domain.FormationTypeList
	case "carousel":
		return domain.FormationTypeCarousel
	case "single":
		return domain.FormationTypeSingle
	case "comparison":
		return domain.FormationTypeComparison
	case "table":
		return domain.FormationTypeTable
	default:
		return domain.FormationTypeGrid
	}
}

// buildVisualWidgets creates widgets using field configs and the GenericCard template
func buildVisualWidgets(fieldConfigs []domain.FieldConfig, template string, size domain.WidgetSize, count int, getEntity EntityGetterFunc, entityType domain.EntityType) []domain.Widget {
	widgets := make([]domain.Widget, 0, count)

	for i := 0; i < count; i++ {
		fieldGetter, currencyGetter, idGetter := getEntity(i)
		atoms := buildAtoms(fieldConfigs, fieldGetter, currencyGetter)
		widget := domain.Widget{
			ID:       generateWidgetID(),
			Template: template,
			Size:     size,
			Priority: i,
			Atoms:    atoms,
			EntityRef: &domain.EntityRef{
				Type: entityType,
				ID:   idGetter(),
			},
		}
		widgets = append(widgets, widget)
	}

	return widgets
}

// resolveServiceFields maps product field names to service equivalents where applicable
func resolveServiceFields(fields []string) []string {
	serviceRanking := fieldRanking["service"]
	serviceSet := make(map[string]bool, len(serviceRanking))
	for _, f := range serviceRanking {
		serviceSet[f] = true
	}

	result := make([]string, 0, len(fields))
	for _, f := range fields {
		if serviceSet[f] {
			result = append(result, f)
		}
	}
	return result
}

// generateWidgetID creates a unique widget ID
func generateWidgetID() string {
	return uuid.New().String()
}

// applyAtomColors sets color in atom.Meta for fields specified in the color map
func applyAtomColors(atoms []domain.Atom, colorMap map[string]string) {
	for i := range atoms {
		if color, ok := colorMap[atoms[i].FieldName]; ok && color != "" {
			if atoms[i].Meta == nil {
				atoms[i].Meta = make(map[string]interface{})
			}
			atoms[i].Meta["color"] = color
		}
	}
}

// applyAtomMeta sets a meta key for atoms matching fields in the map
func applyAtomMeta(atoms []domain.Atom, fieldMap map[string]string, metaKey string) {
	for i := range atoms {
		if val, ok := fieldMap[atoms[i].FieldName]; ok && val != "" {
			if atoms[i].Meta == nil {
				atoms[i].Meta = make(map[string]interface{})
			}
			atoms[i].Meta[metaKey] = val
		}
	}
}

// --- Conditional Styling ---

type conditionalRule struct {
	Field   string
	Op      string
	Value   interface{}
	Display string
	Color   string
}

func parseConditionalRules(raw []interface{}) []conditionalRule {
	rules := make([]conditionalRule, 0, len(raw))
	for _, r := range raw {
		m, ok := r.(map[string]interface{})
		if !ok {
			continue
		}
		rule := conditionalRule{
			Field: fmt.Sprintf("%v", m["field"]),
			Op:    fmt.Sprintf("%v", m["op"]),
			Value: m["value"],
		}
		if d, ok := m["display"].(string); ok {
			rule.Display = d
		}
		if c, ok := m["color"].(string); ok {
			rule.Color = c
		}
		rules = append(rules, rule)
	}
	return rules
}

func applyConditionalStyling(widgets []domain.Widget, rules []conditionalRule) {
	for wi := range widgets {
		for ai := range widgets[wi].Atoms {
			atom := &widgets[wi].Atoms[ai]
			for _, rule := range rules {
				if !strings.EqualFold(atom.FieldName, rule.Field) {
					continue
				}
				if !evalCondition(atom.Value, rule.Op, rule.Value) {
					continue
				}
				if atom.Meta == nil {
					atom.Meta = make(map[string]interface{})
				}
				if rule.Display != "" {
					atom.Display = rule.Display
					atom.Meta["conditional_display"] = rule.Display
				}
				if rule.Color != "" {
					atom.Meta["color"] = rule.Color
				}
			}
		}
	}
}

func evalCondition(atomValue interface{}, op string, ruleValue interface{}) bool {
	av, aOk := toFloat(atomValue)
	rv, rOk := toFloat(ruleValue)
	if !aOk || !rOk {
		// String equality fallback
		if op == "eq" {
			return fmt.Sprintf("%v", atomValue) == fmt.Sprintf("%v", ruleValue)
		}
		return false
	}
	switch op {
	case "eq":
		return av == rv
	case "gt":
		return av > rv
	case "lt":
		return av < rv
	case "gte":
		return av >= rv
	case "lte":
		return av <= rv
	default:
		return false
	}
}

func toFloat(v interface{}) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case float32:
		return float64(val), true
	case int:
		return float64(val), true
	case int64:
		return float64(val), true
	case string:
		f, err := strconv.ParseFloat(val, 64)
		return f, err == nil
	default:
		return 0, false
	}
}
