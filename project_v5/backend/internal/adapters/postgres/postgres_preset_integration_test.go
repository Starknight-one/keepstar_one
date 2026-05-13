//go:build integration

// Run with: TEST_DATABASE_URL=$DATABASE_URL go test -tags=integration ./internal/adapters/postgres/...
//
// Seeds a v5_presets row + v5_preset_versions row for the heybabes tenant
// (or any tenant present in the live catalog), exercises GetPublishedPreset
// and ListPublishedPresets against the live Neon, and cleans up via the
// ON DELETE CASCADE chain (deleting the v5_presets row drops the version).

package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/engine"
	"keepstar_v5/internal/engine/presets"
)

// pickHeybabesOrAnyTenantSlug favours heybabes (chunk-3 already exercises
// it) but falls back to whatever tenant the env happens to have. Tests
// stay portable.
func pickHeybabesOrAnyTenantSlug(t *testing.T, c *Client) string {
	t.Helper()
	ctx := context.Background()
	var slug string
	err := c.pool.QueryRow(ctx, `
		SELECT slug FROM catalog.tenants WHERE slug = 'heybabes' LIMIT 1
	`).Scan(&slug)
	if err == nil && slug != "" {
		return slug
	}
	// Fallback: any tenant.
	err = c.pool.QueryRow(ctx, `SELECT slug FROM catalog.tenants LIMIT 1`).Scan(&slug)
	if err != nil {
		t.Skipf("no tenants in this environment: %v", err)
	}
	return slug
}

// pickTenantSlugWithProducts is like pickHeybabesOrAnyTenantSlug but only
// returns a tenant that has at least minProducts non-deleted products.
// Used by the end-to-end pipeline test which needs real data to fan out.
func pickTenantSlugWithProducts(t *testing.T, c *Client, minProducts int) string {
	t.Helper()
	ctx := context.Background()
	var slug string
	// Prefer heybabes if it qualifies.
	err := c.pool.QueryRow(ctx, `
		SELECT t.slug
		FROM catalog.tenants t
		WHERE t.slug = 'heybabes'
		  AND (SELECT COUNT(*) FROM catalog.products p WHERE p.tenant_id = t.id AND p.deleted_at IS NULL) >= $1
		LIMIT 1
	`, minProducts).Scan(&slug)
	if err == nil && slug != "" {
		return slug
	}
	// Fallback: any tenant with enough products.
	err = c.pool.QueryRow(ctx, `
		SELECT t.slug
		FROM catalog.tenants t
		WHERE (SELECT COUNT(*) FROM catalog.products p WHERE p.tenant_id = t.id AND p.deleted_at IS NULL) >= $1
		ORDER BY t.created_at
		LIMIT 1
	`, minProducts).Scan(&slug)
	if err != nil {
		t.Skipf("no tenant with ≥%d products in this environment: %v", minProducts, err)
	}
	return slug
}

// seedProductCardPreset inserts the embedded product_card preset for the
// given tenant slug and returns the preset id. Test-only — production
// seeding will go through the canvas-microservice once Stream B lands.
func seedProductCardPreset(t *testing.T, c *Client, tenantSlug string, version int) string {
	t.Helper()
	ctx := context.Background()

	var tenantID string
	if err := c.pool.QueryRow(ctx,
		`SELECT id::text FROM catalog.tenants WHERE slug = $1`, tenantSlug,
	).Scan(&tenantID); err != nil {
		t.Fatalf("resolve tenant %q: %v", tenantSlug, err)
	}

	// Use a unique name per test to avoid UNIQUE(tenant_id, name) collision
	// when tests run back-to-back without cleanup completing.
	presetName := "product_card_test_" + t.Name()
	var presetID string
	err := c.pool.QueryRow(ctx, `
		INSERT INTO v5_presets (tenant_id, name, category, entity_type, description, default_replicate)
		VALUES ($1::uuid, $2, 'product', 'product', 'integration test seed', TRUE)
		RETURNING id::text
	`, tenantID, presetName).Scan(&presetID)
	if err != nil {
		t.Fatalf("insert preset header: %v", err)
	}
	t.Cleanup(func() {
		_, _ = c.pool.Exec(context.Background(), `DELETE FROM v5_presets WHERE id = $1::uuid`, presetID)
	})

	if _, err := c.pool.Exec(ctx, `
		INSERT INTO v5_preset_versions (preset_id, version, status, doc_json, published_at)
		VALUES ($1::uuid, $2, 'published', $3::jsonb, NOW())
	`, presetID, version, presets.ProductCardJSON); err != nil {
		t.Fatalf("insert preset version: %v", err)
	}

	t.Setenv("V5_TEST_PRESET_NAME_"+t.Name(), presetName)
	return presetName
}

