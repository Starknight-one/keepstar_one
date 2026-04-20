package engine_v4

// B2 — behavior snapshots for the preset registry. Non-assertive: each test
// runs a real scenario and prints the observable outcome via t.Logf so you
// can eyeball diffs with `go test -v -run TestPresetBehavior_ ./...`.

import (
	"testing"

	"keepstar_v4/internal/domain"
)

// TestPresetBehavior_AllBuildSuccessfully walks every registered preset,
// builds it, executes it through the engine with either one sample product
// (non-replicating presets) or five (replicating presets) and logs the
// resulting formation.
func TestPresetBehavior_AllBuildSuccessfully(t *testing.T) {
	presets := ListPresets()
	t.Logf("registered presets: %d", len(presets))

	for _, p := range presets {
		t.Run(p.Name, func(t *testing.T) {
			dataCount := 1
			if p.DefaultReplicate {
				dataCount = 5
			}
			data := sampleProducts(dataCount)

			out := NewEngine().Execute(ExecuteInput{
				Ops:        p.Build(),
				Data:       data,
				EntityType: "product",
				Replicate:  p.DefaultReplicate,
			})

			t.Logf("preset=%s category=%s default_replicate=%v ops=%d data=%d",
				p.Name, p.Category, p.DefaultReplicate, len(p.Build()), dataCount)
			if out.Formation == nil {
				t.Logf("  formation = nil")
				return
			}
			t.Logf("  mode=%s widgets=%d sections=%d",
				out.Formation.Mode, len(out.Formation.Widgets), len(out.Formation.Sections))
			if len(out.Formation.Widgets) > 0 {
				first := out.Formation.Widgets[0]
				entity := "<nil>"
				if first.EntityRef != nil {
					entity = string(first.EntityRef.Type) + "/" + first.EntityRef.ID
				}
				t.Logf("  widget[0] id=%q size=%s entity=%s atoms=%d",
					first.ID, first.Size, entity, len(first.Atoms))
				for i, a := range first.Atoms {
					t.Logf("    atom[%d] type=%s field=%q value=%v slot=%s",
						i, a.Type, a.FieldName, a.Value, a.Slot)
				}
			}
			if len(out.Warnings) > 0 {
				t.Logf("  warnings: %v", out.Warnings)
			}
		})
	}
}

// TestPresetBehavior_OverrideRedPrice — product_card + an override op that
// recolors the price atom. Documents that override ops run in the same
// ApplyOps batch as the preset ops, so fieldName targeting works.
func TestPresetBehavior_OverrideRedPrice(t *testing.T) {
	p, ok := GetPreset("product_card")
	if !ok {
		t.Logf("product_card not registered — skipping")
		return
	}

	ops := p.Build()
	ops = append(ops, Op{
		Type:   OpUpdate,
		Target: "price",
		Props: map[string]interface{}{
			"textStyle": map[string]interface{}{
				"color":      "red",
				"fontWeight": "bold",
			},
		},
	})

	out := NewEngine().Execute(ExecuteInput{
		Ops:        ops,
		Data:       sampleProducts(3),
		EntityType: "product",
		Replicate:  true,
	})

	t.Logf("product_card + update price color=red (3 products)")
	t.Logf("  widgets=%d", len(out.Formation.Widgets))
	for i, w := range out.Formation.Widgets {
		for j, a := range w.Atoms {
			if a.FieldName == "price" {
				t.Logf("  widget[%d].atom[%d] price textStyle=%+v", i, j, a.TextStyle)
			}
		}
	}
}

// TestPresetBehavior_OverrideDeleteField — product_card + delete rating atom.
func TestPresetBehavior_OverrideDeleteField(t *testing.T) {
	p, ok := GetPreset("product_card")
	if !ok {
		t.Logf("product_card not registered — skipping")
		return
	}

	ops := append(p.Build(), Op{Type: OpDelete, Target: "rating"})

	out := NewEngine().Execute(ExecuteInput{
		Ops:        ops,
		Data:       sampleProducts(3),
		EntityType: "product",
		Replicate:  true,
	})

	t.Logf("product_card + delete rating (3 products)")
	for i, w := range out.Formation.Widgets {
		fields := make([]string, 0, len(w.Atoms))
		for _, a := range w.Atoms {
			name := a.FieldName
			if name == "" {
				name = "<literal>"
			}
			fields = append(fields, name)
		}
		t.Logf("  widget[%d] atoms=%d fields=%v", i, len(w.Atoms), fields)
	}
}

// TestPresetBehavior_OverrideInsertChild — product_card + insert a new atom
// into the $info stack. Confirms that override ops can reference preset refs
// directly because they share the same ApplyOps batch.
func TestPresetBehavior_OverrideInsertChild(t *testing.T) {
	p, ok := GetPreset("product_card")
	if !ok {
		t.Logf("product_card not registered — skipping")
		return
	}

	ops := append(p.Build(), Op{
		Type:   OpInsert,
		Parent: "$info",
		Props: map[string]interface{}{
			"type":  "text",
			"value": "NEW!",
			"wrapper": map[string]interface{}{
				"type":    "badge",
				"variant": "primary",
			},
		},
	})

	out := NewEngine().Execute(ExecuteInput{
		Ops:        ops,
		Data:       sampleProducts(2),
		EntityType: "product",
		Replicate:  true,
	})

	t.Logf("product_card + insert NEW! badge into $info (2 products)")
	for i, w := range out.Formation.Widgets {
		literals := 0
		for _, a := range w.Atoms {
			if a.Type == domain.AtomTypeText && a.FieldName == "" {
				literals++
				t.Logf("  widget[%d] literal text value=%v wrapper=%+v", i, a.Value, a.Wrapper)
			}
		}
		t.Logf("  widget[%d] atoms=%d literals=%d", i, len(w.Atoms), literals)
	}
}

// TestPresetBehavior_SystemPreset — text_explainer with overridden title and
// body. Confirms literal-value update through preset refs (ref name == atom
// fieldName via ref registration).
func TestPresetBehavior_SystemPreset(t *testing.T) {
	p, ok := GetPreset("text_explainer")
	if !ok {
		t.Logf("text_explainer not registered — skipping")
		return
	}

	ops := append(p.Build(),
		Op{Type: OpUpdate, Target: "title", Props: map[string]interface{}{"value": "Why 5000mAh beats 3200mAh"}},
		Op{Type: OpUpdate, Target: "body", Props: map[string]interface{}{"value": "Higher capacity means longer runtime per charge."}},
	)

	out := NewEngine().Execute(ExecuteInput{
		Ops:  ops,
		Data: nil, // freestyle, no binding
	})

	t.Logf("text_explainer + overridden title and body")
	if len(out.Formation.Widgets) > 0 {
		for i, a := range out.Formation.Widgets[0].Atoms {
			t.Logf("  atom[%d] type=%s field=%q value=%v", i, a.Type, a.FieldName, a.Value)
		}
	}
	t.Logf("warnings: %v", out.Warnings)
}

// TestPresetBehavior_UnknownPreset — GetPreset with an unknown name.
func TestPresetBehavior_UnknownPreset(t *testing.T) {
	_, ok := GetPreset("no_such_preset")
	t.Logf("GetPreset(\"no_such_preset\") returned ok=%v", ok)
}
