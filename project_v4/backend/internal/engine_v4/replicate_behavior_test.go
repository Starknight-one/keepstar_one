package engine_v4

// Behavior tests for B3 — explicit replicate flag + limit.
//
// IMPORTANT: these tests are INTENTIONALLY non-assertive. They exercise the
// engine and log what actually happens via t.Logf. The goal is to document
// observable behavior so that after each change you can run
//
//     go test -v -run TestBehavior_ ./internal/engine_v4/...
//
// and eyeball the diff. They will never fail (no t.Fatal / t.Errorf), so
// they won't gatekeep the build — they're a living snapshot.

import (
	"fmt"
	"testing"

	"keepstar_v4/internal/domain"
)

func sampleProducts(n int) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, map[string]interface{}{
			"id":     fmt.Sprintf("p%d", i+1),
			"name":   fmt.Sprintf("Product %d", i+1),
			"price":  10.0 * float64(i+1),
			"images": fmt.Sprintf("img%d.jpg", i+1),
			"rating": 4.0,
			"brand":  fmt.Sprintf("Brand %d", i+1),
		})
	}
	return out
}

// describeFormation prints a compact snapshot of the formation.
func describeFormation(t *testing.T, label string, f *domain.FormationWithData) {
	t.Helper()
	if f == nil {
		t.Logf("[%s] formation = nil", label)
		return
	}
	t.Logf("[%s] mode=%s widgets=%d sections=%d", label, f.Mode, len(f.Widgets), len(f.Sections))
	for i, w := range f.Widgets {
		entity := "<nil>"
		if w.EntityRef != nil {
			entity = fmt.Sprintf("%s/%s", w.EntityRef.Type, w.EntityRef.ID)
		}
		t.Logf("  widget[%d] id=%q size=%s entity=%s atoms=%d", i, w.ID, w.Size, entity, len(w.Atoms))
		for j, a := range w.Atoms {
			t.Logf("    atom[%d] type=%s field=%q value=%v slot=%s", j, a.Type, a.FieldName, a.Value, a.Slot)
		}
	}
}

// TestBehavior_NoReplicate_MultiData — single template + 5 data items, Replicate=false.
// Expectation (documented, not asserted): only 1 widget, bound to data[0].
func TestBehavior_NoReplicate_MultiData(t *testing.T) {
	data := sampleProducts(5)
	out := NewEngine().Execute(ExecuteInput{
		Ops:        ProductCardGridOps(),
		Data:       data,
		EntityType: "product",
		Layout:     "grid",
		Columns:    3,
		Replicate:  false,
	})
	t.Logf("input: ops=%d data=%d replicate=false limit=0", len(ProductCardGridOps()), len(data))
	describeFormation(t, "no-replicate/multi-data", out.Formation)
	t.Logf("warnings: %v", out.Warnings)
}

// TestBehavior_Replicate_NoLimit — explicit replicate, 5 items, no limit.
func TestBehavior_Replicate_NoLimit(t *testing.T) {
	data := sampleProducts(5)
	out := NewEngine().Execute(ExecuteInput{
		Ops:        ProductCardGridOps(),
		Data:       data,
		EntityType: "product",
		Layout:     "grid",
		Columns:    3,
		Replicate:  true,
	})
	t.Logf("input: data=%d replicate=true limit=0", len(data))
	describeFormation(t, "replicate/no-limit", out.Formation)
	t.Logf("warnings: %v", out.Warnings)
}

// TestBehavior_Replicate_WithLimit — 10 items, limit=3.
func TestBehavior_Replicate_WithLimit(t *testing.T) {
	data := sampleProducts(10)
	out := NewEngine().Execute(ExecuteInput{
		Ops:        ProductCardGridOps(),
		Data:       data,
		EntityType: "product",
		Layout:     "grid",
		Columns:    3,
		Replicate:  true,
		Limit:      3,
	})
	t.Logf("input: data=%d replicate=true limit=3", len(data))
	describeFormation(t, "replicate/limit=3", out.Formation)
}

// TestBehavior_Replicate_LimitLargerThanData — limit > data size.
func TestBehavior_Replicate_LimitLargerThanData(t *testing.T) {
	data := sampleProducts(5)
	out := NewEngine().Execute(ExecuteInput{
		Ops:        ProductCardGridOps(),
		Data:       data,
		EntityType: "product",
		Replicate:  true,
		Limit:      100,
	})
	t.Logf("input: data=%d replicate=true limit=100", len(data))
	describeFormation(t, "replicate/limit>data", out.Formation)
}

// TestBehavior_NoReplicate_NoData — freestyle widget, no data at all.
func TestBehavior_NoReplicate_NoData(t *testing.T) {
	ops := []Op{
		{Type: OpInsert, Ref: "w", Parent: "formation", Props: map[string]interface{}{"type": "widget", "size": "medium"}},
		{Type: OpInsert, Ref: "root", Parent: "$w", Props: map[string]interface{}{"type": "column"}},
		{Type: OpInsert, Parent: "$root", Props: map[string]interface{}{"type": "text", "value": "Hello world", "slot": "title"}},
	}
	out := NewEngine().Execute(ExecuteInput{
		Ops:       ops,
		Data:      nil,
		Layout:    "single",
		Replicate: false,
	})
	t.Logf("input: freestyle ops=%d data=0 replicate=false", len(ops))
	describeFormation(t, "freestyle/no-data", out.Formation)
}

// TestBehavior_Replicate_ZeroWidgets — flag true but no widget template inserted.
func TestBehavior_Replicate_ZeroWidgets(t *testing.T) {
	out := NewEngine().Execute(ExecuteInput{
		Ops:       []Op{},
		Data:      sampleProducts(3),
		Replicate: true,
	})
	t.Logf("input: ops=0 data=3 replicate=true")
	describeFormation(t, "replicate/zero-widgets", out.Formation)
	t.Logf("warnings: %v", out.Warnings)
}

// TestBehavior_NoReplicate_MultiWidget — more than one widget already, Replicate=false.
// Documents that the single-widget fast-path does not fire when widgets > 1.
func TestBehavior_NoReplicate_MultiWidget(t *testing.T) {
	ops := []Op{
		{Type: OpInsert, Ref: "w1", Parent: "formation", Props: map[string]interface{}{"type": "widget"}},
		{Type: OpInsert, Parent: "$w1", Props: map[string]interface{}{"type": "text", "value": "left"}},
		{Type: OpInsert, Ref: "w2", Parent: "formation", Props: map[string]interface{}{"type": "widget"}},
		{Type: OpInsert, Parent: "$w2", Props: map[string]interface{}{"type": "text", "value": "right"}},
	}
	out := NewEngine().Execute(ExecuteInput{
		Ops:       ops,
		Data:      sampleProducts(4),
		Replicate: false,
	})
	t.Logf("input: 2 widgets, data=4, replicate=false")
	describeFormation(t, "no-replicate/two-widgets", out.Formation)
}
