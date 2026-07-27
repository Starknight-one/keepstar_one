//go:build integration

// Run with: TEST_DATABASE_URL=$DATABASE_URL go test -tags=integration ./internal/adapters/postgres/...
//
// Exercises the operation-plane adapter against a live DB: template upsert
// idempotency (same id on re-seed), FTS SearchLibrary (no embedding → the
// degraded path every keyless boot takes), the tenant-instance CRUD cycle
// incl. config_schema validation, and the v5_operation_runs audit append.
// All rows are test-prefixed and cleaned up.

package postgres

import (
	"context"
	"os"
	"testing"

	"keepstar_v5/internal/domain"
)

func setupOperationsClient(t *testing.T) (*Client, *OperationAdapter) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration test")
	}
	ctx := context.Background()
	c, err := NewClient(ctx, url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(c.Close)
	// Operation migrations ALTER v5_chat_sessions → state must run first
	// (same order as cmd/server/main.go).
	if err := c.RunStateMigrations(ctx); err != nil {
		t.Fatalf("migrate state: %v", err)
	}
	if err := c.RunOperationMigrations(ctx); err != nil {
		t.Fatalf("migrate operations: %v", err)
	}
	return c, NewOperationAdapter(c)
}

func testTenantUUID(t *testing.T, c *Client) string {
	t.Helper()
	var id string
	if err := c.pool.QueryRow(context.Background(), `SELECT gen_random_uuid()::text`).Scan(&id); err != nil {
		t.Fatalf("gen uuid: %v", err)
	}
	return id
}

func opTestTemplate(name string) domain.OperationTemplate {
	return domain.OperationTemplate{
		Name:        name,
		Kind:        domain.KindQuery,
		Title:       "Integration test operation",
		Description: "integration-test template searching zorbulite fixtures",
		Card:        map[string]any{"does": "nothing real"},
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"query": map[string]any{"type": "string"}},
		},
		ConfigSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"source": map[string]any{"type": "string", "enum": []any{"catalog", "entity"}}},
		},
		Modes:   []domain.PipelineMode{domain.ModeStorefront, domain.ModeCRM},
		Agent:   domain.AgentData,
		MinRole: domain.RoleVisitor,
	}
}

func TestOperationAdapterTemplateSeedAndSearch(t *testing.T) {
	c, a := setupOperationsClient(t)
	ctx := context.Background()
	const name = "itest_zorbulite_query"
	t.Cleanup(func() {
		_, _ = c.pool.Exec(context.Background(), `DELETE FROM v5_operations WHERE name = $1`, name)
	})

	id1, err := a.UpsertTemplate(ctx, opTestTemplate(name), nil)
	if err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	// Idempotency (§3.1 boot seed re-runs every boot): same row id.
	tmpl := opTestTemplate(name)
	tmpl.Description = "integration-test template searching zorbulite fixtures v2"
	id2, err := a.UpsertTemplate(ctx, tmpl, nil)
	if err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	if id1 != id2 {
		t.Fatalf("upsert not idempotent: id %s != %s", id1, id2)
	}

	all, err := a.ListTemplates(ctx)
	if err != nil {
		t.Fatalf("list templates: %v", err)
	}
	var found *domain.OperationTemplate
	for i := range all {
		if all[i].Name == name {
			found = &all[i]
		}
	}
	if found == nil {
		t.Fatalf("ListTemplates missing %s", name)
	}
	if found.Description != tmpl.Description {
		t.Errorf("description not updated: %q", found.Description)
	}
	if len(found.Modes) != 2 || found.Modes[0] != domain.ModeStorefront {
		t.Errorf("modes round-trip broken: %v", found.Modes)
	}
	if found.Card["does"] != "nothing real" {
		t.Errorf("card round-trip broken: %v", found.Card)
	}

	// FTS fallback (nil embedding — the keyless-boot path).
	hits, err := a.SearchLibrary(ctx, nil, "zorbulite", nil, 5)
	if err != nil {
		t.Fatalf("search library: %v", err)
	}
	if len(hits) == 0 || hits[0].Name != name {
		t.Fatalf("FTS search missed %s: %+v", name, hits)
	}
	// kinds filter excludes.
	none, err := a.SearchLibrary(ctx, nil, "zorbulite", []string{"notify"}, 5)
	if err != nil {
		t.Fatalf("search library kinds: %v", err)
	}
	if len(none) != 0 {
		t.Fatalf("kinds filter leaked: %+v", none)
	}
}