func TestPresetAdapterGetPublished(t *testing.T) {
	c := setupClient(t)
	adapter := NewPresetAdapter(c)
	ctx := context.Background()

	tenantSlug := pickHeybabesOrAnyTenantSlug(t, c)
	presetName := seedProductCardPreset(t, c, tenantSlug, 1)

	preset, err := adapter.GetPublishedPreset(ctx, tenantSlug, presetName)
	if err != nil {
		t.Fatalf("GetPublishedPreset: %v", err)
	}
	if preset.Name != presetName {
		t.Errorf("name = %q, want %q", preset.Name, presetName)
	}
	if preset.Status != domain.PresetStatusPublished {
		t.Errorf("status = %q, want published", preset.Status)
	}
	if !preset.DefaultReplicate {
		t.Errorf("default_replicate must be true")
	}
	if preset.Version != 1 {
		t.Errorf("version = %d, want 1", preset.Version)
	}
	if preset.PublishedAt == nil {
		t.Errorf("published_at must be set after seed")
	}
	// DocumentJSON must round-trip through engine.Document, with the
	// outer card replicate marker preserved.
	if len(preset.DocumentJSON) == 0 {
		t.Fatalf("DocumentJSON empty")
	}
	var doc engine.Document
	if err := json.Unmarshal(preset.DocumentJSON, &doc); err != nil {
		t.Fatalf("doc unmarshal: %v", err)
	}
	if len(doc.Children) != 1 || engine.NodeID(doc.Children[0]) != "card" {
		t.Errorf("doc structure lost on round-trip: %+v", doc.Children)
	}
	if rep, _ := doc.Children[0]["replicate"].(bool); !rep {
		t.Errorf("replicate marker lost on round-trip")
	}
}

func TestPresetAdapterGetPublished_NotFound(t *testing.T) {
	c := setupClient(t)
	adapter := NewPresetAdapter(c)
	ctx := context.Background()

	tenantSlug := pickHeybabesOrAnyTenantSlug(t, c)

	_, err := adapter.GetPublishedPreset(ctx, tenantSlug, "preset_that_does_not_exist_zzzzz")
	if !errors.Is(err, domain.ErrPresetNotFound) {
		t.Errorf("err = %v, want ErrPresetNotFound", err)
	}
}

func TestPresetAdapterListPublished(t *testing.T) {
	c := setupClient(t)
	adapter := NewPresetAdapter(c)
	ctx := context.Background()

	tenantSlug := pickHeybabesOrAnyTenantSlug(t, c)
	seededName := seedProductCardPreset(t, c, tenantSlug, 1)

	list, err := adapter.ListPublishedPresets(ctx, tenantSlug)
	if err != nil {
		t.Fatalf("ListPublishedPresets: %v", err)
	}
	// Find our seeded preset in the list (other tenants may share the slug
	// but the query filters by tenant_id, so any other rows are pre-existing
	// presets for the same tenant — fine, just look for ours).
	found := false
	for _, p := range list {
		if p.Name == seededName {
			found = true
			if p.Status != domain.PresetStatusPublished {
				t.Errorf("listed status = %q", p.Status)
			}
			if len(p.DocumentJSON) == 0 {
				t.Errorf("listed DocumentJSON empty")
			}
			break
		}
	}
	if !found {
		t.Errorf("seeded preset %q not in list of %d", seededName, len(list))
	}
}
