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
		p.price, p.currency, COALESCE(st.quantity, p.stock_quantity) as stock_quantity, p.rating, p.images, COALESCE(p.tags, '[]') as tags,
		p.created_at, p.updated_at,
		mp.id, mp.name, mp.description, mp.brand, mp.sku, mp.images, mp.attributes,
		c.name
		FROM catalog.products p
		LEFT JOIN catalog.master_products mp ON p.master_product_id = mp.id
		LEFT JOIN catalog.categories c ON mp.category_id = c.id
		LEFT JOIN catalog.stock st ON st.product_id = p.id AND st.tenant_id = p.tenant_id
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
		var pImagesJSON, tagsJSON, mpImagesJSON, mpAttrsJSON []byte
		var mpID, mpName, mpDesc, mpBrand, mpSKU *string
		var catName *string

		if err := rows.Scan(
			&p.ID, &p.TenantID, &p.MasterProductID, &p.Name, &p.Description,
			&p.Price, &p.Currency, &p.StockQuantity, &p.Rating, &pImagesJSON, &tagsJSON,
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
		if len(tagsJSON) > 0 {
			json.Unmarshal(tagsJSON, &p.Tags)
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
		p.price, p.currency, COALESCE(st.quantity, p.stock_quantity) as stock_quantity, p.rating, p.images, COALESCE(p.tags, '[]') as tags,
		p.created_at, p.updated_at,
		mp.name, mp.description, mp.brand, mp.sku, mp.images, mp.attributes,
		c.name
		FROM catalog.products p
		LEFT JOIN catalog.master_products mp ON p.master_product_id = mp.id
		LEFT JOIN catalog.categories c ON mp.category_id = c.id
		LEFT JOIN catalog.stock st ON st.product_id = p.id AND st.tenant_id = p.tenant_id
		WHERE p.id = $1 AND p.tenant_id = $2`

	var p domain.Product
	var pImagesJSON, tagsJSON, mpImagesJSON, mpAttrsJSON []byte
	var mpName, mpDesc, mpBrand, mpSKU *string
	var catName *string

	err := a.client.pool.QueryRow(ctx, query, productID, tenantID).Scan(
		&p.ID, &p.TenantID, &p.MasterProductID, &p.Name, &p.Description,
		&p.Price, &p.Currency, &p.StockQuantity, &p.Rating, &pImagesJSON, &tagsJSON,
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
	if len(tagsJSON) > 0 {
		json.Unmarshal(tagsJSON, &p.Tags)
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

	// Also update stock table if stock was changed
	if update.Stock != nil {
		stockQuery := `INSERT INTO catalog.stock (tenant_id, product_id, quantity, updated_at)
			VALUES ($1, $2, $3, NOW())
			ON CONFLICT (tenant_id, product_id) DO UPDATE SET
				quantity = EXCLUDED.quantity, updated_at = NOW()`
		_, _ = a.client.pool.Exec(ctx, stockQuery, tenantID, productID, *update.Stock)
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
	tagsJSON, _ := json.Marshal(p.Tags)

	query := `INSERT INTO catalog.products (tenant_id, master_product_id, name, description, price, currency, stock_quantity, rating, images, tags)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (tenant_id, master_product_id) DO UPDATE SET
			price = EXCLUDED.price,
			stock_quantity = EXCLUDED.stock_quantity,
			rating = EXCLUDED.rating,
			images = EXCLUDED.images,
			tags = EXCLUDED.tags,
			updated_at = NOW()
		RETURNING id`

	var id string
	err := a.client.pool.QueryRow(ctx, query,
		p.TenantID, p.MasterProductID, p.Name, p.Description,
		p.Price, p.Currency, p.StockQuantity, p.Rating, imagesJSON, tagsJSON,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("upsert product listing: %w", err)
	}

	// Also upsert into stock table
	if p.StockQuantity > 0 {
		stockQuery := `INSERT INTO catalog.stock (tenant_id, product_id, quantity, updated_at)
			VALUES ($1, $2, $3, NOW())
			ON CONFLICT (tenant_id, product_id) DO UPDATE SET
				quantity = EXCLUDED.quantity, updated_at = NOW()`
		_, _ = a.client.pool.Exec(ctx, stockQuery, p.TenantID, id, p.StockQuantity)
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

// --- Services ---

func (a *CatalogAdapter) ListServices(ctx context.Context, tenantID string, filter domain.AdminProductFilter) ([]domain.Service, int, error) {
	if filter.Limit <= 0 {
		filter.Limit = 25
	}

	where := []string{"sv.tenant_id = $1"}
	args := []any{tenantID}
	argIdx := 2

	if filter.Search != "" {
		where = append(where, fmt.Sprintf(
			"(ms.name ILIKE $%d OR ms.sku ILIKE $%d OR ms.brand ILIKE $%d)",
			argIdx, argIdx, argIdx))
		args = append(args, "%"+filter.Search+"%")
		argIdx++
	}
	if filter.CategoryID != "" {
		where = append(where, fmt.Sprintf("ms.category_id = $%d", argIdx))
		args = append(args, filter.CategoryID)
		argIdx++
	}

	whereClause := strings.Join(where, " AND ")

	countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM catalog.services sv
		LEFT JOIN catalog.master_services ms ON sv.master_service_id = ms.id
		WHERE %s`, whereClause)
	var total int
	if err := a.client.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count services: %w", err)
	}

	query := fmt.Sprintf(`SELECT
		sv.id, sv.tenant_id, sv.master_service_id, sv.name, sv.description,
		sv.price, sv.currency, sv.rating, sv.images, COALESCE(sv.tags, '[]') as tags,
		sv.availability, sv.created_at, sv.updated_at,
		ms.name, ms.description, ms.brand, ms.sku, ms.images, ms.attributes,
		ms.duration, ms.provider,
		c.name
		FROM catalog.services sv
		LEFT JOIN catalog.master_services ms ON sv.master_service_id = ms.id
		LEFT JOIN catalog.categories c ON ms.category_id = c.id
		WHERE %s
		ORDER BY sv.created_at DESC
		LIMIT $%d OFFSET $%d`, whereClause, argIdx, argIdx+1)
	args = append(args, filter.Limit, filter.Offset)

	rows, err := a.client.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list services: %w", err)
	}
	defer rows.Close()

	var services []domain.Service
	for rows.Next() {
		var s domain.Service
		var sImagesJSON, tagsJSON, msImagesJSON, msAttrsJSON []byte
		var msName, msDesc, msBrand, msSKU, msDuration, msProvider *string
		var catName *string

		if err := rows.Scan(
			&s.ID, &s.TenantID, &s.MasterServiceID, &s.Name, &s.Description,
			&s.Price, &s.Currency, &s.Rating, &sImagesJSON, &tagsJSON,
			&s.Availability, &s.CreatedAt, &s.UpdatedAt,
			&msName, &msDesc, &msBrand, &msSKU, &msImagesJSON, &msAttrsJSON,
			&msDuration, &msProvider,
			&catName,
		); err != nil {
			return nil, 0, fmt.Errorf("scan service: %w", err)
		}

		if msName != nil && s.Name == "" {
			s.Name = *msName
		}
		if msDesc != nil && s.Description == "" {
			s.Description = *msDesc
		}
		if msDuration != nil {
			s.Duration = *msDuration
		}
		if msProvider != nil {
			s.Provider = *msProvider
		}
		if catName != nil {
			s.Category = *catName
		}
		if len(sImagesJSON) > 0 && s.Images == nil {
			json.Unmarshal(sImagesJSON, &s.Images)
		}
		if len(s.Images) == 0 && len(msImagesJSON) > 0 {
			json.Unmarshal(msImagesJSON, &s.Images)
		}
		if len(tagsJSON) > 0 {
			json.Unmarshal(tagsJSON, &s.Tags)
		}
		if len(msAttrsJSON) > 0 {
			json.Unmarshal(msAttrsJSON, &s.Attributes)
		}

		s.PriceFormatted = formatPrice(s.Price, s.Currency)
		services = append(services, s)
	}

	return services, total, nil
}

