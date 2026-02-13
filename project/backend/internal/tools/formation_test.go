package tools

import (
	"testing"

	"keepstar/internal/domain"
	"keepstar/internal/presets"
)

// --- BuildFormation tests ---

func TestBuildFormation_GridFourProducts(t *testing.T) {
	preset := presets.ProductGridPreset
	products := testProducts(4)

	formation := BuildFormation(preset, len(products), func(i int) (FieldGetter, CurrencyGetter, IDGetter) {
		p := products[i]
		return productFieldGetter(p), func() string { return p.Currency }, func() string { return p.ID }
	})

	if formation.Mode != domain.FormationTypeGrid {
		t.Errorf("want mode grid, got %s", formation.Mode)
	}
	if len(formation.Widgets) != 4 {
		t.Fatalf("want 4 widgets, got %d", len(formation.Widgets))
	}

	for i, w := range formation.Widgets {
		if w.ID == "" {
			t.Errorf("widget[%d]: missing ID", i)
		}
		if w.Template != domain.WidgetTemplateProductCard {
			t.Errorf("widget[%d]: want template ProductCard, got %s", i, w.Template)
		}
		if w.Size != domain.WidgetSizeMedium {
			t.Errorf("widget[%d]: want size medium, got %s", i, w.Size)
		}
		if w.Priority != i {
			t.Errorf("widget[%d]: want priority %d, got %d", i, i, w.Priority)
		}
		if w.EntityRef == nil {
			t.Errorf("widget[%d]: missing EntityRef", i)
		} else if w.EntityRef.ID != products[i].ID {
			t.Errorf("widget[%d]: want EntityRef.ID %s, got %s", i, products[i].ID, w.EntityRef.ID)
		}
		if len(w.Atoms) == 0 {
			t.Errorf("widget[%d]: no atoms", i)
		}
	}
}

func TestBuildFormation_SingleProductDetail(t *testing.T) {
	preset := presets.ProductDetailPreset
	products := testProducts(1)

	formation := BuildFormation(preset, 1, func(i int) (FieldGetter, CurrencyGetter, IDGetter) {
		p := products[i]
		return productFieldGetter(p), func() string { return p.Currency }, func() string { return p.ID }
	})

	if formation.Mode != domain.FormationTypeSingle {
		t.Errorf("want mode single, got %s", formation.Mode)
	}
	if len(formation.Widgets) != 1 {
		t.Fatalf("want 1 widget, got %d", len(formation.Widgets))
	}
}

func TestBuildFormation_EmptyProducts(t *testing.T) {
	preset := presets.ProductGridPreset
	formation := BuildFormation(preset, 0, func(i int) (FieldGetter, CurrencyGetter, IDGetter) {
		t.Fatal("should not be called")
		return nil, nil, nil
	})

	if len(formation.Widgets) != 0 {
		t.Errorf("want 0 widgets, got %d", len(formation.Widgets))
	}
}

func TestBuildFormation_NilFieldsSkipped(t *testing.T) {
	preset := presets.ProductGridPreset
	emptyProduct := domain.Product{
		ID:    "empty-1",
		Name:  "Only Name",
		Price: 0, // will be set to 0
		// All other fields empty → nil from field getter
	}

	formation := BuildFormation(preset, 1, func(i int) (FieldGetter, CurrencyGetter, IDGetter) {
		return productFieldGetter(emptyProduct), func() string { return "" }, func() string { return emptyProduct.ID }
	})

	if len(formation.Widgets) != 1 {
		t.Fatalf("want 1 widget, got %d", len(formation.Widgets))
	}

	// Only "name" and "price" (price=0 is still returned) should have atoms
	w := formation.Widgets[0]
	for _, a := range w.Atoms {
		if a.Value == nil {
			t.Error("nil value should have been skipped in buildAtoms")
		}
	}
}

func TestBuildFormation_CurrencyMeta(t *testing.T) {
	preset := presets.ProductGridPreset
	p := domain.Product{
		ID:       "p1",
		Name:     "Test",
		Price:    150000,
		Currency: "USD",
		Images:   []string{"img.jpg"},
	}

	formation := BuildFormation(preset, 1, func(i int) (FieldGetter, CurrencyGetter, IDGetter) {
		return productFieldGetter(p), func() string { return p.Currency }, func() string { return p.ID }
	})

	found := false
	for _, a := range formation.Widgets[0].Atoms {
		if a.Subtype == domain.SubtypeCurrency {
			found = true
			if a.Meta == nil {
				t.Error("currency atom missing meta")
			} else if a.Meta["currency"] != "USD" {
				t.Errorf("want currency USD, got %v", a.Meta["currency"])
			}
		}
	}
	if !found {
		t.Error("no currency atom found")
	}
}

