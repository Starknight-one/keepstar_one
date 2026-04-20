package engine_v4

import "keepstar_v4/internal/domain"

// BindData walks the formation and fills atom values from entity data.
// Atoms with FieldName get their Value from the corresponding entity field.
// Widget index maps to entity index (widget 0 → data[0]).
func BindData(formation *domain.FormationWithData, data []map[string]interface{}) {
	if formation == nil || len(data) == 0 {
		return
	}

	// Bind top-level widgets
	for wi := range formation.Widgets {
		var entityData map[string]interface{}
		if wi < len(data) {
			entityData = data[wi]
		}
		bindWidgetAtoms(&formation.Widgets[wi], entityData)
	}

	// Bind section widgets
	widgetOffset := len(formation.Widgets)
	for si := range formation.Sections {
		for wi := range formation.Sections[si].Widgets {
			idx := widgetOffset + wi
			var entityData map[string]interface{}
			if idx < len(data) {
				entityData = data[idx]
			}
			bindWidgetAtoms(&formation.Sections[si].Widgets[wi], entityData)
		}
		widgetOffset += len(formation.Sections[si].Widgets)
	}
}

func bindWidgetAtoms(w *domain.Widget, entityData map[string]interface{}) {
	if entityData == nil {
		return
	}
	// Skip widgets already inline-bound by expandReplicatedWidgets or
	// autoDetectEntityRefs. Top-level positional BindData would otherwise
	// shift data between literal and replicated widgets in compositions.
	if w.Meta != nil {
		if bound, _ := w.Meta["__bound"].(bool); bound {
			return
		}
	}
	for i := range w.Atoms {
		a := &w.Atoms[i]
		if a.FieldName == "" {
			continue // freestyle atom, no binding
		}
		if a.Value != nil && a.Value != "" && a.Value != "<UNKNOWN>" {
			continue // already has value
		}
		if val, ok := entityData[a.FieldName]; ok {
			a.Value = val
		}
	}
}
