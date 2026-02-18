package usecases

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/ports"
)

// Haiku pricing (per 1M tokens)
const (
	haikuInputPricePerM  = 0.80
	haikuOutputPricePerM = 4.00
)

type EnrichmentUseCase struct {
	enrichment ports.EnrichmentPort
	catalog    ports.AdminCatalogPort
	log        *logger.Logger

	mu      sync.RWMutex
	current *domain.EnrichmentJob
}

func NewEnrichmentUseCase(enrichment ports.EnrichmentPort, catalog ports.AdminCatalogPort, log *logger.Logger) *EnrichmentUseCase {
	return &EnrichmentUseCase{enrichment: enrichment, catalog: catalog, log: log}
}

// GetStatus returns the current/last enrichment job status.
func (uc *EnrichmentUseCase) GetStatus() *domain.EnrichmentJob {
	uc.mu.RLock()
	defer uc.mu.RUnlock()
	if uc.current == nil {
		return nil
	}
	job := *uc.current
	return &job
}

// EnrichFile reads a crawl JSON file, enriches each product via LLM,
// and rewrites the file with enriched data (category_slug + attributes).
func (uc *EnrichmentUseCase) EnrichFile(ctx context.Context, filePath string) (*domain.EnrichmentJob, error) {
	// 1. Read and parse JSON file
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	var fileData struct {
		Products []json.RawMessage `json:"products"`
	}
	if err := json.Unmarshal(data, &fileData); err != nil {
		return nil, fmt.Errorf("parse JSON: %w", err)
	}
	if len(fileData.Products) == 0 {
		return nil, fmt.Errorf("no products in file")
	}

	// Parse each product into a map (preserves all original fields)
	products := make([]map[string]any, len(fileData.Products))
	for i, raw := range fileData.Products {
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			return nil, fmt.Errorf("parse product %d: %w", i, err)
		}
		products[i] = m
	}

	// 2. Build enrichment inputs
	inputs := make([]domain.EnrichmentInput, len(products))
	for i, p := range products {
		inputs[i] = buildInputFromMap(p)
	}

	const batchSize = 10
	const workers = 5
	batches := makeBatches(inputs, batchSize)

	// Initialize job tracker
	job := &domain.EnrichmentJob{
		ID:            uuid.New().String(),
		Status:        "processing",
		TotalProducts: len(products),
		TotalBatches:  len(batches),
		Model:         uc.enrichment.Model(),
		StartedAt:     time.Now(),
	}
	uc.mu.Lock()
	uc.current = job
	uc.mu.Unlock()

	uc.log.Info("enrichment_started",
		"job_id", job.ID,
		"file", filePath,
		"products", job.TotalProducts,
		"batches", job.TotalBatches)

	// 3. Process batches through LLM
	var totalInputTokens, totalOutputTokens atomic.Int64
	var processedBatches atomic.Int32
	var errorCount atomic.Int32

	type batchResult struct {
		result *domain.EnrichmentResult
		err    error
	}
	results := make([]batchResult, len(batches))
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup

	for i, batch := range batches {
		wg.Add(1)
		go func(idx int, items []domain.EnrichmentInput) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			res, err := uc.enrichment.EnrichProducts(ctx, items)
			results[idx] = batchResult{result: res, err: err}

			processed := int(processedBatches.Add(1))
			if err != nil {
				errorCount.Add(1)
			}
			if res != nil {
				totalInputTokens.Add(int64(res.InputTokens))
				totalOutputTokens.Add(int64(res.OutputTokens))
			}

			inTok := int(totalInputTokens.Load())
			outTok := int(totalOutputTokens.Load())
			uc.mu.Lock()
			uc.current.ProcessedBatches = processed
			uc.current.ErrorCount = int(errorCount.Load())
			uc.current.InputTokens = inTok
			uc.current.OutputTokens = outTok
			uc.current.EstimatedCostUSD = estimateCost(inTok, outTok)
			uc.mu.Unlock()

			uc.log.Info("enrichment_batch_done",
				"batch", fmt.Sprintf("%d/%d", processed, len(batches)),
				"input_tokens", inTok,
				"output_tokens", outTok,
				"cost_usd", fmt.Sprintf("$%.4f", estimateCost(inTok, outTok)))
		}(i, batch)
	}
	wg.Wait()

	// 4. Merge enriched data back into product maps
	// Build SKU → output index for quick lookup
	skuOutputs := make(map[string]domain.EnrichmentOutput)
	for _, res := range results {
		if res.err != nil {
			continue
		}
		for _, out := range res.result.Outputs {
			skuOutputs[out.SKU] = out
		}
	}

	enriched := 0
	for i, p := range products {
		sku, _ := p["sku"].(string)
		out, ok := skuOutputs[sku]
		if !ok {
			continue
		}

		// Set category_slug at top level (import.go reads this for direct lookup)
		products[i]["category_slug"] = out.CategorySlug

		// Merge enriched attrs into attributes map
		attrs, _ := p["attributes"].(map[string]any)
		if attrs == nil {
			attrs = make(map[string]any)
		}
		attrs["product_form"] = out.ProductForm
		attrs["skin_type"] = out.SkinType
		attrs["concern"] = out.Concern
		attrs["key_ingredients"] = out.KeyIngredients
		products[i]["attributes"] = attrs

		enriched++
	}

	// 5. Write enriched JSON back to file
	output := map[string]any{"products": products}
	enrichedJSON, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal enriched JSON: %w", err)
	}
	if err := os.WriteFile(filePath, enrichedJSON, 0644); err != nil {
		return nil, fmt.Errorf("write file: %w", err)
	}

	// 6. Finalize job
	now := time.Now()
	inTok := int(totalInputTokens.Load())
	outTok := int(totalOutputTokens.Load())

	uc.mu.Lock()
	uc.current.Status = "completed"
	uc.current.EnrichedProducts = enriched
	uc.current.CompletedAt = &now
	uc.current.InputTokens = inTok
	uc.current.OutputTokens = outTok
	uc.current.EstimatedCostUSD = estimateCost(inTok, outTok)
	finalJob := *uc.current
	uc.mu.Unlock()

	uc.log.Info("enrichment_completed",
		"job_id", job.ID,
		"file", filePath,
		"enriched", enriched,
		"total", len(products),
		"input_tokens", inTok,
		"output_tokens", outTok,
		"cost_usd", fmt.Sprintf("$%.4f", finalJob.EstimatedCostUSD),
		"duration", time.Since(job.StartedAt).Round(time.Second))

	return &finalJob, nil
}

