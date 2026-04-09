package engine_v4

import (
	"keepstar_v4/internal/domain"
)

// PartialInput is the input for ExecuteWidgetPartial — a slice of ops that
// represent a single widget group (one insert-widget op + its child ops).
type PartialInput struct {
	Ops        []Op
	Data       []map[string]interface{}
	EntityType string
	Layout     string
	Columns    int
	Size       string
	Replicate  bool
	Limit      int
}

// PartialOutput returns the widgets produced by one partial execution.
// Usually 1 widget, but replicate-enabled templates expand to N.
type PartialOutput struct {
	Widgets  []domain.Widget
	Warnings []string
}

// ExecuteWidgetPartial runs the per-widget portion of the pipeline on a single
// widget group's ops. It does NOT apply cross-widget constraints, does NOT
// stamp formation-level tree IDs, and does NOT build the tree map. The caller
// is expected to collect partial outputs and hand them to FinalizeFormation.
//
// This is the building block used by the streaming Agent2 path to emit a
// widget as soon as its ops arrive, before the full tool input has streamed.
func (e *Engine) ExecuteWidgetPartial(input PartialInput) PartialOutput {
	formation := &domain.FormationWithData{}

	if input.Layout != "" {
		formation.Mode = domain.FormationType(input.Layout)
	}
	if input.Columns > 0 {
		formation.Grid = &domain.GridConfig{Cols: input.Columns}
	}

	var warnings []string
	if len(input.Ops) > 0 {
		warnings = append(warnings, ApplyOps(formation, input.Ops)...)
	}
	if input.Size != "" {
		for i := range formation.Widgets {
			formation.Widgets[i].Size = domain.WidgetSize(input.Size)
		}
	}

	data := input.Data
	if input.Limit > 0 && input.Limit < len(data) {
		data = data[:input.Limit]
	}

	// Legacy Replicate bridge — only when exactly one widget produced.
	if input.Replicate && len(formation.Widgets) == 1 && len(data) > 0 {
		if formation.Widgets[0].ReplicateConfig == nil {
			formation.Widgets[0].ReplicateConfig = &domain.ReplicateConfig{Enabled: true, Limit: input.Limit}
		} else {
			formation.Widgets[0].ReplicateConfig.Enabled = true
			if formation.Widgets[0].ReplicateConfig.Limit == 0 {
				formation.Widgets[0].ReplicateConfig.Limit = input.Limit
			}
		}
	}

	expandReplicatedWidgets(formation, data, input.EntityType)
	autoDetectEntityRefs(formation, data, input.EntityType)

	for i := range formation.Widgets {
		if formation.Widgets[i].EntityRef != nil && len(formation.Widgets[i].Actions) == 0 {
			formation.Widgets[i].Actions = DefaultWidgetActions()
		}
	}

	if len(data) > 0 {
		BindData(formation, data)
	}

	// Per-widget constraints only (atom + widget rules). Cross-widget is
	// deferred to FinalizeFormation where all groups are visible.
	for i := range formation.Widgets {
		applyAtomConstraints(formation.Widgets[i].Atoms)
		applyWidgetConstraints(&formation.Widgets[i])
	}

	return PartialOutput{
		Widgets:  formation.Widgets,
		Warnings: warnings,
	}
}

// FinalizeFormation assembles collected per-widget outputs into a complete
// formation, applies cross-widget constraints (C1/C3 per GroupID), groups
// into sections, stamps tree IDs, and builds the tree map.
//
// formationMeta carries layout/grid settings that would normally be applied
// in Execute step 2. Pass nil Formation to build fresh.
func (e *Engine) FinalizeFormation(widgets []domain.Widget, layout string, columns int) ExecuteOutput {
	formation := &domain.FormationWithData{
		Widgets: widgets,
	}
	if layout != "" {
		formation.Mode = domain.FormationType(layout)
	}
	if columns > 0 {
		formation.Grid = &domain.GridConfig{Cols: columns}
	}

	// Cross-widget constraints — reuses ApplyConstraints which is safe to
	// re-run: per-widget rules are idempotent, cross-widget is the new work.
	ApplyConstraints(formation)

	groupIntoSections(formation)
	StampTreeIDs(formation)
	treeMap := BuildTreeMap(formation)

	return ExecuteOutput{
		Formation: formation,
		TreeMap:   treeMap,
	}
}
