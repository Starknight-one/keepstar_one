package domain

import (
	"strings"
	"testing"
	"time"
)

// --- DigestParam format tests ---

func TestDigestParam_ValuesFormat_LowCardinality(t *testing.T) {
	p := DigestParam{
		Key:         "material",
		Type:        "enum",
		Cardinality: 3,
		Values:      []string{"Mesh", "Leather", "Synthetic"},
	}
	result := formatParam(p)
	if !strings.Contains(result, "material(3)") {
		t.Errorf("expected material(3) in output, got: %s", result)
	}
	if !strings.Contains(result, "Mesh, Leather, Synthetic") {
		t.Errorf("expected full values list, got: %s", result)
	}
	if !strings.Contains(result, "→ filter") {
		t.Errorf("expected → filter hint, got: %s", result)
	}
}

func TestDigestParam_TopFormat_MediumCardinality(t *testing.T) {
	p := DigestParam{
		Key:         "brand",
		Type:        "enum",
		Cardinality: 25,
		Top:         []string{"Nike", "Adidas", "Puma", "Reebok", "ASICS"},
		More:        20,
	}
	result := formatParam(p)
	if !strings.Contains(result, "brand(25") {
		t.Errorf("expected brand(25 in output, got: %s", result)
	}
	if !strings.Contains(result, "top: Nike/Adidas/Puma/Reebok/ASICS") {
		t.Errorf("expected top values, got: %s", result)
	}
	if !strings.Contains(result, "+20") {
		t.Errorf("expected +20 more count, got: %s", result)
	}
	if !strings.Contains(result, "→ filter") {
		t.Errorf("expected → filter hint, got: %s", result)
	}
}

func TestDigestParam_FamiliesFormat_HighCardinality(t *testing.T) {
	p := DigestParam{
		Key:         "color",
		Type:        "enum",
		Cardinality: 55,
		Families:    []string{"Black", "Blue", "Green", "Red", "White"},
	}
	result := formatParam(p)
	if !strings.Contains(result, "color(55") {
		t.Errorf("expected color(55 in output, got: %s", result)
	}
	if !strings.Contains(result, "families: Black/Blue/Green/Red/White") {
		t.Errorf("expected families list, got: %s", result)
	}
	if !strings.Contains(result, "→ vector_query") {
		t.Errorf("expected → vector_query hint for families, got: %s", result)
	}
}

func TestDigestParam_RangeFormat_Numeric(t *testing.T) {
	p := DigestParam{
		Key:         "size",
		Type:        "range",
		Cardinality: 12,
		Range:       "36-47",
	}
	result := formatParam(p)
	if !strings.Contains(result, "size(range: 36-47)") {
		t.Errorf("expected size(range: 36-47), got: %s", result)
	}
	if !strings.Contains(result, "→ filter") {
		t.Errorf("expected → filter hint for range, got: %s", result)
	}
}

// --- ToPromptText tests ---

func TestCatalogDigest_ToPromptText_Basic(t *testing.T) {
	d := &CatalogDigest{
		GeneratedAt:   time.Now(),
		TotalProducts: 100,
		Categories: []DigestCategory{
			{
				Name:       "Running Shoes",
				Slug:       "running-shoes",
				Count:      45,
				PriceRange: [2]int{599000, 1899000},
				Params: []DigestParam{
					{Key: "brand", Type: "enum", Cardinality: 5, Values: []string{"Nike", "Adidas", "Asics", "Hoka", "New Balance"}},
					{Key: "color", Type: "enum", Cardinality: 28, Families: []string{"Black", "White", "Blue", "Red", "Green", "Gray"}},
				},
			},
		},
	}

	result := d.ToPromptText()

	if !strings.Contains(result, "Tenant catalog: 100 products") {
		t.Errorf("expected total products header, got: %s", result)
	}
	if !strings.Contains(result, "Running Shoes (45)") {
		t.Errorf("expected category name with count")
	}
	if !strings.Contains(result, "5990-18990 RUB") {
		t.Errorf("expected price range in rubles, got: %s", result)
	}
	if !strings.Contains(result, "→ filter") {
		t.Error("expected → filter hint")
	}
	if !strings.Contains(result, "→ vector_query") {
		t.Error("expected → vector_query hint")
	}
	if !strings.Contains(result, "Search strategy:") {
		t.Error("expected Search strategy block")
	}
}

func TestCatalogDigest_ToPromptText_Empty(t *testing.T) {
	d := &CatalogDigest{
		GeneratedAt:   time.Now(),
		TotalProducts: 0,
		Categories:    []DigestCategory{},
	}

	result := d.ToPromptText()
	if result != "" {
		t.Errorf("expected empty string for empty digest, got: %q", result)
	}

	// Also test nil
	var nilDigest *CatalogDigest
	result = nilDigest.ToPromptText()
	if result != "" {
		t.Errorf("expected empty string for nil digest, got: %q", result)
	}
}

