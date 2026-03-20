package engine

import (
	"keepstar/internal/domain"
)

// EngineV2Input holds all inputs needed for the v2 visual assembly pipeline.
type EngineV2Input struct {
	EntityType   domain.EntityType
	Products     []domain.Product
	Services     []domain.Service
	FieldDefs    []FieldDefinitionEntry // From field_definitions table (nil = use legacy fallback)
	Instructions *AgentInstructions     // Parsed agent instructions (nil = auto mode)
	Preset       *domain.PresetV2              // Loaded preset (nil = no preset)
	Viewport     ViewportConfig         // Screen/container dimensions
}

// EngineV2Output holds the result of the v2 pipeline.
type EngineV2Output struct {
	Formation *domain.FormationWithData
	Warnings  []string // Non-fatal issues encountered during assembly
}

// ViewportConfig describes the available screen space.
type ViewportConfig struct {
	Width  int `json:"width"`  // Container width in pixels (0 = unknown → 400px default)
	Height int `json:"height"` // Container height in pixels (0 = unknown → unlimited)
}

// EngineV2 is the main v2 visual assembly engine.
// It implements the two-pass layout algorithm with rigidity and constraint rules.
type EngineV2 struct {
	tokens DesignTokensV2
}

// NewEngineV2 creates a new v2 engine with default design tokens.
func NewEngineV2() *EngineV2 {
	return &EngineV2{
		tokens: DefaultDesignTokensV2(),
	}
}

// NewEngineV2WithTokens creates a v2 engine with custom design tokens.
func NewEngineV2WithTokens(tokens DesignTokensV2) *EngineV2 {
	return &EngineV2{tokens: tokens}
}

