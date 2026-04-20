package engine_v4

import (
	"fmt"

	"keepstar_v4/internal/domain"
)

// expandReplicatedWidgets walks formation.Widgets backwards and replaces each
// widget with ReplicateConfig.Enabled=true with N clones bound to data slice items.
//
// Iteration is backwards so splice operations don't shift the indices of widgets
// not yet processed. Literal widgets (no ReplicateConfig or Enabled=false) are
// left intact, preserving multi-widget composition.
//
// Each clone:
//   - gets a fresh copy via deepCopyWidget
//   - is bound inline to data[j] (atoms with FieldName get their values)
//   - receives an EntityRef built from data[j]["id"]
//   - inherits default actions if none were set
//   - is marked Meta["__bound"] = true so the top-level BindData pass skips it
//   - shares a GroupID with siblings from the same template (for group-aware constraints)
func expandReplicatedWidgets(formation *domain.FormationWithData, data []map[string]interface{}, entityType string) {
	if formation == nil || len(formation.Widgets) == 0 {
		return
	}

	groupCounter := 0

	for i := len(formation.Widgets) - 1; i >= 0; i-- {
		cfg := formation.Widgets[i].ReplicateConfig
		if cfg == nil || !cfg.Enabled {
			continue
		}

		end := len(data)
		if cfg.Limit > 0 && cfg.Limit < end {
			end = cfg.Limit
		}

		if end == 0 {
			// No data — drop the placeholder template entirely.
			formation.Widgets = append(formation.Widgets[:i], formation.Widgets[i+1:]...)
			continue
		}

		groupCounter++
		groupID := fmt.Sprintf("rg-%d", groupCounter)

		template := formation.Widgets[i]
		clones := make([]domain.Widget, end)
		for j := 0; j < end; j++ {
			clones[j] = deepCopyWidget(template)
			clones[j].ID = "" // will be re-stamped after expansion

			if id, ok := data[j]["id"]; ok {
				clones[j].EntityRef = &domain.EntityRef{
					Type: domain.EntityType(entityType),
					ID:   fmt.Sprintf("%v", id),
				}
			}

			// Inline bind atoms — bypass the positional BindData pass entirely.
			for k := range clones[j].Atoms {
				a := &clones[j].Atoms[k]
				if a.FieldName == "" {
					continue
				}
				if val, ok := data[j][a.FieldName]; ok {
					a.Value = val
				}
			}

			if len(clones[j].Actions) == 0 {
				clones[j].Actions = DefaultWidgetActions()
			}

			clones[j].ReplicateConfig = &domain.ReplicateConfig{
				Enabled: true,
				Limit:   cfg.Limit,
				GroupID: groupID,
			}

			if clones[j].Meta == nil {
				clones[j].Meta = map[string]interface{}{}
			}
			clones[j].Meta["__bound"] = true
		}

		// Splice clones in place of the template.
		newWidgets := make([]domain.Widget, 0, len(formation.Widgets)-1+len(clones))
		newWidgets = append(newWidgets, formation.Widgets[:i]...)
		newWidgets = append(newWidgets, clones...)
		newWidgets = append(newWidgets, formation.Widgets[i+1:]...)
		formation.Widgets = newWidgets
	}
}

// autoDetectEntityRefs assigns an EntityRef to single (non-replicated) widgets
// that have at least one atom with FieldName but no EntityRef yet, AND
// inline-binds their atoms from data[dataIdx].
//
// The data index defaults to 0; ReplicateConfig.DataIndex can override to pick
// a specific entity (e.g. {dataIndex: 2} for the third item in a comparison).
// Out-of-range indices fall back to data[0].
//
// Inline binding here is critical: BindData maps widget[i] → data[i]
// positionally. In a multi-widget composition, a single entity widget at
// position 5 would otherwise receive data[5] (wrong). Binding here + the
// __bound flag bypasses that pass entirely.
//
// Replicated widgets are skipped — they already received EntityRef + bind in expand.
func autoDetectEntityRefs(formation *domain.FormationWithData, data []map[string]interface{}, entityType string) {
	if formation == nil || len(data) == 0 {
		return
	}

	for i := range formation.Widgets {
		w := &formation.Widgets[i]

		if w.EntityRef != nil {
			continue
		}
		if w.ReplicateConfig != nil && w.ReplicateConfig.Enabled {
			continue // already handled in expand
		}
		if !widgetHasFieldNameAtoms(w) {
			continue // literal widget — no entity binding needed
		}

		dataIdx := 0
		if w.ReplicateConfig != nil && w.ReplicateConfig.DataIndex > 0 {
			dataIdx = w.ReplicateConfig.DataIndex
		}
		if dataIdx >= len(data) {
			dataIdx = 0
		}

		entity := data[dataIdx]
		if id, ok := entity["id"]; ok {
			w.EntityRef = &domain.EntityRef{
				Type: domain.EntityType(entityType),
				ID:   fmt.Sprintf("%v", id),
			}
		}

		// Inline bind atoms — bypass positional BindData entirely.
		for k := range w.Atoms {
			a := &w.Atoms[k]
			if a.FieldName == "" {
				continue
			}
			if val, ok := entity[a.FieldName]; ok {
				a.Value = val
			}
		}

		if w.Meta == nil {
			w.Meta = map[string]interface{}{}
		}
		w.Meta["__bound"] = true
	}
}

func widgetHasFieldNameAtoms(w *domain.Widget) bool {
	for _, a := range w.Atoms {
		if a.FieldName != "" {
			return true
		}
	}
	return false
}
