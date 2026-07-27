package tools

// Tests for the compose_turn visual executor (§4.7 + final owner decision
// 3): ordered TurnBlocks emitted AS PRODUCED to the ctx collector, render
// blocks through the visual_assembly chain, display:"screen" writing
// current.template, up-front validation (no partial emission on bad
// input), the one-call-per-turn guard, and the replicate source/count
// dual shape. Reuses the min* port fakes from tool_visual_assembly_test.go
// (same package).

import (
	"context"
	"strings"
	"testing"

	"keepstar_v5/internal/domain"
)

func composeTool(state *minStatePort, presets map[string]*domain.Preset) *ComposeTurnTool {
	return NewComposeTurnTool(state, &minPresetPort{byName: presets}, &minComponentPort{})
}

func collectorCtx() (context.Context, *domain.TurnBlockCollector) {
	c := domain.NewTurnBlockCollector(nil)
	return domain.WithTurnBlockCollector(context.Background(), c), c
}

// TestComposeTurnOrderedBlocksAsProduced — text / render / text in one call
// → three blocks reach the collector in input order, each forwarded the
// moment it was produced (sink sees block N before block N+1 exists).
func TestComposeTurnOrderedBlocksAsProduced(t *testing.T) {
	state := newMinStatePort([]domain.Product{
		{ID: "p1", Name: "Glow Serum"},
		{ID: "p2", Name: "Hydration Mist"},
	})
	tool := composeTool(state, map[string]*domain.Preset{"product_card": minimalPreset("product_card")})

	var seen []string
	sink := domain.NewTurnBlockCollector(func(b domain.TurnBlock) { seen = append(seen, b.Kind) })
	ctx := domain.WithTurnBlockCollector(context.Background(), sink)

	res, err := tool.Execute(ctx,
		domain.ToolContext{SessionID: "sess-1", TenantSlug: "tenant-x"},
		map[string]interface{}{"blocks": []interface{}{
			map[string]interface{}{"kind": "text", "text": "Here is what I found."},
			map[string]interface{}{"kind": "render", "preset": "product_card", "replicate": "2"},
			map[string]interface{}{"kind": "text", "text": "Want to narrow it down?"},
		}},
	)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if res.IsError {
		t.Fatalf("ToolResult IsError: %s", res.Content)
	}

	blocks := sink.Blocks()
	if len(blocks) != 3 {
		t.Fatalf("collector has %d blocks, want 3: %+v", len(blocks), blocks)
	}
	wantKinds := []string{domain.BlockKindText, domain.BlockKindDocument, domain.BlockKindText}
	for i, want := range wantKinds {
		if blocks[i].Kind != want {
			t.Errorf("blocks[%d].Kind = %q, want %q", i, blocks[i].Kind, want)
		}
		if blocks[i].BlockID == "" {
			t.Errorf("blocks[%d].BlockID empty — apply targets need it", i)
		}
	}
	if blocks[0].BlockID == blocks[1].BlockID || blocks[1].BlockID == blocks[2].BlockID {
		t.Error("block ids must be unique within the turn")
	}
	if blocks[0].Text != "Here is what I found." {
		t.Errorf("blocks[0].Text = %q", blocks[0].Text)
	}

	// The render block ran the full chain: 2 replicated cards bound to the
	// two products, presetInUse stamped.
	doc := blocks[1].Document
	if doc == nil {
		t.Fatal("blocks[1].Document nil")
	}
	if got, _ := doc[domain.TemplatePresetInUseKey].(string); got != "product_card" {
		t.Errorf("presetInUse = %q, want product_card", got)
	}
	children, _ := doc["children"].([]interface{})
	if len(children) != 2 {
		t.Fatalf("document children = %d, want 2 clones", len(children))
	}
	if got := findTitleContent(t, children[0]); got != "Glow Serum" {
		t.Errorf("clone[0] title = %q, want Glow Serum", got)
	}
	if got := findTitleContent(t, children[1]); got != "Hydration Mist" {
		t.Errorf("clone[1] title = %q, want Hydration Mist", got)
	}

	// Inline display (default) must NOT touch current.template.
	if state.saved != nil {
		t.Error("UpdateTemplate called for inline-only blocks")
	}
	if blocks[1].Display != domain.DisplayInline {
		t.Errorf("render block display = %q, want inline default", blocks[1].Display)
	}

	// Streamed as produced, in order.
	if strings.Join(seen, ",") != "text,document,text" {
		t.Errorf("sink order = %v, want [text document text]", seen)
	}
}

