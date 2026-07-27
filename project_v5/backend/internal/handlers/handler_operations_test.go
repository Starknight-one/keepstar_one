package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/ports"
)

// fakeOperationRegistry is a canned ports.OperationRegistry: Execute
// returns result/err verbatim and records the context + call it saw.
type fakeOperationRegistry struct {
	result *domain.OperationResult
	err    error

	gotCtx  domain.OperationContext
	gotCall domain.ToolCall
	calls   int
}

func (f *fakeOperationRegistry) RegisterExecutor(domain.OperationKind, ports.Executor) {}

func (f *fakeOperationRegistry) DefinitionsFor(context.Context, string, domain.PipelineMode, domain.AgentPlane, domain.Role) []domain.ToolDefinition {
	return nil
}

func (f *fakeOperationRegistry) Get(context.Context, string, string) (*domain.OperationSpec, error) {
	return nil, errFake("not used")
}

func (f *fakeOperationRegistry) Execute(_ context.Context, octx domain.OperationContext, call domain.ToolCall) (*domain.OperationResult, error) {
	f.calls++
	f.gotCtx = octx
	f.gotCall = call
	return f.result, f.err
}

func (f *fakeOperationRegistry) InvalidateTenant(string) {}

// deltaStatePort records AddDelta calls; every other StatePort method
// panics via the embedded fakeStatePort (scope-creep tripwire).
type deltaStatePort struct {
	fakeStatePort
	deltas []*domain.Delta
}

func (d *deltaStatePort) AddDelta(_ context.Context, _ string, delta *domain.Delta) (int, error) {
	d.deltas = append(d.deltas, delta)
	return len(d.deltas), nil
}

// invokeServer mounts the handler behind a stub tenant context — the same
// wiring shape RegisterRoutes produces (WithTenant → mux).
func invokeServer(h *OperationsHandler) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/operations/invoke", h.Invoke)
	wrapped := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), ctxKeyTenant{}, &domain.Tenant{ID: "tnt-1", Slug: "acme", Name: "Acme"})
		mux.ServeHTTP(w, r.WithContext(ctx))
	})
	return httptest.NewServer(wrapped)
}

func postInvoke(t *testing.T, url, body string) (*http.Response, map[string]any) {
	t.Helper()
	resp, err := http.Post(url+"/api/v1/operations/invoke", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST failed: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	var out map[string]any
	if resp.Header.Get("Content-Type") == "application/json" {
		_ = json.NewDecoder(resp.Body).Decode(&out)
	}
	return resp, out
}

func testLog() *slog.Logger { return slog.Default() }

func TestInvokeValidation(t *testing.T) {
	reg := &fakeOperationRegistry{result: &domain.OperationResult{Outcome: domain.OutcomeOK}}
	srv := invokeServer(NewOperationsHandler(reg, nil, nil, nil, nil, nil, nil, testLog()))
	defer srv.Close()

	cases := []struct {
		name string
		body string
		want int
	}{
		{"missing sessionId", `{"operation":"book_showing"}`, http.StatusBadRequest},
		{"missing operation", `{"sessionId":"s-1"}`, http.StatusBadRequest},
		{"bad json", `{`, http.StatusBadRequest},
		{"register_user rejected (R6)", `{"sessionId":"s-1","operation":"register_user"}`, http.StatusForbidden},
	}
	for _, c := range cases {
		resp, _ := postInvoke(t, srv.URL, c.body)
		if resp.StatusCode != c.want {
			t.Errorf("%s: status = %d, want %d", c.name, resp.StatusCode, c.want)
		}
	}
	if reg.calls != 0 {
		t.Errorf("registry reached on invalid requests: %d calls", reg.calls)
	}
}

// TestInvokeCheapBucket — the §6.3 cheap rate bucket 429s past the limit.
func TestInvokeCheapBucket(t *testing.T) {
	reg := &fakeOperationRegistry{result: &domain.OperationResult{
		Operation: "book_showing", Kind: domain.KindScheduleSlot,
		Outcome: domain.OutcomeOK, Summary: "created",
	}}
	guard := NewCheapGuard(1, testLog()) // burst 1 → second call rejected
	srv := invokeServer(NewOperationsHandler(reg, nil, nil, nil, nil, nil, guard, testLog()))
	defer srv.Close()

	body := `{"sessionId":"s-1","operation":"book_showing"}`
	resp, _ := postInvoke(t, srv.URL, body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("first call status = %d, want 200", resp.StatusCode)
	}
	resp, _ = postInvoke(t, srv.URL, body)
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Errorf("second call status = %d, want 429", resp.StatusCode)
	}
}

