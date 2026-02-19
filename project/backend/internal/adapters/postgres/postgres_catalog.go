package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	pgvector "github.com/pgvector/pgvector-go"
	"keepstar/internal/domain"
	"keepstar/internal/ports"
)

// CatalogAdapter implements CatalogPort for PostgreSQL
type CatalogAdapter struct {
	client *Client
	log    *slog.Logger
}

// NewCatalogAdapter creates a new CatalogAdapter
func NewCatalogAdapter(client *Client) *CatalogAdapter {
	return &CatalogAdapter{client: client, log: slog.Default()}
}

// GetTenantBySlug retrieves a tenant by its slug
func (a *CatalogAdapter) GetTenantBySlug(ctx context.Context, slug string) (*domain.Tenant, error) {
	if sc := domain.SpanFromContext(ctx); sc != nil {
		endSpan := sc.Start("db.get_tenant")
		defer endSpan()
	}
	query := `
		SELECT id, slug, name, type, settings, created_at, updated_at
		FROM catalog.tenants
		WHERE slug = $1
	`

	var tenant domain.Tenant
	var settingsJSON []byte

	err := a.client.pool.QueryRow(ctx, query, slug).Scan(
		&tenant.ID,
		&tenant.Slug,
		&tenant.Name,
		&tenant.Type,
		&settingsJSON,
		&tenant.CreatedAt,
		&tenant.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrTenantNotFound
		}
		return nil, fmt.Errorf("query tenant: %w", err)
	}

	if len(settingsJSON) > 0 {
		if err := json.Unmarshal(settingsJSON, &tenant.Settings); err != nil {
			return nil, fmt.Errorf("unmarshal settings: %w", err)
		}
	}

	return &tenant, nil
}

// GetCategories retrieves all categories
func (a *CatalogAdapter) GetCategories(ctx context.Context) ([]domain.Category, error) {
	query := `
		SELECT id, name, slug, COALESCE(parent_id::text, '') as parent_id
		FROM catalog.categories
		ORDER BY name
	`

	rows, err := a.client.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query categories: %w", err)
	}
	defer rows.Close()

	var categories []domain.Category
	for rows.Next() {
		var cat domain.Category
		if err := rows.Scan(&cat.ID, &cat.Name, &cat.Slug, &cat.ParentID); err != nil {
			return nil, fmt.Errorf("scan category: %w", err)
		}
		categories = append(categories, cat)
	}

	return categories, nil
}

// GetMasterProduct retrieves a master product by ID
func (a *CatalogAdapter) GetMasterProduct(ctx context.Context, id string) (*domain.MasterProduct, error) {
	query := `
		SELECT id, sku, name, description, brand, category_id, images, owner_tenant_id, created_at, updated_at
		FROM catalog.master_products
		WHERE id = $1
	`

	var product domain.MasterProduct
	var imagesJSON []byte

	err := a.client.pool.QueryRow(ctx, query, id).Scan(
		&product.ID,
		&product.SKU,
		&product.Name,
		&product.Description,
		&product.Brand,
		&product.CategoryID,
		&imagesJSON,
		&product.OwnerTenantID,
		&product.CreatedAt,
		&product.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrProductNotFound
		}
		return nil, fmt.Errorf("query master product: %w", err)
	}

	if len(imagesJSON) > 0 {
		if err := json.Unmarshal(imagesJSON, &product.Images); err != nil {
			return nil, fmt.Errorf("unmarshal images: %w", err)
		}
	}

	return &product, nil
}

