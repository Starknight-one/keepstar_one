package postgres

import (
	"fmt"
	"testing"
	"time"

	"keepstar_v5/internal/domain"
)

// fakeRows implements the deltaRows interface for scanDeltas tests. Each
// element of `rows` is the slice of values that scanDeltas will Scan into
// in order — matching the column list:
//
//	step, trigger, source, actor_id, delta_type, path, action, result,
//	template, turn_id, created_at
type fakeRows struct {
	rows  [][]any
	idx   int
	scanN int
}

func (f *fakeRows) Next() bool {
	if f.idx < len(f.rows) {
		return true
	}
	return false
}

func (f *fakeRows) Scan(dest ...any) error {
	if f.idx >= len(f.rows) {
		return fmt.Errorf("scan past end")
	}
	row := f.rows[f.idx]
	f.idx++
	f.scanN++
	if len(dest) != len(row) {
		return fmt.Errorf("scan arity: dest=%d row=%d", len(dest), len(row))
	}
	for i, src := range row {
		// Each Scan target is a pointer; we assign through it. Type switch
		// covers exactly the types scanDeltas passes.
		switch d := dest[i].(type) {
		case *int:
			if v, ok := src.(int); ok {
				*d = v
			} else if src == nil {
				*d = 0
			} else {
				return fmt.Errorf("col %d: int expected, got %T", i, src)
			}
		case *string:
			if v, ok := src.(string); ok {
				*d = v
			} else if src == nil {
				*d = ""
			} else {
				return fmt.Errorf("col %d: string expected, got %T", i, src)
			}
		case **string:
			if src == nil {
				*d = nil
			} else if v, ok := src.(string); ok {
				s := v
				*d = &s
			} else {
				return fmt.Errorf("col %d: *string expected, got %T", i, src)
			}
		case *[]byte:
			if src == nil {
				*d = nil
			} else if v, ok := src.([]byte); ok {
				*d = v
			} else {
				return fmt.Errorf("col %d: []byte expected, got %T", i, src)
			}
		case *time.Time:
			if v, ok := src.(time.Time); ok {
				*d = v
			} else {
				return fmt.Errorf("col %d: time.Time expected, got %T", i, src)
			}
		default:
			return fmt.Errorf("col %d: unhandled dest type %T", i, dest[i])
		}
	}
	return nil
}

// TestScanDeltasHappyPath verifies that scanDeltas produces a Delta with all
// fields populated when every column has a value. fakeRows takes plain
// string values for nullable text columns (real pgx delivers them via
// **string destinations, allocating internally).
func TestScanDeltasHappyPath(t *testing.T) {
	now := time.Date(2026, 5, 2, 15, 0, 0, 0, time.UTC)
	rows := &fakeRows{rows: [][]any{
		{
			3,                                                   // step
			string(domain.TriggerUserQuery),                     // trigger
			string(domain.SourceLLM),                            // source
			"agent1",                                            // actor_id
			string(domain.DeltaTypeAdd),                         // delta_type
			"data.products",                                     // path
			[]byte(`{"type":"SEARCH","tool":"catalog_search"}`), // action
			[]byte(`{"count":12,"fields":["id","name"]}`),       // result
			[]byte(`{"version":"2.10"}`),                        // template
			"turn-abc",                                          // turn_id
			now,                                                 // created_at
		},
	}}

	a := NewStateAdapter(nil, nil)
	deltas, err := a.scanDeltas(rows)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(deltas) != 1 {
		t.Fatalf("expected 1, got %d", len(deltas))
	}
	d := deltas[0]
	if d.Step != 3 || d.Trigger != domain.TriggerUserQuery {
		t.Errorf("identity: %+v", d)
	}
	if d.Source != domain.SourceLLM || d.ActorID != "agent1" {
		t.Errorf("source/actor: %+v", d)
	}
	if d.DeltaType != domain.DeltaTypeAdd || d.Path != "data.products" {
		t.Errorf("type/path: %+v", d)
	}
	if d.TurnID != "turn-abc" {
		t.Errorf("turn_id: %q", d.TurnID)
	}
	if d.Action.Type != domain.ActionSearch || d.Action.Tool != "catalog_search" {
		t.Errorf("action: %+v", d.Action)
	}
	if d.Result.Count != 12 || len(d.Result.Fields) != 2 {
		t.Errorf("result: %+v", d.Result)
	}
	if d.Template == nil || d.Template["version"] != "2.10" {
		t.Errorf("template: %+v", d.Template)
	}
	if !d.CreatedAt.Equal(now) {
		t.Errorf("created_at: %v", d.CreatedAt)
	}
}

// TestScanDeltasNullableColumns verifies graceful handling of NULL values for
// the nullable string columns (source, actor_id, delta_type, path, turn_id)
// and the nullable JSONB template. This is the path most likely to break in
// the real DB if defaults change.
func TestScanDeltasNullableColumns(t *testing.T) {
	now := time.Now()
	rows := &fakeRows{rows: [][]any{
		{
			1,                              // step
			string(domain.TriggerSystem),   // trigger (NOT NULL in schema)
			nil,                             // source — NULL
			nil,                             // actor_id — NULL
			nil,                             // delta_type — NULL
			nil,                             // path — NULL
			[]byte(`{"type":"SEARCH"}`),    // action — NOT NULL
			[]byte(`{"count":0,"fields":[]}`), // result — NOT NULL
			nil,                             // template — NULL
			nil,                             // turn_id — NULL
			now,                             // created_at
		},
	}}

	a := NewStateAdapter(nil, nil)
	deltas, err := a.scanDeltas(rows)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(deltas) != 1 {
		t.Fatalf("expected 1, got %d", len(deltas))
	}
	d := deltas[0]
	if d.Step != 1 || d.Trigger != domain.TriggerSystem {
		t.Errorf("non-null fields drift: %+v", d)
	}
	if d.Source != "" || d.ActorID != "" || d.DeltaType != "" || d.Path != "" || d.TurnID != "" {
		t.Errorf("NULL columns should produce empty strings, got: %+v", d)
	}
	if d.Template != nil {
		t.Errorf("NULL template should leave Template nil, got %+v", d.Template)
	}
	if d.Action.Type != domain.ActionSearch {
		t.Errorf("action drift: %+v", d.Action)
	}
}

// TestScanDeltasMultipleOrdering verifies scanDeltas preserves row order
// (the SQL has ORDER BY step ASC, but the helper itself just iterates).
func TestScanDeltasMultipleOrdering(t *testing.T) {
	now := time.Now()
	mkRow := func(step int) []any {
		return []any{
			step, string(domain.TriggerUserQuery),
			string(domain.SourceLLM), "a",
			string(domain.DeltaTypeAdd), "data",
			[]byte(`{}`), []byte(`{}`), nil, "", now,
		}
	}
	rows := &fakeRows{rows: [][]any{mkRow(1), mkRow(2), mkRow(3)}}

	a := NewStateAdapter(nil, nil)
	deltas, err := a.scanDeltas(rows)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(deltas) != 3 {
		t.Fatalf("got %d, want 3", len(deltas))
	}
	for i, d := range deltas {
		if d.Step != i+1 {
			t.Errorf("row %d: step %d, want %d", i, d.Step, i+1)
		}
	}
}
