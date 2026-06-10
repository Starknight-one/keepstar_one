package engine

// Component-vocabulary PR1 contract: the three new system presets
// (product_carousel / product_comparison / product_detail_accordion) must
// render through the same zero-LLM chain the canvas-translate fixtures use:
//
//	Materialise → ExpandReplicates → ResolveAndInline → BindData → InjectDefaultActions
//
// and the new substrate/leaf vocabulary (layout.scroll:"x", collapsible +
// defaultOpen, line / rectangle / icon_font leaves) must survive into the
// final document untouched — the engine passes them through, the frontend
// renders them.

import (
	"encoding/json"
	"testing"

	"keepstar_v5/internal/engine/presets"
)

// vocabContractData mirrors canvasContractData but carries every key the
// new presets bind: brand + rating + description for comparison, tags for
// the accordion's details section.
func vocabContractData() []map[string]any {
	return []map[string]any{
		{"id": "p-0", "name": "Oak Coffee Table", "priceFormatted": "$199.00", "heroImage": "https://img.test/p0.jpg", "brand": "Oakline", "rating": 4.5, "description": "Solid oak top with rounded corners.", "tags": []any{"oak", "living-room"}},
		{"id": "p-1", "name": "Walnut Bookshelf", "priceFormatted": "$349.50", "heroImage": "https://img.test/p1.jpg", "brand": "Nutwood", "rating": 4.8, "description": "Five-shelf walnut bookcase.", "tags": []any{"walnut", "storage"}},
		{"id": "p-2", "name": "Birch Side Chair", "priceFormatted": "$89.99", "heroImage": "https://img.test/p2.jpg", "brand": "Northply", "rating": 4.1, "description": "Stackable birch plywood chair.", "tags": []any{"birch", "seating"}},
	}
}

// materialiseSystemPreset parses the embedded seed + every system component
// and merges them the way the production preset port does.
func materialiseSystemPreset(t *testing.T, name string) *Document {
	t.Helper()
	raw, ok := presets.SystemPresetSeeds[name]
	if !ok {
		t.Fatalf("preset %q not in SystemPresetSeeds", name)
	}
	var preset Document
	if err := json.Unmarshal(raw, &preset); err != nil {
		t.Fatalf("unmarshal preset %q: %v", name, err)
	}
	var comps []*Document
	for compName, compRaw := range presets.SystemComponentSeeds {
		var comp Document
		if err := json.Unmarshal(compRaw, &comp); err != nil {
			t.Fatalf("unmarshal component %q: %v", compName, err)
		}
		comps = append(comps, &comp)
	}
	return Materialise(&preset, comps)
}

// vocabClones collects the replicate clones under the named outer frame
// (children stamped with __templateOrigin "card"), asserting count and
// sequential dataIndex like canvasClones does for the fixture grid.
func vocabClones(t *testing.T, doc *Document, outerID string, wantCount int) []Node {
	t.Helper()
	outer := FindNodeByID(doc, outerID)
	if outer == nil {
		t.Fatalf("outer frame %q not found in final doc", outerID)
	}
	var clones []Node
	for _, c := range Children(outer) {
		if origin, _ := c[attrTemplateOrigin].(string); origin == "card" {
			clones = append(clones, c)
		}
	}
	if len(clones) != wantCount {
		t.Fatalf("expected %d replicated cards under %q, got %d", wantCount, outerID, len(clones))
	}
	for i, clone := range clones {
		idx, ok := readDataIndex(clone)
		if !ok || idx != i {
			t.Errorf("clone[%d] dataIndex = %v, want %d", i, clone[attrDataIndex], i)
		}
	}
	return clones
}

// nodesOfType returns every descendant of root with the given type.
func nodesOfType(root Node, typ string) []Node {
	var out []Node
	WalkNodes(root, func(n Node, _ int) {
		if NodeType(n) == typ {
			out = append(out, n)
		}
	})
	return out
}

// boundContents maps fieldBinding → content for every bound text atom
// under root.
func boundContents(root Node) map[string]any {
	out := map[string]any{}
	WalkNodes(root, func(n Node, _ int) {
		if fb, _ := n[attrFieldBinding].(string); fb != "" && NodeType(n) == NodeTypeText {
			out[fb] = n[attrContent]
		}
	})
	return out
}

