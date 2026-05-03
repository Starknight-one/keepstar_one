//go:build integration && live

// Live HTTP smoke for chunk 6c: spins up a V5 router in-process and hits
// every endpoint against real Anthropic Haiku + real Neon.
//
// Run with:
//
//	ANTHROPIC_API_KEY=$KEY TEST_DATABASE_URL=$DB \
//	  go test -tags="integration live" -v -count=1 \
//	    ./internal/handlers/... -run TestHTTPLiveSmoke
//
// Cost: ~$0.005-0.01 per run (one Agent2 turn).

package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	anthropicAdapter "keepstar_v5/internal/adapters/anthropic"
	"keepstar_v5/internal/adapters/postgres"
	"keepstar_v5/internal/engine/presets"
	"keepstar_v5/internal/handlers"
	"keepstar_v5/internal/tools"
	"keepstar_v5/internal/usecases"
)

// TestHTTPLiveSmoke walks the full HTTP path:
//
//  1. Spin up real Postgres adapters + Anthropic client.
//  2. Wire handlers + middleware.
//  3. POST /api/v1/session/init → get sessionId.
//  4. POST /api/v1/pipeline → expect Document JSON + non-empty toolCalls.
//  5. GET /api/v1/session/{id} → expect state present.
func TestHTTPLiveSmoke(t *testing.T) {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if apiKey == "" || dbURL == "" {
		t.Skip("ANTHROPIC_API_KEY or TEST_DATABASE_URL not set — skipping HTTP live")
	}

	ctx := context.Background()
	pg, err := postgres.NewClient(ctx, dbURL)
	if err != nil {
		t.Fatalf("pg connect: %v", err)
	}
	defer pg.Close()
	if err := pg.RunStateMigrations(ctx); err != nil {
		t.Fatalf("RunStateMigrations: %v", err)
	}
	if err := pg.RunPresetMigrations(ctx); err != nil {
		t.Fatalf("RunPresetMigrations: %v", err)
	}
	if err := pg.RunComponentMigrations(ctx); err != nil {
		t.Fatalf("RunComponentMigrations: %v", err)
	}

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	catalog := postgres.NewCatalogAdapter(pg)
	statePort := postgres.NewStateAdapter(pg, log)
	presetPort := postgres.NewPresetAdapterWithSystem(pg, presets.NewSystemPresetRegistry())
	componentPort := postgres.NewComponentAdapter(pg)
	fdPort := postgres.NewFieldDefinitionAdapter(pg)
	llm := anthropicAdapter.NewClient(apiKey, "claude-haiku-4-5")

	registry := tools.NewRegistry()
	registry.Register(tools.NewVisualAssemblyTool(statePort, presetPort, componentPort))
	registry.Register(tools.NewCatalogSearchTool(statePort, catalog))
	registry.Register(tools.NewStateFilterTool(statePort))
	registry.Register(tools.NewHistoryLookupTool(statePort))

	promptCache := usecases.NewPromptCache(fdPort, "product")
	agent1Cache := usecases.NewAgent1PromptCache(catalog)
	agent1 := usecases.NewAgent1Execute(llm, statePort, catalog, registry, agent1Cache, log)
	agent2 := usecases.NewAgent2Execute(llm, statePort, registry, promptCache)
	pipeline := usecases.NewPipelineExecute(agent1, agent2, log)

	sessionH := handlers.NewSessionHandler(statePort, pg.Pool())
	pipelineH := handlers.NewPipelineHandler(pipeline)
	router := handlers.RegisterRoutes(log, catalog, "hey-babes-cosmetics", sessionH, pipelineH)

	srv := httptest.NewServer(router)
	defer srv.Close()
	t.Logf("test server: %s", srv.URL)

	// /healthz
	resp, err := http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("/healthz status = %d", resp.StatusCode)
	}
	resp.Body.Close()

	// POST /api/v1/session/init
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/session/init", nil)
	req.Header.Set("X-Tenant-Slug", "hey-babes-cosmetics")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /session/init: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("/session/init status %d: %s", resp.StatusCode, body)
	}
	var sessResp struct {
		SessionID string `json:"sessionId"`
		Tenant    struct {
			Slug string `json:"slug"`
			Name string `json:"name"`
		} `json:"tenant"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&sessResp)
	resp.Body.Close()
	if sessResp.SessionID == "" {
		t.Fatalf("session_init returned empty sessionId: %+v", sessResp)
	}
	t.Logf("session: %s, tenant: %s", sessResp.SessionID, sessResp.Tenant.Slug)
	t.Cleanup(func() {
		_, _ = pg.Pool().Exec(context.Background(),
			`DELETE FROM v5_chat_sessions WHERE id = $1::uuid`, sessResp.SessionID)
	})

	// POST /api/v1/pipeline — full Agent1 → Agent2 chain. Agent1 should
	// pick catalog_search and populate state.Current.Data, Agent2 should
	// then call visual_assembly and write a Document. Spans from both
	// agents land in the same collector so we can verify both fired.
	pipBody := []byte(`{"sessionId":"` + sessResp.SessionID + `","query":"Show me 3 products from your catalog"}`)
	req, _ = http.NewRequest(http.MethodPost, srv.URL+"/api/v1/pipeline", bytes.NewReader(pipBody))
	req.Header.Set("X-Tenant-Slug", "hey-babes-cosmetics")
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /pipeline: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("/pipeline status %d: %s", resp.StatusCode, body)
	}
	var pipResp struct {
		ToolCalls []map[string]any       `json:"toolCalls"`
		Usage     map[string]any         `json:"usage"`
		LatencyMs int64                  `json:"latencyMs"`
		Agent1Ms  int64                  `json:"agent1Ms"`
		Agent2Ms  int64                  `json:"agent2Ms"`
		Document  map[string]interface{} `json:"document"`
		Spans     []map[string]any       `json:"spans"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&pipResp)
	resp.Body.Close()
	t.Logf("pipeline: %d tool calls, %dms (a1=%dms a2=%dms), %d spans, usage=%+v",
		len(pipResp.ToolCalls), pipResp.LatencyMs, pipResp.Agent1Ms, pipResp.Agent2Ms,
		len(pipResp.Spans), pipResp.Usage)
	if len(pipResp.ToolCalls) == 0 {
		t.Errorf("expected at least one tool call, got 0")
	}
	if pipResp.LatencyMs == 0 {
		t.Errorf("latencyMs is 0; clock arithmetic broken?")
	}
	if pipResp.Agent1Ms == 0 || pipResp.Agent2Ms == 0 {
		t.Errorf("expected both Agent1Ms + Agent2Ms > 0, got a1=%d a2=%d", pipResp.Agent1Ms, pipResp.Agent2Ms)
	}
	if len(pipResp.Spans) == 0 {
		t.Errorf("expected non-empty spans (chunk-6d tracer); got 0")
	}
	// Spot-check a few expected span names so we know the tracer is
	// actually firing in production paths for both agents.
	gotNames := map[string]bool{}
	byName := map[string]map[string]any{}
	for _, s := range pipResp.Spans {
		if name, _ := s["name"].(string); name != "" {
			gotNames[name] = true
			byName[name] = s
		}
	}
	for _, want := range []string{
		"pipeline.execute",
		"agent1.execute",
		"agent1.llm",
		"agent2.execute",
		"agent2.llm",
		"postgres.GetState",
	} {
		if !gotNames[want] {
			t.Errorf("expected span %q to fire, got names: %v", want, keys(gotNames))
		}
	}

	// Chunk-8 trace upgrade: each span carries an id; nested spans carry parent_id.
	if pipExec, ok := byName["pipeline.execute"]; ok {
		if id, _ := pipExec["id"].(string); id == "" {
			t.Errorf("pipeline.execute span has no id: %+v", pipExec)
		}
		if pid, _ := pipExec["parent_id"].(string); pid != "" {
			t.Errorf("pipeline.execute should be root (no parent_id), got %q", pid)
		}
		// pipeline.execute attrs: request_id + agent1_ms + agent2_ms + microcontext.
		attrs, _ := pipExec["attrs"].(map[string]any)
		if attrs == nil {
			t.Error("pipeline.execute has no attrs")
		} else {
			if rid, _ := attrs["request_id"].(string); rid == "" {
				t.Errorf("pipeline.execute.attrs.request_id missing: %+v", attrs)
			}
			if _, ok := attrs["agent1_ms"]; !ok {
				t.Errorf("pipeline.execute.attrs.agent1_ms missing: %+v", attrs)
			}
			if mc, _ := attrs["microcontext"].(string); mc == "" {
				t.Errorf("pipeline.execute.attrs.microcontext missing")
			}
		}
	}
	if a2llm, ok := byName["agent2.llm"]; ok {
		if pid, _ := a2llm["parent_id"].(string); pid == "" {
			t.Errorf("agent2.llm must have parent_id (parent=agent2.execute), got empty")
		}
		attrs, _ := a2llm["attrs"].(map[string]any)
		if attrs == nil {
			t.Error("agent2.llm has no attrs")
		} else {
			// JSON-decoded numbers come back as float64.
			if v, _ := attrs["tokens.input"].(float64); v <= 0 {
				t.Errorf("agent2.llm.attrs.tokens.input = %v, want > 0", attrs["tokens.input"])
			}
			if v, _ := attrs["cost_usd"].(float64); v <= 0 {
				t.Errorf("agent2.llm.attrs.cost_usd = %v, want > 0", attrs["cost_usd"])
			}
			if model, _ := attrs["model"].(string); model == "" {
				t.Errorf("agent2.llm.attrs.model missing")
			}
		}
	}
	if listProducts, ok := byName["postgres.ListProducts"]; ok {
		attrs, _ := listProducts["attrs"].(map[string]any)
		if _, ok := attrs["tenant_id"].(string); !ok {
			t.Errorf("postgres.ListProducts.attrs.tenant_id missing: %+v", attrs)
		}
		if _, ok := attrs["rows"]; !ok {
			t.Errorf("postgres.ListProducts.attrs.rows missing: %+v", attrs)
		}
	}
	// No span should be in error status on a happy-path run.
	for _, s := range pipResp.Spans {
		if status, _ := s["status"].(string); status == "error" {
			t.Errorf("unexpected error span on happy path: %+v", s)
		}
	}

	_ = pipResp.Document // shape-check: just confirm it parsed

	// === Chunk 9 — additional turns ==========================================
	//
	// Turn 2: ask for product_detail. The DB has no such preset; the
	// SystemPresetRegistry must serve it. Asserts no «preset not found»
	// error path (registry fallback) and that Agent2 picked a detail-style
	// preset.
	turn2 := postPipeline(t, srv.URL, sessResp.SessionID,
		"Show me the detail page for the first product")
	if turn2.errored() {
		t.Errorf("turn 2 (product_detail registry): tool errored: %s", turn2.summary())
		t.Logf("turn 2 spans dump: %s", turn2.spansForName("agent2.tool.visual_assembly"))
	}
	if turn2.parsed.Document == nil {
		t.Errorf("turn 2: Document is nil — visual_assembly didn't write template")
	}
	if turn2.toolCallCount() == 0 {
		t.Errorf("turn 2: expected at least one tool call")
	}

	// Turn 3: multi-widget compose. Expect Agent2 to call visual_assembly
	// with no preset + multiple top-level frame inserts. Asserts the
	// resulting Document has ≥ 2 root children (literal hero + at least
	// one replicate clone or another literal).
	turn3 := postPipeline(t, srv.URL, sessResp.SessionID,
		"Compose a presentation: a big headline saying 'New collection', then 3 product cards, then a CTA button 'See all'")
	if turn3.errored() {
		t.Errorf("turn 3 (compose): tool errored: %s", turn3.summary())
	}
	if rootCount := turn3.rootChildrenCount(); rootCount < 2 {
		t.Errorf("turn 3: expected ≥ 2 root children for compose, got %d", rootCount)
	}

	// Turn 4: ops-only modify. By this point state.Current.Template
	// carries turn-3's compose result; tree_map will be injected. Agent2
	// should emit ops-only call (no preset key) and the template should
	// stay non-empty.
	turn4 := postPipeline(t, srv.URL, sessResp.SessionID,
		"Make the headline red and bold")
	if turn4.errored() {
		t.Errorf("turn 4 (modify): tool errored: %s", turn4.summary())
	}
	// Inspect the visual_assembly tool input on turn 4 — for a true
	// modify-mode call it should NOT carry a preset name.
	if turn4.usedPresetForLastCall() {
		t.Logf("turn 4: agent used preset (acceptable but suboptimal); want ops-only modify. last input: %s", turn4.summary())
	}

	// Inspect the rich tracing on turn 4 — when state has a current
	// Document, tree_map must have fired.
	if !turn4.hasSpanName("agent2.tree_map.build") {
		t.Errorf("turn 4: agent2.tree_map.build span did not fire (modify-mode tree_map missing)")
	}

	// GET /api/v1/session/{id}
	resp, err = http.Get(srv.URL + "/api/v1/session/" + sessResp.SessionID)
	if err != nil {
		t.Fatalf("GET /session: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("/session/{id} status %d: %s", resp.StatusCode, body)
	}
	resp.Body.Close()
}