func TestCatalogDigest_ToPromptText_LargeCategories(t *testing.T) {
	cats := make([]DigestCategory, 32)
	for i := range cats {
		cats[i] = DigestCategory{
			Name:       "Category" + strings.Repeat("X", i),
			Slug:       "cat",
			Count:      100 - i,
			PriceRange: [2]int{100000, 500000},
		}
	}

	d := &CatalogDigest{
		GeneratedAt:   time.Now(),
		TotalProducts: 3200,
		Categories:    cats,
	}

	result := d.ToPromptText()

	if !strings.Contains(result, "...and 7 more categories") {
		t.Errorf("expected '...and 7 more categories', got: %s", result)
	}

	// Should show exactly 25 categories
	categoryCount := strings.Count(result, "Category")
	if categoryCount != 25 {
		t.Errorf("expected 25 categories shown, got %d", categoryCount)
	}
}

func TestCatalogDigest_ToPromptText_PriceFormatting(t *testing.T) {
	d := &CatalogDigest{
		GeneratedAt:   time.Now(),
		TotalProducts: 10,
		Categories: []DigestCategory{
			{
				Name:       "Sneakers",
				Slug:       "sneakers",
				Count:      10,
				PriceRange: [2]int{599000, 1899000},
			},
		},
	}

	result := d.ToPromptText()
	if !strings.Contains(result, "5990-18990 RUB") {
		t.Errorf("expected kopecks→rubles conversion (5990-18990 RUB), got: %s", result)
	}
}

func TestCatalogDigest_ToPromptText_BrandHandling(t *testing.T) {
	d := &CatalogDigest{
		GeneratedAt:   time.Now(),
		TotalProducts: 50,
		Categories: []DigestCategory{
			{
				Name:       "Running Shoes",
				Slug:       "running-shoes",
				Count:      50,
				PriceRange: [2]int{500000, 1500000},
				Params: []DigestParam{
					{Key: "brand", Type: "enum", Cardinality: 5, Values: []string{"Nike", "Adidas", "Asics", "Hoka", "NB"}},
					{Key: "color", Type: "enum", Cardinality: 3, Values: []string{"Black", "White", "Red"}},
				},
			},
		},
	}

	result := d.ToPromptText()
	// Brand should appear before color in output
	brandIdx := strings.Index(result, "brand(5)")
	colorIdx := strings.Index(result, "color(3)")
	if brandIdx < 0 || colorIdx < 0 {
		t.Fatalf("expected brand and color params, got: %s", result)
	}
	if brandIdx > colorIdx {
		t.Errorf("expected brand to come before color, brand at %d, color at %d", brandIdx, colorIdx)
	}
}

func TestCatalogDigest_ToPromptText_FilterHints(t *testing.T) {
	d := &CatalogDigest{
		GeneratedAt:   time.Now(),
		TotalProducts: 20,
		Categories: []DigestCategory{
			{
				Name:       "Shoes",
				Slug:       "shoes",
				Count:      20,
				PriceRange: [2]int{300000, 900000},
				Params: []DigestParam{
					{Key: "brand", Type: "enum", Cardinality: 3, Values: []string{"Nike", "Adidas", "Puma"}},
					{Key: "size", Type: "range", Cardinality: 12, Range: "36-47"},
					{Key: "color", Type: "enum", Cardinality: 55, Families: []string{"Black", "Blue", "Green", "Red"}},
				},
			},
		},
	}

	result := d.ToPromptText()

	lines := strings.Split(result, "\n")
	for _, line := range lines {
		if strings.Contains(line, "brand(") {
			if !strings.Contains(line, "→ filter") {
				t.Errorf("brand should have → filter, got: %s", line)
			}
		}
		if strings.Contains(line, "size(") {
			if !strings.Contains(line, "→ filter") {
				t.Errorf("size should have → filter, got: %s", line)
			}
		}
		if strings.Contains(line, "color(") {
			if !strings.Contains(line, "→ vector_query") {
				t.Errorf("color(55) should have → vector_query, got: %s", line)
			}
		}
	}
}

func TestCatalogDigest_ToPromptText_ServicesNoPrice(t *testing.T) {
	d := &CatalogDigest{
		GeneratedAt:   time.Now(),
		TotalProducts: 30,
		Categories: []DigestCategory{
			{
				Name:       "Cleaning",
				Slug:       "cleaning",
				Count:      30,
				PriceRange: [2]int{0, 150000}, // min=0 means "from N"
			},
		},
	}

	result := d.ToPromptText()
	if !strings.Contains(result, "from 1500 RUB") {
		t.Errorf("expected 'from 1500 RUB' for service with min=0, got: %s", result)
	}
}

// --- ComputeFamilies tests ---

func TestComputeFamilies_ColorGrouping(t *testing.T) {
	values := []string{
		"Красный", "Бордовый", "Алый",         // → Red
		"Салатовый", "Зелёный", "Травяной",     // → Green
		"Синий", "Голубой",                       // → Blue
		"Чёрный",                                 // → Black
	}

	families := ComputeFamilies("color", values)

	familySet := make(map[string]bool)
	for _, f := range families {
		familySet[f] = true
	}

	expected := []string{"Red", "Green", "Blue", "Black"}
	for _, e := range expected {
		if !familySet[e] {
			t.Errorf("expected family %q in result, got: %v", e, families)
		}
	}

	// Should NOT have "Красный" as-is
	for _, f := range families {
		if f == "Красный" || f == "Салатовый" {
			t.Errorf("raw color %q should be mapped to family, not preserved", f)
		}
	}
}