// ListProducts retrieves products for a tenant with optional filtering and merging with master products
func (a *CatalogAdapter) ListProducts(ctx context.Context, tenantID string, filter ports.ProductFilter) ([]domain.Product, int, error) {
	if sc := domain.SpanFromContext(ctx); sc != nil {
		endSpan := sc.Start("db.list_products")
		defer endSpan()
	}
	// Build query with filters
	baseQuery := `
		SELECT
			p.id, p.tenant_id, COALESCE(p.master_product_id::text, '') as master_product_id,
			COALESCE(p.name, '') as name, COALESCE(p.description, '') as description,
			p.price, p.currency, COALESCE(s.quantity, 0) as stock_quantity, COALESCE(p.rating, 0) as rating,
			COALESCE(p.images, '[]') as images, COALESCE(p.tags, '[]') as tags,
			mp.id as mp_id, mp.sku, mp.name as mp_name, mp.description as mp_description,
			mp.brand, mp.category_id, mp.images as mp_images,
			c.name as category_name,
			COALESCE(mp.product_form, '') as product_form,
			COALESCE(mp.texture, '') as texture,
			COALESCE(mp.routine_step, '') as routine_step,
			mp.skin_type, mp.concern, mp.key_ingredients, mp.target_area,
			COALESCE(mp.marketing_claim, '') as marketing_claim,
			mp.benefits
		FROM catalog.products p
		LEFT JOIN catalog.master_products mp ON p.master_product_id = mp.id
		LEFT JOIN catalog.categories c ON mp.category_id = c.id
		LEFT JOIN catalog.stock s ON s.product_id = p.id AND s.tenant_id = p.tenant_id
		WHERE p.tenant_id = $1
	`

	countQuery := `
		SELECT COUNT(*)
		FROM catalog.products p
		LEFT JOIN catalog.master_products mp ON p.master_product_id = mp.id
		LEFT JOIN catalog.categories c ON mp.category_id = c.id
		WHERE p.tenant_id = $1
	`

	args := []interface{}{tenantID}
	argNum := 2
	var conditions []string

	if filter.CategoryID != "" {
		conditions = append(conditions, fmt.Sprintf("mp.category_id = $%d", argNum))
		args = append(args, filter.CategoryID)
		argNum++
	}

	if filter.Brand != "" {
		conditions = append(conditions, fmt.Sprintf("mp.brand ILIKE $%d", argNum))
		args = append(args, "%"+filter.Brand+"%")
		argNum++
	}

	if filter.MinPrice > 0 {
		conditions = append(conditions, fmt.Sprintf("p.price >= $%d", argNum))
		args = append(args, filter.MinPrice)
		argNum++
	}

	if filter.MaxPrice > 0 {
		conditions = append(conditions, fmt.Sprintf("p.price <= $%d", argNum))
		args = append(args, filter.MaxPrice)
		argNum++
	}

	if filter.Search != "" {
		// Split search into words and match ANY word in any field (OR between words)
		words := strings.Fields(filter.Search)
		if len(words) == 1 {
			conditions = append(conditions, fmt.Sprintf("(p.name ILIKE $%d OR mp.name ILIKE $%d OR mp.brand ILIKE $%d)", argNum, argNum, argNum))
			args = append(args, "%"+words[0]+"%")
			argNum++
		} else if len(words) > 1 {
			var wordConds []string
			for _, word := range words {
				wordConds = append(wordConds, fmt.Sprintf("(p.name ILIKE $%d OR mp.name ILIKE $%d OR mp.brand ILIKE $%d)", argNum, argNum, argNum))
				args = append(args, "%"+word+"%")
				argNum++
			}
			conditions = append(conditions, "("+strings.Join(wordConds, " OR ")+")")
		}
	}

	if filter.CategoryName != "" {
		conditions = append(conditions, fmt.Sprintf("(c.name ILIKE $%d OR c.slug ILIKE $%d)", argNum, argNum))
		args = append(args, "%"+filter.CategoryName+"%")
		argNum++
	}

	// Typed PIM filters
	if filter.ProductForm != "" {
		conditions = append(conditions, fmt.Sprintf("mp.product_form = $%d", argNum))
		args = append(args, filter.ProductForm)
		argNum++
	}
	if filter.SkinType != "" {
		conditions = append(conditions, fmt.Sprintf("$%d = ANY(mp.skin_type)", argNum))
		args = append(args, filter.SkinType)
		argNum++
	}
	if filter.Concern != "" {
		conditions = append(conditions, fmt.Sprintf("$%d = ANY(mp.concern)", argNum))
		args = append(args, filter.Concern)
		argNum++
	}
	if filter.KeyIngredient != "" {
		conditions = append(conditions, fmt.Sprintf("$%d = ANY(mp.key_ingredients)", argNum))
		args = append(args, filter.KeyIngredient)
		argNum++
	}
	if filter.TargetArea != "" {
		conditions = append(conditions, fmt.Sprintf("$%d = ANY(mp.target_area)", argNum))
		args = append(args, filter.TargetArea)
		argNum++
	}
	if filter.RoutineStep != "" {
		conditions = append(conditions, fmt.Sprintf("mp.routine_step = $%d", argNum))
		args = append(args, filter.RoutineStep)
		argNum++
	}
	if filter.Texture != "" {
		conditions = append(conditions, fmt.Sprintf("mp.texture = $%d", argNum))
		args = append(args, filter.Texture)
		argNum++
	}

	if len(conditions) > 0 {
		condStr := " AND " + strings.Join(conditions, " AND ")
		baseQuery += condStr
		countQuery += condStr
	}

	// Get total count
	var total int
	if err := a.client.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count products: %w", err)
	}

	// Add pagination
	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	// Dynamic ORDER BY (whitelist only — prevents SQL injection)
	orderClause := "p.created_at DESC"
	if filter.SortField != "" {
		sortOrder := "ASC"
		if strings.ToUpper(filter.SortOrder) == "DESC" {
			sortOrder = "DESC"
		}
		switch filter.SortField {
		case "price":
			orderClause = fmt.Sprintf("p.price %s", sortOrder)
		case "rating":
			orderClause = fmt.Sprintf("p.rating %s", sortOrder)
		case "name":
			orderClause = fmt.Sprintf("COALESCE(p.name, mp.name) %s", sortOrder)
		}
	}
	baseQuery += fmt.Sprintf(" ORDER BY %s LIMIT $%d OFFSET $%d", orderClause, argNum, argNum+1)
	args = append(args, limit, offset)

	rows, err := a.client.pool.Query(ctx, baseQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("query products: %w", err)
	}
	defer rows.Close()

	var products []domain.Product
	for rows.Next() {
		var p domain.Product
		var masterProductID, mpID, mpSKU, mpName, mpDesc, mpBrand, mpCategoryID, categoryName *string
		var productImagesJSON, tagsJSON, mpImagesJSON []byte
		var mpProductForm, mpTexture, mpRoutineStep, mpMarketingClaim *string
		var mpSkinType, mpConcern, mpKeyIngredients, mpTargetArea, mpBenefits []string

		err := rows.Scan(
			&p.ID, &p.TenantID, &masterProductID,
			&p.Name, &p.Description, &p.Price, &p.Currency, &p.StockQuantity, &p.Rating, &productImagesJSON, &tagsJSON,
			&mpID, &mpSKU, &mpName, &mpDesc,
			&mpBrand, &mpCategoryID, &mpImagesJSON,
			&categoryName,
			&mpProductForm, &mpTexture, &mpRoutineStep,
			&mpSkinType, &mpConcern, &mpKeyIngredients, &mpTargetArea,
			&mpMarketingClaim, &mpBenefits,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("scan product: %w", err)
		}

		// Parse product images
		if len(productImagesJSON) > 0 {
			if err := json.Unmarshal(productImagesJSON, &p.Images); err != nil {
				return nil, 0, fmt.Errorf("unmarshal product images: %w", err)
			}
		}

		// Parse tags
		if len(tagsJSON) > 0 {
			json.Unmarshal(tagsJSON, &p.Tags)
		}

		// Merge with master product data
		if err := mergeProductWithMaster(&p, masterProductRow{
			MasterProductID: masterProductID,
			Name:            mpName,
			Description:     mpDesc,
			Brand:           mpBrand,
			CategoryName:    categoryName,
			ImagesJSON:      mpImagesJSON,
			ProductForm:     mpProductForm,
			Texture:         mpTexture,
			RoutineStep:     mpRoutineStep,
			SkinType:        mpSkinType,
			Concern:         mpConcern,
			KeyIngredients:  mpKeyIngredients,
			TargetArea:      mpTargetArea,
			MarketingClaim:  mpMarketingClaim,
			Benefits:        mpBenefits,
		}); err != nil {
			return nil, 0, err
		}

		// Format price (kopecks to rubles with formatting)
		p.PriceFormatted = formatPrice(p.Price, p.Currency)

		products = append(products, p)
	}

	return products, total, nil
}

