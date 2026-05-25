package usecases

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/ports"
)

// EnrichmentTrigger is a narrow hook the import pipeline calls once a job
// finishes so freshly-landed products get PIM-enriched in the background.
// Nil trigger is supported (anthropic-less deployments, tests).
type EnrichmentTrigger interface {
	EnrichFromDBIncrementalAsync(tenantID string)
}

type ImportUseCase struct {
	catalog    ports.AdminCatalogPort
	importDB   ports.ImportPort
	embedding  ports.EmbeddingPort
	enrichHook EnrichmentTrigger
	log        *logger.Logger
}

func NewImportUseCase(catalog ports.AdminCatalogPort, importDB ports.ImportPort, embedding ports.EmbeddingPort, log *logger.Logger) *ImportUseCase {
	return &ImportUseCase{catalog: catalog, importDB: importDB, embedding: embedding, log: log}
}

// SetEnrichmentTrigger wires the post-import enrichment hook. Separate from
// the ctor so main.go can build usecases independently of whether anthropic
// credentials are present.
func (uc *ImportUseCase) SetEnrichmentTrigger(t EnrichmentTrigger) {
	uc.enrichHook = t
}

type ImportItem struct {
	Type         string         `json:"type"` // "product" (default) or "service"
	SKU          string         `json:"sku"`
	Name         string         `json:"name"`
	Brand        string         `json:"brand"`
	Category     string         `json:"category"`
	CategorySlug string         `json:"category_slug"` // direct slug lookup (from enriched data)
	Price        int            `json:"price"`
	Currency     string         `json:"currency"`
	Stock        int            `json:"stock"`
	Rating       float64        `json:"rating"`
	Images       []string       `json:"images"`
	Attributes   map[string]any `json:"attributes"`
	Tags         []string       `json:"tags"`
	SourceSystem string         `json:"source_system"` // integration tag — "shopify"/"csv"/"google_sheets"/"manual"
	SourceID     string         `json:"source_id"`     // external stable id
}

type ImportRequest struct {
	Products []ImportItem `json:"products"`
}

func (uc *ImportUseCase) Upload(ctx context.Context, tenantID string, req ImportRequest) (*domain.ImportJob, error) {
	if len(req.Products) == 0 {
		return nil, fmt.Errorf("no products to import")
	}

	job := &domain.ImportJob{
		TenantID:   tenantID,
		FileName:   fmt.Sprintf("import-%d.json", time.Now().Unix()),
		Status:     domain.ImportStatusPending,
		TotalItems: len(req.Products),
		Errors:     []string{},
	}
	job, err := uc.importDB.CreateImportJob(ctx, job)
	if err != nil {
		return nil, fmt.Errorf("create import job: %w", err)
	}

	go uc.processImport(job.ID, tenantID, req.Products)

	return job, nil
}

// UploadWithJobName creates an import job with a caller-supplied file_name
// (e.g. the uploaded CSV filename or "shopify-initial-sync"). Returns the
// job so the caller can track progress via the existing polling endpoints.
func (uc *ImportUseCase) UploadWithJobName(ctx context.Context, tenantID string, fileName string, items []ImportItem) (*domain.ImportJob, error) {
	if len(items) == 0 {
		return nil, fmt.Errorf("no products to import")
	}
	if fileName == "" {
		fileName = fmt.Sprintf("import-%d.json", time.Now().Unix())
	}
	job := &domain.ImportJob{
		TenantID:   tenantID,
		FileName:   fileName,
		Status:     domain.ImportStatusPending,
		TotalItems: len(items),
		Errors:     []string{},
	}
	job, err := uc.importDB.CreateImportJob(ctx, job)
	if err != nil {
		return nil, fmt.Errorf("create import job: %w", err)
	}
	go uc.processImport(job.ID, tenantID, items)
	return job, nil
}

// UpsertSingle performs a per-item upsert without creating an ImportJob.
// Used by Shopify webhooks (products/create, products/update) where a job
// row would be noise.
func (uc *ImportUseCase) UpsertSingle(ctx context.Context, tenantID string, item ImportItem) error {
	currency := uc.resolveCurrency(ctx, tenantID, item.Currency)
	return uc.processItemWithCurrency(ctx, tenantID, item, currency)
}

