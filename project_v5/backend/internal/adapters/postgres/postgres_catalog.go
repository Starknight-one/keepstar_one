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

// catalogCategoryJoin resolves a leaf category name via the master_categories
// junction (Group D: catalog.categories + master_products.category_id dropped).
const catalogCategoryJoin = `
	LEFT JOIN LATERAL (
		SELECT mc2.name FROM catalog.master_product_categories mpc
		JOIN catalog.master_categories mc2 ON mc2.id = mpc.master_category_id
		WHERE mpc.master_product_id = mp.id LIMIT 1
	) cat ON true`

const catalogProductSelect = catalogVectorSelect + `
	FROM catalog.products p
	LEFT JOIN catalog.master_variants mv ON mv.id = p.master_variant_id
	LEFT JOIN catalog.master_products mp ON mp.id = COALESCE(p.master_product_id, mv.master_product_id)` +
	catalogCategoryJoin

// catalogFilterConditions translates a ProductFilter into SQL predicates over
// the vertical-agnostic schema: typed attrs via mp.tier2 jsonb, categories via
// the master_categories junction. Returns the conditions and the grown
// args/argNum so caller can append ORDER/LIMIT params after.
func catalogFilterConditions(filter ports.ProductFilter, args []interface{}, argNum int) ([]string, []interface{}, int) {
	var conditions []string
	if filter.CategoryID != "" {
		conditions = append(conditions, fmt.Sprintf(
			"EXISTS (SELECT 1 FROM catalog.master_product_categories mpc WHERE mpc.master_product_id = mp.id AND mpc.master_category_id = $%d)", argNum))
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
		conditions = append(conditions, fmt.Sprintf(
			"EXISTS (SELECT 1 FROM catalog.master_product_categories mpc JOIN catalog.master_categories mc2 ON mc2.id = mpc.master_category_id WHERE mpc.master_product_id = mp.id AND (mc2.name ILIKE $%d OR mc2.slug ILIKE $%d))", argNum, argNum))
		args = append(args, "%"+filter.CategoryName+"%")
		argNum++
	}
	// Typed per-vertical attributes live in mp.tier2 (Group D). Scalars match
	// with ->>; arrays with @> to_jsonb(value).
	scalarAttrs := []struct {
		col string
		val string
	}{
		{"product_form", filter.ProductForm},
		{"routine_step", filter.RoutineStep},
		{"texture", filter.Texture},
	}
	for _, s := range scalarAttrs {
		if s.val != "" {
			conditions = append(conditions, fmt.Sprintf("mp.tier2->>'%s' = $%d", s.col, argNum))
			args = append(args, s.val)
			argNum++
		}
	}
	arrayAttrs := []struct {
		col string
		val string
	}{
		{"skin_type", filter.SkinType},
		{"concern", filter.Concern},
		{"key_ingredients", filter.KeyIngredient},
		{"target_area", filter.TargetArea},
	}
	for _, s := range arrayAttrs {
		if s.val != "" {
			conditions = append(conditions, fmt.Sprintf("mp.tier2->'%s' @> to_jsonb($%d::text)", s.col, argNum))
			args = append(args, s.val)
			argNum++
		}
	}
	return conditions, args, argNum
}

