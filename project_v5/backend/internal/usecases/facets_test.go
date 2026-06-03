package usecases

import (
	"testing"

	"keepstar_v5/internal/domain"
)

// prod builds a furniture-ish product. SKU is high-cardinality + on the
// skip list, so it must never surface as a facet — the test guards that.
func prod(id, brand string, priceKopecks int, material string) domain.Product {
	return domain.Product{
		ID:    id,
		Name:  "Item " + id, // skip list (free text)
		Brand: brand,
		Price: priceKopecks,
		SKU:   "SKU-" + id, // skip list (identifier)
		Tier2: map[string]interface{}{"material": material},
	}
}

func sampleProducts() []domain.Product {
	return []domain.Product{
		prod("1", "HUIMO", 19900, "wood"),
		prod("2", "HUIMO", 29900, "metal"),
		prod("3", "STHOUYN", 9900, "wood"),
		prod("4", "AILEEKISS", 13500, "wood"),
	}
}

func findFacet(facets []Facet, key string) *Facet {
	for i := range facets {
		if facets[i].Key == key {
			return &facets[i]
		}
	}
	return nil
}

func valueCount(f *Facet, value string) (int, bool) {
	for _, v := range f.Values {
		if v.Value == value {
			return v.Count, true
		}
	}
	return 0, false
}

func fptr(v float64) *float64 { return &v }

// Facets must be DERIVED FROM THE DATA (vertical-agnostic): a numeric
// attribute becomes a range, a low-cardinality attribute (incl. tier2
// "material") becomes an enum with counts, and identifiers / free text are
// never exposed. This is the whole point — the panel adapts to whatever the
// products actually carry, with no hardcoded list.
func TestBuildGuidedFacets_DerivesTypedFacetsFromData(t *testing.T) {
	facets := BuildGuidedFacets(sampleProducts(), nil)

	price := findFacet(facets, "price")
	if price == nil || price.Type != "range" {
		t.Fatalf("price should be a range facet, got %+v", price)
	}
	if price.Unit != "currency" || price.Min != 9900 || price.Max != 29900 {
		t.Errorf("price range wrong: unit=%q min=%v max=%v", price.Unit, price.Min, price.Max)
	}

	brand := findFacet(facets, "brand")
	if brand == nil || brand.Type != "enum" {
		t.Fatalf("brand should be an enum facet, got %+v", brand)
	}
	if c, ok := valueCount(brand, "HUIMO"); !ok || c != 2 {
		t.Errorf("brand HUIMO count = %d, want 2", c)
	}

	// tier2 attribute surfaces with no hardcoding — the data-aware win.
	material := findFacet(facets, "material")
	if material == nil || material.Type != "enum" {
		t.Fatalf("material (tier2) should be an enum facet, got %+v", material)
	}
	if c, _ := valueCount(material, "wood"); c != 3 {
		t.Errorf("material wood count = %d, want 3", c)
	}

	// Identifiers / free text must never become filters.
	if findFacet(facets, "sku") != nil {
		t.Error("sku must not be a facet (skip list / identifier)")
	}
	if findFacet(facets, "name") != nil {
		t.Error("name must not be a facet (skip list / free text)")
	}
}

// Refine is a deterministic in-memory narrow: enum membership and numeric
// range, composing across facets (AND) and within an enum (OR).
func TestFilterProducts_EnumAndRange(t *testing.T) {
	all := sampleProducts()

	byBrand := FilterProducts(all, []AppliedFilter{{Key: "brand", Type: "enum", Values: []string{"HUIMO"}}})
	if len(byBrand) != 2 {
		t.Errorf("brand=HUIMO → %d products, want 2", len(byBrand))
	}

	byPrice := FilterProducts(all, []AppliedFilter{{Key: "price", Type: "range", Max: fptr(15000)}})
	if len(byPrice) != 2 { // 9900 + 13500
		t.Errorf("price<=15000 → %d products, want 2", len(byPrice))
	}

	byMaterial := FilterProducts(all, []AppliedFilter{{Key: "material", Type: "enum", Values: []string{"wood"}}})
	if len(byMaterial) != 3 {
		t.Errorf("material=wood → %d products, want 3", len(byMaterial))
	}
}

// Guided faceting: each facet's domain is computed over the set matching
// every OTHER active filter — so a facet you've filtered by still shows all
// its options, while the rest narrow to reflect the selection.
func TestBuildGuidedFacets_ExcludesOwnFilter(t *testing.T) {
	active := []AppliedFilter{{Key: "brand", Type: "enum", Values: []string{"HUIMO"}}}
	facets := BuildGuidedFacets(sampleProducts(), active)

	// brand facet ignores its own filter → still shows every brand.
	brand := findFacet(facets, "brand")
	if brand == nil || len(brand.Values) != 3 {
		t.Errorf("brand facet should still list all 3 brands, got %+v", brand)
	}

	// price facet reflects the brand filter → only HUIMO items (199, 299).
	price := findFacet(facets, "price")
	if price == nil || price.Min != 19900 || price.Max != 29900 {
		t.Errorf("guided price range should be HUIMO-only [19900,29900], got %+v", price)
	}

	// material facet reflects the brand filter → HUIMO has wood + metal.
	material := findFacet(facets, "material")
	if material == nil || len(material.Values) != 2 {
		t.Errorf("guided material should be HUIMO-only (wood,metal), got %+v", material)
	}
}
