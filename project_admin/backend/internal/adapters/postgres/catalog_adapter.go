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
	"keepstar-admin/internal/logger"
)

type CatalogAdapter struct {
	client *Client
	log    *logger.Logger
}

func NewCatalogAdapter(client *Client, log *logger.Logger) *CatalogAdapter {
	return &CatalogAdapter{client: client, log: log}
}

// --- Tenant ---

func (a *CatalogAdapter) GetTenantByID(ctx context.Context, id string) (*domain.Tenant, error) {
	query := `SELECT id, slug, name, type, settings, created_at, updated_at
		FROM catalog.tenants WHERE id = $1 AND deleted_at IS NULL`

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
	if sc := domain.SpanFromContext(ctx); sc != nil {
		endSpan := sc.Start("db.admin.update_tenant_settings")
		defer endSpan()
	}
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
	if sc := domain.SpanFromContext(ctx); sc != nil {
		endSpan := sc.Start("db.admin.list_products")
		defer endSpan()
	}
	if filter.Limit <= 0 {
		filter.Limit = 25
	}

	where := []string{"p.tenant_id = $1", "p.deleted_at IS NULL"}
	args := []any{tenantID}
	argIdx := 2

	if filter.Search != "" {
		where = append(where, fmt.Sprintf(
			"(mp.name ILIKE $%d OR mp.sku ILIKE $%d OR mv.sku ILIKE $%d OR mp.brand ILIKE $%d OR p.display_name ILIKE $%d OR p.original_name ILIKE $%d)",
			argIdx, argIdx, argIdx, argIdx, argIdx, argIdx))
		args = append(args, "%"+filter.Search+"%")
		argIdx++
	}
	if filter.CategoryID != "" {
		where = append(where, fmt.Sprintf(`mp.id IN (
				SELECT mpc.master_product_id FROM catalog.master_product_categories mpc
				WHERE mpc.master_category_id IN (
					WITH RECURSIVE sub AS (
						SELECT id FROM catalog.master_categories WHERE id = $%d
						UNION ALL
						SELECT c.id FROM catalog.master_categories c
						JOIN sub s ON c.parent_id = s.id
					) SELECT id FROM sub
				)
			)`, argIdx))
		args = append(args, filter.CategoryID)
		argIdx++
	}

	whereClause := strings.Join(where, " AND ")

	// Two-path join: legacy products go through p.master_product_id directly;
	// new metadata-first products go through master_variants. COALESCE picks
	// whichever path resolves the master row.
	const joins = `
		LEFT JOIN catalog.master_variants mv ON mv.id = p.master_variant_id
		LEFT JOIN catalog.master_products mp ON mp.id = COALESCE(p.master_product_id, mv.master_product_id)
		LEFT JOIN LATERAL (
			SELECT mc.id, mc.name FROM catalog.master_product_categories mpc
			JOIN catalog.master_categories mc ON mc.id = mpc.master_category_id
			WHERE mpc.master_product_id = mp.id ORDER BY mc.name LIMIT 1
		) c ON true
		LEFT JOIN catalog.stock st ON st.product_id = p.id AND st.tenant_id = p.tenant_id`

	// Count
	countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM catalog.products p%s WHERE %s`, joins, whereClause)
	var total int
	if err := a.client.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count products: %w", err)
	}

	// Fetch
	query := fmt.Sprintf(`SELECT
		p.id, p.tenant_id,
		COALESCE(p.master_product_id::text, '') AS master_product_id,
		COALESCE(p.master_variant_id::text, '') AS master_variant_id,
		p.name,
		COALESCE(p.display_name, '') AS display_name,
		COALESCE(p.original_name, '') AS original_name,
		p.description,
		p.price, p.currency,
		COALESCE(st.quantity, p.stock_quantity) AS stock_quantity,
		p.rating, p.images, COALESCE(p.tags, '[]') AS tags,
		COALESCE(p.raw_attributes, '{}'::jsonb) AS raw_attributes,
		COALESCE(p.media, '[]'::jsonb) AS media,
		p.created_at, p.updated_at,
		mp.id, mp.name, mp.description, mp.brand, mp.sku, mp.images,
		mv.sku, mv.gtins, mv.size, mv.color, mv.image_url, mv.weight_g, mv.volume_ml,
		c.name
		FROM catalog.products p%s
		WHERE %s
		ORDER BY p.created_at DESC
		LIMIT $%d OFFSET $%d`, joins, whereClause, argIdx, argIdx+1)
	args = append(args, filter.Limit, filter.Offset)

	rows, err := a.client.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list products: %w", err)
	}
	defer rows.Close()

	var products []domain.Product
	for rows.Next() {
		var p domain.Product
		var pImagesJSON, tagsJSON, rawAttrsJSON, mediaJSON, mpImagesJSON []byte
		var mpID, mpName, mpDesc, mpBrand, mpSKU *string
		var mvSKU, mvSize, mvColor, mvImageURL *string
		var mvGTINs []string
		var mvWeightG, mvVolumeML *int
		var catName *string

		if err := rows.Scan(
			&p.ID, &p.TenantID, &p.MasterProductID, &p.MasterVariantID,
			&p.Name, &p.DisplayName, &p.OriginalName, &p.Description,
			&p.Price, &p.Currency, &p.StockQuantity, &p.Rating, &pImagesJSON, &tagsJSON,
			&rawAttrsJSON, &mediaJSON,
			&p.CreatedAt, &p.UpdatedAt,
			&mpID, &mpName, &mpDesc, &mpBrand, &mpSKU, &mpImagesJSON,
			&mvSKU, &mvGTINs, &mvSize, &mvColor, &mvImageURL, &mvWeightG, &mvVolumeML,
			&catName,
		); err != nil {
			return nil, 0, fmt.Errorf("scan product: %w", err)
		}

		mergeProductFromJoins(&p,
			pImagesJSON, tagsJSON, rawAttrsJSON, mediaJSON, mpImagesJSON,
			mpName, mpDesc, mpBrand, mpSKU,
			mvSKU, mvGTINs, mvSize, mvColor, mvImageURL, mvWeightG, mvVolumeML,
			catName,
		)
		p.PriceFormatted = formatPrice(p.Price, p.Currency)
		products = append(products, p)
	}

	return products, total, nil
}

// mergeProductFromJoins applies COALESCE-style merging over the listing,
// master_variants, and master_products rows. Listing fields take priority,
// then variant fields, then master fields. Used by both ListProducts and
// GetProduct to keep behaviour consistent.
func mergeProductFromJoins(
	p *domain.Product,
	pImagesJSON, tagsJSON, rawAttrsJSON, mediaJSON, mpImagesJSON []byte,
	mpName, mpDesc, mpBrand, mpSKU *string,
	mvSKU *string, mvGTINs []string, mvSize, mvColor, mvImageURL *string,
	mvWeightG, mvVolumeML *int,
	catName *string,
) {
	// Display name: listing.display_name → listing.original_name → listing.name → master.name
	if p.DisplayName != "" {
		p.Name = p.DisplayName
	} else if p.OriginalName != "" {
		p.Name = p.OriginalName
	} else if p.Name == "" && mpName != nil {
		p.Name = *mpName
	}
	if p.Description == "" && mpDesc != nil {
		p.Description = *mpDesc
	}
	if mpBrand != nil {
		p.Brand = *mpBrand
	}
	if catName != nil {
		p.Category = *catName
	}

	// SKU: variant first, then legacy master sku.
	if mvSKU != nil && *mvSKU != "" {
		p.SKU = *mvSKU
	} else if mpSKU != nil {
		p.SKU = *mpSKU
	}
	if len(mvGTINs) > 0 {
		p.GTINs = mvGTINs
	}
	if mvSize != nil {
		p.Size = *mvSize
	}
	if mvColor != nil {
		p.Color = *mvColor
	}
	if mvWeightG != nil {
		p.WeightG = mvWeightG
	}
	if mvVolumeML != nil {
		p.VolumeML = mvVolumeML
	}

	// Images: listing media (new) → listing.images (legacy) → variant.image_url → master.images
	if len(mediaJSON) > 0 {
		_ = json.Unmarshal(mediaJSON, &p.Media)
	}
	if len(pImagesJSON) > 0 {
		_ = json.Unmarshal(pImagesJSON, &p.Images)
	}
	if len(p.Images) == 0 && mvImageURL != nil && *mvImageURL != "" {
		p.Images = []string{*mvImageURL}
	}
	if len(p.Images) == 0 && len(mpImagesJSON) > 0 {
		_ = json.Unmarshal(mpImagesJSON, &p.Images)
	}

	if len(tagsJSON) > 0 {
		_ = json.Unmarshal(tagsJSON, &p.Tags)
	}
	if len(rawAttrsJSON) > 0 {
		_ = json.Unmarshal(rawAttrsJSON, &p.RawAttributes)
	}
}

func (a *CatalogAdapter) GetProduct(ctx context.Context, tenantID string, productID string) (*domain.Product, error) {
	if sc := domain.SpanFromContext(ctx); sc != nil {
		endSpan := sc.Start("db.admin.get_product")
		defer endSpan()
	}
	query := `SELECT
		p.id, p.tenant_id,
		COALESCE(p.master_product_id::text, '') AS master_product_id,
		COALESCE(p.master_variant_id::text, '') AS master_variant_id,
		p.name,
		COALESCE(p.display_name, '') AS display_name,
		COALESCE(p.original_name, '') AS original_name,
		p.description,
		p.price, p.currency,
		COALESCE(st.quantity, p.stock_quantity) AS stock_quantity,
		p.rating, p.images, COALESCE(p.tags, '[]') AS tags,
		COALESCE(p.raw_attributes, '{}'::jsonb) AS raw_attributes,
		COALESCE(p.media, '[]'::jsonb) AS media,
		p.created_at, p.updated_at,
		mp.id, mp.name, mp.description, mp.brand, mp.sku, mp.images,
		mv.sku, mv.gtins, mv.size, mv.color, mv.image_url, mv.weight_g, mv.volume_ml,
		c.name
		FROM catalog.products p
		LEFT JOIN catalog.master_variants mv ON mv.id = p.master_variant_id
		LEFT JOIN catalog.master_products mp ON mp.id = COALESCE(p.master_product_id, mv.master_product_id)
		LEFT JOIN LATERAL (
			SELECT mc.id, mc.name FROM catalog.master_product_categories mpc
			JOIN catalog.master_categories mc ON mc.id = mpc.master_category_id
			WHERE mpc.master_product_id = mp.id ORDER BY mc.name LIMIT 1
		) c ON true
		LEFT JOIN catalog.stock st ON st.product_id = p.id AND st.tenant_id = p.tenant_id
		WHERE p.id = $1 AND p.tenant_id = $2 AND p.deleted_at IS NULL`

	var p domain.Product
	var pImagesJSON, tagsJSON, rawAttrsJSON, mediaJSON, mpImagesJSON []byte
	var mpID, mpName, mpDesc, mpBrand, mpSKU *string
	var mvSKU, mvSize, mvColor, mvImageURL *string
	var mvGTINs []string
	var mvWeightG, mvVolumeML *int
	var catName *string

	err := a.client.pool.QueryRow(ctx, query, productID, tenantID).Scan(
		&p.ID, &p.TenantID, &p.MasterProductID, &p.MasterVariantID,
		&p.Name, &p.DisplayName, &p.OriginalName, &p.Description,
		&p.Price, &p.Currency, &p.StockQuantity, &p.Rating, &pImagesJSON, &tagsJSON,
		&rawAttrsJSON, &mediaJSON,
		&p.CreatedAt, &p.UpdatedAt,
		&mpID, &mpName, &mpDesc, &mpBrand, &mpSKU, &mpImagesJSON,
		&mvSKU, &mvGTINs, &mvSize, &mvColor, &mvImageURL, &mvWeightG, &mvVolumeML,
		&catName,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrProductNotFound
		}
		return nil, fmt.Errorf("get product: %w", err)
	}

	mergeProductFromJoins(&p,
		pImagesJSON, tagsJSON, rawAttrsJSON, mediaJSON, mpImagesJSON,
		mpName, mpDesc, mpBrand, mpSKU,
		mvSKU, mvGTINs, mvSize, mvColor, mvImageURL, mvWeightG, mvVolumeML,
		catName,
	)
	p.PriceFormatted = formatPrice(p.Price, p.Currency)
	return &p, nil
}

func (a *CatalogAdapter) UpdateProduct(ctx context.Context, tenantID string, productID string, update domain.ProductUpdate) error {
	if sc := domain.SpanFromContext(ctx); sc != nil {
		endSpan := sc.Start("db.admin.update_product")
		defer endSpan()
	}
	sets := []string{}
	args := []any{}
	argIdx := 1

	if update.Name != nil {
		sets = append(sets, fmt.Sprintf("name = $%d", argIdx))
		args = append(args, *update.Name)
		argIdx++
	}
	if update.DisplayName != nil {
		sets = append(sets, fmt.Sprintf("display_name = NULLIF($%d, '')", argIdx))
		args = append(args, *update.DisplayName)
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
	if update.RawAttributes != nil {
		raJSON, err := json.Marshal(*update.RawAttributes)
		if err != nil {
			return fmt.Errorf("marshal raw_attributes: %w", err)
		}
		sets = append(sets, fmt.Sprintf("raw_attributes = $%d", argIdx))
		args = append(args, raJSON)
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

// GetCategories returns all categories that themselves or via any descendant
// have ≥1 non-deleted product for the given tenant. Includes a transitive
// product count so parent nodes show totals across their subtree.
func (a *CatalogAdapter) GetCategories(ctx context.Context, tenantID string) ([]domain.Category, error) {
	query := `
		WITH RECURSIVE descendants AS (
		    SELECT id, id AS root_id FROM catalog.master_categories
		    UNION ALL
		    SELECT c.id, d.root_id
		    FROM catalog.master_categories c
		    JOIN descendants d ON c.parent_id = d.id
		),
		counts AS (
		    SELECT d.root_id, COUNT(DISTINCT p.id) AS n
		    FROM descendants d
		    LEFT JOIN catalog.master_product_categories mpc ON mpc.master_category_id = d.id
			    LEFT JOIN catalog.master_products mp ON mp.id = mpc.master_product_id
		    LEFT JOIN catalog.products p
		        ON p.master_product_id = mp.id
		        AND p.tenant_id = $1
		        AND p.deleted_at IS NULL
		    GROUP BY d.root_id
		)
		SELECT c.id, c.name, c.slug, COALESCE(c.parent_id::text, ''),
		       COALESCE(counts.n, 0)::int AS product_count
		FROM catalog.master_categories c
		LEFT JOIN counts ON counts.root_id = c.id
		WHERE COALESCE(counts.n, 0) > 0
		ORDER BY c.name`
	rows, err := a.client.pool.Query(ctx, query, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list categories: %w", err)
	}
	defer rows.Close()

	var cats []domain.Category
	for rows.Next() {
		var c domain.Category
		if err := rows.Scan(&c.ID, &c.Name, &c.Slug, &c.ParentID, &c.ProductCount); err != nil {
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
		`SELECT id FROM catalog.master_categories WHERE slug = $1`, slug).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("query category: %w", err)
	}

	// Create
	err = a.client.pool.QueryRow(ctx,
		`INSERT INTO catalog.master_categories (name, slug, vertical) VALUES ($1, $2, 'unknown')
		ON CONFLICT (slug) DO UPDATE SET name = EXCLUDED.name
		RETURNING id`, name, slug).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("create category: %w", err)
	}
	return id, nil
}

// --- Import upserts ---

func (a *CatalogAdapter) UpsertMasterProduct(ctx context.Context, mp *domain.MasterProduct) (string, error) {
	if sc := domain.SpanFromContext(ctx); sc != nil {
		endSpan := sc.Start("db.admin.upsert_master_product")
		defer endSpan()
	}
	imagesJSON, _ := json.Marshal(mp.Images)

	query := `INSERT INTO catalog.master_products
			(sku, name, description, brand, images, owner_tenant_id, source_system, source_id)
		VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7,''), NULLIF($8,''))
		ON CONFLICT (sku) DO UPDATE SET
			name = EXCLUDED.name,
			brand = EXCLUDED.brand,
			images = EXCLUDED.images,
			source_system = COALESCE(EXCLUDED.source_system, catalog.master_products.source_system),
			source_id = COALESCE(EXCLUDED.source_id, catalog.master_products.source_id),
			updated_at = NOW()
		RETURNING id`

	var id string
	err := a.client.pool.QueryRow(ctx, query,
		mp.SKU, mp.Name, mp.Description, mp.Brand,
		imagesJSON, mp.OwnerTenantID, mp.SourceSystem, mp.SourceID,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("upsert master product: %w", err)
	}
	// Group D: categorization via the master_product_categories junction.
	if mp.CategoryID != "" {
		if _, err := a.client.pool.Exec(ctx,
			`INSERT INTO catalog.master_product_categories (master_product_id, master_category_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, id, mp.CategoryID); err != nil {
			return "", fmt.Errorf("link master category: %w", err)
		}
	}
	return id, nil
}