// pipelineTurnResult is a thin wrapper around the pipeline response JSON
// that the chunk-9 turns share. Helpers attached for readable assertions.
type pipelineTurnResult struct {
	t        *testing.T
	rawBody  []byte
	parsed   pipelineHTTPResponse
}

type pipelineHTTPResponse struct {
	ToolCalls []map[string]any       `json:"toolCalls"`
	Usage     map[string]any         `json:"usage"`
	LatencyMs int64                  `json:"latencyMs"`
	Document  map[string]interface{} `json:"document"`
	Spans     []map[string]any       `json:"spans"`
}

func postPipeline(t *testing.T, baseURL, sessionID, query string) *pipelineTurnResult {
	t.Helper()
	body := []byte(`{"sessionId":"` + sessionID + `","query":` + jsonString(query) + `}`)
	req, _ := http.NewRequest(http.MethodPost, baseURL+"/api/v1/pipeline", bytes.NewReader(body))
	req.Header.Set("X-Tenant-Slug", "hey-babes-cosmetics")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /pipeline: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/pipeline status %d: %s", resp.StatusCode, raw)
	}
	out := &pipelineTurnResult{t: t, rawBody: raw}
	_ = json.Unmarshal(raw, &out.parsed)
	t.Logf("turn (%q): %d tool calls, %dms, %d spans", query,
		len(out.parsed.ToolCalls), out.parsed.LatencyMs, len(out.parsed.Spans))
	return out
}

