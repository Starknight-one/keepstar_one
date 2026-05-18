package usecases

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"keepstar-admin/internal/adapters/anthropic"
	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/ports"
)

// =============================================================================
// Test scaffold — FakeAgentSender + fakeAgentRunsPort + capturingArtifact
// =============================================================================

// fakeAgentSender implements AgentSender. Scripted response queue + request
// capture so tests can assert on what was sent.
type fakeAgentSender struct {
	responses []*anthropic.MessagesResponse
	requests  []anthropic.MessagesRequest
	idx       int
	sendErr   error // optional; returned on next Send if non-nil
}

func (f *fakeAgentSender) Send(_ context.Context, req anthropic.MessagesRequest) (*anthropic.MessagesResponse, error) {
	f.requests = append(f.requests, req)
	if f.sendErr != nil {
		return nil, f.sendErr
	}
	if f.idx >= len(f.responses) {
		return nil, fmt.Errorf("fakeAgentSender: out of scripted responses (idx=%d, queue=%d)", f.idx, len(f.responses))
	}
	resp := f.responses[f.idx]
	f.idx++
	return resp, nil
}

// textOnlyResponse — assistant emitted only text (no tool_use). Used to drive
// the no-tool-use nudge path.
func textOnlyResponse(text string, tokensIn, tokensOut int) *anthropic.MessagesResponse {
	return &anthropic.MessagesResponse{
		Content:    []anthropic.ContentBlock{{Type: "text", Text: text}},
		StopReason: "end_turn",
		Usage:      anthropic.Usage{InputTokens: tokensIn, OutputTokens: tokensOut},
	}
}

// readToolResponse — assistant called one read-only tool (count_total etc).
func readToolResponse(toolName string, input any, tokensIn, tokensOut int) *anthropic.MessagesResponse {
	inputJSON, _ := json.Marshal(input)
	return &anthropic.MessagesResponse{
		Content: []anthropic.ContentBlock{
			{Type: "tool_use", ID: "toolu_" + toolName, Name: toolName, Input: inputJSON},
		},
		StopReason: "tool_use",
		Usage:      anthropic.Usage{InputTokens: tokensIn, OutputTokens: tokensOut},
	}
}

// commitArtifactResponse — assistant called commit_artifact with a serialized
// artifact body. Mirrors the JSON schema in discovery_v2_tools.go.
func commitArtifactResponse(art *domain.MappingArtifactV3, tokensIn, tokensOut int) *anthropic.MessagesResponse {
	body := map[string]any{
		"branches":       art.Branches,
		"classify_rules": art.ClassifyRules,
		"notes":          art.Notes,
	}
	bodyJSON, _ := json.Marshal(body)
	return &anthropic.MessagesResponse{
		Content: []anthropic.ContentBlock{
			{Type: "tool_use", ID: "toolu_commit", Name: "commit_artifact", Input: bodyJSON},
		},
		StopReason: "tool_use",
		Usage:      anthropic.Usage{InputTokens: tokensIn, OutputTokens: tokensOut},
	}
}

// fakeAgentRunsPort captures Start/Finish/AppendTool/AddTokens calls.
type fakeAgentRunsPort struct {
	starts   []ports.AgentRunStart
	finishes []agentFinishCall
	tools    []ports.AgentToolCall
	tokens   []agentTokenCall
	runID    string
}

type agentFinishCall struct{ runID, status, artifactID string }
type agentTokenCall struct {
	runID                  string
	tokensIn, tokensOut    int
	costDelta              float64
}

func newFakeAgentRunsPort() *fakeAgentRunsPort {
	return &fakeAgentRunsPort{runID: "run-test-1"}
}