func (uc *ImportUseCase) GetJob(ctx context.Context, tenantID string, jobID string) (*domain.ImportJob, error) {
	return uc.importDB.GetImportJob(ctx, tenantID, jobID)
}

func (uc *ImportUseCase) ListJobs(ctx context.Context, tenantID string, limit int, offset int) ([]domain.ImportJob, int, error) {
	return uc.importDB.ListImportJobs(ctx, tenantID, limit, offset)
}

func (uc *ImportUseCase) processImport(jobID string, tenantID string, items []ImportItem) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	uc.importDB.UpdateImportJobProgress(ctx, jobID, 0, domain.ImportStatusProcessing)
	uc.log.Info("import_started", "job_id", jobID, "items", len(items))

	// Resolve tenant currency once per job — beats hitting DB per item.
	tenantCurrency := uc.resolveCurrency(ctx, tenantID, "")

	processed := 0
	errorCount := 0

	for i, item := range items {
		currency := tenantCurrency
		if item.Currency != "" {
			currency = item.Currency
		}
		if err := uc.processItemWithCurrency(ctx, tenantID, item, currency); err != nil {
			errorCount++
			errMsg := fmt.Sprintf("item %d (sku=%s): %v", i+1, item.SKU, err)
			uc.importDB.AppendImportError(ctx, jobID, errMsg)
			uc.log.Error("import_item_error", "job_id", jobID, "sku", item.SKU, "error", err)
		}
		processed++

		if processed%10 == 0 {
			uc.importDB.UpdateImportJobProgress(ctx, jobID, processed, domain.ImportStatusProcessing)
		}
	}

	status := domain.ImportStatusCompleted
	if errorCount == len(items) {
		status = domain.ImportStatusFailed
	}
	uc.importDB.CompleteImportJob(ctx, jobID, status, processed, errorCount)
	uc.log.Info("import_completed", "job_id", jobID, "processed", processed, "errors", errorCount)

	go uc.postImport(tenantID)
}

// resolveCurrency returns the effective currency. Priority: explicit override
// → tenant setting → "USD" fallback.
func (uc *ImportUseCase) resolveCurrency(ctx context.Context, tenantID, override string) string {
	if override != "" {
		return override
	}
	tenant, err := uc.catalog.GetTenantByID(ctx, tenantID)
	if err == nil && tenant != nil && tenant.Settings != nil {
		if raw, err := json.Marshal(tenant.Settings); err == nil {
			var s domain.TenantSettings
			if err := json.Unmarshal(raw, &s); err == nil && s.Currency != "" {
				return s.Currency
			}
		}
	}
	return "USD"
}

func (uc *ImportUseCase) processItemWithCurrency(ctx context.Context, tenantID string, item ImportItem, currency string) error {
	if item.SKU == "" || item.Name == "" {
		return fmt.Errorf("sku and name are required")
	}

	// Category: prefer direct slug lookup, fall back to slugify+create
	var categoryID string
	if item.CategorySlug != "" {
		cat, err := uc.catalog.GetCategoryBySlug(ctx, item.CategorySlug)
		if err == nil {
			categoryID = cat.ID
		} else {
			uc.log.Error("category_slug_not_found", "slug", item.CategorySlug, "sku", item.SKU)
		}
	}
	if categoryID == "" {
		catSlug := slugify(item.Category)
		if catSlug == "" {
			catSlug = "uncategorized"
		}
		catName := item.Category
		if catName == "" {
			catName = "Uncategorized"
		}
		var err error
		categoryID, err = uc.catalog.GetOrCreateCategory(ctx, catName, catSlug)
		if err != nil {
			return fmt.Errorf("category: %w", err)
		}
	}

	if currency == "" {
		currency = "USD"
	}

	// Default source_system for imports where the caller didn't tag a source
	// (e.g. hand-crafted JSON via the legacy ImportPage).
	if item.SourceSystem == "" && item.SourceID == "" {
		item.SourceSystem = "manual"
	}

	return uc.processProductItem(ctx, tenantID, item, categoryID, currency)
}

