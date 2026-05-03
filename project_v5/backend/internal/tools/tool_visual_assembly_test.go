package tools

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"keepstar_v5/internal/domain"
)

// minStatePort is a tiny in-memory stand-in for ports.StatePort. We only
// implement the methods VisualAssemblyTool calls (GetState, UpdateTemplate);
// every other method panics if invoked, which catches accidental scope
// creep in the tool implementation.
type minStatePort struct {
	state *domain.SessionState
	saved map[string]interface{}
}

func newMinStatePort(products []domain.Product) *minStatePort {
	return &minStatePort{
		state: &domain.SessionState{
			SessionID: "sess-1",
			Current:   domain.StateCurrent{Data: domain.StateData{Products: products}},
		},
	}
}

func (m *minStatePort) GetState(_ context.Context, _ string) (*domain.SessionState, error) {
	cp := *m.state
	return &cp, nil
}
func (m *minStatePort) UpdateTemplate(_ context.Context, _ string, tpl map[string]interface{}, _ domain.DeltaInfo) (int, error) {
	m.saved = tpl
	m.state.Current.Template = tpl
	return 1, nil
}

// Stubs — visual_assembly should NEVER call these. Any invocation is a
// regression in the tool's contract.
func (m *minStatePort) CreateState(context.Context, string) (*domain.SessionState, error) {
	panic("not used")
}
func (m *minStatePort) UpdateState(context.Context, *domain.SessionState) error { panic("not used") }
func (m *minStatePort) AddDelta(context.Context, string, *domain.Delta) (int, error) {
	panic("not used")
}
func (m *minStatePort) GetDeltas(context.Context, string) ([]domain.Delta, error) {
	panic("not used")
}
func (m *minStatePort) GetDeltasSince(context.Context, string, int) ([]domain.Delta, error) {
	panic("not used")
}
func (m *minStatePort) GetDeltasUntil(context.Context, string, int) ([]domain.Delta, error) {
	panic("not used")
}
func (m *minStatePort) UpdateData(context.Context, string, domain.StateData, domain.StateMeta, domain.DeltaInfo) (int, error) {
	panic("not used")
}
func (m *minStatePort) UpdateView(context.Context, string, domain.ViewState, []domain.ViewSnapshot, domain.DeltaInfo) (int, error) {
	panic("not used")
}
func (m *minStatePort) UpdateActions(context.Context, string, domain.StateActions, domain.DeltaInfo) (int, error) {
	panic("not used")
}
func (m *minStatePort) AppendConversation(context.Context, string, []domain.LLMMessage) error {
	panic("not used")
}
func (m *minStatePort) AppendAgent2History(context.Context, string, []domain.LLMMessage) error {
	panic("not used")
}
func (m *minStatePort) PushView(context.Context, string, *domain.ViewSnapshot) error {
	panic("not used")
}
func (m *minStatePort) PopView(context.Context, string) (*domain.ViewSnapshot, error) {
	panic("not used")
}
func (m *minStatePort) GetViewStack(context.Context, string) ([]domain.ViewSnapshot, error) {
	panic("not used")
}

// minPresetPort serves a single preset by name; everything else returns
// ErrPresetNotFound. Used to test happy path + missing preset.
type minPresetPort struct {
	byName map[string]*domain.Preset
}

func (p *minPresetPort) GetPublishedPreset(_ context.Context, _ string, name string) (*domain.Preset, error) {
	if pr, ok := p.byName[name]; ok {
		return pr, nil
	}
	return nil, domain.ErrPresetNotFound
}
func (p *minPresetPort) ListPublishedPresets(context.Context, string) ([]domain.Preset, error) {
	panic("not used")
}

// minComponentPort returns a fixed list. Empty list is fine — Materialise
// handles it.
type minComponentPort struct {
	items []domain.Component
}

func (c *minComponentPort) GetPublishedComponent(context.Context, string, string) (*domain.Component, error) {
	panic("not used")
}
func (c *minComponentPort) ListPublishedComponents(context.Context, string) ([]domain.Component, error) {
	cp := make([]domain.Component, len(c.items))
	copy(cp, c.items)
	return cp, nil
}

// minimalPreset builds a barely-valid Preset with a one-frame Document
// containing a text node bound to "name". Replicate marker on the frame.
func minimalPreset(name string) *domain.Preset {
	body := []byte(`{
	  "version": "2.10",
	  "children": [
	    {
	      "type": "frame", "id": "card", "replicate": true,
	      "children": [
	        {"type": "text", "id": "title", "fieldBinding": "name"}
	      ]
	    }
	  ]
	}`)
	return &domain.Preset{
		ID:           "p-1",
		TenantID:     "t-1",
		Name:         name,
		EntityType:   "product",
		Status:       domain.PresetStatusPublished,
		DocumentJSON: body,
	}
}

