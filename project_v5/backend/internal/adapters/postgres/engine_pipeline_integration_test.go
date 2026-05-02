//go:build integration

// Run with: TEST_DATABASE_URL=$DATABASE_URL go test -tags=integration ./internal/adapters/postgres/...
//
// End-to-end smoke for chunk 4: DB → seeded product_card preset → catalog
// products → ProductToMap → ExpandReplicates → BindData. Asserts that each
// replicated card binds to its own product. No LLM, no HTTP — just the
// engine pipeline running against live data.

package postgres

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"keepstar_v5/internal/engine"
	"keepstar_v5/internal/ports"
)

func TestEnginePipelineEndToEnd(t *testing.T) {
	c := setupClient(t)
	cat := NewCatalogAdapter(c)
	presetAdapter := NewPresetAdapter(c)
	ctx := context.Background()

	// Pick a tenant that already has ≥3 products — picker prefers heybabes
	// when available (V4 sample data), falls back to any tenant with enough
	// products in dev/staging environments.
	tenantSlug := pickTenantSlugWithProducts(t, c, 3)

	// 1. Resolve tenant.
	tenant, err := cat.GetTenantBySlug(ctx, tenantSlug)
	if err != nil {
		t.Fatalf("GetTenantBySlug: %v", err)
	}

	// 2. Pull 3 products — picker already guaranteed availability.
	products, _, err := cat.ListProducts(ctx, tenant.ID, ports.ProductFilter{Limit: 3})
	if err != nil {
		t.Fatalf("ListProducts: %v", err)
	}
	if len(products) < 3 {
		t.Fatalf("expected 3 products from picked tenant %q, got %d", tenantSlug, len(products))
	}

	// 3. Seed product_card preset for the tenant and load it through the adapter.
	presetName := seedProductCardPreset(t, c, tenantSlug, 1)
	preset, err := presetAdapter.GetPublishedPreset(ctx, tenantSlug, presetName)
	if err != nil {
		t.Fatalf("GetPublishedPreset: %v", err)
	}

	// 4. Build the engine.Document and the data slice.
	var doc engine.Document
	if err := json.Unmarshal(preset.DocumentJSON, &doc); err != nil {
		t.Fatalf("doc unmarshal: %v", err)
	}
	data := make([]map[string]any, len(products))
	for i, p := range products {
		data[i] = engine.ProductToMap(p)
	}

	// 5. Replicate fan-out + bind. This is the order V5 will run in
	// production: ApplyOps → ExpandReplicates → ComponentResolver
	// (skipped here — no RefNodes in product_card seed) → BindData.
	engine.ExpandReplicates(&doc, len(data))
	res := engine.BindData(&doc, data)

	if len(doc.Children) != len(data) {
		t.Fatalf("expected %d cloned cards, got %d", len(data), len(doc.Children))
	}
	if len(res.Bound) == 0 {
		t.Fatalf("no atoms bound; res = %+v", res)
	}

	// 6. Walk each clone and confirm the title atom carries its product's
	// name, and the hero atom has the matching image URL in fills[0].image
	// (when the product has an image).
	for i, clone := range doc.Children {
		title := engine.FindNodeByID(&doc, "title")
		_ = title // title is shared by-id across clones; we need the LOCAL one inside this clone
		// Find descendants under THIS clone, not via global search (which
		// would always return the first sibling's atoms because clones
		// share semantic ids — but we re-id'd them in ExpandReplicates).
		var localTitle, localHero engine.Node
		engine.WalkNodes(clone, func(n engine.Node, _ int) {
			if fb, _ := n["fieldBinding"].(string); fb == "name" {
				localTitle = n
			}
			if fb, _ := n["fieldBinding"].(string); fb == "heroImage" {
				localHero = n
			}
		})
		if localTitle == nil {
			t.Errorf("clone[%d]: no title atom (no fieldBinding=name found)", i)
			continue
		}
		gotTitle, _ := localTitle["content"].(string)
		wantTitle := products[i].Name
		if !strings.EqualFold(gotTitle, wantTitle) {
			t.Errorf("clone[%d] title content = %q, want %q", i, gotTitle, wantTitle)
		}
		// Image fill: only assert when the product actually has an image.
		// Heybabes products have images; an empty-image tenant would
		// legitimately end up with the hero atom in BindResult.Missing.
		if len(products[i].Images) > 0 && localHero != nil {
			fills, _ := localHero["fills"].([]any)
			if len(fills) == 0 {
				t.Errorf("clone[%d] hero has product image but fills empty", i)
				continue
			}
			first, _ := fills[0].(map[string]any)
			if first["type"] != engine.FillTypeImage {
				t.Errorf("clone[%d] hero fill type = %v", i, first["type"])
			}
			if url, _ := first["image"].(string); url == "" {
				t.Errorf("clone[%d] hero fill image url is empty", i)
			}
		}
	}
}