func estimateCost(inputTokens, outputTokens int) float64 {
	return (float64(inputTokens)/1_000_000)*haikuInputPricePerM +
		(float64(outputTokens)/1_000_000)*haikuOutputPricePerM
}

func buildInputFromMap(p map[string]any) domain.EnrichmentInput {
	str := func(key string) string {
		v, _ := p[key].(string)
		return v
	}
	attrStr := func(key string) string {
		attrs, _ := p["attributes"].(map[string]any)
		if attrs == nil {
			return ""
		}
		v, _ := attrs[key].(string)
		return v
	}

	return domain.EnrichmentInput{
		SKU:               str("sku"),
		Name:              str("name"),
		Brand:             str("brand"),
		Description:       attrStr("description"),
		Ingredients:       attrStr("ingredients"),
		ActiveIngredients: attrStr("active_ingredients"),
		SkinType:          attrStr("skin_type"),
		Benefits:          attrStr("benefits"),
		HowToUse:          attrStr("how_to_use"),
	}
}

func makeBatches(items []domain.EnrichmentInput, size int) [][]domain.EnrichmentInput {
	var batches [][]domain.EnrichmentInput
	for i := 0; i < len(items); i += size {
		end := i + size
		if end > len(items) {
			end = len(items)
		}
		batches = append(batches, items[i:end])
	}
	return batches
}

// --- V2: DB-based enrichment ---