func (f *fakeAgentRunsPort) Start(_ context.Context, in *ports.AgentRunStart) (string, error) {
	f.starts = append(f.starts, *in)
	return f.runID, nil
}
func (f *fakeAgentRunsPort) AppendTool(_ context.Context, _ string, call ports.AgentToolCall) error {
	f.tools = append(f.tools, call)
	return nil
}
func (f *fakeAgentRunsPort) AddTokens(_ context.Context, runID string, tIn, tOut int, costDelta float64) error {
	f.tokens = append(f.tokens, agentTokenCall{runID, tIn, tOut, costDelta})
	return nil
}
func (f *fakeAgentRunsPort) Finish(_ context.Context, runID, status, artifactID string) error {
	f.finishes = append(f.finishes, agentFinishCall{runID, status, artifactID})
	return nil
}
func (f *fakeAgentRunsPort) GetByID(_ context.Context, _ string) (*ports.AgentRun, error) {
	return nil, nil
}
func (f *fakeAgentRunsPort) ListForTenant(_ context.Context, _ string, _, _ int) ([]*ports.AgentRun, error) {
	return nil, nil
}

// capturingArtifact captures every Save call so tests can assert what was
// committed. Get returns whatever is currently stored — Save mutates it so
// downstream re-fetches see the new artifact (sc 111 cascade path).
type capturingArtifact struct {
	art   *domain.MappingArtifactV3
	saved []*domain.MappingArtifactV3
}

func (c *capturingArtifact) Save(_ context.Context, _ string, a *domain.MappingArtifactV3) error {
	c.art = a
	cp := *a
	c.saved = append(c.saved, &cp)
	return nil
}
func (c *capturingArtifact) Get(_ context.Context, _ string) (*domain.MappingArtifactV3, *ports.MappingArtifactMeta, error) {
	return c.art, &ports.MappingArtifactMeta{Status: "active"}, nil
}
func (c *capturingArtifact) MarkStale(_ context.Context, _ string) error { return nil }

// mkDiscovery wires a DiscoveryV2 use-case with all fakes. inbox is empty by
// default — tests that need rows append to inbox.items.
func mkDiscovery() (*DiscoveryV2, *fakeAgentSender, *fakeInbox, *capturingArtifact, *fakeAgentRunsPort, *fakeActionLog) {
	sender := &fakeAgentSender{}
	inbox := newFakeInbox()
	artifact := &capturingArtifact{}
	runs := newFakeAgentRunsPort()
	log := &fakeActionLog{}
	d := NewDiscoveryV2(sender, inbox, artifact, log, runs, logger.New("error"))
	return d, sender, inbox, artifact, runs, log
}

// simpleCommittedArtifact — minimal valid artifact the agent might commit
// for a single-vertical (cosmetics) tenant. Mirrors the FieldMap shape
// apply_v2 expects.
func simpleCommittedArtifact() *domain.MappingArtifactV3 {
	return &domain.MappingArtifactV3{
		Version: 3,
		Branches: []domain.VerticalBranch{{
			Vertical: "cosmetics",
			FieldMap: []domain.FieldMappingRule{
				{From: "title", To: "master.name"},
				{From: "vendor", To: "master.brand"},
				{From: "variants[0].sku", To: "master.sku"},
			},
		}},
	}
}

// =============================================================================
// Tests
// =============================================================================

