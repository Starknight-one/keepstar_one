package operations

import (
	"context"
	"encoding/json"
	"testing"

	"keepstar_v5/internal/domain"
)

// stubTool is a canned tools.Tool for wrapper mapping tests.
type stubTool struct {
	def      domain.ToolDefinition
	result   *domain.ToolResult
	err      error
	lastCtx  domain.ToolContext
	lastArgs map[string]interface{}
}

func (s *stubTool) Definition() domain.ToolDefinition { return s.def }
func (s *stubTool) Execute(_ context.Context, toolCtx domain.ToolContext, input map[string]interface{}) (*domain.ToolResult, error) {
	s.lastCtx = toolCtx
	s.lastArgs = input
	return s.result, s.err
}

func wrapStub(res *domain.ToolResult) (*LegacyExecutor, *stubTool) {
	tool := &stubTool{
		def:    domain.ToolDefinition{Name: "catalog_search", Description: "d", InputSchema: map[string]any{"type": "object"}},
		result: res,
	}
	return WrapCatalogSearch(tool, nil), tool
}

// TestWrapLegacySummariesByteExact is the §4.1 byte-equality gate: today's
// legacy Summary strings survive the wrap UNCHANGED so conversation-prefix
// caches stay warm, and the two known "empty:" strings map to OutcomeEmpty
// at the wrapper — the last place that prefix is ever read.
func TestWrapLegacySummariesByteExact(t *testing.T) {
	cases := []struct {
		name    string
		result  *domain.ToolResult
		outcome domain.OpOutcome
		isError bool
	}{
		{
			name:    "catalog ok",
			result:  &domain.ToolResult{Content: "ok: found 12 products"},
			outcome: domain.OutcomeOK,
		},
		{
			name:    "catalog empty",
			result:  &domain.ToolResult{Content: "empty: 0 results, previous data preserved"},
			outcome: domain.OutcomeEmpty,
		},
		{
			name:    "state filter ok",
			result:  &domain.ToolResult{Content: "ok: filtered 3 items from 12 products"},
			outcome: domain.OutcomeOK,
		},
		{
			name:    "state filter empty",
			result:  &domain.ToolResult{Content: "empty: 0 results from 8 products, data preserved"},
			outcome: domain.OutcomeEmpty,
		},
		{
			name:    "tool error",
			result:  &domain.ToolResult{Content: "error: mode is required", IsError: true},
			outcome: domain.OutcomeError,
			isError: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ex, _ := wrapStub(tc.result)
			res, err := ex.Execute(context.Background(), domain.OperationContext{}, map[string]any{})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if res.Summary != tc.result.Content {
				t.Errorf("Summary %q, want byte-exact %q", res.Summary, tc.result.Content)
			}
			if res.Outcome != tc.outcome {
				t.Errorf("Outcome %q, want %q", res.Outcome, tc.outcome)
			}
			bridged := res.ToToolResult("tool-1")
			if bridged.Content != tc.result.Content {
				t.Errorf("bridged Content %q, want %q", bridged.Content, tc.result.Content)
			}
			if bridged.IsError != tc.isError {
				t.Errorf("bridged IsError %v, want %v", bridged.IsError, tc.isError)
			}
		})
	}
}

func TestWrapLegacyPassesContextAndInputVerbatim(t *testing.T) {
	ex, tool := wrapStub(&domain.ToolResult{Content: "ok"})
	input := map[string]any{"vector_query": "serums"}
	_, err := ex.Execute(context.Background(), domain.OperationContext{
		SessionID:  "sess-1",
		TenantSlug: "acme",
		TurnID:     "turn-1",
		Mode:       domain.ModeStorefront,
		Role:       domain.RoleVisitor,
	}, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := domain.ToolContext{SessionID: "sess-1", TenantSlug: "acme", TurnID: "turn-1"}
	if tool.lastCtx != want {
		t.Errorf("ToolContext %+v, want %+v", tool.lastCtx, want)
	}
	if tool.lastArgs["vector_query"] != "serums" {
		t.Errorf("input not passed verbatim: %#v", tool.lastArgs)
	}
}

func TestWrapLegacyIsPassthrough(t *testing.T) {
	ex, _ := wrapStub(&domain.ToolResult{Content: "ok"})
	if !ex.Passthrough() {
		t.Error("legacy wraps must be passthrough (no registry validation)")
	}
}

// TestWrapCatalogSearchTemplateMatchesDefinition pins the wrap's template
// row to the tool's Definition() — the LLM-facing bytes cannot drift.
func TestWrapCatalogSearchTemplateMatchesDefinition(t *testing.T) {
	tool := &stubTool{def: domain.ToolDefinition{
		Name:        "catalog_search",
		Description: "the exact legacy description",
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{"vector_query": map[string]any{"type": "string"}}},
	}}
	ex := WrapCatalogSearch(tool, nil)

	def := ex.Template().ToolDefinition()
	wantJSON, _ := json.Marshal(tool.Definition())
	gotJSON, _ := json.Marshal(def)
	if string(gotJSON) != string(wantJSON) {
		t.Errorf("wrapped definition drifted:\n got %s\nwant %s", gotJSON, wantJSON)
	}

	row := ex.TemplateRow()
	if !row.AutoEnabled || row.Kind != domain.KindQuery {
		t.Errorf("catalog_search row must be auto-enabled kind=query, got %+v", row)
	}
	// R16: onboarding never sees catalog_search.
	for _, m := range row.Modes {
		if m == domain.ModeOnboarding {
			t.Error("catalog_search must not carry the onboarding mode (R16)")
		}
	}
}

// digestOnce serves a fixed digest and counts calls.
type digestOnce struct {
	digest *domain.CatalogDigest
	calls  int
}

func (d *digestOnce) BuildCatalogDigest(context.Context, string) (*domain.CatalogDigest, error) {
	d.calls++
	return d.digest, nil
}

func TestWrapCatalogSearchSpecForTenantDerivesSchema(t *testing.T) {
	tool := &stubTool{def: domain.ToolDefinition{
		Name:        "catalog_search",
		Description: "d",
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
	}}
	digests := &digestOnce{digest: &domain.CatalogDigest{
		TotalProducts: 10,
		SharedFilters: []domain.DigestSharedFilter{{Key: "beds", Values: []string{"1", "2", "3"}}},
	}}
	ex := WrapCatalogSearch(tool, digests)

	spec, err := ex.SpecForTenant(context.Background(), domain.Tenant{ID: "tnt-1"}, nil)
	if err != nil {
		t.Fatalf("SpecForTenant: %v", err)
	}
	props := spec.InputSchema["properties"].(map[string]interface{})
	filters := props["filters"].(map[string]interface{})["properties"].(map[string]interface{})
	if _, ok := filters["beds"]; !ok {
		t.Errorf("digest-derived filter missing from per-tenant schema: %#v", filters)
	}

	// Empty digest → fail-open to the static template schema.
	exEmpty := WrapCatalogSearch(tool, &digestOnce{digest: &domain.CatalogDigest{TotalProducts: 0}})
	specEmpty, _ := exEmpty.SpecForTenant(context.Background(), domain.Tenant{ID: "tnt-1"}, nil)
	got, _ := json.Marshal(specEmpty.InputSchema)
	want, _ := json.Marshal(tool.Definition().InputSchema)
	if string(got) != string(want) {
		t.Errorf("empty digest must keep the static schema:\n got %s\nwant %s", got, want)
	}
}