// TestComposeTurnScreenBlockWritesTemplate — display:"screen" ALSO writes
// current.template (navigation/back-stack semantics unchanged).
func TestComposeTurnScreenBlockWritesTemplate(t *testing.T) {
	state := newMinStatePort([]domain.Product{{ID: "p1", Name: "Glow Serum"}})
	tool := composeTool(state, map[string]*domain.Preset{"product_card": minimalPreset("product_card")})
	ctx, sink := collectorCtx()

	res, err := tool.Execute(ctx,
		domain.ToolContext{SessionID: "sess-1", TenantSlug: "tenant-x"},
		map[string]interface{}{"blocks": []interface{}{
			map[string]interface{}{"kind": "render", "preset": "product_card", "display": "screen"},
		}},
	)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if res.IsError {
		t.Fatalf("ToolResult IsError: %s", res.Content)
	}
	if state.saved == nil {
		t.Fatal("UpdateTemplate NOT called for a screen block")
	}
	blocks := sink.Blocks()
	if len(blocks) != 1 || blocks[0].Display != domain.DisplayScreen {
		t.Fatalf("blocks = %+v, want one screen document block", blocks)
	}
	if got, _ := state.saved[domain.TemplatePresetInUseKey].(string); got != "product_card" {
		t.Errorf("persisted template presetInUse = %q, want product_card", got)
	}
}

// TestComposeTurnOneCallPerTurn — a second call on the same turn's
// collector is refused with IsError and emits nothing further.
func TestComposeTurnOneCallPerTurn(t *testing.T) {
	state := newMinStatePort(nil)
	tool := composeTool(state, nil)
	ctx, sink := collectorCtx()

	input := map[string]interface{}{"blocks": []interface{}{
		map[string]interface{}{"kind": "text", "text": "first"},
	}}
	if res, err := tool.Execute(ctx, domain.ToolContext{SessionID: "sess-1"}, input); err != nil || res.IsError {
		t.Fatalf("first call failed: err=%v res=%+v", err, res)
	}
	res, err := tool.Execute(ctx, domain.ToolContext{SessionID: "sess-1"}, input)
	if err != nil {
		t.Fatalf("second call Go error: %v", err)
	}
	if !res.IsError {
		t.Fatalf("second call must be IsError (one call per turn), got %+v", res)
	}
	if sink.Count() != 1 {
		t.Errorf("collector count = %d after refused second call, want 1", sink.Count())
	}
}

// TestComposeTurnValidationRejectsWholeCallBeforeEmission — bad input never
// emits partial blocks, so a failed call does not consume the turn and the
// LLM retry path stays open.
func TestComposeTurnValidationRejectsWholeCallBeforeEmission(t *testing.T) {
	state := newMinStatePort(nil)
	tool := composeTool(state, nil)

	cases := []struct {
		name  string
		input map[string]interface{}
	}{
		{"missing blocks", map[string]interface{}{}},
		{"empty blocks", map[string]interface{}{"blocks": []interface{}{}}},
		{"unknown kind", map[string]interface{}{"blocks": []interface{}{
			map[string]interface{}{"kind": "video"},
		}}},
		{"text without text", map[string]interface{}{"blocks": []interface{}{
			map[string]interface{}{"kind": "text", "text": "  "},
		}}},
		{"render without preset or ops", map[string]interface{}{"blocks": []interface{}{
			map[string]interface{}{"kind": "render"},
		}}},
		{"bad display", map[string]interface{}{"blocks": []interface{}{
			map[string]interface{}{"kind": "render", "preset": "p", "display": "modal"},
		}}},
		{"nine blocks", map[string]interface{}{"blocks": func() []interface{} {
			out := make([]interface{}, 9)
			for i := range out {
				out[i] = map[string]interface{}{"kind": "text", "text": "t"}
			}
			return out
		}()}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, sink := collectorCtx()
			res, err := tool.Execute(ctx, domain.ToolContext{SessionID: "sess-1"}, tc.input)
			if err != nil {
				t.Fatalf("Go error: %v", err)
			}
			if !res.IsError {
				t.Fatalf("want IsError, got %+v", res)
			}
			if sink.Count() != 0 {
				t.Errorf("emitted %d blocks on invalid input, want 0", sink.Count())
			}
		})
	}
}