// TestScenario_112_AgentConfig_BudgetTurnsWallclock_SentToLLM verifies:
// «Discovery agent запускается с Sonnet 4.6, $5 budget, 30 turns max, 10 min
// wallclock, prompt-cached system block».
// Unit assertion: first sent MessagesRequest has Sonnet model, SystemBlocks
// with cache_control marker on the static block, Tools list with
// cache_control on the final tool def.
func TestScenario_112_AgentConfig_BudgetTurnsWallclock_SentToLLM(t *testing.T) {
	d, sender, _, _, _, _ := mkDiscovery()
	sender.responses = []*anthropic.MessagesResponse{
		commitArtifactResponse(simpleCommittedArtifact(), 100, 50),
	}
	if _, err := d.Discover(context.Background(), "t-1", "first_install", nil); err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(sender.requests) != 1 {
		t.Fatalf("requests = %d, want 1", len(sender.requests))
	}
	req := sender.requests[0]
	if req.Model != "claude-sonnet-4-6" {
		t.Errorf("model = %q, want claude-sonnet-4-6", req.Model)
	}
	if req.MaxTokens != 4096 {
		t.Errorf("max_tokens = %d, want 4096", req.MaxTokens)
	}
	if len(req.SystemBlocks) < 1 {
		t.Fatalf("SystemBlocks empty — want at least one cached block")
	}
	if req.SystemBlocks[0].CacheControl == nil || req.SystemBlocks[0].CacheControl.Type != "ephemeral" {
		t.Errorf("first SystemBlock missing cache_control: %+v", req.SystemBlocks[0])
	}
	if len(req.Tools) < 1 {
		t.Fatalf("Tools empty")
	}
	last := req.Tools[len(req.Tools)-1]
	if last.CacheControl == nil || last.CacheControl.Type != "ephemeral" {
		t.Errorf("last tool def missing cache_control (Anthropic caches up-to-and-including): %+v", last)
	}
}

// TestScenario_114_CommitArtifact_PersistsMappingArtifactV3 verifies:
// «Agent коммитит artifact через commit_artifact tool → MappingArtifactV3
// с branches[] + classify_rules → сохраняется».
func TestScenario_114_CommitArtifact_PersistsMappingArtifactV3(t *testing.T) {
	d, sender, _, artifact, _, _ := mkDiscovery()
	committed := &domain.MappingArtifactV3{
		Version: 3,
		Branches: []domain.VerticalBranch{
			{Vertical: "cosmetics", FieldMap: []domain.FieldMappingRule{
				{From: "title", To: "master.name"},
				{From: "vendor", To: "master.brand"},
				{From: "variants[0].sku", To: "master.sku"},
				{From: "skin_type", To: "cosmetics.skin_type", Transform: "split:,"},
			}},
		},
		ClassifyRules: []domain.ClassifyRule{
			{When: "product_type contains 'serum'", ThenVertical: "cosmetics"},
		},
		Notes: "discovered cosmetics-only tenant",
	}
	sender.responses = []*anthropic.MessagesResponse{
		commitArtifactResponse(committed, 200, 100),
	}
	got, err := d.Discover(context.Background(), "t-1", "first_install", nil)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if got == nil {
		t.Fatal("Discover returned nil artifact")
	}
	if got.Version != 3 {
		t.Errorf("artifact.Version = %d, want 3", got.Version)
	}
	if len(got.Branches) != 1 || got.Branches[0].Vertical != "cosmetics" {
		t.Errorf("branches = %+v, want one cosmetics branch", got.Branches)
	}
	if len(got.ClassifyRules) != 1 {
		t.Errorf("classify_rules = %d, want 1", len(got.ClassifyRules))
	}
	if len(artifact.saved) != 1 {
		t.Errorf("artifact.Save called %d times, want 1", len(artifact.saved))
	}
}

