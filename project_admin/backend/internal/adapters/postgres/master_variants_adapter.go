// Package postgres — master_variants + master_cosmetics adapter.
// Spec: docs/New features/admin_catalog_design_2026-04-23.md §3.1.
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"
	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/ports"
)

type MasterVariantsAdapter struct {
	client *Client
	log    *logger.Logger
}

func NewMasterVariantsAdapter(client *Client, log *logger.Logger) *MasterVariantsAdapter {
	return &MasterVariantsAdapter{client: client, log: log}
}

// Compile-time interface conformance.
var _ ports.MasterVariantsPort = (*MasterVariantsAdapter)(nil)

// UpsertMasterVariant inserts when sku is empty or no match exists, otherwise
// updates by (master_product_id, sku). Caller is responsible for setting
// master_product_id.
func (a *MasterVariantsAdapter) UpsertMasterVariant(ctx context.Context, mv *domain.MasterVariant) (string, error) {
	if mv.MasterProductID == "" {
		return "", errors.New("master_variants: master_product_id required")
	}
	axesJSON, _ := json.Marshal(mv.Axes)
	parseStatusJSON, _ := json.Marshal(mv.ParseStatus)
	if mv.Axes == nil {
		axesJSON = []byte("{}")
	}
	if mv.ParseStatus == nil {
		parseStatusJSON = []byte("{}")
	}
	if mv.GTINs == nil {
		mv.GTINs = []string{}
	}
	if mv.VariantKind == "" {
		mv.VariantKind = domain.VariantKindReal
	}

	// Try to find existing by (master_product_id, sku) when sku non-empty.
	if mv.SKU != "" {
		var existing string
		err := a.client.pool.QueryRow(ctx, `
			SELECT id FROM catalog.master_variants
			WHERE master_product_id = $1 AND sku = $2 LIMIT 1`,
			mv.MasterProductID, mv.SKU).Scan(&existing)
		if err == nil {
			// Update existing.
			_, err := a.client.pool.Exec(ctx, `
				UPDATE catalog.master_variants SET
					gtins = $1, image_url = $2, weight_g = $3, volume_ml = $4,
					length_mm = $5, width_mm = $6, height_mm = $7,
					color = $8, size = $9, material = $10,
					axes = $11, variant_kind = $12,
					weight_raw = $13, volume_raw = $14, parse_status = $15,
					updated_at = NOW()
				WHERE id = $16`,
				mv.GTINs, mv.ImageURL, mv.WeightG, mv.VolumeML,
				mv.LengthMM, mv.WidthMM, mv.HeightMM,
				mv.Color, mv.Size, mv.Material,
				axesJSON, mv.VariantKind,
				mv.WeightRaw, mv.VolumeRaw, parseStatusJSON,
				existing)
			if err != nil {
				return "", fmt.Errorf("update master_variant: %w", err)
			}
			return existing, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return "", fmt.Errorf("lookup master_variant: %w", err)
		}
	}

	// Insert new.
	var id string
	err := a.client.pool.QueryRow(ctx, `
		INSERT INTO catalog.master_variants (
			master_product_id, sku, gtins, image_url,
			weight_g, volume_ml, length_mm, width_mm, height_mm,
			color, size, material, axes, variant_kind,
			weight_raw, volume_raw, parse_status
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)
		RETURNING id`,
		mv.MasterProductID, mv.SKU, mv.GTINs, mv.ImageURL,
		mv.WeightG, mv.VolumeML, mv.LengthMM, mv.WidthMM, mv.HeightMM,
		mv.Color, mv.Size, mv.Material, axesJSON, mv.VariantKind,
		mv.WeightRaw, mv.VolumeRaw, parseStatusJSON,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("insert master_variant: %w", err)
	}
	return id, nil
}

func (a *MasterVariantsAdapter) GetMasterVariant(ctx context.Context, id string) (*domain.MasterVariant, error) {
	var mv domain.MasterVariant
	var axes, parseStatus []byte
	err := a.client.pool.QueryRow(ctx, `
		SELECT id, master_product_id, sku, gtins, image_url,
			weight_g, volume_ml, length_mm, width_mm, height_mm,
			color, size, material, axes, variant_kind,
			weight_raw, volume_raw, parse_status, created_at, updated_at
		FROM catalog.master_variants WHERE id = $1`, id).Scan(
		&mv.ID, &mv.MasterProductID, &mv.SKU, &mv.GTINs, &mv.ImageURL,
		&mv.WeightG, &mv.VolumeML, &mv.LengthMM, &mv.WidthMM, &mv.HeightMM,
		&mv.Color, &mv.Size, &mv.Material, &axes, &mv.VariantKind,
		&mv.WeightRaw, &mv.VolumeRaw, &parseStatus, &mv.CreatedAt, &mv.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get master_variant: %w", err)
	}
	_ = json.Unmarshal(axes, &mv.Axes)
	_ = json.Unmarshal(parseStatus, &mv.ParseStatus)
	return &mv, nil
}

