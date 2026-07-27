package handlers

// Tests for the streamed turn protocol on the wire (§4.7 + final owner
// decision 3): /pipeline/stream emits one `event: block` frame per
// TurnBlock AS PRODUCED (between the stage frames and the terminal
// result), the terminal `result` frame carries the full blocks[] array
// next to the back-compat document, and plain POST /pipeline gains
// `blocks` — omitted entirely on legacy single-document turns. Reuses the
// SSE helpers from handler_pipeline_stream_test.go (same package).

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/usecases"
)

// fakeBlocksRunner mirrors the real orchestrator's turn shape: OnStage
// once after Agent1, then each block through OnBlock AS PRODUCED, then the
// response with the aggregated blocks array.
type fakeBlocksRunner struct {
	stage  usecases.StageEvent
	blocks []domain.TurnBlock
	resp   *usecases.PipelineExecuteResponse
}

func (f *fakeBlocksRunner) Execute(_ context.Context, req usecases.PipelineExecuteRequest) (*usecases.PipelineExecuteResponse, error) {
	if req.OnStage != nil {
		req.OnStage(f.stage)
	}
	for _, b := range f.blocks {
		if req.OnBlock != nil {
			req.OnBlock(b)
		}
	}
	resp := *f.resp
	resp.Blocks = f.blocks
	return &resp, nil
}

func cannedBlocks() []domain.TurnBlock {
	return []domain.TurnBlock{
		{BlockID: "blk_aa", Kind: domain.BlockKindText, Text: "Here is your uploader."},
		{BlockID: "blk_bb", Kind: domain.BlockKindDocument, Display: domain.DisplayInline,
			Document: map[string]interface{}{"version": "2.10", "__presetInUse": "uploader_card"}},
		{BlockID: "blk_cc", Kind: domain.BlockKindText, Text: "Accept the defaults?"},
	}
}

func TestPipelineStreamEmitsBlockFrames(t *testing.T) {
	resp := cannedResponse()
	resp.Manifest = &usecases.ManifestStatusSummary{Staged: 2, Applied: 1, Total: 3}
	h := &PipelineHandler{
		pipeline: &fakeBlocksRunner{stage: cannedStage, blocks: cannedBlocks(), resp: resp},
		log:      discardLog(),
	}

	rec := httptest.NewRecorder()
	h.PipelineStream(rec, streamRequest())
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}

	frames := parseFrames(t, rec.Body.String())
	// data_start, data_done, 3× block, result.
	if len(frames) != 6 {
		t.Fatalf("expected 6 frames, got %d: %+v", len(frames), frames)
	}
	wantEvents := []string{"stage", "stage", "block", "block", "block", "result"}
	for i, want := range wantEvents {
		if frames[i].event != want {
			t.Errorf("frame[%d].event = %q, want %q", i, frames[i].event, want)
		}
	}

	// Block frames are TurnBlock JSON, in emission order.
	var b1 domain.TurnBlock
	if err := json.Unmarshal([]byte(frames[2].data), &b1); err != nil {
		t.Fatalf("block frame not TurnBlock JSON: %v (%q)", err, frames[2].data)
	}
	if b1.BlockID != "blk_aa" || b1.Kind != domain.BlockKindText || b1.Text != "Here is your uploader." {
		t.Errorf("block[0] = %+v, want blk_aa text frame", b1)
	}
	var b2 domain.TurnBlock
	if err := json.Unmarshal([]byte(frames[3].data), &b2); err != nil {
		t.Fatalf("block frame not TurnBlock JSON: %v", err)
	}
	if b2.Kind != domain.BlockKindDocument || b2.Display != domain.DisplayInline || b2.Document == nil {
		t.Errorf("block[1] = %+v, want inline document frame", b2)
	}

	// Terminal result: full blocks array + back-compat document + the
	// manifest status summary (must match POST /pipeline — the widget's
	// contextual chips read it off the stream's result frame too).
	var result struct {
		Document map[string]interface{}          `json:"document"`
		Blocks   []domain.TurnBlock              `json:"blocks"`
		Manifest *usecases.ManifestStatusSummary `json:"manifest"`
	}
	if err := json.Unmarshal([]byte(frames[5].data), &result); err != nil {
		t.Fatalf("result data not JSON: %v", err)
	}
	if len(result.Blocks) != 3 {
		t.Fatalf("result.blocks len = %d, want 3", len(result.Blocks))
	}
	if result.Blocks[0].BlockID != "blk_aa" || result.Blocks[2].BlockID != "blk_cc" {
		t.Errorf("result.blocks order = %+v, want [blk_aa blk_bb blk_cc]", result.Blocks)
	}
	if v, _ := result.Document["version"].(string); v != "2.10" {
		t.Errorf("back-compat document.version = %q, want 2.10", v)
	}
	if result.Manifest == nil || result.Manifest.Staged != 2 || result.Manifest.Applied != 1 {
		t.Errorf("result.manifest = %+v, want {staged:2 applied:1 total:3}", result.Manifest)
	}
}

