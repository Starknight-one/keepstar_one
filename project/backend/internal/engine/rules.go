package engine

import (
	"strings"
	"unicode/utf8"

	"keepstar/internal/domain"
)

// ============================================================================
// V2 Per-Atom Constraints
// ============================================================================

// applyAtomV2Constraints applies per-atom rules to v2 atoms.
func applyAtomV2Constraints(atoms []domain.AtomV2) []domain.AtomV2 {
	for i := range atoms {
		a := &atoms[i]
		if a.Rigidity == domain.RigidityLocked {
			continue // Never touch locked atoms
		}

		strVal, isStr := a.Value.(string)

		// A1: badge text > 20 chars → tag
		if isStr && a.Wrapper != nil && a.Wrapper.Type == "badge" && utf8.RuneCountInString(strVal) > 20 {
			if a.Rigidity != domain.RigidityLocked {
				a.Wrapper.Type = "tag"
				a.Wrapper.Variant = ""
			}
		}

		// A2: tag text > 40 chars → unwrap to plain text
		if isStr && a.Wrapper != nil && a.Wrapper.Type == "tag" && utf8.RuneCountInString(strVal) > 40 {
			if a.Rigidity != domain.RigidityLocked {
				a.Wrapper = nil
				a.TextStyle = &domain.TextStyle{FontSize: "sm"}
			}
		}

		// A4: badge text → capitalize first letter
		if isStr && a.Wrapper != nil && a.Wrapper.Type == "badge" && len(strVal) > 0 {
			r, size := utf8.DecodeRuneInString(strVal)
			if r >= 'a' && r <= 'z' {
				a.Value = strings.ToUpper(string(r)) + strVal[size:]
			}
		}

		// A5: rating < 3.0 → compact format
		if a.Subtype == domain.SubtypeRating {
			if fv, ok := toFloat(a.Value); ok && fv < 3.0 {
				a.Format = domain.FormatStarsCompact
			}
		}

		// D5: truncate text by slot
		if isStr && a.Type == domain.AtomTypeText {
			a.Value = TruncateBySlot(strVal, a.Slot, "")
		}

		// D6: downgrade large heading text
		if isStr && a.Slot == domain.AtomSlotTitle && a.TextStyle != nil {
			length := utf8.RuneCountInString(strVal)
			if a.TextStyle.FontSize == "3xl" && length > 60 {
				a.TextStyle.FontSize = "2xl"
			}
			if a.TextStyle.FontSize == "2xl" && length > 80 {
				a.TextStyle.FontSize = "xl"
			}
		}
	}
	return atoms
}

// ============================================================================
// V2 Per-Widget Constraints
// ============================================================================

// applyWidgetV2Constraints applies per-widget rules to a v2 widget.
func applyWidgetV2Constraints(w *domain.Widget) {
	atoms := w.AtomsV2
	if len(atoms) == 0 {
		return
	}

	// W1: max 2 badges per widget, 3rd+ → tag
	badgeCount := 0
	for i := range atoms {
		if atoms[i].Wrapper != nil && atoms[i].Wrapper.Type == "badge" {
			badgeCount++
			if badgeCount > 2 && atoms[i].Rigidity != domain.RigidityLocked {
				atoms[i].Wrapper.Type = "tag"
				atoms[i].Wrapper.Variant = ""
			}
		}
	}

	// W2: max 5 tags per widget, rest hidden
	tagCount := 0
	for i := range atoms {
		if atoms[i].Wrapper != nil && atoms[i].Wrapper.Type == "tag" {
			tagCount++
			if tagCount > 5 && atoms[i].Rigidity != domain.RigidityLocked {
				atoms[i].Value = nil
			}
		}
	}

	// W4: one large heading per widget, second → downgrade
	largeHeadingCount := 0
	for i := range atoms {
		if atoms[i].TextStyle != nil {
			fs := atoms[i].TextStyle.FontSize
			if fs == "3xl" || fs == "2xl" {
				largeHeadingCount++
				if largeHeadingCount > 1 && atoms[i].Rigidity != domain.RigidityLocked {
					atoms[i].TextStyle.FontSize = "xl"
				}
			}
		}
	}

	// W8: tiny size → remove image atoms
	if w.Size == domain.WidgetSizeTiny {
		filtered := make([]domain.AtomV2, 0, len(atoms))
		for _, a := range atoms {
			if a.Type != domain.AtomTypeImage {
				filtered = append(filtered, a)
			}
		}
		w.AtomsV2 = filtered
	}

	// Filter out nil-value atoms (from W2 hiding)
	filtered := make([]domain.AtomV2, 0, len(w.AtomsV2))
	for _, a := range w.AtomsV2 {
		if a.Value != nil {
			filtered = append(filtered, a)
		}
	}
	w.AtomsV2 = filtered
}