// TestScenario_115_ActionLog_DiscoveryStartAndDone_AgentRunStored verifies:
// «action_log пишет discovery_start (ok) и discovery_done (ok+committed:true).
// agent_runs хранит full timeline с tokens, cost».
func TestScenario_115_ActionLog_DiscoveryStartAndDone_AgentRunStored(t *testing.T) {
	d, sender, _, _, runs, log := mkDiscovery()
	sender.responses = []*anthropic.MessagesResponse{
		commitArtifactResponse(simpleCommittedArtifact(), 500, 100),
	}
	if _, err := d.Discover(context.Background(), "t-1", "first_install", nil); err != nil {
		t.Fatalf("Discover: %v", err)
	}

	var startEntry, doneEntry *ports.TenantActionLogEntry
	for _, e := range log.entries {
		switch e.Action {
		case "discovery_start":
			startEntry = e
		case "discovery_done":
			doneEntry = e
		}
	}
	if startEntry == nil {
		t.Error("discovery_start action_log entry missing")
	} else if startEntry.Status != "ok" {
		t.Errorf("discovery_start status = %q, want ok", startEntry.Status)
	}
	if doneEntry == nil {
		t.Error("discovery_done action_log entry missing")
	} else if doneEntry.Status != "ok" {
		t.Errorf("discovery_done status = %q, want ok (on success)", doneEntry.Status)
	}

	if len(runs.starts) != 1 {
		t.Errorf("agent_runs.Start called %d times, want 1", len(runs.starts))
	}
	if runs.starts[0].Trigger != "first_install" {
		t.Errorf("agent_runs start trigger = %q, want first_install", runs.starts[0].Trigger)
	}
	if len(runs.finishes) != 1 || runs.finishes[0].status != "success" {
		t.Errorf("agent_runs.Finish = %+v, want one entry with status=success", runs.finishes)
	}
	if len(runs.tokens) == 0 {
		t.Error("agent_runs.AddTokens never called — token accounting missing")
	}
}

// TestScenario_116_SingleVertical_ArtifactWithOneBranch verifies:
// «Если у tenant 10 SKU косметики — agent коммитит artifact с одним branch=cosmetics».
func TestScenario_116_SingleVertical_ArtifactWithOneBranch(t *testing.T) {
	d, sender, _, _, _, _ := mkDiscovery()
	sender.responses = []*anthropic.MessagesResponse{
		commitArtifactResponse(simpleCommittedArtifact(), 100, 50),
	}
	got, err := d.Discover(context.Background(), "t-1", "first_install", nil)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if got == nil || len(got.Branches) != 1 {
		t.Fatalf("branches = %+v, want exactly one", got.Branches)
	}
	if got.Branches[0].Vertical != "cosmetics" {
		t.Errorf("vertical = %q, want cosmetics", got.Branches[0].Vertical)
	}
}

// TestScenario_117_MultiVertical_TwoBranches_ClassifyRules verifies:
// «Если у tenant 10 cosmetics + 10 electronics — agent коммитит artifact с
// двумя branches и optionally classify_rules».
func TestScenario_117_MultiVertical_TwoBranches_ClassifyRules(t *testing.T) {
	d, sender, _, _, _, _ := mkDiscovery()
	multi := &domain.MappingArtifactV3{
		Version: 3,
		Branches: []domain.VerticalBranch{
			{Vertical: "cosmetics", FieldMap: []domain.FieldMappingRule{
				{From: "title", To: "master.name"},
				{From: "vendor", To: "master.brand"},
				{From: "variants[0].sku", To: "master.sku"},
			}},
			{Vertical: "electronics", FieldMap: []domain.FieldMappingRule{
				{From: "title", To: "master.name"},
				{From: "vendor", To: "master.brand"},
				{From: "variants[0].sku", To: "master.sku"},
				{From: "cpu", To: "tier3.cpu"},
			}},
		},
		ClassifyRules: []domain.ClassifyRule{
			{When: "brand = 'Apple'", ThenVertical: "electronics"},
		},
	}
	sender.responses = []*anthropic.MessagesResponse{
		commitArtifactResponse(multi, 150, 80),
	}
	got, err := d.Discover(context.Background(), "t-1", "first_install", nil)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(got.Branches) != 2 {
		t.Errorf("branches = %d, want 2", len(got.Branches))
	}
	if len(got.ClassifyRules) != 1 {
		t.Errorf("classify_rules = %d, want 1", len(got.ClassifyRules))
	}
}

