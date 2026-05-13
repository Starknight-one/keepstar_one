//go:build integration

// Run with: TEST_DATABASE_URL=$DATABASE_URL go test -tags=integration ./internal/adapters/postgres/...
//
// Mirror of postgres_preset_integration_test.go for the v5_components +
// v5_component_versions tables. Seeds a tiny single-atom component via a
// helper, exercises Get/NotFound/List, cleans up via ON DELETE CASCADE.

package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/engine"
)

// seedComponent inserts a component row + version for the given tenant
// with the provided doc_json body. Returns the unique-per-test component
// name. ON DELETE CASCADE drops the version row when the component row is
// deleted at test cleanup.
func seedComponent(t *testing.T, c *Client, tenantSlug, namePrefix string, docJSON []byte, version int) string {
	t.Helper()
	ctx := context.Background()

	var tenantID string
	if err := c.pool.QueryRow(ctx,
		`SELECT id::text FROM catalog.tenants WHERE slug = $1`, tenantSlug,
	).Scan(&tenantID); err != nil {
		t.Fatalf("resolve tenant %q: %v", tenantSlug, err)
	}

	compName := namePrefix + "_" + t.Name()
	var compID string
	if err := c.pool.QueryRow(ctx, `
		INSERT INTO v5_components (tenant_id, name, category, description)
		VALUES ($1::uuid, $2, 'atom', 'integration test seed')
		RETURNING id::text
	`, tenantID, compName).Scan(&compID); err != nil {
		t.Fatalf("insert component header: %v", err)
	}
	t.Cleanup(func() {
		_, _ = c.pool.Exec(context.Background(), `DELETE FROM v5_components WHERE id = $1::uuid`, compID)
	})

	if _, err := c.pool.Exec(ctx, `
		INSERT INTO v5_component_versions (component_id, version, status, doc_json, published_at)
		VALUES ($1::uuid, $2, 'published', $3::jsonb, NOW())
	`, compID, version, docJSON); err != nil {
		t.Fatalf("insert component version: %v", err)
	}
	return compName
}

func tinyComponentDocBytes(t *testing.T, rootID string) []byte {
	t.Helper()
	doc := engine.NewDocument()
	doc.Children = []engine.Node{
		{
			"type": "frame", "id": rootID,
			"children": []engine.Node{
				{"type": "text", "id": rootID + "-leaf", "fieldBinding": "name"},
			},
		},
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal tiny component: %v", err)
	}
	return raw
}

func TestComponentAdapterGetPublished(t *testing.T) {
	c := setupClient(t)
	adapter := NewComponentAdapter(c)
	ctx := context.Background()

	tenantSlug := pickHeybabesOrAnyTenantSlug(t, c)
	compName := seedComponent(t, c, tenantSlug, "tiny_comp",
		tinyComponentDocBytes(t, "tiny-root"), 1)

	got, err := adapter.GetPublishedComponent(ctx, tenantSlug, compName)
	if err != nil {
		t.Fatalf("GetPublishedComponent: %v", err)
	}
	if got.Name != compName {
		t.Errorf("name = %q, want %q", got.Name, compName)
	}
	if got.Status != domain.ComponentStatusPublished {
		t.Errorf("status = %q", got.Status)
	}
	if got.Version != 1 {
		t.Errorf("version = %d", got.Version)
	}
	if got.PublishedAt == nil {
		t.Errorf("published_at must be set")
	}
	// DocumentJSON round-trips through engine.Document.
	var doc engine.Document
	if err := json.Unmarshal(got.DocumentJSON, &doc); err != nil {
		t.Fatalf("doc unmarshal: %v", err)
	}
	if len(doc.Children) != 1 || engine.NodeID(doc.Children[0]) != "tiny-root" {
		t.Errorf("doc structure lost: %+v", doc.Children)
	}
}

func TestComponentAdapterGetPublished_NotFound(t *testing.T) {
	c := setupClient(t)
	adapter := NewComponentAdapter(c)
	ctx := context.Background()
	tenantSlug := pickHeybabesOrAnyTenantSlug(t, c)

	_, err := adapter.GetPublishedComponent(ctx, tenantSlug, "no_such_component_zzzzz")
	if !errors.Is(err, domain.ErrComponentNotFound) {
		t.Errorf("err = %v, want ErrComponentNotFound", err)
	}
}

func TestComponentAdapterListPublished(t *testing.T) {
	c := setupClient(t)
	adapter := NewComponentAdapter(c)
	ctx := context.Background()

	tenantSlug := pickHeybabesOrAnyTenantSlug(t, c)
	compName := seedComponent(t, c, tenantSlug, "tiny_comp",
		tinyComponentDocBytes(t, "tiny-root"), 1)

	list, err := adapter.ListPublishedComponents(ctx, tenantSlug)
	if err != nil {
		t.Fatalf("ListPublishedComponents: %v", err)
	}
	found := false
	for _, ci := range list {
		if ci.Name == compName {
			found = true
			if len(ci.DocumentJSON) == 0 {
				t.Errorf("listed DocumentJSON empty")
			}
			break
		}
	}
	if !found {
		t.Errorf("seeded component %q not in list of %d", compName, len(list))
	}
}
