package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	pgvector "github.com/pgvector/pgvector-go"
	"keepstar/internal/domain"
	"keepstar/internal/ports"
)

// CatalogAdapter implements CatalogPort for PostgreSQL
type CatalogAdapter struct {
	client *Client
}

// NewCatalogAdapter creates a new CatalogAdapter
func NewCatalogAdapter(client *Client) *CatalogAdapter {
	return &CatalogAdapter{client: client}
}

// GetTenantBySlug retrieves a tenant by its slug
func (a *CatalogAdapter) GetTenantBySlug(ctx context.Context, slug string) (*domain.Tenant, error) {
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
		if err.Error() == "no rows in result set" {
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
		SELECT id, sku, name, description, brand, category_id, images, attributes, owner_tenant_id, created_at, updated_at
		FROM catalog.master_products
		WHERE id = $1
	`

	var product domain.MasterProduct
	var imagesJSON, attributesJSON []byte

	err := a.client.pool.QueryRow(ctx, query, id).Scan(
		&product.ID,
		&product.SKU,
		&product.Name,
		&product.Description,
		&product.Brand,
		&product.CategoryID,
		&imagesJSON,
		&attributesJSON,
		&product.OwnerTenantID,
		&product.CreatedAt,
		&product.UpdatedAt,
	)

	if err != nil {
		if err.Error() == "no rows in result set" {
			return nil, domain.ErrProductNotFound
		}
		return nil, fmt.Errorf("query master product: %w", err)
	}

	if len(imagesJSON) > 0 {
		if err := json.Unmarshal(imagesJSON, &product.Images); err != nil {
			return nil, fmt.Errorf("unmarshal images: %w", err)
		}
	}

	if len(attributesJSON) > 0 {
		if err := json.Unmarshal(attributesJSON, &product.Attributes); err != nil {
			return nil, fmt.Errorf("unmarshal attributes: %w", err)
		}
	}

	return &product, nil
}

// ListProducts retrieves products for a tenant with optional filtering and merging with master products
func (a *CatalogAdapter) ListProducts(ctx context.Context, tenantID string, filter ports.ProductFilter) ([]domain.Product, int, error) {
	// Build query with filters
	baseQuery := `
		SELECT
			p.id, p.tenant_id, COALESCE(p.master_product_id::text, '') as master_product_id,
			COALESCE(p.name, '') as name, COALESCE(p.description, '') as description,
			p.price, p.currency, p.stock_quantity, COALESCE(p.rating, 0) as rating, COALESCE(p.images, '[]') as images,
			mp.id as mp_id, mp.sku, mp.name as mp_name, mp.description as mp_description,
			mp.brand, mp.category_id, mp.images as mp_images, mp.attributes,
			c.name as category_name
		FROM catalog.products p
		LEFT JOIN catalog.master_products mp ON p.master_product_id = mp.id
		LEFT JOIN catalog.categories c ON mp.category_id = c.id
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

	for key, value := range filter.Attributes {
		conditions = append(conditions, fmt.Sprintf("mp.attributes->>$%d ILIKE $%d", argNum, argNum+1))
		args = append(args, key, "%"+value+"%")
		argNum += 2
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
		var productImagesJSON, mpImagesJSON, attributesJSON []byte

		err := rows.Scan(
			&p.ID, &p.TenantID, &masterProductID,
			&p.Name, &p.Description, &p.Price, &p.Currency, &p.StockQuantity, &p.Rating, &productImagesJSON,
			&mpID, &mpSKU, &mpName, &mpDesc,
			&mpBrand, &mpCategoryID, &mpImagesJSON, &attributesJSON,
			&categoryName,
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

		// Merge with master product data
		if masterProductID != nil && *masterProductID != "" {
			p.MasterProductID = *masterProductID

			// Use master name if product name is empty
			if p.Name == "" && mpName != nil {
				p.Name = *mpName
			}

			// Use master description if product description is empty
			if p.Description == "" && mpDesc != nil {
				p.Description = *mpDesc
			}

			// Set brand from master
			if mpBrand != nil {
				p.Brand = *mpBrand
			}

			// Set category from master
			if categoryName != nil {
				p.Category = *categoryName
			}

			// Merge images: if product has no images, use master's
			if len(p.Images) == 0 && len(mpImagesJSON) > 0 {
				if err := json.Unmarshal(mpImagesJSON, &p.Images); err != nil {
					return nil, 0, fmt.Errorf("unmarshal master images: %w", err)
				}
			}

			// Parse attributes from master
			if len(attributesJSON) > 0 {
				if err := json.Unmarshal(attributesJSON, &p.Attributes); err != nil {
					return nil, 0, fmt.Errorf("unmarshal attributes: %w", err)
				}
			}
		}

		// Format price (kopecks to rubles with formatting)
		p.PriceFormatted = formatPrice(p.Price, p.Currency)

		products = append(products, p)
	}

	return products, total, nil
}

// GetProduct retrieves a single product by ID with master data merging
func (a *CatalogAdapter) GetProduct(ctx context.Context, tenantID string, productID string) (*domain.Product, error) {
	query := `
		SELECT
			p.id, p.tenant_id, COALESCE(p.master_product_id::text, '') as master_product_id,
			COALESCE(p.name, '') as name, COALESCE(p.description, '') as description,
			p.price, p.currency, p.stock_quantity, COALESCE(p.rating, 0) as rating, COALESCE(p.images, '[]') as images,
			mp.id as mp_id, mp.sku, mp.name as mp_name, mp.description as mp_description,
			mp.brand, mp.category_id, mp.images as mp_images, mp.attributes,
			c.name as category_name
		FROM catalog.products p
		LEFT JOIN catalog.master_products mp ON p.master_product_id = mp.id
		LEFT JOIN catalog.categories c ON mp.category_id = c.id
		WHERE p.tenant_id = $1 AND p.id = $2
	`

	var p domain.Product
	var masterProductID, mpID, mpSKU, mpName, mpDesc, mpBrand, mpCategoryID, categoryName *string
	var productImagesJSON, mpImagesJSON, attributesJSON []byte

	err := a.client.pool.QueryRow(ctx, query, tenantID, productID).Scan(
		&p.ID, &p.TenantID, &masterProductID,
		&p.Name, &p.Description, &p.Price, &p.Currency, &p.StockQuantity, &p.Rating, &productImagesJSON,
		&mpID, &mpSKU, &mpName, &mpDesc,
		&mpBrand, &mpCategoryID, &mpImagesJSON, &attributesJSON,
		&categoryName,
	)

	if err != nil {
		if err.Error() == "no rows in result set" {
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

	// Merge with master product data
	if masterProductID != nil && *masterProductID != "" {
		p.MasterProductID = *masterProductID

		if p.Name == "" && mpName != nil {
			p.Name = *mpName
		}

		if p.Description == "" && mpDesc != nil {
			p.Description = *mpDesc
		}

		if mpBrand != nil {
			p.Brand = *mpBrand
		}

		if categoryName != nil {
			p.Category = *categoryName
		}

		if len(p.Images) == 0 && len(mpImagesJSON) > 0 {
			if err := json.Unmarshal(mpImagesJSON, &p.Images); err != nil {
				return nil, fmt.Errorf("unmarshal master images: %w", err)
			}
		}

		if len(attributesJSON) > 0 {
			if err := json.Unmarshal(attributesJSON, &p.Attributes); err != nil {
				return nil, fmt.Errorf("unmarshal attributes: %w", err)
			}
		}
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

	symbol := "₽"
	if currency == "USD" {
		symbol = "$"
	} else if currency == "EUR" {
		symbol = "€"
	}

	return result.String() + " " + symbol
}

// VectorSearch finds products by semantic similarity via pgvector cosine distance.
// filter may be nil for unfiltered search.
func (a *CatalogAdapter) VectorSearch(ctx context.Context, tenantID string, embedding []float32, limit int, filter *ports.VectorFilter) ([]domain.Product, error) {
	query := `
		SELECT
			p.id, p.tenant_id, COALESCE(p.master_product_id::text, '') as master_product_id,
			COALESCE(p.name, '') as name, COALESCE(p.description, '') as description,
			p.price, p.currency, p.stock_quantity, COALESCE(p.rating, 0) as rating, COALESCE(p.images, '[]') as images,
			mp.id as mp_id, mp.sku, mp.name as mp_name, mp.description as mp_description,
			mp.brand, mp.category_id, mp.images as mp_images, mp.attributes,
			c.name as category_name
		FROM catalog.products p
		JOIN catalog.master_products mp ON p.master_product_id = mp.id
		LEFT JOIN catalog.categories c ON mp.category_id = c.id
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
		var productImagesJSON, mpImagesJSON, attributesJSON []byte

		err := rows.Scan(
			&p.ID, &p.TenantID, &masterProductID,
			&p.Name, &p.Description, &p.Price, &p.Currency, &p.StockQuantity, &p.Rating, &productImagesJSON,
			&mpID, &mpSKU, &mpName, &mpDesc,
			&mpBrand, &mpCategoryID, &mpImagesJSON, &attributesJSON,
			&categoryName,
		)
		if err != nil {
			return nil, fmt.Errorf("scan vector product: %w", err)
		}

		if len(productImagesJSON) > 0 {
			if err := json.Unmarshal(productImagesJSON, &p.Images); err != nil {
				return nil, fmt.Errorf("unmarshal product images: %w", err)
			}
		}

		if masterProductID != nil && *masterProductID != "" {
			p.MasterProductID = *masterProductID
			if p.Name == "" && mpName != nil {
				p.Name = *mpName
			}
			if p.Description == "" && mpDesc != nil {
				p.Description = *mpDesc
			}
			if mpBrand != nil {
				p.Brand = *mpBrand
			}
			if categoryName != nil {
				p.Category = *categoryName
			}
			if len(p.Images) == 0 && len(mpImagesJSON) > 0 {
				if err := json.Unmarshal(mpImagesJSON, &p.Images); err != nil {
					return nil, fmt.Errorf("unmarshal master images: %w", err)
				}
			}
			if len(attributesJSON) > 0 {
				if err := json.Unmarshal(attributesJSON, &p.Attributes); err != nil {
					return nil, fmt.Errorf("unmarshal attributes: %w", err)
				}
			}
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
		       COALESCE(c.name, '') as category_name, COALESCE(mp.attributes::text, '{}') as attributes
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
		var attrsJSON string
		if err := rows.Scan(&p.ID, &p.SKU, &p.Name, &p.Description, &p.Brand, &p.CategoryID, &p.CategoryName, &attrsJSON); err != nil {
			return nil, fmt.Errorf("scan master product: %w", err)
		}
		if attrsJSON != "{}" && attrsJSON != "" {
			_ = json.Unmarshal([]byte(attrsJSON), &p.Attributes)
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
			_ = json.Unmarshal(settingsJSON, &t.Settings)
		}
		tenants = append(tenants, t)
	}

	return tenants, nil
}

// GenerateCatalogDigest computes a compact catalog meta-schema for a tenant.
func (a *CatalogAdapter) GenerateCatalogDigest(ctx context.Context, tenantID string) (*domain.CatalogDigest, error) {
	// Query 1: categories + brands + prices
	catQuery := `
		SELECT
			c.name AS category_name,
			c.slug AS category_slug,
			COUNT(DISTINCT mp.id) AS product_count,
			ARRAY_AGG(DISTINCT mp.brand) FILTER (WHERE mp.brand IS NOT NULL AND mp.brand != '') AS brands,
			MIN(p.price) AS min_price,
			MAX(p.price) AS max_price
		FROM catalog.products p
		JOIN catalog.master_products mp ON p.master_product_id = mp.id
		JOIN catalog.categories c ON mp.category_id = c.id
		WHERE p.tenant_id = $1
		GROUP BY c.name, c.slug
		ORDER BY product_count DESC
	`

	catRows, err := a.client.pool.Query(ctx, catQuery, tenantID)
	if err != nil {
		return nil, fmt.Errorf("query categories for digest: %w", err)
	}
	defer catRows.Close()

	type catInfo struct {
		name     string
		slug     string
		count    int
		brands   []string
		minPrice int
		maxPrice int
	}
	var catInfos []catInfo
	totalProducts := 0

	for catRows.Next() {
		var ci catInfo
		var brands []string
		if err := catRows.Scan(&ci.name, &ci.slug, &ci.count, &brands, &ci.minPrice, &ci.maxPrice); err != nil {
			return nil, fmt.Errorf("scan category digest: %w", err)
		}
		// Filter out empty brands
		for _, b := range brands {
			if b != "" {
				ci.brands = append(ci.brands, b)
			}
		}
		totalProducts += ci.count
		catInfos = append(catInfos, ci)
	}

	if len(catInfos) == 0 {
		return &domain.CatalogDigest{
			GeneratedAt:   time.Now(),
			TotalProducts: 0,
			Categories:    []domain.DigestCategory{},
		}, nil
	}

	// Query 2: attributes with cardinality per category
	attrQuery := `
		SELECT
			c.name AS category_name,
			attr.key AS attr_key,
			COUNT(DISTINCT attr.value) AS cardinality,
			ARRAY_AGG(DISTINCT attr.value ORDER BY attr.value) AS all_values
		FROM catalog.products p
		JOIN catalog.master_products mp ON p.master_product_id = mp.id
		JOIN catalog.categories c ON mp.category_id = c.id,
		LATERAL jsonb_each_text(mp.attributes) AS attr(key, value)
		WHERE p.tenant_id = $1
		  AND attr.value IS NOT NULL AND attr.value != ''
		GROUP BY c.name, attr.key
		ORDER BY c.name, cardinality DESC
	`

	attrRows, err := a.client.pool.Query(ctx, attrQuery, tenantID)
	if err != nil {
		return nil, fmt.Errorf("query attributes for digest: %w", err)
	}
	defer attrRows.Close()

	// Build category → attrs map
	type attrInfo struct {
		key         string
		cardinality int
		values      []string
	}
	catAttrs := make(map[string][]attrInfo)

	for attrRows.Next() {
		var catName, attrKey string
		var cardinality int
		var values []string
		if err := attrRows.Scan(&catName, &attrKey, &cardinality, &values); err != nil {
			return nil, fmt.Errorf("scan attribute digest: %w", err)
		}
		catAttrs[catName] = append(catAttrs[catName], attrInfo{
			key:         attrKey,
			cardinality: cardinality,
			values:      values,
		})
	}

	// Build digest categories
	categories := make([]domain.DigestCategory, 0, len(catInfos))
	for _, ci := range catInfos {
		dc := domain.DigestCategory{
			Name:       ci.name,
			Slug:       ci.slug,
			Count:      ci.count,
			PriceRange: [2]int{ci.minPrice, ci.maxPrice},
		}

		// Brand as first param
		if len(ci.brands) > 0 {
			dc.Params = append(dc.Params, buildDigestParam("brand", ci.brands))
		}

		// Attributes
		if attrs, ok := catAttrs[ci.name]; ok {
			for _, ai := range attrs {
				dc.Params = append(dc.Params, buildDigestParam(ai.key, ai.values))
			}
		}

		categories = append(categories, dc)
	}

	return &domain.CatalogDigest{
		GeneratedAt:   time.Now(),
		TotalProducts: totalProducts,
		Categories:    categories,
	}, nil
}

// buildDigestParam creates a DigestParam applying cardinality rules.
func buildDigestParam(key string, values []string) domain.DigestParam {
	cardinality := len(values)

	// Check if values are numeric (for range params like size)
	if isNumericValues(values) && cardinality > 1 {
		minVal, maxVal := numericRange(values)
		return domain.DigestParam{
			Key:         key,
			Type:        "range",
			Cardinality: cardinality,
			Range:       fmt.Sprintf("%s-%s", minVal, maxVal),
		}
	}

	p := domain.DigestParam{
		Key:         key,
		Type:        "enum",
		Cardinality: cardinality,
	}

	switch {
	case cardinality <= 15:
		p.Values = values
	case cardinality <= 50:
		top := 5
		if top > cardinality {
			top = cardinality
		}
		p.Top = values[:top]
		p.More = cardinality - top
	default:
		// 50+ values → families
		p.Families = domain.ComputeFamilies(key, values)
		if len(p.Families) == 0 {
			// Fallback: top 10 values
			top := 10
			if top > cardinality {
				top = cardinality
			}
			p.Top = values[:top]
			p.More = cardinality - top
		}
	}

	return p
}

// stripNumericSuffix removes common unit suffixes for numeric parsing.
func stripNumericSuffix(s string) string {
	clean := strings.TrimSpace(s)
	// Remove common unit suffixes
	for _, suffix := range []string{"GB", "MB", "TB", "mm", "inch", "\"", " inch"} {
		clean = strings.TrimSuffix(clean, suffix)
	}
	return strings.TrimSpace(clean)
}

// isNumericValues checks if all values are purely numeric (possibly with unit suffixes).
func isNumericValues(values []string) bool {
	if len(values) == 0 {
		return false
	}
	for _, v := range values {
		clean := stripNumericSuffix(v)
		if _, err := strconv.ParseFloat(clean, 64); err != nil {
			return false
		}
	}
	return true
}

// numericRange returns min and max original string representations from numeric values.
func numericRange(values []string) (string, string) {
	if len(values) == 0 {
		return "0", "0"
	}
	// Sort numerically using stripped values
	sorted := make([]string, len(values))
	copy(sorted, values)
	sort.Slice(sorted, func(i, j int) bool {
		ni, _ := strconv.ParseFloat(stripNumericSuffix(sorted[i]), 64)
		nj, _ := strconv.ParseFloat(stripNumericSuffix(sorted[j]), 64)
		return ni < nj
	})
	return sorted[0], sorted[len(sorted)-1]
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
	query := `SELECT catalog_digest FROM catalog.tenants WHERE id = $1`

	var digestJSON []byte
	err := a.client.pool.QueryRow(ctx, query, tenantID).Scan(&digestJSON)
	if err != nil {
		if err.Error() == "no rows in result set" {
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