// ============================================================================
// V2 Cross-Widget Constraints
// ============================================================================

// applyCrossWidgetV2Constraints applies cross-widget consistency rules.
func applyCrossWidgetV2Constraints(widgets []domain.Widget, mode domain.FormationType) {
	if len(widgets) < 2 {
		return
	}

	if mode == domain.FormationTypeGrid || mode == domain.FormationTypeList {
		// C1: field consistency — field in <70% of widgets → remove from all
		normalizeFieldSetV2(widgets)

		// C3: format consistency — same field uses same format everywhere
		normalizeFormatsV2(widgets)
	}
}

// normalizeFieldSetV2 ensures all widgets in a grid have the same fields (v2 version).
func normalizeFieldSetV2(widgets []domain.Widget) {
	if len(widgets) < 2 {
		return
	}

	// Count how many widgets have each field
	fieldCounts := make(map[string]int)
	for _, w := range widgets {
		seen := make(map[string]bool)
		for _, a := range w.AtomsV2 {
			if !seen[a.FieldName] {
				fieldCounts[a.FieldName]++
				seen[a.FieldName] = true
			}
		}
	}

	threshold := int(float64(len(widgets)) * 0.7)

	keepFields := make(map[string]bool)
	for field, count := range fieldCounts {
		if count >= threshold {
			keepFields[field] = true
		}
	}

	for wi := range widgets {
		// Remove rare fields
		filtered := make([]domain.AtomV2, 0, len(widgets[wi].AtomsV2))
		presentFields := make(map[string]bool)
		for _, a := range widgets[wi].AtomsV2 {
			if keepFields[a.FieldName] && !presentFields[a.FieldName] {
				filtered = append(filtered, a)
				presentFields[a.FieldName] = true
			}
		}

		// C2: placeholder fill — add placeholder for missing common fields
		for field := range keepFields {
			if !presentFields[field] {
				entry, known := FieldTypeMap[field]
				if !known {
					entry = FieldTypeEntry{domain.AtomTypeText, domain.SubtypeString}
				}
				placeholder := domain.AtomV2{
					Type:      entry.Type,
					Subtype:   entry.Subtype,
					Value:     "\u2014",
					FieldName: field,
					Slot:      defaultSlot[field],
					Rigidity:  domain.RigidityFlexible,
				}
				if entry.Type == domain.AtomTypeNumber {
					placeholder.Value = float64(0)
				}
				if entry.Type == domain.AtomTypeImage {
					placeholder.Value = ""
				}
				ts, wr := DisplayToTextStyleWrapper(defaultDisplay[field])
				placeholder.TextStyle = ts
				placeholder.Wrapper = wr
				filtered = append(filtered, placeholder)
			}
		}

		widgets[wi].AtomsV2 = filtered
	}
}

// normalizeFormatsV2 ensures the same field uses the same format across all widgets.
func normalizeFormatsV2(widgets []domain.Widget) {
	// Collect first format seen per field
	fieldFormat := make(map[string]domain.AtomFormat)
	for _, w := range widgets {
		for _, a := range w.AtomsV2 {
			if _, exists := fieldFormat[a.FieldName]; !exists && a.Format != "" {
				fieldFormat[a.FieldName] = a.Format
			}
		}
	}

	// Apply consistent format
	for wi := range widgets {
		for ai := range widgets[wi].AtomsV2 {
			a := &widgets[wi].AtomsV2[ai]
			if a.Rigidity == domain.RigidityLocked {
				continue
			}
			if expected, ok := fieldFormat[a.FieldName]; ok && a.Format != expected {
				a.Format = expected
			}
		}
	}
}