func (a *CatalogAdapter) GetService(ctx context.Context, tenantID string, serviceID string) (*domain.Service, error) {
	query := `SELECT
		sv.id, sv.tenant_id, sv.master_service_id, sv.name, sv.description,
		sv.price, sv.currency, sv.rating, sv.images, COALESCE(sv.tags, '[]') as tags,
		sv.availability, sv.created_at, sv.updated_at,
		ms.name, ms.description, ms.brand, ms.sku, ms.images, ms.attributes,
		ms.duration, ms.provider,
		c.name
		FROM catalog.services sv
		LEFT JOIN catalog.master_services ms ON sv.master_service_id = ms.id
		LEFT JOIN catalog.categories c ON ms.category_id = c.id
		WHERE sv.id = $1 AND sv.tenant_id = $2`

	var s domain.Service
	var sImagesJSON, tagsJSON, msImagesJSON, msAttrsJSON []byte
	var msName, msDesc, msBrand, msSKU, msDuration, msProvider *string
	var catName *string

	err := a.client.pool.QueryRow(ctx, query, serviceID, tenantID).Scan(
		&s.ID, &s.TenantID, &s.MasterServiceID, &s.Name, &s.Description,
		&s.Price, &s.Currency, &s.Rating, &sImagesJSON, &tagsJSON,
		&s.Availability, &s.CreatedAt, &s.UpdatedAt,
		&msName, &msDesc, &msBrand, &msSKU, &msImagesJSON, &msAttrsJSON,
		&msDuration, &msProvider,
		&catName,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrProductNotFound
		}
		return nil, fmt.Errorf("get service: %w", err)
	}

	if msName != nil && s.Name == "" {
		s.Name = *msName
	}
	if msDesc != nil && s.Description == "" {
		s.Description = *msDesc
	}
	if msDuration != nil {
		s.Duration = *msDuration
	}
	if msProvider != nil {
		s.Provider = *msProvider
	}
	if catName != nil {
		s.Category = *catName
	}
	if len(sImagesJSON) > 0 {
		json.Unmarshal(sImagesJSON, &s.Images)
	}
	if len(s.Images) == 0 && len(msImagesJSON) > 0 {
		json.Unmarshal(msImagesJSON, &s.Images)
	}
	if len(tagsJSON) > 0 {
		json.Unmarshal(tagsJSON, &s.Tags)
	}
	if len(msAttrsJSON) > 0 {
		json.Unmarshal(msAttrsJSON, &s.Attributes)
	}

	s.PriceFormatted = formatPrice(s.Price, s.Currency)
	return &s, nil
}