// Execute runs the full v2 visual assembly pipeline.
//
// Pipeline steps:
//  1. buildTypedAtoms — create AtomV2 from data using field definitions
//  2. applyValues — apply agent instructions (show/hide/order/textStyle/wrapper)
//  3. applyAtomConstraints — per-atom rules (badge overflow, text truncation)
//  4. buildLayout — auto-group atoms into LayoutNode tree
//  5. budgetDown — distribute available space top-down
//  6. needsUp — calculate actual needs bottom-up
//  7. (if needs > budget: apply junction rules, repeat max 2x)
//  8. applyWidgetConstraints — per-widget rules
//  9. buildFormation — assemble final formation
//  10. applyCrossWidgetConstraints — cross-widget consistency
func (e *EngineV2) Execute(input EngineV2Input) EngineV2Output {
	var warnings []string

	// Determine entity count and data maps
	entityCount := len(input.Products) + len(input.Services)
	if entityCount == 0 {
		return EngineV2Output{
			Formation: &domain.FormationWithData{
				Mode:    domain.FormationTypeGrid,
				Widgets: []domain.Widget{},
			},
		}
	}

	// Resolve field ranking (from definitions or legacy fallback)
	var fieldNames []string
	if len(input.FieldDefs) > 0 {
		fieldNames = FieldRankingFromDefinitions(input.FieldDefs)
	} else {
		fieldNames = fieldRanking[string(input.EntityType)]
		if fieldNames == nil {
			fieldNames = fieldRanking["product"]
		}
	}

	// Determine effective count for AutoResolve (limit takes priority if set)
	effectiveCount := entityCount
	if input.Instructions != nil && input.Instructions.Limit > 0 && input.Instructions.Limit < entityCount {
		effectiveCount = input.Instructions.Limit
	}

	// Auto-resolve layout/size from effective entity count
	resolved := AutoResolve(string(input.EntityType), effectiveCount)

	// Apply instructions overrides if present
	if input.Instructions != nil {
		fieldNames, resolved = applyInstructionOverrides(input.Instructions, fieldNames, resolved)
	}

	// Cap fields by MaxFields
	if len(fieldNames) > resolved.MaxFields {
		fieldNames = fieldNames[:resolved.MaxFields]
	}

	// Step 1: Build typed atoms for each entity
	widgets := e.buildWidgets(input, fieldNames, resolved)

	// Step 1.5: Apply PresetV2 field styling (preset overrides auto-defaults)
	if input.Preset != nil {
		for i := range widgets {
			widgets[i].AtomsV2 = applyPresetV2Fields(widgets[i].AtomsV2, input.Preset)
		}
	}

	// Step 2: Apply per-atom overrides from agent instructions (highest priority)
	if input.Instructions != nil && len(input.Instructions.Atoms) > 0 {
		for i := range widgets {
			widgets[i].AtomsV2 = applyAtomOverrides(widgets[i].AtomsV2, input.Instructions.Atoms)
		}
	}

	// Step 2.5: Apply mediaStyle and iconStyle overrides from instructions
	if input.Instructions != nil && len(input.Instructions.Atoms) > 0 {
		for i := range widgets {
			applyMediaIconOverrides(widgets[i].AtomsV2, input.Instructions.Atoms)
		}
	}

	// Step 2.6: Set default MediaStyle for image/video atoms
	for i := range widgets {
		applyDefaultMediaStyle(widgets[i].AtomsV2, resolved.Size)
	}

	// Steps 3-7: Apply constraints and layout passes
	for i := range widgets {
		// Step 3: Per-atom constraints
		widgets[i].AtomsV2 = applyAtomV2Constraints(widgets[i].AtomsV2)

		// Step 4: Build layout tree (sequential if agent specified order)
		if input.Instructions != nil && len(input.Instructions.Order) > 0 {
			widgets[i].Layout = AutoLayoutSequential(widgets[i].AtomsV2)
		} else {
			widgets[i].Layout = AutoLayout(widgets[i].AtomsV2)
		}

		// Step 4b: Apply direction override (horizontal → row layout with hero left, content right)
		if input.Instructions != nil && input.Instructions.Direction == "horizontal" && widgets[i].Layout != nil {
			applyHorizontalDirection(widgets[i].Layout)
		}
	}

	// Steps 5-6: Budget down / needs up (max 2 iterations)
	viewport := input.Viewport
	if viewport.Width == 0 {
		viewport.Width = 400 // Default chat widget width
	}
	for iteration := 0; iteration < 2; iteration++ {
		budgets := BudgetDown(viewport, resolved, len(widgets))
		needs := NeedsUp(widgets, e.tokens)

		if needs.TotalHeight <= budgets.FormationHeight || budgets.FormationHeight == 0 {
			break // Fits within budget
		}

		// Apply junction rules to resolve overflow
		ruleWarnings := applyJunctionRules(widgets, budgets, needs, resolved)
		warnings = append(warnings, ruleWarnings...)

		// F1 viewport-fit: reduce columns if widget too narrow
		if budgets.WidgetWidth < 150 && budgets.Columns > 1 {
			// Reduce columns in resolved for next BudgetDown iteration
			newCols := budgets.Columns - 1
			if newCols < 1 {
				newCols = 1
			}
			// Override via viewport width trick: re-resolve won't help,
			// so we store in resolved for formation building
			resolved.Layout = "grid"
			resolved.Size = domain.WidgetSizeMedium // Prevent tiny widgets
		}
		// F2/F4: mobile reflow / min-widget-width
		if (budgets.FormationWidth > 0 && budgets.FormationWidth < 320) || budgets.WidgetWidth < 120 {
			resolved.Layout = "list" // Force single-column
		}
	}

	// Step 8: Per-widget constraints
	for i := range widgets {
		applyWidgetV2Constraints(&widgets[i])
	}

	// Step 8.5: Apply widget container overrides from instructions
	if input.Instructions != nil {
		applyWidgetContainerOverrides(widgets, input.Instructions)
	}

	// Step 9: Build formation
	mode := ParseFormationType(resolved.Layout)
	formation := &domain.FormationWithData{
		Mode:    mode,
		Widgets: widgets,
	}
	if mode == domain.FormationTypeGrid {
		grid := CalcGridConfig(len(widgets), resolved.Size)
		// Apply columns override from instructions
		if input.Instructions != nil && input.Instructions.Columns > 0 && input.Instructions.Columns <= 4 {
			grid.Cols = input.Instructions.Columns
		}
		formation.Grid = grid
	}

	// Step 10: Cross-widget constraints (show-fields are protected from C1 normalization)
	var protectedFields []string
	if input.Instructions != nil {
		protectedFields = input.Instructions.Show
	}
	applyCrossWidgetV2Constraints(formation.Widgets, mode, protectedFields...)

	// Generate v1 compat (Atoms + Zones from AtomsV2 + Layout)
	for i := range formation.Widgets {
		WidgetV2ToLegacy(&formation.Widgets[i])
	}

	return EngineV2Output{
		Formation: formation,
		Warnings:  warnings,
	}
}