func (a *CatalogAdapter) UpsertProductListing(ctx context.Context, p *domain.Product) (string, error) {
	if sc := domain.SpanFromContext(ctx); sc != nil {
		endSpan := sc.Start("db.admin.upsert_product_listing")
		defer endSpan()
	}
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

// --- Enrichment ---

func (a *CatalogAdapter) GetCategoryBySlug(ctx context.Context, slug string) (*domain.Category, error) {
	query := `SELECT id, name, slug, COALESCE(parent_id::text, '') FROM catalog.master_categories WHERE slug = $1`
	var c domain.Category
	err := a.client.pool.QueryRow(ctx, query, slug).Scan(&c.ID, &c.Name, &c.Slug, &c.ParentID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrCategoryNotFound
		}
		return nil, fmt.Errorf("get category by slug: %w", err)
	}
	return &c, nil
}

// GetMasterProductsForEnrichment returns products without PIM data for enrichment.
func (a *CatalogAdapter) GetMasterProductsForEnrichment(ctx context.Context, tenantID string) ([]domain.MasterProduct, error) {
	// Products that don't have enriched PIM data yet (empty tier2). Group D:
	// "enriched" = tier2 populated (per-vertical typed columns removed).
	query := `SELECT mp.id, mp.sku, mp.name, mp.description, mp.brand, COALESCE(c.id::text,''),
		mp.images, mp.owner_tenant_id, c.name
		FROM catalog.master_products mp
		LEFT JOIN LATERAL (
			SELECT mc.id, mc.name FROM catalog.master_product_categories mpc
			JOIN catalog.master_categories mc ON mc.id = mpc.master_category_id
			WHERE mpc.master_product_id = mp.id ORDER BY mc.name LIMIT 1
		) c ON true
		WHERE mp.owner_tenant_id = $1
			AND (mp.tier2 IS NULL OR mp.tier2 = '{}'::jsonb)
		ORDER BY mp.created_at`

	rows, err := a.client.pool.Query(ctx, query, tenantID)
	if err != nil {
		return nil, fmt.Errorf("get products for enrichment: %w", err)
	}
	defer rows.Close()

	var products []domain.MasterProduct
	for rows.Next() {
		var mp domain.MasterProduct
		var imagesJSON []byte
		var catName *string
		if err := rows.Scan(
			&mp.ID, &mp.SKU, &mp.Name, &mp.Description, &mp.Brand, &mp.CategoryID,
			&imagesJSON, &mp.OwnerTenantID, &catName,
		); err != nil {
			return nil, fmt.Errorf("scan master product: %w", err)
		}
		if len(imagesJSON) > 0 {
			json.Unmarshal(imagesJSON, &mp.Images)
		}
		if catName != nil {
			mp.CategoryName = *catName
		}
		products = append(products, mp)
	}
	return products, nil
}

