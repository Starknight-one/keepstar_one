//go:build integration && live

// Live end-to-end smoke for chunk 6b: Agent2Execute against a real
// Anthropic Haiku call + a real Neon database.
//
// Run with:
//
//	ANTHROPIC_API_KEY=$ANTHROPIC_API_KEY \
//	TEST_DATABASE_URL=$DATABASE_URL \
//	  go test -tags=live -v -count=1 \
//	    ./internal/adapters/postgres/... -run TestAgent2LiveSmoke
//
// Cost: ~$0.005-0.01 per run (one Haiku call, ~5K input + few hundred
// output tokens). Skips cleanly when either env var is absent.
//
// Lives in package postgres because it reuses the chunk-4/5 seed helpers
// (setupClient, pickTenantSlugWithProducts, seedComponentFromBytes,
// seedPresetFromBytes) — duplicating them into a new package would be
// busywork.

package postgres

import (
	"context"
	"io"
	"log/slog"
	"os"
	"testing"

	anthropicAdapter "keepstar_v5/internal/adapters/anthropic"
	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/engine/presets"
	"keepstar_v5/internal/ports"
	"keepstar_v5/internal/tools"
	"keepstar_v5/internal/usecases"
)

// TestAgent2LiveSmoke walks the full chunk-6b path:
//   1. Spin up Postgres adapters + Anthropic client.
//   2. Pick a tenant with ≥ 3 products, seed product_card preset +
//      price_rating / brand_badge components.
//   3. Create a session, populate state.Current.Data with 3 products
//      (simulates "Agent1 already ran" — Agent1 isn't implemented yet).
//   4. Call Agent2Execute("Show me 3 products from your catalog").
//   5. Assert: visual_assembly tool was called with reasonable params,
//      Document landed in state.Current.Template, cache write happened.
//   6. Run a SECOND call to verify cache READ (CacheReadInputTokens > 0).
func TestAgent2LiveSmoke(t *testing.T) {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		t.Skip("ANTHROPIC_API_KEY not set — skipping live Agent2 smoke")
	}
	if os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("TEST_DATABASE_URL not set — skipping live Agent2 smoke")
	}

	c := setupClient(t)
	ctx := context.Background()

	// 1. Adapters + ports.
	cat := NewCatalogAdapter(c)
	statePort := NewStateAdapter(c, slog.New(slog.NewTextHandler(io.Discard, nil)))
	presetPort := NewPresetAdapter(c)
	componentPort := NewComponentAdapter(c)
	fdPort := NewFieldDefinitionAdapter(c)
	llm := anthropicAdapter.NewClient(apiKey, "claude-haiku-4-5")

	// 2. Pick tenant + seed preset + components.
	tenantSlug := pickTenantSlugWithProducts(t, c, 3)
	tenant, err := cat.GetTenantBySlug(ctx, tenantSlug)
	if err != nil {
		t.Fatalf("GetTenantBySlug: %v", err)
	}

	products, _, err := cat.ListProducts(ctx, tenant.ID, ports.ProductFilter{Limit: 3})
	if err != nil {
		t.Fatalf("ListProducts: %v", err)
	}
	if len(products) < 3 {
		t.Fatalf("expected 3 products from %q, got %d", tenantSlug, len(products))
	}

	seedComponentFromBytes(t, c, tenantSlug, "price_rating", presets.ComponentPriceRatingJSON)
	seedComponentFromBytes(t, c, tenantSlug, "brand_badge", presets.ComponentBrandBadgeJSON)
	// Seed with EXACT name "product_card" (no test-name suffix) so the
	// LLM's preset choice from the prompt catalog matches what the DB has.
	seedPresetWithExactName(t, c, tenantSlug, "product_card", presets.ProductCardJSON)
	t.Logf("seeded preset %q for tenant %q", "product_card", tenantSlug)

	// 3. Build registry with visual_assembly wired to live ports.
	registry := tools.NewRegistry()
	registry.Register(tools.NewVisualAssemblyTool(statePort, presetPort, componentPort))

	// Build Agent2 use case.
	promptCache := usecases.NewPromptCache(fdPort, "product")
	agent2 := usecases.NewAgent2Execute(llm, statePort, registry, promptCache)

	// 4. Create session + pre-populate state.Current.Data (Agent1 stand-in).
	// session id must be a UUID — newTestSession inserts a v5_chat_sessions
	// row and returns the auto-generated id, with cleanup via t.Cleanup.
	sessionID := newTestSession(t, c)
	if _, err := statePort.CreateState(ctx, sessionID); err != nil {
		t.Fatalf("CreateState: %v", err)
	}

	if _, err := statePort.UpdateData(ctx, sessionID,
		domain.StateData{Products: products},
		domain.StateMeta{Count: len(products), ProductCount: len(products)},
		domain.DeltaInfo{
			Source: domain.SourceSystem, ActorID: "smoke", DeltaType: domain.DeltaTypeUpdate,
			Trigger: domain.TriggerSystem, Path: "current.data",
		}); err != nil {
		t.Fatalf("UpdateData: %v", err)
	}

	// 5. First Agent2 call — expect cache WRITE.
	resp1, err := agent2.Execute(ctx, usecases.Agent2ExecuteRequest{
		SessionID:  sessionID,
		TenantSlug: tenantSlug,
		UserQuery:  "Show me 3 products from your catalog.",
	})
	if err != nil {
		t.Fatalf("Agent2Execute: %v", err)
	}
	t.Logf("turn 1: %d tool calls, %d input tokens (cache_write=%d, cache_read=%d), %d output, $%.6f",
		len(resp1.ToolCalls),
		resp1.Usage.InputTokens,
		resp1.Usage.CacheCreationInputTokens,
		resp1.Usage.CacheReadInputTokens,
		resp1.Usage.OutputTokens,
		resp1.Usage.CostUSD,
	)

	if len(resp1.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d: %+v", len(resp1.ToolCalls), resp1.ToolCalls)
	}
	tc := resp1.ToolCalls[0]
	if tc.Name != "visual_assembly" {
		t.Errorf("tool name = %q, want visual_assembly", tc.Name)
	}
	preset, _ := tc.Input["preset"].(string)
	if !startsWith(preset, "product_") && preset != "" {
		t.Logf("warning: model picked unusual preset %q (not product_*)", preset)
	}
	if resp1.Document == nil {
		t.Errorf("Document not written to state.Current.Template")
	}
	if resp1.Usage.InputTokens == 0 {
		t.Errorf("InputTokens == 0; LLM call didn't actually run?")
	}
	// Cache write should have fired since CacheConfig has CacheTools+CacheSystem true.
	if resp1.Usage.CacheCreationInputTokens == 0 {
		t.Errorf("CacheCreationInputTokens == 0; cache_control not applied or prefix < threshold")
	}

	// 6. Second call — expect cache READ to fire.
	resp2, err := agent2.Execute(ctx, usecases.Agent2ExecuteRequest{
		SessionID:  sessionID,
		TenantSlug: tenantSlug,
		UserQuery:  "Show 5 instead.",
	})
	if err != nil {
		t.Fatalf("Agent2Execute (turn 2): %v", err)
	}
	t.Logf("turn 2: %d tool calls, %d input tokens (cache_write=%d, cache_read=%d), %d output, $%.6f",
		len(resp2.ToolCalls),
		resp2.Usage.InputTokens,
		resp2.Usage.CacheCreationInputTokens,
		resp2.Usage.CacheReadInputTokens,
		resp2.Usage.OutputTokens,
		resp2.Usage.CostUSD,
	)
	if resp2.Usage.CacheReadInputTokens == 0 {
		t.Errorf("turn 2: CacheReadInputTokens == 0; cache miss on subsequent turn")
	}
	t.Logf("two-turn total cost: $%.6f (V4 baseline ~$0.0021 for 2 turns)",
		resp1.Usage.CostUSD+resp2.Usage.CostUSD)
}

