package engine

import "testing"

func TestExpandSimpleRef(t *testing.T) {
	doc := NewDocument()
	doc.Children = []Node{
		{
			"type":     "frame",
			"id":       "btn",
			"name":     "Button",
			"reusable": true,
			"children": []Node{
				{"type": "rectangle", "id": "bg", "fill": "#FF0000"},
				{"type": "text", "id": "label", "content": "Click"},
			},
		},
		{"type": "ref", "id": "x1", "ref": "btn"},
	}
	cr := NewComponentResolver()
	refNode := FindNodeByID(doc, "x1")
	res := cr.Expand(refNode, doc)
	if !res.Ok {
		t.Fatalf("expand: %s", res.Error)
	}
	// Root id should be x1 (the RefNode's id)
	if NodeID(res.Root) != "x1" {
		t.Errorf("root id = %q, want x1", NodeID(res.Root))
	}
	// Children should be cloned
	cs := Children(res.Root)
	if len(cs) != 2 || NodeID(cs[1]) != "label" {
		t.Fatalf("clone children: %+v", cs)
	}
}

func TestExpandWithRootOverrides(t *testing.T) {
	doc := NewDocument()
	doc.Children = []Node{
		{
			"type":     "frame",
			"id":       "btn",
			"reusable": true,
			"opacity":  1.0,
			"children": []Node{},
		},
		{"type": "ref", "id": "x1", "ref": "btn", "opacity": 0.5},
	}
	cr := NewComponentResolver()
	res := cr.Expand(FindNodeByID(doc, "x1"), doc)
	if !res.Ok {
		t.Fatalf("expand: %s", res.Error)
	}
	if res.Root["opacity"] != 0.5 {
		t.Errorf("root opacity = %v, want 0.5 (override)", res.Root["opacity"])
	}
	// Source must not have changed
	src := FindNodeByID(doc, "btn")
	if src["opacity"] != 1.0 {
		t.Errorf("source opacity changed: %v", src["opacity"])
	}
}

func TestExpandWithDescendantOverrides(t *testing.T) {
	doc := NewDocument()
	doc.Children = []Node{
		{
			"type":     "frame",
			"id":       "btn",
			"reusable": true,
			"children": []Node{
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
	cr := NewComponentResolver()
	res := cr.Expand(FindNodeByID(doc, "x1"), doc)
	if !res.Ok {
		t.Fatalf("expand: %s", res.Error)
	}
	cs := Children(res.Root)
	if cs[0]["content"] != "Submit" {
		t.Errorf("descendant content not overridden: %v", cs[0]["content"])
	}
	// Source unchanged
	src := FindNodeByID(doc, "btn")
	srcCs := Children(src)
	if srcCs[0]["content"] != "Click" {
		t.Errorf("source content changed: %v", srcCs[0]["content"])
	}
}

func TestExpandWithSlotInjection(t *testing.T) {
	doc := NewDocument()
	doc.Children = []Node{
		{
			"type":     "frame",
			"id":       "card",
			"reusable": true,
			"children": []Node{
				{
					"type": "frame",
					"id":   "body",
					"slot": []any{"content"},
					"children": []Node{
						{"type": "text", "id": "placeholder", "content": "default"},
					},
				},
			},
		},
		{
			"type": "ref",
			"id":   "x1",
			"ref":  "card",
			"descendants": map[string]any{
				"body": map[string]any{
					"children": []Node{
						{"type": "text", "id": "injected", "content": "custom"},
					},
				},
			},
		},
	}
	cr := NewComponentResolver()
	res := cr.Expand(FindNodeByID(doc, "x1"), doc)
	if !res.Ok {
		t.Fatal(res.Error)
	}
	body := FindNodeByID(res.Root, "body")
	cs := Children(body)
	if len(cs) != 1 || NodeID(cs[0]) != "injected" {
		t.Fatalf("slot injection failed: %+v", cs)
	}
}

func TestExpandMissingSource(t *testing.T) {
	doc := NewDocument()
	doc.Children = []Node{
		{"type": "ref", "id": "x1", "ref": "ghost"},
	}
	cr := NewComponentResolver()
	res := cr.Expand(FindNodeByID(doc, "x1"), doc)
	if res.Ok {
		t.Fatal("expand should fail on missing source")
	}
}