func (a *CatalogAdapter) UpdateService(ctx context.Context, tenantID string, serviceID string, update domain.ProductUpdate) error {
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
	if update.Rating != nil {
		sets = append(sets, fmt.Sprintf("rating = $%d", argIdx))
		args = append(args, *update.Rating)
		argIdx++
	}

	if len(sets) == 0 {
		return nil
	}

	sets = append(sets, "updated_at = NOW()")
	query := fmt.Sprintf("UPDATE catalog.services SET %s WHERE id = $%d AND tenant_id = $%d",
		strings.Join(sets, ", "), argIdx, argIdx+1)
	args = append(args, serviceID, tenantID)

	tag, err := a.client.pool.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("update service: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrProductNotFound
	}
	return nil
}

// --- Import upserts for services ---

func (a *CatalogAdapter) UpsertMasterService(ctx context.Context, ms *domain.MasterService) (string, error) {
	imagesJSON, _ := json.Marshal(ms.Images)
	attrsJSON, _ := json.Marshal(ms.Attributes)

	query := `INSERT INTO catalog.master_services (sku, name, description, brand, category_id, images, attributes, duration, provider, owner_tenant_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (sku) DO UPDATE SET
			name = EXCLUDED.name,
			brand = EXCLUDED.brand,
			images = EXCLUDED.images,
			attributes = EXCLUDED.attributes,
			duration = EXCLUDED.duration,
			provider = EXCLUDED.provider,
			updated_at = NOW()
		RETURNING id`

	var id string
	err := a.client.pool.QueryRow(ctx, query,
		ms.SKU, ms.Name, ms.Description, ms.Brand, ms.CategoryID,
		imagesJSON, attrsJSON, ms.Duration, ms.Provider, ms.OwnerTenantID,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("upsert master service: %w", err)
	}
	return id, nil
}

