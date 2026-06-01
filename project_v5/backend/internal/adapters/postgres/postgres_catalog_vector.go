package postgres

import (
	"context"
	"fmt"
	"log/slog"

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
		JOIN catalog.master_products mp ON mp.id = COALESCE(p.master_product_id, mv.master_product_id)` +
		catalogCategoryJoin + `
		WHERE p.tenant_id = $1
		  AND p.deleted_at IS NULL
		  AND mp.embedding IS NOT NULL
	`

	args := []interface{}{tenantID, pgvector.NewVector(embedding)}
	argNum := 3

	if filter != nil {
		// Reuse the ProductFilter predicate builder (tier2/junction). Vector
		// search has no price range, so map only the shared narrowing fields.
		conds, newArgs, newNum := catalogFilterConditions(ports.ProductFilter{
			Brand: filter.Brand, CategoryName: filter.CategoryName,
			ProductForm: filter.ProductForm, SkinType: filter.SkinType,
			Concern: filter.Concern, KeyIngredient: filter.KeyIngredient,
			TargetArea: filter.TargetArea, RoutineStep: filter.RoutineStep,
			Texture: filter.Texture,
		}, args, argNum)
		for _, c := range conds {
			query += " AND " + c
		}
		args = newArgs
		argNum = newNum
	}

	query += fmt.Sprintf(" ORDER BY mp.embedding <=> $2 LIMIT $%d", argNum)
	args = append(args, limit)

	rows, err := a.client.pool.Query(ctx, query, args...)
	if err != nil {
		if isUndefinedTable(err) {
			slog.WarnContext(ctx, "catalog tables absent; VectorSearch degrading to empty", "tenant", tenantID)
			return nil, nil
		}
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

// SearchProjection ranks catalog.tenant_search_projection rows by a fused
// score — cosine similarity (when an embedding is supplied) plus full-text
// ts_rank — then hydrates display fields from the canonical tables via the
// shared scan path. This replaces the old keyword+vector+RRF pair and is the
// single read the catalog_search tool uses. embedding may be nil (FTS-only,
// graceful degradation when no OpenAI key). filter.Search is the FTS query;
// other filter fields narrow via tier2/junction predicates.
func (a *CatalogAdapter) SearchProjection(ctx context.Context, tenantID string, embedding []float32, filter ports.ProductFilter, limit int) ([]domain.Product, error) {
	if sc := domain.SpanFromContext(ctx); sc != nil {
		end := sc.Start("postgres.SearchProjection")
		defer end(fmt.Sprintf("tenant_id=%s limit=%d", tenantID, limit))
	}
	if limit <= 0 {
		limit = 20
	}

	var embArg interface{}
	if len(embedding) > 0 {
		embArg = pgvector.NewVector(embedding)
	}

	// $1 tenant, $2 embedding, $3 fts query; structured filter params from $4.
	args := []interface{}{tenantID, embArg, filter.Search}
	argNum := 4
	structFilter := filter
	structFilter.Search = "" // text handled by FTS, not ILIKE — avoid double-narrowing
	conds, args, argNum := catalogFilterConditions(structFilter, args, argNum)

	where := ""
	for _, c := range conds {
		where += " AND " + c
	}

	query := `
		WITH ranked AS (
			SELECT tsp.master_variant_id,
			       COALESCE(CASE WHEN $2::vector IS NOT NULL AND tsp.embedding IS NOT NULL
			                     THEN 1 - (tsp.embedding <=> $2::vector) END, 0)
			       + COALESCE(CASE WHEN $3 <> '' THEN ts_rank(tsp.search_tsv, plainto_tsquery('simple', $3)) END, 0) AS score
			FROM catalog.tenant_search_projection tsp
			JOIN catalog.products p ON p.tenant_id = tsp.tenant_id AND p.master_variant_id = tsp.master_variant_id AND p.deleted_at IS NULL
			LEFT JOIN catalog.master_products mp ON mp.id = tsp.master_product_id
			WHERE tsp.tenant_id = $1
			  AND ($2::vector IS NOT NULL OR $3 = '' OR tsp.search_tsv @@ plainto_tsquery('simple', $3))` + where + `
			ORDER BY score DESC
			LIMIT $` + fmt.Sprintf("%d", argNum) + `
		)` + catalogVectorSelect + `
		FROM ranked r
		JOIN catalog.products p ON p.tenant_id = $1 AND p.master_variant_id = r.master_variant_id AND p.deleted_at IS NULL
		LEFT JOIN catalog.master_variants mv ON mv.id = p.master_variant_id
		LEFT JOIN catalog.master_products mp ON mp.id = COALESCE(p.master_product_id, mv.master_product_id)` +
		catalogCategoryJoin + `
		ORDER BY r.score DESC`
	args = append(args, limit)

	rows, err := a.client.pool.Query(ctx, query, args...)
	if err != nil {
		if isUndefinedTable(err) {
			slog.WarnContext(ctx, "catalog tables absent; SearchProjection degrading to empty", "tenant", tenantID)
			return nil, nil
		}
		return nil, fmt.Errorf("search projection: %w", err)
	}
	defer rows.Close()

	var products []domain.Product
	for rows.Next() {
		p, err := scanCatalogProduct(rows)
		if err != nil {
			return nil, fmt.Errorf("scan projection product: %w", err)
		}
		p.PriceFormatted = formatPrice(p.Price, p.Currency)
		products = append(products, *p)
	}
	return products, rows.Err()
}

// catalogVectorSelect mirrors catalogProductSelect (the column list) but
// without the FROM/JOIN tail — VectorSearch builds those itself with INNER
// on master_products, while ListProducts uses LEFT. Keeping the SELECT clause
// in one place lets scanCatalogProduct stay the single source of scan truth.
// catalogVectorSelect is the shared SELECT column list scanned by
// scanCatalogProduct. Group D: vertical-agnostic — typed attrs come from
// mp.tier2 (master_cosmetics dropped), stock from p.stock_quantity (catalog.stock
// dropped), category name from the master_categories junction (categories +
// mp.category_id dropped). ListProducts/VectorSearch/SearchProjection all build
// the FROM tail themselves and reuse this list.
const catalogVectorSelect = `
	SELECT
		p.id, p.tenant_id,
		COALESCE(p.master_product_id::text, '') as master_product_id,
		COALESCE(p.master_variant_id::text, '') as master_variant_id,
		COALESCE(p.name, '') as name,
		COALESCE(p.display_name, '') as display_name,
		COALESCE(p.original_name, '') as original_name,
		COALESCE(p.description, '') as description,
		p.price, p.currency, COALESCE(p.stock_quantity, 0) as stock_quantity, COALESCE(p.rating, 0) as rating,
		COALESCE(p.images, '[]') as images, COALESCE(p.tags, '[]') as tags,
		mp.id as mp_id, mp.sku, mp.name as mp_name, mp.description as mp_description,
		mp.brand, mp.images as mp_images,
		mv.sku as mv_sku, mv.gtins as mv_gtins, mv.size as mv_size,
		mv.color as mv_color, mv.image_url as mv_image_url,
		mv.weight_g as mv_weight_g, mv.volume_ml as mv_volume_ml,
		cat.name as category_name,
		COALESCE(p.extra, '{}'::jsonb) as extra,
		COALESCE(mp.tier2, '{}'::jsonb) as mp_tier2
`