func (uc *ImportUseCase) processProductItem(ctx context.Context, tenantID string, item ImportItem, categoryID, currency string) error {
	mp := &domain.MasterProduct{
		SKU:           item.SKU,
		Name:          item.Name,
		Brand:         item.Brand,
		CategoryID:    categoryID,
		Images:        item.Images,
		OwnerTenantID: tenantID,
		SourceSystem:  item.SourceSystem,
		SourceID:      item.SourceID,
	}
	mpID, err := uc.catalog.UpsertMasterProduct(ctx, mp)
	if err != nil {
		return fmt.Errorf("master product: %w", err)
	}

	p := &domain.Product{
		TenantID:        tenantID,
		MasterProductID: mpID,
		Name:            item.Name,
		Price:           item.Price,
		Currency:        currency,
		StockQuantity:   item.Stock,
		Rating:          item.Rating,
		Images:          item.Images,
		Tags:            item.Tags,
	}
	_, err = uc.catalog.UpsertProductListing(ctx, p)
	if err != nil {
		return fmt.Errorf("product listing: %w", err)
	}

	return nil
}

func (uc *ImportUseCase) postImport(tenantID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	if uc.embedding != nil {
		uc.embedProducts(ctx, tenantID)
	}

	if err := uc.catalog.GenerateCatalogDigest(ctx, tenantID); err != nil {
		uc.log.Error("post_import_digest_failed", "error", err)
	} else {
		uc.log.Info("post_import_digest_completed", "tenant_id", tenantID)
	}

	// Trigger incremental PIM enrichment for newly-landed products. Products
	// are already searchable via keyword+vector from the embed pass above;
	// this lights up structured filters asynchronously.
	if uc.enrichHook != nil {
		uc.enrichHook.EnrichFromDBIncrementalAsync(tenantID)
	}
}

func (uc *ImportUseCase) embedProducts(ctx context.Context, tenantID string) {
	products, err := uc.catalog.GetMasterProductsWithoutEmbedding(ctx, tenantID)
	if err != nil {
		uc.log.Error("post_import_get_products_failed", "error", err)
		return
	}
	if len(products) == 0 {
		return
	}

	uc.log.Info("post_import_product_embedding_started", "count", len(products))
	texts := make([]string, len(products))
	for i, p := range products {
		texts[i] = buildEmbeddingText(p)
	}

	batchSize := 100
	for i := 0; i < len(texts); i += batchSize {
		end := i + batchSize
		if end > len(texts) {
			end = len(texts)
		}
		embeddings, err := uc.embedding.Embed(ctx, texts[i:end])
		if err != nil {
			uc.log.Error("post_import_embed_failed", "error", err)
			break
		}
		for j, emb := range embeddings {
			uc.catalog.SeedEmbedding(ctx, products[i+j].ID, emb)
		}
	}
	uc.log.Info("post_import_product_embedding_completed", "count", len(products))
}

// buildEmbeddingText creates a compact semantic text for vector embedding.
// Uses PIM structured fields (~30 tokens). Falls back to name+brand+category for unenriched.
func buildEmbeddingText(p domain.MasterProduct) string {
	parts := []string{p.Name}
	if p.Brand != "" {
		parts = append(parts, p.Brand)
	}
	if p.CategoryName != "" {
		parts = append(parts, p.CategoryName)
	}
	if p.ProductForm != "" {
		parts = append(parts, p.ProductForm)
	}
	if p.Texture != "" {
		parts = append(parts, p.Texture)
	}
	if p.MarketingClaim != "" {
		parts = append(parts, p.MarketingClaim)
	}
	if len(p.SkinType) > 0 {
		parts = append(parts, strings.Join(p.SkinType, " "))
	}
	if len(p.Concern) > 0 {
		parts = append(parts, strings.Join(p.Concern, " "))
	}
	if len(p.KeyIngredients) > 0 {
		parts = append(parts, strings.Join(p.KeyIngredients, " "))
	}
	if p.RoutineStep != "" {
		parts = append(parts, p.RoutineStep)
	}
	return strings.Join(parts, " ")
}
