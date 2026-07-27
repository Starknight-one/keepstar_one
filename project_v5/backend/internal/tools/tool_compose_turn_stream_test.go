package tools

// Streaming dedupe handshake tests (lane D): an EarlyEmitter on the ctx
// has already put the leading text run on the wire mid-generation;
// compose_turn's Execute must skip exactly that prefix (no duplicates, no
// reordering), emit the rest, and enforce one-call-per-turn through the
// emitter's Claim flag. Reuses the min* port fakes from
// tool_visual_assembly_test.go (same package).

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/streaming"
)

// streamedTurnCtx wires the production trio the way agent2_execute does:
// collector + emitter on one ctx, emitter feeding collector.Emit.
func streamedTurnCtx() (context.Context, *domain.TurnBlockCollector, *streaming.EarlyEmitter) {
	c := domain.NewTurnBlockCollector(nil)
	e := streaming.NewEarlyEmitter(c.Emit)
	ctx := domain.WithTurnBlockCollector(context.Background(), c)
	ctx = streaming.WithEarlyEmitter(ctx, e)
	return ctx, c, e
}

// TestComposeTurnSkipsEarlyEmittedPrefix — the core no-duplicates
// guarantee: two text blocks streamed mid-generation + a render block
// assembled at execute time = exactly 3 blocks in the collector, in
// order, with the early BlockIDs preserved.
func TestComposeTurnSkipsEarlyEmittedPrefix(t *testing.T) {
	state := newMinStatePort([]domain.Product{{ID: "p1", Name: "Glow Serum"}})
	tool := composeTool(state, map[string]*domain.Preset{"product_card": minimalPreset("product_card")})
	ctx, sink, early := streamedTurnCtx()

	input := map[string]interface{}{"blocks": []interface{}{
		map[string]interface{}{"kind": "text", "text": "Here is the plan."},
		map[string]interface{}{"kind": "text", "text": "Two parts."},
		map[string]interface{}{"kind": "render", "preset": "product_card"},
	}}

	// Mid-generation: the adapter hook feeds the model's partial JSON.
	raw, _ := json.Marshal(input)
	half := len(raw) / 2
	early.Feed(string(raw[:half]))
	early.Feed(string(raw[half:]))
	if early.Count() != 2 {
		t.Fatalf("early Count = %d, want 2 (leading text run)", early.Count())
	}
	earlyIDs := []string{sink.Blocks()[0].BlockID, sink.Blocks()[1].BlockID}

	// Execute time: same input arrives as the parsed tool call.
	res, err := tool.Execute(ctx, domain.ToolContext{SessionID: "sess-1", TenantSlug: "tenant-x"}, input)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if res.IsError {
		t.Fatalf("ToolResult IsError: %s", res.Content)
	}

	blocks := sink.Blocks()
	if len(blocks) != 3 {
		t.Fatalf("collector has %d blocks, want 3 (2 early + 1 assembled): %+v", len(blocks), blocks)
	}
	if blocks[0].Text != "Here is the plan." || blocks[1].Text != "Two parts." {
		t.Errorf("early prefix reordered/duplicated: %+v", blocks[:2])
	}
	if blocks[0].BlockID != earlyIDs[0] || blocks[1].BlockID != earlyIDs[1] {
		t.Error("early BlockIDs must survive execute (terminal frame settles on them)")
	}
	if blocks[2].Kind != domain.BlockKindDocument || blocks[2].Document == nil {
		t.Fatalf("blocks[2] = %+v, want the assembled document", blocks[2])
	}
	// Summary counts include the early prefix — the turn DID show 3 blocks.
	if !strings.Contains(res.Content, "blocks=3") || !strings.Contains(res.Content, "text=2") {
		t.Errorf("summary %q, want blocks=3 text=2", res.Content)
	}
	if !early.Claimed() {
		t.Error("successful compose must claim the turn")
	}
}

// TestComposeTurnSecondCallRefusedAfterClaim — one call per turn holds
// with the emitter guard, including the pure-text edge where execute
// emits nothing new (everything was early-emitted).
func TestComposeTurnSecondCallRefusedAfterClaim(t *testing.T) {
	state := newMinStatePort(nil)
	tool := composeTool(state, nil)
	ctx, sink, early := streamedTurnCtx()

	input := map[string]interface{}{"blocks": []interface{}{
		map[string]interface{}{"kind": "text", "text": "only text"},
	}}
	raw, _ := json.Marshal(input)
	early.Feed(string(raw))

	res, err := tool.Execute(ctx, domain.ToolContext{SessionID: "s", TenantSlug: "t"}, input)
	if err != nil || res.IsError {
		t.Fatalf("first call failed: err=%v res=%+v", err, res)
	}
	if sink.Count() != 1 {
		t.Fatalf("collector count = %d, want 1 (no duplicate of the early block)", sink.Count())
	}

	res2, err := tool.Execute(ctx, domain.ToolContext{SessionID: "s", TenantSlug: "t"}, input)
	if err != nil {
		t.Fatalf("second call transport error: %v", err)
	}
	if !res2.IsError || !strings.Contains(res2.Content, "one call per turn") {
		t.Fatalf("second call = %+v, want one-call-per-turn refusal", res2)
	}
	if sink.Count() != 1 {
		t.Errorf("refused call emitted blocks: count = %d", sink.Count())
	}
}