// --- Enrichment V2 ---

func (a *CatalogAdapter) GetAllMasterProducts(ctx context.Context, tenantID string) ([]domain.MasterProduct, error) {
	query := `SELECT mp.id, mp.sku, mp.name, mp.description, mp.brand, COALESCE(c.id::text,''),
		mp.images, mp.owner_tenant_id, c.name
		FROM catalog.master_products mp
		LEFT JOIN LATERAL (
			SELECT mc.id, mc.name FROM catalog.master_product_categories mpc
			JOIN catalog.master_categories mc ON mc.id = mpc.master_category_id
			WHERE mpc.master_product_id = mp.id ORDER BY mc.name LIMIT 1
		) c ON true
		WHERE mp.owner_tenant_id = $1
		ORDER BY mp.created_at`

	rows, err := a.client.pool.Query(ctx, query, tenantID)
	if err != nil {
		return nil, fmt.Errorf("get all master products: %w", err)
	}
	defer rows.Close()

	var products []domain.MasterProduct
	for rows.Next() {
		var mp domain.MasterProduct
		var imagesJSON []byte
		var catName *string
		if err := rows.Scan(
			&mp.ID, &mp.SKU, &mp.Name, &mp.Description, &mp.Brand, &mp.CategoryID,
			&imagesJSON, &mp.OwnerTenantID, &catName,
		); err != nil {
			return nil, fmt.Errorf("scan master product: %w", err)
		}
		if len(imagesJSON) > 0 {
			json.Unmarshal(imagesJSON, &mp.Images)
		}
		if catName != nil {
			mp.CategoryName = *catName
		}
		products = append(products, mp)
	}
	return products, nil
}

