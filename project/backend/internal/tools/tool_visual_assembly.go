package tools

import (
	"context"
	"fmt"
	"regexp"
	"sort"

	"github.com/google/uuid"
	"keepstar/internal/domain"
	"keepstar/internal/ports"
	"keepstar/internal/presets"
)

// layoutKeywords matches user requests that explicitly ask for layout change
var layoutKeywords = regexp.MustCompile(`(?i)(grid|грид|список|list|сравни|сравнение|compar|карусел|carousel|горизонтально|вертикально|horizontal|vertical)`)

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
					"description": "Optional shortcut: load a preset as base (product_grid, product_detail, etc). If omitted, defaults engine decides.",
				},
				"layout": map[string]interface{}{
					"type":        "string",
					"enum":        []string{"grid", "list", "single", "carousel", "comparison"},
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
	if showRaw, ok := input["show"].([]interface{}); ok && len(showRaw) > 0 {
		// show = add semantics: merge show fields with base fields, show-fields first (priority)
		showFields := make([]string, 0, len(showRaw))
		for _, s := range showRaw {
			if name, ok := s.(string); ok {
				showFields = append(showFields, name)
			}
		}
		seen := make(map[string]bool, len(showFields)+len(fields))
		merged := make([]string, 0, len(showFields)+len(fields))
		for _, f := range showFields {
			if !seen[f] {
				merged = append(merged, f)
				seen[f] = true
			}
		}
		for _, f := range fields {
			if !seen[f] {
				merged = append(merged, f)
				seen[f] = true
			}
		}
		fields = merged
	}

	if hideRaw, ok := input["hide"].([]interface{}); ok && len(hideRaw) > 0 {
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

	// Step 7: Apply layout/size overrides
	layoutExplicit := false
	if layoutStr, ok := input["layout"].(string); ok && layoutStr != "" {
		layout = layoutStr
		layoutExplicit = true
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

	// Step 7.5: Parse color and direction
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

	// Step 8.5: Apply max atoms constraint per size
	if max, ok := MaxAtomsPerSize[string(size)]; ok && len(fieldConfigs) > max {
		fieldConfigs = fieldConfigs[:max]
	}

	// Step 9: Determine template and formation mode
	template := "GenericCard"
	formationMode := parseFormationType(layout)

	// Step 10: Build formation
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

	// Step 11: Apply color, per-atom size, shape, and direction to widgets
	if len(colorMap) > 0 {
		for wi := range formation.Widgets {
			applyAtomColors(formation.Widgets[wi].Atoms, colorMap)
		}
	}
	if len(perAtomSize) > 0 {
		for wi := range formation.Widgets {
			applyAtomMeta(formation.Widgets[wi].Atoms, perAtomSize, "size")
		}
	}
	if len(shapeMap) > 0 {
		for wi := range formation.Widgets {
			applyAtomMeta(formation.Widgets[wi].Atoms, shapeMap, "shape")
		}
	}
	if direction != "" {
		for wi := range formation.Widgets {
			if formation.Widgets[wi].Meta == nil {
				formation.Widgets[wi].Meta = make(map[string]interface{})
			}
			formation.Widgets[wi].Meta["direction"] = direction
		}
	}

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
	return &domain.ToolResult{
		Content: fmt.Sprintf("ok: rendered %d entities with visual_assembly layout=%s size=%s fields=%v", totalEntities, layout, size, fields),
	}, nil
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
