package usecases

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/operations"
	"keepstar_v5/internal/ports"
	"keepstar_v5/internal/tools"
)

// ─── fakes ───────────────────────────────────────────────────────────────

type fakeLLM struct {
	resp *domain.LLMResponse
	err  error
	last struct {
		systemPrompt string
		messages     []domain.LLMMessage
		tools        []domain.ToolDefinition
		cfg          ports.CacheConfig
	}
}

func (f *fakeLLM) ChatWithToolsCached(_ context.Context, sys string, msgs []domain.LLMMessage, tdefs []domain.ToolDefinition, cfg ports.CacheConfig) (*domain.LLMResponse, error) {
	f.last.systemPrompt = sys
	f.last.messages = msgs
	f.last.tools = tdefs
	f.last.cfg = cfg
	if f.err != nil {
		return nil, f.err
	}
	if f.resp != nil {
		return f.resp, nil
	}
	return &domain.LLMResponse{StopReason: "end_turn"}, nil
}

func (f *fakeLLM) CountInputTokens(_ context.Context, _ string, _ []domain.LLMMessage, _ []domain.ToolDefinition) (int, error) {
	return 0, nil
}

type fakeCatalog struct {
	tenant   *domain.Tenant
	products []domain.Product
	digest   *domain.CatalogDigest
}

func (c *fakeCatalog) GetTenantBySlug(_ context.Context, slug string) (*domain.Tenant, error) {
	if c.tenant != nil {
		return c.tenant, nil
	}
	return &domain.Tenant{ID: "tnt-1", Slug: slug}, nil
}
func (c *fakeCatalog) ListProducts(_ context.Context, _ string, _ ports.ProductFilter) ([]domain.Product, int, error) {
	return c.products, len(c.products), nil
}
func (c *fakeCatalog) GetProduct(_ context.Context, _, _ string) (*domain.Product, error) {
	return nil, domain.ErrProductNotFound
}
func (c *fakeCatalog) BuildCatalogDigest(_ context.Context, _ string) (*domain.CatalogDigest, error) {
	return c.digest, nil
}
func (c *fakeCatalog) VectorSearch(_ context.Context, _ string, _ []float32, _ int, _ *ports.VectorFilter) ([]domain.Product, error) {
	return c.products, nil
}
func (c *fakeCatalog) SearchProjection(_ context.Context, _ string, _ []float32, _ ports.ProductFilter, _ int) ([]domain.Product, error) {
	return c.products, nil
}

// ─── helpers ─────────────────────────────────────────────────────────────