// TestComposeTurnFailedValidationWithNoEarlyBlocksKeepsRetryOpen — the
// existing retry contract: a call rejected before anything reached the
// wire does not consume the turn, emitter present or not.
func TestComposeTurnFailedValidationWithNoEarlyBlocksKeepsRetryOpen(t *testing.T) {
	state := newMinStatePort(nil)
	tool := composeTool(state, nil)
	ctx, sink, early := streamedTurnCtx()

	bad := map[string]interface{}{"blocks": []interface{}{
		map[string]interface{}{"kind": "text", "text": "   "},
	}}
	res, err := tool.Execute(ctx, domain.ToolContext{SessionID: "s", TenantSlug: "t"}, bad)
	if err != nil || !res.IsError {
		t.Fatalf("want IsError on invalid input, got err=%v res=%+v", err, res)
	}
	if early.Claimed() {
		t.Fatal("failed validation with nothing emitted must NOT claim the turn")
	}
	if sink.Count() != 0 {
		t.Fatalf("collector count = %d, want 0", sink.Count())
	}

	good := map[string]interface{}{"blocks": []interface{}{
		map[string]interface{}{"kind": "text", "text": "retry works"},
	}}
	res2, err := tool.Execute(ctx, domain.ToolContext{SessionID: "s", TenantSlug: "t"}, good)
	if err != nil || res2.IsError {
		t.Fatalf("retry refused: err=%v res=%+v", err, res2)
	}
	if sink.Count() != 1 {
		t.Errorf("retry emitted %d blocks, want 1", sink.Count())
	}
}

// TestComposeTurnEarlyPrefixOnWireClaimsEvenOnLaterFailure — once the
// early prefix reached the wire, a later validation failure still burns
// the turn (a sibling call must not compose on top of half-streamed
// output); the streamed prefix stands in the collector for the terminal
// frame.
func TestComposeTurnEarlyPrefixOnWireClaimsEvenOnLaterFailure(t *testing.T) {
	state := newMinStatePort(nil)
	tool := composeTool(state, nil)
	ctx, sink, early := streamedTurnCtx()

	// Model streamed a good text block, then an invalid one.
	badInput := map[string]interface{}{"blocks": []interface{}{
		map[string]interface{}{"kind": "text", "text": "good lead-in"},
		map[string]interface{}{"kind": "render"}, // no preset, no ops → invalid
	}}
	raw, _ := json.Marshal(badInput)
	early.Feed(string(raw))
	if early.Count() != 1 {
		t.Fatalf("early Count = %d, want 1", early.Count())
	}

	res, err := tool.Execute(ctx, domain.ToolContext{SessionID: "s", TenantSlug: "t"}, badInput)
	if err != nil || !res.IsError {
		t.Fatalf("want IsError, got err=%v res=%+v", err, res)
	}
	if !early.Claimed() {
		t.Fatal("turn with streamed output must be claimed even when validation fails")
	}
	if sink.Count() != 1 {
		t.Errorf("collector count = %d, want the streamed prefix intact", sink.Count())
	}

	// A sibling compose call in the same turn is refused.
	res2, _ := tool.Execute(ctx, domain.ToolContext{SessionID: "s", TenantSlug: "t"},
		map[string]interface{}{"blocks": []interface{}{
			map[string]interface{}{"kind": "text", "text": "sibling"},
		}})
	if !res2.IsError || !strings.Contains(res2.Content, "one call per turn") {
		t.Fatalf("sibling call = %+v, want refusal", res2)
	}
}

// TestComposeTurnHookLessTurnUnchanged — no emitter on the ctx: the
// legacy collector-count guard and full execute-time emission behave
// exactly as before (storefront and fake-LLM paths).
func TestComposeTurnHookLessTurnUnchanged(t *testing.T) {
	state := newMinStatePort(nil)
	tool := composeTool(state, nil)
	ctx, sink := collectorCtx()

	input := map[string]interface{}{"blocks": []interface{}{
		map[string]interface{}{"kind": "text", "text": "plain path"},
	}}
	res, err := tool.Execute(ctx, domain.ToolContext{SessionID: "s", TenantSlug: "t"}, input)
	if err != nil || res.IsError {
		t.Fatalf("hook-less call failed: err=%v res=%+v", err, res)
	}
	if sink.Count() != 1 {
		t.Fatalf("collector count = %d, want 1", sink.Count())
	}
	res2, _ := tool.Execute(ctx, domain.ToolContext{SessionID: "s", TenantSlug: "t"}, input)
	if !res2.IsError {
		t.Fatal("legacy one-call-per-turn guard broken")
	}
}
