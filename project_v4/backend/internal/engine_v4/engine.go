package engine_v4

import (
	"encoding/json"
	"fmt"

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

	switch {
	case input.Replicate && len(formation.Widgets) == 1 && len(formation.Sections) == 0 && len(input.Data) > 0:
		// Duplicate the template for every data item.
		replicateWidgets(formation, input.Data, input.EntityType)
	case !input.Replicate && len(formation.Widgets) == 1 && len(input.Data) > 0:
		// No replication — bind the first data item to the single widget.
		// Trim data so BindData (indexed by widget position) does not leak extras.
		input.Data = input.Data[:1]
		if id, ok := input.Data[0]["id"]; ok {
			formation.Widgets[0].EntityRef = &domain.EntityRef{
				Type: domain.EntityType(input.EntityType),
				ID:   fmt.Sprintf("%v", id),
			}
		}
	}

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

// replicateWidgets deep-copies the single template widget for each data item.
func replicateWidgets(formation *domain.FormationWithData, data []map[string]interface{}, entityType string) {
	template := formation.Widgets[0]
	widgets := make([]domain.Widget, len(data))

	for i := range data {
		widgets[i] = deepCopyWidget(template)
		widgets[i].ID = "" // will be stamped by StampTreeIDs
		if id, ok := data[i]["id"]; ok {
			widgets[i].EntityRef = &domain.EntityRef{
				Type: domain.EntityType(entityType),
				ID:   fmt.Sprintf("%v", id),
			}
		}
	}

	formation.Widgets = widgets
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

