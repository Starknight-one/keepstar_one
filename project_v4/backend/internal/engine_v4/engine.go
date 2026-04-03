package engine_v4

import (
	"fmt"

	"keepstar_v4/internal/domain"
)

// Engine is the V4 Pencil-based visual assembly engine.
// ONE pipeline: preset → ops → data-binding → constraints → IDs.
type Engine struct {
	presets map[string]PresetFunc
}

// NewEngine creates a V4 engine with registered presets.
func NewEngine() *Engine {
	return &Engine{
		presets: defaultPresets(),
	}
}

// Execute runs the single unified pipeline.
func (e *Engine) Execute(input ExecuteInput) ExecuteOutput {
	var warnings []string
	var formation *domain.FormationWithData

	// Step 1: Start with existing formation or create from preset
	if input.Formation != nil {
		formation = input.Formation
	} else if input.Preset != "" {
		presetFn, ok := e.presets[input.Preset]
		if !ok {
			return ExecuteOutput{
				Warnings: []string{fmt.Sprintf("unknown preset: %s", input.Preset)},
			}
		}
		formation = presetFn(input.Data, input.EntityType)
	} else {
		// No preset, no existing formation — start with empty
		formation = &domain.FormationWithData{}
	}

	// Apply formation-level settings
	if input.Layout != "" {
		formation.Mode = domain.FormationType(input.Layout)
	}
	if input.Columns > 0 {
		if formation.Grid == nil {
			formation.Grid = &domain.GridConfig{}
		}
		formation.Grid.Cols = input.Columns
	}

	// Step 2: Apply operations
	if len(input.Ops) > 0 {
		opWarnings := ApplyOps(formation, input.Ops)
		warnings = append(warnings, opWarnings...)
	}

	// Step 3: Bind data to atoms with FieldName
	if len(input.Data) > 0 {
		BindData(formation, input.Data)
	}

	// Step 4: Apply constraints (ALL atoms, no bypass)
	ApplyConstraints(formation)

	// Step 5: Stamp stable IDs
	StampTreeIDs(formation)

	// Step 6: Build compact tree map for Agent2 context
	treeMap := BuildTreeMap(formation)

	return ExecuteOutput{
		Formation: formation,
		TreeMap:   treeMap,
		Warnings:  warnings,
	}
}

// GetPresetNames returns available preset names for tool schema.
func (e *Engine) GetPresetNames() []string {
	names := make([]string, 0, len(e.presets))
	for name := range e.presets {
		names = append(names, name)
	}
	return names
}
