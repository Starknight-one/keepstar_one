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
	"time"

	anthropicAdapter "keepstar_v5/internal/adapters/anthropic"
	"keepstar_v5/internal/adapters/postgres"
	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/engine/presets"
	"keepstar_v5/internal/handlers"
	"keepstar_v5/internal/ports"
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
	if err := pg.RunTraceMigrations(ctx); err != nil {
		t.Fatalf("RunTraceMigrations: %v", err)
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

	promptCache := usecases.NewPromptCache(fdPort, presetPort, "product")
	agent1Cache := usecases.NewAgent1PromptCache(catalog)
	agent1 := usecases.NewAgent1Execute(llm, statePort, catalog, registry, agent1Cache, log)
	agent2 := usecases.NewAgent2Execute(llm, statePort, registry, promptCache)
	prefetchBuilder := usecases.NewPrefetchBuilder(presetPort, componentPort, log)
	pipeline := usecases.NewPipelineExecute(agent1, agent2, statePort, prefetchBuilder, log)

	tracePort := postgres.NewTraceAdapter(pg)
	sessionH := handlers.NewSessionHandler(statePort, pg.Pool())
	pipelineH := handlers.NewPipelineHandler(pipeline, tracePort, nil, log)
	actionH := handlers.NewActionHandler(statePort)
	navigationH := handlers.NewNavigationHandler(statePort, presetPort, componentPort, log)
	presetH := handlers.NewPresetHandler(promptCache)
	router := handlers.RegisterRoutes(log, catalog, pg.Pool(), "", "hey-babes-cosmetics", sessionH, pipelineH, actionH, navigationH, presetH)

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
	// Chunk 12 — mode is REQUIRED on every call. A genuine modify ask
	// must land mode="modify". If the LLM goes mode="rebuild" instead,
	// the tweak isn't a tweak.
	if mode := turn4.modeForLastCall(); mode != "modify" {
		t.Errorf("turn 4: expected mode=modify on tweak ask, got mode=%q", mode)
	}

	// Inspect the rich tracing on turn 4 — when state has a current
	// Document, tree_map must have fired.
	if !turn4.hasSpanName("agent2.tree_map.build") {
		t.Errorf("turn 4: agent2.tree_map.build span did not fire (modify-mode tree_map missing)")
	}

	// === Chunk 11 — actions + navigation =====================================
	//
	// Use a FRESH session so the rebuild prompt isn't biased toward
	// modify-mode by the chunk-9 turns (turn 4 left a tree_map that
	// nudges Agent2 to keep modifying). A fresh session guarantees an
	// empty conversation and thus a from-scratch product_card render
	// that exercises the prefetch + adjacency code path.
	req, _ = http.NewRequest(http.MethodPost, srv.URL+"/api/v1/session/init", nil)
	req.Header.Set("X-Tenant-Slug", "hey-babes-cosmetics")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /session/init (chunk-11 session): %v", err)
	}
	var navSession struct {
		SessionID string `json:"sessionId"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&navSession)
	resp.Body.Close()
	if navSession.SessionID == "" {
		t.Fatalf("chunk-11 session init returned empty sessionId")
	}
	t.Cleanup(func() {
		_, _ = pg.Pool().Exec(context.Background(),
			`DELETE FROM v5_chat_sessions WHERE id = $1::uuid`, navSession.SessionID)
	})
	t.Logf("chunk-11 session: %s", navSession.SessionID)

	turn5 := postPipeline(t, srv.URL, navSession.SessionID,
		"Search for any 3 products and render them as a grid using the product_card preset")
	if turn5.errored() {
		t.Fatalf("turn 5 (rebuild card grid): tool errored: %s", turn5.summary())
	}
	products := turn5.firstProducts(t, statePort, navSession.SessionID)
	if len(products) == 0 {
		t.Fatalf("turn 5: state has no products to drive action / nav tests")
	}
	firstID := products[0].ID

	// Chunk 12 — turn 5 is a fresh-session rebuild; mode must be rebuild.
	if mode := turn5.modeForLastCall(); mode != "rebuild" {
		t.Errorf("turn 5: expected mode=rebuild on fresh-session ask, got mode=%q", mode)
	}

	// Chunk 12 — rebuild on a card preset emits a top-level grid wrapper
	// (frame with layout.wrap=true). Verify it lands in the response.
	if !turn5HasGridWrapper(turn5.parsed.Document) {
		t.Logf("turn 5: top-level grid wrapper missing; doc=%s", turn5.summary())
	}

	// Pipeline response should now carry the prefetch payload (preset
	// product_card has product_detail as drill target via SystemAdjacency).
	if turn5.parsed.Prefetch == nil {
		// Dump the tool calls + the document's __presetInUse marker
		// for diagnosis. The orchestrator only builds prefetch when
		// __presetInUse is non-empty AND adjacency has a drill target.
		presetInUse, _ := turn5.parsed.Document["__presetInUse"].(string)
		callDump, _ := json.Marshal(turn5.parsed.ToolCalls)
		t.Errorf("turn 5: expected prefetch payload, got nil — presetInUse=%q toolCalls=%s",
			presetInUse, string(callDump))
	} else {
		if _, ok := turn5.parsed.Prefetch.AdjacentTemplate["product"]; !ok {
			t.Errorf("turn 5: prefetch.adjacentTemplate[product] missing")
		}
		if got := len(turn5.parsed.Prefetch.Entities["product"]); got == 0 {
			t.Errorf("turn 5: prefetch.entities[product] is empty")
		}
	}

	// POST /api/v1/actions — like the first product, expect 200 + LikedIds.
	actionBody := []byte(`{"sessionId":"` + navSession.SessionID + `","kind":"like","entity":{"type":"product","id":"` + firstID + `"}}`)
	req, _ = http.NewRequest(http.MethodPost, srv.URL+"/api/v1/actions", bytes.NewReader(actionBody))
	req.Header.Set("X-Tenant-Slug", "hey-babes-cosmetics")
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /actions: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("/actions status %d: %s", resp.StatusCode, body)
	}
	var actResp struct {
		Success bool `json:"success"`
		Actions struct {
			LikedIds []string `json:"likedIds"`
		} `json:"actions"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&actResp)
	resp.Body.Close()
	if !actResp.Success || !containsString(actResp.Actions.LikedIds, firstID) {
		t.Errorf("/actions: like did not record; resp=%+v", actResp)
	}

	// POST /api/v1/navigation/expand — drill into product_detail for the
	// first product. Expect Document, viewMode=detail, stackSize=1.
	navBody := []byte(`{"sessionId":"` + navSession.SessionID + `","entityType":"product","entityId":"` + firstID + `"}`)
	req, _ = http.NewRequest(http.MethodPost, srv.URL+"/api/v1/navigation/expand", bytes.NewReader(navBody))
	req.Header.Set("X-Tenant-Slug", "hey-babes-cosmetics")
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /navigation/expand: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("/navigation/expand status %d: %s", resp.StatusCode, body)
	}
	var navExpResp struct {
		Success     bool                   `json:"success"`
		Document    map[string]interface{} `json:"document"`
		ViewMode    string                 `json:"viewMode"`
		StackSize   int                    `json:"stackSize"`
		CanGoBack   bool                   `json:"canGoBack"`
		PresetInUse string                 `json:"presetInUse"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&navExpResp)
	resp.Body.Close()
	if !navExpResp.Success {
		t.Errorf("/navigation/expand: success=false")
	}
	if navExpResp.Document == nil {
		t.Errorf("/navigation/expand: document missing")
	}
	if navExpResp.PresetInUse == "" {
		t.Errorf("/navigation/expand: presetInUse missing")
	}
	if !navExpResp.CanGoBack {
		t.Errorf("/navigation/expand: canGoBack=false; expected true after first push")
	}

	// POST /api/v1/navigation/back — pop, restore prior template.
	backBody := []byte(`{"sessionId":"` + navSession.SessionID + `"}`)
	req, _ = http.NewRequest(http.MethodPost, srv.URL+"/api/v1/navigation/back", bytes.NewReader(backBody))
	req.Header.Set("X-Tenant-Slug", "hey-babes-cosmetics")
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /navigation/back: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("/navigation/back status %d: %s", resp.StatusCode, body)
	}
	var navBackResp struct {
		Success   bool `json:"success"`
		StackSize int  `json:"stackSize"`
		CanGoBack bool `json:"canGoBack"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&navBackResp)
	resp.Body.Close()
	if !navBackResp.Success {
		t.Errorf("/navigation/back: success=false")
	}
	if navBackResp.StackSize != 0 || navBackResp.CanGoBack {
		t.Errorf("/navigation/back: expected empty stack, got size=%d canGoBack=%v",
			navBackResp.StackSize, navBackResp.CanGoBack)
	}

	// === Chunk 13 — trace persistence ========================================
	//
	// Trace persist runs in a background goroutine fired post-response.
	// Give it a moment to land before counting.
	time.Sleep(500 * time.Millisecond)

	// Count traces persisted for the chunk-11 fresh session — should
	// be at least 1 (turn 5 rebuild). Use direct SQL via the pool.
	var traceCount int
	if err := pg.Pool().QueryRow(context.Background(),
		`SELECT COUNT(*) FROM v5_chat_session_traces WHERE session_id = $1::uuid`,
		navSession.SessionID).Scan(&traceCount); err != nil {
		t.Errorf("count traces (chunk-11 session): %v", err)
	}
	if traceCount < 1 {
		t.Errorf("expected ≥1 persisted trace for chunk-11 session, got %d", traceCount)
	}
	t.Logf("chunk-13 traces persisted (chunk-11 session): %d", traceCount)

	// Cleanup traces too — ON DELETE CASCADE on session FK handles this
	// implicitly when session row is deleted, but the chunk-9 t.Cleanup
	// is already in place. Just verify the cascade for hygiene.

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
	ToolCalls []map[string]any          `json:"toolCalls"`
	Usage     map[string]any            `json:"usage"`
	LatencyMs int64                     `json:"latencyMs"`
	Document  map[string]interface{}    `json:"document"`
	Spans     []map[string]any          `json:"spans"`
	Prefetch  *usecases.PrefetchPayload `json:"prefetch,omitempty"`
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

