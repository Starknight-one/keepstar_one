package tools

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/ports"
)

// CatalogSearchTool runs hybrid retrieval over the shared catalog and writes
// the merged result into state.Current.Data. The full V4 flow ports here:
//
//  1. Phase 1 — embed `vector_query` via EmbeddingPort and run keyword SQL
//     in parallel goroutines.
//  2. Phase 2 — pgvector cosine search using the embedding from Phase 1
//     (skipped if embedding failed; the tool then degrades to keyword-only).
//  3. RRF merge — combine ranked lists with k=60, weighting keyword 1.5×
//     base or 2.0× when structured filters are present.
//
// The `embedding` port is optional. nil → keyword-only (also used in tests
// and when OPENAI_API_KEY is unset). On embedding error the tool logs the
// reason in span meta and falls back to keyword-only.
type CatalogSearchTool struct {
	state     ports.StatePort
	catalog   ports.CatalogPort
	embedding ports.EmbeddingPort
}

// NewCatalogSearchTool wires the tool with its three ports. embedding may be
// nil (keyword-only fallback).
func NewCatalogSearchTool(state ports.StatePort, catalog ports.CatalogPort, embedding ports.EmbeddingPort) *CatalogSearchTool {
	return &CatalogSearchTool{state: state, catalog: catalog, embedding: embedding}
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
							"description": "Minimum price in USD (dollars, major units)",
						},
						"max_price": map[string]interface{}{
							"type":        "number",
							"description": "Maximum price in USD (dollars, major units)",
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

// Execute parses tool input, runs hybrid retrieval, and writes the merged
// products into state.Current.Data via UpdateData. On 0 final results the
// previous data is preserved and only an empty delta is recorded (V4
// behaviour at tool_catalog_search.go:442-461).
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
	var minPriceMajor, maxPriceMajor int
	var productForm, skinType, concern, keyIngredient, routineStep, texture, targetArea string

	if filters, ok := input["filters"].(map[string]interface{}); ok {
		brand, _ = filters["brand"].(string)
		category, _ = filters["category"].(string)
		if v, ok := filters["min_price"].(float64); ok {
			minPriceMajor = int(v)
		}
		if v, ok := filters["max_price"].(float64); ok {
			maxPriceMajor = int(v)
		}
		productForm, _ = filters["product_form"].(string)
		skinType, _ = filters["skin_type"].(string)
		concern, _ = filters["concern"].(string)
		keyIngredient, _ = filters["key_ingredient"].(string)
		routineStep, _ = filters["routine_step"].(string)
		texture, _ = filters["texture"].(string)
		targetArea, _ = filters["target_area"].(string)
	}

	// LLM speaks dollars (major units); postgres stores integer cents.
	minPriceCents := minPriceMajor * 100
	maxPriceCents := maxPriceMajor * 100
	if minPriceMajor > 0 || maxPriceMajor > 0 {
		meta["price_conversion"] = fmt.Sprintf("%d/%d руб → %d/%d коп", minPriceMajor, maxPriceMajor, minPriceCents, maxPriceCents)
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
		MinPrice:      minPriceCents,
		MaxPrice:      maxPriceCents,
		SortField:     sortBy,
		SortOrder:     sortOrder,
		// Over-fetch each branch by 2× so RRF has a richer pool to merge
		// before truncating to `limit`. V4 does the same.
		Limit:         limit * 2,
		ProductForm:   productForm,
		SkinType:      skinType,
		Concern:       concern,
		KeyIngredient: keyIngredient,
		TargetArea:    targetArea,
		RoutineStep:   routineStep,
		Texture:       texture,
	}
	// If brand is set, strip it from the keyword query so the brand filter
	// alone scopes results — otherwise the keyword stage double-filters and
	// over-narrows. V4 trick at tool_catalog_search.go:223-228.
	if filter.Brand != "" && filter.Search != "" {
		cleaned := strings.TrimSpace(removeSubstringIgnoreCase(filter.Search, filter.Brand))
		if cleaned != "" {
			filter.Search = cleaned
		}
	}

	// ── Embed the query (best-effort; nil → FTS-only degradation) ──
	var queryEmbedding []float32
	if t.embedding != nil && vectorQuery != "" {
		searchText := vectorQuery
		if brand != "" {
			// Embed the brand alongside the query so vector results bias
			// toward the brand without a hard filter (V4 pattern).
			searchText = vectorQuery + " " + brand
		}
		start := time.Now()
		embeddings, err := t.embedding.Embed(ctx, []string{searchText})
		meta["embed_ms"] = time.Since(start).Milliseconds()
		if err != nil {
			meta["embed_error"] = err.Error()
		} else if len(embeddings) > 0 {
			queryEmbedding = embeddings[0]
		}
	}

	// ── Single fused search over the per-tenant projection slice ──
	// filter.Limit was 2× for the old RRF over-fetch; SearchProjection takes
	// the real limit directly.
	start := time.Now()
	merged, searchErr := t.catalog.SearchProjection(ctx, tenant.ID, queryEmbedding, filter, limit)
	meta["search_ms"] = time.Since(start).Milliseconds()
	if searchErr != nil {
		meta["search_error"] = searchErr.Error()
	}

	for i := range merged {
		NormalizeProduct(&merged[i])
	}

	// ── Per-tenant price/stock visibility (hide-only; default visible) ──
	// settingVisible treats nil/missing/non-bool as visible; only an explicit
	// boolean false hides. Suppression happens after normalization so the
	// zeroed values stay zeroed downstream (binding skips zero price/stock).
	//
	// This is the single suppression chokepoint: zeroing the TYPED struct
	// fields is not enough, because binding_to_map.ProductToMap also layers
	// Tier2 (master JSONB) and Extra (per-listing JSONB) and fills any key the
	// typed fields left empty — so a hidden 'price'/'currency'/'stockQuantity'
	// could re-enter from JSONB at bind time and render. We therefore also
	// delete those keys from Extra and Tier2 here so nothing downstream can
	// re-introduce a suppressed field. Defaults (visible) are untouched.
	priceVisible := settingVisible(tenant.Settings, "priceVisible")
	if !priceVisible {
		for i := range merged {
			merged[i].Price = 0
			merged[i].PriceFormatted = ""
			merged[i].Currency = ""
			deleteKeys(merged[i].Extra, "price", "priceFormatted", "currency")
			deleteKeys(merged[i].Tier2, "price", "priceFormatted", "currency")
		}
	}
	if !settingVisible(tenant.Settings, "stockVisible") {
		for i := range merged {
			merged[i].StockQuantity = 0
			deleteKeys(merged[i].Extra, "stockQuantity")
			deleteKeys(merged[i].Tier2, "stockQuantity")
		}
	}

	if queryEmbedding != nil {
		meta["search_type"] = "hybrid"
	} else {
		meta["search_type"] = "fts"
	}
	meta["count"] = len(merged)
	if input != nil {
		// Surface the actual query so trace inspection no longer has to dig
		// through conversation_history. Keep filter values too — same
		// reason, this is the diagnosis blind spot from chunk-15 prod.
		meta["input"] = map[string]interface{}{
			"vector_query": vectorQuery,
			"brand":        brand,
			"category":     category,
			"product_form": productForm,
			"skin_type":    skinType,
			"concern":      concern,
			"limit":        limit,
		}
	}

	if len(merged) == 0 {
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

	fields := extractProductFields(merged[0], priceVisible)

	data := domain.StateData{Products: merged}
	stateMeta := domain.StateMeta{
		Count:        len(merged),
		ProductCount: len(merged),
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
		Result:    domain.ResultMeta{Count: len(merged), Fields: fields},
	}
	if _, err := t.state.UpdateData(ctx, toolCtx.SessionID, data, stateMeta, info); err != nil {
		return nil, fmt.Errorf("update data: %w", err)
	}

	return &domain.ToolResult{
		Content:  fmt.Sprintf("ok: found %d products", len(merged)),
		Metadata: meta,
	}, nil
}

// rrfMerge combines keyword and vector ranked lists via Reciprocal Rank
// Fusion (k=60). Keyword gets 1.5× base weight, or 2.0× when structured
// filters are present (V4 weights at tool_catalog_search.go:506-549).
// Truncated to `limit` after sort.
func rrfMerge(keyword, vector []domain.Product, limit int, hasFilters bool) []domain.Product {
	const k = 60
	scores := make(map[string]float64)
	products := make(map[string]domain.Product)

	keywordWeight := 1.5
	if hasFilters {
		keywordWeight = 2.0
	}

	for rank, p := range keyword {
		scores[p.ID] += keywordWeight / float64(k+rank+1)
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
	sorted := make([]scored, 0, len(scores))
	for id, score := range scores {
		sorted = append(sorted, scored{id, score})
	}
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].score == sorted[j].score {
			return sorted[i].id < sorted[j].id // stable tie-break by ID
		}
		return sorted[i].score > sorted[j].score
	})

	result := make([]domain.Product, 0, limit)
	for i, s := range sorted {
		if i >= limit {
			break
		}
		result = append(result, products[s.id])
	}
	return result
}

// extractProductFields lists the typed fields populated on the first result.
// Agent2's prompt-cache fields block reads StateMeta.Fields downstream.
// priceVisible gates the "price" field: when the tenant hides price we omit
// it so StateMeta.Fields doesn't advertise a field the binding won't supply.
func extractProductFields(p domain.Product, priceVisible bool) []string {
	fields := []string{"id", "name"}
	if priceVisible {
		fields = append(fields, "price")
	}
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

// settingVisible reads a hide-only visibility flag from tenant settings.
// Thin wrapper over domain.SettingVisible — the single source of truth shared
// with the Agent2 <fields> prompt block (usecases/prompt_cache.go).
func settingVisible(settings map[string]any, key string) bool {
	return domain.SettingVisible(settings, key)
}

// deleteKeys removes the given keys from m (no-op on a nil map). Used to strip
// suppressed price/stock entries out of Product.Extra / Product.Tier2 so the
// JSONB layers in binding_to_map.ProductToMap can never re-introduce a field
// the tenant has hidden.
func deleteKeys(m map[string]interface{}, keys ...string) {
	if m == nil {
		return
	}
	for _, k := range keys {
		delete(m, k)
	}
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