// TestScenario_118_BudgetThreshold_ForceFinalizeNudge verifies:
// «Если agent доходит до 90% от $5 budget — на следующем turn'е дополнительный
// nudge force commit».
// Mechanic: response 0 has huge token usage crossing 90% cost. After dispatching
// the tool, the user-side reply message gets a force-finalize text appended.
// On next iteration (turn 1) the model commits.
// Pricing (Sonnet 4.6): input $3/M, output $15/M → 90% of $5 = $4.50. We use
// 1.5M input tokens to deterministically cross the threshold in one turn.
func TestScenario_118_BudgetThreshold_ForceFinalizeNudge(t *testing.T) {
	d, sender, _, _, _, _ := mkDiscovery()
	sender.responses = []*anthropic.MessagesResponse{
		readToolResponse("count_total", map[string]any{}, 1_500_000, 100),
		commitArtifactResponse(simpleCommittedArtifact(), 100, 50),
	}
	if _, err := d.Discover(context.Background(), "t-1", "first_install", nil); err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(sender.requests) != 2 {
		t.Fatalf("requests = %d, want 2 (read then commit after nudge)", len(sender.requests))
	}
	// On the second request, the LAST user message must contain a text block
	// with the force-finalize nudge.
	last := sender.requests[1].Messages[len(sender.requests[1].Messages)-1]
	var foundNudge bool
	for _, blk := range last.Content {
		if blk.Type == "text" && strings.Contains(strings.ToLower(blk.Text), "budget warning") {
			foundNudge = true
			break
		}
	}
	if !foundNudge {
		t.Errorf("force-finalize nudge missing from second request's last user message: %+v", last.Content)
	}
}

// TestScenario_119_EndTurnWithoutCommit_3NudgesThenFail verifies:
// «Если agent end_turn'ит без commit — до 3 nudge'ей. Потом — fail».
// Mechanic: 4 consecutive text-only responses. nudgesUsed goes 0→1→2→3, then
// on the 4th iteration nudgesUsed >= maxNudges → return err.
func TestScenario_119_EndTurnWithoutCommit_3NudgesThenFail(t *testing.T) {
	d, sender, _, _, runs, _ := mkDiscovery()
	sender.responses = []*anthropic.MessagesResponse{
		textOnlyResponse("I think we're done.", 100, 50),
		textOnlyResponse("Yes, definitely done.", 100, 50),
		textOnlyResponse("Confirmed done.", 100, 50),
		textOnlyResponse("Still done.", 100, 50),
	}
	got, err := d.Discover(context.Background(), "t-1", "first_install", nil)
	if err == nil {
		t.Fatalf("expected error after 3 nudges, got artifact=%v", got)
	}
	if !strings.Contains(err.Error(), "nudge") && !strings.Contains(err.Error(), "without committing") {
		t.Errorf("err = %q, want mention of nudge/commit failure", err)
	}
	if len(runs.finishes) != 1 || runs.finishes[0].status != "failed" {
		t.Errorf("agent_runs.Finish status = %+v, want one entry with status=failed", runs.finishes)
	}
}

// TestScenario_120_BudgetExhausted_ArtifactNotOverwritten verifies:
// «Если budget exhausted без commit — artifact_run помечается budget_exhausted,
// действующий artifact (если был) НЕ перезаписывается».
// Pre-seed: capturingArtifact.art = existing previously-committed artifact.
// Script: one read-tool response with usage that overshoots $5 budget. After
// turn 0 the cost exceeds budget; turn 1 trips the budget check at the top
// of the loop → errBudgetExhausted. No commit happened, so Save is never
// called by the post-loop branch — existing artifact unchanged.
func TestScenario_120_BudgetExhausted_ArtifactNotOverwritten(t *testing.T) {
	d, sender, _, artifact, runs, _ := mkDiscovery()
	pre := simpleCommittedArtifact()
	pre.Notes = "previously committed — must survive"
	artifact.art = pre

	// Output tokens at $15/M: 400_000 output = $6 — overshoots the $5 cap.
	sender.responses = []*anthropic.MessagesResponse{
		readToolResponse("count_total", map[string]any{}, 50, 400_000),
	}
	got, err := d.Discover(context.Background(), "t-1", "first_install", nil)
	if err == nil {
		t.Fatalf("expected budget-exhausted error, got artifact=%v", got)
	}
	if got != nil {
		t.Errorf("returned artifact = %+v, want nil when budget exhausted", got)
	}
	if artifact.art == nil || artifact.art.Notes != "previously committed — must survive" {
		t.Errorf("existing artifact overwritten: art=%+v", artifact.art)
	}
	if len(artifact.saved) != 0 {
		t.Errorf("artifact.Save called %d times — should be 0 on budget exhaustion", len(artifact.saved))
	}
	if len(runs.finishes) != 1 || runs.finishes[0].status != "budget_exhausted" {
		t.Errorf("agent_runs.Finish = %+v, want one entry status=budget_exhausted", runs.finishes)
	}
}