// GetUnenrichedMasterProducts returns masters with no tier2 attributes yet —
// the incremental enrichment path used by the post-import auto-trigger. Group D:
// "enriched" now means "tier2 populated" (enrichment_version column removed).
func (a *CatalogAdapter) GetUnenrichedMasterProducts(ctx context.Context, tenantID string) ([]domain.MasterProduct, error) {
	query := `SELECT mp.id, mp.sku, mp.name, mp.description, mp.brand, COALESCE(c.id::text,''),
		mp.images, mp.owner_tenant_id, c.name
		FROM catalog.master_products mp
		LEFT JOIN LATERAL (
			SELECT mc.id, mc.name FROM catalog.master_product_categories mpc
			JOIN catalog.master_categories mc ON mc.id = mpc.master_category_id
			WHERE mpc.master_product_id = mp.id ORDER BY mc.name LIMIT 1
		) c ON true
		WHERE mp.owner_tenant_id = $1
			AND (mp.tier2 IS NULL OR mp.tier2 = '{}'::jsonb)
		ORDER BY mp.created_at`

	rows, err := a.client.pool.Query(ctx, query, tenantID)
	if err != nil {
		return nil, fmt.Errorf("get unenriched master products: %w", err)
	}
	defer rows.Close()

	var products []domain.MasterProduct
	for rows.Next() {
		var mp domain.MasterProduct
		var imagesJSON []byte
		var catName *string
		if err := rows.Scan(
			&mp.ID, &mp.SKU, &mp.Name, &mp.Description, &mp.Brand, &mp.CategoryID,
			&imagesJSON, &mp.OwnerTenantID, &catName,
		); err != nil {
			return nil, fmt.Errorf("scan master product: %w", err)
		}
		if len(imagesJSON) > 0 {
			json.Unmarshal(imagesJSON, &mp.Images)
		}
		if catName != nil {
			mp.CategoryName = *catName
		}
		products = append(products, mp)
	}
	return products, nil
}

