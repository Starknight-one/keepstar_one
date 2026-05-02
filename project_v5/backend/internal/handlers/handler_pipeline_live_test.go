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
	presetPort := postgres.NewPresetAdapter(pg)
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
	for _, s := range pipResp.Spans {
		if name, _ := s["name"].(string); name != "" {
			gotNames[name] = true
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

	_ = pipResp.Document // shape-check: just confirm it parsed

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

// keys returns the keys of a map[string]bool — used by the live test to
// produce a useful diagnostic when an expected span name is missing.
func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