// TestScenario_122_NarrowDiscovery_MappingMiss_FewerToolsAndDifferentPrompt
// verifies:
// «Narrow discovery_v2 (mapping_miss) фокусируется на конкретном поле —
// system prompt инструктирует "не re-discover весь каталог"».
// Unit assertion: when trigger='mapping_miss', a SystemBlock contains the
// mapping_miss focusing instruction (line 399-401 in discovery_v2.go).
func TestScenario_122_NarrowDiscovery_MappingMiss_FewerToolsAndDifferentPrompt(t *testing.T) {
	d, sender, _, _, runs, _ := mkDiscovery()
	sender.responses = []*anthropic.MessagesResponse{
		commitArtifactResponse(simpleCommittedArtifact(), 50, 30),
	}
	payload, _ := json.Marshal(map[string]any{"field": "skin_type", "inbox_item_id": "i1"})
	if _, err := d.Discover(context.Background(), "t-1", "mapping_miss", payload); err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(sender.requests) < 1 {
		t.Fatal("no requests sent")
	}
	// Check dynamic SystemBlock (last one) carries the mapping_miss instruction.
	blocks := sender.requests[0].SystemBlocks
	if len(blocks) < 2 {
		t.Fatalf("SystemBlocks = %d, want >= 2 (static + dynamic)", len(blocks))
	}
	dynamic := blocks[len(blocks)-1].Text
	if !strings.Contains(dynamic, "mapping_miss") && !strings.Contains(dynamic, "mapping miss") {
		t.Errorf("dynamic SystemBlock missing mapping_miss focus: %q", dynamic)
	}
	if !strings.Contains(strings.ToLower(dynamic), "offending field") &&
		!strings.Contains(strings.ToLower(dynamic), "re-discovering") {
		t.Errorf("dynamic block missing narrow-focus language: %q", dynamic)
	}
	// agent_runs trigger captured correctly.
	if len(runs.starts) != 1 || runs.starts[0].Trigger != "mapping_miss" {
		t.Errorf("agent_runs trigger = %+v, want mapping_miss", runs.starts)
	}
}

