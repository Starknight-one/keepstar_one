package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

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

	baseQuery += fmt.Sprintf(" ORDER BY p.created_at DESC LIMIT $%d OFFSET $%d", argNum, argNum+1)
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
