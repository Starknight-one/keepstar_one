package tools

import (
	"testing"

	"keepstar_v5/internal/domain"
)

// The per-tenant schema must carry the tenant's real vocabulary: enum
// filters from SharedFilters, min_/max_ pairs from NumericRanges, and the
// base brand/category/price props — and none of the legacy cosmetics enums.
func TestCatalogSearchSchemaForDigest(t *testing.T) {
	d := &domain.CatalogDigest{
		TotalProducts: 42,
		SharedFilters: []domain.DigestSharedFilter{
			{Key: "dealType", Values: []string{"sale", "rent"}},
		},
		NumericRanges: []domain.DigestNumericRange{
			{Key: "rooms", Min: 1, Max: 5},
		},
		PriceMin: 50000, PriceMax: 900000,
	}
	schema := CatalogSearchSchemaForDigest(d)
	if schema == nil {
		t.Fatal("nil schema for a populated digest")
	}
	props := schema["properties"].(map[string]interface{})["filters"].(map[string]interface{})["properties"].(map[string]interface{})

	deal, ok := props["dealType"].(map[string]interface{})
	if !ok {
		t.Fatalf("dealType filter missing: %v", props)
	}
	if vals := deal["enum"].([]string); len(vals) != 2 || vals[0] != "sale" {
		t.Errorf("dealType enum: %v", vals)
	}
	for _, k := range []string{"min_rooms", "max_rooms", "brand", "min_price", "max_price"} {
		if _, ok := props[k]; !ok {
			t.Errorf("expected prop %q missing", k)
		}
	}
	if _, ok := props["skin_type"]; ok {
		t.Error("legacy cosmetics enum leaked into per-tenant schema")
	}

	if s := CatalogSearchSchemaForDigest(nil); s != nil {
		t.Error("nil digest must yield nil schema (fallback to static)")
	}
	if s := CatalogSearchSchemaForDigest(&domain.CatalogDigest{}); s != nil {
		t.Error("empty digest must yield nil schema")
	}
}
