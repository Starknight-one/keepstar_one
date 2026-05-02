//go:build integration

// Run with: TEST_DATABASE_URL=$DATABASE_URL go test -tags=integration ./internal/adapters/postgres/...
//
// Read-only tests against the shared catalog.* schema. We only assert
// shape and not specific row contents — the prod catalog evolves and
// we want the tests to stay green as long as the schema is right.

package postgres

import (
	"context"
	"testing"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/ports"
)

// pickAnyProductTenant finds a tenant slug that has at least one product
// in the live catalog. Tests are tenant-agnostic — we don't bake slugs in
// to the test code because the seed varies between dev/prod environments.
func pickAnyProductTenant(t *testing.T, c *Client) string {
	t.Helper()
	var slug string
	err := c.pool.QueryRow(context.Background(), `
		SELECT t.slug
		FROM catalog.tenants t
		WHERE EXISTS (SELECT 1 FROM catalog.products p WHERE p.tenant_id = t.id AND p.deleted_at IS NULL)
		LIMIT 1
	`).Scan(&slug)
	if err != nil {
		t.Skipf("no tenant with products in this environment: %v", err)
	}
	return slug
}

// TestCatalogAdapterListProducts hits ListProducts for any tenant that has
// products and asserts the structural promises: rows come back, master
// fields are merged in, and the JSONB read path doesn't blow up.
func TestCatalogAdapterListProducts(t *testing.T) {
	c := setupClient(t)
	cat := NewCatalogAdapter(c)
	ctx := context.Background()

	slug := pickAnyProductTenant(t, c)

	tenant, err := cat.GetTenantBySlug(ctx, slug)
	if err != nil {
		t.Fatalf("get tenant: %v", err)
	}
	if tenant.Slug != slug {
		t.Errorf("tenant slug drift: got %q, want %q", tenant.Slug, slug)
	}

	products, total, err := cat.ListProducts(ctx, tenant.ID, ports.ProductFilter{Limit: 5})
	if err != nil {
		t.Fatalf("list products: %v", err)
	}
	if total == 0 || len(products) == 0 {
		t.Fatalf("expected products for tenant %q (total=%d)", slug, total)
	}

	// Sanity: master JOIN didn't blow up. We don't require Brand/Category
	// to be populated (test-electronics tenant has neither in master).
	// Just verify the scan didn't return zero-valued products.
	for _, p := range products {
		if p.ID == "" || p.TenantID == "" {
			t.Errorf("scan produced empty id/tenant: %+v", p)
		}
	}
	// Tier2/Extra is optional; just log presence.
	hasTier2OrExtra := false
	for _, p := range products {
		if len(p.Tier2) > 0 || len(p.Extra) > 0 {
			hasTier2OrExtra = true
			break
		}
	}
	if !hasTier2OrExtra {
		t.Logf("none of %d products carried Tier2 or Extra — OK if curator hasn't filled them", len(products))
	}

	// First product should round-trip via GetProduct.
	first := products[0]
	got, err := cat.GetProduct(ctx, tenant.ID, first.ID)
	if err != nil {
		t.Fatalf("get product: %v", err)
	}
	if got.ID != first.ID {
		t.Errorf("get product id drift: %q vs %q", got.ID, first.ID)
	}
	if got.PriceFormatted == "" && got.Price > 0 {
		t.Errorf("priceFormatted should be populated for non-zero price: %+v", got)
	}
}

// TestFieldDefinitionAdapter hits the field_definitions table for whichever
// tenant has field defs seeded. Asserts the per-tenant config shape: every
// row has non-empty FieldName/Label/AtomType.
func TestFieldDefinitionAdapter(t *testing.T) {
	c := setupClient(t)
	fd := NewFieldDefinitionAdapter(c)
	ctx := context.Background()

	// Find a tenant that has field defs seeded.
	var slug string
	err := c.pool.QueryRow(ctx, `
		SELECT t.slug
		FROM catalog.tenants t
		WHERE EXISTS (SELECT 1 FROM catalog.field_definitions fd WHERE fd.tenant_id = t.id)
		LIMIT 1
	`).Scan(&slug)
	if err != nil {
		t.Skipf("no tenant has field_definitions seeded in this env: %v", err)
	}

	defs, err := fd.ListFieldDefinitions(ctx, slug, domain.EntityTypeProduct)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(defs) == 0 {
		t.Skipf("no product field definitions seeded for %q (services only?)", slug)
	}

	// Sanity: every entry has a non-empty FieldName + Label, types are
	// strings (not zero-coerced).
	for _, d := range defs {
		if d.FieldName == "" {
			t.Errorf("empty field_name in definition: %+v", d)
		}
		if d.Label == "" {
			t.Errorf("empty label for field %q", d.FieldName)
		}
		if d.AtomType == "" {
			t.Errorf("empty atom_type for field %q", d.FieldName)
		}
	}

	// Spot-check: GetFieldDefinition for the first entry returns the same row.
	first := defs[0]
	got, err := fd.GetFieldDefinition(ctx, slug, domain.EntityTypeProduct, first.FieldName)
	if err != nil {
		t.Fatalf("get %q: %v", first.FieldName, err)
	}
	if got.FieldName != first.FieldName || got.Label != first.Label {
		t.Errorf("get vs list drift: %+v vs %+v", got, first)
	}

	// SampleFieldValues — best-effort. Should produce at least one sample
	// for at least one field in a populated catalog.
	samples, err := fd.SampleFieldValues(ctx, slug, domain.EntityTypeProduct, 3)
	if err != nil {
		t.Fatalf("sample: %v", err)
	}
	if len(samples) == 0 {
		t.Logf("no samples returned — tenant %q may have no products", slug)
	}
}

