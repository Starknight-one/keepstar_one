package tools

import (
	"testing"

	"keepstar/internal/domain"
)

func TestRRFMerge_BothEmpty(t *testing.T) {
	result := rrfMerge(nil, nil, 10, false)
	if len(result) != 0 {
		t.Errorf("both empty: want 0, got %d", len(result))
	}
}

func TestRRFMerge_OnlyKeyword(t *testing.T) {
	keyword := []domain.Product{
		{ID: "k1", Name: "K1"},
		{ID: "k2", Name: "K2"},
	}
	result := rrfMerge(keyword, nil, 10, false)
	if len(result) != 2 {
		t.Fatalf("only keyword: want 2, got %d", len(result))
	}
	if result[0].ID != "k1" {
		t.Errorf("first should be k1, got %s", result[0].ID)
	}
}

func TestRRFMerge_OnlyVector(t *testing.T) {
	vector := []domain.Product{
		{ID: "v1", Name: "V1"},
		{ID: "v2", Name: "V2"},
	}
	result := rrfMerge(nil, vector, 10, false)
	if len(result) != 2 {
		t.Fatalf("only vector: want 2, got %d", len(result))
	}
	if result[0].ID != "v1" {
		t.Errorf("first should be v1, got %s", result[0].ID)
	}
}

func TestRRFMerge_OverlapBoosted(t *testing.T) {
	keyword := []domain.Product{
		{ID: "shared", Name: "Shared"},
		{ID: "k-only", Name: "K Only"},
	}
	vector := []domain.Product{
		{ID: "shared", Name: "Shared"},
		{ID: "v-only", Name: "V Only"},
	}

	result := rrfMerge(keyword, vector, 10, false)

	// "shared" should be ranked first (appears in both lists)
	if len(result) < 1 {
		t.Fatal("no results")
	}
	if result[0].ID != "shared" {
		t.Errorf("overlapping item should rank first, got %s", result[0].ID)
	}
	if len(result) != 3 {
		t.Errorf("want 3 unique results, got %d", len(result))
	}
}

func TestRRFMerge_KeywordWeight15xDefault(t *testing.T) {
	// Same item at rank 0 in keyword vs rank 0 in vector
	// keyword score: 1.5/(60+0+1) = 1.5/61
	// vector score: 1.0/(60+0+1) = 1.0/61
	// keyword contributes 50% more per rank
	keyword := []domain.Product{
		{ID: "k1", Name: "K1"},
	}
	vector := []domain.Product{
		{ID: "v1", Name: "V1"},
	}

	result := rrfMerge(keyword, vector, 10, false)
	if len(result) != 2 {
		t.Fatalf("want 2, got %d", len(result))
	}
	// k1 has 1.5x weight, so should rank higher
	if result[0].ID != "k1" {
		t.Errorf("keyword item with 1.5x weight should rank first, got %s", result[0].ID)
	}
}

func TestRRFMerge_KeywordWeight20xWithFilters(t *testing.T) {
	keyword := []domain.Product{
		{ID: "k1", Name: "K1"},
	}
	vector := []domain.Product{
		{ID: "v1", Name: "V1"},
	}

	result := rrfMerge(keyword, vector, 10, true) // hasFilters=true
	if len(result) != 2 {
		t.Fatalf("want 2, got %d", len(result))
	}
	if result[0].ID != "k1" {
		t.Errorf("keyword with 2.0x filter weight should rank first, got %s", result[0].ID)
	}
}

func TestRRFMerge_LimitApplied(t *testing.T) {
	keyword := make([]domain.Product, 10)
	for i := range keyword {
		keyword[i] = domain.Product{ID: "k" + string(rune('0'+i))}
	}
	result := rrfMerge(keyword, nil, 3, false)
	if len(result) != 3 {
		t.Errorf("limit 3: want 3, got %d", len(result))
	}
}

func TestRRFMerge_StableOrder(t *testing.T) {
	keyword := []domain.Product{
		{ID: "a"}, {ID: "b"}, {ID: "c"},
	}
	vector := []domain.Product{
		{ID: "a"}, {ID: "b"}, {ID: "c"},
	}

	// Run multiple times to check stability
	var firstOrder []string
	for run := 0; run < 10; run++ {
		result := rrfMerge(keyword, vector, 10, false)
		order := make([]string, len(result))
		for i, p := range result {
			order[i] = p.ID
		}
		if firstOrder == nil {
			firstOrder = order
		} else {
			for i, id := range order {
				if id != firstOrder[i] {
					t.Errorf("run %d: unstable order at %d: want %s, got %s", run, i, firstOrder[i], id)
				}
			}
		}
	}
}

// --- rrfMergeServices tests ---

func TestRRFMergeServices_BothEmpty(t *testing.T) {
	result := rrfMergeServices(nil, nil, 10, false)
	if result != nil {
		t.Errorf("both empty: want nil, got %v", result)
	}
}

func TestRRFMergeServices_OnlyKeyword(t *testing.T) {
	keyword := []domain.Service{
		{ID: "s1", Name: "S1"},
		{ID: "s2", Name: "S2"},
	}
	result := rrfMergeServices(keyword, nil, 10, false)
	if len(result) != 2 {
		t.Fatalf("want 2, got %d", len(result))
	}
}

// --- catalogExtractProductFields / catalogExtractServiceFields ---

func TestCatalogExtractProductFields_AllPresent(t *testing.T) {
	p := domain.Product{
		ID:          "p1",
		Name:        "Shoe",
		Price:       100,
		Description: "Nice",
		Brand:       "Nike",
		Category:    "Sneakers",
		Rating:      4.5,
		Images:      []string{"img.jpg"},
	}

	fields := catalogExtractProductFields(p)

	required := []string{"id", "name", "price"}
	for _, r := range required {
		found := false
		for _, f := range fields {
			if f == r {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing required field %q", r)
		}
	}

	// Optional fields should be present when set
	optionals := map[string]bool{"description": true, "brand": true, "category": true, "rating": true, "images": true}
	for _, f := range fields {
		delete(optionals, f)
	}
	for missing := range optionals {
		t.Errorf("optional field %q should be present", missing)
	}
}

func TestCatalogExtractProductFields_MinimalProduct(t *testing.T) {
	p := domain.Product{ID: "p1", Name: "Shoe", Price: 100}
	fields := catalogExtractProductFields(p)

	if len(fields) != 3 {
		t.Errorf("minimal product: want 3 fields, got %d: %v", len(fields), fields)
	}
}

func TestCatalogExtractServiceFields_AllPresent(t *testing.T) {
	s := domain.Service{
		ID:          "s1",
		Name:        "Yoga",
		Price:       100,
		Description: "Relaxing",
		Category:    "Wellness",
		Duration:    "1h",
		Provider:    "Studio",
		Rating:      4.5,
		Images:      []string{"img.jpg"},
	}

	fields := catalogExtractServiceFields(s)

	required := []string{"id", "name", "price"}
	for _, r := range required {
		found := false
		for _, f := range fields {
			if f == r {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing required field %q", r)
		}
	}
}