func TestPipelinePostJSONCarriesBlocks(t *testing.T) {
	h := &PipelineHandler{
		pipeline: &fakeBlocksRunner{stage: cannedStage, blocks: cannedBlocks(), resp: cannedResponse()},
		log:      discardLog(),
	}

	rec := httptest.NewRecorder()
	h.Pipeline(rec, streamRequest())
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}

	var out struct {
		Document map[string]interface{} `json:"document"`
		Blocks   []domain.TurnBlock     `json:"blocks"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("response not JSON: %v", err)
	}
	if len(out.Blocks) != 3 {
		t.Fatalf("blocks len = %d, want 3", len(out.Blocks))
	}
	if out.Blocks[1].Kind != domain.BlockKindDocument || out.Blocks[1].Document == nil {
		t.Errorf("blocks[1] = %+v, want document block", out.Blocks[1])
	}
	if v, _ := out.Document["version"].(string); v != "2.10" {
		t.Errorf("document.version = %q, want 2.10", v)
	}
}

// TestPipelineLegacyTurnOmitsBlocks — a turn without compose_turn (today's
// storefront) must not grow a blocks key at all: absent blocks = legacy
// single-document turn, old bundles unaffected.
func TestPipelineLegacyTurnOmitsBlocks(t *testing.T) {
	stage := cannedStage
	h := &PipelineHandler{
		pipeline: &fakePipelineRunner{stage: &stage, resp: cannedResponse()},
		log:      discardLog(),
	}

	rec := httptest.NewRecorder()
	h.Pipeline(rec, streamRequest())
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("response not JSON: %v", err)
	}
	if _, present := raw["blocks"]; present {
		t.Error("legacy turn response contains a blocks key — must be omitted when empty")
	}

	// Same on the stream: no block frames, no blocks key in the result.
	rec = httptest.NewRecorder()
	h.PipelineStream(rec, streamRequest())
	frames := parseFrames(t, rec.Body.String())
	for _, f := range frames {
		if f.event == "block" {
			t.Fatalf("legacy turn emitted a block frame: %+v", f)
		}
	}
	var resultRaw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(frames[len(frames)-1].data), &resultRaw); err != nil {
		t.Fatalf("result data not JSON: %v", err)
	}
	if _, present := resultRaw["blocks"]; present {
		t.Error("legacy stream result contains a blocks key — must be omitted when empty")
	}
}

// TestOnboardingCapResponseShape — the §6.3 turn-cap answer is a graceful
// 200 turn: one inline text block (blocks-aware shells render it) plus a
// non-nil document (legacy renderers see an empty doc, never a crash or an
// error status). The mode-wiring itself (session row → gate) is exercised
// by the live e2e; this pins the wire shape.
func TestOnboardingCapResponseShape(t *testing.T) {
	resp := onboardingCapResponse()
	if resp.Document == nil {
		t.Error("cap response document nil — legacy renderers need an (empty) object")
	}
	if len(resp.Blocks) != 1 {
		t.Fatalf("cap response blocks = %d, want 1", len(resp.Blocks))
	}
	b := resp.Blocks[0]
	if b.Kind != domain.BlockKindText || b.Text == "" || b.Display != domain.DisplayInline {
		t.Errorf("cap block = %+v, want inline text", b)
	}
	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var wire map[string]any
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := wire["blocks"]; !ok {
		t.Error("wire body missing blocks key")
	}
}
