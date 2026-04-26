// Package postgres — master_variants + master_cosmetics adapter.
// Spec: docs/New features/admin_catalog_design_2026-04-23.md §3.1.
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

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
