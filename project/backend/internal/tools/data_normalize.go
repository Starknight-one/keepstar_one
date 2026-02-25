package tools

import (
	"strings"

	"keepstar/internal/domain"
)

// NormalizeProduct cleans up product data: trims strings, deduplicates arrays, filters empty images
func NormalizeProduct(p *domain.Product) {
	p.Name = strings.TrimSpace(p.Name)
	p.Brand = strings.TrimSpace(p.Brand)
	p.Category = strings.TrimSpace(p.Category)
	p.Description = strings.TrimSpace(p.Description)
	p.ProductForm = strings.TrimSpace(p.ProductForm)

	p.Tags = dedup(p.Tags)
	p.SkinType = dedup(p.SkinType)
	p.Concern = dedup(p.Concern)
	p.KeyIngredients = dedup(p.KeyIngredients)
	p.Images = filterEmpty(p.Images)
}

// NormalizeService cleans up service data: trims strings, filters empty images
func NormalizeService(s *domain.Service) {
	s.Name = strings.TrimSpace(s.Name)
	s.Category = strings.TrimSpace(s.Category)
	s.Description = strings.TrimSpace(s.Description)
	s.Provider = strings.TrimSpace(s.Provider)
	s.Duration = strings.TrimSpace(s.Duration)
	s.Images = filterEmpty(s.Images)
}

// dedup removes duplicate strings preserving order
func dedup(items []string) []string {
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
		lower := strings.ToLower(trimmed)
		if !seen[lower] {
			seen[lower] = true
			result = append(result, trimmed)
		}
	}
	return result
}

// filterEmpty removes empty/whitespace-only strings from a slice
func filterEmpty(items []string) []string {
	if len(items) == 0 {
		return items
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
