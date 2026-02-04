package tools

import (
	"context"
	"fmt"

	"keepstar/internal/domain"
	"keepstar/internal/ports"
)

// SearchProductsTool searches products and writes to state
type SearchProductsTool struct {
	statePort   ports.StatePort
	catalogPort ports.CatalogPort
}

// NewSearchProductsTool creates the tool
func NewSearchProductsTool(statePort ports.StatePort, catalogPort ports.CatalogPort) *SearchProductsTool {
	return &SearchProductsTool{
		statePort:   statePort,
		catalogPort: catalogPort,
	}
}

// Definition returns the tool definition for LLM
func (t *SearchProductsTool) Definition() domain.ToolDefinition {
	return domain.ToolDefinition{
		Name:        "search_products",
		Description: "Search for products by query. Results are written to state, not returned directly.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"query": map[string]interface{}{
					"type":        "string",
					"description": "Search query (e.g., 'ноутбуки', 'Nike shoes')",
				},
				"category": map[string]interface{}{
					"type":        "string",
					"description": "Category filter (optional)",
				},
				"brand": map[string]interface{}{
					"type":        "string",
					"description": "Brand filter (optional)",
				},
				"min_price": map[string]interface{}{
					"type":        "number",
					"description": "Minimum price (optional)",
				},
				"max_price": map[string]interface{}{
					"type":        "number",
					"description": "Maximum price (optional)",
				},
				"limit": map[string]interface{}{
					"type":        "integer",
					"description": "Max results (default 10)",
				},
			},
			"required": []string{"query"},
		},
	}
}

// Execute searches products, writes to state, returns "ok" or "empty"
func (t *SearchProductsTool) Execute(ctx context.Context, sessionID string, input map[string]interface{}) (*domain.ToolResult, error) {
	// Parse input
	query, _ := input["query"].(string)
	category, _ := input["category"].(string)
	brand, _ := input["brand"].(string)
	minPrice := 0
	maxPrice := 0
	limit := 10

	if mp, ok := input["min_price"].(float64); ok {
		minPrice = int(mp)
	}
	if mp, ok := input["max_price"].(float64); ok {
		maxPrice = int(mp)
	}
	if l, ok := input["limit"].(float64); ok {
		limit = int(l)
	}

	// Build filter
	// Note: If brand is specified, don't use query as search to avoid AND conflict
	// The query often contains the brand name which would cause double filtering
	filter := ports.ProductFilter{
		Brand:    brand,
		MinPrice: minPrice,
		MaxPrice: maxPrice,
		Limit:    limit,
	}
	// Only use query as search if no brand specified
	if brand == "" {
		filter.Search = query
	}
	// CategoryID is string in ProductFilter, use category as-is
	if category != "" {
		filter.CategoryID = category
	}

	// Get state (or create if not exists)
	state, err := t.statePort.GetState(ctx, sessionID)
	if err == domain.ErrSessionNotFound {
		state, err = t.statePort.CreateState(ctx, sessionID)
	}
	if err != nil {
		return nil, fmt.Errorf("get/create state: %w", err)
	}

	// Get tenant from state (set by Agent1 from request header X-Tenant-Slug)
	// Falls back to "nike" if not set
	tenantSlug := "nike"
	if state.Current.Meta.Aliases != nil {
		if slug, ok := state.Current.Meta.Aliases["tenant_slug"]; ok && slug != "" {
			tenantSlug = slug
		}
	}

	// Resolve tenant ID from slug
	tenant, err := t.catalogPort.GetTenantBySlug(ctx, tenantSlug)
	if err != nil {
		return nil, fmt.Errorf("get tenant: %w", err)
	}

	// Search products
	products, total, err := t.catalogPort.ListProducts(ctx, tenant.ID, filter)
	if err != nil {
		return nil, fmt.Errorf("search products: %w", err)
	}

	if total == 0 {
		return &domain.ToolResult{Content: "empty"}, nil
	}

	// Extract field names from first product
	fields := extractProductFields(products[0])

	// Update state with products
	state.Current.Data.Products = products
	state.Current.Meta = domain.StateMeta{
		Count:   total,
		Fields:  fields,
		Aliases: make(map[string]string),
	}
	if err := t.statePort.UpdateState(ctx, state); err != nil {
		return nil, fmt.Errorf("update state: %w", err)
	}

	return &domain.ToolResult{
		Content: fmt.Sprintf("ok: found %d products", total),
	}, nil
}

// extractProductFields gets field names from a product
func extractProductFields(p domain.Product) []string {
	fields := []string{"id", "name", "price"}
	if p.Description != "" {
		fields = append(fields, "description")
	}
	if p.Brand != "" {
		fields = append(fields, "brand")
	}
	if p.Category != "" {
		fields = append(fields, "category")
	}
	if p.Rating > 0 {
		fields = append(fields, "rating")
	}
	if len(p.Images) > 0 {
		fields = append(fields, "images")
	}
	return fields
}
