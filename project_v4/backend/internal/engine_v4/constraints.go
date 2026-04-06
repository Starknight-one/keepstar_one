package engine_v4

import (
	"unicode/utf8"

	"keepstar_v4/internal/domain"
)

// ApplyConstraints runs constraint rules on the formation.
// ALL atoms go through constraints — no RigidityLocked bypass for inserted atoms.
//
// Cross-widget constraints (C1 field-presence threshold, C3 format consistency)
// are scoped to replicate groups: clones from one template share a GroupID and
// get cross-checked against each other only. Literal widgets (no GroupID) are
// each treated as their own group of one — no cross-constraints apply, so a
// hero with one field doesn't drag down a gallery of cards or vice versa.
func ApplyConstraints(formation *domain.FormationWithData) {
	if formation == nil {
		return
	}

	var allWidgets []*domain.Widget
	for i := range formation.Widgets {
		allWidgets = append(allWidgets, &formation.Widgets[i])
	}
	for i := range formation.Sections {
		for j := range formation.Sections[i].Widgets {
			allWidgets = append(allWidgets, &formation.Sections[i].Widgets[j])
		}
	}

	// Per-atom constraints
	for _, w := range allWidgets {
		applyAtomConstraints(w.Atoms)
	}

	// Per-widget constraints
	for _, w := range allWidgets {
		applyWidgetConstraints(w)
	}

	// Group-aware cross-widget constraints.
	// Bucket widgets by ReplicateConfig.GroupID. Literals (no GroupID) are skipped:
	// a single literal widget is a group of one and has no peers to compare against.
	groups := make(map[string][]*domain.Widget)
	for _, w := range allWidgets {
		if w.ReplicateConfig == nil || w.ReplicateConfig.GroupID == "" {
			continue
		}
		gid := w.ReplicateConfig.GroupID
		groups[gid] = append(groups[gid], w)
	}
	for _, group := range groups {
		if len(group) > 1 {
			applyCrossWidgetConstraints(group)
		}
	}
}

// applyAtomConstraints applies per-atom rules.
func applyAtomConstraints(atoms []domain.Atom) {
	for i := range atoms {
		a := &atoms[i]

		// A1: Badge text > 20 chars → tag
		if a.Wrapper != nil && a.Wrapper.Type == "badge" {
			if strVal, ok := a.Value.(string); ok && utf8.RuneCountInString(strVal) > 20 {
				a.Wrapper.Type = "tag"
			}
		}

		// A2: Tag text > 40 chars → unwrap
		if a.Wrapper != nil && a.Wrapper.Type == "tag" {
			if strVal, ok := a.Value.(string); ok && utf8.RuneCountInString(strVal) > 40 {
				a.Wrapper = nil
			}
		}
	}
}

// applyWidgetConstraints applies per-widget rules.
func applyWidgetConstraints(w *domain.Widget) {
	// W8: Tiny size → remove image atoms
	if w.Size == domain.WidgetSizeTiny {
		filtered := w.Atoms[:0]
		for _, a := range w.Atoms {
			if a.Type != domain.AtomTypeImage {
				filtered = append(filtered, a)
			}
		}
		w.Atoms = filtered
	}
}

// applyCrossWidgetConstraints ensures consistency across widgets.
func applyCrossWidgetConstraints(widgets []*domain.Widget) {
	if len(widgets) < 2 {
		return
	}

	// C1: Field present in < 70% of widgets → remove from all
	fieldCounts := make(map[string]int)
	for _, w := range widgets {
		seen := make(map[string]bool)
		for _, a := range w.Atoms {
			if a.FieldName != "" && !seen[a.FieldName] {
				fieldCounts[a.FieldName]++
				seen[a.FieldName] = true
			}
		}
	}

	threshold := int(float64(len(widgets)) * 0.7)
	removeFields := make(map[string]bool)
	for field, count := range fieldCounts {
		if count < threshold {
			removeFields[field] = true
		}
	}

	if len(removeFields) > 0 {
		for _, w := range widgets {
			filtered := w.Atoms[:0]
			for _, a := range w.Atoms {
				if !removeFields[a.FieldName] {
					filtered = append(filtered, a)
				}
			}
			w.Atoms = filtered
		}
	}

	// C3: Same field → same format across all widgets
	fieldFormat := make(map[string]domain.AtomFormat)
	for _, w := range widgets {
		for _, a := range w.Atoms {
			if a.FieldName != "" && a.Format != "" {
				if _, exists := fieldFormat[a.FieldName]; !exists {
					fieldFormat[a.FieldName] = a.Format
				}
			}
		}
	}
	for _, w := range widgets {
		for i := range w.Atoms {
			if w.Atoms[i].FieldName != "" {
				if fmt, ok := fieldFormat[w.Atoms[i].FieldName]; ok {
					w.Atoms[i].Format = fmt
				}
			}
		}
	}
}
