package handlers

// Unit tests for POST /api/v1/pipeline/stream — SSE frame order and wire
// shape, incremental flushing through the WithLogging middleware chain
// (the recordingWriter.Unwrap fix), mid-stream error frames, and the
// pre-stream guard rejection staying plain JSON.

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/usecases"
)

// --- fakes -----------------------------------------------------------------

// fakePipelineRunner is a canned pipelineRunner. When stage is set, Execute
// invokes req.OnStage with it; when block is set, Execute then parks on the
// channel before returning (lets tests assert frames arrive incrementally).
type fakePipelineRunner struct {
	stage *usecases.StageEvent
	resp  *usecases.PipelineExecuteResponse
	err   error
	block chan struct{}
}

func (f *fakePipelineRunner) Execute(_ context.Context, req usecases.PipelineExecuteRequest) (*usecases.PipelineExecuteResponse, error) {
	if f.stage != nil && req.OnStage != nil {
		req.OnStage(*f.stage)
	}
	if f.block != nil {
		<-f.block
	}
	if f.err != nil {
		return nil, f.err
	}
	return f.resp, nil
}

// --- harness ---------------------------------------------------------------

func discardLog() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// cannedStage mirrors the spec's wire example for the data_done frame.
var cannedStage = usecases.StageEvent{
	Phase:    "data_done",
	Signal:   "new_search: 12 items found",
	ToolName: "catalog_search",
	Count:    12,
	Agent1Ms: 2031,
}

func cannedResponse() *usecases.PipelineExecuteResponse {
	return &usecases.PipelineExecuteResponse{
		Document:  map[string]interface{}{"version": "2.10"},
		LatencyMs: 4321,
		Agent1Ms:  2031,
		Agent2Ms:  2290,
	}
}

// streamRequest builds a valid /pipeline/stream request with the tenant
// pre-stamped (direct handler calls skip the WithTenant middleware).
func streamRequest() *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/pipeline/stream",
		strings.NewReader(`{"sessionId":"s1","query":"show me serums"}`))
	ctx := context.WithValue(req.Context(), ctxKeyTenant{}, &domain.Tenant{ID: "tnt-1", Slug: "acme"})
	return req.WithContext(ctx)
}

type sseFrame struct {
	event string
	data  string
}

// parseFrames splits a complete SSE body into (event, data) frames.
func parseFrames(t *testing.T, body string) []sseFrame {
	t.Helper()
	var frames []sseFrame
	for _, raw := range strings.Split(strings.TrimRight(body, "\n"), "\n\n") {
		if raw == "" {
			continue
		}
		lines := strings.Split(raw, "\n")
		if len(lines) != 2 || !strings.HasPrefix(lines[0], "event: ") || !strings.HasPrefix(lines[1], "data: ") {
			t.Fatalf("malformed SSE frame: %q", raw)
		}
		frames = append(frames, sseFrame{
			event: strings.TrimPrefix(lines[0], "event: "),
			data:  strings.TrimPrefix(lines[1], "data: "),
		})
	}
	return frames
}

// readSSEFrame reads one frame off a live stream (blank line terminates it).
func readSSEFrame(r *bufio.Reader) (sseFrame, error) {
	var f sseFrame
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return f, err
		}
		line = strings.TrimRight(line, "\n")
		switch {
		case line == "":
			if f.event != "" || f.data != "" {
				return f, nil
			}
		case strings.HasPrefix(line, "event: "):
			f.event = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			f.data = strings.TrimPrefix(line, "data: ")
		}
	}
}

// --- tests -----------------------------------------------------------------

