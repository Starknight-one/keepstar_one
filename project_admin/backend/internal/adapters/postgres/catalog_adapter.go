package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	pgvector "github.com/pgvector/pgvector-go"
	"keepstar-admin/internal/domain"
)

type CatalogAdapter struct {
	client *Client
}

func NewCatalogAdapter(client *Client) *CatalogAdapter {
	return &CatalogAdapter{client: client}
}

// --- Tenant ---

func (a *CatalogAdapter) GetTenantByID(ctx context.Context, id string) (*domain.Tenant, error) {
	query := `SELECT id, slug, name, type, settings, created_at, updated_at
		FROM catalog.tenants WHERE id = $1`

	var t domain.Tenant
	var settingsJSON []byte
	err := a.client.pool.QueryRow(ctx, query, id).Scan(
		&t.ID, &t.Slug, &t.Name, &t.Type, &settingsJSON, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrTenantNotFound
		}
		return nil, fmt.Errorf("query tenant: %w", err)
	}
	if len(settingsJSON) > 0 {
		json.Unmarshal(settingsJSON, &t.Settings)
	}
	return &t, nil
}

func (a *CatalogAdapter) CreateTenant(ctx context.Context, tenant *domain.Tenant) (*domain.Tenant, error) {
	settingsJSON, _ := json.Marshal(tenant.Settings)
	query := `INSERT INTO catalog.tenants (slug, name, type, settings)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at, updated_at`

	err := a.client.pool.QueryRow(ctx, query,
		tenant.Slug, tenant.Name, tenant.Type, settingsJSON,
	).Scan(&tenant.ID, &tenant.CreatedAt, &tenant.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("create tenant: %w", err)
	}
	return tenant, nil
}

func (a *CatalogAdapter) UpdateTenantSettings(ctx context.Context, tenantID string, settings domain.TenantSettings) error {
	settingsJSON, err := json.Marshal(settings)
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}
	query := `UPDATE catalog.tenants SET settings = $1, updated_at = NOW() WHERE id = $2`
	tag, err := a.client.pool.Exec(ctx, query, settingsJSON, tenantID)
	if err != nil {
		return fmt.Errorf("update tenant settings: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrTenantNotFound
	}
	return nil
}

// --- Products ---