// TestComposeTurnMissingPresetSkipsBlock — a per-block assembly failure
// (preset not found) skips that block, keeps the rest of the turn, and
// reports the failure in the summary. All-blocks-failed → IsError.
func TestComposeTurnMissingPresetSkipsBlock(t *testing.T) {
	state := newMinStatePort(nil)
	tool := composeTool(state, nil) // no presets published
	ctx, sink := collectorCtx()

	res, err := tool.Execute(ctx,
		domain.ToolContext{SessionID: "sess-1", TenantSlug: "tenant-x"},
		map[string]interface{}{"blocks": []interface{}{
			map[string]interface{}{"kind": "text", "text": "intro"},
			map[string]interface{}{"kind": "render", "preset": "no_such_preset"},
		}},
	)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if res.IsError {
		t.Fatalf("partial failure must not be IsError: %s", res.Content)
	}
	if !strings.Contains(res.Content, "failed") || !strings.Contains(res.Content, "no_such_preset") {
		t.Errorf("summary must report the skipped block, got %q", res.Content)
	}
	if sink.Count() != 1 {
		t.Errorf("collector count = %d, want 1 (text only)", sink.Count())
	}

	// All render blocks failing and nothing emitted → IsError, retry open.
	ctx2, sink2 := collectorCtx()
	res, err = tool.Execute(ctx2,
		domain.ToolContext{SessionID: "sess-1", TenantSlug: "tenant-x"},
		map[string]interface{}{"blocks": []interface{}{
			map[string]interface{}{"kind": "render", "preset": "no_such_preset"},
		}},
	)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if !res.IsError {
		t.Fatalf("zero rendered blocks must be IsError, got %+v", res)
	}
	if sink2.Count() != 0 {
		t.Errorf("collector count = %d, want 0", sink2.Count())
	}
}

// TestComposeTurnReplicateEntitySource — replicate names an entity set
// (R23 synthetic-set path): the block fans out one clone per record of
// THAT set and binds only its rows.
func TestComposeTurnReplicateEntitySource(t *testing.T) {
	state := newMinStatePort(nil)
	state.state.Current.Data.Entities = []domain.EntitySet{{
		Slug:      "opCard",
		Name:      "Operation cards",
		Synthetic: true,
		Records: []domain.EntityRecord{
			{ID: "r1", EntitySlug: "opCard", Data: map[string]any{"name": "Book a showing"}},
			{ID: "r2", EntitySlug: "opCard", Data: map[string]any{"name": "Search leads"}},
			{ID: "r3", EntitySlug: "opCard", Data: map[string]any{"name": "Notify agent"}},
		},
	}}
	tool := composeTool(state, map[string]*domain.Preset{"operation_card": minimalPreset("operation_card")})
	ctx, sink := collectorCtx()

	res, err := tool.Execute(ctx,
		domain.ToolContext{SessionID: "sess-1", TenantSlug: "tenant-x"},
		map[string]interface{}{"blocks": []interface{}{
			map[string]interface{}{"kind": "render", "preset": "operation_card", "replicate": "opCard"},
		}},
	)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if res.IsError {
		t.Fatalf("ToolResult IsError: %s", res.Content)
	}
	blocks := sink.Blocks()
	if len(blocks) != 1 {
		t.Fatalf("blocks = %d, want 1", len(blocks))
	}
	children, _ := blocks[0].Document["children"].([]interface{})
	if len(children) != 3 {
		t.Fatalf("clones = %d, want 3 (one per opCard record)", len(children))
	}
	if got := findTitleContent(t, children[0]); got != "Book a showing" {
		t.Errorf("clone[0] title = %q, want Book a showing", got)
	}
	if got := findTitleContent(t, children[2]); got != "Notify agent" {
		t.Errorf("clone[2] title = %q, want Notify agent", got)
	}
}

// TestComposeTurnNoCollectorStillComposes — no collector on ctx (direct
// registry invocation, unit harnesses): the call still validates, runs and
// summarises; it just has nowhere to stream.
func TestComposeTurnNoCollectorStillComposes(t *testing.T) {
	state := newMinStatePort(nil)
	tool := composeTool(state, nil)

	res, err := tool.Execute(context.Background(),
		domain.ToolContext{SessionID: "sess-1"},
		map[string]interface{}{"blocks": []interface{}{
			map[string]interface{}{"kind": "text", "text": "hello"},
		}},
	)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if res.IsError {
		t.Fatalf("ToolResult IsError: %s", res.Content)
	}
	if !strings.HasPrefix(res.Content, "OK blocks=1") {
		t.Errorf("summary = %q, want OK blocks=1 prefix", res.Content)
	}
}

// onboardingStatePort wraps minStatePort with the OnboardingStatePort
// extension so the deterministic uploader arming (R25) can be exercised.
type onboardingStatePort struct {
	*minStatePort
	manifest *domain.OnboardingManifest
}

func (o *onboardingStatePort) GetOnboarding(_ context.Context, _ string) (*domain.OnboardingManifest, error) {
	return o.manifest, nil
}

func (o *onboardingStatePort) UpdateOnboarding(_ context.Context, _ string, m *domain.OnboardingManifest, _ domain.DeltaInfo) (int, error) {
	o.manifest = m
	return 1, nil
}