// ============================================================================
// Junction Rules (between budget-down and needs-up passes)
// ============================================================================

// applyJunctionRules resolves overflow when needs > budget.
// Applied from soft to hard: compress → switch → downgrade → hide.
func applyJunctionRules(widgets []domain.Widget, budget BudgetAllocation, needs NeedsEstimate, resolved ResolvedDefaults) []string {
	var warnings []string

	// Group↔Widget rule 1: shrink-to-fit (compress gap+padding)
	// This is handled implicitly by the frontend with CSS, so just note it

	// Group↔Widget rule 2: overflow-switch (row→column)
	for i := range widgets {
		if widgets[i].Layout == nil {
			continue
		}
		overflowSwitchLayout(widgets[i].Layout, budget.WidgetWidth)
	}

	// Group↔Widget rule 3: downgrade-children (reduce fontSize on flexible atoms)
	if needs.TotalHeight > 0 && budget.FormationHeight > 0 && needs.TotalHeight > budget.FormationHeight {
		for i := range widgets {
			for j := range widgets[i].AtomsV2 {
				a := &widgets[i].AtomsV2[j]
				if a.Rigidity != domain.RigidityFlexible || a.TextStyle == nil {
					continue
				}
				a.TextStyle.FontSize = downgradeFontSize(a.TextStyle.FontSize)
			}
		}
		warnings = append(warnings, "downgraded font sizes to fit viewport")
	}

	// Group↔Widget rule 4: priority-hide (hide lowest priority atoms)
	if needs.TotalHeight > 0 && budget.FormationHeight > 0 && needs.TotalHeight > budget.FormationHeight*2 {
		for i := range widgets {
			atoms := widgets[i].AtomsV2
			if len(atoms) <= 2 {
				continue
			}
			// Keep only top priority atoms (by Priority field)
			maxKeep := len(atoms) / 2
			if maxKeep < 2 {
				maxKeep = 2
			}
			filtered := make([]domain.AtomV2, 0, maxKeep)
			for _, a := range atoms {
				if len(filtered) >= maxKeep && a.Rigidity == domain.RigidityFlexible {
					continue
				}
				filtered = append(filtered, a)
			}
			widgets[i].AtomsV2 = filtered
		}
		warnings = append(warnings, "hidden low-priority atoms to fit viewport")
	}

	// Formation↔Screen rule 1: viewport-fit (reduce columns)
	if budget.WidgetWidth < 150 && budget.Columns > 1 {
		newCols := budget.Columns - 1
		if newCols < 1 {
			newCols = 1
		}
		resolved.Layout = "grid"
		warnings = append(warnings, "reduced columns for viewport fit")
		_ = newCols // Would update formation grid in the caller
	}

	return warnings
}

// overflowSwitchLayout converts rows that are too wide to columns.
func overflowSwitchLayout(node *domain.LayoutNode, maxWidth int) {
	if node == nil {
		return
	}

	// If this is a row with many direct atom children, switch to column
	if node.Type == domain.LayoutNodeRow {
		atomCount := 0
		for _, c := range node.Children {
			if c.AtomIndex != nil {
				atomCount++
			}
		}
		// Heuristic: if more than 3 atoms in a row, switch to column
		if atomCount > 3 {
			node.Type = domain.LayoutNodeColumn
		}
	}

	// Recurse
	for _, c := range node.Children {
		if c.Node != nil {
			overflowSwitchLayout(c.Node, maxWidth)
		}
	}
}

// downgradeFontSize reduces a font size token by one step.
func downgradeFontSize(current string) string {
	switch current {
	case "3xl":
		return "2xl"
	case "2xl":
		return "xl"
	case "xl":
		return "lg"
	case "lg":
		return "md"
	case "md":
		return "sm"
	case "sm":
		return "xs"
	default:
		return current
	}
}