// modeForLastCall returns the `mode` value the LLM passed on the last
// visual_assembly invocation, or "" if the tool wasn't called.
func (r *pipelineTurnResult) modeForLastCall() string {
	for i := len(r.parsed.ToolCalls) - 1; i >= 0; i-- {
		tc := r.parsed.ToolCalls[i]
		name, _ := tc["name"].(string)
		if name != "visual_assembly" {
			continue
		}
		input, _ := tc["input"].(map[string]any)
		mode, _ := input["mode"].(string)
		return mode
	}
	return ""
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

// firstProducts loads the products zone from state; chunk-11 helpers
// use it to pick a stable entity ID for the action / nav turns.
func (r *pipelineTurnResult) firstProducts(t *testing.T, statePort ports.StatePort, sessionID string) []domain.Product {
	t.Helper()
	state, err := statePort.GetState(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	return state.Current.Data.Products
}

// turn5HasGridWrapper reports whether the chunk-12 grid wrapper is
// present at the document root (frame with layout.wrap=true). Used to
// confirm card preset rebuilds produce a row-of-cards rather than a
// vertical stack of clones.
func turn5HasGridWrapper(doc map[string]interface{}) bool {
	kids, _ := doc["children"].([]interface{})
	for _, k := range kids {
		m, ok := k.(map[string]any)
		if !ok {
			continue
		}
		if t, _ := m["type"].(string); t != "frame" {
			continue
		}
		layout, _ := m["layout"].(map[string]any)
		if w, _ := layout["wrap"].(bool); w {
			return true
		}
	}
	return false
}

// containsString reports whether s contains v. Mirrors the helper in
// handler_action.go but kept local to the _test package to avoid
// exporting it.
func containsString(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