func (a *MasterVariantsAdapter) ListMasterVariants(ctx context.Context, masterProductID string) ([]domain.MasterVariant, error) {
	rows, err := a.client.pool.Query(ctx, `
		SELECT id, master_product_id, sku, gtins, image_url,
			weight_g, volume_ml, length_mm, width_mm, height_mm,
			color, size, material, axes, variant_kind,
			weight_raw, volume_raw, parse_status, created_at, updated_at
		FROM catalog.master_variants WHERE master_product_id = $1
		ORDER BY created_at`, masterProductID)
	if err != nil {
		return nil, fmt.Errorf("list master_variants: %w", err)
	}
	defer rows.Close()
	return scanVariants(rows)
}

// FindByGTIN uses Postgres array overlap (`gtins && $1`). Spec §4.6 step 1.
func (a *MasterVariantsAdapter) FindByGTIN(ctx context.Context, gtins []string) ([]domain.MasterVariant, error) {
	if len(gtins) == 0 {
		return nil, nil
	}
	rows, err := a.client.pool.Query(ctx, `
		SELECT id, master_product_id, sku, gtins, image_url,
			weight_g, volume_ml, length_mm, width_mm, height_mm,
			color, size, material, axes, variant_kind,
			weight_raw, volume_raw, parse_status, created_at, updated_at
		FROM catalog.master_variants WHERE gtins && $1`, gtins)
	if err != nil {
		return nil, fmt.Errorf("find by gtin: %w", err)
	}
	defer rows.Close()
	return scanVariants(rows)
}

// FindByVendorAndSKU joins master_products to filter by brand. Case-insensitive
// on brand match. Spec §4.6 step 2.
func (a *MasterVariantsAdapter) FindByVendorAndSKU(ctx context.Context, vendor, sku string) ([]domain.MasterVariant, error) {
	if vendor == "" || sku == "" {
		return nil, nil
	}
	rows, err := a.client.pool.Query(ctx, `
		SELECT mv.id, mv.master_product_id, mv.sku, mv.gtins, mv.image_url,
			mv.weight_g, mv.volume_ml, mv.length_mm, mv.width_mm, mv.height_mm,
			mv.color, mv.size, mv.material, mv.axes, mv.variant_kind,
			mv.weight_raw, mv.volume_raw, mv.parse_status, mv.created_at, mv.updated_at
		FROM catalog.master_variants mv
		JOIN catalog.master_products mp ON mp.id = mv.master_product_id
		WHERE mp.brand ILIKE $1 AND mv.sku = $2`, vendor, sku)
	if err != nil {
		return nil, fmt.Errorf("find by vendor+sku: %w", err)
	}
	defer rows.Close()
	return scanVariants(rows)
}

// FindByVendorAndFuzzyName runs a trigram similarity scan over master_products
// for products whose brand matches the vendor (ILIKE), returns variants of
// matches whose product name's trigram similarity to title is >= threshold.
// Spec §4.6 step 3.
//
// Uses pg_trgm's similarity() function. Index `idx_catalog_mp_name_trgm`
// keeps this acceptable at scale; without it this would be a full scan.
func (a *MasterVariantsAdapter) FindByVendorAndFuzzyName(ctx context.Context, vendor, title string, threshold float64, limit int) ([]ports.MasterVariantWithScore, error) {
	if vendor == "" || title == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 10
	}
	if threshold <= 0 {
		threshold = 0.85
	}
	rows, err := a.client.pool.Query(ctx, `
		SELECT mv.id, mv.master_product_id, mv.sku, mv.gtins, mv.image_url,
			mv.weight_g, mv.volume_ml, mv.length_mm, mv.width_mm, mv.height_mm,
			mv.color, mv.size, mv.material, mv.axes, mv.variant_kind,
			mv.weight_raw, mv.volume_raw, mv.parse_status, mv.created_at, mv.updated_at,
			similarity(mp.name, $2)::float8 AS sim
		FROM catalog.master_variants mv
		JOIN catalog.master_products mp ON mp.id = mv.master_product_id
		WHERE mp.brand ILIKE $1
		  AND similarity(mp.name, $2) >= $3
		ORDER BY sim DESC
		LIMIT $4`, vendor, title, threshold, limit)
	if err != nil {
		return nil, fmt.Errorf("fuzzy match: %w", err)
	}
	defer rows.Close()
	return scanVariantsWithScore(rows)
}

