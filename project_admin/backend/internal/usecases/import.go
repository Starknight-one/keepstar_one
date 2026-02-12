package usecases

import (
	"context"
	"fmt"
	"strings"
	"time"

	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/ports"
)

type ImportUseCase struct {
	catalog   ports.AdminCatalogPort
	importDB  ports.ImportPort
	embedding ports.EmbeddingPort
	log       *logger.Logger
}

func NewImportUseCase(catalog ports.AdminCatalogPort, importDB ports.ImportPort, embedding ports.EmbeddingPort, log *logger.Logger) *ImportUseCase {
	return &ImportUseCase{catalog: catalog, importDB: importDB, embedding: embedding, log: log}
}

type ImportItem struct {
	Type         string         `json:"type"`         // "product" (default) or "service"
	SKU          string         `json:"sku"`
	Name         string         `json:"name"`
	Brand        string         `json:"brand"`
	Category     string         `json:"category"`
	Price        int            `json:"price"`
	Currency     string         `json:"currency"`
	Stock        int            `json:"stock"`
	Rating       float64        `json:"rating"`
	Images       []string       `json:"images"`
	Attributes   map[string]any `json:"attributes"`
	Tags         []string       `json:"tags"`
	Duration     string         `json:"duration"`     // service-specific
	Provider     string         `json:"provider"`     // service-specific
	Availability string         `json:"availability"` // service-specific
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

	// Launch background processing
	go uc.processImport(job.ID, tenantID, req.Products)

	return job, nil
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

	processed := 0
	errorCount := 0

	for i, item := range items {
		if err := uc.processItem(ctx, tenantID, item); err != nil {
			errorCount++
			errMsg := fmt.Sprintf("item %d (sku=%s): %v", i+1, item.SKU, err)
			uc.importDB.AppendImportError(ctx, jobID, errMsg)
			uc.log.Error("import_item_error", "job_id", jobID, "sku", item.SKU, "error", err)
		}
		processed++

		// Progress update every 10 items
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

	// Post-import: embeddings + digest
	go uc.postImport(tenantID)
}

func (uc *ImportUseCase) processItem(ctx context.Context, tenantID string, item ImportItem) error {
	if item.SKU == "" || item.Name == "" {
		return fmt.Errorf("sku and name are required")
	}

	// Category
	catSlug := slugify(item.Category)
	if catSlug == "" {
		catSlug = "uncategorized"
	}
	catName := item.Category
	if catName == "" {
		catName = "Uncategorized"
	}
	categoryID, err := uc.catalog.GetOrCreateCategory(ctx, catName, catSlug)
	if err != nil {
		return fmt.Errorf("category: %w", err)
	}

	currency := item.Currency
	if currency == "" {
		currency = "RUB"
	}

	// Branch by type
	if item.Type == "service" {
		return uc.processServiceItem(ctx, tenantID, item, categoryID, currency)
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
		Attributes:    item.Attributes,
		OwnerTenantID: tenantID,
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

func (uc *ImportUseCase) processServiceItem(ctx context.Context, tenantID string, item ImportItem, categoryID, currency string) error {
	ms := &domain.MasterService{
		SKU:           item.SKU,
		Name:          item.Name,
		Brand:         item.Brand,
		CategoryID:    categoryID,
		Images:        item.Images,
		Attributes:    item.Attributes,
		Duration:      item.Duration,
		Provider:      item.Provider,
		OwnerTenantID: tenantID,
	}
	msID, err := uc.catalog.UpsertMasterService(ctx, ms)
	if err != nil {
		return fmt.Errorf("master service: %w", err)
	}

	availability := item.Availability
	if availability == "" {
		availability = "available"
	}

	s := &domain.Service{
		TenantID:        tenantID,
		MasterServiceID: msID,
		Name:            item.Name,
		Price:           item.Price,
		Currency:        currency,
		Rating:          item.Rating,
		Images:          item.Images,
		Tags:            item.Tags,
		Availability:    availability,
	}
	_, err = uc.catalog.UpsertServiceListing(ctx, s)
	if err != nil {
		return fmt.Errorf("service listing: %w", err)
	}

	return nil
}

func (uc *ImportUseCase) postImport(tenantID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	if uc.embedding != nil {
		// Product embeddings
		uc.embedProducts(ctx, tenantID)

		// Service embeddings
		uc.embedServices(ctx, tenantID)
	}

	// Digest
	if err := uc.catalog.GenerateCatalogDigest(ctx, tenantID); err != nil {
		uc.log.Error("post_import_digest_failed", "error", err)
	} else {
		uc.log.Info("post_import_digest_completed", "tenant_id", tenantID)
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
		text := p.Name
		if p.Description != "" {
			text += " " + p.Description
		}
		if p.Brand != "" {
			text += " " + p.Brand
		}
		if p.CategoryName != "" {
			text += " " + p.CategoryName
		}
		if p.Attributes != nil {
			for _, key := range []string{"color", "material", "type", "size"} {
				if v, ok := p.Attributes[key]; ok {
					if s, ok := v.(string); ok && s != "" {
						text += " " + s
					}
				}
			}
		}
		texts[i] = text
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

func (uc *ImportUseCase) embedServices(ctx context.Context, tenantID string) {
	services, err := uc.catalog.GetMasterServicesWithoutEmbedding(ctx, tenantID)
	if err != nil {
		uc.log.Error("post_import_get_services_failed", "error", err)
		return
	}
	if len(services) == 0 {
		return
	}

	uc.log.Info("post_import_service_embedding_started", "count", len(services))
	texts := make([]string, len(services))
	for i, s := range services {
		text := s.Name
		if s.Description != "" {
			text += " " + s.Description
		}
		if s.Brand != "" {
			text += " " + s.Brand
		}
		if s.CategoryName != "" {
			text += " " + s.CategoryName
		}
		if s.Duration != "" {
			text += " " + s.Duration
		}
		if s.Provider != "" {
			text += " " + s.Provider
		}
		texts[i] = text
	}

	batchSize := 100
	for i := 0; i < len(texts); i += batchSize {
		end := i + batchSize
		if end > len(texts) {
			end = len(texts)
		}
		embeddings, err := uc.embedding.Embed(ctx, texts[i:end])
		if err != nil {
			uc.log.Error("post_import_service_embed_failed", "error", err)
			break
		}
		for j, emb := range embeddings {
			uc.catalog.SeedServiceEmbedding(ctx, services[i+j].ID, emb)
		}
	}
	uc.log.Info("post_import_service_embedding_completed", "count", len(services))
}

// slugify reused from auth.go — simple version
func slugifyImport(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, " ", "-")
	return s
}
