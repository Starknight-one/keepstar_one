package engine

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestNewDocument(t *testing.T) {
	d := NewDocument()
	if d.Version != DocumentVersion {
		t.Errorf("version = %q, want %q", d.Version, DocumentVersion)
	}
	if d.Children == nil {
		t.Errorf("Children should be non-nil empty slice")
	}
	if len(d.Children) != 0 {
		t.Errorf("fresh doc should have 0 children, got %d", len(d.Children))
	}
}

// TestDocumentJSONRoundTrip verifies that a Document with a non-trivial tree
// (frames, text, ref with descendants overrides, variables, themes) survives
// Marshal → Unmarshal with the same observable shape.
func TestDocumentJSONRoundTrip(t *testing.T) {
	src := NewDocument()
	src.Themes = map[string][]string{"mode": {"light", "dark"}}
	src.Variables = map[string]VariableDef{
		"primary": {Type: VariableTypeColor, Value: "#FF0000"},
	}
	src.Children = []Node{
		{
			"type":     "frame",
			"id":       "btn",
			"reusable": true,
			"layout":   "horizontal",
			"children": []Node{
				{"type": "rectangle", "id": "bg", "fill": "#5BA4D9"},
				{"type": "text", "id": "label", "content": "Click"},
			},
		},
		{
			"type": "ref",
			"id":   "x1",
			"ref":  "btn",
			"descendants": map[string]any{
				"label": map[string]any{"content": "Submit"},
			},
		},
	}

	raw, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var dst Document
	if err := json.Unmarshal(raw, &dst); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if dst.Version != src.Version {
		t.Errorf("version drift: %q vs %q", dst.Version, src.Version)
	}
	if !reflect.DeepEqual(dst.Themes, src.Themes) {
		t.Errorf("themes drift: %+v vs %+v", dst.Themes, src.Themes)
	}
	if len(dst.Variables) != len(src.Variables) {
		t.Errorf("variables count: %d vs %d", len(dst.Variables), len(src.Variables))
	}

	// Engine helpers should still work after round-trip — Children() copes
	// with []any (the JSON unmarshal output) the same way as []Node.
	btn := FindNodeByID(&dst, "btn")
	if btn == nil {
		t.Fatal("btn not found after round-trip")
	}
	cs := Children(btn)
	if len(cs) != 2 {
		t.Fatalf("btn children count after round-trip: %d, want 2", len(cs))
	}
	if NodeID(cs[1]) != "label" {
		t.Errorf("child id after round-trip: %q, want label", NodeID(cs[1]))
	}

	// ComponentResolver should expand correctly using post-unmarshal data.
	cr := NewComponentResolver()
	res := cr.Expand(FindNodeByID(&dst, "x1"), &dst)
	if !res.Ok {
		t.Fatalf("expand after round-trip failed: %s", res.Error)
	}
	rcs := Children(res.Root)
	if len(rcs) != 2 || rcs[1]["content"] != "Submit" {
		t.Fatalf("post-roundtrip override lost: %+v", rcs)
	}
}
