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

// TestVisualAssemblyOpsIgnored — passing ops shouldn't fail; it's a no-op
// in chunk 6b. Result still reports preset+replicate normally.
func TestVisualAssemblyOpsIgnored(t *testing.T) {
	state := newMinStatePort([]domain.Product{{ID: "p1", Name: "X"}})
	tool := NewVisualAssemblyTool(state,
		&minPresetPort{byName: map[string]*domain.Preset{"p": minimalPreset("p")}},
		&minComponentPort{},
	)
	res, err := tool.Execute(context.Background(),
		domain.ToolContext{SessionID: "sess-1", TenantSlug: "tenant-x"},
		map[string]interface{}{
			"preset":    "p",
			"replicate": 1,
			"ops": []interface{}{
				map[string]interface{}{"op": "update", "target": "title", "props": map[string]interface{}{"format": "stars"}},
			},
		},
	)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if res.IsError {
		t.Fatalf("ToolResult IsError: %s", res.Content)
	}
	if state.saved == nil {
		t.Fatalf("UpdateTemplate not called")
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
