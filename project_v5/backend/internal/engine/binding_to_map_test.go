package engine

import (
	"reflect"
	"testing"

	"keepstar_v5/internal/domain"
)

func TestProductToMapHeybabesShape(t *testing.T) {
	p := domain.Product{
		ID:             "p1",
		Name:           "COSRX BHA Power",
		Price:          32900,
		PriceFormatted: "329 ₽",
		Currency:       "RUB",
		Brand:          "COSRX",
		Rating:         4.5,
		Images:         []string{"https://cdn/img1.jpg", "https://cdn/img2.jpg"},
		SkinType:       []string{"oily", "combination"},
		Concern:        []string{"acne"},
	}
	m := ProductToMap(p)

	if m["name"] != "COSRX BHA Power" {
		t.Errorf("name: %v", m["name"])
	}
	if m["price"] != 32900 {
		t.Errorf("price: %v", m["price"])
	}
	if m["heroImage"] != "https://cdn/img1.jpg" {
		t.Errorf("heroImage: %v", m["heroImage"])
	}
	if !reflect.DeepEqual(m["skinType"], []string{"oily", "combination"}) {
		t.Errorf("skinType: %v", m["skinType"])
	}
}

func TestProductToMapElectronicsExtra(t *testing.T) {
	p := domain.Product{
		ID:    "p1",
		Name:  "MacBook Pro 14",
		Price: 199900,
		Extra: map[string]any{
			"cpu":           "Apple M3 Pro",
			"ram":           "18GB",
			"battery_life":  "18 hours",
			"manufacturer":  "Apple",
			"cover_image":   "https://cdn/mbp14.webp",
		},
	}
	m := ProductToMap(p)

	for _, key := range []string{"cpu", "ram", "battery_life", "manufacturer", "cover_image"} {
		if _, ok := m[key]; !ok {
			t.Errorf("Extra key %q lost in map", key)
		}
	}
	if m["cpu"] != "Apple M3 Pro" {
		t.Errorf("cpu: %v", m["cpu"])
	}
}

// TestProductToMapTier2WinsOverExtra documents the V5 precedence flip vs V4.
// Same key in both Tier2 (curator) and Extra (vendor): Tier2 wins.
func TestProductToMapTier2WinsOverExtra(t *testing.T) {
	p := domain.Product{
		ID:   "p1",
		Name: "MacBook Pro 14",
		Tier2: map[string]any{
			"manufacturer": "Apple Inc.", // curator-corrected canonical
		},
		Extra: map[string]any{
			"manufacturer": "Apple", // raw from vendor feed
		},
	}
	m := ProductToMap(p)
	if m["manufacturer"] != "Apple Inc." {
		t.Errorf("Tier2 should win over Extra; got %v, want \"Apple Inc.\"", m["manufacturer"])
	}
}

// TestProductToMapTypedWinsOverTier2 documents that typed PIM columns
// (canonical) always win over both Tier2 and Extra.
func TestProductToMapTypedWinsOverTier2(t *testing.T) {
	p := domain.Product{
		ID:    "p1",
		Brand: "COSRX", // typed column
		Tier2: map[string]any{
			"brand": "Different Brand", // curator-side, but typed wins
		},
	}
	m := ProductToMap(p)
	if m["brand"] != "COSRX" {
		t.Errorf("typed should win over Tier2; got %v", m["brand"])
	}
}

func TestProductToMapEmptyProduct(t *testing.T) {
	p := domain.Product{ID: "p1"}
	m := ProductToMap(p)
	// Only stockQuantity is always written (0 is a meaningful signal).
	if len(m) != 1 || m["stockQuantity"] != 0 {
		t.Errorf("empty product should yield {stockQuantity:0}, got %v", m)
	}
}

func TestServiceToMap(t *testing.T) {
	s := domain.Service{
		ID:       "s1",
		Name:     "Haircut",
		Price:    150000,
		Duration: "30 min",
		Provider: "Salon X",
		Attributes: map[string]any{
			"specialist": "Anna",
			"category":   "from-attributes", // overridden by typed below
		},
		Category: "Beauty",
		Extra: map[string]any{
			"specialist": "Different",       // loses to Attributes
			"booking_url": "https://book.io", // unique → kept
		},
	}
	m := ServiceToMap(s)
	if m["name"] != "Haircut" || m["duration"] != "30 min" {
		t.Errorf("typed fields drift: %+v", m)
	}
	if m["category"] != "Beauty" {
		t.Errorf("typed should win over Attributes for category: %v", m["category"])
	}
	if m["specialist"] != "Anna" {
		t.Errorf("Attributes should win over Extra for specialist: %v", m["specialist"])
	}
	if m["booking_url"] != "https://book.io" {
		t.Errorf("Extra-only key lost: %v", m)
	}
}