func TestPipelineStreamFrameOrderAndShape(t *testing.T) {
	stage := cannedStage
	h := &PipelineHandler{
		pipeline: &fakePipelineRunner{stage: &stage, resp: cannedResponse()},
		log:      discardLog(),
	}

	rec := httptest.NewRecorder()
	h.PipelineStream(rec, streamRequest())

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "text/event-stream; charset=utf-8" {
		t.Fatalf("Content-Type = %q, want text/event-stream; charset=utf-8", got)
	}
	if !rec.Flushed {
		t.Error("recorder not flushed — frames would sit in the server buffer")
	}

	frames := parseFrames(t, rec.Body.String())
	if len(frames) != 3 {
		t.Fatalf("expected 3 frames (data_start, data_done, result), got %d: %+v", len(frames), frames)
	}

	if frames[0].event != "stage" {
		t.Errorf("frame[0].event = %q, want stage", frames[0].event)
	}
	var start struct {
		Phase string `json:"phase"`
	}
	if err := json.Unmarshal([]byte(frames[0].data), &start); err != nil {
		t.Fatalf("frame[0] data not JSON: %v (%q)", err, frames[0].data)
	}
	if start.Phase != "data_start" {
		t.Errorf("frame[0].phase = %q, want data_start", start.Phase)
	}

	if frames[1].event != "stage" {
		t.Errorf("frame[1].event = %q, want stage", frames[1].event)
	}
	var done stageDataDoneFrame
	if err := json.Unmarshal([]byte(frames[1].data), &done); err != nil {
		t.Fatalf("frame[1] data not JSON: %v (%q)", err, frames[1].data)
	}
	want := stageDataDoneFrame{
		Phase:    "data_done",
		Signal:   "new_search: 12 items found",
		ToolName: "catalog_search",
		Count:    12,
		Agent1Ms: 2031,
	}
	if done != want {
		t.Errorf("data_done frame = %+v, want %+v", done, want)
	}

	if frames[2].event != "result" {
		t.Errorf("frame[2].event = %q, want result", frames[2].event)
	}
	var result struct {
		Document  map[string]interface{} `json:"document"`
		LatencyMs int64                  `json:"latencyMs"`
		Agent1Ms  int64                  `json:"agent1Ms"`
		Agent2Ms  int64                  `json:"agent2Ms"`
	}
	if err := json.Unmarshal([]byte(frames[2].data), &result); err != nil {
		t.Fatalf("frame[2] data not JSON: %v (%q)", err, frames[2].data)
	}
	if v, _ := result.Document["version"].(string); v != "2.10" {
		t.Errorf("result document.version = %q, want 2.10", v)
	}
	if result.LatencyMs != 4321 || result.Agent1Ms != 2031 || result.Agent2Ms != 2290 {
		t.Errorf("result latencies = %d/%d/%d, want 4321/2031/2290",
			result.LatencyMs, result.Agent1Ms, result.Agent2Ms)
	}
}