// GetProduct retrieves a single product by ID with master data merging
func (a *CatalogAdapter) GetProduct(ctx context.Context, tenantID string, productID string) (*domain.Product, error) {
	if sc := domain.SpanFromContext(ctx); sc != nil {
		endSpan := sc.Start("db.get_product")
		defer endSpan()
	}
	query := `
		SELECT
			p.id, p.tenant_id, COALESCE(p.master_product_id::text, '') as master_product_id,
			COALESCE(p.name, '') as name, COALESCE(p.description, '') as description,
			p.price, p.currency, COALESCE(s.quantity, 0) as stock_quantity, COALESCE(p.rating, 0) as rating,
			COALESCE(p.images, '[]') as images, COALESCE(p.tags, '[]') as tags,
			mp.id as mp_id, mp.sku, mp.name as mp_name, mp.description as mp_description,
			mp.brand, mp.category_id, mp.images as mp_images,
			c.name as category_name,
			COALESCE(mp.product_form, '') as product_form,
			COALESCE(mp.texture, '') as texture,
			COALESCE(mp.routine_step, '') as routine_step,
			mp.skin_type, mp.concern, mp.key_ingredients, mp.target_area,
			COALESCE(mp.marketing_claim, '') as marketing_claim,
			mp.benefits
		FROM catalog.products p
		LEFT JOIN catalog.master_products mp ON p.master_product_id = mp.id
		LEFT JOIN catalog.categories c ON mp.category_id = c.id
		LEFT JOIN catalog.stock s ON s.product_id = p.id AND s.tenant_id = p.tenant_id
		WHERE p.tenant_id = $1 AND p.id = $2
	`

	var p domain.Product
	var masterProductID, mpID, mpSKU, mpName, mpDesc, mpBrand, mpCategoryID, categoryName *string
	var productImagesJSON, tagsJSON, mpImagesJSON []byte
	var mpProductForm, mpTexture, mpRoutineStep, mpMarketingClaim *string
	var mpSkinType, mpConcern, mpKeyIngredients, mpTargetArea, mpBenefits []string

	err := a.client.pool.QueryRow(ctx, query, tenantID, productID).Scan(
		&p.ID, &p.TenantID, &masterProductID,
		&p.Name, &p.Description, &p.Price, &p.Currency, &p.StockQuantity, &p.Rating, &productImagesJSON, &tagsJSON,
		&mpID, &mpSKU, &mpName, &mpDesc,
		&mpBrand, &mpCategoryID, &mpImagesJSON,
		&categoryName,
		&mpProductForm, &mpTexture, &mpRoutineStep,
		&mpSkinType, &mpConcern, &mpKeyIngredients, &mpTargetArea,
		&mpMarketingClaim, &mpBenefits,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrProductNotFound
		}
		return nil, fmt.Errorf("query product: %w", err)
	}

	// Parse product images
	if len(productImagesJSON) > 0 {
		if err := json.Unmarshal(productImagesJSON, &p.Images); err != nil {
			return nil, fmt.Errorf("unmarshal product images: %w", err)
		}
	}

	// Parse tags
	if len(tagsJSON) > 0 {
		json.Unmarshal(tagsJSON, &p.Tags)
	}

	// Merge with master product data
	if err := mergeProductWithMaster(&p, masterProductRow{
		MasterProductID: masterProductID,
		Name:            mpName,
		Description:     mpDesc,
		Brand:           mpBrand,
		CategoryName:    categoryName,
		ImagesJSON:      mpImagesJSON,
		ProductForm:     mpProductForm,
		Texture:         mpTexture,
		RoutineStep:     mpRoutineStep,
		SkinType:        mpSkinType,
		Concern:         mpConcern,
		KeyIngredients:  mpKeyIngredients,
		TargetArea:      mpTargetArea,
		MarketingClaim:  mpMarketingClaim,
		Benefits:        mpBenefits,
	}); err != nil {
		return nil, err
	}

	p.PriceFormatted = formatPrice(p.Price, p.Currency)

	return &p, nil
}

// formatPrice formats price from kopecks to rubles with thousand separators
func formatPrice(kopecks int, currency string) string {
	rubles := kopecks / 100

	// Format with thousand separators
	str := fmt.Sprintf("%d", rubles)
	var result strings.Builder

	for i, c := range str {
		if i > 0 && (len(str)-i)%3 == 0 {
			result.WriteString(" ")
		}
		result.WriteRune(c)
	}

	var symbol string
	switch currency {
	case "USD":
		symbol = "$"
	case "EUR":
		symbol = "€"
	default:
		symbol = "₽"
	}

	return result.String() + " " + symbol
}

// masterProductRow holds scanned master-product join columns.
type masterProductRow struct {
	MasterProductID *string
	Name            *string
	Description     *string
	Brand           *string
	CategoryName    *string
	ImagesJSON      []byte
	// PIM fields
	ProductForm    *string
	Texture        *string
	RoutineStep    *string
	SkinType       []string
	Concern        []string
	KeyIngredients []string
	TargetArea     []string
	MarketingClaim *string
	Benefits       []string
}

// mergeProductWithMaster fills product fields from a master-product row.
func mergeProductWithMaster(p *domain.Product, mp masterProductRow) error {
	if mp.MasterProductID == nil || *mp.MasterProductID == "" {
		return nil
	}
	p.MasterProductID = *mp.MasterProductID

	if p.Name == "" && mp.Name != nil {
		p.Name = *mp.Name
	}
	if p.Description == "" && mp.Description != nil {
		p.Description = *mp.Description
	}
	if mp.Brand != nil {
		p.Brand = *mp.Brand
	}
	if mp.CategoryName != nil {
		p.Category = *mp.CategoryName
	}
	if len(p.Images) == 0 && len(mp.ImagesJSON) > 0 {
		if err := json.Unmarshal(mp.ImagesJSON, &p.Images); err != nil {
			return fmt.Errorf("unmarshal master images: %w", err)
		}
	}
	// PIM fields — name already contains the clean short name from DB
	if mp.ProductForm != nil {
		p.ProductForm = *mp.ProductForm
	}
	if mp.Texture != nil {
		p.Texture = *mp.Texture
	}
	if mp.RoutineStep != nil {
		p.RoutineStep = *mp.RoutineStep
	}
	p.SkinType = mp.SkinType
	p.Concern = mp.Concern
	p.KeyIngredients = mp.KeyIngredients
	p.TargetArea = mp.TargetArea
	if mp.MarketingClaim != nil {
		p.MarketingClaim = *mp.MarketingClaim
	}
	p.Benefits = mp.Benefits
	return nil
}

