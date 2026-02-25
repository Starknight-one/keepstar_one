package tools

import (
	"fmt"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"

	"keepstar/internal/domain"
)

// ============================================================================
// Level 0 — Data Sanitization (applied in field getters / buildAtoms)
// ============================================================================

// D5: Truncate text by slot and layout
func TruncateBySlot(value string, slot domain.AtomSlot, layout string) string {
	maxLen := 0
	switch slot {
	case domain.AtomSlotSecondary:
		switch layout {
		case "grid":
			maxLen = 120
		case "list":
			maxLen = 200
		default:
			maxLen = 300
		}
	case domain.AtomSlotTitle:
		maxLen = 100
	case domain.AtomSlotPrimary:
		maxLen = 60
	}
	if maxLen > 0 && utf8.RuneCountInString(value) > maxLen {
		runes := []rune(value)
		return string(runes[:maxLen-1]) + "…"
	}
	return value
}

// D6: Downgrade display for long names (h1 + >60 chars → h2)
func DowngradeDisplayForLength(value string, display string) string {
	length := utf8.RuneCountInString(value)
	if display == "h1" && length > 60 {
		return "h2"
	}
	if display == "h2" && length > 80 {
		return "h3"
	}
	return display
}

// D7: Validate image URL — return empty string if invalid
func ValidateImageURL(rawURL interface{}) interface{} {
	switch v := rawURL.(type) {
	case string:
		if !isValidImageURL(v) {
			return nil
		}
		return v
	case []interface{}:
		valid := make([]interface{}, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && isValidImageURL(s) {
				valid = append(valid, s)
			}
		}
		if len(valid) == 0 {
			return nil
		}
		return valid
	case []string:
		valid := make([]string, 0, len(v))
		for _, s := range v {
			if isValidImageURL(s) {
				valid = append(valid, s)
			}
		}
		if len(valid) == 0 {
			return nil
		}
		return valid
	default:
		return nil
	}
}

func isValidImageURL(s string) bool {
	if s == "" {
		return false
	}
	u, err := url.Parse(s)
	if err != nil {
		return false
	}
	return u.Scheme == "http" || u.Scheme == "https"
}

// ============================================================================
// Level 1 — Per-Atom Constraints (after atom creation)
// ============================================================================

// ApplyAtomConstraints applies per-atom rules to all atoms in a widget
func ApplyAtomConstraints(atoms []domain.Atom) []domain.Atom {
	for i := range atoms {
		atom := &atoms[i]
		strVal, isStr := atom.Value.(string)

		// A1: badge text > 20 chars → tag
		if isStr && strings.HasPrefix(atom.Display, "badge") && utf8.RuneCountInString(strVal) > 20 {
			atom.Display = "tag"
		}

		// A2: tag text > 40 chars → body-sm
		if isStr && strings.HasPrefix(atom.Display, "tag") && utf8.RuneCountInString(strVal) > 40 {
			atom.Display = "body-sm"
		}

		// A4: badge text → capitalize first letter
		if isStr && strings.HasPrefix(atom.Display, "badge") && len(strVal) > 0 {
			r, size := utf8.DecodeRuneInString(strVal)
			if unicode.IsLower(r) {
				atom.Value = string(unicode.ToUpper(r)) + strVal[size:]
			}
		}

		// A5: rating < 3.0 → rating-compact
		if atom.Display == "rating" || atom.Display == "rating-text" {
			if fv, ok := toFloat(atom.Value); ok && fv < 3.0 {
				atom.Display = "rating-compact"
			}
		}

		// D5: truncate text by slot
		if isStr && atom.Type == domain.AtomTypeText {
			atom.Value = TruncateBySlot(strVal, atom.Slot, "")
		}

		// D6: downgrade display for long names
		if isStr && atom.Slot == domain.AtomSlotTitle {
			atom.Display = DowngradeDisplayForLength(strVal, atom.Display)
		}
	}
	return atoms
}

// ============================================================================
// Level 2 — Per-Widget Constraints (after widget assembly)
// ============================================================================