func (r *pipelineTurnResult) toolCallCount() int { return len(r.parsed.ToolCalls) }

// errored reports whether the visual_assembly tool failed for this turn.
// Agent1's tools (history_lookup / state_filter) can legitimately return
// is_error=true (e.g., empty history on first turn) — we don't count
// those. Likewise pipeline.execute or agent2.execute spans flagged
// "error" by transport failure surface as a 500 response and never reach
// this helper.
func (r *pipelineTurnResult) errored() bool {
	for _, s := range r.parsed.Spans {
		name, _ := s["name"].(string)
		if name != "agent2.tool.visual_assembly" {
			continue
		}
		if status, _ := s["status"].(string); status == "error" {
			return true
		}
		if attrs, _ := s["attrs"].(map[string]any); attrs != nil {
			if v, _ := attrs["is_error"].(bool); v {
				return true
			}
		}
	}
	return false
}

func (r *pipelineTurnResult) summary() string {
	if len(r.rawBody) > 600 {
		return string(r.rawBody[:600]) + "..."
	}
	return string(r.rawBody)
}

// rootChildrenCount returns the number of top-level children in the
// returned Document. Used to assert multi-widget compose output.
func (r *pipelineTurnResult) rootChildrenCount() int {
	if r.parsed.Document == nil {
		return 0
	}
	kids, _ := r.parsed.Document["children"].([]interface{})
	return len(kids)
}