// VectorSearch finds products by semantic similarity via pgvector cosine distance.
// filter may be nil for unfiltered search.
func (a *CatalogAdapter) VectorSearch(ctx context.Context, tenantID string, embedding []float32, limit int, filter *ports.VectorFilter) ([]domain.Product, error) {
	if sc := domain.SpanFromContext(ctx); sc != nil {
		endSpan := sc.Start("db.vector_search")
		defer endSpan()
	}
	query := `
		SELECT
			p.id, p.tenant_id, COALESCE(p.master_product_id::text, '') as master_product_id,
			COALESCE(p.name, '') as name, COALESCE(p.description, '') as description,
			p.price, p.currency, COALESCE(st.quantity, 0) as stock_quantity, COALESCE(p.rating, 0) as rating,
			COALESCE(p.images, '[]') as images, COALESCE(p.tags, '[]') as tags,
			mp.id as mp_id, mp.sku, mp.name as mp_name, mp.description as mp_description,
			mp.brand, mp.category_id, mp.images as mp_images,
			c.name as category_name,
			COALESCE(mp.product_form, '') as product_form,
			COALESCE(mp.texture, '') as texture,
			COALESCE(mp.routine_step, '') as routine_step,
			mp.skin_type, mp.concern, mp.key_ingredients, mp.target_area,
			COALESCE(mp.marketing_claim, '') as marketing_claim,
			mp.benefits
		FROM catalog.products p
		JOIN catalog.master_products mp ON p.master_product_id = mp.id
		LEFT JOIN catalog.categories c ON mp.category_id = c.id
		LEFT JOIN catalog.stock st ON st.product_id = p.id AND st.tenant_id = p.tenant_id
		WHERE p.tenant_id = $1
		  AND mp.embedding IS NOT NULL
	`

	args := []interface{}{tenantID, pgvector.NewVector(embedding)}
	argNum := 3

	if filter != nil {
		if filter.Brand != "" {
			query += fmt.Sprintf(" AND mp.brand ILIKE $%d", argNum)
			args = append(args, "%"+filter.Brand+"%")
			argNum++
		}
		if filter.CategoryName != "" {
			query += fmt.Sprintf(" AND (c.name ILIKE $%d OR c.slug ILIKE $%d)", argNum, argNum)
			args = append(args, "%"+filter.CategoryName+"%")
			argNum++
		}
		if filter.ProductForm != "" {
			query += fmt.Sprintf(" AND mp.product_form = $%d", argNum)
			args = append(args, filter.ProductForm)
			argNum++
		}
		if filter.SkinType != "" {
			query += fmt.Sprintf(" AND $%d = ANY(mp.skin_type)", argNum)
			args = append(args, filter.SkinType)
			argNum++
		}
		if filter.Concern != "" {
			query += fmt.Sprintf(" AND $%d = ANY(mp.concern)", argNum)
			args = append(args, filter.Concern)
			argNum++
		}
		if filter.RoutineStep != "" {
			query += fmt.Sprintf(" AND mp.routine_step = $%d", argNum)
			args = append(args, filter.RoutineStep)
			argNum++
		}
		if filter.Texture != "" {
			query += fmt.Sprintf(" AND mp.texture = $%d", argNum)
			args = append(args, filter.Texture)
			argNum++
		}
		if filter.KeyIngredient != "" {
			query += fmt.Sprintf(" AND $%d = ANY(mp.key_ingredients)", argNum)
			args = append(args, filter.KeyIngredient)
			argNum++
		}
		if filter.TargetArea != "" {
			query += fmt.Sprintf(" AND $%d = ANY(mp.target_area)", argNum)
			args = append(args, filter.TargetArea)
			argNum++
		}
	}

	query += fmt.Sprintf(" ORDER BY mp.embedding <=> $2 LIMIT $%d", argNum)
	args = append(args, limit)

	rows, err := a.client.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("vector search: %w", err)
	}
	defer rows.Close()

	var products []domain.Product
	for rows.Next() {
		var p domain.Product
		var masterProductID, mpID, mpSKU, mpName, mpDesc, mpBrand, mpCategoryID, categoryName *string
		var productImagesJSON, tagsJSON, mpImagesJSON []byte
		var mpProductForm, mpTexture, mpRoutineStep, mpMarketingClaim *string
		var mpSkinType, mpConcern, mpKeyIngredients, mpTargetArea, mpBenefits []string

		err := rows.Scan(
			&p.ID, &p.TenantID, &masterProductID,
			&p.Name, &p.Description, &p.Price, &p.Currency, &p.StockQuantity, &p.Rating, &productImagesJSON, &tagsJSON,
			&mpID, &mpSKU, &mpName, &mpDesc,
			&mpBrand, &mpCategoryID, &mpImagesJSON,
			&categoryName,
			&mpProductForm, &mpTexture, &mpRoutineStep,
			&mpSkinType, &mpConcern, &mpKeyIngredients, &mpTargetArea,
			&mpMarketingClaim, &mpBenefits,
		)
		if err != nil {
			return nil, fmt.Errorf("scan vector product: %w", err)
		}

		if len(productImagesJSON) > 0 {
			if err := json.Unmarshal(productImagesJSON, &p.Images); err != nil {
				return nil, fmt.Errorf("unmarshal product images: %w", err)
			}
		}

		if len(tagsJSON) > 0 {
			json.Unmarshal(tagsJSON, &p.Tags)
		}

		if err := mergeProductWithMaster(&p, masterProductRow{
			MasterProductID: masterProductID,
			Name:            mpName,
			Description:     mpDesc,
			Brand:           mpBrand,
			CategoryName:    categoryName,
			ImagesJSON:      mpImagesJSON,
			ProductForm:     mpProductForm,
			Texture:         mpTexture,
			RoutineStep:     mpRoutineStep,
			SkinType:        mpSkinType,
			Concern:         mpConcern,
			KeyIngredients:  mpKeyIngredients,
			TargetArea:      mpTargetArea,
			MarketingClaim:  mpMarketingClaim,
			Benefits:        mpBenefits,
		}); err != nil {
			return nil, err
		}

		p.PriceFormatted = formatPrice(p.Price, p.Currency)
		products = append(products, p)
	}

	return products, nil
}

// SeedEmbedding saves embedding for a master product.
func (a *CatalogAdapter) SeedEmbedding(ctx context.Context, masterProductID string, embedding []float32) error {
	query := `UPDATE catalog.master_products SET embedding = $2 WHERE id = $1`
	_, err := a.client.pool.Exec(ctx, query, masterProductID, pgvector.NewVector(embedding))
	if err != nil {
		return fmt.Errorf("seed embedding: %w", err)
	}
	return nil
}

// GetMasterProductsWithoutEmbedding returns master products that need embeddings.
// Includes CategoryName via JOIN for richer embedding text.
func (a *CatalogAdapter) GetMasterProductsWithoutEmbedding(ctx context.Context) ([]domain.MasterProduct, error) {
	query := `
		SELECT mp.id, mp.sku, mp.name, COALESCE(mp.description, '') as description,
		       COALESCE(mp.brand, '') as brand, COALESCE(mp.category_id::text, '') as category_id,
		       COALESCE(c.name, '') as category_name,
		       COALESCE(mp.product_form, '') as product_form,
		       COALESCE(mp.texture, '') as texture,
		       COALESCE(mp.routine_step, '') as routine_step,
		       mp.skin_type, mp.concern, mp.key_ingredients, mp.target_area,
		       COALESCE(mp.marketing_claim, '') as marketing_claim,
		       mp.benefits,
		       COALESCE(mp.enrichment_version, 0) as enrichment_version
		FROM catalog.master_products mp
		LEFT JOIN catalog.categories c ON mp.category_id = c.id
		WHERE mp.embedding IS NULL
		ORDER BY mp.created_at
	`

	rows, err := a.client.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query products without embedding: %w", err)
	}
	defer rows.Close()

	var products []domain.MasterProduct
	for rows.Next() {
		var p domain.MasterProduct
		if err := rows.Scan(
			&p.ID, &p.SKU, &p.Name, &p.Description, &p.Brand, &p.CategoryID, &p.CategoryName,
			&p.ProductForm, &p.Texture, &p.RoutineStep,
			&p.SkinType, &p.Concern, &p.KeyIngredients, &p.TargetArea,
			&p.MarketingClaim, &p.Benefits, &p.EnrichmentVersion,
		); err != nil {
			return nil, fmt.Errorf("scan master product: %w", err)
		}
		products = append(products, p)
	}

	return products, nil
}

