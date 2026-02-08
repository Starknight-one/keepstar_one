package prompts

import (
	"strings"
	"testing"
	"time"

	"keepstar/internal/domain"
)

func TestBuildAgent1ContextPrompt_WithDigest(t *testing.T) {
	digest := &domain.CatalogDigest{
		GeneratedAt:   time.Now(),
		TotalProducts: 100,
		Categories: []domain.DigestCategory{
			{
				Name:       "Running Shoes",
				Slug:       "running-shoes",
				Count:      45,
				PriceRange: [2]int{599000, 1899000},
				Params: []domain.DigestParam{
					{Key: "brand", Type: "enum", Cardinality: 5, Values: []string{"Nike", "Adidas", "Asics"}},
				},
			},
		},
	}

	meta := domain.StateMeta{ProductCount: 0, ServiceCount: 0}
	result := BuildAgent1ContextPrompt(meta, nil, "покажи кроссовки", digest)

	if !strings.Contains(result, "<catalog>") {
		t.Error("expected <catalog> block in output")
	}
	if !strings.Contains(result, "</catalog>") {
		t.Error("expected </catalog> closing tag")
	}
	if !strings.Contains(result, "Running Shoes") {
		t.Error("expected category name in catalog block")
	}
	if !strings.Contains(result, "покажи кроссовки") {
		t.Error("expected user query in output")
	}
	// Should NOT have <state> block since no data loaded
	if strings.Contains(result, "<state>") {
		t.Error("expected no <state> block when no data loaded")
	}
}

func TestBuildAgent1ContextPrompt_WithoutDigest(t *testing.T) {
	meta := domain.StateMeta{ProductCount: 5, ServiceCount: 0, Fields: []string{"id", "name", "price"}}
	result := BuildAgent1ContextPrompt(meta, nil, "покажи дешевле", nil)

	if strings.Contains(result, "<catalog>") {
		t.Error("expected no <catalog> block when digest is nil")
	}
	if !strings.Contains(result, "<state>") {
		t.Error("expected <state> block when data is loaded")
	}
	if !strings.Contains(result, "покажи дешевле") {
		t.Error("expected user query in output")
	}
}

func TestBuildAgent1ContextPrompt_DigestAndState(t *testing.T) {
	digest := &domain.CatalogDigest{
		GeneratedAt:   time.Now(),
		TotalProducts: 50,
		Categories: []domain.DigestCategory{
			{
				Name:       "Sneakers",
				Slug:       "sneakers",
				Count:      50,
				PriceRange: [2]int{300000, 900000},
			},
		},
	}

	meta := domain.StateMeta{
		ProductCount: 10,
		ServiceCount: 0,
		Fields:       []string{"id", "name", "price", "brand"},
	}
	result := BuildAgent1ContextPrompt(meta, nil, "а теперь Adidas", digest)

	// Both blocks should be present
	if !strings.Contains(result, "<catalog>") {
		t.Error("expected <catalog> block")
	}
	if !strings.Contains(result, "<state>") {
		t.Error("expected <state> block")
	}

	// Catalog should come before state
	catalogIdx := strings.Index(result, "<catalog>")
	stateIdx := strings.Index(result, "<state>")
	if catalogIdx > stateIdx {
		t.Errorf("expected <catalog> before <state>, catalog at %d, state at %d", catalogIdx, stateIdx)
	}

	// Query should come after state
	queryIdx := strings.Index(result, "а теперь Adidas")
	if queryIdx < stateIdx {
		t.Errorf("expected query after <state>, query at %d, state at %d", queryIdx, stateIdx)
	}
}