// FindByEmbedding runs a cosine-distance vector search against
// master_variants.embedding. Returns variants whose distance is <= (1 -
// threshold). Caller passes threshold as a "higher is more similar" value
// (e.g. 0.92), and we convert to distance internally to keep call-sites
// readable. Spec §4.6 step 4.
//
// Returns empty when embedding is nil or empty.
func (a *MasterVariantsAdapter) FindByEmbedding(ctx context.Context, embedding []float32, threshold float64, limit int) ([]ports.MasterVariantWithScore, error) {
	if len(embedding) == 0 {
		return nil, nil
	}
	if limit <= 0 {
		limit = 5
	}
	if threshold <= 0 {
		threshold = 0.92
	}
	maxDistance := 1 - threshold
	// Format the vector as a Postgres pgvector literal '[v1,v2,...]'.
	vecLit := pgvectorLiteral(embedding)
	rows, err := a.client.pool.Query(ctx, `
		SELECT mv.id, mv.master_product_id, mv.sku, mv.gtins, mv.image_url,
			mv.weight_g, mv.volume_ml, mv.length_mm, mv.width_mm, mv.height_mm,
			mv.color, mv.size, mv.material, mv.axes, mv.variant_kind,
			mv.weight_raw, mv.volume_raw, mv.parse_status, mv.created_at, mv.updated_at,
			(1 - (mv.embedding <=> $1::vector))::float8 AS sim
		FROM catalog.master_variants mv
		WHERE mv.embedding IS NOT NULL
		  AND (mv.embedding <=> $1::vector) <= $2
		ORDER BY mv.embedding <=> $1::vector ASC
		LIMIT $3`, vecLit, maxDistance, limit)
	if err != nil {
		return nil, fmt.Errorf("embedding match: %w", err)
	}
	defer rows.Close()
	return scanVariantsWithScore(rows)
}

func scanVariantsWithScore(rows pgx.Rows) ([]ports.MasterVariantWithScore, error) {
	var out []ports.MasterVariantWithScore
	for rows.Next() {
		var mv domain.MasterVariant
		var axes, parseStatus []byte
		var score float64
		if err := rows.Scan(
			&mv.ID, &mv.MasterProductID, &mv.SKU, &mv.GTINs, &mv.ImageURL,
			&mv.WeightG, &mv.VolumeML, &mv.LengthMM, &mv.WidthMM, &mv.HeightMM,
			&mv.Color, &mv.Size, &mv.Material, &axes, &mv.VariantKind,
			&mv.WeightRaw, &mv.VolumeRaw, &parseStatus, &mv.CreatedAt, &mv.UpdatedAt,
			&score,
		); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(axes, &mv.Axes)
		_ = json.Unmarshal(parseStatus, &mv.ParseStatus)
		out = append(out, ports.MasterVariantWithScore{
			MasterVariant: mv,
			MatchedScore:  score,
		})
	}
	return out, rows.Err()
}

// pgvectorLiteral renders a float32 slice as the pgvector textual format
// '[v1,v2,...]'. Avoids pulling in pgx/v5/pgvector subpackage just for this.
func pgvectorLiteral(v []float32) string {
	if len(v) == 0 {
		return "[]"
	}
	var b []byte
	b = append(b, '[')
	for i, f := range v {
		if i > 0 {
			b = append(b, ',')
		}
		b = appendFloat32(b, f)
	}
	b = append(b, ']')
	return string(b)
}

func appendFloat32(b []byte, f float32) []byte {
	// strconv.AppendFloat with -1 precision is the canonical form pgvector
	// expects; bitSize 32 keeps trailing-digit noise in check.
	return strconv.AppendFloat(b, float64(f), 'f', -1, 32)
}

func scanVariants(rows pgx.Rows) ([]domain.MasterVariant, error) {
	var out []domain.MasterVariant
	for rows.Next() {
		var mv domain.MasterVariant
		var axes, parseStatus []byte
		if err := rows.Scan(
			&mv.ID, &mv.MasterProductID, &mv.SKU, &mv.GTINs, &mv.ImageURL,
			&mv.WeightG, &mv.VolumeML, &mv.LengthMM, &mv.WidthMM, &mv.HeightMM,
			&mv.Color, &mv.Size, &mv.Material, &axes, &mv.VariantKind,
			&mv.WeightRaw, &mv.VolumeRaw, &parseStatus, &mv.CreatedAt, &mv.UpdatedAt,
		); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(axes, &mv.Axes)
		_ = json.Unmarshal(parseStatus, &mv.ParseStatus)
		out = append(out, mv)
	}
	return out, rows.Err()
}

