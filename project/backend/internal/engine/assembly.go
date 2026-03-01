package engine

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"keepstar/internal/domain"
	"keepstar/internal/presets"
)

// BuildVisualWidgets creates widgets using field configs and the GenericCard template
func BuildVisualWidgets(fieldConfigs []domain.FieldConfig, template string, size domain.WidgetSize, count int, getEntity EntityGetterFunc, entityType domain.EntityType) []domain.Widget {
	widgets := make([]domain.Widget, 0, count)

	for i := 0; i < count; i++ {
		fieldGetter, currencyGetter, idGetter := getEntity(i)
		atoms := BuildAtoms(fieldConfigs, fieldGetter, currencyGetter)
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

// ResolveServiceFields maps product field names to service equivalents where applicable
func ResolveServiceFields(fields []string) []string {
	serviceRanking := FieldRanking()["service"]
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

// FieldRanking returns the field ranking map (for use by assembly logic)
func FieldRanking() map[string][]string {
	return fieldRanking
}

// generateWidgetID creates a unique widget ID
func generateWidgetID() string {
	return uuid.New().String()
}

// ApplyAtomColors sets color in atom.Meta for fields specified in the color map
func ApplyAtomColors(atoms []domain.Atom, colorMap map[string]string) {
	for i := range atoms {
		if color, ok := colorMap[atoms[i].FieldName]; ok && color != "" {
			if atoms[i].Meta == nil {
				atoms[i].Meta = make(map[string]interface{})
			}
			atoms[i].Meta["color"] = color
		}
	}
}

// ApplyAtomMeta sets a meta key for atoms matching fields in the map
func ApplyAtomMeta(atoms []domain.Atom, fieldMap map[string]string, metaKey string) {
	for i := range atoms {
		if val, ok := fieldMap[atoms[i].FieldName]; ok && val != "" {
			if atoms[i].Meta == nil {
				atoms[i].Meta = make(map[string]interface{})
			}
			atoms[i].Meta[metaKey] = val
		}
	}
}

// ParseFormationType converts string to FormationType
func ParseFormationType(mode string) domain.FormationType {
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

// --- Conditional Styling ---

// ConditionalRule defines a conditional styling rule
type ConditionalRule struct {
	Field   string
	Op      string
	Value   interface{}
	Display string
	Color   string
}

// ParseConditionalRules parses raw conditional rule input
func ParseConditionalRules(raw []interface{}) []ConditionalRule {
	rules := make([]ConditionalRule, 0, len(raw))
	for _, r := range raw {
		m, ok := r.(map[string]interface{})
		if !ok {
			continue
		}
		rule := ConditionalRule{
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

// ApplyConditionalStyling applies conditional styling rules to widgets
func ApplyConditionalStyling(widgets []domain.Widget, rules []ConditionalRule) {
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

// toFloat converts various numeric types to float64
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

// BuildComposedFormation builds a multi-section formation from compose[] input
func BuildComposedFormation(presetRegistry *presets.PresetRegistry, composeRaw []interface{}, products []domain.Product, services []domain.Service, displayOverrides map[string]string, formatOverrides map[string]string, template string, size domain.WidgetSize, entityType string) *domain.FormationWithData {
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

		sectionFieldConfigs := BuildFieldConfigsWithFormat(sectionFields, displayOverrides, formatOverrides)
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
			sectionWidgets = BuildVisualWidgets(sectionFieldConfigs, template, size, pCount, func(i int) (FieldGetter, CurrencyGetter, IDGetter) {
				p := products[pOff+i]
				return ProductFieldGetter(p), func() string { return p.Currency }, func() string { return p.ID }
			}, domain.EntityTypeProduct)
			productOffset += pCount
		}
		if remaining > 0 {
			sOff := serviceOffset
			sw := BuildVisualWidgets(sectionFieldConfigs, template, size, remaining, func(i int) (FieldGetter, CurrencyGetter, IDGetter) {
				s := services[sOff+i]
				return ServiceFieldGetter(s), func() string { return s.Currency }, func() string { return s.ID }
			}, domain.EntityTypeService)
			sectionWidgets = append(sectionWidgets, sw...)
			serviceOffset += remaining
		}

		// Calculate layout zones for section widgets
		sTokens := DefaultDesignTokens()
		for wi := range sectionWidgets {
			sectionWidgets[wi].Zones = CalculateZones(sectionWidgets[wi].Atoms, sTokens)
		}

		sMode := ParseFormationType(sectionMode)
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

// ApplyPostProcessing applies color, size, shape, layer, anchor, direction, place, and pagination
func ApplyPostProcessing(formation *domain.FormationWithData, colorMap, perAtomSize, shapeMap, layerMap, anchorMap map[string]string, direction, place string, paginationLimit, paginationOffset int) *domain.FormationWithData {
	// Apply color, per-atom size, shape, layer, anchor, and direction to widgets
	for wi := range formation.Widgets {
		if len(colorMap) > 0 {
			ApplyAtomColors(formation.Widgets[wi].Atoms, colorMap)
		}
		if len(perAtomSize) > 0 {
			ApplyAtomMeta(formation.Widgets[wi].Atoms, perAtomSize, "size")
		}
		if len(shapeMap) > 0 {
			ApplyAtomMeta(formation.Widgets[wi].Atoms, shapeMap, "shape")
		}
		if len(layerMap) > 0 {
			ApplyAtomMeta(formation.Widgets[wi].Atoms, layerMap, "layer")
		}
		if len(anchorMap) > 0 {
			ApplyAtomMeta(formation.Widgets[wi].Atoms, anchorMap, "anchor")
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
