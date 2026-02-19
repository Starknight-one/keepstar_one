package domain

import (
	"strings"
	"testing"
	"time"
)

func TestCatalogDigest_ToPromptText_Basic(t *testing.T) {
	d := &CatalogDigest{
		GeneratedAt:   time.Now(),
		TotalProducts: 962,
		CategoryTree: []DigestCategoryGroup{
			{Name: "face-care", Slug: "face-care", Children: []DigestCategoryLeaf{
				{Name: "Очищение", Slug: "cleansing", Count: 120},
				{Name: "Тонизирование", Slug: "toning", Count: 85},
			}},
			{Name: "makeup", Slug: "makeup", Children: []DigestCategoryLeaf{
				{Name: "Лицо", Slug: "makeup-face", Count: 40},
			}},
		},
		SharedFilters: []DigestSharedFilter{
			{Key: "skin_type", Values: []string{"dry", "normal", "oily"}},
			{Key: "concern", Values: []string{"acne", "hydration"}},
		},
		TopBrands:      []string{"COSRX", "MEDI-PEEL"},
		TopIngredients: []string{"hyaluronic-acid", "niacinamide"},
	}

	result := d.ToPromptText()

	// Header
	if !strings.Contains(result, "962 products") {
		t.Errorf("expected '962 products', got: %s", result)
	}
	// Category tree
	if !strings.Contains(result, "face-care: cleansing(120), toning(85)") {
		t.Errorf("expected category tree line, got: %s", result)
	}
	if !strings.Contains(result, "makeup: makeup-face(40)") {
		t.Errorf("expected makeup line, got: %s", result)
	}
	// Shared filters
	if !strings.Contains(result, "skin_type: dry|normal|oily") {
		t.Errorf("expected skin_type filter, got: %s", result)
	}
	// Brands
	if !strings.Contains(result, "brands(2): COSRX, MEDI-PEEL") {
		t.Errorf("expected brands line, got: %s", result)
	}
	// Ingredients
	if !strings.Contains(result, "ingredients(2): hyaluronic-acid, niacinamide") {
		t.Errorf("expected ingredients line, got: %s", result)
	}
	// Search rule
	if !strings.Contains(result, "enum value") {
		t.Errorf("expected search rule, got: %s", result)
	}
}

func TestCatalogDigest_ToPromptText_Empty(t *testing.T) {
	d := &CatalogDigest{
		GeneratedAt:   time.Now(),
		TotalProducts: 0,
	}
	if result := d.ToPromptText(); result != "" {
		t.Errorf("expected empty for 0 products, got: %q", result)
	}

	var nilDigest *CatalogDigest
	if result := nilDigest.ToPromptText(); result != "" {
		t.Errorf("expected empty for nil digest, got: %q", result)
	}
}

func TestCatalogDigest_ToPromptText_Compact(t *testing.T) {
	// Build a realistic digest and verify it's under 500 tokens (~2000 chars)
	d := &CatalogDigest{
		GeneratedAt:   time.Now(),
		TotalProducts: 962,
		CategoryTree: []DigestCategoryGroup{
			{Name: "face-care", Slug: "face-care", Children: []DigestCategoryLeaf{
				{Slug: "cleansing", Count: 120},
				{Slug: "toning", Count: 85},
				{Slug: "serums", Count: 110},
				{Slug: "moisturizing", Count: 95},
				{Slug: "suncare", Count: 30},
				{Slug: "masks", Count: 55},
				{Slug: "exfoliation", Count: 45},
				{Slug: "spot-treatment", Count: 40},
				{Slug: "essences", Count: 35},
				{Slug: "lip-care", Count: 25},
			}},
			{Name: "makeup", Slug: "makeup", Children: []DigestCategoryLeaf{
				{Slug: "makeup-face", Count: 40},
				{Slug: "makeup-eyes", Count: 30},
				{Slug: "makeup-lips", Count: 20},
				{Slug: "makeup-setting", Count: 10},
			}},
			{Name: "body", Slug: "body", Children: []DigestCategoryLeaf{
				{Slug: "body-cleansing", Count: 15},
				{Slug: "body-moisturizing", Count: 15},
				{Slug: "body-fragrance", Count: 10},
			}},
			{Name: "hair", Slug: "hair", Children: []DigestCategoryLeaf{
				{Slug: "hair-shampoo", Count: 10},
				{Slug: "hair-conditioner", Count: 7},
				{Slug: "hair-treatment", Count: 5},
			}},
		},
		SharedFilters: []DigestSharedFilter{
			{Key: "product_form", Values: []string{"cream", "gel", "serum", "toner", "essence", "lotion", "oil", "balm", "foam", "mousse", "mist", "spray", "powder", "stick", "patch", "sheet-mask", "wash-off-mask", "peel", "scrub", "soap"}},
			{Key: "skin_type", Values: []string{"normal", "dry", "oily", "combination", "sensitive", "acne-prone", "mature"}},
			{Key: "concern", Values: []string{"hydration", "anti-aging", "brightening", "acne", "pores", "dark-spots", "redness", "sun-protection", "exfoliation", "firmness", "dark-circles", "lip-dryness", "oil-control", "texture", "dullness"}},
			{Key: "routine_step", Values: []string{"cleansing", "toning", "exfoliation", "treatment", "moisturizing", "sun-protection", "makeup"}},
			{Key: "texture", Values: []string{"watery", "gel", "milky", "creamy", "thick", "oily", "powdery", "foamy", "balmy"}},
			{Key: "target_area", Values: []string{"face", "eye-area", "lips", "neck", "body", "hands", "feet", "scalp"}},
		},
		TopBrands:      []string{"COSRX", "MEDI-PEEL", "Holika Holika", "The Ordinary", "La Roche-Posay"},
		TopIngredients: []string{"hyaluronic-acid", "niacinamide", "retinol", "vitamin-c", "centella-asiatica"},
	}

	result := d.ToPromptText()

	// Should be well under 2000 chars (~500 tokens)
	if len(result) > 2000 {
		t.Errorf("digest too long: %d chars (max 2000), content:\n%s", len(result), result)
	}
	t.Logf("Digest size: %d chars\n%s", len(result), result)
}
