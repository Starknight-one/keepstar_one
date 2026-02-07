package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"keepstar/internal/domain"
	"keepstar/internal/ports"
)

// CatalogSearchTool is a meta-tool that normalizes queries and searches the catalog
type CatalogSearchTool struct {
	statePort   ports.StatePort
	catalogPort ports.CatalogPort
	normalizer  *QueryNormalizer
}

// NewCatalogSearchTool creates the catalog search meta-tool
func NewCatalogSearchTool(statePort ports.StatePort, catalogPort ports.CatalogPort, normalizer *QueryNormalizer) *CatalogSearchTool {
	return &CatalogSearchTool{
		statePort:   statePort,
		catalogPort: catalogPort,
		normalizer:  normalizer,
	}
}

// Definition returns the tool definition for LLM
func (t *CatalogSearchTool) Definition() domain.ToolDefinition {
	return domain.ToolDefinition{
		Name:        "catalog_search",
		Description: "Search product catalog. Handles any language, slang, aliases. Pass user text as-is in query/brand fields.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"query": map[string]interface{}{
					"type":        "string",
					"description": "Search text in ANY language (e.g. 'кроссы', 'sneakers', 'ноутбук'). Normalized automatically.",
				},
				"brand": map[string]interface{}{
					"type":        "string",
					"description": "Brand in ANY language/transliteration (e.g. 'Найк', 'Nike', 'Самсунг'). Normalized automatically.",
				},
				"category": map[string]interface{}{
					"type":        "string",
					"description": "Category filter (optional)",
				},
				"min_price": map[string]interface{}{
					"type":        "number",
					"description": "Minimum price in RUBLES (optional). Example: 10000 means 10,000 rubles.",
				},
				"max_price": map[string]interface{}{
					"type":        "number",
					"description": "Maximum price in RUBLES (optional). Example: 50000 means 50,000 rubles.",
				},
				"sort_by": map[string]interface{}{
					"type":        "string",
					"enum":        []string{"price", "rating", "name"},
					"description": "Sort field (optional)",
				},
				"sort_order": map[string]interface{}{
					"type":        "string",
					"enum":        []string{"asc", "desc"},
					"description": "Sort direction (optional, default asc)",
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

// Execute runs the catalog search: normalize → filter → SQL → fallback cascade → state write
func (t *CatalogSearchTool) Execute(ctx context.Context, toolCtx ToolContext, input map[string]interface{}) (*domain.ToolResult, error) {
	// Trace metadata — collected throughout execution
	meta := map[string]interface{}{}

	// Parse input
	query, _ := input["query"].(string)
	brand, _ := input["brand"].(string)
	category, _ := input["category"].(string)
	sortBy, _ := input["sort_by"].(string)
	sortOrder, _ := input["sort_order"].(string)
	minPrice := 0
	maxPrice := 0
	limit := 10

	if v, ok := input["min_price"].(float64); ok {
		minPrice = int(v)
	}
	if v, ok := input["max_price"].(float64); ok {
		maxPrice = int(v)
	}
	if v, ok := input["limit"].(float64); ok {
		limit = int(v)
	}

	// Convert prices: rubles → kopecks (×100)
	minPriceKopecks := minPrice * 100
	maxPriceKopecks := maxPrice * 100
	if minPrice > 0 || maxPrice > 0 {
		meta["price_conversion"] = fmt.Sprintf("%d/%d руб → %d/%d коп", minPrice, maxPrice, minPriceKopecks, maxPriceKopecks)
	}

	// Normalize query and brand (fast path for English, LLM for other languages)
	normalizeStart := time.Now()
	normalized, err := t.normalizer.Normalize(ctx, query, brand)
	normalizeMs := time.Since(normalizeStart).Milliseconds()
	if err != nil {
		return nil, fmt.Errorf("normalize query: %w", err)
	}

	// Trace: normalize step
	normPath := "fast"
	if normalized.SourceLang != "en" {
		normPath = "llm"
	}
	meta["normalize_ms"] = normalizeMs
	meta["normalize_path"] = normPath
	meta["normalize_input"] = fmt.Sprintf("query=%q brand=%q", query, brand)
	meta["normalize_output"] = fmt.Sprintf("query=%q brand=%q lang=%s alias=%v", normalized.Query, normalized.Brand, normalized.SourceLang, normalized.AliasResolved)

	// Build filter from normalized result + declarative params
	filter := ports.ProductFilter{
		Search:     normalized.Query,
		Brand:      normalized.Brand,
		CategoryID: category,
		MinPrice:   minPriceKopecks,
		MaxPrice:   maxPriceKopecks,
		SortField:  sortBy,
		SortOrder:  sortOrder,
		Limit:      limit,
	}

	// Strip brand from search to avoid double filtering (AND conflict)
	if filter.Brand != "" && filter.Search != "" {
		cleaned := strings.TrimSpace(removeSubstringIgnoreCase(filter.Search, filter.Brand))
		if cleaned != "" {
			filter.Search = cleaned
		}
	}

	// Trace: final SQL filter
	meta["sql_filter"] = fmt.Sprintf("search=%q brand=%q cat=%q price=%d..%d sort=%s/%s limit=%d",
		filter.Search, filter.Brand, filter.CategoryID, filter.MinPrice, filter.MaxPrice,
		filter.SortField, filter.SortOrder, filter.Limit)

	// Get state
	state, err := t.statePort.GetState(ctx, toolCtx.SessionID)
	if err == domain.ErrSessionNotFound {
		state, err = t.statePort.CreateState(ctx, toolCtx.SessionID)
	}
	if err != nil {
		return nil, fmt.Errorf("get/create state: %w", err)
	}

	// Resolve tenant (slug → UUID)
	tenantSlug := "nike"
	if state.Current.Meta.Aliases != nil {
		if slug, ok := state.Current.Meta.Aliases["tenant_slug"]; ok && slug != "" {
			tenantSlug = slug
		}
	}
	meta["tenant"] = tenantSlug

	tenant, err := t.catalogPort.GetTenantBySlug(ctx, tenantSlug)
	if err != nil {
		return nil, fmt.Errorf("get tenant: %w", err)
	}

	// Search with fallback cascade
	sqlStart := time.Now()
	products, total, fallbackStep, err := t.searchWithFallback(ctx, tenant.ID, filter)
	sqlMs := time.Since(sqlStart).Milliseconds()
	if err != nil {
		return nil, fmt.Errorf("search products: %w", err)
	}

	meta["sql_ms"] = sqlMs
	meta["fallback_step"] = fallbackStep // 0=direct hit, 1=brand only, 2=search only, 3=empty

	if total == 0 {
		// Empty result — don't overwrite state data, just record delta
		info := domain.DeltaInfo{
			TurnID:    toolCtx.TurnID,
			Trigger:   domain.TriggerUserQuery,
			Source:    domain.SourceLLM,
			ActorID:   toolCtx.ActorID,
			DeltaType: domain.DeltaTypeAdd,
			Path:      "data.products",
			Action:    domain.Action{Type: domain.ActionSearch, Tool: "catalog_search", Params: input},
			Result:    domain.ResultMeta{Count: 0},
		}
		if _, err := t.statePort.AddDelta(ctx, toolCtx.SessionID, info.ToDelta()); err != nil {
			return nil, fmt.Errorf("add empty delta: %w", err)
		}
		return &domain.ToolResult{
			Content:  "empty: 0 results, previous data preserved",
			Metadata: meta,
		}, nil
	}

	// Extract fields from first product
	fields := catalogExtractProductFields(products[0])

	// UpdateData zone-write: atomic data + delta
	data := domain.StateData{Products: products}
	stateMeta := domain.StateMeta{
		Count:   total,
		Fields:  fields,
		Aliases: state.Current.Meta.Aliases, // preserve tenant_slug
	}

	info := domain.DeltaInfo{
		TurnID:    toolCtx.TurnID,
		Trigger:   domain.TriggerUserQuery,
		Source:    domain.SourceLLM,
		ActorID:   toolCtx.ActorID,
		DeltaType: domain.DeltaTypeAdd,
		Path:      "data.products",
		Action:    domain.Action{Type: domain.ActionSearch, Tool: "catalog_search", Params: input},
		Result:    domain.ResultMeta{Count: total, Fields: fields},
	}
	if _, err := t.statePort.UpdateData(ctx, toolCtx.SessionID, data, stateMeta, info); err != nil {
		return nil, fmt.Errorf("update data: %w", err)
	}

	return &domain.ToolResult{
		Content:  fmt.Sprintf("ok: found %d products", total),
		Metadata: meta,
	}, nil
}

// searchWithFallback implements the fallback cascade:
// 1. brand + search + filters → step 0
// 2. brand only + filters (remove search) → step 1
// 3. search only + filters (remove brand) → step 2
// Returns: products, total, fallbackStep (0=direct, 1=brand-only, 2=search-only, 3=empty), error
func (t *CatalogSearchTool) searchWithFallback(ctx context.Context, tenantID string, filter ports.ProductFilter) ([]domain.Product, int, int, error) {
	// Attempt 1: full filter
	products, total, err := t.catalogPort.ListProducts(ctx, tenantID, filter)
	if err != nil {
		return nil, 0, 0, err
	}
	if total > 0 {
		return products, total, 0, nil
	}

	// Attempt 2: brand only (remove search)
	if filter.Brand != "" && filter.Search != "" {
		fallback := filter
		fallback.Search = ""
		products, total, err = t.catalogPort.ListProducts(ctx, tenantID, fallback)
		if err != nil {
			return nil, 0, 0, err
		}
		if total > 0 {
			return products, total, 1, nil
		}
	}

	// Attempt 3: search only (remove brand)
	if filter.Brand != "" && filter.Search != "" {
		fallback := filter
		fallback.Brand = ""
		products, total, err = t.catalogPort.ListProducts(ctx, tenantID, fallback)
		if err != nil {
			return nil, 0, 0, err
		}
		if total > 0 {
			return products, total, 2, nil
		}
	}

	return nil, 0, 3, nil
}

// catalogExtractProductFields gets field names from a product (copy from search_products)
func catalogExtractProductFields(p domain.Product) []string {
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
