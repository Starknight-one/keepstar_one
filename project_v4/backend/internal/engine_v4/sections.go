package engine_v4

import (
	"keepstar_v4/internal/domain"
)

// groupIntoSections walks formation.Widgets and groups widgets into
// formation.Sections by ReplicateConfig.GroupID.
//
// Grouping rules:
//   - Each literal widget (no GroupID) becomes its own single-mode section.
//     Hero/explainer/cta-style blocks need their own layout container — they
//     can't share a row with replicate-group cards.
//   - Consecutive widgets sharing the same non-empty GroupID collapse into a
//     single grid-mode section. Columns come from GridColumnsForCount(n).
//   - Order of formation.Widgets is preserved across sections.
//
// Single-section rollback: if grouping yields exactly one section, the
// function flattens back to formation.Widgets + formation.Mode (taken from
// the section). This preserves backwards compat for the legacy single-widget
// flows ("show me creams" → 6 cards → flat grid; "show me details" → 1
// detail → flat single).
//
// Multi-section result sets formation.Mode = composed, formation.Widgets = nil,
// and formation.Sections = the grouped result. The frontend FormationRenderer
// already handles the recursive sections path — no frontend change needed.
//
// Mutates formation in place. No-ops on nil formation, empty Widgets, or if
// Sections were already populated by an upstream caller (don't double-group).
func groupIntoSections(formation *domain.FormationWithData) {
	if formation == nil || len(formation.Widgets) == 0 {
		return
	}
	if len(formation.Sections) > 0 {
		return
	}

	var sections []domain.FormationSection
	var bucket []domain.Widget
	currentGroup := ""

	flush := func() {
		if len(bucket) == 0 {
			return
		}
		sec := domain.FormationSection{Widgets: bucket}
		first := bucket[0]
		if first.ReplicateConfig != nil && first.ReplicateConfig.GroupID != "" {
			sec.Mode = domain.FormationTypeGrid
			sec.Grid = &domain.GridConfig{Cols: GridColumnsForCount(len(bucket))}
		} else {
			sec.Mode = domain.FormationTypeSingle
		}
		sections = append(sections, sec)
		bucket = nil
	}

	for _, w := range formation.Widgets {
		gid := ""
		if w.ReplicateConfig != nil {
			gid = w.ReplicateConfig.GroupID
		}

		if gid == "" {
			// Literal: flush any pending replicate group first, then this
			// literal becomes its own single-mode section.
			flush()
			currentGroup = ""
			bucket = []domain.Widget{w}
			flush()
			continue
		}

		// Replicate-group widget: flush on group change, then accumulate.
		if gid != currentGroup {
			flush()
			currentGroup = gid
		}
		bucket = append(bucket, w)
	}
	flush()

	if len(sections) == 0 {
		return
	}

	// Single-section rollback for backwards compat. Legacy single-widget flows
	// (one preset, one detail) stay flat — frontend keeps rendering them via
	// the existing single/grid paths.
	if len(sections) == 1 {
		sec := sections[0]
		// Don't override an explicitly-set Mode (e.g. user passed input.Layout).
		if formation.Mode == "" {
			formation.Mode = sec.Mode
		}
		if formation.Grid == nil && sec.Grid != nil {
			formation.Grid = sec.Grid
		}
		formation.Widgets = sec.Widgets
		formation.Sections = nil
		return
	}

	// Multi-section composition: sections are the source of truth.
	formation.Mode = domain.FormationTypeComposed
	formation.Widgets = nil
	formation.Sections = sections
}