// EnrichFromDB reads all master products from DB, enriches via LLM v2 prompt,
// and writes PIM fields directly to DB columns.
func (uc *EnrichmentUseCase) EnrichFromDB(ctx context.Context, tenantID string) (*domain.EnrichmentJob, error) {
	// 1. Get all master products for tenant
	products, err := uc.catalog.GetAllMasterProducts(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("get master products: %w", err)
	}
	if len(products) == 0 {
		return nil, fmt.Errorf("no master products found for tenant %s", tenantID)
	}

	// 2. Build enrichment inputs
	inputs := make([]domain.EnrichmentInput, len(products))
	for i, mp := range products {
		inputs[i] = buildInputFromMasterProduct(mp)
	}

	// Build SKU → MasterProduct ID lookup
	skuToID := make(map[string]string, len(products))
	for _, mp := range products {
		skuToID[mp.SKU] = mp.ID
	}

	const batchSize = 10
	const workers = 5
	batches := makeBatches(inputs, batchSize)

	// 3. Initialize job tracker
	job := &domain.EnrichmentJob{
		ID:            uuid.New().String(),
		TenantID:      tenantID,
		Status:        "processing",
		TotalProducts: len(products),
		TotalBatches:  len(batches),
		Model:         uc.enrichment.Model(),
		StartedAt:     time.Now(),
	}
	uc.mu.Lock()
	uc.current = job
	uc.mu.Unlock()

	uc.log.Info("enrichment_v2_started",
		"job_id", job.ID,
		"tenant_id", tenantID,
		"products", job.TotalProducts,
		"batches", job.TotalBatches)

	// 4. Process batches through LLM v2
	var totalInputTokens, totalOutputTokens atomic.Int64
	var processedBatches atomic.Int32
	var errorCount atomic.Int32

	type batchResult struct {
		result *domain.EnrichmentResultV2
		err    error
	}
	results := make([]batchResult, len(batches))
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup

	for i, batch := range batches {
		wg.Add(1)
		go func(idx int, items []domain.EnrichmentInput) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			res, err := uc.enrichment.EnrichProductsV2(ctx, items)
			results[idx] = batchResult{result: res, err: err}

			processed := int(processedBatches.Add(1))
			if err != nil {
				errorCount.Add(1)
				uc.log.Error("enrichment_v2_batch_error",
					"batch", fmt.Sprintf("%d/%d", processed, len(batches)),
					"error", err)
			}
			if res != nil {
				totalInputTokens.Add(int64(res.InputTokens))
				totalOutputTokens.Add(int64(res.OutputTokens))
			}

			inTok := int(totalInputTokens.Load())
			outTok := int(totalOutputTokens.Load())
			uc.mu.Lock()
			uc.current.ProcessedBatches = processed
			uc.current.ErrorCount = int(errorCount.Load())
			uc.current.InputTokens = inTok
			uc.current.OutputTokens = outTok
			uc.current.EstimatedCostUSD = estimateCost(inTok, outTok)
			uc.mu.Unlock()

			uc.log.Info("enrichment_v2_batch_done",
				"batch", fmt.Sprintf("%d/%d", processed, len(batches)),
				"input_tokens", inTok,
				"output_tokens", outTok,
				"cost_usd", fmt.Sprintf("$%.4f", estimateCost(inTok, outTok)))
		}(i, batch)
	}
	wg.Wait()

	// 5. Write enriched data to DB
	enriched := 0
	totalOutputs := 0
	skuMisses := 0
	for _, res := range results {
		if res.err != nil {
			continue
		}
		totalOutputs += len(res.result.Outputs)
		for _, out := range res.result.Outputs {
			productID, ok := skuToID[out.SKU]
			if !ok {
				skuMisses++
				if skuMisses <= 10 {
					uc.log.Error("enrichment_v2_sku_miss",
						"sku_from_llm", out.SKU,
						"short_name", out.ShortName)
				}
				continue
			}

			// Resolve category slug to ID
			var categoryID string
			if out.CategorySlug != "" {
				cat, err := uc.catalog.GetCategoryBySlug(ctx, out.CategorySlug)
				if err == nil {
					categoryID = cat.ID
				}
			}

			if err := uc.catalog.UpdateMasterProductPIM(ctx, productID, categoryID, out); err != nil {
				uc.log.Error("enrichment_v2_update_error",
					"product_id", productID,
					"sku", out.SKU,
					"error", err)
				continue
			}
			enriched++
		}
	}

	uc.log.Info("enrichment_v2_sku_stats",
		"total_outputs", totalOutputs,
		"sku_misses", skuMisses,
		"enriched", enriched)

	// 6. Finalize job
	now := time.Now()
	inTok := int(totalInputTokens.Load())
	outTok := int(totalOutputTokens.Load())

	uc.mu.Lock()
	uc.current.Status = "completed"
	uc.current.EnrichedProducts = enriched
	uc.current.CompletedAt = &now
	uc.current.InputTokens = inTok
	uc.current.OutputTokens = outTok
	uc.current.EstimatedCostUSD = estimateCost(inTok, outTok)
	finalJob := *uc.current
	uc.mu.Unlock()

	uc.log.Info("enrichment_v2_completed",
		"job_id", job.ID,
		"tenant_id", tenantID,
		"enriched", enriched,
		"total", len(products),
		"input_tokens", inTok,
		"output_tokens", outTok,
		"cost_usd", fmt.Sprintf("$%.4f", finalJob.EstimatedCostUSD),
		"duration", time.Since(job.StartedAt).Round(time.Second))

	return &finalJob, nil
}

func buildInputFromMasterProduct(mp domain.MasterProduct) domain.EnrichmentInput {
	return domain.EnrichmentInput{
		SKU:               mp.SKU,
		Name:              mp.Name,
		Brand:             mp.Brand,
		Description:       mp.Description,
		Ingredients:       "",
		ActiveIngredients: "",
		SkinType:          "",
		Benefits:          "",
		HowToUse:          mp.HowToUse,
	}
}