// TestVisualAssemblyHappyPath — preset + replicate=2 + 2 products → tool
// runs the full pipeline and writes a Document with 2 cloned cards bound
// to the products.
func TestVisualAssemblyHappyPath(t *testing.T) {
	state := newMinStatePort([]domain.Product{
		{ID: "p1", Name: "Glow Serum"},
		{ID: "p2", Name: "Hydration Mist"},
	})
	tool := NewVisualAssemblyTool(state,
		&minPresetPort{byName: map[string]*domain.Preset{"product_card": minimalPreset("product_card")}},
		&minComponentPort{},
	)

	res, err := tool.Execute(context.Background(),
		domain.ToolContext{SessionID: "sess-1", TenantSlug: "tenant-x"},
		map[string]interface{}{"preset": "product_card", "replicate": 2},
	)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if res.IsError {
		t.Fatalf("ToolResult IsError: %s", res.Content)
	}
	if state.saved == nil {
		t.Fatalf("UpdateTemplate was not called")
	}

	// Document round-trip: should have 2 cloned card subtrees at top level
	// (no reusable defs since we passed no components).
	rawChildren, _ := state.saved["children"].([]interface{})
	if len(rawChildren) != 2 {
		t.Fatalf("expected 2 top-level children (cloned cards), got %d", len(rawChildren))
	}
	// First clone → product[0].name = "Glow Serum"
	titleContent := findTitleContent(t, rawChildren[0])
	if titleContent != "Glow Serum" {
		t.Errorf("clone[0] title = %q, want Glow Serum", titleContent)
	}
	titleContent = findTitleContent(t, rawChildren[1])
	if titleContent != "Hydration Mist" {
		t.Errorf("clone[1] title = %q, want Hydration Mist", titleContent)
	}
}

// TestVisualAssemblyMissingPreset — invalid preset name → ToolResult with
// IsError=true (NOT a Go error). The model sees the failure and can retry.
func TestVisualAssemblyMissingPreset(t *testing.T) {
	state := newMinStatePort(nil)
	tool := NewVisualAssemblyTool(state,
		&minPresetPort{byName: map[string]*domain.Preset{}},
		&minComponentPort{},
	)
	res, err := tool.Execute(context.Background(),
		domain.ToolContext{SessionID: "sess-1", TenantSlug: "tenant-x"},
		map[string]interface{}{"preset": "no_such_preset"},
	)
	if err != nil {
		t.Fatalf("expected ToolResult, got Go error: %v", err)
	}
	if !res.IsError {
		t.Errorf("expected IsError=true, got %+v", res)
	}
	if state.saved != nil {
		t.Errorf("UpdateTemplate must NOT be called on lookup failure")
	}
}

// TestVisualAssemblyMissingPresetField — the tool requires a string preset
// field. Missing → ToolResult with IsError=true.
func TestVisualAssemblyMissingPresetField(t *testing.T) {
	state := newMinStatePort(nil)
	tool := NewVisualAssemblyTool(state,
		&minPresetPort{byName: map[string]*domain.Preset{}},
		&minComponentPort{},
	)
	res, err := tool.Execute(context.Background(),
		domain.ToolContext{SessionID: "sess-1", TenantSlug: "tenant-x"},
		map[string]interface{}{},
	)
	if err != nil {
		t.Fatalf("expected ToolResult, got Go error: %v", err)
	}
	if !res.IsError {
		t.Errorf("expected IsError=true on missing preset, got %+v", res)
	}
}

// TestVisualAssemblyOpsApplied — ops on the un-replicated tree land on
// every replicate clone. Verifies the chunk-6c integration: ApplyOps runs
// before ExpandReplicates, so an `update` on the title atom propagates to
// each cloned card.
func TestVisualAssemblyOpsApplied(t *testing.T) {
	state := newMinStatePort([]domain.Product{
		{ID: "p1", Name: "Glow Serum"},
		{ID: "p2", Name: "Hydration Mist"},
	})
	tool := NewVisualAssemblyTool(state,
		&minPresetPort{byName: map[string]*domain.Preset{"p": minimalPreset("p")}},
		&minComponentPort{},
	)
	res, err := tool.Execute(context.Background(),
		domain.ToolContext{SessionID: "sess-1", TenantSlug: "tenant-x"},
		map[string]interface{}{
			"preset":    "p",
			"replicate": 2,
			// Update the title atom's `format` prop. The op runs against
			// the un-replicated tree, so both clones inherit the change.
			"ops": []interface{}{
				map[string]interface{}{
					"op":     "update",
					"target": "title",
					"props":  map[string]interface{}{"format": "stars-compact"},
				},
			},
		},
	)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if res.IsError {
		t.Fatalf("ToolResult IsError: %s", res.Content)
	}

	// Both clones must carry the updated `format` AND the bound content.
	rawChildren, _ := state.saved["children"].([]interface{})
	if len(rawChildren) != 2 {
		t.Fatalf("expected 2 cloned cards, got %d", len(rawChildren))
	}
	for i, child := range rawChildren {
		fmt := findFieldNamedFormat(child)
		if fmt != "stars-compact" {
			t.Errorf("clone[%d] format = %q, want stars-compact (op didn't propagate)", i, fmt)
		}
	}
}

