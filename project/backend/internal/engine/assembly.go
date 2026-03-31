package engine

import (
	"strconv"

	"github.com/google/uuid"
	"keepstar/internal/domain"
)

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