// GetAllTenants returns all tenants for batch operations (e.g. digest generation).
func (a *CatalogAdapter) GetAllTenants(ctx context.Context) ([]domain.Tenant, error) {
	query := `
		SELECT id, slug, name, type, settings, created_at, updated_at
		FROM catalog.tenants
		ORDER BY slug
	`

	rows, err := a.client.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query all tenants: %w", err)
	}
	defer rows.Close()

	var tenants []domain.Tenant
	for rows.Next() {
		var t domain.Tenant
		var settingsJSON []byte
		if err := rows.Scan(&t.ID, &t.Slug, &t.Name, &t.Type, &settingsJSON, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan tenant: %w", err)
		}
		if len(settingsJSON) > 0 {
			if err := json.Unmarshal(settingsJSON, &t.Settings); err != nil {
				a.log.Warn("unmarshal tenant settings", "tenant", t.Slug, "error", err)
			}
		}
		tenants = append(tenants, t)
	}

	return tenants, nil
}

// GenerateCatalogDigest computes a compact catalog meta-schema for a tenant.
func (a *CatalogAdapter) GenerateCatalogDigest(ctx context.Context, tenantID string) (*domain.CatalogDigest, error) {
	// Query 1: category tree (with parent_id for grouping)
	catQuery := `
		SELECT c.name, c.slug, COALESCE(pc.slug, '') AS parent_slug,
		       COUNT(DISTINCT mp.id) AS product_count
		FROM catalog.products p
		JOIN catalog.master_products mp ON p.master_product_id = mp.id
		JOIN catalog.categories c ON mp.category_id = c.id
		LEFT JOIN catalog.categories pc ON c.parent_id = pc.id
		WHERE p.tenant_id = $1
		GROUP BY c.id, c.name, c.slug, pc.slug
		ORDER BY product_count DESC
	`
	catRows, err := a.client.pool.Query(ctx, catQuery, tenantID)
	if err != nil {
		return nil, fmt.Errorf("query categories for digest: %w", err)
	}
	defer catRows.Close()

	type leafInfo struct {
		name       string
		slug       string
		parentSlug string
		count      int
	}
	var leaves []leafInfo
	totalProducts := 0

	for catRows.Next() {
		var li leafInfo
		if err := catRows.Scan(&li.name, &li.slug, &li.parentSlug, &li.count); err != nil {
			return nil, fmt.Errorf("scan category digest: %w", err)
		}
		totalProducts += li.count
		leaves = append(leaves, li)
	}

	if len(leaves) == 0 {
		return &domain.CatalogDigest{
			GeneratedAt:   time.Now(),
			TotalProducts: 0,
		}, nil
	}

	// Build category tree: group by parent
	groupMap := make(map[string]*domain.DigestCategoryGroup)
	var groupOrder []string
	for _, li := range leaves {
		parent := li.parentSlug
		if parent == "" {
			parent = li.slug // root category without parent = standalone group
		}
		if _, ok := groupMap[parent]; !ok {
			groupMap[parent] = &domain.DigestCategoryGroup{Slug: parent, Name: parent}
			groupOrder = append(groupOrder, parent)
		}
		// Root-level categories (no parent) add themselves as a leaf;
		// sub-categories add under their parent group.
		groupMap[parent].Children = append(groupMap[parent].Children, domain.DigestCategoryLeaf{
			Name: li.name, Slug: li.slug, Count: li.count,
		})
	}

	tree := make([]domain.DigestCategoryGroup, 0, len(groupOrder))
	for _, slug := range groupOrder {
		tree = append(tree, *groupMap[slug])
	}

	// Query 2: shared filters — global distinct values (NOT per-category)
	filterQuery := `
		SELECT attr_key, ARRAY_AGG(DISTINCT attr_value ORDER BY attr_value) AS all_values
		FROM (
			SELECT 'product_form' AS attr_key, mp.product_form AS attr_value
			FROM catalog.products p JOIN catalog.master_products mp ON p.master_product_id = mp.id
			WHERE p.tenant_id = $1 AND mp.product_form IS NOT NULL AND mp.product_form != ''
			UNION ALL
			SELECT 'texture', mp.texture
			FROM catalog.products p JOIN catalog.master_products mp ON p.master_product_id = mp.id
			WHERE p.tenant_id = $1 AND mp.texture IS NOT NULL AND mp.texture != ''
			UNION ALL
			SELECT 'routine_step', mp.routine_step
			FROM catalog.products p JOIN catalog.master_products mp ON p.master_product_id = mp.id
			WHERE p.tenant_id = $1 AND mp.routine_step IS NOT NULL AND mp.routine_step != ''
			UNION ALL
			SELECT 'skin_type', unnest(mp.skin_type)
			FROM catalog.products p JOIN catalog.master_products mp ON p.master_product_id = mp.id
			WHERE p.tenant_id = $1 AND mp.skin_type IS NOT NULL
			UNION ALL
			SELECT 'concern', unnest(mp.concern)
			FROM catalog.products p JOIN catalog.master_products mp ON p.master_product_id = mp.id
			WHERE p.tenant_id = $1 AND mp.concern IS NOT NULL
			UNION ALL
			SELECT 'key_ingredient', unnest(mp.key_ingredients)
			FROM catalog.products p JOIN catalog.master_products mp ON p.master_product_id = mp.id
			WHERE p.tenant_id = $1 AND mp.key_ingredients IS NOT NULL
			UNION ALL
			SELECT 'target_area', unnest(mp.target_area)
			FROM catalog.products p JOIN catalog.master_products mp ON p.master_product_id = mp.id
			WHERE p.tenant_id = $1 AND mp.target_area IS NOT NULL
		) AS attrs
		WHERE attr_value IS NOT NULL AND attr_value != ''
		GROUP BY attr_key
		ORDER BY attr_key
	`
	filterRows, err := a.client.pool.Query(ctx, filterQuery, tenantID)
	if err != nil {
		return nil, fmt.Errorf("query filters for digest: %w", err)
	}
	defer filterRows.Close()

	var sharedFilters []domain.DigestSharedFilter
	for filterRows.Next() {
		var key string
		var values []string
		if err := filterRows.Scan(&key, &values); err != nil {
			return nil, fmt.Errorf("scan filter digest: %w", err)
		}
		sharedFilters = append(sharedFilters, domain.DigestSharedFilter{Key: key, Values: values})
	}

	// Query 3: top brands (top 30 by product count)
	brandQuery := `
		SELECT mp.brand
		FROM catalog.products p
		JOIN catalog.master_products mp ON p.master_product_id = mp.id
		WHERE p.tenant_id = $1 AND mp.brand IS NOT NULL AND mp.brand != ''
		GROUP BY mp.brand
		ORDER BY COUNT(*) DESC
		LIMIT 30
	`
	brandRows, err := a.client.pool.Query(ctx, brandQuery, tenantID)
	if err != nil {
		return nil, fmt.Errorf("query brands for digest: %w", err)
	}
	defer brandRows.Close()

	var topBrands []string
	for brandRows.Next() {
		var brand string
		if err := brandRows.Scan(&brand); err != nil {
			return nil, fmt.Errorf("scan brand digest: %w", err)
		}
		topBrands = append(topBrands, brand)
	}

	// Query 4: top ingredients (top 30 by frequency)
	var topIngredients []string
	ingrQuery := `
		SELECT i.inci_name
		FROM catalog.product_ingredients pi
		JOIN catalog.ingredients i ON pi.ingredient_id = i.id
		JOIN catalog.master_products mp ON pi.master_product_id = mp.id
		JOIN catalog.products p ON p.master_product_id = mp.id
		WHERE p.tenant_id = $1
		GROUP BY i.inci_name
		ORDER BY COUNT(*) DESC
		LIMIT 30
	`
	ingrRows, err := a.client.pool.Query(ctx, ingrQuery, tenantID)
	if err != nil {
		// Not fatal — ingredients table may not be seeded yet
		a.log.Warn("digest_ingredients_query_failed", "error", err)
	} else {
		defer ingrRows.Close()
		for ingrRows.Next() {
			var name string
			if err := ingrRows.Scan(&name); err != nil {
				a.log.Warn("digest_ingredient_scan_failed", "error", err)
				break
			}
			topIngredients = append(topIngredients, name)
		}
	}

	return &domain.CatalogDigest{
		GeneratedAt:    time.Now(),
		TotalProducts:  totalProducts,
		CategoryTree:   tree,
		SharedFilters:  sharedFilters,
		TopBrands:      topBrands,
		TopIngredients: topIngredients,
	}, nil
}