func (a *CatalogAdapter) UpsertServiceListing(ctx context.Context, s *domain.Service) (string, error) {
	imagesJSON, _ := json.Marshal(s.Images)
	tagsJSON, _ := json.Marshal(s.Tags)

	query := `INSERT INTO catalog.services (tenant_id, master_service_id, name, description, price, currency, rating, images, tags, availability)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (tenant_id, master_service_id) DO UPDATE SET
			price = EXCLUDED.price,
			rating = EXCLUDED.rating,
			images = EXCLUDED.images,
			tags = EXCLUDED.tags,
			availability = EXCLUDED.availability,
			updated_at = NOW()
		RETURNING id`

	var id string
	err := a.client.pool.QueryRow(ctx, query,
		s.TenantID, s.MasterServiceID, s.Name, s.Description,
		s.Price, s.Currency, s.Rating, imagesJSON, tagsJSON, s.Availability,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("upsert service listing: %w", err)
	}
	return id, nil
}

// --- Stock ---

func (a *CatalogAdapter) BulkUpdateStock(ctx context.Context, tenantID string, items []domain.StockUpdate) (int, error) {
	updated := 0
	for _, item := range items {
		// Upsert stock row: resolve SKU → product_id via master_products
		query := `
			INSERT INTO catalog.stock (tenant_id, product_id, quantity, updated_at)
			SELECT $1, p.id, $3, NOW()
			FROM catalog.products p
			JOIN catalog.master_products mp ON p.master_product_id = mp.id
			WHERE mp.sku = $2 AND p.tenant_id = $1
			ON CONFLICT (tenant_id, product_id) DO UPDATE SET
				quantity = EXCLUDED.quantity, updated_at = NOW()`

		tag, err := a.client.pool.Exec(ctx, query, tenantID, item.SKU, item.Quantity)
		if err != nil {
			return updated, fmt.Errorf("upsert stock for sku=%s: %w", item.SKU, err)
		}
		if tag.RowsAffected() > 0 {
			updated++
		}

		// Optional: update price
		if item.Price != nil {
			priceQuery := `
				UPDATE catalog.products p SET price = $3, updated_at = NOW()
				FROM catalog.master_products mp
				WHERE p.master_product_id = mp.id AND mp.sku = $2 AND p.tenant_id = $1`
			a.client.pool.Exec(ctx, priceQuery, tenantID, item.SKU, *item.Price)
		}
	}
	return updated, nil
}

// --- Post-import for services ---

func (a *CatalogAdapter) GetMasterServicesWithoutEmbedding(ctx context.Context, tenantID string) ([]domain.MasterService, error) {
	query := `SELECT ms.id, ms.sku, ms.name, ms.description, ms.brand, ms.category_id,
		ms.images, ms.attributes, ms.owner_tenant_id, ms.duration, ms.provider, c.name
		FROM catalog.master_services ms
		LEFT JOIN catalog.categories c ON ms.category_id = c.id
		WHERE ms.embedding IS NULL AND ms.owner_tenant_id = $1
		ORDER BY ms.created_at`

	rows, err := a.client.pool.Query(ctx, query, tenantID)
	if err != nil {
		return nil, fmt.Errorf("get services without embedding: %w", err)
	}
	defer rows.Close()

	var services []domain.MasterService
	for rows.Next() {
		var ms domain.MasterService
		var imagesJSON, attrsJSON []byte
		var catName *string
		if err := rows.Scan(
			&ms.ID, &ms.SKU, &ms.Name, &ms.Description, &ms.Brand, &ms.CategoryID,
			&imagesJSON, &attrsJSON, &ms.OwnerTenantID, &ms.Duration, &ms.Provider, &catName,
		); err != nil {
			return nil, fmt.Errorf("scan master service: %w", err)
		}
		if len(imagesJSON) > 0 {
			json.Unmarshal(imagesJSON, &ms.Images)
		}
		if len(attrsJSON) > 0 {
			json.Unmarshal(attrsJSON, &ms.Attributes)
		}
		if catName != nil {
			ms.CategoryName = *catName
		}
		services = append(services, ms)
	}
	return services, nil
}

func (a *CatalogAdapter) SeedServiceEmbedding(ctx context.Context, masterServiceID string, embedding []float32) error {
	query := `UPDATE catalog.master_services SET embedding = $1 WHERE id = $2`
	_, err := a.client.pool.Exec(ctx, query, pgvector.NewVector(embedding), masterServiceID)
	if err != nil {
		return fmt.Errorf("seed service embedding: %w", err)
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