func TestOperationAdapterInstanceCycleAndRuns(t *testing.T) {
	c, a := setupOperationsClient(t)
	ctx := context.Background()
	const name = "itest_instance_query"
	tenantID := testTenantUUID(t, c)
	t.Cleanup(func() {
		_, _ = c.pool.Exec(context.Background(), `DELETE FROM v5_tenant_operations WHERE tenant_id = $1::uuid`, tenantID)
		_, _ = c.pool.Exec(context.Background(), `DELETE FROM v5_operation_runs WHERE tenant_id = $1::uuid`, tenantID)
		_, _ = c.pool.Exec(context.Background(), `DELETE FROM v5_operations WHERE name = $1`, name)
	})
	if _, err := a.UpsertTemplate(ctx, opTestTemplate(name), nil); err != nil {
		t.Fatalf("seed template: %v", err)
	}

	// Config violating config_schema → error (validated on write, §3.1).
	if _, err := a.EnableOperation(ctx, tenantID, name, "itest_search", map[string]any{"source": "bogus"}); err == nil {
		t.Fatalf("invalid config accepted")
	}
	inst, err := a.EnableOperation(ctx, tenantID, name, "itest_search", map[string]any{"source": "catalog"})
	if err != nil {
		t.Fatalf("enable: %v", err)
	}
	if inst.Config["source"] != "catalog" || !inst.Enabled {
		t.Fatalf("instance shape wrong: %+v", inst)
	}
	// Idempotent re-enable (applier retry contract) → same instance id.
	again, err := a.EnableOperation(ctx, tenantID, name, "itest_search", map[string]any{"source": "entity"})
	if err != nil {
		t.Fatalf("re-enable: %v", err)
	}
	if again.ID != inst.ID {
		t.Fatalf("re-enable created a second row: %s != %s", again.ID, inst.ID)
	}

	if err := a.UpdateInstanceConfig(ctx, tenantID, "itest_search", map[string]any{"source": "catalog"}); err != nil {
		t.Fatalf("update config: %v", err)
	}
	if err := a.UpdateInstanceConfig(ctx, tenantID, "itest_missing", map[string]any{}); err == nil {
		t.Fatalf("update of unknown instance accepted")
	}

	enabled, err := a.ListEnabled(ctx, tenantID)
	if err != nil {
		t.Fatalf("list enabled: %v", err)
	}
	if len(enabled) != 1 || enabled[0].Name != "itest_search" || enabled[0].Config["source"] != "catalog" {
		t.Fatalf("list enabled wrong: %+v", enabled)
	}

	if err := a.DisableOperation(ctx, tenantID, "itest_search"); err != nil {
		t.Fatalf("disable: %v", err)
	}
	enabled, err = a.ListEnabled(ctx, tenantID)
	if err != nil {
		t.Fatalf("list enabled after disable: %v", err)
	}
	if len(enabled) != 0 {
		t.Fatalf("disabled instance still listed: %+v", enabled)
	}

	// Audit append — and the fail-loud guard for missing tenant identity.
	run := domain.OperationRun{
		TenantID: tenantID, SessionID: "itest-session", Operation: "itest_search",
		Kind: domain.KindQuery, Mode: domain.ModeStorefront, ActorRole: domain.RoleVisitor,
		Input: map[string]any{"query": "zorbulite"}, Outcome: domain.OutcomeOK, LatencyMS: 3,
	}
	if err := a.Append(ctx, run); err != nil {
		t.Fatalf("append run: %v", err)
	}
	var got int
	if err := c.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM v5_operation_runs WHERE tenant_id = $1::uuid AND operation = 'itest_search' AND outcome = 'ok'
	`, tenantID).Scan(&got); err != nil {
		t.Fatalf("count runs: %v", err)
	}
	if got != 1 {
		t.Fatalf("run row count = %d, want 1", got)
	}
	run.TenantID = ""
	if err := a.Append(ctx, run); err == nil {
		t.Fatalf("append without tenant id accepted")
	}
}