// SaveCatalogDigest persists the computed digest to the tenants table.
func (a *CatalogAdapter) SaveCatalogDigest(ctx context.Context, tenantID string, digest *domain.CatalogDigest) error {
	digestJSON, err := json.Marshal(digest)
	if err != nil {
		return fmt.Errorf("marshal digest: %w", err)
	}

	query := `UPDATE catalog.tenants SET catalog_digest = $2, updated_at = NOW() WHERE id = $1`
	_, err = a.client.pool.Exec(ctx, query, tenantID, digestJSON)
	if err != nil {
		return fmt.Errorf("save digest: %w", err)
	}
	return nil
}

// GetCatalogDigest returns the pre-computed digest from tenants.catalog_digest.
func (a *CatalogAdapter) GetCatalogDigest(ctx context.Context, tenantID string) (*domain.CatalogDigest, error) {
	if sc := domain.SpanFromContext(ctx); sc != nil {
		endSpan := sc.Start("db.get_catalog_digest")
		defer endSpan()
	}
	query := `SELECT catalog_digest FROM catalog.tenants WHERE id = $1`

	var digestJSON []byte
	err := a.client.pool.QueryRow(ctx, query, tenantID).Scan(&digestJSON)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("query digest: %w", err)
	}

	if len(digestJSON) == 0 {
		return nil, nil
	}

	var digest domain.CatalogDigest
	if err := json.Unmarshal(digestJSON, &digest); err != nil {
		return nil, fmt.Errorf("unmarshal digest: %w", err)
	}

	return &digest, nil
}

// --- Stock ---

// GetStock retrieves stock for a specific product.
func (a *CatalogAdapter) GetStock(ctx context.Context, tenantID string, productID string) (*domain.Stock, error) {
	query := `SELECT tenant_id, product_id, quantity, reserved, updated_at
		FROM catalog.stock WHERE tenant_id = $1 AND product_id = $2`

	var s domain.Stock
	err := a.client.pool.QueryRow(ctx, query, tenantID, productID).Scan(
		&s.TenantID, &s.ProductID, &s.Quantity, &s.Reserved, &s.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &domain.Stock{TenantID: tenantID, ProductID: productID}, nil
		}
		return nil, fmt.Errorf("query stock: %w", err)
	}
	return &s, nil
}

// --- Services ---

// masterServiceRow holds scanned master-service join columns.
type masterServiceRow struct {
	MasterServiceID *string
	Name            *string
	Description     *string
	Brand           *string
	Duration        *string
	Provider        *string
	CategoryName    *string
	ImagesJSON      []byte
}

// mergeServiceWithMaster fills service fields from a master-service row.
func mergeServiceWithMaster(s *domain.Service, ms masterServiceRow) error {
	if ms.MasterServiceID == nil || *ms.MasterServiceID == "" {
		return nil
	}
	s.MasterServiceID = *ms.MasterServiceID

	if s.Name == "" && ms.Name != nil {
		s.Name = *ms.Name
	}
	if s.Description == "" && ms.Description != nil {
		s.Description = *ms.Description
	}
	if ms.Duration != nil && s.Duration == "" {
		s.Duration = *ms.Duration
	}
	if ms.Provider != nil && s.Provider == "" {
		s.Provider = *ms.Provider
	}
	if ms.CategoryName != nil {
		s.Category = *ms.CategoryName
	}
	if len(s.Images) == 0 && len(ms.ImagesJSON) > 0 {
		if err := json.Unmarshal(ms.ImagesJSON, &s.Images); err != nil {
			return fmt.Errorf("unmarshal master service images: %w", err)
		}
	}
	return nil
}