func setupAgent1(t *testing.T, products []domain.Product, llmResp *domain.LLMResponse) (*Agent1Execute, *fakeLLM, *mockStatePort, *fakeCatalog) {
	t.Helper()
	state := newMockStatePort()
	if _, err := state.CreateState(context.Background(), "sess-1"); err != nil {
		t.Fatalf("CreateState: %v", err)
	}
	state.states["sess-1"].Current.Data = domain.StateData{Products: products}
	state.states["sess-1"].Current.Meta = domain.StateMeta{
		ProductCount: len(products),
		Aliases:      map[string]string{"tenant_slug": "acme"},
	}

	cat := &fakeCatalog{
		tenant:   &domain.Tenant{ID: "tnt-1", Slug: "acme"},
		products: products,
		digest:   &domain.CatalogDigest{TotalProducts: 100},
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	registry := newLegacyOpsRegistry(state, cat, log)

	llm := &fakeLLM{resp: llmResp}
	cache := NewAgent1PromptCache(cat)

	uc := NewAgent1Execute(llm, state, cat, registry, cache, log)
	return uc, llm, state, cat
}

// newLegacyOpsRegistry builds an operation registry over the three legacy
// Agent1 tool wraps — the M1 production wiring minus store/runs (unit
// tests need neither instances nor audit persistence).
func newLegacyOpsRegistry(state ports.StatePort, cat *fakeCatalog, log *slog.Logger) *operations.Registry {
	reg := operations.NewRegistry(operations.RegistryConfig{Tenants: cat, Log: log})
	reg.RegisterExecutor(domain.KindQuery, operations.WrapCatalogSearch(tools.NewCatalogSearchTool(state, cat, nil), cat))
	reg.RegisterExecutor(domain.KindInternal, operations.WrapStateFilter(tools.NewStateFilterTool(state)))
	reg.RegisterExecutor(domain.KindInternal, operations.WrapHistoryLookup(tools.NewHistoryLookupTool(state)))
	return reg
}

// ─── tests ───────────────────────────────────────────────────────────────

func TestAgent1DeterministicGuardFires(t *testing.T) {
	products := []domain.Product{
		{ID: "p1", Name: "X", Brand: "COSRX"},
		{ID: "p2", Name: "Y", Brand: "MEDI-PEEL"},
	}
	uc, llm, _, _ := setupAgent1(t, products, nil)

	resp, err := uc.Execute(context.Background(), Agent1ExecuteRequest{
		SessionID:  "sess-1",
		TenantSlug: "acme",
		UserQuery:  "только COSRX",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Bypassed {
		t.Errorf("expected Bypassed=true on 'только COSRX' with loaded products")
	}
	if resp.ToolName != "_internal_state_filter" {
		t.Errorf("ToolName=%q, want _internal_state_filter", resp.ToolName)
	}
	if llm.last.systemPrompt != "" {
		t.Errorf("LLM should NOT be called on deterministic guard; got systemPrompt of len %d", len(llm.last.systemPrompt))
	}
}

func TestAgent1DeterministicGuardSkippedOnStyleRequest(t *testing.T) {
	products := []domain.Product{{ID: "p1", Name: "X"}, {ID: "p2", Name: "Y"}}
	uc, llm, _, _ := setupAgent1(t, products, &domain.LLMResponse{StopReason: "end_turn"})

	resp, err := uc.Execute(context.Background(), Agent1ExecuteRequest{
		SessionID:  "sess-1",
		TenantSlug: "acme",
		UserQuery:  "убери цену", // style request — must NOT trigger the guard
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Bypassed {
		t.Errorf("style request must NOT trigger deterministic guard")
	}
	if llm.last.systemPrompt == "" {
		t.Error("expected LLM to be called on style request")
	}
}

func TestAgent1DeterministicGuardSkippedWhenNoData(t *testing.T) {
	uc, llm, _, _ := setupAgent1(t, nil, &domain.LLMResponse{StopReason: "end_turn"})

	resp, err := uc.Execute(context.Background(), Agent1ExecuteRequest{
		SessionID:  "sess-1",
		TenantSlug: "acme",
		UserQuery:  "только COSRX", // filter trigger but no data → must go via LLM (catalog_search)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Bypassed {
		t.Errorf("guard must NOT fire when no products are loaded")
	}
	if llm.last.systemPrompt == "" {
		t.Error("expected LLM to be called when data is empty")
	}
}

func TestAgent1LLMCallsCatalogSearch(t *testing.T) {
	llmResp := &domain.LLMResponse{
		StopReason: "tool_use",
		ToolCalls: []domain.ToolCall{
			{ID: "tool-1", Name: "catalog_search", Input: map[string]interface{}{"vector_query": "сыворотки"}},
		},
		Usage: domain.LLMUsage{InputTokens: 100, OutputTokens: 20, CostUSD: 0.001},
	}
	uc, llm, _, _ := setupAgent1(t, nil, llmResp)
	// Seed the catalog so catalog_search will return something.
	uc.registry = func() ports.OperationRegistry {
		state := newMockStatePort()
		if _, err := state.CreateState(context.Background(), "sess-1"); err != nil {
			t.Fatalf("CreateState: %v", err)
		}
		state.states["sess-1"].Current.Meta = domain.StateMeta{Aliases: map[string]string{"tenant_slug": "acme"}}
		// rebind via uc.state below
		uc.state = state
		cat := &fakeCatalog{
			tenant:   &domain.Tenant{ID: "tnt-1", Slug: "acme"},
			products: []domain.Product{{ID: "p1", Name: "Hyaluronic Serum"}},
		}
		log := slog.New(slog.NewTextHandler(io.Discard, nil))
		return newLegacyOpsRegistry(state, cat, log)
	}()

	resp, err := uc.Execute(context.Background(), Agent1ExecuteRequest{
		SessionID:  "sess-1",
		TenantSlug: "acme",
		UserQuery:  "покажи сыворотки",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Bypassed {
		t.Error("guard must NOT fire on a fresh search query")
	}
	if resp.ToolName != "catalog_search" {
		t.Errorf("ToolName=%q, want catalog_search", resp.ToolName)
	}
	if resp.ProductsFound != 1 {
		t.Errorf("ProductsFound=%d, want 1", resp.ProductsFound)
	}
	if llm.last.cfg.ToolChoice != "auto" {
		t.Errorf("expected ToolChoice=auto for Agent1, got %q", llm.last.cfg.ToolChoice)
	}
	if llm.last.cfg.CacheTools {
		t.Errorf("CacheTools must be false for Agent1 (below 2048-token minimum)")
	}
}

func TestAgent1LLMDeclinesTool(t *testing.T) {
	llmResp := &domain.LLMResponse{
		StopReason: "end_turn",
		Text:       "I'll just adjust the display.",
		Usage:      domain.LLMUsage{InputTokens: 50, OutputTokens: 10, CostUSD: 0.0005},
	}
	uc, _, state, _ := setupAgent1(t, []domain.Product{{ID: "p1"}}, llmResp)

	resp, err := uc.Execute(context.Background(), Agent1ExecuteRequest{
		SessionID:  "sess-1",
		TenantSlug: "acme",
		UserQuery:  "поменяй цвет",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ToolName != "" {
		t.Errorf("expected empty ToolName when LLM declines, got %q", resp.ToolName)
	}
	// Conversation history should still record the user message even when no
	// tool fired — the bare user turn lets the next Agent1 call see context.
	hist := state.states["sess-1"].ConversationHistory
	if len(hist) != 1 || hist[0].Role != "user" || hist[0].Content != "поменяй цвет" {
		t.Errorf("expected single user msg in history, got %+v", hist)
	}
}

// failingRegistry registers a tool whose Execute always errors out, to test
// the retry-with-empty-input path. The retry uses the same registry, so we
// also count the calls to confirm only one retry happens.
type countingTool struct {
	def      domain.ToolDefinition
	calls    int
	errFirst bool
}

func (c *countingTool) Definition() domain.ToolDefinition { return c.def }
func (c *countingTool) Execute(_ context.Context, _ domain.ToolContext, _ map[string]interface{}) (*domain.ToolResult, error) {
	c.calls++
	if c.errFirst && c.calls == 1 {
		return nil, errors.New("simulated panic-recovered error")
	}
	return &domain.ToolResult{Content: "ok-after-retry"}, nil
}

func TestAgent1ToolRetryWithEmptyInput(t *testing.T) {
	state := newMockStatePort()
	if _, err := state.CreateState(context.Background(), "sess-1"); err != nil {
		t.Fatalf("CreateState: %v", err)
	}
	state.states["sess-1"].Current.Meta.Aliases = map[string]string{"tenant_slug": "acme"}

	llmResp := &domain.LLMResponse{
		StopReason: "tool_use",
		ToolCalls: []domain.ToolCall{
			{ID: "tool-1", Name: "catalog_search", Input: map[string]interface{}{"vector_query": "x"}},
		},
	}
	llm := &fakeLLM{resp: llmResp}
	cat := &fakeCatalog{tenant: &domain.Tenant{ID: "tnt-1", Slug: "acme"}}

	failing := &countingTool{
		def: domain.ToolDefinition{
			Name:        "catalog_search",
			Description: "stub",
			InputSchema: map[string]interface{}{"type": "object"},
		},
		errFirst: true,
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	registry := operations.NewRegistry(operations.RegistryConfig{Tenants: cat, Log: log})
	registry.RegisterExecutor(domain.KindQuery, operations.WrapLegacyTool(failing, domain.OperationTemplate{
		Name:        "catalog_search",
		Kind:        domain.KindQuery,
		Title:       "stub",
		Description: "stub",
		InputSchema: failing.def.InputSchema,
		Modes:       []domain.PipelineMode{domain.ModeStorefront, domain.ModeCRM},
		Agent:       domain.AgentData,
		MinRole:     domain.RoleVisitor,
		AutoEnabled: true,
	}))

	cache := NewAgent1PromptCache(cat)
	uc := NewAgent1Execute(llm, state, cat, registry, cache, log)

	resp, err := uc.Execute(context.Background(), Agent1ExecuteRequest{
		SessionID:  "sess-1",
		TenantSlug: "acme",
		UserQuery:  "покажи",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if failing.calls != 2 {
		t.Errorf("expected exactly 2 calls (one + retry), got %d", failing.calls)
	}
	if resp.ToolResult == nil || resp.ToolResult.Content != "ok-after-retry" {
		t.Errorf("expected retry result, got %+v", resp.ToolResult)
	}
}
