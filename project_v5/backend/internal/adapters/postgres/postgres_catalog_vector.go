package postgres

import (
	"context"
	"fmt"

	"github.com/pgvector/pgvector-go"
	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/ports"
)

// VectorSearch finds products by pgvector cosine distance over
// master_products.embedding. Mirrors V4 (postgres_catalog.go:699-887): same
// JOINs, same INNER on master (semantic search requires an embedded master),
// same filter predicates, same scan path via scanCatalogProduct.
//
// filter may be nil for unfiltered search. Limit is required (caller passes
// from tool config). Embedding dimension must match the column type
// (vector(384) → 384 floats).
func (a *CatalogAdapter) VectorSearch(ctx context.Context, tenantID string, embedding []float32, limit int, filter *ports.VectorFilter) ([]domain.Product, error) {
	if sc := domain.SpanFromContext(ctx); sc != nil {
		end := sc.Start("postgres.VectorSearch")
		defer end(fmt.Sprintf("tenant_id=%s limit=%d", tenantID, limit))
	}

	// JOIN order intentionally differs from ListProducts: VectorSearch
	// requires an embedded master, so master_products is INNER, not LEFT.
	// Without the embedding NOT NULL guard pgvector would return rows with
	// undefined distances and corrupt the ranking.
	query := catalogVectorSelect + `
		FROM catalog.products p
		LEFT JOIN catalog.master_variants mv ON mv.id = p.master_variant_id
		JOIN catalog.master_products mp ON mp.id = COALESCE(p.master_product_id, mv.master_product_id)
		LEFT JOIN catalog.master_cosmetics mc ON mc.master_product_id = mp.id
		LEFT JOIN catalog.categories c ON mp.category_id = c.id
		LEFT JOIN catalog.stock s ON s.product_id = p.id AND s.tenant_id = p.tenant_id
		WHERE p.tenant_id = $1
		  AND p.deleted_at IS NULL
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
			query += fmt.Sprintf(" AND mc.product_form = $%d", argNum)
			args = append(args, filter.ProductForm)
			argNum++
		}
		if filter.SkinType != "" {
			query += fmt.Sprintf(" AND $%d = ANY(mc.skin_type)", argNum)
			args = append(args, filter.SkinType)
			argNum++
		}
		if filter.Concern != "" {
			query += fmt.Sprintf(" AND $%d = ANY(mc.concern)", argNum)
			args = append(args, filter.Concern)
			argNum++
		}
		if filter.RoutineStep != "" {
			query += fmt.Sprintf(" AND mc.routine_step = $%d", argNum)
			args = append(args, filter.RoutineStep)
			argNum++
		}
		if filter.Texture != "" {
			query += fmt.Sprintf(" AND mc.texture = $%d", argNum)
			args = append(args, filter.Texture)
			argNum++
		}
		if filter.KeyIngredient != "" {
			query += fmt.Sprintf(" AND $%d = ANY(mc.key_ingredients)", argNum)
			args = append(args, filter.KeyIngredient)
			argNum++
		}
		if filter.TargetArea != "" {
			query += fmt.Sprintf(" AND $%d = ANY(mc.target_area)", argNum)
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
		p, err := scanCatalogProduct(rows)
		if err != nil {
			return nil, fmt.Errorf("scan vector product: %w", err)
		}
		p.PriceFormatted = formatPrice(p.Price, p.Currency)
		products = append(products, *p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("vector rows: %w", err)
	}
	return products, nil
}

// catalogVectorSelect mirrors catalogProductSelect (the column list) but
// without the FROM/JOIN tail — VectorSearch builds those itself with INNER
// on master_products, while ListProducts uses LEFT. Keeping the SELECT clause
// in one place lets scanCatalogProduct stay the single source of scan truth.
const catalogVectorSelect = `
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
		COALESCE(mc.product_form, '') as product_form,
		COALESCE(mc.texture, '') as texture,
		COALESCE(mc.routine_step, '') as routine_step,
		COALESCE(mc.skin_type, '{}'::text[]) as skin_type,
		COALESCE(mc.concern, '{}'::text[]) as concern,
		COALESCE(mc.key_ingredients, '{}'::text[]) as key_ingredients,
		COALESCE(mc.target_area, '{}'::text[]) as target_area,
		COALESCE(mc.marketing_claim, '') as marketing_claim,
		COALESCE(mc.benefits, '{}'::text[]) as benefits,
		COALESCE(p.extra, '{}'::jsonb) as extra,
		COALESCE(mp.tier3, mp.tier2, '{}'::jsonb) as mp_tier2
`
