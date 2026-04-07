package engine_v4

import (
	"encoding/json"

	"keepstar_v4/internal/domain"
)

// Engine is the V4 Pencil-based visual assembly engine.
// Ops-only pipeline: ops → replicate → bind → constraints → IDs.
type Engine struct{}

// NewEngine creates a V4 engine.
func NewEngine() *Engine {
	return &Engine{}
}

// Execute runs the ops-only pipeline.
func (e *Engine) Execute(input ExecuteInput) ExecuteOutput {
	var warnings []string
	var formation *domain.FormationWithData

	// Step 1: Start with existing formation or empty
	if input.Formation != nil {
		formation = input.Formation
	} else {
		formation = &domain.FormationWithData{}
	}

	// Step 2: Apply formation-level settings
	if input.Layout != "" {
		formation.Mode = domain.FormationType(input.Layout)
	}
	if input.Columns > 0 {
		if formation.Grid == nil {
			formation.Grid = &domain.GridConfig{}
		}
		formation.Grid.Cols = input.Columns
	}
	if input.Size != "" {
		for i := range formation.Widgets {
			formation.Widgets[i].Size = domain.WidgetSize(input.Size)
		}
	}

	// Step 3: Apply operations
	if len(input.Ops) > 0 {
		opWarnings := ApplyOps(formation, input.Ops)
		warnings = append(warnings, opWarnings...)
	}

	// Step 4: Replicate — explicit flag (B3). No magic auto-replication.
	// Apply Limit first so both replication and single-bind see the sliced slice.
	if input.Limit > 0 && input.Limit < len(input.Data) {
		input.Data = input.Data[:input.Limit]
	}

	// Bridge: legacy top-level input.Replicate flag → set ReplicateConfig on the
	// single template widget. After this point only ReplicateConfig is consulted.
	// Multi-widget compositions set ReplicateConfig per-widget via ops.insertWidget
	// (Phase 2) and skip this bridge entirely.
	if input.Replicate && len(formation.Widgets) == 1 && len(formation.Sections) == 0 && len(input.Data) > 0 {
		if formation.Widgets[0].ReplicateConfig == nil {
			formation.Widgets[0].ReplicateConfig = &domain.ReplicateConfig{Enabled: true, Limit: input.Limit}
		} else {
			formation.Widgets[0].ReplicateConfig.Enabled = true
			if formation.Widgets[0].ReplicateConfig.Limit == 0 {
				formation.Widgets[0].ReplicateConfig.Limit = input.Limit
			}
		}
	}

	// Step 4a: Expand replicated widget templates in place (multi-widget safe).
	expandReplicatedWidgets(formation, input.Data, input.EntityType)

	// Step 4b: Auto-detect EntityRef + inline bind for single non-replicated entity widgets.
	autoDetectEntityRefs(formation, input.Data, input.EntityType)

	// Step 4.5: Inject default actions for entity-bound widgets
	for i := range formation.Widgets {
		if formation.Widgets[i].EntityRef != nil && len(formation.Widgets[i].Actions) == 0 {
			formation.Widgets[i].Actions = DefaultWidgetActions()
		}
	}

	// Step 5: Bind data to atoms with FieldName
	if len(input.Data) > 0 {
		BindData(formation, input.Data)
	}

	// Step 6: Apply constraints (ALL atoms, no bypass)
	ApplyConstraints(formation)

	// Step 6.5: Group widgets into sections by ReplicateConfig.GroupID.
	// Multi-widget compositions become formation.Sections; single-widget flows
	// stay flat (rollback). Frontend FormationRenderer handles both paths.
	groupIntoSections(formation)

	// Step 7: Stamp stable IDs
	StampTreeIDs(formation)

	// Step 8: Build compact tree map for Agent2 context
	treeMap := BuildTreeMap(formation)

	return ExecuteOutput{
		Formation: formation,
		TreeMap:   treeMap,
		Warnings:  warnings,
	}
}

// deepCopyWidget creates a deep copy via JSON roundtrip.
func deepCopyWidget(src domain.Widget) domain.Widget {
	data, err := json.Marshal(src)
	if err != nil {
		return src
	}
	var dst domain.Widget
	if err := json.Unmarshal(data, &dst); err != nil {
		return src
	}
	return dst
}

