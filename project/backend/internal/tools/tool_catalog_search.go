package tools

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"keepstar/internal/domain"
	"keepstar/internal/ports"
)

// CatalogSearchTool is a meta-tool with hybrid search: keyword SQL + vector pgvector + RRF merge
type CatalogSearchTool struct {
	statePort   ports.StatePort
	catalogPort ports.CatalogPort
	embedding   ports.EmbeddingPort // nil = keyword-only mode
}

// NewCatalogSearchTool creates the catalog search meta-tool
func NewCatalogSearchTool(statePort ports.StatePort, catalogPort ports.CatalogPort, embedding ports.EmbeddingPort) *CatalogSearchTool {
	return &CatalogSearchTool{
		statePort:   statePort,
		catalogPort: catalogPort,
		embedding:   embedding,
	}
}

// Definition returns the tool definition for LLM
func (t *CatalogSearchTool) Definition() domain.ToolDefinition {
	return domain.ToolDefinition{
		Name:        "catalog_search",
		Description: "Hybrid product search. Put structured/exact filters in 'filters'. Put semantic search intent in 'vector_query' in user's original language.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"vector_query": map[string]interface{}{
					"type":        "string",
					"description": "Semantic search in user's original language. Vector search handles multilingual matching. Example: 'кроссы для бега', 'lightweight laptop for work'.",
				},
				"filters": map[string]interface{}{
					"type":        "object",
					"description": "Exact keyword filters. Only include filters you're confident about.",
					"properties": map[string]interface{}{
						"brand": map[string]interface{}{
							"type":        "string",
							"description": "Brand name in English (e.g. Nike, Samsung, Apple)",
						},
						"category": map[string]interface{}{
							"type":        "string",
							"description": "Product category (e.g. Sneakers, Laptops, Headphones)",
						},
						"min_price": map[string]interface{}{
							"type":        "number",
							"description": "Minimum price in RUBLES",
						},
						"max_price": map[string]interface{}{
							"type":        "number",
							"description": "Maximum price in RUBLES",
						},
						"color": map[string]interface{}{
							"type":        "string",
							"description": "Product color in English (e.g. Black, White, Blue)",
						},
						"material": map[string]interface{}{
							"type":        "string",
							"description": "Material (e.g. Leather, Mesh, Fleece)",
						},
						"storage": map[string]interface{}{
							"type":        "string",
							"description": "Storage capacity (e.g. 128GB, 256GB, 512GB)",
						},
						"ram": map[string]interface{}{
							"type":        "string",
							"description": "RAM size (e.g. 8GB, 16GB)",
						},
						"size": map[string]interface{}{
							"type":        "string",
							"description": "Size (e.g. 11 inch, 44mm)",
						},
					},
				},
				"sort_by": map[string]interface{}{
					"type": "string",
					"enum": []string{"price", "rating", "name"},
				},
				"sort_order": map[string]interface{}{
					"type": "string",
					"enum": []string{"asc", "desc"},
				},
				"limit": map[string]interface{}{
					"type":        "integer",
					"description": "Max results (default 10)",
				},
			},
			"required": []string{"vector_query"},
		},
	}
}