// findFieldNamedFormat walks a node tree post-replicate, returns the
// `format` attribute on the title (fieldBinding=="name") node. Used by
// the ops-applied test so we can assert per-clone state.
func findFieldNamedFormat(node interface{}) string {
	m, ok := node.(map[string]interface{})
	if !ok {
		return ""
	}
	if fb, _ := m["fieldBinding"].(string); fb == "name" {
		s, _ := m["format"].(string)
		return s
	}
	if children, ok := m["children"].([]interface{}); ok {
		for _, c := range children {
			if got := findFieldNamedFormat(c); got != "" {
				return got
			}
		}
	}
	return ""
}

// TestVisualAssemblyOpsBadShape — ops with malformed shape fails as a
// ToolResult IsError, not a Go error.
func TestVisualAssemblyOpsBadShape(t *testing.T) {
	state := newMinStatePort(nil)
	tool := NewVisualAssemblyTool(state,
		&minPresetPort{byName: map[string]*domain.Preset{"p": minimalPreset("p")}},
		&minComponentPort{},
	)
	res, err := tool.Execute(context.Background(),
		domain.ToolContext{SessionID: "sess-1", TenantSlug: "tenant-x"},
		map[string]interface{}{
			"preset": "p",
			"ops":    "not-an-array",
		},
	)
	if err != nil {
		t.Fatalf("expected ToolResult, got Go error: %v", err)
	}
	if !res.IsError {
		t.Errorf("expected IsError on bad ops shape, got %+v", res)
	}
}

// TestVisualAssemblyDefinitionStable — the schema must not drift across
// runs of GetDefinitions, otherwise the prompt cache busts. Specifically:
// JSON-marshalling the InputSchema twice produces identical bytes.
func TestVisualAssemblyDefinitionStable(t *testing.T) {
	tool := NewVisualAssemblyTool(nil, nil, nil)
	def := tool.Definition()
	if def.Name != "visual_assembly" {
		t.Errorf("Name = %q, want visual_assembly", def.Name)
	}
	first, _ := json.Marshal(def.InputSchema)
	second, _ := json.Marshal(tool.Definition().InputSchema)
	if string(first) != string(second) {
		t.Errorf("schema marshals differ across calls; cache will bust\n#1=%s\n#2=%s", first, second)
	}
}

// findTitleContent walks a node tree (post-replicate, so ids are fresh)
// looking for the node carrying fieldBinding=="name" and returns its
// content. Matching by binding is stable; matching by id would not be
// since reIDSubtree mints new ids per replicate clone.
func findTitleContent(t *testing.T, node interface{}) string {
	t.Helper()
	m, ok := node.(map[string]interface{})
	if !ok {
		return ""
	}
	if fb, _ := m["fieldBinding"].(string); fb == "name" {
		c, _ := m["content"].(string)
		return c
	}
	if children, ok := m["children"].([]interface{}); ok {
		for _, c := range children {
			if got := findTitleContent(t, c); got != "" {
				return got
			}
		}
	}
	return ""
}

// silence unused-import warnings
var _ = errors.Is

// TestVisualAssemblyFreestyle — preset omitted, ops only: tool starts
// from an empty Document and builds the tree from ops alone. Mirrors
// V4's "BUILDING FROM SCRATCH" path.
func TestVisualAssemblyFreestyle(t *testing.T) {
	state := newMinStatePort([]domain.Product{
		{ID: "p1", Name: "Glow Serum"},
	})
	tool := NewVisualAssemblyTool(state,
		&minPresetPort{byName: map[string]*domain.Preset{}}, // no preset available
		&minComponentPort{},
	)
	res, err := tool.Execute(context.Background(),
		domain.ToolContext{SessionID: "sess-1", TenantSlug: "tenant-x"},
		map[string]interface{}{
			"ops": []interface{}{
				map[string]interface{}{
					"op":     "insert",
					"parent": "formation",
					"props":  map[string]interface{}{"type": "frame", "id": "card", "replicate": true},
				},
				map[string]interface{}{
					"op":     "insert",
					"parent": "card",
					"props":  map[string]interface{}{"type": "text", "id": "title", "fieldBinding": "name"},
				},
			},
			"replicate": 1,
		},
	)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.IsError {
		t.Fatalf("ToolResult IsError: %s", res.Content)
	}
	rawChildren, _ := state.saved["children"].([]interface{})
	if len(rawChildren) != 1 {
		t.Fatalf("expected 1 root child, got %d", len(rawChildren))
	}
	if findTitleContent(t, rawChildren[0]) != "Glow Serum" {
		t.Errorf("freestyle did not bind product[0].name")
	}
}