// ListServices retrieves services for a tenant with filtering.
func (a *CatalogAdapter) ListServices(ctx context.Context, tenantID string, filter ports.ProductFilter) ([]domain.Service, int, error) {
	if sc := domain.SpanFromContext(ctx); sc != nil {
		endSpan := sc.Start("db.list_services")
		defer endSpan()
	}
	baseQuery := `
		SELECT
			sv.id, sv.tenant_id, COALESCE(sv.master_service_id::text, '') as master_service_id,
			COALESCE(sv.name, '') as name, COALESCE(sv.description, '') as description,
			sv.price, sv.currency, COALESCE(sv.rating, 0) as rating,
			COALESCE(sv.images, '[]') as images, COALESCE(sv.tags, '[]') as tags,
			sv.availability,
			ms.id as ms_id, ms.name as ms_name, ms.description as ms_description,
			ms.brand, ms.duration, ms.provider,
			ms.images as ms_images,
			c.name as category_name
		FROM catalog.services sv
		LEFT JOIN catalog.master_services ms ON sv.master_service_id = ms.id
		LEFT JOIN catalog.categories c ON ms.category_id = c.id
		WHERE sv.tenant_id = $1
	`

	countQuery := `
		SELECT COUNT(*)
		FROM catalog.services sv
		LEFT JOIN catalog.master_services ms ON sv.master_service_id = ms.id
		LEFT JOIN catalog.categories c ON ms.category_id = c.id
		WHERE sv.tenant_id = $1
	`

	args := []interface{}{tenantID}
	argNum := 2
	var conditions []string

	if filter.CategoryName != "" {
		conditions = append(conditions, fmt.Sprintf("(c.name ILIKE $%d OR c.slug ILIKE $%d)", argNum, argNum))
		args = append(args, "%"+filter.CategoryName+"%")
		argNum++
	}

	if filter.Brand != "" {
		conditions = append(conditions, fmt.Sprintf("ms.brand ILIKE $%d", argNum))
		args = append(args, "%"+filter.Brand+"%")
		argNum++
	}

	if filter.MinPrice > 0 {
		conditions = append(conditions, fmt.Sprintf("sv.price >= $%d", argNum))
		args = append(args, filter.MinPrice)
		argNum++
	}

	if filter.MaxPrice > 0 {
		conditions = append(conditions, fmt.Sprintf("sv.price <= $%d", argNum))
		args = append(args, filter.MaxPrice)
		argNum++
	}

	if filter.Search != "" {
		words := strings.Fields(filter.Search)
		if len(words) == 1 {
			conditions = append(conditions, fmt.Sprintf("(sv.name ILIKE $%d OR ms.name ILIKE $%d OR ms.brand ILIKE $%d)", argNum, argNum, argNum))
			args = append(args, "%"+words[0]+"%")
			argNum++
		} else if len(words) > 1 {
			var wordConds []string
			for _, word := range words {
				wordConds = append(wordConds, fmt.Sprintf("(sv.name ILIKE $%d OR ms.name ILIKE $%d OR ms.brand ILIKE $%d)", argNum, argNum, argNum))
				args = append(args, "%"+word+"%")
				argNum++
			}
			conditions = append(conditions, "("+strings.Join(wordConds, " OR ")+")")
		}
	}

	if len(conditions) > 0 {
		condStr := " AND " + strings.Join(conditions, " AND ")
		baseQuery += condStr
		countQuery += condStr
	}

	var total int
	if err := a.client.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count services: %w", err)
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	orderClause := "sv.created_at DESC"
	if filter.SortField != "" {
		sortOrder := "ASC"
		if strings.ToUpper(filter.SortOrder) == "DESC" {
			sortOrder = "DESC"
		}
		switch filter.SortField {
		case "price":
			orderClause = fmt.Sprintf("sv.price %s", sortOrder)
		case "rating":
			orderClause = fmt.Sprintf("sv.rating %s", sortOrder)
		case "name":
			orderClause = fmt.Sprintf("COALESCE(sv.name, ms.name) %s", sortOrder)
		}
	}
	baseQuery += fmt.Sprintf(" ORDER BY %s LIMIT $%d OFFSET $%d", orderClause, argNum, argNum+1)
	args = append(args, limit, offset)

	rows, err := a.client.pool.Query(ctx, baseQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("query services: %w", err)
	}
	defer rows.Close()

	var services []domain.Service
	for rows.Next() {
		var s domain.Service
		var masterServiceID, msID, msName, msDesc, msBrand, msDuration, msProvider, categoryName *string
		var serviceImagesJSON, tagsJSON, msImagesJSON []byte

		err := rows.Scan(
			&s.ID, &s.TenantID, &masterServiceID,
			&s.Name, &s.Description,
			&s.Price, &s.Currency, &s.Rating,
			&serviceImagesJSON, &tagsJSON,
			&s.Availability,
			&msID, &msName, &msDesc,
			&msBrand, &msDuration, &msProvider,
			&msImagesJSON,
			&categoryName,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("scan service: %w", err)
		}

		if len(serviceImagesJSON) > 0 {
			json.Unmarshal(serviceImagesJSON, &s.Images)
		}
		if len(tagsJSON) > 0 {
			json.Unmarshal(tagsJSON, &s.Tags)
		}

		if err := mergeServiceWithMaster(&s, masterServiceRow{
			MasterServiceID: masterServiceID,
			Name:            msName,
			Description:     msDesc,
			Brand:           msBrand,
			Duration:        msDuration,
			Provider:        msProvider,
			CategoryName:    categoryName,
			ImagesJSON:      msImagesJSON,
		}); err != nil {
			return nil, 0, err
		}

		s.PriceFormatted = formatPrice(s.Price, s.Currency)
		services = append(services, s)
	}

	return services, total, nil
}

