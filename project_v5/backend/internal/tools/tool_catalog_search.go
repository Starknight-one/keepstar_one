package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/ports"
)

// CatalogSearchTool fetches products from the shared catalog and writes the
// result to state.Current.Data. V5 chunk 7 is keyword-only: V4's hybrid path
// (pgvector + RRF) is deferred until V5 grows an EmbeddingPort. The schema
// keeps `vector_query` so V4's Agent1 prompt copies over byte-identically;
// the executor routes that string into ListProducts' Search field
// (multi-word AND-logic ILIKE in postgres_catalog.go:139-159).
type CatalogSearchTool struct {
	state   ports.StatePort
	catalog ports.CatalogPort
}

// NewCatalogSearchTool wires the tool with the two ports it needs. Both are
// required; the catalog port is read-only.
func NewCatalogSearchTool(state ports.StatePort, catalog ports.CatalogPort) *CatalogSearchTool {
	return &CatalogSearchTool{state: state, catalog: catalog}
}

var _ Tool = (*CatalogSearchTool)(nil)

// Definition mirrors V4's tool_catalog_search.go:32-121 minus the `service`
// entity_type (V5 catalog has products only — re-add when services land).
func (t *CatalogSearchTool) Definition() domain.ToolDefinition {
	return domain.ToolDefinition{
		Name:        "catalog_search",
		Description: "Catalog search for products. Put structured/exact filters in 'filters'. Put semantic search intent in 'vector_query' in user's original language.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"vector_query": map[string]interface{}{
					"type":        "string",
					"description": "Semantic search in user's original language. Example: 'кроссы для бега', 'lightweight laptop for work'.",
				},
				"filters": map[string]interface{}{
					"type":        "object",
					"description": "Exact filters. Only include filters you're confident about.",
					"properties": map[string]interface{}{
						"brand": map[string]interface{}{
							"type":        "string",
							"description": "Brand name (e.g. COSRX, MEDI-PEEL, Holika Holika)",
						},
						"category": map[string]interface{}{
							"type":        "string",
							"description": "Category name (e.g. Сыворотки, Кремы)",
						},
						"min_price": map[string]interface{}{
							"type":        "number",
							"description": "Minimum price in RUBLES",
						},
						"max_price": map[string]interface{}{
							"type":        "number",
							"description": "Maximum price in RUBLES",
						},
						"product_form": map[string]interface{}{
							"type":        "string",
							"enum":        []string{"cream", "gel", "serum", "toner", "essence", "lotion", "oil", "balm", "foam", "mousse", "mist", "spray", "powder", "stick", "patch", "sheet-mask", "wash-off-mask", "peel", "scrub", "soap"},
							"description": "Product form/type",
						},
						"skin_type": map[string]interface{}{
							"type":        "string",
							"enum":        []string{"normal", "dry", "oily", "combination", "sensitive", "acne-prone", "mature"},
							"description": "Target skin type",
						},
						"concern": map[string]interface{}{
							"type":        "string",
							"enum":        []string{"hydration", "anti-aging", "brightening", "acne", "pores", "dark-spots", "redness", "sun-protection", "exfoliation", "firmness", "dark-circles", "lip-dryness", "oil-control", "texture", "dullness"},
							"description": "Skin concern to address",
						},
						"key_ingredient": map[string]interface{}{
							"type":        "string",
							"enum":        []string{"hyaluronic-acid", "niacinamide", "retinol", "vitamin-c", "salicylic-acid", "glycolic-acid", "centella-asiatica", "ceramides", "peptides", "snail-mucin", "tea-tree", "aloe-vera", "collagen", "aha-bha", "squalane", "shea-butter", "argan-oil", "rice-extract", "green-tea", "propolis", "mugwort", "panthenol", "zinc", "turmeric", "charcoal"},
							"description": "Key active ingredient",
						},
						"routine_step": map[string]interface{}{
							"type":        "string",
							"enum":        []string{"cleansing", "toning", "exfoliation", "treatment", "moisturizing", "sun-protection", "makeup"},
							"description": "Step in skincare routine",
						},
						"texture": map[string]interface{}{
							"type":        "string",
							"enum":        []string{"watery", "gel", "milky", "creamy", "thick", "oily", "powdery", "foamy", "balmy"},
							"description": "Product texture",
						},
						"target_area": map[string]interface{}{
							"type":        "string",
							"enum":        []string{"face", "eye-area", "lips", "neck", "body", "hands", "feet", "scalp"},
							"description": "Target application area",
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
					"description": "Max results (default 50, 0 = no limit)",
				},
			},
			"required": []string{"vector_query"},
		},
	}
}