// assertActionsPopulated finds the "actions" frame under root and checks
// InjectDefaultActions filled it with buttons wired to wantEntityID.
func assertActionsPopulated(t *testing.T, root Node, label string, wantEntityID any) {
	t.Helper()
	actions := FindNodeByID(root, "actions")
	if actions == nil {
		t.Errorf("%s: actions frame missing", label)
		return
	}
	buttons := Children(actions)
	if len(buttons) != 2 {
		t.Errorf("%s: actions frame expected 2 default buttons (like+cart), got %d", label, len(buttons))
		return
	}
	for _, b := range buttons {
		act, _ := b[attrAction].(map[string]any)
		if act == nil {
			t.Errorf("%s: injected button missing action payload: %v", label, b)
			continue
		}
		entity, _ := act["entity"].(map[string]any)
		if entity == nil || entity["id"] != wantEntityID {
			t.Errorf("%s: injected action entity = %v, want id %v", label, act["entity"], wantEntityID)
		}
	}
}

// TestVocabularyPreset_ProductCarousel — scroll strip fans out to 3 bound
// cards; the layout.scroll:"x" substrate attr survives the full chain.
func TestVocabularyPreset_ProductCarousel(t *testing.T) {
	doc := materialiseSystemPreset(t, "product_carousel")
	data := vocabContractData()

	runCanvasContractPipeline(t, doc, data)

	strip := FindNodeByID(doc, "strip")
	if strip == nil {
		t.Fatal("strip frame missing")
	}
	layout, _ := strip["layout"].(map[string]any)
	if layout == nil || layout["scroll"] != "x" {
		t.Errorf("strip layout.scroll = %v, want \"x\" (layout: %v)", layout["scroll"], layout)
	}
	if layout["wrap"] != false {
		t.Errorf("strip layout.wrap = %v, want false", layout["wrap"])
	}

	clones := vocabClones(t, doc, "strip", len(data))
	for i, clone := range clones {
		assertCanvasCloneBound(t, clone, i, data[i])
		got := boundContents(clone)
		if got["rating"] != data[i]["rating"] {
			t.Errorf("clone[%d] rating content = %v, want %v", i, got["rating"], data[i]["rating"])
		}
		assertActionsPopulated(t, clone, "carousel clone", data[i]["id"])
	}
}

// TestVocabularyPreset_ProductComparison — comparison columns fan out with
// inline price/rating/brand/description bindings and keep both line
// dividers per card.
func TestVocabularyPreset_ProductComparison(t *testing.T) {
	doc := materialiseSystemPreset(t, "product_comparison")
	data := vocabContractData()

	runCanvasContractPipeline(t, doc, data)

	compare := FindNodeByID(doc, "compare")
	if compare == nil {
		t.Fatal("compare frame missing")
	}
	layout, _ := compare["layout"].(map[string]any)
	if layout == nil || layout["scroll"] != "x" {
		t.Errorf("compare layout.scroll = %v, want \"x\"", layout["scroll"])
	}

	clones := vocabClones(t, doc, "compare", len(data))
	for i, clone := range clones {
		assertCanvasCloneBound(t, clone, i, data[i])
		got := boundContents(clone)
		for _, field := range []string{"rating", "brand", "description"} {
			if got[field] != data[i][field] {
				t.Errorf("clone[%d] %s content = %v, want %v", i, field, got[field], data[i][field])
			}
		}
		if lines := nodesOfType(clone, NodeTypeLine); len(lines) != 2 {
			t.Errorf("clone[%d] expected 2 line dividers, got %d", i, len(lines))
		}
		assertActionsPopulated(t, clone, "comparison clone", data[i]["id"])
	}
}