// buildWidgets creates Widget structs with AtomV2 for each entity.
func (e *EngineV2) buildWidgets(input EngineV2Input, fieldNames []string, resolved ResolvedDefaults) []domain.Widget {
	entityCount := len(input.Products) + len(input.Services)
	widgets := make([]domain.Widget, 0, entityCount)

	template := "GenericCard"
	size := resolved.Size

	// Build field configs from definitions or legacy
	var fieldConfigs []domain.FieldConfig
	if len(input.FieldDefs) > 0 {
		fieldConfigs = BuildFieldConfigsFromDefinitions(input.FieldDefs, fieldNames, nil, nil)
	} else {
		fieldConfigs = BuildFieldConfigsWithFormat(fieldNames, nil, nil)
	}

	// Build defMap for label lookup
	defMap := make(map[string]FieldDefinitionEntry, len(input.FieldDefs))
	for _, d := range input.FieldDefs {
		defMap[d.FieldName] = d
	}

	buildAtomV2s := func(data map[string]interface{}, configs []domain.FieldConfig) []domain.AtomV2 {
		atoms := make([]domain.AtomV2, 0, len(configs))
		for _, fc := range configs {
			value := data[fc.Name]
			if value == nil {
				continue
			}
			// Skip empty string values (e.g. missing description in DB)
			if strVal, ok := value.(string); ok && strVal == "" {
				continue
			}

			// D7: validate image URLs
			if fc.AtomType == domain.AtomTypeImage {
				value = ValidateImageURL(value)
				if value == nil {
					continue
				}
			}

			atom := domain.AtomV2{
				Type:      fc.AtomType,
				Subtype:   fc.Subtype,
				Value:     value,
				Format:    fc.Format,
				Slot:      fc.Slot,
				FieldName: fc.Name,
				Priority:  fc.Priority,
				Rigidity:  domain.RigidityFlexible,
			}

			// Set label from field definition
			if def, ok := defMap[fc.Name]; ok {
				atom.Label = def.Label
			}

			// Set default textStyle + wrapper from display
			atom.TextStyle, atom.Wrapper = DisplayToTextStyleWrapper(string(fc.Display))

			// Currency meta
			if fc.Subtype == domain.SubtypeCurrency {
				currency := ""
				if c, ok := data["currency"].(string); ok {
					currency = c
				}
				if currency == "" {
					currency = "$"
				}
				atom.Meta = map[string]interface{}{"currency": currency}
			}

			atoms = append(atoms, atom)
		}
		return atoms
	}

	for i, p := range input.Products {
		data := ProductToMap(p)
		atomsV2 := buildAtomV2s(data, fieldConfigs)
		w := domain.Widget{
			ID:       generateWidgetID(),
			Template: template,
			Size:     size,
			Priority: i,
			AtomsV2:  atomsV2,
			EntityRef: &domain.EntityRef{
				Type: domain.EntityTypeProduct,
				ID:   p.ID,
			},
		}
		widgets = append(widgets, w)
	}

	for i, s := range input.Services {
		data := ServiceToMap(s)
		atomsV2 := buildAtomV2s(data, fieldConfigs)
		w := domain.Widget{
			ID:       generateWidgetID(),
			Template: template,
			Size:     size,
			Priority: len(input.Products) + i,
			AtomsV2:  atomsV2,
			EntityRef: &domain.EntityRef{
				Type: domain.EntityTypeService,
				ID:   s.ID,
			},
		}
		widgets = append(widgets, w)
	}

	return widgets
}