// Execute resolves the tenant, runs one keyword + filter SQL, and writes the
// result to state.Current.Data via UpdateData. On 0 results: previous data
// is preserved, only an empty delta is recorded (V4 pattern at
// tool_catalog_search.go:442-461).
func (t *CatalogSearchTool) Execute(ctx context.Context, toolCtx domain.ToolContext, input map[string]interface{}) (*domain.ToolResult, error) {
	meta := map[string]interface{}{}

	vectorQuery, _ := input["vector_query"].(string)
	sortBy, _ := input["sort_by"].(string)
	sortOrder, _ := input["sort_order"].(string)

	limit := 50
	if v, ok := input["limit"].(float64); ok {
		limit = int(v)
	}
	if limit == 0 {
		limit = 200 // safety cap
	}

	var brand, category string
	var minPriceRubles, maxPriceRubles int
	var productForm, skinType, concern, keyIngredient, routineStep, texture, targetArea string

	if filters, ok := input["filters"].(map[string]interface{}); ok {
		brand, _ = filters["brand"].(string)
		category, _ = filters["category"].(string)
		if v, ok := filters["min_price"].(float64); ok {
			minPriceRubles = int(v)
		}
		if v, ok := filters["max_price"].(float64); ok {
			maxPriceRubles = int(v)
		}
		productForm, _ = filters["product_form"].(string)
		skinType, _ = filters["skin_type"].(string)
		concern, _ = filters["concern"].(string)
		keyIngredient, _ = filters["key_ingredient"].(string)
		routineStep, _ = filters["routine_step"].(string)
		texture, _ = filters["texture"].(string)
		targetArea, _ = filters["target_area"].(string)
	}

	// LLM speaks rubles; postgres stores integer kopecks.
	minPriceKopecks := minPriceRubles * 100
	maxPriceKopecks := maxPriceRubles * 100
	if minPriceRubles > 0 || maxPriceRubles > 0 {
		meta["price_conversion"] = fmt.Sprintf("%d/%d руб → %d/%d коп", minPriceRubles, maxPriceRubles, minPriceKopecks, maxPriceKopecks)
	}

	state, err := t.state.GetState(ctx, toolCtx.SessionID)
	if err != nil {
		return nil, fmt.Errorf("get state: %w", err)
	}

	tenantSlug := toolCtx.TenantSlug
	if tenantSlug == "" && state.Current.Meta.Aliases != nil {
		if slug, ok := state.Current.Meta.Aliases["tenant_slug"]; ok {
			tenantSlug = slug
		}
	}
	if tenantSlug == "" {
		return errorResult("tenant_slug missing from tool context and state aliases"), nil
	}
	meta["tenant"] = tenantSlug

	tenant, err := t.catalog.GetTenantBySlug(ctx, tenantSlug)
	if err != nil {
		return nil, fmt.Errorf("get tenant %q: %w", tenantSlug, err)
	}

	filter := ports.ProductFilter{
		Search:        vectorQuery,
		Brand:         brand,
		CategoryName:  category,
		MinPrice:      minPriceKopecks,
		MaxPrice:      maxPriceKopecks,
		SortField:     sortBy,
		SortOrder:     sortOrder,
		Limit:         limit,
		ProductForm:   productForm,
		SkinType:      skinType,
		Concern:       concern,
		KeyIngredient: keyIngredient,
		TargetArea:    targetArea,
		RoutineStep:   routineStep,
		Texture:       texture,
	}
	// V4 trick: if brand is set, strip it from Search so the keyword query
	// doesn't double-filter and over-narrow results
	// (tool_catalog_search.go:223-228).
	if filter.Brand != "" && filter.Search != "" {
		cleaned := strings.TrimSpace(removeSubstringIgnoreCase(filter.Search, filter.Brand))
		if cleaned != "" {
			filter.Search = cleaned
		}
	}

	sqlStart := time.Now()
	products, _, err := t.catalog.ListProducts(ctx, tenant.ID, filter)
	if err != nil {
		return nil, fmt.Errorf("list products: %w", err)
	}
	meta["sql_ms"] = time.Since(sqlStart).Milliseconds()
	meta["search_type"] = "keyword"
	meta["count"] = len(products)

	// 0 results — preserve existing data, record an empty delta only.
	if len(products) == 0 {
		info := domain.DeltaInfo{
			TurnID:    toolCtx.TurnID,
			Trigger:   domain.TriggerUserQuery,
			Source:    domain.SourceLLM,
			ActorID:   "agent1",
			DeltaType: domain.DeltaTypeAdd,
			Path:      "data.products",
			Action:    domain.Action{Type: domain.ActionSearch, Tool: "catalog_search", Params: input},
			Result:    domain.ResultMeta{Count: 0},
		}
		if _, err := t.state.AddDelta(ctx, toolCtx.SessionID, info.ToDelta()); err != nil {
			return nil, fmt.Errorf("add empty delta: %w", err)
		}
		return &domain.ToolResult{
			Content:  "empty: 0 results, previous data preserved",
			Metadata: meta,
		}, nil
	}

	fields := extractProductFields(products[0])

	data := domain.StateData{Products: products}
	stateMeta := domain.StateMeta{
		Count:        len(products),
		ProductCount: len(products),
		Fields:       fields,
		Aliases:      state.Current.Meta.Aliases,
	}
	info := domain.DeltaInfo{
		TurnID:    toolCtx.TurnID,
		Trigger:   domain.TriggerUserQuery,
		Source:    domain.SourceLLM,
		ActorID:   "agent1",
		DeltaType: domain.DeltaTypeAdd,
		Path:      "data.products",
		Action:    domain.Action{Type: domain.ActionSearch, Tool: "catalog_search", Params: input},
		Result:    domain.ResultMeta{Count: len(products), Fields: fields},
	}
	if _, err := t.state.UpdateData(ctx, toolCtx.SessionID, data, stateMeta, info); err != nil {
		return nil, fmt.Errorf("update data: %w", err)
	}

	return &domain.ToolResult{
		Content:  fmt.Sprintf("ok: found %d products", len(products)),
		Metadata: meta,
	}, nil
}

// extractProductFields lists the typed fields populated on the first result.
// Agent2's prompt-cache fields block reads StateMeta.Fields downstream.
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
	if len(p.Tags) > 0 {
		fields = append(fields, "tags")
	}
	if p.StockQuantity > 0 {
		fields = append(fields, "stockQuantity")
	}
	if p.ProductForm != "" {
		fields = append(fields, "productForm")
	}
	if len(p.SkinType) > 0 {
		fields = append(fields, "skinType")
	}
	if len(p.Concern) > 0 {
		fields = append(fields, "concern")
	}
	if len(p.KeyIngredients) > 0 {
		fields = append(fields, "keyIngredients")
	}
	return fields
}

// removeSubstringIgnoreCase removes the first occurrence of substr from s,
// matching case-insensitively. Used to strip a known brand name from the
// keyword query so the brand filter alone scopes the result set.
func removeSubstringIgnoreCase(s, substr string) string {
	if substr == "" {
		return s
	}
	lowS := strings.ToLower(s)
	lowSub := strings.ToLower(substr)
	idx := strings.Index(lowS, lowSub)
	if idx < 0 {
		return s
	}
	return s[:idx] + s[idx+len(substr):]
}