func (a *CatalogAdapter) ListProducts(ctx context.Context, tenantID string, filter domain.AdminProductFilter) ([]domain.Product, int, error) {
	if filter.Limit <= 0 {
		filter.Limit = 25
	}

	where := []string{"p.tenant_id = $1"}
	args := []any{tenantID}
	argIdx := 2

	if filter.Search != "" {
		where = append(where, fmt.Sprintf(
			"(mp.name ILIKE $%d OR mp.sku ILIKE $%d OR mp.brand ILIKE $%d)",
			argIdx, argIdx, argIdx))
		args = append(args, "%"+filter.Search+"%")
		argIdx++
	}
	if filter.CategoryID != "" {
		where = append(where, fmt.Sprintf("mp.category_id = $%d", argIdx))
		args = append(args, filter.CategoryID)
		argIdx++
	}

	whereClause := strings.Join(where, " AND ")

	// Count
	countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM catalog.products p
		LEFT JOIN catalog.master_products mp ON p.master_product_id = mp.id
		WHERE %s`, whereClause)
	var total int
	if err := a.client.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count products: %w", err)
	}

	// Fetch
	query := fmt.Sprintf(`SELECT
		p.id, p.tenant_id, p.master_product_id, p.name, p.description,
		p.price, p.currency, p.stock_quantity, p.rating, p.images,
		p.created_at, p.updated_at,
		mp.id, mp.name, mp.description, mp.brand, mp.sku, mp.images, mp.attributes,
		c.name
		FROM catalog.products p
		LEFT JOIN catalog.master_products mp ON p.master_product_id = mp.id
		LEFT JOIN catalog.categories c ON mp.category_id = c.id
		WHERE %s
		ORDER BY p.created_at DESC
		LIMIT $%d OFFSET $%d`, whereClause, argIdx, argIdx+1)
	args = append(args, filter.Limit, filter.Offset)

	rows, err := a.client.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list products: %w", err)
	}
	defer rows.Close()

	var products []domain.Product
	for rows.Next() {
		var p domain.Product
		var pImagesJSON, mpImagesJSON, mpAttrsJSON []byte
		var mpID, mpName, mpDesc, mpBrand, mpSKU *string
		var catName *string

		if err := rows.Scan(
			&p.ID, &p.TenantID, &p.MasterProductID, &p.Name, &p.Description,
			&p.Price, &p.Currency, &p.StockQuantity, &p.Rating, &pImagesJSON,
			&p.CreatedAt, &p.UpdatedAt,
			&mpID, &mpName, &mpDesc, &mpBrand, &mpSKU, &mpImagesJSON, &mpAttrsJSON,
			&catName,
		); err != nil {
			return nil, 0, fmt.Errorf("scan product: %w", err)
		}

		// Merge master product data
		if mpName != nil && p.Name == "" {
			p.Name = *mpName
		}
		if mpDesc != nil && p.Description == "" {
			p.Description = *mpDesc
		}
		if mpBrand != nil {
			p.Brand = *mpBrand
		}
		if catName != nil {
			p.Category = *catName
		}
		if len(p.Images) == 0 && len(mpImagesJSON) > 0 {
			json.Unmarshal(mpImagesJSON, &p.Images)
		}
		if len(pImagesJSON) > 0 && p.Images == nil {
			json.Unmarshal(pImagesJSON, &p.Images)
		}
		if len(mpAttrsJSON) > 0 {
			json.Unmarshal(mpAttrsJSON, &p.Attributes)
		}

		p.PriceFormatted = formatPrice(p.Price, p.Currency)
		products = append(products, p)
	}

	return products, total, nil
}

func (a *CatalogAdapter) GetProduct(ctx context.Context, tenantID string, productID string) (*domain.Product, error) {
	query := `SELECT
		p.id, p.tenant_id, p.master_product_id, p.name, p.description,
		p.price, p.currency, p.stock_quantity, p.rating, p.images,
		p.created_at, p.updated_at,
		mp.name, mp.description, mp.brand, mp.sku, mp.images, mp.attributes,
		c.name
		FROM catalog.products p
		LEFT JOIN catalog.master_products mp ON p.master_product_id = mp.id
		LEFT JOIN catalog.categories c ON mp.category_id = c.id
		WHERE p.id = $1 AND p.tenant_id = $2`

	var p domain.Product
	var pImagesJSON, mpImagesJSON, mpAttrsJSON []byte
	var mpName, mpDesc, mpBrand, mpSKU *string
	var catName *string

	err := a.client.pool.QueryRow(ctx, query, productID, tenantID).Scan(
		&p.ID, &p.TenantID, &p.MasterProductID, &p.Name, &p.Description,
		&p.Price, &p.Currency, &p.StockQuantity, &p.Rating, &pImagesJSON,
		&p.CreatedAt, &p.UpdatedAt,
		&mpName, &mpDesc, &mpBrand, &mpSKU, &mpImagesJSON, &mpAttrsJSON,
		&catName,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrProductNotFound
		}
		return nil, fmt.Errorf("get product: %w", err)
	}

	if mpName != nil && p.Name == "" {
		p.Name = *mpName
	}
	if mpDesc != nil && p.Description == "" {
		p.Description = *mpDesc
	}
	if mpBrand != nil {
		p.Brand = *mpBrand
	}
	if catName != nil {
		p.Category = *catName
	}
	if len(pImagesJSON) > 0 {
		json.Unmarshal(pImagesJSON, &p.Images)
	}
	if len(p.Images) == 0 && len(mpImagesJSON) > 0 {
		json.Unmarshal(mpImagesJSON, &p.Images)
	}
	if len(mpAttrsJSON) > 0 {
		json.Unmarshal(mpAttrsJSON, &p.Attributes)
	}

	p.PriceFormatted = formatPrice(p.Price, p.Currency)
	return &p, nil
}

func (a *CatalogAdapter) UpdateProduct(ctx context.Context, tenantID string, productID string, update domain.ProductUpdate) error {
	sets := []string{}
	args := []any{}
	argIdx := 1

	if update.Name != nil {
		sets = append(sets, fmt.Sprintf("name = $%d", argIdx))
		args = append(args, *update.Name)
		argIdx++
	}
	if update.Description != nil {
		sets = append(sets, fmt.Sprintf("description = $%d", argIdx))
		args = append(args, *update.Description)
		argIdx++
	}
	if update.Price != nil {
		sets = append(sets, fmt.Sprintf("price = $%d", argIdx))
		args = append(args, *update.Price)
		argIdx++
	}
	if update.Stock != nil {
		sets = append(sets, fmt.Sprintf("stock_quantity = $%d", argIdx))
		args = append(args, *update.Stock)
		argIdx++
	}
	if update.Rating != nil {
		sets = append(sets, fmt.Sprintf("rating = $%d", argIdx))
		args = append(args, *update.Rating)
		argIdx++
	}

	if len(sets) == 0 {
		return nil
	}

	sets = append(sets, "updated_at = NOW()")
	query := fmt.Sprintf("UPDATE catalog.products SET %s WHERE id = $%d AND tenant_id = $%d",
		strings.Join(sets, ", "), argIdx, argIdx+1)
	args = append(args, productID, tenantID)

	tag, err := a.client.pool.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("update product: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrProductNotFound
	}
	return nil
}

// --- Categories ---

func (a *CatalogAdapter) GetCategories(ctx context.Context) ([]domain.Category, error) {
	query := `SELECT id, name, slug, COALESCE(parent_id::text, '') FROM catalog.categories ORDER BY name`
	rows, err := a.client.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list categories: %w", err)
	}
	defer rows.Close()

	var cats []domain.Category
	for rows.Next() {
		var c domain.Category
		if err := rows.Scan(&c.ID, &c.Name, &c.Slug, &c.ParentID); err != nil {
			return nil, fmt.Errorf("scan category: %w", err)
		}
		cats = append(cats, c)
	}
	return cats, nil
}

func (a *CatalogAdapter) GetOrCreateCategory(ctx context.Context, name string, slug string) (string, error) {
	// Try to get first
	var id string
	err := a.client.pool.QueryRow(ctx,
		`SELECT id FROM catalog.categories WHERE slug = $1`, slug).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("query category: %w", err)
	}

	// Create
	err = a.client.pool.QueryRow(ctx,
		`INSERT INTO catalog.categories (name, slug) VALUES ($1, $2)
		ON CONFLICT (slug) DO UPDATE SET name = EXCLUDED.name
		RETURNING id`, name, slug).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("create category: %w", err)
	}
	return id, nil
}

// --- Import upserts ---

func (a *CatalogAdapter) UpsertMasterProduct(ctx context.Context, mp *domain.MasterProduct) (string, error) {
	imagesJSON, _ := json.Marshal(mp.Images)
	attrsJSON, _ := json.Marshal(mp.Attributes)

	query := `INSERT INTO catalog.master_products (sku, name, description, brand, category_id, images, attributes, owner_tenant_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (sku) DO UPDATE SET
			name = EXCLUDED.name,
			brand = EXCLUDED.brand,
			images = EXCLUDED.images,
			attributes = EXCLUDED.attributes,
			updated_at = NOW()
		RETURNING id`

	var id string
	err := a.client.pool.QueryRow(ctx, query,
		mp.SKU, mp.Name, mp.Description, mp.Brand, mp.CategoryID,
		imagesJSON, attrsJSON, mp.OwnerTenantID,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("upsert master product: %w", err)
	}
	return id, nil
}

func (a *CatalogAdapter) UpsertProductListing(ctx context.Context, p *domain.Product) (string, error) {
	imagesJSON, _ := json.Marshal(p.Images)

	query := `INSERT INTO catalog.products (tenant_id, master_product_id, name, description, price, currency, stock_quantity, rating, images)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (tenant_id, master_product_id) DO UPDATE SET
			price = EXCLUDED.price,
			stock_quantity = EXCLUDED.stock_quantity,
			rating = EXCLUDED.rating,
			images = EXCLUDED.images,
			updated_at = NOW()
		RETURNING id`

	var id string
	err := a.client.pool.QueryRow(ctx, query,
		p.TenantID, p.MasterProductID, p.Name, p.Description,
		p.Price, p.Currency, p.StockQuantity, p.Rating, imagesJSON,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("upsert product listing: %w", err)
	}
	return id, nil
}

// --- Post-import ---

func (a *CatalogAdapter) GetMasterProductsWithoutEmbedding(ctx context.Context, tenantID string) ([]domain.MasterProduct, error) {
	query := `SELECT mp.id, mp.sku, mp.name, mp.description, mp.brand, mp.category_id,
		mp.images, mp.attributes, mp.owner_tenant_id, c.name
		FROM catalog.master_products mp
		LEFT JOIN catalog.categories c ON mp.category_id = c.id
		WHERE mp.embedding IS NULL AND mp.owner_tenant_id = $1
		ORDER BY mp.created_at`

	rows, err := a.client.pool.Query(ctx, query, tenantID)
	if err != nil {
		return nil, fmt.Errorf("get products without embedding: %w", err)
	}
	defer rows.Close()

	var products []domain.MasterProduct
	for rows.Next() {
		var mp domain.MasterProduct
		var imagesJSON, attrsJSON []byte
		var catName *string
		if err := rows.Scan(
			&mp.ID, &mp.SKU, &mp.Name, &mp.Description, &mp.Brand, &mp.CategoryID,
			&imagesJSON, &attrsJSON, &mp.OwnerTenantID, &catName,
		); err != nil {
			return nil, fmt.Errorf("scan master product: %w", err)
		}
		if len(imagesJSON) > 0 {
			json.Unmarshal(imagesJSON, &mp.Images)
		}
		if len(attrsJSON) > 0 {
			json.Unmarshal(attrsJSON, &mp.Attributes)
		}
		if catName != nil {
			mp.CategoryName = *catName
		}
		products = append(products, mp)
	}
	return products, nil
}

func (a *CatalogAdapter) SeedEmbedding(ctx context.Context, masterProductID string, embedding []float32) error {
	query := `UPDATE catalog.master_products SET embedding = $1 WHERE id = $2`
	_, err := a.client.pool.Exec(ctx, query, pgvector.NewVector(embedding), masterProductID)
	if err != nil {
		return fmt.Errorf("seed embedding: %w", err)
	}
	return nil
}

func (a *CatalogAdapter) GenerateCatalogDigest(ctx context.Context, tenantID string) error {
	// Build a compact digest of the tenant's catalog
	query := `
		WITH tenant_products AS (
			SELECT mp.id, mp.name, mp.brand, mp.attributes, c.name AS category_name, p.price, p.currency
			FROM catalog.products p
			JOIN catalog.master_products mp ON p.master_product_id = mp.id
			LEFT JOIN catalog.categories c ON mp.category_id = c.id
			WHERE p.tenant_id = $1
		)
		SELECT json_build_object(
			'totalProducts', (SELECT COUNT(*) FROM tenant_products),
			'categories', COALESCE((
				SELECT json_agg(DISTINCT category_name)
				FROM tenant_products WHERE category_name IS NOT NULL
			), '[]'::json),
			'brands', COALESCE((
				SELECT json_agg(DISTINCT brand)
				FROM tenant_products WHERE brand IS NOT NULL AND brand != ''
			), '[]'::json)
		)`

	var digestJSON []byte
	if err := a.client.pool.QueryRow(ctx, query, tenantID).Scan(&digestJSON); err != nil {
		return fmt.Errorf("generate digest: %w", err)
	}

	_, err := a.client.pool.Exec(ctx,
		`UPDATE catalog.tenants SET catalog_digest = $1, updated_at = NOW() WHERE id = $2`,
		digestJSON, tenantID)
	if err != nil {
		return fmt.Errorf("save digest: %w", err)
	}
	return nil
}

// --- helpers ---

func formatPrice(kopecks int, currency string) string {
	rubles := kopecks / 100
	str := fmt.Sprintf("%d", rubles)
	var result strings.Builder
	for i, c := range str {
		if i > 0 && (len(str)-i)%3 == 0 {
			result.WriteString(" ")
		}
		result.WriteRune(c)
	}
	symbol := "₽"
	switch currency {
	case "USD":
		symbol = "$"
	case "EUR":
		symbol = "€"
	}
	return result.String() + " " + symbol
}