// UpsertMasterCosmetics writes the Tier 2 cosmetics row for a variant.
func (a *MasterVariantsAdapter) UpsertMasterCosmetics(ctx context.Context, mc *domain.MasterCosmetics) error {
	if mc.MasterVariantID == "" {
		return errors.New("master_cosmetics: master_variant_id required")
	}
	if mc.SkinType == nil {
		mc.SkinType = []string{}
	}
	if mc.Concern == nil {
		mc.Concern = []string{}
	}
	if mc.Ingredients == nil {
		mc.Ingredients = []string{}
	}
	_, err := a.client.pool.Exec(ctx, `
		INSERT INTO catalog.master_cosmetics (master_variant_id, skin_type, concern, ingredients, scent, spf)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (master_variant_id) DO UPDATE SET
			skin_type = EXCLUDED.skin_type,
			concern = EXCLUDED.concern,
			ingredients = EXCLUDED.ingredients,
			scent = EXCLUDED.scent,
			spf = EXCLUDED.spf,
			updated_at = NOW()`,
		mc.MasterVariantID, mc.SkinType, mc.Concern, mc.Ingredients, mc.Scent, mc.SPF)
	if err != nil {
		return fmt.Errorf("upsert master_cosmetics: %w", err)
	}
	return nil
}

// FindMasterProductsByEmbedding cosine-searches master_products.embedding.
// Optional vertical filter narrows scope; empty = search all. Limit is
// capped at 50 to keep response size bounded for the discovery agent
// (which sees the result list as a single tool response).
func (a *MasterVariantsAdapter) FindMasterProductsByEmbedding(ctx context.Context, embedding []float32, vertical string, limit int) ([]ports.MasterProductSummary, error) {
	if len(embedding) == 0 {
		return nil, nil
	}
	if limit <= 0 || limit > 50 {
		limit = 5
	}
	vecLit := pgvectorLiteral(embedding)
	args := []any{vecLit, limit}
	verticalClause := ""
	if vertical != "" {
		verticalClause = "AND mp.vertical = $3"
		args = append(args, vertical)
	}
	q := `
		SELECT mp.id, mp.name, COALESCE(mp.brand, ''), mp.vertical, mp.confidence,
			COALESCE(mp.owner_tenant_id::text, ''),
			(1 - (mp.embedding <=> $1::vector))::float8 AS sim
		FROM catalog.master_products mp
		WHERE mp.embedding IS NOT NULL ` + verticalClause + `
		ORDER BY mp.embedding <=> $1::vector ASC
		LIMIT $2`
	rows, err := a.client.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("master products embedding search: %w", err)
	}
	defer rows.Close()
	out := make([]ports.MasterProductSummary, 0, limit)
	for rows.Next() {
		var s ports.MasterProductSummary
		if err := rows.Scan(&s.ID, &s.Name, &s.Brand, &s.Vertical, &s.Confidence, &s.OwnerTenantID, &s.Score); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// GetMasterProductSummary returns one master + its variants in the
// discovery-agent-facing summary shape. Variants are capped at 20 (rare to
// have more on a single master, and the agent only needs to recognize the
// shape — not enumerate every SKU).
func (a *MasterVariantsAdapter) GetMasterProductSummary(ctx context.Context, masterProductID string) (*ports.MasterProductSummary, error) {
	var s ports.MasterProductSummary
	err := a.client.pool.QueryRow(ctx, `
		SELECT mp.id, mp.name, COALESCE(mp.brand, ''), mp.vertical, mp.confidence,
			COALESCE(mp.owner_tenant_id::text, '')
		FROM catalog.master_products mp WHERE mp.id = $1`, masterProductID).Scan(
		&s.ID, &s.Name, &s.Brand, &s.Vertical, &s.Confidence, &s.OwnerTenantID,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get master product summary: %w", err)
	}

	// Variants snippet — minimum the agent needs to recognize variant shape.
	rows, err := a.client.pool.Query(ctx, `
		SELECT id, COALESCE(sku, ''), axes,
			COALESCE(array_length(gtins, 1), 0) > 0 AS has_gtin,
			variant_kind
		FROM catalog.master_variants
		WHERE master_product_id = $1
		ORDER BY created_at LIMIT 20`, masterProductID)
	if err != nil {
		return nil, fmt.Errorf("get variant snippets: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var snip ports.MasterVariantSnippet
		var axes []byte
		if err := rows.Scan(&snip.ID, &snip.SKU, &axes, &snip.HasGTIN, &snip.VariantKind); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(axes, &snip.Axes)
		s.Variants = append(s.Variants, snip)
	}
	return &s, rows.Err()
}

func (a *MasterVariantsAdapter) GetMasterCosmetics(ctx context.Context, masterVariantID string) (*domain.MasterCosmetics, error) {
	var mc domain.MasterCosmetics
	err := a.client.pool.QueryRow(ctx, `
		SELECT master_variant_id, skin_type, concern, ingredients, scent, spf, updated_at
		FROM catalog.master_cosmetics WHERE master_variant_id = $1`, masterVariantID).Scan(
		&mc.MasterVariantID, &mc.SkinType, &mc.Concern, &mc.Ingredients, &mc.Scent, &mc.SPF, &mc.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get master_cosmetics: %w", err)
	}
	return &mc, nil
}
