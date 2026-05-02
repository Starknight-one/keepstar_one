//go:build integration

// Run with: TEST_DATABASE_URL=$DATABASE_URL go test -tags=integration ./internal/adapters/postgres/...
//
// End-to-end smoke for chunk 5: DB → seeded components + two presets →
// catalog products → Materialise → ExpandReplicates → ResolveAndInline
// → BindData. Asserts that each replicated card binds to its own product
// AND the same components serve both presets correctly. No LLM, no HTTP.

package postgres

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/engine"
	"keepstar_v5/internal/engine/presets"
	"keepstar_v5/internal/ports"
)

// seedComponentFromBytes is the chunk-5 helper that seeds an
// embedded-JSON component (price_rating / brand_badge) for a given
// tenant. Returns the component's name. Cleanup via CASCADE.
func seedComponentFromBytes(t *testing.T, c *Client, tenantSlug, namePrefix string, body []byte) string {
	return seedComponent(t, c, tenantSlug, namePrefix, body, 1)
}

// seedPresetFromBytes is like seedProductCardPreset but for an arbitrary
// embedded preset body. Returns the preset's name.
func seedPresetFromBytes(t *testing.T, c *Client, tenantSlug, namePrefix string, body []byte) string {
	t.Helper()
	ctx := context.Background()

	var tenantID string
	if err := c.pool.QueryRow(ctx,
		`SELECT id::text FROM catalog.tenants WHERE slug = $1`, tenantSlug,
	).Scan(&tenantID); err != nil {
		t.Fatalf("resolve tenant %q: %v", tenantSlug, err)
	}
	presetName := namePrefix + "_" + t.Name()
	var presetID string
	if err := c.pool.QueryRow(ctx, `
		INSERT INTO v5_presets (tenant_id, name, category, entity_type, description, default_replicate)
		VALUES ($1::uuid, $2, 'product', 'product', 'integration test seed', TRUE)
		RETURNING id::text
	`, tenantID, presetName).Scan(&presetID); err != nil {
		t.Fatalf("insert preset header: %v", err)
	}
	t.Cleanup(func() {
		_, _ = c.pool.Exec(context.Background(), `DELETE FROM v5_presets WHERE id = $1::uuid`, presetID)
	})
	if _, err := c.pool.Exec(ctx, `
		INSERT INTO v5_preset_versions (preset_id, version, status, doc_json, published_at)
		VALUES ($1::uuid, 1, 'published', $2::jsonb, NOW())
	`, presetID, body); err != nil {
		t.Fatalf("insert preset version: %v", err)
	}
	return presetName
}

// runPipelineFor applies the full chunk-5 pipeline for one preset against
// a fixed slice of products and returns the merged document plus the
// replicate stats so the caller can do specific assertions.
func runPipelineFor(t *testing.T, presetDoc *engine.Document, componentDocs []*engine.Document, products []domain.Product) (*engine.Document, engine.ResolveStats, engine.BindResult) {
	t.Helper()
	merged := engine.Materialise(presetDoc, componentDocs)

	data := make([]map[string]any, len(products))
	for i, p := range products {
		data[i] = engine.ProductToMap(p)
	}
	engine.ExpandReplicates(merged, len(data))
	resolveStats := engine.ResolveAndInline(merged)
	bindRes := engine.BindData(merged, data)
	return merged, resolveStats, bindRes
}