func startsWith(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

// seedPresetWithExactName inserts a preset header + version under the
// provided name, deleting any prior row with the same name first so the
// test is rerunnable. Cleanup via t.Cleanup. Used by the live smoke test
// where the LLM picks preset names from the prompt catalog and we need
// the DB to mirror that catalog exactly.
func seedPresetWithExactName(t *testing.T, c *Client, tenantSlug, name string, body []byte) {
	t.Helper()
	ctx := context.Background()

	var tenantID string
	if err := c.pool.QueryRow(ctx,
		`SELECT id::text FROM catalog.tenants WHERE slug = $1`, tenantSlug,
	).Scan(&tenantID); err != nil {
		t.Fatalf("resolve tenant %q: %v", tenantSlug, err)
	}

	// Drop any prior row with this name so reruns work.
	_, _ = c.pool.Exec(ctx,
		`DELETE FROM v5_presets WHERE tenant_id = $1::uuid AND name = $2`, tenantID, name)

	var presetID string
	if err := c.pool.QueryRow(ctx, `
		INSERT INTO v5_presets (tenant_id, name, category, entity_type, description, default_replicate)
		VALUES ($1::uuid, $2, 'product', 'product', 'live smoke seed', TRUE)
		RETURNING id::text
	`, tenantID, name).Scan(&presetID); err != nil {
		t.Fatalf("insert preset header: %v", err)
	}
	t.Cleanup(func() {
		_, _ = c.pool.Exec(context.Background(),
			`DELETE FROM v5_presets WHERE id = $1::uuid`, presetID)
	})
	if _, err := c.pool.Exec(ctx, `
		INSERT INTO v5_preset_versions (preset_id, version, status, doc_json, published_at)
		VALUES ($1::uuid, 1, 'published', $2::jsonb, NOW())
	`, presetID, body); err != nil {
		t.Fatalf("insert preset version: %v", err)
	}
}
