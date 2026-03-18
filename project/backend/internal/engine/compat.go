package engine

import (
	"keepstar/internal/domain"
)

// AtomV2ToLegacy converts an AtomV2 to a legacy Atom for backward compatibility.
func AtomV2ToLegacy(a domain.AtomV2) domain.Atom {
	display := inferLegacyDisplay(a)

	legacy := domain.Atom{
		Type:      a.Type,
		Subtype:   a.Subtype,
		Format:    a.Format,
		Display:   display,
		Value:     a.Value,
		FieldName: a.FieldName,
		Slot:      a.Slot,
		Meta:      a.Meta,
	}
	return legacy
}

// AtomsV2ToLegacy converts a slice of AtomV2 to legacy Atoms.
func AtomsV2ToLegacy(atoms []domain.AtomV2) []domain.Atom {
	result := make([]domain.Atom, len(atoms))
	for i, a := range atoms {
		result[i] = AtomV2ToLegacy(a)
	}
	return result
}

// inferLegacyDisplay maps textStyle+wrapper back to a single v1 display string.
func inferLegacyDisplay(a domain.AtomV2) string {
	// If wrapper is set, it takes precedence for display
	if a.Wrapper != nil && a.Wrapper.Type != "" && a.Wrapper.Type != "none" {
		switch a.Wrapper.Type {
		case "badge":
			if a.Wrapper.Variant != "" {
				return "badge-" + a.Wrapper.Variant
			}
			return "badge"
		case "tag":
			if a.Wrapper.Variant == "active" {
				return "tag-active"
			}
			return "tag"
		case "pill":
			return "tag" // pill → tag in v1
		case "button":
			if a.Wrapper.Variant != "" {
				return "button-" + a.Wrapper.Variant
			}
			return "button-primary"
		case "progress":
			return "progress"
		case "avatar":
			return "avatar"
		case "link":
			return "body" // link wrapper → body text in v1
		case "tooltip", "alert":
			return "body-sm"
		}
	}

	// Map textStyle to legacy heading/body display
	if a.TextStyle != nil {
		switch a.TextStyle.FontSize {
		case "3xl":
			return "h1"
		case "2xl":
			return "h2"
		case "xl":
			return "h3"
		case "lg":
			if a.TextStyle.FontWeight == "bold" || a.TextStyle.FontWeight == "semibold" {
				return "h4"
			}
			return "body-lg"
		case "md":
			return "body"
		case "sm":
			return "body-sm"
		case "xs":
			return "caption"
		}
	}

	// Special display for price/rating based on subtype
	if a.Subtype == domain.SubtypeCurrency {
		return "price"
	}
	if a.Subtype == domain.SubtypeRating {
		return "rating-compact"
	}

	// Image type
	if a.Type == domain.AtomTypeImage {
		return "image-cover"
	}

	return "body"
}

// LayoutToZones converts a LayoutNode tree to flat v1 Zone slice for backward compatibility.
func LayoutToZones(layout *domain.LayoutNode, atomCount int) []domain.Zone {
	if layout == nil {
		return nil
	}

	var zones []domain.Zone
	flattenLayoutNode(layout, &zones)
	return zones
}

// flattenLayoutNode recursively flattens a layout tree into v1 zones.
func flattenLayoutNode(node *domain.LayoutNode, zones *[]domain.Zone) {
	if node == nil {
		return
	}

	// Collect atom indices at this level
	var indices []int
	for _, child := range node.Children {
		if child.AtomIndex != nil {
			indices = append(indices, *child.AtomIndex)
		}
	}

	// If this node has direct atom children, create a zone
	if len(indices) > 0 {
		zoneType := layoutNodeTypeToZoneType(node)
		*zones = append(*zones, domain.Zone{
			Type:        zoneType,
			AtomIndices: indices,
		})
	}

	// Recurse into nested nodes
	for _, child := range node.Children {
		if child.Node != nil {
			flattenLayoutNode(child.Node, zones)
		}
	}
}

// layoutNodeTypeToZoneType maps v2 LayoutNodeType to v1 ZoneType.
func layoutNodeTypeToZoneType(node *domain.LayoutNode) domain.ZoneType {
	switch node.Type {
	case domain.LayoutNodeRow:
		return domain.ZoneRow
	case domain.LayoutNodeColumn:
		return domain.ZoneStack
	case domain.LayoutNodeFlow:
		return domain.ZoneFlow
	case domain.LayoutNodeSpan:
		return domain.ZoneHero
	default:
		return domain.ZoneStack
	}
}

// WidgetV2ToLegacy converts v2 widget fields to populate v1 fields for backward compatibility.
// It fills Atoms and Zones from AtomsV2 and Layout if they exist.
func WidgetV2ToLegacy(w *domain.Widget) {
	if len(w.AtomsV2) > 0 && len(w.Atoms) == 0 {
		w.Atoms = AtomsV2ToLegacy(w.AtomsV2)
	}
	if w.Layout != nil && len(w.Zones) == 0 {
		w.Zones = LayoutToZones(w.Layout, len(w.Atoms))
	}
}