func TestEnginePipelineWithComponents(t *testing.T) {
	c := setupClient(t)
	cat := NewCatalogAdapter(c)
	presetAdapter := NewPresetAdapter(c)
	componentAdapter := NewComponentAdapter(c)
	ctx := context.Background()

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

	// Seed both components AND both presets for this tenant.
	priceRatingName := seedComponentFromBytes(t, c, tenantSlug, "price_rating", presets.ComponentPriceRatingJSON)
	brandBadgeName := seedComponentFromBytes(t, c, tenantSlug, "brand_badge", presets.ComponentBrandBadgeJSON)
	cardName := seedPresetFromBytes(t, c, tenantSlug, "product_card", presets.ProductCardJSON)
	rowName := seedPresetFromBytes(t, c, tenantSlug, "product_card_list_row", presets.ProductCardListRowJSON)

	// Verify adapters can find what we seeded.
	pr, err := componentAdapter.GetPublishedComponent(ctx, tenantSlug, priceRatingName)
	if err != nil {
		t.Fatalf("load price_rating component: %v", err)
	}
	bb, err := componentAdapter.GetPublishedComponent(ctx, tenantSlug, brandBadgeName)
	if err != nil {
		t.Fatalf("load brand_badge component: %v", err)
	}
	cardPreset, err := presetAdapter.GetPublishedPreset(ctx, tenantSlug, cardName)
	if err != nil {
		t.Fatalf("load product_card preset: %v", err)
	}
	rowPreset, err := presetAdapter.GetPublishedPreset(ctx, tenantSlug, rowName)
	if err != nil {
		t.Fatalf("load product_card_list_row preset: %v", err)
	}

	// Convert raw DocumentJSONs to engine.Documents.
	cardDoc := mustParseDoc(t, cardPreset.DocumentJSON)
	rowDoc := mustParseDoc(t, rowPreset.DocumentJSON)
	prDoc := mustParseDoc(t, pr.DocumentJSON)
	bbDoc := mustParseDoc(t, bb.DocumentJSON)

	componentDocs := []*engine.Document{prDoc, bbDoc}

	// === Pipeline run #1: product_card ===
	t.Run("product_card", func(t *testing.T) {
		merged, resolveStats, bindRes := runPipelineFor(t, cardDoc, componentDocs, products)

		// Expected refs: 2 per replicated card × 3 cards = 6 successful
		// resolutions. Failed list must be empty.
		if resolveStats.Resolved != 6 {
			t.Errorf("expected 6 refs resolved, got %d (failed: %v)", resolveStats.Resolved, resolveStats.Failed)
		}
		if len(resolveStats.Failed) > 0 {
			t.Errorf("ref resolutions failed: %v", resolveStats.Failed)
		}

		// 3 cloned cards + 2 reusable component definitions = 5 top-level children.
		if got := len(merged.Children); got != 5 {
			t.Fatalf("expected 5 top-level children (3 clones + 2 components), got %d", got)
		}

		// Reusable definitions must NOT have bound atoms — they are templates.
		for _, child := range merged.Children {
			if r, _ := child["reusable"].(bool); !r {
				continue
			}
			engine.WalkNodes(child, func(n engine.Node, _ int) {
				if _, has := n["__bound"]; has {
					t.Errorf("reusable subtree atom got __bound: %+v", n)
				}
			})
		}

		// Per-clone assertions: title content, brand content (via ref), price content (via ref).
		clones := topLevelInstances(merged)
		if len(clones) != 3 {
			t.Fatalf("expected 3 instance clones, got %d", len(clones))
		}
		for i, clone := range clones {
			assertCardCloneBound(t, clone, i, products[i])
		}
		_ = bindRes // pure smoke
	})

	// === Pipeline run #2: product_card_list_row, same components ===
	t.Run("product_card_list_row", func(t *testing.T) {
		merged, resolveStats, _ := runPipelineFor(t, rowDoc, componentDocs, products)

		if resolveStats.Resolved != 6 {
			t.Errorf("expected 6 refs resolved (3×2), got %d (failed: %v)", resolveStats.Resolved, resolveStats.Failed)
		}

		clones := topLevelInstances(merged)
		if len(clones) != 3 {
			t.Fatalf("expected 3 row instances, got %d", len(clones))
		}
		for i, clone := range clones {
			// row-card has the same shared bindings (name/heroImage/price/brand)
			// PLUS the unique description binding (skipped if catalog row has none).
			assertCardCloneBound(t, clone, i, products[i])

			// description: only assert presence when product has it.
			if products[i].Description == "" {
				continue
			}
			var desc engine.Node
			engine.WalkNodes(clone, func(n engine.Node, _ int) {
				if fb, _ := n["fieldBinding"].(string); fb == "description" {
					desc = n
				}
			})
			if desc == nil {
				t.Errorf("row[%d] missing description atom", i)
				continue
			}
			gotDesc, _ := desc["content"].(string)
			if gotDesc == "" {
				t.Errorf("row[%d] description content empty (product has description)", i)
			}
		}
	})
}

// topLevelInstances returns the children of doc that are NOT reusable
// component definitions (i.e. the replicate clones).
func topLevelInstances(doc *engine.Document) []engine.Node {
	var out []engine.Node
	for _, c := range doc.Children {
		if r, _ := c["reusable"].(bool); r {
			continue
		}
		out = append(out, c)
	}
	return out
}

// assertCardCloneBound walks one clone and verifies that the shared
// bindings (name, brand, price/rating, hero image) all landed on the
// expected product. Used by both the product_card and list_row branches
// since they share these atoms.
func assertCardCloneBound(t *testing.T, clone engine.Node, i int, product domain.Product) {
	t.Helper()
	var (
		nameNode, brandNode, priceNode, heroNode engine.Node
	)
	engine.WalkNodes(clone, func(n engine.Node, _ int) {
		switch fb, _ := n["fieldBinding"].(string); fb {
		case "name":
			nameNode = n
		case "brand":
			brandNode = n
		case "priceFormatted":
			priceNode = n
		case "heroImage":
			heroNode = n
		}
	})
	// name & brand are mandatory for any product seed; assert.
	if nameNode == nil {
		t.Errorf("clone[%d]: no name atom", i)
	} else if got, _ := nameNode["content"].(string); !strings.EqualFold(got, product.Name) {
		t.Errorf("clone[%d] name = %q, want %q", i, got, product.Name)
	}
	if brandNode == nil {
		t.Errorf("clone[%d]: no brand atom (ref expansion likely failed)", i)
	} else if got, _ := brandNode["content"].(string); product.Brand != "" && got != product.Brand {
		t.Errorf("clone[%d] brand = %q, want %q", i, got, product.Brand)
	}
	if priceNode == nil {
		t.Errorf("clone[%d]: no price atom (price-rating ref expansion likely failed)", i)
	} else if got, _ := priceNode["content"].(string); product.PriceFormatted != "" && got != product.PriceFormatted {
		t.Errorf("clone[%d] price = %q, want %q", i, got, product.PriceFormatted)
	}
	// Hero only checked when product has images; otherwise heroNode might
	// legitimately be in BindResult.Missing.
	if len(product.Images) > 0 && heroNode != nil {
		fills, _ := heroNode["fills"].([]any)
		if len(fills) == 0 {
			t.Errorf("clone[%d] hero has product image but fills empty", i)
			return
		}
		first, _ := fills[0].(map[string]any)
		if first["type"] != engine.FillTypeImage {
			t.Errorf("clone[%d] hero fill type = %v", i, first["type"])
		}
		if url, _ := first["image"].(string); url == "" {
			t.Errorf("clone[%d] hero image url empty", i)
		}
	}
}

func mustParseDoc(t *testing.T, raw json.RawMessage) *engine.Document {
	t.Helper()
	var doc engine.Document
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse engine.Document: %v", err)
	}
	return &doc
}