// TestInvokeOkScheduleSlot — ok schedule_slot without an adopted
// success_plaque preset: status ok, apply degrades to the form ack, the
// R28 RECORD_CREATE delta lands on Path "records", and the raw params
// never enter the delta (R6 discipline).
func TestInvokeOkScheduleSlot(t *testing.T) {
	reg := &fakeOperationRegistry{result: &domain.OperationResult{
		Operation: "book_showing", Kind: domain.KindScheduleSlot,
		Outcome: domain.OutcomeOK, Count: 1,
		EntityKind: "lead", RecordID: "rec-42",
		Summary: "showing booked for Aug 1, 3:00 PM",
	}}
	state := &deltaStatePort{}
	srv := invokeServer(NewOperationsHandler(reg, state, nil, nil, nil, nil, nil, testLog()))
	defer srv.Close()

	resp, out := postInvoke(t, srv.URL,
		`{"sessionId":"s-1","operation":"book_showing","formId":"f-1",
		  "entity":{"type":"product","id":"prod-9"},
		  "params":{"name":"Ana","phone":"+5511999999999","preferredTime":"2026-08-01T15:00:00Z"}}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if out["status"] != "ok" {
		t.Errorf("status field = %v, want ok", out["status"])
	}

	// Registry saw the session-derived context (visitor default, tenant
	// scoped) and the raw call.
	if reg.gotCtx.Role != domain.RoleVisitor || reg.gotCtx.Mode != domain.ModeStorefront {
		t.Errorf("octx role/mode = %s/%s, want visitor/storefront defaults", reg.gotCtx.Role, reg.gotCtx.Mode)
	}
	if reg.gotCtx.TenantSlug != "acme" || reg.gotCtx.TenantID != "tnt-1" {
		t.Errorf("octx tenant = %s/%s", reg.gotCtx.TenantSlug, reg.gotCtx.TenantID)
	}
	if reg.gotCtx.ActorID != "visitor:s-1" {
		t.Errorf("octx actorId = %q, want visitor:s-1", reg.gotCtx.ActorID)
	}
	if reg.gotCall.Name != "book_showing" {
		t.Errorf("call name = %q", reg.gotCall.Name)
	}

	// Apply fell back to the form ack (no success_plaque adopted).
	apply, _ := out["apply"].([]any)
	if len(apply) != 1 {
		t.Fatalf("apply len = %d, want 1", len(apply))
	}
	entry, _ := apply[0].(map[string]any)
	if entry["target"] != "form" || entry["formId"] != "f-1" || entry["status"] != "ok" {
		t.Errorf("apply[0] = %v, want form ack for f-1", entry)
	}

	// R28 delta: RECORD_CREATE on Path records, no raw params.
	if len(state.deltas) != 1 {
		t.Fatalf("deltas = %d, want 1", len(state.deltas))
	}
	d := state.deltas[0]
	if d.Action.Type != domain.ActionRecordCreate || d.Path != "records" {
		t.Errorf("delta = %s/%s, want RECORD_CREATE/records", d.Action.Type, d.Path)
	}
	if d.Action.Params["recordId"] != "rec-42" || d.Action.Params["entityKind"] != "lead" {
		t.Errorf("delta params = %v", d.Action.Params)
	}
	raw, _ := json.Marshal(d)
	if strings.Contains(string(raw), "+5511999999999") || strings.Contains(string(raw), "Ana") {
		t.Errorf("raw operation params leaked into the delta: %s", raw)
	}

	// Public result projection carries identity only.
	result, _ := out["result"].(map[string]any)
	if result["recordId"] != "rec-42" || result["outcome"] != "ok" {
		t.Errorf("result = %v", result)
	}
	if _, exists := result["output"]; exists {
		t.Error("raw output leaked into the public result")
	}
}

// TestInvokeBlockApply — with success_plaque adopted, ok create-class
// results swap the originating block: apply[0] = {target:"block", blockId,
// document} with the plaque bound from the result and the theme attached.
func TestInvokeBlockApply(t *testing.T) {
	reg := &fakeOperationRegistry{result: &domain.OperationResult{
		Operation: "book_showing", Kind: domain.KindScheduleSlot,
		Outcome: domain.OutcomeOK, RecordID: "rec-7",
		Summary: "showing booked",
	}}
	plaque := &domain.Preset{
		ID: "p-plaque", TenantID: "tnt-1", Name: successPlaquePreset,
		Status: domain.PresetStatusPublished,
		DocumentJSON: []byte(`{
		  "version": "2.10",
		  "children": [
		    {"type": "frame", "id": "plaque", "children": [
		      {"type": "text", "id": "msg", "fieldBinding": "summary"}
		    ]}
		  ]
		}`),
	}
	presets := &minPresetPortH{byName: map[string]*domain.Preset{successPlaquePreset: plaque}}
	srv := invokeServer(NewOperationsHandler(reg, nil, presets, nil, nil, nil, nil, testLog()))
	defer srv.Close()

	resp, out := postInvoke(t, srv.URL,
		`{"sessionId":"s-1","operation":"book_showing","blockId":"b-3","formId":"f-1"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	apply, _ := out["apply"].([]any)
	if len(apply) != 1 {
		t.Fatalf("apply len = %d, want 1", len(apply))
	}
	entry, _ := apply[0].(map[string]any)
	if entry["target"] != "block" || entry["blockId"] != "b-3" {
		t.Errorf("apply[0] = %v, want block swap for b-3", entry)
	}
	doc, _ := entry["document"].(map[string]any)
	if doc == nil {
		t.Fatal("apply[0].document missing")
	}
	raw, _ := json.Marshal(doc)
	if !strings.Contains(string(raw), "showing booked") {
		t.Errorf("plaque did not bind the result summary: %s", raw)
	}
	if _, ok := doc["theme"]; !ok {
		t.Error("theme not attached to the block document")
	}
}

// TestInvokeErrorOutcome — invalid/denied outcomes stay HTTP 200 with
// status "error" and a form-target apply carrying the human-readable
// Summary; no delta is written.
func TestInvokeErrorOutcome(t *testing.T) {
	reg := &fakeOperationRegistry{result: &domain.OperationResult{
		Operation: "book_showing", Kind: domain.KindScheduleSlot,
		Outcome: domain.OutcomeInvalid,
		Summary: "invalid: preferredTime is in the past",
	}}
	state := &deltaStatePort{}
	srv := invokeServer(NewOperationsHandler(reg, state, nil, nil, nil, nil, nil, testLog()))
	defer srv.Close()

	resp, out := postInvoke(t, srv.URL,
		`{"sessionId":"s-1","operation":"book_showing","formId":"f-1"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 (outcome errors ride the body)", resp.StatusCode)
	}
	if out["status"] != "error" {
		t.Errorf("status field = %v, want error", out["status"])
	}
	apply, _ := out["apply"].([]any)
	if len(apply) != 1 {
		t.Fatalf("apply len = %d, want 1", len(apply))
	}
	entry, _ := apply[0].(map[string]any)
	if entry["target"] != "form" || entry["status"] != "error" ||
		entry["message"] != "invalid: preferredTime is in the past" {
		t.Errorf("apply[0] = %v", entry)
	}
	if len(state.deltas) != 0 {
		t.Errorf("delta written for a failed mutation: %d", len(state.deltas))
	}
}

// TestInvokeNoDeltaForNonMutation — notify-class ok results write no R28
// delta (audited via v5_operation_runs + v5_events only).
func TestInvokeNoDeltaForNonMutation(t *testing.T) {
	reg := &fakeOperationRegistry{result: &domain.OperationResult{
		Operation: "notify_agent", Kind: domain.KindNotify,
		Outcome: domain.OutcomeOK, Summary: "notified",
	}}
	state := &deltaStatePort{}
	srv := invokeServer(NewOperationsHandler(reg, state, nil, nil, nil, nil, nil, testLog()))
	defer srv.Close()

	resp, out := postInvoke(t, srv.URL, `{"sessionId":"s-1","operation":"notify_agent"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if out["status"] != "ok" {
		t.Errorf("status field = %v", out["status"])
	}
	apply, _ := out["apply"].([]any)
	if len(apply) != 0 {
		t.Errorf("apply = %v, want empty (no formId, no block swap)", apply)
	}
	if len(state.deltas) != 0 {
		t.Errorf("notify wrote a delta: %d", len(state.deltas))
	}
}

// TestInvokeTransportError — a Go error from Execute is a 500 (transport
// failure), never a fabricated ok body.
func TestInvokeTransportError(t *testing.T) {
	reg := &fakeOperationRegistry{err: errFake("db down")}
	srv := invokeServer(NewOperationsHandler(reg, nil, nil, nil, nil, nil, nil, testLog()))
	defer srv.Close()

	resp, _ := postInvoke(t, srv.URL, `{"sessionId":"s-1","operation":"book_showing"}`)
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", resp.StatusCode)
	}
}

// minPresetPortH mirrors tools' minPresetPort for the handlers package.
type minPresetPortH struct {
	byName map[string]*domain.Preset
}

func (p *minPresetPortH) GetPublishedPreset(_ context.Context, _ string, name string) (*domain.Preset, error) {
	if pr, ok := p.byName[name]; ok {
		return pr, nil
	}
	return nil, domain.ErrPresetNotFound
}

func (p *minPresetPortH) ListPublishedPresets(context.Context, string) ([]domain.Preset, error) {
	panic("not used")
}