// Execute runs the hybrid catalog search: keyword SQL + vector pgvector + RRF merge → state write
func (t *CatalogSearchTool) Execute(ctx context.Context, toolCtx ToolContext, input map[string]interface{}) (*domain.ToolResult, error) {
	meta := map[string]interface{}{}

	// Parse input
	vectorQuery, _ := input["vector_query"].(string)
	sortBy, _ := input["sort_by"].(string)
	sortOrder, _ := input["sort_order"].(string)
	limit := 10
	if v, ok := input["limit"].(float64); ok {
		limit = int(v)
	}

	// Parse filters object
	var brand, category string
	var minPrice, maxPrice int
	attributes := make(map[string]string)

	if filters, ok := input["filters"].(map[string]interface{}); ok {
		brand, _ = filters["brand"].(string)
		category, _ = filters["category"].(string)
		if v, ok := filters["min_price"].(float64); ok {
			minPrice = int(v)
		}
		if v, ok := filters["max_price"].(float64); ok {
			maxPrice = int(v)
		}
		// Collect JSONB attributes (everything that's not a known column filter)
		knownFilters := map[string]bool{"brand": true, "category": true, "min_price": true, "max_price": true}
		for key, val := range filters {
			if !knownFilters[key] {
				if strVal, ok := val.(string); ok {
					attributes[key] = strVal
				}
			}
		}
	}

	// Convert prices: rubles → kopecks (×100)
	minPriceKopecks := minPrice * 100
	maxPriceKopecks := maxPrice * 100
	if minPrice > 0 || maxPrice > 0 {
		meta["price_conversion"] = fmt.Sprintf("%d/%d руб → %d/%d коп", minPrice, maxPrice, minPriceKopecks, maxPriceKopecks)
	}

	// Generate query embedding
	var queryEmbedding []float32
	if t.embedding != nil && vectorQuery != "" {
		embedStart := time.Now()
		searchText := vectorQuery
		if brand != "" {
			searchText = vectorQuery + " " + brand
		}
		embeddings, err := t.embedding.Embed(ctx, []string{searchText})
		if err == nil && len(embeddings) > 0 {
			queryEmbedding = embeddings[0]
		}
		meta["embed_ms"] = time.Since(embedStart).Milliseconds()
	}

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

	// Keyword search
	filter := ports.ProductFilter{
		Search:       vectorQuery, // also used for ILIKE fallback
		Brand:        brand,
		CategoryName: category, // agent passes category name/slug → ILIKE on c.name/c.slug
		MinPrice:     minPriceKopecks,
		MaxPrice:     maxPriceKopecks,
		SortField:    sortBy,
		SortOrder:    sortOrder,
		Limit:        limit * 2,
		Attributes:   attributes, // JSONB filters
	}

	// Strip brand from ILIKE search to avoid AND conflict
	if filter.Brand != "" && filter.Search != "" {
		cleaned := strings.TrimSpace(removeSubstringIgnoreCase(filter.Search, filter.Brand))
		if cleaned != "" {
			filter.Search = cleaned
		}
	}

	sqlStart := time.Now()
	keywordProducts, _, _ := t.catalogPort.ListProducts(ctx, tenant.ID, filter)
	meta["sql_ms"] = time.Since(sqlStart).Milliseconds()

	// Vector search
	var vectorProducts []domain.Product
	if queryEmbedding != nil {
		vectorStart := time.Now()
		vectorProducts, _ = t.catalogPort.VectorSearch(ctx, tenant.ID, queryEmbedding, limit*2)
		meta["vector_ms"] = time.Since(vectorStart).Milliseconds()
	}
	meta["keyword_count"] = len(keywordProducts)
	meta["vector_count"] = len(vectorProducts)

	// RRF merge
	merged := rrfMerge(keywordProducts, vectorProducts, limit)
	total := len(merged)
	meta["merged_count"] = total
	if len(keywordProducts) > 0 && len(vectorProducts) > 0 {
		meta["search_type"] = "hybrid"
	} else if len(vectorProducts) > 0 {
		meta["search_type"] = "vector"
	} else {
		meta["search_type"] = "keyword"
	}

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
	fields := catalogExtractProductFields(merged[0])

	// UpdateData zone-write: atomic data + delta
	data := domain.StateData{Products: merged}
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

// rrfMerge combines keyword and vector results using Reciprocal Rank Fusion (k=60).
func rrfMerge(keyword, vector []domain.Product, limit int) []domain.Product {
	const k = 60
	scores := make(map[string]float64)
	products := make(map[string]domain.Product)

	for rank, p := range keyword {
		scores[p.ID] += 1.0 / float64(k+rank+1)
		products[p.ID] = p
	}
	for rank, p := range vector {
		scores[p.ID] += 1.0 / float64(k+rank+1)
		if _, exists := products[p.ID]; !exists {
			products[p.ID] = p
		}
	}

	type scored struct {
		id    string
		score float64
	}
	var sorted []scored
	for id, score := range scores {
		sorted = append(sorted, scored{id, score})
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].score > sorted[j].score
	})

	var result []domain.Product
	for i, s := range sorted {
		if i >= limit {
			break
		}
		result = append(result, products[s.id])
	}
	return result
}

// catalogExtractProductFields gets field names from a product
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