// GetService retrieves a single service by ID with master data merging.
func (a *CatalogAdapter) GetService(ctx context.Context, tenantID string, serviceID string) (*domain.Service, error) {
	if sc := domain.SpanFromContext(ctx); sc != nil {
		endSpan := sc.Start("db.get_service")
		defer endSpan()
	}
	query := `
		SELECT
			sv.id, sv.tenant_id, COALESCE(sv.master_service_id::text, '') as master_service_id,
			COALESCE(sv.name, '') as name, COALESCE(sv.description, '') as description,
			sv.price, sv.currency, COALESCE(sv.rating, 0) as rating,
			COALESCE(sv.images, '[]') as images, COALESCE(sv.tags, '[]') as tags,
			sv.availability,
			ms.id as ms_id, ms.name as ms_name, ms.description as ms_description,
			ms.brand, ms.duration, ms.provider,
			ms.images as ms_images,
			c.name as category_name
		FROM catalog.services sv
		LEFT JOIN catalog.master_services ms ON sv.master_service_id = ms.id
		LEFT JOIN catalog.categories c ON ms.category_id = c.id
		WHERE sv.tenant_id = $1 AND sv.id = $2
	`

	var s domain.Service
	var masterServiceID, msID, msName, msDesc, msBrand, msDuration, msProvider, categoryName *string
	var serviceImagesJSON, tagsJSON, msImagesJSON []byte

	err := a.client.pool.QueryRow(ctx, query, tenantID, serviceID).Scan(
		&s.ID, &s.TenantID, &masterServiceID,
		&s.Name, &s.Description,
		&s.Price, &s.Currency, &s.Rating,
		&serviceImagesJSON, &tagsJSON,
		&s.Availability,
		&msID, &msName, &msDesc,
		&msBrand, &msDuration, &msProvider,
		&msImagesJSON,
		&categoryName,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrProductNotFound
		}
		return nil, fmt.Errorf("query service: %w", err)
	}

	if len(serviceImagesJSON) > 0 {
		json.Unmarshal(serviceImagesJSON, &s.Images)
	}
	if len(tagsJSON) > 0 {
		json.Unmarshal(tagsJSON, &s.Tags)
	}

	if err := mergeServiceWithMaster(&s, masterServiceRow{
		MasterServiceID: masterServiceID,
		Name:            msName,
		Description:     msDesc,
		Brand:           msBrand,
		Duration:        msDuration,
		Provider:        msProvider,
		CategoryName:    categoryName,
		ImagesJSON:      msImagesJSON,
	}); err != nil {
		return nil, err
	}

	s.PriceFormatted = formatPrice(s.Price, s.Currency)
	return &s, nil
}

// VectorSearchServices finds services by semantic similarity via pgvector.
func (a *CatalogAdapter) VectorSearchServices(ctx context.Context, tenantID string, embedding []float32, limit int, filter *ports.VectorFilter) ([]domain.Service, error) {
	if sc := domain.SpanFromContext(ctx); sc != nil {
		endSpan := sc.Start("db.vector_search_services")
		defer endSpan()
	}
	query := `
		SELECT
			sv.id, sv.tenant_id, COALESCE(sv.master_service_id::text, '') as master_service_id,
			COALESCE(sv.name, '') as name, COALESCE(sv.description, '') as description,
			sv.price, sv.currency, COALESCE(sv.rating, 0) as rating,
			COALESCE(sv.images, '[]') as images, COALESCE(sv.tags, '[]') as tags,
			sv.availability,
			ms.id as ms_id, ms.name as ms_name, ms.description as ms_description,
			ms.brand, ms.duration, ms.provider,
			ms.images as ms_images,
			c.name as category_name
		FROM catalog.services sv
		JOIN catalog.master_services ms ON sv.master_service_id = ms.id
		LEFT JOIN catalog.categories c ON ms.category_id = c.id
		WHERE sv.tenant_id = $1
		  AND ms.embedding IS NOT NULL
	`

	args := []interface{}{tenantID, pgvector.NewVector(embedding)}
	argNum := 3

	if filter != nil {
		if filter.Brand != "" {
			query += fmt.Sprintf(" AND ms.brand ILIKE $%d", argNum)
			args = append(args, "%"+filter.Brand+"%")
			argNum++
		}
		if filter.CategoryName != "" {
			query += fmt.Sprintf(" AND (c.name ILIKE $%d OR c.slug ILIKE $%d)", argNum, argNum)
			args = append(args, "%"+filter.CategoryName+"%")
			argNum++
		}
	}

	query += fmt.Sprintf(" ORDER BY ms.embedding <=> $2 LIMIT $%d", argNum)
	args = append(args, limit)

	rows, err := a.client.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("vector search services: %w", err)
	}
	defer rows.Close()

	var services []domain.Service
	for rows.Next() {
		var s domain.Service
		var masterServiceID, msID, msName, msDesc, msBrand, msDuration, msProvider, categoryName *string
		var serviceImagesJSON, tagsJSON, msImagesJSON []byte

		err := rows.Scan(
			&s.ID, &s.TenantID, &masterServiceID,
			&s.Name, &s.Description,
			&s.Price, &s.Currency, &s.Rating,
			&serviceImagesJSON, &tagsJSON,
			&s.Availability,
			&msID, &msName, &msDesc,
			&msBrand, &msDuration, &msProvider,
			&msImagesJSON,
			&categoryName,
		)
		if err != nil {
			return nil, fmt.Errorf("scan vector service: %w", err)
		}

		if len(serviceImagesJSON) > 0 {
			json.Unmarshal(serviceImagesJSON, &s.Images)
		}
		if len(tagsJSON) > 0 {
			json.Unmarshal(tagsJSON, &s.Tags)
		}

		if err := mergeServiceWithMaster(&s, masterServiceRow{
			MasterServiceID: masterServiceID,
			Name:            msName,
			Description:     msDesc,
			Brand:           msBrand,
			Duration:        msDuration,
			Provider:        msProvider,
			CategoryName:    categoryName,
			ImagesJSON:      msImagesJSON,
		}); err != nil {
			return nil, err
		}

		s.PriceFormatted = formatPrice(s.Price, s.Currency)
		services = append(services, s)
	}

	return services, nil
}

// SeedServiceEmbedding saves embedding for a master service.
func (a *CatalogAdapter) SeedServiceEmbedding(ctx context.Context, masterServiceID string, embedding []float32) error {
	query := `UPDATE catalog.master_services SET embedding = $2 WHERE id = $1`
	_, err := a.client.pool.Exec(ctx, query, masterServiceID, pgvector.NewVector(embedding))
	if err != nil {
		return fmt.Errorf("seed service embedding: %w", err)
	}
	return nil
}

// GetMasterServicesWithoutEmbedding returns master services that need embeddings.
func (a *CatalogAdapter) GetMasterServicesWithoutEmbedding(ctx context.Context) ([]domain.MasterService, error) {
	query := `
		SELECT ms.id, ms.sku, ms.name, COALESCE(ms.description, '') as description,
		       COALESCE(ms.brand, '') as brand, COALESCE(ms.category_id::text, '') as category_id,
		       COALESCE(c.name, '') as category_name,
		       COALESCE(ms.duration, '') as duration, COALESCE(ms.provider, '') as provider
		FROM catalog.master_services ms
		LEFT JOIN catalog.categories c ON ms.category_id = c.id
		WHERE ms.embedding IS NULL
		ORDER BY ms.created_at
	`

	rows, err := a.client.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query services without embedding: %w", err)
	}
	defer rows.Close()

	var services []domain.MasterService
	for rows.Next() {
		var s domain.MasterService
		if err := rows.Scan(&s.ID, &s.SKU, &s.Name, &s.Description, &s.Brand, &s.CategoryID, &s.CategoryName, &s.Duration, &s.Provider); err != nil {
			return nil, fmt.Errorf("scan master service: %w", err)
		}
		services = append(services, s)
	}

	return services, nil
}
