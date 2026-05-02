package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5"
	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/ports"
)

// CatalogAdapter implements ports.CatalogPort against the shared catalog.*
// schema. Read-only — V5 never writes catalog (admin/curator own that).
//
// Two-path master resolution: legacy products via products.master_product_id;
// new metadata-first listings via master_variants. COALESCE picks whichever
// path is populated, so heybabes (no master_variant_id) and electronics
// (variant-first) both work in one query.
//
// Tier2 (master-curated JSONB) is unmarshalled into Product.Tier2 in
// mergeProductWithMaster. Per chunk-3 plan the binding-side precedence is
// Typed > Tier2 > Extra (curator wins) — that lives in ProductToMap, not
// here. This adapter just delivers the raw layered data.
type CatalogAdapter struct {
	client *Client
	log    *slog.Logger
}

func NewCatalogAdapter(client *Client) *CatalogAdapter {
	return &CatalogAdapter{client: client, log: slog.Default()}
}

func (a *CatalogAdapter) GetTenantBySlug(ctx context.Context, slug string) (*domain.Tenant, error) {
	if sc := domain.SpanFromContext(ctx); sc != nil {
		end := sc.Start("postgres.GetTenantBySlug")
		defer end(slug)
	}
	query := `
		SELECT id, slug, name, type, settings, created_at, updated_at
		FROM catalog.tenants
		WHERE slug = $1
	`
	var tenant domain.Tenant
	var settingsJSON []byte
	err := a.client.pool.QueryRow(ctx, query, slug).Scan(
		&tenant.ID, &tenant.Slug, &tenant.Name, &tenant.Type,
		&settingsJSON, &tenant.CreatedAt, &tenant.UpdatedAt,
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

const catalogProductSelect = `
	SELECT
		p.id, p.tenant_id,
		COALESCE(p.master_product_id::text, '') as master_product_id,
		COALESCE(p.master_variant_id::text, '') as master_variant_id,
		COALESCE(p.name, '') as name,
		COALESCE(p.display_name, '') as display_name,
		COALESCE(p.original_name, '') as original_name,
		COALESCE(p.description, '') as description,
		p.price, p.currency, COALESCE(s.quantity, 0) as stock_quantity, COALESCE(p.rating, 0) as rating,
		COALESCE(p.images, '[]') as images, COALESCE(p.tags, '[]') as tags,
		mp.id as mp_id, mp.sku, mp.name as mp_name, mp.description as mp_description,
		mp.brand, mp.category_id, mp.images as mp_images,
		mv.sku as mv_sku, mv.gtins as mv_gtins, mv.size as mv_size,
		mv.color as mv_color, mv.image_url as mv_image_url,
		mv.weight_g as mv_weight_g, mv.volume_ml as mv_volume_ml,
		c.name as category_name,
		COALESCE(mp.product_form, '') as product_form,
		COALESCE(mp.texture, '') as texture,
		COALESCE(mp.routine_step, '') as routine_step,
		mp.skin_type, mp.concern, mp.key_ingredients, mp.target_area,
		COALESCE(mp.marketing_claim, '') as marketing_claim,
		mp.benefits,
		COALESCE(p.extra, '{}'::jsonb) as extra,
		COALESCE(mp.tier2, '{}'::jsonb) as mp_tier2
	FROM catalog.products p
	LEFT JOIN catalog.master_variants mv ON mv.id = p.master_variant_id
	LEFT JOIN catalog.master_products mp ON mp.id = COALESCE(p.master_product_id, mv.master_product_id)
	LEFT JOIN catalog.categories c ON mp.category_id = c.id
	LEFT JOIN catalog.stock s ON s.product_id = p.id AND s.tenant_id = p.tenant_id
`

func (a *CatalogAdapter) ListProducts(ctx context.Context, tenantID string, filter ports.ProductFilter) ([]domain.Product, int, error) {
	if sc := domain.SpanFromContext(ctx); sc != nil {
		end := sc.Start("postgres.ListProducts")
		defer end(fmt.Sprintf("limit=%d", filter.Limit))
	}
	baseQuery := catalogProductSelect + ` WHERE p.tenant_id = $1 AND p.deleted_at IS NULL`
	countQuery := `
		SELECT COUNT(*)
		FROM catalog.products p
		LEFT JOIN catalog.master_variants mv ON mv.id = p.master_variant_id
		LEFT JOIN catalog.master_products mp ON mp.id = COALESCE(p.master_product_id, mv.master_product_id)
		LEFT JOIN catalog.categories c ON mp.category_id = c.id
		LEFT JOIN catalog.stock s ON s.product_id = p.id AND s.tenant_id = p.tenant_id
		WHERE p.tenant_id = $1 AND p.deleted_at IS NULL
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
		const searchPredicate = `(
			p.name ILIKE %[1]s OR p.display_name ILIKE %[1]s OR p.original_name ILIKE %[1]s OR
			mp.name ILIKE %[1]s OR mp.brand ILIKE %[1]s OR
			p.raw_attributes::text ILIKE %[1]s
		)`
		words := strings.Fields(filter.Search)
		if len(words) == 1 {
			conditions = append(conditions, fmt.Sprintf(searchPredicate, fmt.Sprintf("$%d", argNum)))
			args = append(args, "%"+words[0]+"%")
			argNum++
		} else if len(words) > 1 {
			var wordConds []string
			for _, word := range words {
				wordConds = append(wordConds, fmt.Sprintf(searchPredicate, fmt.Sprintf("$%d", argNum)))
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

	var total int
	if err := a.client.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count products: %w", err)
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

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
		p, err := scanCatalogProduct(rows)
		if err != nil {
			return nil, 0, err
		}
		p.PriceFormatted = formatPrice(p.Price, p.Currency)
		products = append(products, *p)
	}
	return products, total, nil
}

func (a *CatalogAdapter) GetProduct(ctx context.Context, tenantID string, productID string) (*domain.Product, error) {
	if sc := domain.SpanFromContext(ctx); sc != nil {
		end := sc.Start("postgres.GetProduct")
		defer end(productID)
	}
	query := catalogProductSelect + ` WHERE p.tenant_id = $1 AND p.id = $2 AND p.deleted_at IS NULL`
	row := a.client.pool.QueryRow(ctx, query, tenantID, productID)

	p, err := scanCatalogProduct(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrProductNotFound
		}
		return nil, fmt.Errorf("query product: %w", err)
	}
	p.PriceFormatted = formatPrice(p.Price, p.Currency)
	return p, nil
}

// rowScanner is the minimal surface satisfied by both pgx.Rows and pgx.Row.
type rowScanner interface {
	Scan(dest ...any) error
}

// scanCatalogProduct scans one product row from catalogProductSelect into a
// fully-merged domain.Product (typed + Extra + Tier2 + variant + master).
func scanCatalogProduct(row rowScanner) (*domain.Product, error) {
	var p domain.Product
	var masterProductID, mpID, mpSKU, mpName, mpDesc, mpBrand, mpCategoryID, categoryName *string
	var productImagesJSON, tagsJSON, mpImagesJSON, extraJSON, mpTier2JSON []byte
	var mpProductForm, mpTexture, mpRoutineStep, mpMarketingClaim *string
	var mpSkinType, mpConcern, mpKeyIngredients, mpTargetArea, mpBenefits []string
	var mvSKU, mvSize, mvColor, mvImageURL *string
	var mvGTINs []string
	var mvWeightG, mvVolumeML *int

	err := row.Scan(
		&p.ID, &p.TenantID, &masterProductID, &p.MasterVariantID,
		&p.Name, &p.DisplayName, &p.OriginalName,
		&p.Description, &p.Price, &p.Currency, &p.StockQuantity, &p.Rating, &productImagesJSON, &tagsJSON,
		&mpID, &mpSKU, &mpName, &mpDesc,
		&mpBrand, &mpCategoryID, &mpImagesJSON,
		&mvSKU, &mvGTINs, &mvSize, &mvColor, &mvImageURL, &mvWeightG, &mvVolumeML,
		&categoryName,
		&mpProductForm, &mpTexture, &mpRoutineStep,
		&mpSkinType, &mpConcern, &mpKeyIngredients, &mpTargetArea,
		&mpMarketingClaim, &mpBenefits,
		&extraJSON, &mpTier2JSON,
	)
	if err != nil {
		return nil, err
	}

	if len(productImagesJSON) > 0 {
		if err := json.Unmarshal(productImagesJSON, &p.Images); err != nil {
			return nil, fmt.Errorf("unmarshal product images: %w", err)
		}
	}
	if len(tagsJSON) > 0 {
		_ = json.Unmarshal(tagsJSON, &p.Tags)
	}
	if len(extraJSON) > 0 {
		_ = json.Unmarshal(extraJSON, &p.Extra)
	}

	if err := mergeProductWithMaster(&p, masterProductRow{
		MasterProductID: masterProductID,
		Name:            mpName,
		Description:     mpDesc,
		Brand:           mpBrand,
		SKU:             mpSKU,
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
		Tier2JSON:       mpTier2JSON,
		VariantSKU:      mvSKU,
		VariantGTINs:    mvGTINs,
		VariantSize:     mvSize,
		VariantColor:    mvColor,
		VariantImage:    mvImageURL,
		VariantWeightG:  mvWeightG,
		VariantVolumeML: mvVolumeML,
	}); err != nil {
		return nil, err
	}
	return &p, nil
}

type masterProductRow struct {
	MasterProductID *string
	Name            *string
	Description     *string
	Brand           *string
	SKU             *string
	CategoryName    *string
	ImagesJSON      []byte
	ProductForm     *string
	Texture         *string
	RoutineStep     *string
	SkinType        []string
	Concern         []string
	KeyIngredients  []string
	TargetArea      []string
	MarketingClaim  *string
	Benefits        []string
	Tier2JSON       []byte
	VariantSKU      *string
	VariantGTINs    []string
	VariantSize     *string
	VariantColor    *string
	VariantImage    *string
	VariantWeightG  *int
	VariantVolumeML *int
}

func mergeProductWithMaster(p *domain.Product, mp masterProductRow) error {
	switch {
	case p.DisplayName != "":
		p.Name = p.DisplayName
	case p.OriginalName != "":
		p.Name = p.OriginalName
	case p.Name == "" && mp.Name != nil:
		p.Name = *mp.Name
	}

	if mp.MasterProductID != nil && *mp.MasterProductID != "" {
		p.MasterProductID = *mp.MasterProductID
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

	if mp.VariantSKU != nil && *mp.VariantSKU != "" {
		p.SKU = *mp.VariantSKU
	} else if mp.SKU != nil {
		p.SKU = *mp.SKU
	}
	if len(mp.VariantGTINs) > 0 {
		p.GTINs = mp.VariantGTINs
	}
	if mp.VariantSize != nil {
		p.Size = *mp.VariantSize
	}
	if mp.VariantColor != nil {
		p.Color = *mp.VariantColor
	}
	if mp.VariantWeightG != nil {
		p.WeightG = mp.VariantWeightG
	}
	if mp.VariantVolumeML != nil {
		p.VolumeML = mp.VariantVolumeML
	}

	if len(p.Images) == 0 && mp.VariantImage != nil && *mp.VariantImage != "" {
		p.Images = []string{*mp.VariantImage}
	}
	if len(p.Images) == 0 && len(mp.ImagesJSON) > 0 {
		if err := json.Unmarshal(mp.ImagesJSON, &p.Images); err != nil {
			return fmt.Errorf("unmarshal master images: %w", err)
		}
	}

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

	if len(mp.Tier2JSON) > 0 {
		_ = json.Unmarshal(mp.Tier2JSON, &p.Tier2)
	}
	return nil
}

// formatPrice converts price-in-kopecks to a display string with thousand
// separators and a currency symbol.
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
