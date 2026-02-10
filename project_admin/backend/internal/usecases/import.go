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
	SKU        string         `json:"sku"`
	Name       string         `json:"name"`
	Brand      string         `json:"brand"`
	Category   string         `json:"category"`
	Price      int            `json:"price"`
	Currency   string         `json:"currency"`
	Stock      int            `json:"stock"`
	Rating     float64        `json:"rating"`
	Images     []string       `json:"images"`
	Attributes map[string]any `json:"attributes"`
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

	// Master product
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

	// Product listing
	currency := item.Currency
	if currency == "" {
		currency = "RUB"
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
	}
	_, err = uc.catalog.UpsertProductListing(ctx, p)
	if err != nil {
		return fmt.Errorf("product listing: %w", err)
	}

	return nil
}

func (uc *ImportUseCase) postImport(tenantID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Embeddings
	if uc.embedding != nil {
		products, err := uc.catalog.GetMasterProductsWithoutEmbedding(ctx, tenantID)
		if err != nil {
			uc.log.Error("post_import_get_products_failed", "error", err)
			return
		}
		if len(products) > 0 {
			uc.log.Info("post_import_embedding_started", "count", len(products))
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
			uc.log.Info("post_import_embedding_completed", "count", len(products))
		}
	}

	// Digest
	if err := uc.catalog.GenerateCatalogDigest(ctx, tenantID); err != nil {
		uc.log.Error("post_import_digest_failed", "error", err)
	} else {
		uc.log.Info("post_import_digest_completed", "tenant_id", tenantID)
	}
}

// slugify reused from auth.go — simple version
func slugifyImport(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, " ", "-")
	return s
}