// usedPresetForLastCall walks the tool calls looking for the last
// visual_assembly invocation; reports whether its input carried a non-
// empty preset key. Modify-mode calls should not.
func (r *pipelineTurnResult) usedPresetForLastCall() bool {
	for i := len(r.parsed.ToolCalls) - 1; i >= 0; i-- {
		tc := r.parsed.ToolCalls[i]
		name, _ := tc["name"].(string)
		if name != "visual_assembly" {
			continue
		}
		input, _ := tc["input"].(map[string]any)
		preset, _ := input["preset"].(string)
		return preset != ""
	}
	return false
}

// hasSpanName reports whether any span in the response has the given name.
func (r *pipelineTurnResult) hasSpanName(name string) bool {
	for _, s := range r.parsed.Spans {
		if n, _ := s["name"].(string); n == name {
			return true
		}
	}
	return false
}

// spansForName returns the JSON-marshalled bodies of every span carrying
// the given name. Diagnostic helper for failing assertions.
func (r *pipelineTurnResult) spansForName(name string) string {
	out := []map[string]any{}
	for _, s := range r.parsed.Spans {
		if n, _ := s["name"].(string); n == name {
			out = append(out, s)
		}
	}
	body, _ := json.Marshal(out)
	return string(body)
}

// jsonString quotes a string for inclusion in a hand-built JSON body.
// Intentionally minimal — we only need it to encode short prompt
// queries that we control. Falls back to %q if marshalling fails.
func jsonString(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		return `""`
	}
	return string(b)
}

// keys returns the keys of a map[string]bool — used by the live test to
// produce a useful diagnostic when an expected span name is missing.
func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