// DisplayToTextStyleWrapper converts a legacy display string to v2 TextStyle + WrapperConfig.
func DisplayToTextStyleWrapper(display string) (*domain.TextStyle, *domain.WrapperConfig) {
	switch display {
	case "h1":
		return &domain.TextStyle{FontSize: "3xl", FontWeight: "bold", LineClamp: 2}, nil
	case "h2":
		return &domain.TextStyle{FontSize: "2xl", FontWeight: "semibold", LineClamp: 2}, nil
	case "h3":
		return &domain.TextStyle{FontSize: "xl", FontWeight: "semibold", LineClamp: 3}, nil
	case "h4":
		return &domain.TextStyle{FontSize: "lg", FontWeight: "medium", LineClamp: 3}, nil
	case "body-lg":
		return &domain.TextStyle{FontSize: "lg"}, nil
	case "body":
		return &domain.TextStyle{FontSize: "md"}, nil
	case "body-sm":
		return &domain.TextStyle{FontSize: "sm", LineClamp: 4}, nil
	case "caption":
		return &domain.TextStyle{FontSize: "xs"}, nil
	case "badge":
		return &domain.TextStyle{FontSize: "xs", FontWeight: "medium"}, &domain.WrapperConfig{Type: "badge"}
	case "badge-success":
		return &domain.TextStyle{FontSize: "xs", FontWeight: "medium"}, &domain.WrapperConfig{Type: "badge", Variant: "success"}
	case "badge-error":
		return &domain.TextStyle{FontSize: "xs", FontWeight: "medium"}, &domain.WrapperConfig{Type: "badge", Variant: "error"}
	case "badge-warning":
		return &domain.TextStyle{FontSize: "xs", FontWeight: "medium"}, &domain.WrapperConfig{Type: "badge", Variant: "warning"}
	case "tag", "tag-active":
		variant := ""
		if display == "tag-active" {
			variant = "active"
		}
		return &domain.TextStyle{FontSize: "xs"}, &domain.WrapperConfig{Type: "tag", Variant: variant}
	case "price", "price-lg":
		fs := "lg"
		if display == "price-lg" {
			fs = "xl"
		}
		return &domain.TextStyle{FontSize: fs, FontWeight: "bold"}, nil
	case "price-old":
		return &domain.TextStyle{FontSize: "md", TextDecoration: "line-through", Color: "muted"}, nil
	case "price-discount":
		return &domain.TextStyle{FontSize: "lg", FontWeight: "bold", Color: "error"}, nil
	case "rating", "rating-text", "rating-compact":
		return &domain.TextStyle{FontSize: "sm"}, nil
	case "image-cover", "image":
		return nil, nil
	case "avatar", "avatar-sm", "avatar-lg":
		return nil, &domain.WrapperConfig{Type: "avatar"}
	case "button-primary":
		return &domain.TextStyle{FontSize: "sm", FontWeight: "medium"}, &domain.WrapperConfig{Type: "button", Variant: "primary"}
	case "button-secondary":
		return &domain.TextStyle{FontSize: "sm", FontWeight: "medium"}, &domain.WrapperConfig{Type: "button", Variant: "secondary"}
	case "button-outline":
		return &domain.TextStyle{FontSize: "sm", FontWeight: "medium"}, &domain.WrapperConfig{Type: "button", Variant: "outline"}
	case "progress":
		return nil, &domain.WrapperConfig{Type: "progress"}
	case "percent":
		return &domain.TextStyle{FontSize: "sm"}, nil
	default:
		return &domain.TextStyle{FontSize: "md"}, nil
	}
}

// applyHorizontalDirection converts a vertical column layout into horizontal row layout.
// Hero/media goes left (fixed width), content goes right (flex).
func applyHorizontalDirection(root *domain.LayoutNode) {
	if root.Type != domain.LayoutNodeColumn || len(root.Children) < 2 {
		return
	}

	// Find hero/media child and content children
	var heroChild *domain.LayoutChild
	var contentChildren []domain.LayoutChild

	for _, c := range root.Children {
		if c.Node != nil && (c.Node.Name == "hero" || c.Node.Name == "media") {
			heroChild = &c
		} else {
			contentChildren = append(contentChildren, c)
		}
	}

	if heroChild == nil {
		return
	}

	// Build horizontal row: [hero (fixed)] [content column (flex)]
	contentCol := &domain.LayoutNode{
		Type: domain.LayoutNodeColumn,
		Gap:  "sm",
		Name: "content",
	}
	contentCol.Children = contentChildren

	heroChild.Node.Overflow = "hidden"

	root.Type = domain.LayoutNodeRow
	root.Gap = "md"
	root.Align = "start"
	root.Children = []domain.LayoutChild{
		*heroChild,
		domain.NewNodeChild(contentCol),
	}
}

