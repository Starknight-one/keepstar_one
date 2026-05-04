package tools

import (
	"strings"

	"keepstar_v5/internal/domain"
)

// NormalizeProduct cleans up a product fetched from the catalog before it
// reaches the LLM context: trims surrounding whitespace from text fields,
// dedupes case-insensitive duplicates from string slices, and drops empty
// image URLs. Mirrors V4 internal/tools/data_normalize.go.
//
// Reasoning: master_products data accumulates duplicates over time (Shopify
// re-imports, manual edits, vendor feeds). Without dedup the LLM sees noisy
// repeated tags / ingredients / skin types and may anchor on phantom signals
// or waste tokens. V5 has no Service entity so the V4 NormalizeService
// helper is intentionally not ported.
func NormalizeProduct(p *domain.Product) {
	if p == nil {
		return
	}
	p.Name = strings.TrimSpace(p.Name)
	p.Brand = strings.TrimSpace(p.Brand)
	p.Category = strings.TrimSpace(p.Category)
	p.Description = strings.TrimSpace(p.Description)
	p.ProductForm = strings.TrimSpace(p.ProductForm)

	p.Tags = dedupStrings(p.Tags)
	p.SkinType = dedupStrings(p.SkinType)
	p.Concern = dedupStrings(p.Concern)
	p.KeyIngredients = dedupStrings(p.KeyIngredients)
	p.Images = filterEmptyStrings(p.Images)
}

// dedupStrings removes case-insensitive duplicates while preserving the
// first occurrence's original casing. Empty strings are dropped.
func dedupStrings(items []string) []string {
	if len(items) == 0 {
		return items
	}
	seen := make(map[string]bool, len(items))
	result := make([]string, 0, len(items))
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, trimmed)
	}
	return result
}

// filterEmptyStrings removes empty / whitespace-only entries from a slice
// while keeping the rest in original order. Used to scrub broken image URLs
// before products land in state.
func filterEmptyStrings(items []string) []string {
	if len(items) == 0 {
		return items
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item) != "" {
			result = append(result, item)
		}
	}
	return result
}
