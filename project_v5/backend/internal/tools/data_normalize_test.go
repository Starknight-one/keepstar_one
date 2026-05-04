package tools

import (
	"reflect"
	"testing"

	"keepstar_v5/internal/domain"
)

func TestNormalizeProduct_TrimsAndDedupes(t *testing.T) {
	p := &domain.Product{
		Name:        "  Snail Cream  ",
		Brand:       " COSRX ",
		Description: "\nfor dry skin\n",
		ProductForm: "  cream  ",
		Tags:        []string{"hydration", "Hydration", "  ", "anti-aging", "anti-aging "},
		SkinType:    []string{"dry", "DRY", "sensitive"},
		Images:      []string{"https://a/1.jpg", "", "  ", "https://a/2.jpg"},
	}
	NormalizeProduct(p)

	if p.Name != "Snail Cream" {
		t.Errorf("Name=%q, want trimmed", p.Name)
	}
	if p.Brand != "COSRX" {
		t.Errorf("Brand=%q, want trimmed", p.Brand)
	}
	if p.Description != "for dry skin" {
		t.Errorf("Description=%q, want trimmed", p.Description)
	}
	if !reflect.DeepEqual(p.Tags, []string{"hydration", "anti-aging"}) {
		t.Errorf("Tags=%v, want dedup case-insensitive", p.Tags)
	}
	if !reflect.DeepEqual(p.SkinType, []string{"dry", "sensitive"}) {
		t.Errorf("SkinType=%v, want dedup case-insensitive", p.SkinType)
	}
	if !reflect.DeepEqual(p.Images, []string{"https://a/1.jpg", "https://a/2.jpg"}) {
		t.Errorf("Images=%v, want empty filtered", p.Images)
	}
}

func TestNormalizeProduct_NilSafe(t *testing.T) {
	NormalizeProduct(nil) // must not panic
}

func TestRRFMerge_KeywordOnly(t *testing.T) {
	keyword := []domain.Product{{ID: "a"}, {ID: "b"}}
	got := rrfMerge(keyword, nil, 10, false)
	if len(got) != 2 || got[0].ID != "a" || got[1].ID != "b" {
		t.Errorf("got=%v, want [a, b]", got)
	}
}

func TestRRFMerge_VectorOnly(t *testing.T) {
	vector := []domain.Product{{ID: "x"}, {ID: "y"}}
	got := rrfMerge(nil, vector, 10, false)
	if len(got) != 2 || got[0].ID != "x" || got[1].ID != "y" {
		t.Errorf("got=%v, want [x, y]", got)
	}
}

func TestRRFMerge_KeywordOutweighsVector(t *testing.T) {
	// Same product appears in both lists. Keyword weight (1.5x) should
	// rank a keyword-only hit above a vector-only hit at equal rank.
	keyword := []domain.Product{{ID: "kw1"}}
	vector := []domain.Product{{ID: "vec1"}}
	got := rrfMerge(keyword, vector, 10, false)
	if len(got) != 2 {
		t.Fatalf("len=%d, want 2", len(got))
	}
	if got[0].ID != "kw1" {
		t.Errorf("kw1 should rank above vec1 at equal index, got order %v", []string{got[0].ID, got[1].ID})
	}
}

func TestRRFMerge_FiltersBoostKeyword(t *testing.T) {
	// At identical ranks, hasFilters=true raises keyword from 1.5x to 2.0x;
	// that doesn't change ordering (both still > 1.0x vector) but it should
	// bury vector-only hits further. Sanity test: ensure the merged set
	// still surfaces all unique IDs.
	keyword := []domain.Product{{ID: "k"}}
	vector := []domain.Product{{ID: "v"}}
	got := rrfMerge(keyword, vector, 10, true)
	if len(got) != 2 {
		t.Fatalf("hasFilters merge len=%d, want 2", len(got))
	}
	if got[0].ID != "k" {
		t.Errorf("hasFilters: keyword should top, got %s", got[0].ID)
	}
}

func TestRRFMerge_LimitTruncates(t *testing.T) {
	keyword := []domain.Product{{ID: "a"}, {ID: "b"}, {ID: "c"}, {ID: "d"}}
	got := rrfMerge(keyword, nil, 2, false)
	if len(got) != 2 {
		t.Errorf("len=%d, want truncated to 2", len(got))
	}
}