func TestBuildFormation_EmptyCurrencyFallback(t *testing.T) {
	preset := presets.ProductGridPreset
	p := domain.Product{ID: "p1", Name: "Test", Price: 100, Currency: ""}

	formation := BuildFormation(preset, 1, func(i int) (FieldGetter, CurrencyGetter, IDGetter) {
		return productFieldGetter(p), func() string { return "" }, func() string { return p.ID }
	})

	for _, a := range formation.Widgets[0].Atoms {
		if a.Subtype == domain.SubtypeCurrency && a.Meta != nil {
			if a.Meta["currency"] != "$" {
				t.Errorf("empty currency should fallback to $, got %v", a.Meta["currency"])
			}
		}
	}
}

func TestBuildFormation_Services(t *testing.T) {
	preset := presets.ServiceCardPreset
	services := testServices(3)

	formation := BuildFormation(preset, len(services), func(i int) (FieldGetter, CurrencyGetter, IDGetter) {
		s := services[i]
		return serviceFieldGetter(s), func() string { return s.Currency }, func() string { return s.ID }
	})

	if formation.Mode != domain.FormationTypeGrid {
		t.Errorf("want mode grid, got %s", formation.Mode)
	}
	if len(formation.Widgets) != 3 {
		t.Fatalf("want 3 widgets, got %d", len(formation.Widgets))
	}
}

// --- BuildTemplateFormation tests ---

func TestBuildTemplateFormation_AllValuesNil(t *testing.T) {
	preset := presets.ProductGridPreset
	tmpl := BuildTemplateFormation(preset)

	if len(tmpl.Widgets) != 1 {
		t.Fatalf("want 1 template widget, got %d", len(tmpl.Widgets))
	}

	w := tmpl.Widgets[0]
	if w.ID != "template" {
		t.Errorf("want widget ID 'template', got %q", w.ID)
	}

	for _, a := range w.Atoms {
		if a.Value != nil {
			t.Errorf("template atom %q should have nil value, got %v", a.FieldName, a.Value)
		}
	}
}

func TestBuildTemplateFormation_FieldNamesPopulated(t *testing.T) {
	preset := presets.ProductGridPreset
	tmpl := BuildTemplateFormation(preset)

	atoms := tmpl.Widgets[0].Atoms
	if len(atoms) == 0 {
		t.Fatal("no atoms in template")
	}

	for _, a := range atoms {
		if a.FieldName == "" {
			t.Error("template atom missing FieldName")
		}
	}
}

func TestBuildTemplateFormation_CurrencySentinel(t *testing.T) {
	preset := presets.ProductGridPreset
	tmpl := BuildTemplateFormation(preset)

	found := false
	for _, a := range tmpl.Widgets[0].Atoms {
		if a.Subtype == domain.SubtypeCurrency {
			found = true
			if a.Meta == nil || a.Meta["currency"] != "__ENTITY_CURRENCY__" {
				t.Errorf("currency atom should have sentinel, got %v", a.Meta)
			}
		}
	}
	if !found {
		t.Error("no currency atom in template")
	}
}

func TestBuildTemplateFormation_ImageMeta(t *testing.T) {
	preset := presets.ProductGridPreset
	tmpl := BuildTemplateFormation(preset)

	for _, a := range tmpl.Widgets[0].Atoms {
		if a.Type == domain.AtomTypeImage {
			if a.Meta == nil || a.Meta["size"] != "large" {
				t.Errorf("image atom should have size:large meta, got %v", a.Meta)
			}
		}
	}
}

// --- Field getter tests ---

func TestProductFieldGetter_AllFields(t *testing.T) {
	p := domain.Product{
		ID:            "p1",
		Name:          "Nike Air",
		Description:   "Running shoe",
		Price:         150000,
		Brand:         "Nike",
		Category:      "Sneakers",
		Rating:        4.5,
		StockQuantity: 42,
		Images:        []string{"img.jpg"},
		Tags:          []string{"new"},
		Attributes:    map[string]interface{}{"color": "black"},
	}

	fg := productFieldGetter(p)

	tests := []struct {
		field string
		want  interface{}
	}{
		{"id", "p1"},
		{"name", "Nike Air"},
		{"description", "Running shoe"},
		{"price", 150000},
		{"brand", "Nike"},
		{"category", "Sneakers"},
	}

	for _, tc := range tests {
		val := fg(tc.field)
		if val != tc.want {
			t.Errorf("productFieldGetter(%q): want %v, got %v", tc.field, tc.want, val)
		}
	}

	if fg("rating") != 4.5 {
		t.Errorf("rating: want 4.5, got %v", fg("rating"))
	}
	if fg("stockQuantity") != 42 {
		t.Errorf("stockQuantity: want 42, got %v", fg("stockQuantity"))
	}
}

