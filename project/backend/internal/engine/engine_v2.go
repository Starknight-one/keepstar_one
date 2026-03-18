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

	// Steps 3-7: Apply constraints and layout passes
	for i := range widgets {
		// Step 3: Per-atom constraints
		widgets[i].AtomsV2 = applyAtomV2Constraints(widgets[i].AtomsV2)

		// Step 4: Build layout tree
		widgets[i].Layout = AutoLayout(widgets[i].AtomsV2)
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
	}

	// Step 8: Per-widget constraints
	for i := range widgets {
		applyWidgetV2Constraints(&widgets[i])
	}

	// Step 9: Build formation
	mode := ParseFormationType(resolved.Layout)
	formation := &domain.FormationWithData{
		Mode:    mode,
		Widgets: widgets,
	}
	if mode == domain.FormationTypeGrid {
		formation.Grid = CalcGridConfig(len(widgets), resolved.Size)
	}

	// Step 10: Cross-widget constraints
	applyCrossWidgetV2Constraints(formation.Widgets, mode)

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
		return &domain.TextStyle{FontSize: "3xl", FontWeight: "bold"}, nil
	case "h2":
		return &domain.TextStyle{FontSize: "2xl", FontWeight: "semibold"}, nil
	case "h3":
		return &domain.TextStyle{FontSize: "xl", FontWeight: "semibold"}, nil
	case "h4":
		return &domain.TextStyle{FontSize: "lg", FontWeight: "medium"}, nil
	case "body-lg":
		return &domain.TextStyle{FontSize: "lg"}, nil
	case "body":
		return &domain.TextStyle{FontSize: "md"}, nil
	case "body-sm":
		return &domain.TextStyle{FontSize: "sm"}, nil
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

// applyInstructionOverrides modifies field list and resolved defaults based on agent instructions.
func applyInstructionOverrides(instr *AgentInstructions, fields []string, resolved ResolvedDefaults) ([]string, ResolvedDefaults) {
	// Apply show fields (prepend to list)
	if len(instr.Show) > 0 {
		seen := make(map[string]bool)
		merged := make([]string, 0)
		for _, f := range instr.Show {
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
		// Agent explicitly requested fields — raise MaxFields to honor them
		if len(instr.Show) > resolved.MaxFields {
			resolved.MaxFields = len(instr.Show)
		}
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