// TestVisualAssemblyMultiWidgetCompose — preset omitted, ops insert
// MULTIPLE top-level frames (hero literal + replicated gallery + cta
// literal). Mirrors V4's COMPOSING path. Asserts ≥3 root children and
// at least one with replicate:true (the gallery).
func TestVisualAssemblyMultiWidgetCompose(t *testing.T) {
	state := newMinStatePort([]domain.Product{
		{ID: "p1", Name: "Glow Serum"},
		{ID: "p2", Name: "Hydration Mist"},
	})
	tool := NewVisualAssemblyTool(state,
		&minPresetPort{byName: map[string]*domain.Preset{}},
		&minComponentPort{},
	)
	res, err := tool.Execute(context.Background(),
		domain.ToolContext{SessionID: "sess-1", TenantSlug: "tenant-x"},
		map[string]interface{}{
			"ops": []interface{}{
				// hero literal
				map[string]interface{}{
					"op":     "insert",
					"parent": "root",
					"props":  map[string]interface{}{"type": "frame", "id": "hero"},
				},
				map[string]interface{}{
					"op":     "insert",
					"parent": "hero",
					"props":  map[string]interface{}{"type": "text", "id": "hero-text", "content": "New collection"},
				},
				// replicated gallery
				map[string]interface{}{
					"op":     "insert",
					"parent": "",
					"props":  map[string]interface{}{"type": "frame", "id": "gallery", "replicate": true},
				},
				map[string]interface{}{
					"op":     "insert",
					"parent": "gallery",
					"props":  map[string]interface{}{"type": "text", "id": "g-title", "fieldBinding": "name"},
				},
				// cta literal
				map[string]interface{}{
					"op":     "insert",
					"parent": "formation",
					"props":  map[string]interface{}{"type": "frame", "id": "cta"},
				},
				map[string]interface{}{
					"op":     "insert",
					"parent": "cta",
					"props":  map[string]interface{}{"type": "text", "id": "cta-text", "content": "Shop now", "wrapper": "button"},
				},
			},
			"replicate": 2,
		},
	)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.IsError {
		t.Fatalf("ToolResult IsError: %s", res.Content)
	}
	rawChildren, _ := state.saved["children"].([]interface{})
	// Expect: hero literal + 2 replicated gallery clones + cta literal = 4.
	// (Gallery's replicate:true expands to 2 since we have 2 products.)
	if len(rawChildren) < 3 {
		t.Fatalf("expected ≥3 root children for compose, got %d", len(rawChildren))
	}
	// At least one root child must have content "New collection" (hero).
	foundHero := false
	for _, c := range rawChildren {
		if findContentString(c, "New collection") {
			foundHero = true
			break
		}
	}
	if !foundHero {
		t.Errorf("hero literal not found among root children")
	}
}

// TestVisualAssemblyBothAbsentErrors — neither preset nor ops → IsError
// (cheaper to surface to LLM than silently render an empty Document).
func TestVisualAssemblyBothAbsentErrors(t *testing.T) {
	state := newMinStatePort(nil)
	tool := NewVisualAssemblyTool(state,
		&minPresetPort{byName: map[string]*domain.Preset{}},
		&minComponentPort{},
	)
	res, err := tool.Execute(context.Background(),
		domain.ToolContext{SessionID: "sess-1", TenantSlug: "tenant-x"},
		map[string]interface{}{},
	)
	if err != nil {
		t.Fatalf("expected ToolResult, got Go error: %v", err)
	}
	if !res.IsError {
		t.Errorf("expected IsError when both preset and ops absent, got %+v", res)
	}
}

// findContentString walks a node tree and returns true if any text node's
// content matches the needle.
func findContentString(node interface{}, needle string) bool {
	m, ok := node.(map[string]interface{})
	if !ok {
		return false
	}
	if c, _ := m["content"].(string); c == needle {
		return true
	}
	if children, ok := m["children"].([]interface{}); ok {
		for _, c := range children {
			if findContentString(c, needle) {
				return true
			}
		}
	}
	return false
}