// UpsertListingFromSource — harvester-lite write path (curator pivot 2026-04-27).
// Idempotent upsert keyed by (tenant_id, source_system, source_id) via the
// unique partial index added in catalog_migrations.go. NEVER writes to master_*.
// On re-import (re-running harvester) UPDATE keeps existing master_*_id linkage —
// curator's merge step is the only writer of those columns.
func (a *CatalogAdapter) UpsertListingFromSource(ctx context.Context, p *domain.ListingFromSource) (string, error) {
	if sc := domain.SpanFromContext(ctx); sc != nil {
		endSpan := sc.Start("db.admin.upsert_listing_from_source")
		defer endSpan()
	}
	if p.TenantID == "" || p.SourceSystem == "" || p.SourceID == "" {
		return "", fmt.Errorf("upsert listing: tenant_id/source_system/source_id required")
	}
	imagesJSON, _ := json.Marshal(p.Images)
	mediaJSON, _ := json.Marshal(p.Media)
	rawAttrsJSON, _ := json.Marshal(p.RawAttributes)

	const q = `
		INSERT INTO catalog.products (
			tenant_id, name, original_name, description,
			price, currency, stock_quantity, images, media, raw_attributes,
			source_system, source_id, payload_hash, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, NOW(), NOW())
		ON CONFLICT (tenant_id, source_system, source_id)
			WHERE source_system IS NOT NULL AND source_id IS NOT NULL
		DO UPDATE SET
			name           = EXCLUDED.name,
			original_name  = EXCLUDED.original_name,
			description    = EXCLUDED.description,
			price          = EXCLUDED.price,
			currency       = EXCLUDED.currency,
			stock_quantity = EXCLUDED.stock_quantity,
			images         = EXCLUDED.images,
			media          = EXCLUDED.media,
			raw_attributes = EXCLUDED.raw_attributes,
			payload_hash   = EXCLUDED.payload_hash,
			deleted_at     = NULL,
			updated_at     = NOW()
		RETURNING id`
	var id string
	err := a.client.pool.QueryRow(ctx, q,
		p.TenantID, p.Name, p.OriginalName, p.Description,
		p.PriceCents, p.Currency, p.StockQuantity,
		imagesJSON, mediaJSON, rawAttrsJSON,
		p.SourceSystem, p.SourceID, p.PayloadHash,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("upsert listing from source: %w", err)
	}
	return id, nil
}