func TestProductFieldGetter_EmptyFieldsReturnNil(t *testing.T) {
	p := domain.Product{ID: "p1"}
	fg := productFieldGetter(p)

	nilFields := []string{"name", "description", "brand", "category", "images", "tags", "attributes"}
	for _, f := range nilFields {
		if fg(f) != nil {
			t.Errorf("empty product field %q should return nil, got %v", f, fg(f))
		}
	}
}

func TestProductFieldGetter_ZeroRatingReturnsNil(t *testing.T) {
	p := domain.Product{ID: "p1", Rating: 0}
	fg := productFieldGetter(p)
	if fg("rating") != nil {
		t.Errorf("zero rating should return nil, got %v", fg("rating"))
	}
}

func TestProductFieldGetter_UnknownField(t *testing.T) {
	p := domain.Product{ID: "p1"}
	fg := productFieldGetter(p)
	if fg("nonexistent") != nil {
		t.Errorf("unknown field should return nil, got %v", fg("nonexistent"))
	}
}

func TestServiceFieldGetter_AllFields(t *testing.T) {
	s := domain.Service{
		ID:           "s1",
		Name:         "Yoga Class",
		Description:  "Relaxing yoga",
		Price:        50000,
		Duration:     "1 hour",
		Provider:     "YogaSpace",
		Availability: "available",
		Rating:       4.8,
		Images:       []string{"img.jpg"},
		Attributes:   map[string]interface{}{"level": "beginner"},
	}

	fg := serviceFieldGetter(s)

	if fg("name") != "Yoga Class" {
		t.Errorf("name: want Yoga Class, got %v", fg("name"))
	}
	if fg("duration") != "1 hour" {
		t.Errorf("duration: want '1 hour', got %v", fg("duration"))
	}
	if fg("provider") != "YogaSpace" {
		t.Errorf("provider: want YogaSpace, got %v", fg("provider"))
	}
	if fg("availability") != "available" {
		t.Errorf("availability: want available, got %v", fg("availability"))
	}
}

func TestServiceFieldGetter_EmptyFieldsReturnNil(t *testing.T) {
	s := domain.Service{ID: "s1"}
	fg := serviceFieldGetter(s)

	nilFields := []string{"name", "description", "duration", "provider", "availability", "images", "attributes"}
	for _, f := range nilFields {
		if fg(f) != nil {
			t.Errorf("empty service field %q should return nil, got %v", f, fg(f))
		}
	}
}

func TestServiceFieldGetter_UnknownField(t *testing.T) {
	s := domain.Service{ID: "s1"}
	fg := serviceFieldGetter(s)
	if fg("brand") != nil {
		t.Errorf("services don't have 'brand', should return nil")
	}
}

// --- formatPrice is in postgres adapter, not here ---
// --- nonEmpty helper test ---

func TestNonEmpty_EmptyString(t *testing.T) {
	if nonEmpty("") != nil {
		t.Error("nonEmpty('') should return nil")
	}
}

func TestNonEmpty_NonEmptyString(t *testing.T) {
	if nonEmpty("hello") != "hello" {
		t.Error("nonEmpty('hello') should return 'hello'")
	}
}

// --- helpers ---

func testProducts(n int) []domain.Product {
	products := make([]domain.Product, n)
	for i := 0; i < n; i++ {
		products[i] = domain.Product{
			ID:       "prod-" + string(rune('A'+i)),
			Name:     "Product " + string(rune('A'+i)),
			Price:    (i + 1) * 10000,
			Currency: "RUB",
			Brand:    "TestBrand",
			Images:   []string{"img.jpg"},
			Rating:   4.0 + float64(i)/10,
		}
	}
	return products
}

func testServices(n int) []domain.Service {
	services := make([]domain.Service, n)
	for i := 0; i < n; i++ {
		services[i] = domain.Service{
			ID:       "svc-" + string(rune('A'+i)),
			Name:     "Service " + string(rune('A'+i)),
			Price:    (i + 1) * 5000,
			Currency: "RUB",
			Duration: "30 min",
			Provider: "Provider",
		}
	}
	return services
}