// uploaderPreset — an uploader_card-shaped preset: a disarmed upload node
// with the seeded note.
func uploaderPreset() *domain.Preset {
	body := []byte(`{
	  "version": "2.10",
	  "children": [
	    {
	      "type": "frame", "id": "uploader",
	      "children": [
	        {"type": "upload", "id": "up-input", "name": "file",
	         "accept": [".csv", ".json"], "maxSizeMb": 20, "disarmed": true,
	         "note": "The uploader activates once you confirm the plan."}
	      ]
	    }
	  ]
	}`)
	return &domain.Preset{
		ID:           "p-up",
		TenantID:     "t-1",
		Name:         "uploader_card",
		EntityType:   "product",
		Status:       domain.PresetStatusPublished,
		DocumentJSON: body,
	}
}

// TestComposeTurnArmsUploaderFromManifestToken — R25 beat 3: once the
// manifest carries the minted issue_ingest_door token, a rendered upload
// node comes out ARMED (disarmed=false, token bound, seeded note cleared)
// WITHOUT the token ever passing through the LLM input.
func TestComposeTurnArmsUploaderFromManifestToken(t *testing.T) {
	state := &onboardingStatePort{
		minStatePort: newMinStatePort(nil),
		manifest: &domain.OnboardingManifest{Steps: []domain.ManifestStep{
			{ID: "issue_ingest_door-5", Op: "issue_ingest_door", Status: domain.ManifestStepAccepted,
				Result: map[string]any{"token": "tok-abc123"}},
		}},
	}
	tool := NewComposeTurnTool(state, &minPresetPort{byName: map[string]*domain.Preset{"uploader_card": uploaderPreset()}}, &minComponentPort{})
	ctx, sink := collectorCtx()

	res, err := tool.Execute(ctx,
		domain.ToolContext{SessionID: "sess-1", TenantSlug: "tenant-x"},
		map[string]interface{}{"blocks": []interface{}{
			map[string]interface{}{"kind": "render", "preset": "uploader_card"},
		}},
	)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if res.IsError {
		t.Fatalf("ToolResult IsError: %s", res.Content)
	}
	blocks := sink.Blocks()
	if len(blocks) != 1 || blocks[0].Document == nil {
		t.Fatalf("want 1 document block, got %+v", blocks)
	}
	up := findNodeByID(t, blocks[0].Document, "up-input")
	if got, _ := up["disarmed"].(bool); got {
		t.Error("upload node still disarmed after token minted")
	}
	if got, _ := up["token"].(string); got != "tok-abc123" {
		t.Errorf("upload token = %q, want tok-abc123", got)
	}
	if got, _ := up["note"].(string); got != "" {
		t.Errorf("seeded note not cleared: %q", got)
	}
}

// TestComposeTurnUploaderStaysDisarmedWithoutToken — beat 2: no token in
// the manifest (or no manifest at all) → the seed's disarmed shape passes
// through untouched.
func TestComposeTurnUploaderStaysDisarmedWithoutToken(t *testing.T) {
	state := &onboardingStatePort{minStatePort: newMinStatePort(nil)}
	tool := NewComposeTurnTool(state, &minPresetPort{byName: map[string]*domain.Preset{"uploader_card": uploaderPreset()}}, &minComponentPort{})
	ctx, sink := collectorCtx()

	res, err := tool.Execute(ctx,
		domain.ToolContext{SessionID: "sess-1", TenantSlug: "tenant-x"},
		map[string]interface{}{"blocks": []interface{}{
			map[string]interface{}{"kind": "render", "preset": "uploader_card"},
		}},
	)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if res.IsError {
		t.Fatalf("ToolResult IsError: %s", res.Content)
	}
	up := findNodeByID(t, sink.Blocks()[0].Document, "up-input")
	if got, _ := up["disarmed"].(bool); !got {
		t.Error("upload node armed without a minted token")
	}
	if got, _ := up["token"].(string); got != "" {
		t.Errorf("upload token = %q, want empty", got)
	}
}

// findNodeByID walks a marshaled document map for a node id.
func findNodeByID(t *testing.T, doc map[string]interface{}, id string) map[string]interface{} {
	t.Helper()
	var found map[string]interface{}
	var walk func(n map[string]interface{})
	walk = func(n map[string]interface{}) {
		if got, _ := n["id"].(string); got == id {
			found = n
			return
		}
		children, _ := n["children"].([]interface{})
		for _, c := range children {
			if m, ok := c.(map[string]interface{}); ok {
				walk(m)
			}
		}
	}
	walk(doc)
	if found == nil {
		t.Fatalf("node %q not found in document", id)
	}
	return found
}