// applyInstructionOverrides modifies field list and resolved defaults based on agent instructions.
func applyInstructionOverrides(instr *AgentInstructions, fields []string, resolved ResolvedDefaults) ([]string, ResolvedDefaults) {
	// Apply show fields — additive to current resolved set (not all field definitions)
	// show:["rating"] + current [images,name,price] → [images,name,price,rating], MaxFields=4
	if len(instr.Show) > 0 {
		// Start with resolved fields (the current visible set), not all field defs
		baseFields := resolved.Fields
		if len(baseFields) == 0 {
			baseFields = fields
		}
		seen := make(map[string]bool)
		merged := make([]string, 0)
		// Keep existing fields first
		for _, f := range baseFields {
			if !seen[f] {
				merged = append(merged, f)
				seen[f] = true
			}
		}
		// Append new show fields
		newCount := 0
		for _, f := range instr.Show {
			if !seen[f] {
				merged = append(merged, f)
				seen[f] = true
				newCount++
			}
		}
		fields = merged
		// Raise MaxFields to accommodate added fields
		needed := len(merged)
		if needed > resolved.MaxFields {
			resolved.MaxFields = needed
		}
		_ = newCount
	}

	// Apply hide fields
	if len(instr.Hide) > 0 {
		hideSet := make(map[string]bool)
		for _, f := range instr.Hide {
			hideSet[f] = true
		}
		filtered := make([]string, 0, len(fields))
		for _, f := range fields {
			if !hideSet[f] {
				filtered = append(filtered, f)
			}
		}
		fields = filtered
	}

	// Apply order
	if len(instr.Order) > 0 {
		seen := make(map[string]bool)
		ordered := make([]string, 0)
		for _, f := range instr.Order {
			if !seen[f] {
				ordered = append(ordered, f)
				seen[f] = true
			}
		}
		for _, f := range fields {
			if !seen[f] {
				ordered = append(ordered, f)
				seen[f] = true
			}
		}
		fields = ordered
	}

	// Apply layout override
	if instr.Layout != "" {
		resolved.Layout = instr.Layout
	}

	// Apply size override
	if instr.Size != "" {
		resolved.Size = domain.WidgetSize(instr.Size)
	}

	return fields, resolved
}

// mergeTextStyle merges src into dst (src overrides non-zero fields in dst).
func mergeTextStyle(dst, src *domain.TextStyle) {
	if src.FontSize != "" {
		dst.FontSize = src.FontSize
	}
	if src.FontWeight != "" {
		dst.FontWeight = src.FontWeight
	}
	if src.Color != "" {
		dst.Color = src.Color
	}
	if src.TextDecoration != "" {
		dst.TextDecoration = src.TextDecoration
	}
	if src.TextTransform != "" {
		dst.TextTransform = src.TextTransform
	}
	if src.LineClamp > 0 {
		dst.LineClamp = src.LineClamp
	}
	if src.Truncate > 0 {
		dst.Truncate = src.Truncate
	}
	if src.LineHeight != "" {
		dst.LineHeight = src.LineHeight
	}
	if src.LetterSpacing != "" {
		dst.LetterSpacing = src.LetterSpacing
	}
}

// applyPresetV2Fields applies PresetV2 field styling to atoms.
// Preset values override auto-defaults but are overridden by agent instructions.
func applyPresetV2Fields(atoms []domain.AtomV2, preset *domain.PresetV2) []domain.AtomV2 {
	fieldMap := make(map[string]domain.PresetV2Field, len(preset.Fields))
	for _, f := range preset.Fields {
		fieldMap[f.FieldName] = f
	}
	for i := range atoms {
		pf, ok := fieldMap[atoms[i].FieldName]
		if !ok {
			continue
		}
		// TextStyle: merge (preset overrides defaults, doesn't wipe)
		if pf.TextStyle != nil {
			if atoms[i].TextStyle == nil {
				atoms[i].TextStyle = &domain.TextStyle{}
			}
			mergeTextStyle(atoms[i].TextStyle, pf.TextStyle)
		}
		// Wrapper: replace
		if pf.Wrapper != nil {
			atoms[i].Wrapper = pf.Wrapper
		}
		// Format: override if set
		if pf.Format != "" {
			atoms[i].Format = pf.Format
		}
		// Slot: override if set
		if pf.Slot != "" {
			atoms[i].Slot = pf.Slot
		}
		// Priority + Rigidity
		atoms[i].Priority = pf.Priority
		if pf.Rigidity != "" {
			atoms[i].Rigidity = pf.Rigidity
		}
	}
	return atoms
}