// ApplyWidgetConstraints applies per-widget rules
func ApplyWidgetConstraints(widget *domain.Widget) {
	// W1: max 2 badges per widget, third → tag
	badgeCount := 0
	for i := range widget.Atoms {
		if strings.HasPrefix(widget.Atoms[i].Display, "badge") {
			badgeCount++
			if badgeCount > 2 {
				widget.Atoms[i].Display = "tag"
			}
		}
	}

	// W2: max 5 tags per widget, rest hidden
	tagCount := 0
	for i := range widget.Atoms {
		if strings.HasPrefix(widget.Atoms[i].Display, "tag") {
			tagCount++
			if tagCount > 5 {
				widget.Atoms[i].Value = nil // will be filtered by null guard
			}
		}
	}

	// W4: one h1/h2 per widget, second → h3
	headingCount := 0
	for i := range widget.Atoms {
		d := widget.Atoms[i].Display
		if d == "h1" || d == "h2" {
			headingCount++
			if headingCount > 1 {
				widget.Atoms[i].Display = "h3"
			}
		}
	}

	// W7: horizontal + >4 atoms → vertical
	if widget.Meta != nil {
		if dir, ok := widget.Meta["direction"].(string); ok && dir == "horizontal" {
			if len(widget.Atoms) > 4 {
				widget.Meta["direction"] = "vertical"
			}
		}
	}

	// W8: tiny size → remove image atoms
	if widget.Size == domain.WidgetSizeTiny {
		filtered := make([]domain.Atom, 0, len(widget.Atoms))
		for _, a := range widget.Atoms {
			if a.Type != domain.AtomTypeImage {
				filtered = append(filtered, a)
			}
		}
		widget.Atoms = filtered
	}
}

// ============================================================================
// Level 4 — Cross-Widget Constraints (after formation assembly)
// ============================================================================

// ApplyCrossWidgetConstraints applies cross-widget rules for grid/list formations
func ApplyCrossWidgetConstraints(widgets []domain.Widget, mode domain.FormationType) {
	if len(widgets) < 2 {
		return
	}

	// C1: uniform field set in grid — field present in <70% of widgets → remove
	if mode == domain.FormationTypeGrid || mode == domain.FormationTypeList {
		normalizeFieldSet(widgets)
	}
}

// normalizeFieldSet ensures all widgets in a grid have the same fields
func normalizeFieldSet(widgets []domain.Widget) {
	if len(widgets) < 2 {
		return
	}

	// Count how many widgets have each field
	fieldCounts := make(map[string]int)
	for _, w := range widgets {
		seen := make(map[string]bool)
		for _, a := range w.Atoms {
			if !seen[a.FieldName] {
				fieldCounts[a.FieldName]++
				seen[a.FieldName] = true
			}
		}
	}

	threshold := int(float64(len(widgets)) * 0.7)

	// Build set of fields to keep
	keepFields := make(map[string]bool)
	for field, count := range fieldCounts {
		if count >= threshold {
			keepFields[field] = true
		}
	}

	// Apply: remove rare fields, add placeholder "—" for missing common fields
	for wi := range widgets {
		// Remove rare fields
		filtered := make([]domain.Atom, 0, len(widgets[wi].Atoms))
		presentFields := make(map[string]bool)
		for _, a := range widgets[wi].Atoms {
			if keepFields[a.FieldName] {
				filtered = append(filtered, a)
				presentFields[a.FieldName] = true
			}
		}

		// Add placeholder for missing common fields
		for field := range keepFields {
			if !presentFields[field] {
				entry, known := fieldTypeMap[field]
				if !known {
					entry = fieldTypeEntry{domain.AtomTypeText, domain.SubtypeString}
				}
				placeholder := domain.Atom{
					Type:      entry.Type,
					Subtype:   entry.Subtype,
					Display:   defaultDisplay[field],
					Value:     fmt.Sprintf("—"),
					FieldName: field,
					Slot:      defaultSlot[field],
				}
				// Number placeholders stay nil (null guard will handle)
				if entry.Type == domain.AtomTypeNumber || entry.Type == domain.AtomTypeImage {
					placeholder.Value = nil
				}
				filtered = append(filtered, placeholder)
			}
		}

		widgets[wi].Atoms = filtered
	}
}