// TestScenario_121and123_MappingMiss_ActionLogAndArtifactRefetch verifies:
// «apply_v2 натыкается на rule → wraps в mappingMissErr → action_log пишет
// mapping_miss, триггерит discovery.Discover(trigger='mapping_miss')» (sc 121)
// AND
// «После narrow discovery apply_v2 re-fetch'ит artifact и продолжает loop» (sc 123).
//
// Setup: two inbox rows. The first row's artifact rule fails → mapping_miss
// → discovery commits a fresh artifact (without the bad rule) → second row
// applies cleanly using the refreshed artifact.
func TestScenario_121and123_MappingMiss_ActionLogAndArtifactRefetch(t *testing.T) {
	sender := &fakeAgentSender{}
	inbox := newFakeInbox()
	// Pre-seed a "bad" artifact whose unknown master column will trip
	// every apply attempt.
	badArtifact := &domain.MappingArtifactV3{
		Version: 3,
		Branches: []domain.VerticalBranch{{
			Vertical: "cosmetics",
			FieldMap: []domain.FieldMappingRule{
				{From: "title", To: "master.name"},
				{From: "vendor", To: "master.brand"},
				{From: "variants[0].sku", To: "master.sku"},
				{From: "vendor", To: "master.bogus_column"}, // mapping_miss
			},
		}},
	}
	artifact := &capturingArtifact{art: badArtifact}
	runs := newFakeAgentRunsPort()
	log := &fakeActionLog{}
	llog := logger.New("error")

	discovery := NewDiscoveryV2(sender, inbox, artifact, log, runs, llog)
	writer := newFakeWriter()
	seedCosmeticsAlias(writer)
	apply := NewApplyV2(inbox, artifact, writer, log, discovery, llog)

	// When discovery is triggered, it commits the "good" artifact (no bad rule).
	sender.responses = []*anthropic.MessagesResponse{
		commitArtifactResponse(simpleCommittedArtifact(), 100, 50),
	}

	inbox.items = []*domain.InboxItem{
		mkInbox("i1", "gid://shopify/Product/1", map[string]any{
			"title":        "Cream A",
			"vendor":       "Brand",
			"product_type": "Cream",
			"variants":     []any{map[string]any{"sku": "CA-1"}},
		}),
		mkInbox("i2", "gid://shopify/Product/2", map[string]any{
			"title":        "Cream B",
			"vendor":       "Brand",
			"product_type": "Cream",
			"variants":     []any{map[string]any{"sku": "CB-1"}},
		}),
	}

	res, err := apply.ApplyForTenant(context.Background(), "t-test")
	if err != nil {
		t.Fatalf("ApplyForTenant: %v", err)
	}

	// Sc 121: at least one mapping_miss recorded + action_log entry written.
	if res.MappingMisses < 1 {
		t.Errorf("mapping_misses = %d, want >= 1", res.MappingMisses)
	}
	var sawMappingMiss bool
	for _, e := range log.entries {
		if e.Action == "mapping_miss" {
			sawMappingMiss = true
			break
		}
	}
	if !sawMappingMiss {
		t.Error("sc 121: mapping_miss action_log entry not written when discovery wired")
	}
	// Discovery was triggered with the right trigger.
	if len(runs.starts) < 1 || runs.starts[0].Trigger != "mapping_miss" {
		t.Errorf("sc 121: agent_runs.Start trigger = %+v, want mapping_miss", runs.starts)
	}

	// Sc 123: after narrow discovery the apply re-fetched the (now good)
	// artifact and applied row #2. Expect at least one Applied.
	if res.Applied < 1 {
		t.Errorf("sc 123: applied = %d, want >= 1 (refetch should let row #2 apply)", res.Applied)
	}
	if len(artifact.saved) < 1 {
		t.Errorf("sc 123: artifact.Save not called by discovery — refetch path didn't fire")
	}
}