// TestPipelineStreamIncrementalThroughMiddleware proves the
// recordingWriter.Unwrap fix: with WithLogging in front, the data_done frame
// must reach the client WHILE the runner is still blocked between OnStage
// and return — i.e. Flush propagates through the middleware's writer wrapper.
func TestPipelineStreamIncrementalThroughMiddleware(t *testing.T) {
	stage := cannedStage
	block := make(chan struct{})
	h := &PipelineHandler{
		pipeline: &fakePipelineRunner{stage: &stage, resp: cannedResponse(), block: block},
		log:      discardLog(),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/pipeline/stream", h.PipelineStream)
	// Stand-in for WithTenant — stamps the tenant the handler requires.
	stamped := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), ctxKeyTenant{}, &domain.Tenant{ID: "tnt-1", Slug: "acme"})
		mux.ServeHTTP(w, r.WithContext(ctx))
	})
	srv := httptest.NewServer(WithLogging(discardLog())(stamped))
	defer srv.Close()
	// Unpark the runner even when an assertion fails mid-test — otherwise
	// srv.Close() waits forever on the in-flight handler.
	var unblockOnce sync.Once
	unblock := func() { unblockOnce.Do(func() { close(block) }) }
	defer unblock()

	// Bounded client: without working flush propagation not even the
	// response headers leave the server, so an unbounded Post would hang
	// the whole test instead of failing it.
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(srv.URL+"/api/v1/pipeline/stream", "application/json",
		strings.NewReader(`{"sessionId":"s1","query":"show me serums"}`))
	if err != nil {
		t.Fatalf("POST did not return headers — flush likely not propagating: %v", err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream; charset=utf-8" {
		t.Fatalf("Content-Type = %q, want text/event-stream; charset=utf-8", ct)
	}

	frames := make(chan sseFrame, 8)
	readErrs := make(chan error, 1)
	go func() {
		r := bufio.NewReader(resp.Body)
		for {
			f, err := readSSEFrame(r)
			if err != nil {
				readErrs <- err
				return
			}
			frames <- f
		}
	}()
	waitFrame := func(stage string) sseFrame {
		t.Helper()
		select {
		case f := <-frames:
			return f
		case err := <-readErrs:
			t.Fatalf("stream ended waiting for %s: %v", stage, err)
		case <-time.After(5 * time.Second):
			t.Fatalf("timeout waiting for %s — Flush did not propagate through the middleware chain", stage)
		}
		return sseFrame{}
	}

	f := waitFrame("data_start")
	if f.event != "stage" || !strings.Contains(f.data, `"data_start"`) {
		t.Fatalf("first frame = %+v, want stage data_start", f)
	}
	// Runner is still parked on block here: receiving data_done now proves
	// incremental delivery.
	f = waitFrame("data_done (runner still blocked)")
	if f.event != "stage" || !strings.Contains(f.data, `"data_done"`) {
		t.Fatalf("second frame = %+v, want stage data_done", f)
	}

	unblock()
	f = waitFrame("result")
	if f.event != "result" {
		t.Fatalf("third frame = %+v, want result", f)
	}
	var result struct {
		Document map[string]interface{} `json:"document"`
	}
	if err := json.Unmarshal([]byte(f.data), &result); err != nil {
		t.Fatalf("result data not JSON: %v (%q)", err, f.data)
	}
	if v, _ := result.Document["version"].(string); v != "2.10" {
		t.Errorf("result document.version = %q, want 2.10", v)
	}
}

func TestPipelineStreamSessionKilledMidStream(t *testing.T) {
	stage := cannedStage
	h := &PipelineHandler{
		pipeline: &fakePipelineRunner{stage: &stage, err: fmt.Errorf("agent2: %w", domain.ErrSessionKilled)},
		log:      discardLog(),
	}

	rec := httptest.NewRecorder()
	h.PipelineStream(rec, streamRequest())

	frames := parseFrames(t, rec.Body.String())
	if len(frames) != 3 {
		t.Fatalf("expected 3 frames (data_start, data_done, error), got %d: %+v", len(frames), frames)
	}
	last := frames[2]
	if last.event != "error" {
		t.Fatalf("last frame event = %q, want error", last.event)
	}
	var ef streamErrorFrame
	if err := json.Unmarshal([]byte(last.data), &ef); err != nil {
		t.Fatalf("error data not JSON: %v (%q)", err, last.data)
	}
	if ef.Status != http.StatusConflict || ef.Message != "session closed" {
		t.Errorf("error frame = %+v, want {409 session closed}", ef)
	}
}

func TestPipelineStreamRateLimitedIsPlainJSON(t *testing.T) {
	stage := cannedStage
	guard := NewPipelineGuard(1, 0, discardLog())
	fixed := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	guard.now = func() time.Time { return fixed } // frozen clock: no token refill
	h := &PipelineHandler{
		pipeline: &fakePipelineRunner{stage: &stage, resp: cannedResponse()},
		guard:    guard,
		log:      discardLog(),
	}

	// Burst of 1 — the first request spends the only token.
	rec := httptest.NewRecorder()
	h.PipelineStream(rec, streamRequest())
	if rec.Code != http.StatusOK {
		t.Fatalf("first request status = %d, want 200", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.PipelineStream(rec, streamRequest())
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("second request status = %d, want 429", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); strings.Contains(ct, "text/event-stream") {
		t.Errorf("rate-limit rejection must not be SSE, got Content-Type %q", ct)
	}
	if body := strings.TrimSpace(rec.Body.String()); body != "rate limit exceeded, slow down" {
		t.Errorf("body = %q, want plain rate-limit message", body)
	}
}