// applyAtomOverrides applies per-atom overrides from AgentInstructions.
// These have the highest priority (agent explicitly requested).
func applyAtomOverrides(atoms []domain.AtomV2, overrides map[string]AtomOverride) []domain.AtomV2 {
	if len(overrides) == 0 {
		return atoms
	}
	for i := range atoms {
		ov, ok := overrides[atoms[i].FieldName]
		if !ok {
			continue
		}
		if ov.TextStyle != nil {
			if atoms[i].TextStyle == nil {
				atoms[i].TextStyle = &domain.TextStyle{}
			}
			mergeTextStyle(atoms[i].TextStyle, ov.TextStyle)
		}
		if ov.Wrapper != nil {
			atoms[i].Wrapper = ov.Wrapper
		}
		if ov.Format != "" {
			atoms[i].Format = domain.AtomFormat(ov.Format)
		}
		if ov.Color != "" {
			if atoms[i].TextStyle == nil {
				atoms[i].TextStyle = &domain.TextStyle{}
			}
			atoms[i].TextStyle.Color = ov.Color
		}
		if ov.Rigidity != "" {
			atoms[i].Rigidity = ov.Rigidity
		}
	}
	return atoms
}

// applyDefaultMediaStyle sets default MediaStyle for image/video atoms based on widget size.
func applyDefaultMediaStyle(atoms []domain.AtomV2, size domain.WidgetSize) {
	for i := range atoms {
		if atoms[i].MediaStyle != nil {
			continue // Already set (by preset or override)
		}
		switch atoms[i].Type {
		case domain.AtomTypeImage:
			switch size {
			case domain.WidgetSizeLarge, "xl":
				atoms[i].MediaStyle = &domain.MediaStyle{AspectRatio: "16:9", ObjectFit: "cover"}
			case domain.WidgetSizeMedium:
				atoms[i].MediaStyle = &domain.MediaStyle{AspectRatio: "4:3", ObjectFit: "cover"}
			default: // small, tiny
				atoms[i].MediaStyle = &domain.MediaStyle{AspectRatio: "1:1", ObjectFit: "cover"}
			}
		case domain.AtomTypeVideo:
			atoms[i].MediaStyle = &domain.MediaStyle{AspectRatio: "16:9", ObjectFit: "cover", Controls: true}
		case domain.AtomTypeAudio:
			atoms[i].MediaStyle = &domain.MediaStyle{Controls: true}
		}
	}
}

// applyMediaIconOverrides applies mediaStyle and iconStyle from atom overrides.
func applyMediaIconOverrides(atoms []domain.AtomV2, overrides map[string]AtomOverride) {
	for i := range atoms {
		ov, ok := overrides[atoms[i].FieldName]
		if !ok {
			continue
		}
		if ov.MediaStyle != nil {
			atoms[i].MediaStyle = ov.MediaStyle
		}
		if ov.IconStyle != nil {
			atoms[i].IconStyle = ov.IconStyle
		}
	}
}

// applyWidgetContainerOverrides applies widget-level visual container overrides from instructions.
func applyWidgetContainerOverrides(widgets []domain.Widget, instr *AgentInstructions) {
	if instr == nil {
		return
	}
	for i := range widgets {
		if widgets[i].Layout == nil {
			continue
		}
		root := widgets[i].Layout
		if instr.WidgetPadding != "" {
			root.Padding = instr.WidgetPadding
		}
		if instr.WidgetBackground != "" {
			root.Background = instr.WidgetBackground
		}
		if instr.WidgetBorderRadius != "" {
			root.BorderRadius = instr.WidgetBorderRadius
		}
		if instr.WidgetShadow != "" {
			root.Shadow = instr.WidgetShadow
		}
		if instr.WidgetBorder != "" {
			root.Border = instr.WidgetBorder
		}
		if instr.Gap != "" {
			root.Gap = instr.Gap
		}
	}
}

// AutoSelectPreset chooses a default preset based on entity type, layout and size.
// Used when the agent doesn't explicitly specify a preset.
func AutoSelectPreset(entityType domain.EntityType, layout string, size domain.WidgetSize) string {
	if entityType == domain.EntityTypeService {
		switch {
		case layout == "single" || size == domain.WidgetSizeLarge:
			return "service_detail"
		default:
			return "service_card"
		}
	}
	// Default: product
	switch {
	case layout == "single" || size == domain.WidgetSizeLarge:
		return "product_card_detail"
	case layout == "list":
		return "product_row"
	default:
		return "product_card_grid"
	}
}