// SoftDeleteProductBySource flips catalog.products.deleted_at for the listing
// that ties back to the given (source_system, source_id) on master_products.
// Only the tenant-scoped listing is removed from surface; the master row
// stays so future re-sync can reanimate it.
func (a *CatalogAdapter) SoftDeleteProductBySource(ctx context.Context, tenantID, sourceSystem, sourceID string) error {
	_, err := a.client.pool.Exec(ctx, `
		UPDATE catalog.products p SET deleted_at = NOW(), updated_at = NOW()
		FROM catalog.master_products mp
		WHERE p.tenant_id = $1
			AND p.master_product_id = mp.id
			AND mp.source_system = $2
			AND mp.source_id = $3
			AND p.deleted_at IS NULL`,
		tenantID, sourceSystem, sourceID)
	if err != nil {
		return fmt.Errorf("soft delete by source: %w", err)
	}
	return nil
}

// UpdateMasterProductPIM writes the LLM enrichment output to the master. Group D:
// typed per-vertical attributes go to master_products.tier2 (jsonb), provenance
// (original_name, product_line) to tier3 — no per-vertical typed columns.
func (a *CatalogAdapter) UpdateMasterProductPIM(ctx context.Context, productID string, categoryID string, out domain.EnrichmentOutputV2) error {
	tier2 := map[string]any{}
	if out.ProductForm != "" {
		tier2["product_form"] = out.ProductForm
	}
	if out.Texture != "" {
		tier2["texture"] = out.Texture
	}
	if out.RoutineStep != "" {
		tier2["routine_step"] = out.RoutineStep
	}
	if out.RoutineTime != "" {
		tier2["routine_time"] = out.RoutineTime
	}
	if out.ApplicationMethod != "" {
		tier2["application_method"] = out.ApplicationMethod
	}
	if out.MarketingClaim != "" {
		tier2["marketing_claim"] = out.MarketingClaim
	}
	if len(out.SkinType) > 0 {
		tier2["skin_type"] = out.SkinType
	}
	if len(out.Concern) > 0 {
		tier2["concern"] = out.Concern
	}
	if len(out.KeyIngredients) > 0 {
		tier2["key_ingredients"] = out.KeyIngredients
	}
	if len(out.TargetArea) > 0 {
		tier2["target_area"] = out.TargetArea
	}
	if len(out.FreeFrom) > 0 {
		tier2["free_from"] = out.FreeFrom
	}
	if len(out.Benefits) > 0 {
		tier2["benefits"] = out.Benefits
	}
	tier3 := map[string]any{}
	if out.OriginalName != "" {
		tier3["original_name"] = out.OriginalName
	}
	if out.ProductLine != "" {
		tier3["product_line"] = out.ProductLine
	}
	tier2JSON, _ := json.Marshal(tier2)
	tier3JSON, _ := json.Marshal(tier3)

	query := `UPDATE catalog.master_products SET
		name = $1,
		tier2 = COALESCE(tier2, '{}'::jsonb) || $2::jsonb,
		tier3 = COALESCE(tier3, '{}'::jsonb) || $3::jsonb,
		updated_at = NOW()
	WHERE id = $4`
	args := []any{out.ShortName, tier2JSON, tier3JSON, productID}

	tag, err := a.client.pool.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("update master product PIM: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrProductNotFound
	}
	// Group D: categorization via the master_product_categories junction.
	if categoryID != "" {
		if _, err := a.client.pool.Exec(ctx,
			`INSERT INTO catalog.master_product_categories (master_product_id, master_category_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, productID, categoryID); err != nil {
			return fmt.Errorf("link master category: %w", err)
		}
	}
	return nil
}

// --- Post-import ---

// jsonStr / jsonStrSlice extract typed values from a decoded tier2/tier3 jsonb
// map. Arrays decode as []any, so jsonStrSlice narrows to []string.
func jsonStr(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func jsonStrSlice(m map[string]any, key string) []string {
	raw, ok := m[key].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, e := range raw {
		if s, ok := e.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func (a *CatalogAdapter) GetMasterProductsWithoutEmbedding(ctx context.Context, tenantID string) ([]domain.MasterProduct, error) {
	query := `SELECT mp.id, mp.sku, mp.name, mp.description, mp.brand, COALESCE(c.id::text,''),
		mp.images, mp.owner_tenant_id, c.name, COALESCE(mp.tier2, '{}'::jsonb)
		FROM catalog.master_products mp
		LEFT JOIN LATERAL (
			SELECT mc.id, mc.name FROM catalog.master_product_categories mpc
			JOIN catalog.master_categories mc ON mc.id = mpc.master_category_id
			WHERE mpc.master_product_id = mp.id ORDER BY mc.name LIMIT 1
		) c ON true
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
		var imagesJSON, tier2JSON []byte
		var catName *string
		if err := rows.Scan(
			&mp.ID, &mp.SKU, &mp.Name, &mp.Description, &mp.Brand, &mp.CategoryID,
			&imagesJSON, &mp.OwnerTenantID, &catName, &tier2JSON,
		); err != nil {
			return nil, fmt.Errorf("scan master product: %w", err)
		}
		if len(imagesJSON) > 0 {
			json.Unmarshal(imagesJSON, &mp.Images)
		}
		if catName != nil {
			mp.CategoryName = *catName
		}
		// Group D: typed attrs come from tier2 jsonb (master_cosmetics removed).
		if len(tier2JSON) > 0 {
			var t2 map[string]any
			if json.Unmarshal(tier2JSON, &t2) == nil {
				mp.ProductForm = jsonStr(t2, "product_form")
				mp.Texture = jsonStr(t2, "texture")
				mp.RoutineStep = jsonStr(t2, "routine_step")
				mp.MarketingClaim = jsonStr(t2, "marketing_claim")
				mp.SkinType = jsonStrSlice(t2, "skin_type")
				mp.Concern = jsonStrSlice(t2, "concern")
				mp.KeyIngredients = jsonStrSlice(t2, "key_ingredients")
			}
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
	if sc := domain.SpanFromContext(ctx); sc != nil {
		endSpan := sc.Start("db.admin.generate_catalog_digest")
		defer endSpan()
	}
	// Build a compact digest of the tenant's catalog
	query := `
		WITH tenant_products AS (
			SELECT mp.id, mp.name, mp.brand, mp.attributes, c.name AS category_name, p.price, p.currency
			FROM catalog.products p
			JOIN catalog.master_products mp ON p.master_product_id = mp.id
			LEFT JOIN LATERAL (
			SELECT mc.id, mc.name FROM catalog.master_product_categories mpc
			JOIN catalog.master_categories mc ON mc.id = mpc.master_category_id
			WHERE mpc.master_product_id = mp.id ORDER BY mc.name LIMIT 1
		) c ON true
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

// --- Stock ---

func (a *CatalogAdapter) BulkUpdateStock(ctx context.Context, tenantID string, items []domain.StockUpdate) (int, error) {
	if sc := domain.SpanFromContext(ctx); sc != nil {
		endSpan := sc.Start("db.admin.bulk_update_stock")
		defer endSpan()
	}
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

// --- helpers ---

// formatPrice renders cents → human-readable string. Default currency is USD.
// Supported: USD ($1,234.56), EUR (€1,234.56). Other ISO codes render with their
// 3-letter code prefix (e.g. "GBP 1,234.56") rather than guessing a symbol.
func formatPrice(cents int, currency string) string {
	if currency == "" {
		currency = "USD"
	}
	whole := cents / 100
	fraction := cents % 100
	if fraction < 0 {
		fraction = -fraction
	}
	str := fmt.Sprintf("%d", whole)
	var sb strings.Builder
	for i, c := range str {
		if i > 0 && (len(str)-i)%3 == 0 {
			sb.WriteByte(',')
		}
		sb.WriteRune(c)
	}
	body := fmt.Sprintf("%s.%02d", sb.String(), fraction)
	switch currency {
	case "USD":
		return "$" + body
	case "EUR":
		return "€" + body
	default:
		return currency + " " + body
	}
}