// TestScenario_124_MappingMiss_3PassCapOnDiscoveryTriggers verifies:
// «Если за один apply-run сработало больше 3 mapping_miss подряд — дальнейшие
// НЕ триггерят discovery (защита от storm'а)».
//
// Setup: 5 inbox rows all causing mapping_miss + discovery wired. Each
// discovery call returns the same bad artifact (so the next item also misses).
// After 3 triggers the cap (applyV2MaxMissesPerRun=3) kicks in — items 4+5
// still increment res.MappingMisses but do NOT call discovery.Discover.
func TestScenario_124_MappingMiss_3PassCapOnDiscoveryTriggers(t *testing.T) {
	sender := &fakeAgentSender{}
	inbox := newFakeInbox()
	badArtifact := &domain.MappingArtifactV3{
		Version: 3,
		Branches: []domain.VerticalBranch{{
			Vertical: "cosmetics",
			FieldMap: []domain.FieldMappingRule{
				{From: "title", To: "master.name"},
				{From: "vendor", To: "master.brand"},
				{From: "variants[0].sku", To: "master.sku"},
				{From: "vendor", To: "master.bogus_column"},
			},
		}},
	}
	artifact := &capturingArtifact{art: badArtifact}
	runs := newFakeAgentRunsPort()
	log := &fakeActionLog{}
	llog := logger.New("error")
	discovery := NewDiscoveryV2(sender, inbox, artifact, log, runs, llog)
	writer := newFakeWriter()
	seedCosmeticsAlias(writer)
	apply := NewApplyV2(inbox, artifact, writer, log, discovery, llog)

	// Discovery scripted to commit the SAME bad artifact 5 times — but only
	// 3 will ever be invoked due to the cap.
	for i := 0; i < 5; i++ {
		sender.responses = append(sender.responses, commitArtifactResponse(badArtifact, 50, 30))
	}

	// 5 items all causing mapping_miss.
	for i := 1; i <= 5; i++ {
		inbox.items = append(inbox.items, mkInbox(
			fmt.Sprintf("i%d", i),
			fmt.Sprintf("gid://shopify/Product/%d", i),
			map[string]any{
				"title":        fmt.Sprintf("Cream %d", i),
				"vendor":       "Brand",
				"product_type": "Cream",
				"variants":     []any{map[string]any{"sku": fmt.Sprintf("C-%d", i)}},
			}))
	}

	res, err := apply.ApplyForTenant(context.Background(), "t-test")
	if err != nil {
		t.Fatalf("ApplyForTenant: %v", err)
	}

	if res.MappingMisses != 5 {
		t.Errorf("mapping_misses = %d, want 5 (counter never stops)", res.MappingMisses)
	}
	if len(runs.starts) != 3 {
		t.Errorf("discovery.Discover called %d times, want exactly 3 (3-pass cap)", len(runs.starts))
	}
}

// TestScenario_111_FirstInstall_CascadesFromApply_WhenArtifactNil verifies:
// «apply_v2 видит artifact=NULL → cascade'ится в discovery.Discover(trigger='first_install')».
// Integration-style unit: apply_v2 + discovery + their shared artifact/inbox
// fakes wired together. capturingArtifact starts with art=nil; discovery
// commits an artifact mid-call; apply uses the returned artifact directly
// (per apply_v2.go:130) and applies the inbox row.
func TestScenario_111_FirstInstall_CascadesFromApply_WhenArtifactNil(t *testing.T) {
	sender := &fakeAgentSender{}
	inbox := newFakeInbox()
	artifact := &capturingArtifact{} // art = nil
	runs := newFakeAgentRunsPort()
	log := &fakeActionLog{}
	llog := logger.New("error")

	discovery := NewDiscoveryV2(sender, inbox, artifact, log, runs, llog)
	writer := newFakeWriter()
	seedCosmeticsAlias(writer)
	apply := NewApplyV2(inbox, artifact, writer, log, discovery, llog)

	// Discovery will commit this artifact when triggered.
	sender.responses = []*anthropic.MessagesResponse{
		commitArtifactResponse(simpleCommittedArtifact(), 100, 50),
	}

	// One inbox row matching the cosmetics branch the agent will commit.
	inbox.items = []*domain.InboxItem{
		mkInbox("i1", "gid://shopify/Product/1", map[string]any{
			"title":        "Hyaluronic Cream",
			"vendor":       "Brand",
			"product_type": "Cream",
			"variants":     []any{map[string]any{"sku": "HC-1"}},
		}),
	}

	res, err := apply.ApplyForTenant(context.Background(), "t-test")
	if err != nil {
		t.Fatalf("ApplyForTenant: %v", err)
	}
	if res.Applied != 1 {
		t.Errorf("applied = %d, want 1 (cascade discovery → apply)", res.Applied)
	}
	if len(artifact.saved) != 1 {
		t.Errorf("artifact saves = %d, want 1 (discovery committed once)", len(artifact.saved))
	}
	if len(runs.starts) != 1 || runs.starts[0].Trigger != "first_install" {
		t.Errorf("agent_runs trigger = %+v, want one start with first_install", runs.starts)
	}
}
