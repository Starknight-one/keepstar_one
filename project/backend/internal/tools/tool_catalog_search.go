package tools

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"
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
		Description: "Hybrid catalog search for products and services. Put structured/exact filters in 'filters'. Put semantic search intent in 'vector_query' in user's original language.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"vector_query": map[string]interface{}{
					"type":        "string",
					"description": "Semantic search in user's original language. Vector search handles multilingual matching. Example: 'кроссы для бега', 'lightweight laptop for work'.",
				},
				"entity_type": map[string]interface{}{
					"type":        "string",
					"enum":        []string{"product", "service", "all"},
					"description": "Type of catalog entity to search. Default 'all' searches both products and services.",
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

// Execute runs the hybrid catalog search: keyword SQL + vector pgvector + RRF merge → state write
func (t *CatalogSearchTool) Execute(ctx context.Context, toolCtx ToolContext, input map[string]interface{}) (*domain.ToolResult, error) {
	meta := map[string]interface{}{}

	// Parse input
	vectorQuery, _ := input["vector_query"].(string)
	sortBy, _ := input["sort_by"].(string)
	sortOrder, _ := input["sort_order"].(string)
	entityType, _ := input["entity_type"].(string)
	if entityType == "" {
		entityType = "all"
	}
	limit := 50
	if v, ok := input["limit"].(float64); ok {
		limit = int(v)
	}
	if limit == 0 {
		limit = 200 // safety cap
	}

	// Parse filters object
	var brand, category string
	var minPrice, maxPrice int
	var productForm, skinType, concern, keyIngredient, routineStep, texture, targetArea string

	if filters, ok := input["filters"].(map[string]interface{}); ok {
		brand, _ = filters["brand"].(string)
		category, _ = filters["category"].(string)
		if v, ok := filters["min_price"].(float64); ok {
			minPrice = int(v)
		}
		if v, ok := filters["max_price"].(float64); ok {
			maxPrice = int(v)
		}
		productForm, _ = filters["product_form"].(string)
		skinType, _ = filters["skin_type"].(string)
		concern, _ = filters["concern"].(string)
		keyIngredient, _ = filters["key_ingredient"].(string)
		routineStep, _ = filters["routine_step"].(string)
		texture, _ = filters["texture"].(string)
		targetArea, _ = filters["target_area"].(string)
	}

	// Convert prices: rubles → kopecks (×100)
	minPriceKopecks := minPrice * 100
	maxPriceKopecks := maxPrice * 100
	if minPrice > 0 || maxPrice > 0 {
		meta["price_conversion"] = fmt.Sprintf("%d/%d руб → %d/%d коп", minPrice, maxPrice, minPriceKopecks, maxPriceKopecks)
	}

	// Span instrumentation
	sc := domain.SpanFromContext(ctx)
	stage := domain.StageFromContext(ctx)

	// ── Phase 0: GetState → GetTenant → prepare filters (sequential) ──

	state, err := t.statePort.GetState(ctx, toolCtx.SessionID)
	if err == domain.ErrSessionNotFound {
		state, err = t.statePort.CreateState(ctx, toolCtx.SessionID)
	}
	if err != nil {
		return nil, fmt.Errorf("get/create state: %w", err)
	}

	tenantSlug := toolCtx.TenantSlug
	if tenantSlug == "" {
		if state.Current.Meta.Aliases != nil {
			if slug, ok := state.Current.Meta.Aliases["tenant_slug"]; ok && slug != "" {
				tenantSlug = slug
			}
		}
	}
	if tenantSlug == "" {
		tenantSlug = "nike"
	}
	meta["tenant"] = tenantSlug

	tenant, err := t.catalogPort.GetTenantBySlug(ctx, tenantSlug)
	if err != nil {
		return nil, fmt.Errorf("get tenant: %w", err)
	}

	// Prepare product filter
	filter := ports.ProductFilter{
		Search:        vectorQuery,
		Brand:         brand,
		CategoryName:  category,
		MinPrice:      minPriceKopecks,
		MaxPrice:      maxPriceKopecks,
		SortField:     sortBy,
		SortOrder:     sortOrder,
		Limit:         limit * 2,
		ProductForm:   productForm,
		SkinType:      skinType,
		Concern:       concern,
		KeyIngredient: keyIngredient,
		TargetArea:    targetArea,
		RoutineStep:   routineStep,
		Texture:       texture,
	}
	if filter.Brand != "" && filter.Search != "" {
		cleaned := strings.TrimSpace(removeSubstringIgnoreCase(filter.Search, filter.Brand))
		if cleaned != "" {
			filter.Search = cleaned
		}
	}

	// Prepare service filter
	svcFilter := ports.ProductFilter{
		Search:       vectorQuery,
		Brand:        brand,
		CategoryName: category,
		MinPrice:     minPriceKopecks,
		MaxPrice:     maxPriceKopecks,
		SortField:    sortBy,
		SortOrder:    sortOrder,
		Limit:        limit * 2,
	}
	if svcFilter.Brand != "" && svcFilter.Search != "" {
		cleaned := strings.TrimSpace(removeSubstringIgnoreCase(svcFilter.Search, svcFilter.Brand))
		if cleaned != "" {
			svcFilter.Search = cleaned
		}
	}

	// ── Phase 1: Embedding + keyword searches in parallel ──

	var queryEmbedding []float32
	var keywordProducts []domain.Product
	var keywordServices []domain.Service
	var embedMs, sqlMs, svcSqlMs int64
	var embedErr error

	g1, ctx1 := errgroup.WithContext(ctx)

	// Goroutine 1: Embedding
	if t.embedding != nil && vectorQuery != "" {
		g1.Go(func() error {
			var endEmbed func(...string)
			if sc != nil && stage != "" {
				endEmbed = sc.Start(stage + ".tool.embed")
			}
			embedStart := time.Now()
			searchText := vectorQuery
			if brand != "" {
				searchText = vectorQuery + " " + brand
			}
			embeddings, err := t.embedding.Embed(ctx1, []string{searchText})
			embedMs = time.Since(embedStart).Milliseconds()
			if err != nil {
				embedErr = err
			} else if len(embeddings) > 0 {
				queryEmbedding = embeddings[0]
			}
			if endEmbed != nil {
				endEmbed("OpenAI embedding")
			}
			return nil
		})
	}

	// Goroutine 2: Keyword products
	if entityType != "service" {
		g1.Go(func() error {
			var endSQL func(...string)
			if sc != nil && stage != "" {
				endSQL = sc.Start(stage + ".tool.sql")
			}
			sqlStart := time.Now()
			keywordProducts, _, _ = t.catalogPort.ListProducts(ctx1, tenant.ID, filter)
			sqlMs = time.Since(sqlStart).Milliseconds()
			if endSQL != nil {
				endSQL("keyword search")
			}
			return nil
		})
	}

	// Goroutine 3: Keyword services
	if entityType == "service" || entityType == "all" {
		g1.Go(func() error {
			var endSvcSQL func(...string)
			if sc != nil && stage != "" {
				endSvcSQL = sc.Start(stage + ".tool.svc_sql")
			}
			svcSqlStart := time.Now()
			keywordServices, _, _ = t.catalogPort.ListServices(ctx1, tenant.ID, svcFilter)
			svcSqlMs = time.Since(svcSqlStart).Milliseconds()
			if endSvcSQL != nil {
				endSvcSQL("keyword services")
			}
			return nil
		})
	}

	_ = g1.Wait()

	// Record Phase 1 timing in meta
	if embedMs > 0 {
		meta["embed_ms"] = embedMs
	}
	if embedErr != nil {
		meta["embed_error"] = embedErr.Error()
	}
	if sqlMs > 0 {
		meta["sql_ms"] = sqlMs
	}
	if svcSqlMs > 0 {
		meta["svc_sql_ms"] = svcSqlMs
	}

	// ── Phase 2: Vector searches in parallel (only if embedding ready) ──

	var vectorProducts []domain.Product
	var vectorServices []domain.Service
	var vectorMs, svcVectorMs int64
	var vectorErr, svcVectorErr error

	if queryEmbedding != nil {
		// Build vector filter once (shared, read-only)
		var vf *ports.VectorFilter
		if brand != "" || category != "" || productForm != "" || skinType != "" || concern != "" || keyIngredient != "" || targetArea != "" || routineStep != "" || texture != "" {
			vf = &ports.VectorFilter{Brand: brand, CategoryName: category, ProductForm: productForm, SkinType: skinType, Concern: concern, KeyIngredient: keyIngredient, TargetArea: targetArea, RoutineStep: routineStep, Texture: texture}
		}

		g2, ctx2 := errgroup.WithContext(ctx)

		// Goroutine 1: Vector product search
		if entityType != "service" {
			g2.Go(func() error {
				var endVector func(...string)
				if sc != nil && stage != "" {
					endVector = sc.Start(stage + ".tool.vector")
				}
				vectorStart := time.Now()
				vectorProducts, vectorErr = t.catalogPort.VectorSearch(ctx2, tenant.ID, queryEmbedding, limit*2, vf)
				vectorMs = time.Since(vectorStart).Milliseconds()
				if endVector != nil {
					endVector("pgvector")
				}
				return nil
			})
		}

		// Goroutine 2: Vector service search
		if entityType == "service" || entityType == "all" {
			g2.Go(func() error {
				var endSvcVector func(...string)
				if sc != nil && stage != "" {
					endSvcVector = sc.Start(stage + ".tool.svc_vector")
				}
				svcVectorStart := time.Now()
				var svcVF *ports.VectorFilter
				if brand != "" || category != "" {
					svcVF = &ports.VectorFilter{Brand: brand, CategoryName: category}
				}
				vectorServices, svcVectorErr = t.catalogPort.VectorSearchServices(ctx2, tenant.ID, queryEmbedding, limit*2, svcVF)
				svcVectorMs = time.Since(svcVectorStart).Milliseconds()
				if endSvcVector != nil {
					endSvcVector("pgvector services")
				}
				return nil
			})
		}

		_ = g2.Wait()
	}

	// Record Phase 2 timing in meta
	if vectorMs > 0 {
		meta["vector_ms"] = vectorMs
	}
	if vectorErr != nil {
		meta["vector_error"] = vectorErr.Error()
	}
	if svcVectorMs > 0 {
		meta["svc_vector_ms"] = svcVectorMs
	}
	if svcVectorErr != nil {
		meta["svc_vector_error"] = svcVectorErr.Error()
	}

	meta["keyword_count"] = len(keywordProducts)
	meta["vector_count"] = len(vectorProducts)
	if entityType == "service" || entityType == "all" {
		meta["service_keyword_count"] = len(keywordServices)
		meta["service_vector_count"] = len(vectorServices)
	}

	// RRF merge for products
	hasFilters := brand != "" || category != "" || productForm != "" || skinType != "" || concern != "" || keyIngredient != "" || routineStep != "" || texture != "" || targetArea != ""
	var merged []domain.Product
	if entityType != "service" {
		merged = rrfMerge(keywordProducts, vectorProducts, limit, hasFilters)
	}

	// RRF merge for services
	mergedServices := rrfMergeServices(keywordServices, vectorServices, limit, hasFilters)

	total := len(merged) + len(mergedServices)
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

	// Extract fields from first product or service
	var fields []string
	if len(merged) > 0 {
		fields = catalogExtractProductFields(merged[0])
	} else if len(mergedServices) > 0 {
		fields = catalogExtractServiceFields(mergedServices[0])
	}

	// UpdateData zone-write: atomic data + delta
	data := domain.StateData{Products: merged, Services: mergedServices}
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

	resultMsg := fmt.Sprintf("ok: found %d products", len(merged))
	if len(mergedServices) > 0 {
		resultMsg += fmt.Sprintf(", %d services", len(mergedServices))
	}

	return &domain.ToolResult{
		Content:  resultMsg,
		Metadata: meta,
	}, nil
}

// rrfMerge combines keyword and vector results using Reciprocal Rank Fusion (k=60).
// Keyword results are weighted higher (1.5×, or 2.0× when structured filters are present).
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

// rrfMergeServices combines keyword and vector service results using RRF (k=60).
func rrfMergeServices(keyword, vector []domain.Service, limit int, hasFilters bool) []domain.Service {
	if len(keyword) == 0 && len(vector) == 0 {
		return nil
	}

	const k = 60
	scores := make(map[string]float64)
	services := make(map[string]domain.Service)

	keywordWeight := 1.5
	if hasFilters {
		keywordWeight = 2.0
	}

	for rank, s := range keyword {
		scores[s.ID] += keywordWeight / float64(k+rank+1)
		services[s.ID] = s
	}
	for rank, s := range vector {
		scores[s.ID] += 1.0 / float64(k+rank+1)
		if _, exists := services[s.ID]; !exists {
			services[s.ID] = s
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

	var result []domain.Service
	for i, s := range sorted {
		if i >= limit {
			break
		}
		result = append(result, services[s.id])
	}
	return result
}

// catalogExtractServiceFields gets field names from a service
func catalogExtractServiceFields(s domain.Service) []string {
	fields := []string{"id", "name", "price"}
	if s.Description != "" {
		fields = append(fields, "description")
	}
	if s.Category != "" {
		fields = append(fields, "category")
	}
	if s.Duration != "" {
		fields = append(fields, "duration")
	}
	if s.Provider != "" {
		fields = append(fields, "provider")
	}
	if s.Rating > 0 {
		fields = append(fields, "rating")
	}
	if len(s.Images) > 0 {
		fields = append(fields, "images")
	}
	return fields
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