// TestVocabularyPreset_ProductDetailAccordion — single-entity detail (no
// replicate marker): collapsible sections keep their attrs, the summary
// stays a literal text, the body atoms bind to data[0], and the actions
// frame is filled via the single-entity fallback.
func TestVocabularyPreset_ProductDetailAccordion(t *testing.T) {
	doc := materialiseSystemPreset(t, "product_detail_accordion")
	data := vocabContractData()[:1]

	runCanvasContractPipeline(t, doc, data)

	detail := FindNodeByID(doc, "detail")
	if detail == nil {
		t.Fatal("detail frame missing")
	}
	assertCanvasCloneBound(t, detail, 0, data[0])
	got := boundContents(detail)
	if got["description"] != data[0]["description"] {
		t.Errorf("description content = %v, want %v", got["description"], data[0]["description"])
	}
	if got["brand"] != data[0]["brand"] {
		t.Errorf("brand content = %v, want %v", got["brand"], data[0]["brand"])
	}

	descSection := FindNodeByID(detail, "description-section")
	if descSection == nil {
		t.Fatal("description-section missing")
	}
	if c, _ := descSection["collapsible"].(bool); !c {
		t.Errorf("description-section collapsible = %v, want true", descSection["collapsible"])
	}
	if o, _ := descSection["defaultOpen"].(bool); !o {
		t.Errorf("description-section defaultOpen = %v, want true", descSection["defaultOpen"])
	}
	descKids := Children(descSection)
	if len(descKids) != 2 {
		t.Fatalf("description-section expected 2 children (summary+body), got %d", len(descKids))
	}
	summary := descKids[0]
	if summary[attrContent] != "Description" {
		t.Errorf("summary content = %v, want literal \"Description\"", summary[attrContent])
	}
	if fb, ok := summary[attrFieldBinding]; ok {
		t.Errorf("summary must stay literal, has fieldBinding %v", fb)
	}

	detailsSection := FindNodeByID(detail, "details-section")
	if detailsSection == nil {
		t.Fatal("details-section missing")
	}
	if c, _ := detailsSection["collapsible"].(bool); !c {
		t.Errorf("details-section collapsible = %v, want true", detailsSection["collapsible"])
	}
	if _, hasOpen := detailsSection["defaultOpen"]; hasOpen {
		t.Errorf("details-section must default closed, carries defaultOpen")
	}
	if dk := Children(detailsSection); len(dk) == 0 || dk[0][attrContent] != "Details" {
		t.Errorf("details-section first child must be the literal \"Details\" summary, got %v", dk)
	}

	assertActionsPopulated(t, detail, "accordion detail", data[0]["id"])
}

// TestCanvasTranslateContract_NewTypes — a translate-shaped card carrying
// the three new leaf types. The chain must preserve them verbatim (type,
// fills, stroke, iconFontName) while binding + action injection keep
// working around them.
func TestCanvasTranslateContract_NewTypes(t *testing.T) {
	doc := loadCanvasFixture(t, "canvas_translate_card_newtypes.json")
	data := canvasContractData()

	runCanvasContractPipeline(t, doc, data)

	clones := canvasClones(t, doc, len(data))
	for i, clone := range clones {
		assertCanvasCloneBound(t, clone, i, data[i])

		rects := nodesOfType(clone, NodeTypeRectangle)
		if len(rects) != 1 {
			t.Fatalf("clone[%d] expected 1 rectangle, got %d", i, len(rects))
		}
		fills, _ := rects[0][attrFills].([]any)
		if len(fills) != 1 || fills[0] != "#F0924A" {
			t.Errorf("clone[%d] rectangle fills = %v, want [\"#F0924A\"]", i, rects[0][attrFills])
		}
		if cr, _ := rects[0]["cornerRadius"].(float64); cr != 4 {
			t.Errorf("clone[%d] rectangle cornerRadius = %v, want 4", i, rects[0]["cornerRadius"])
		}

		lines := nodesOfType(clone, NodeTypeLine)
		if len(lines) != 1 {
			t.Fatalf("clone[%d] expected 1 line, got %d", i, len(lines))
		}
		stroke, _ := lines[0]["stroke"].(map[string]any)
		if stroke == nil || stroke["thickness"] != float64(1) || stroke["fill"] != "#e5e7eb" {
			t.Errorf("clone[%d] line stroke = %v, want {thickness:1 fill:#e5e7eb}", i, lines[0]["stroke"])
		}

		icons := nodesOfType(clone, NodeTypeIconFont)
		if len(icons) != 1 {
			t.Fatalf("clone[%d] expected 1 icon_font, got %d", i, len(icons))
		}
		if icons[0]["iconFontName"] != "truck" {
			t.Errorf("clone[%d] iconFontName = %v, want truck", i, icons[0]["iconFontName"])
		}
		if icons[0]["fill"] != "#5BA4D9" || icons[0]["width"] != float64(16) {
			t.Errorf("clone[%d] icon fill/width = %v/%v, want #5BA4D9/16", i, icons[0]["fill"], icons[0]["width"])
		}

		assertActionsPopulated(t, clone, "newtypes clone", data[i]["id"])
	}
}