func (a *CatalogAdapter) ListProducts(ctx context.Context, tenantID string, filter ports.ProductFilter) (products []domain.Product, total int, err error) {
	var span *domain.SpanHandle
	if sc := domain.SpanFromContext(ctx); sc != nil {
		ctx, span = sc.StartSpan(ctx, "postgres.ListProducts")
		span.SetAttrs(map[string]any{
			"tenant_id":  tenantID,
			"limit":      filter.Limit,
			"has_search": filter.Search != "",
			"has_filter": filter.Brand != "" || filter.CategoryName != "" || filter.MinPrice > 0 || filter.MaxPrice > 0,
		})
		defer func() {
			if err != nil {
				span.SetError(err)
			} else {
				span.SetAttr("rows", len(products))
				span.SetAttr("total", total)
			}
			span.End()
		}()
	}
	baseQuery := catalogProductSelect + ` WHERE p.tenant_id = $1 AND p.deleted_at IS NULL`
	countQuery := `
		SELECT COUNT(*)
		FROM catalog.products p
		LEFT JOIN catalog.master_variants mv ON mv.id = p.master_variant_id
		LEFT JOIN catalog.master_products mp ON mp.id = COALESCE(p.master_product_id, mv.master_product_id)
		WHERE p.tenant_id = $1 AND p.deleted_at IS NULL
	`

	args := []interface{}{tenantID}
	argNum := 2
	conditions, args, argNum := catalogFilterConditions(filter, args, argNum)

	if len(conditions) > 0 {
		condStr := " AND " + strings.Join(conditions, " AND ")
		baseQuery += condStr
		countQuery += condStr
	}

	if err = a.client.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		if isUndefinedTable(err) {
			slog.WarnContext(ctx, "catalog.products/master tables absent; ListProducts degrading to empty", "tenant", tenantID)
			return nil, 0, nil
		}
		err = fmt.Errorf("count products: %w", err)
		return nil, 0, err
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
		if isUndefinedTable(err) {
			slog.WarnContext(ctx, "catalog.products/master tables absent; ListProducts degrading to empty", "tenant", tenantID)
			return nil, 0, nil
		}
		err = fmt.Errorf("query products: %w", err)
		return nil, 0, err
	}
	defer rows.Close()

	for rows.Next() {
		p, scanErr := scanCatalogProduct(rows)
		if scanErr != nil {
			err = scanErr
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
		if isUndefinedTable(err) {
			slog.WarnContext(ctx, "catalog.products/master tables absent; GetProduct → not found", "tenant", tenantID)
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
	var masterProductID, mpID, mpSKU, mpName, mpDesc, mpBrand, categoryName *string
	var productImagesJSON, tagsJSON, mpImagesJSON, extraJSON, mpTier2JSON []byte
	var mvSKU, mvSize, mvColor, mvImageURL *string
	var mvGTINs []string
	var mvWeightG, mvVolumeML *int

	err := row.Scan(
		&p.ID, &p.TenantID, &masterProductID, &p.MasterVariantID,
		&p.Name, &p.DisplayName, &p.OriginalName,
		&p.Description, &p.Price, &p.Currency, &p.StockQuantity, &p.Rating, &productImagesJSON, &tagsJSON,
		&mpID, &mpSKU, &mpName, &mpDesc,
		&mpBrand, &mpImagesJSON,
		&mvSKU, &mvGTINs, &mvSize, &mvColor, &mvImageURL, &mvWeightG, &mvVolumeML,
		&categoryName,
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

	// Group D: typed attributes are vertical-agnostic and live in tier2 jsonb
	// (master_cosmetics dropped). Populate the legacy cosmetics-shaped fields
	// from tier2 so domain.Product — and the pipeline/widget that consume it —
	// stay unchanged. Non-cosmetics verticals simply leave these empty.
	if len(mp.Tier2JSON) > 0 {
		_ = json.Unmarshal(mp.Tier2JSON, &p.Tier2)
	}
	p.ProductForm = tier2String(p.Tier2, "product_form")
	p.Texture = tier2String(p.Tier2, "texture")
	p.RoutineStep = tier2String(p.Tier2, "routine_step")
	p.MarketingClaim = tier2String(p.Tier2, "marketing_claim")
	p.SkinType = tier2Strings(p.Tier2, "skin_type")
	p.Concern = tier2Strings(p.Tier2, "concern")
	p.KeyIngredients = tier2Strings(p.Tier2, "key_ingredients")
	p.TargetArea = tier2Strings(p.Tier2, "target_area")
	p.Benefits = tier2Strings(p.Tier2, "benefits")
	return nil
}

// tier2String reads a scalar string attribute from the tier2 map ("" if absent).
func tier2String(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	if s, ok := m[key].(string); ok {
		return s
	}
	return ""
}

// tier2Strings reads an array-of-strings attribute from tier2 (nil if absent).
func tier2Strings(m map[string]any, key string) []string {
	if m == nil {
		return nil
	}
	raw, ok := m[key].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, e := range raw {
		if s, ok := e.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// formatPrice converts price-in-cents to a display string with thousand
// separators and a currency symbol. USD-only product decision (owner,
// 2026-07-27): anything not explicitly EUR renders as dollars — including
// legacy rows still stamped RUB.
func formatPrice(cents int, currency string) string {
	major := cents / 100
	str := fmt.Sprintf("%d", major)
	var result strings.Builder
	for i, c := range str {
		if i > 0 && (len(str)-i)%3 == 0 {
			result.WriteString(",")
		}
		result.WriteRune(c)
	}
	if currency == "EUR" {
		return "€" + result.String()
	}
	return "$" + result.String()
}

// BuildCatalogDigest assembles the per-tenant compact catalog digest used by
// Agent1's system prompt (~300-400 tokens once formatted via ToPromptText).
//
// Four queries:
//  1. category tree (joined with parent_id for grouping)
//  2. shared filters (UNION ALL across product_form / texture / routine_step
//     / unnest(skin_type|concern|key_ingredients|target_area))
//  3. top-30 brands by product count
//  4. top-30 ingredients by frequency (uses mp.key_ingredients array directly
//     — V5 doesn't depend on catalog.product_ingredients seeding)
//
// Builds the digest on demand; the use case caches it in process.
func (a *CatalogAdapter) BuildCatalogDigest(ctx context.Context, tenantID string) (digest *domain.CatalogDigest, err error) {
	var span *domain.SpanHandle
	if sc := domain.SpanFromContext(ctx); sc != nil {
		ctx, span = sc.StartSpan(ctx, "postgres.BuildCatalogDigest")
		span.SetAttr("tenant_id", tenantID)
		defer func() {
			if err != nil {
				span.SetError(err)
			} else if digest != nil {
				span.SetAttrs(map[string]any{
					"total_products":   digest.TotalProducts,
					"category_groups":  len(digest.CategoryTree),
					"shared_filters":   len(digest.SharedFilters),
					"top_brands":       len(digest.TopBrands),
					"top_ingredients":  len(digest.TopIngredients),
				})
			}
			span.End()
		}()
	}

	// 1. Category tree — via the master_categories junction (Group D:
	// catalog.categories + mp.category_id dropped).
	catQuery := `
		SELECT c.name, c.slug, COALESCE(pc.slug, '') AS parent_slug,
		       COUNT(DISTINCT mp.id) AS product_count
		FROM catalog.products p
		JOIN catalog.master_products mp ON p.master_product_id = mp.id
		JOIN catalog.master_product_categories mpc ON mpc.master_product_id = mp.id
		JOIN catalog.master_categories c ON c.id = mpc.master_category_id
		LEFT JOIN catalog.master_categories pc ON c.parent_id = pc.id
		WHERE p.tenant_id = $1 AND p.deleted_at IS NULL
		GROUP BY c.id, c.name, c.slug, pc.slug
		ORDER BY product_count DESC
	`
	catRows, err := a.client.pool.Query(ctx, catQuery, tenantID)
	if err != nil {
		if isUndefinedTable(err) {
			slog.WarnContext(ctx, "catalog tables absent; BuildCatalogDigest → empty digest", "tenant", tenantID)
			return &domain.CatalogDigest{GeneratedAt: time.Now(), TotalProducts: 0}, nil
		}
		return nil, fmt.Errorf("query categories: %w", err)
	}
	defer catRows.Close()

	type leafInfo struct {
		name, slug, parentSlug string
		count                  int
	}
	var leaves []leafInfo
	totalProducts := 0
	for catRows.Next() {
		var li leafInfo
		if err := catRows.Scan(&li.name, &li.slug, &li.parentSlug, &li.count); err != nil {
			return nil, fmt.Errorf("scan category: %w", err)
		}
		totalProducts += li.count
		leaves = append(leaves, li)
	}

	if len(leaves) == 0 {
		return &domain.CatalogDigest{GeneratedAt: time.Now(), TotalProducts: 0}, nil
	}

	// Group by parent slug; leaves without a parent become standalone groups.
	groupMap := make(map[string]*domain.DigestCategoryGroup)
	var groupOrder []string
	for _, li := range leaves {
		parent := li.parentSlug
		if parent == "" {
			parent = li.slug
		}
		if _, ok := groupMap[parent]; !ok {
			groupMap[parent] = &domain.DigestCategoryGroup{Slug: parent, Name: parent}
			groupOrder = append(groupOrder, parent)
		}
		groupMap[parent].Children = append(groupMap[parent].Children, domain.DigestCategoryLeaf{
			Name: li.name, Slug: li.slug, Count: li.count,
		})
	}
	tree := make([]domain.DigestCategoryGroup, 0, len(groupOrder))
	for _, slug := range groupOrder {
		tree = append(tree, *groupMap[slug])
	}

	// 2. Shared filters — global distinct values from tier2 jsonb (Group D:
	// master_cosmetics dropped; typed attrs are vertical-agnostic in tier2).
	// jsonb_array_elements_text is guarded by jsonb_typeof = 'array' so a
	// scalar/absent key never errors.
	filterQuery := `
		SELECT attr_key, ARRAY_AGG(DISTINCT attr_value ORDER BY attr_value) AS all_values
		FROM (
			SELECT 'product_form' AS attr_key, mp.tier2->>'product_form' AS attr_value
			FROM catalog.products p
			JOIN catalog.master_products mp ON p.master_product_id = mp.id
			WHERE p.tenant_id = $1 AND p.deleted_at IS NULL AND COALESCE(mp.tier2->>'product_form','') != ''
			UNION ALL
			SELECT 'texture', mp.tier2->>'texture'
			FROM catalog.products p
			JOIN catalog.master_products mp ON p.master_product_id = mp.id
			WHERE p.tenant_id = $1 AND p.deleted_at IS NULL AND COALESCE(mp.tier2->>'texture','') != ''
			UNION ALL
			SELECT 'routine_step', mp.tier2->>'routine_step'
			FROM catalog.products p
			JOIN catalog.master_products mp ON p.master_product_id = mp.id
			WHERE p.tenant_id = $1 AND p.deleted_at IS NULL AND COALESCE(mp.tier2->>'routine_step','') != ''
			UNION ALL
			SELECT 'skin_type', jsonb_array_elements_text(mp.tier2->'skin_type')
			FROM catalog.products p
			JOIN catalog.master_products mp ON p.master_product_id = mp.id
			WHERE p.tenant_id = $1 AND p.deleted_at IS NULL AND jsonb_typeof(mp.tier2->'skin_type') = 'array'
			UNION ALL
			SELECT 'concern', jsonb_array_elements_text(mp.tier2->'concern')
			FROM catalog.products p
			JOIN catalog.master_products mp ON p.master_product_id = mp.id
			WHERE p.tenant_id = $1 AND p.deleted_at IS NULL AND jsonb_typeof(mp.tier2->'concern') = 'array'
			UNION ALL
			SELECT 'key_ingredient', jsonb_array_elements_text(mp.tier2->'key_ingredients')
			FROM catalog.products p
			JOIN catalog.master_products mp ON p.master_product_id = mp.id
			WHERE p.tenant_id = $1 AND p.deleted_at IS NULL AND jsonb_typeof(mp.tier2->'key_ingredients') = 'array'
			UNION ALL
			SELECT 'target_area', jsonb_array_elements_text(mp.tier2->'target_area')
			FROM catalog.products p
			JOIN catalog.master_products mp ON p.master_product_id = mp.id
			WHERE p.tenant_id = $1 AND p.deleted_at IS NULL AND jsonb_typeof(mp.tier2->'target_area') = 'array'
		) AS attrs
		WHERE attr_value IS NOT NULL AND attr_value != ''
		GROUP BY attr_key
		ORDER BY attr_key
	`
	filterRows, err := a.client.pool.Query(ctx, filterQuery, tenantID)
	if err != nil {
		return nil, fmt.Errorf("query filters: %w", err)
	}
	defer filterRows.Close()

	var sharedFilters []domain.DigestSharedFilter
	for filterRows.Next() {
		var key string
		var values []string
		if err := filterRows.Scan(&key, &values); err != nil {
			return nil, fmt.Errorf("scan filter: %w", err)
		}
		sharedFilters = append(sharedFilters, domain.DigestSharedFilter{Key: key, Values: values})
	}

	// 3. Top brands.
	brandQuery := `
		SELECT mp.brand
		FROM catalog.products p
		JOIN catalog.master_products mp ON p.master_product_id = mp.id
		WHERE p.tenant_id = $1 AND p.deleted_at IS NULL AND mp.brand IS NOT NULL AND mp.brand != ''
		GROUP BY mp.brand
		ORDER BY COUNT(*) DESC
		LIMIT 30
	`
	brandRows, err := a.client.pool.Query(ctx, brandQuery, tenantID)
	if err != nil {
		return nil, fmt.Errorf("query brands: %w", err)
	}
	defer brandRows.Close()

	var topBrands []string
	for brandRows.Next() {
		var brand string
		if err := brandRows.Scan(&brand); err != nil {
			return nil, fmt.Errorf("scan brand: %w", err)
		}
		topBrands = append(topBrands, brand)
	}

	// 4. Top ingredients — from tier2.key_ingredients (Group D). Guarded by
	// jsonb_typeof so non-array/absent keys yield 0 rows → empty list.
	ingrQuery := `
		SELECT ingredient
		FROM (
			SELECT jsonb_array_elements_text(mp.tier2->'key_ingredients') AS ingredient
			FROM catalog.products p
			JOIN catalog.master_products mp ON p.master_product_id = mp.id
			WHERE p.tenant_id = $1 AND p.deleted_at IS NULL AND jsonb_typeof(mp.tier2->'key_ingredients') = 'array'
		) AS x
		WHERE ingredient IS NOT NULL AND ingredient != ''
		GROUP BY ingredient
		ORDER BY COUNT(*) DESC
		LIMIT 30
	`
	var topIngredients []string
	if ingrRows, err := a.client.pool.Query(ctx, ingrQuery, tenantID); err != nil {
		// Not fatal — digest is still useful without top ingredients.
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
