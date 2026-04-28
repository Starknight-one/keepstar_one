// Package postgres — destructive write half of merge-apply (Phase D3,
// catalog completion 2026-04-28).
//
// Each method below performs one proposal's writes inside a single pgx
// transaction. The use case (MergeApplyUseCase.ApplyProposals) calls one
// method per proposal in a loop; partial-apply is natural because each
// proposal's tx commits or rolls back independently.
//
// We don't reuse the Upsert* / UpdateMasterProductPIM methods on
// CatalogAdapter / MasterVariantsAdapter because those open their own
// transactions through the pool. Composing them at the use-case layer would
// either leak pgx.Tx through the port boundary or sacrifice atomicity. So
// we re-implement the writes inline against a tx — the SQL is duplicated,
// the atomicity is preserved.
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/ports"
)

type MergeApplyTxAdapter struct {
	client *Client
	log    *logger.Logger
}

func NewMergeApplyTxAdapter(client *Client, log *logger.Logger) *MergeApplyTxAdapter {
	return &MergeApplyTxAdapter{client: client, log: log}
}

var _ ports.MergeApplyTxPort = (*MergeApplyTxAdapter)(nil)

// ApplyNewMaster inserts a fresh master_product + master_variant and links
// the listing to them. tx-atomic. Returns the new IDs.
//
// Note: master_products.sku has a UNIQUE constraint and an ON CONFLICT path
// in UpsertMasterProduct elsewhere. We mirror that here using ON CONFLICT
// DO UPDATE so re-applying a proposal whose SKU already exists upgrades the
// existing master rather than failing — the listing still gets linked. If
// the proposal carries no SKU at all we synthesize one from a UUID so the
// NOT NULL constraint passes (curator can rename later via UpdateProduct).
func (a *MergeApplyTxAdapter) ApplyNewMaster(ctx context.Context, listingID string, pm *domain.ProposedMaster) (string, string, error) {
	if listingID == "" {
		return "", "", errors.New("merge_apply: listing_id required")
	}
	if pm == nil {
		return "", "", errors.New("merge_apply: proposed_master required")
	}

	tx, err := a.client.pool.Begin(ctx)
	if err != nil {
		return "", "", fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Look up listing.tenant_id — owner_tenant_id on the new master should
	// match the source tenant (curator can later transfer ownership).
	var tenantID string
	if err := tx.QueryRow(ctx,
		`SELECT tenant_id::text FROM catalog.products WHERE id = $1`, listingID).Scan(&tenantID); err != nil {
		return "", "", fmt.Errorf("lookup listing tenant: %w", err)
	}

	sku := pm.Variant.SKU
	if sku == "" {
		// Synthesize unique sku — schema requires NOT NULL. Curator UI can
		// rename via UpdateProduct after apply.
		sku = "MM-" + uuid.NewString()
	}
	images := []string{}
	if pm.ImageURL != "" {
		images = []string{pm.ImageURL}
	}
	imagesJSON, _ := json.Marshal(images)
	tier2JSON, _ := json.Marshal(pm.Tier2)
	if len(pm.Tier2) == 0 {
		tier2JSON = []byte("{}")
	}
	vertical := pm.Vertical
	if vertical == "" {
		vertical = "unknown"
	}

	var masterProductID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO catalog.master_products
			(sku, name, description, brand, images, owner_tenant_id, vertical, tier2)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (sku) DO UPDATE SET
			name = EXCLUDED.name,
			brand = COALESCE(NULLIF(EXCLUDED.brand,''), catalog.master_products.brand),
			images = EXCLUDED.images,
			vertical = EXCLUDED.vertical,
			tier2 = catalog.master_products.tier2 || EXCLUDED.tier2,
			updated_at = NOW()
		RETURNING id`,
		sku, pm.Name, pm.Description, pm.Brand, imagesJSON, tenantID, vertical, tier2JSON,
	).Scan(&masterProductID); err != nil {
		return "", "", fmt.Errorf("insert master_product: %w", err)
	}

	// master_variant — primary variant for this master.
	axesJSON, _ := json.Marshal(pm.Variant.Axes)
	if pm.Variant.Axes == nil {
		axesJSON = []byte("{}")
	}
	gtins := pm.Variant.GTINs
	if gtins == nil {
		gtins = []string{}
	}
	var masterVariantID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO catalog.master_variants
			(master_product_id, sku, gtins, image_url,
			 weight_g, volume_ml, color, size, material, axes, variant_kind)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		RETURNING id`,
		masterProductID, sku, gtins, pm.ImageURL,
		pm.Variant.WeightG, pm.Variant.VolumeML,
		pm.Variant.Color, pm.Variant.Size, pm.Variant.Material,
		axesJSON, domain.VariantKindReal,
	).Scan(&masterVariantID); err != nil {
		return "", "", fmt.Errorf("insert master_variant: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE catalog.products
		SET master_product_id = $1, master_variant_id = $2, updated_at = NOW()
		WHERE id = $3`,
		masterProductID, masterVariantID, listingID); err != nil {
		return "", "", fmt.Errorf("link listing: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return "", "", fmt.Errorf("commit: %w", err)
	}
	return masterProductID, masterVariantID, nil
}

// ApplyLinkExisting updates the listing to point at an existing master
// product+variant. propagateToMaster carries field decisions whose action
// is "propagate_to_master" — we apply them as a tier2 JSONB merge on
// master_products in the same tx.
func (a *MergeApplyTxAdapter) ApplyLinkExisting(ctx context.Context, listingID, masterProductID, masterVariantID string, propagateToMaster []domain.FieldDecision) error {
	if listingID == "" || masterProductID == "" {
		return errors.New("merge_apply: listing_id + master_product_id required")
	}
	tx, err := a.client.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		UPDATE catalog.products
		SET master_product_id = $1::uuid, master_variant_id = NULLIF($2,'')::uuid, updated_at = NOW()
		WHERE id = $3::uuid`,
		masterProductID, masterVariantID, listingID); err != nil {
		return fmt.Errorf("link listing: %w", err)
	}

	// Apply tier2 propagations — merge each decided field's listing-side
	// value onto master_products.tier2 by jsonb concat. Field paths must
	// start with "tier2." for this to apply (everything else is a no-op
	// for the MVP — tier-1 promotion is curator-only).
	tier2Patch := map[string]interface{}{}
	for _, fd := range propagateToMaster {
		if fd.Action != "propagate_to_master" {
			continue
		}
		key := strings.TrimPrefix(strings.TrimPrefix(fd.Field, "master.tier2."), "tier2.")
		if key == "" || key == fd.Field {
			continue
		}
		v := fd.ProposedValue
		if v == nil {
			v = fd.ListingValue
		}
		if v == nil {
			continue
		}
		tier2Patch[key] = v
	}
	if len(tier2Patch) > 0 {
		patchJSON, _ := json.Marshal(tier2Patch)
		if _, err := tx.Exec(ctx, `
			UPDATE catalog.master_products
			SET tier2 = tier2 || $1::jsonb, updated_at = NOW()
			WHERE id = $2`,
			patchJSON, masterProductID); err != nil {
			return fmt.Errorf("propagate tier2: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// ApplyVariantOfExisting attaches a new master_variant to an existing
// master_product and links the listing to it.
func (a *MergeApplyTxAdapter) ApplyVariantOfExisting(ctx context.Context, listingID, parentMasterProductID string, pv *domain.ProposedVariant) (string, error) {
	if listingID == "" || parentMasterProductID == "" || pv == nil {
		return "", errors.New("merge_apply: listing_id + parent_master_product_id + proposed_variant required")
	}
	tx, err := a.client.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	sku := pv.SKU
	if sku == "" {
		sku = "MV-" + uuid.NewString()
	}
	axesJSON, _ := json.Marshal(pv.Axes)
	if pv.Axes == nil {
		axesJSON = []byte("{}")
	}
	gtins := pv.GTINs
	if gtins == nil {
		gtins = []string{}
	}
	var masterVariantID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO catalog.master_variants
			(master_product_id, sku, gtins, image_url,
			 weight_g, volume_ml, color, size, material, axes, variant_kind)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		RETURNING id`,
		parentMasterProductID, sku, gtins, pv.ImageURL,
		pv.WeightG, pv.VolumeML,
		pv.Color, pv.Size, pv.Material,
		axesJSON, domain.VariantKindReal,
	).Scan(&masterVariantID); err != nil {
		return "", fmt.Errorf("insert master_variant: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE catalog.products
		SET master_product_id = $1, master_variant_id = $2, updated_at = NOW()
		WHERE id = $3`,
		parentMasterProductID, masterVariantID, listingID); err != nil {
		return "", fmt.Errorf("link listing: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("commit: %w", err)
	}
	return masterVariantID, nil
}

// RestoreListingLink resets the listing's master_*_id to a previous state.
// Empty strings clear the FK columns. Single-statement, no tx wrap needed.
//
// SQL cast trickery: NULLIF($N,'')::uuid is required because pgx binds
// text-typed Go strings, but master_*_id is uuid. NULLIF + ::uuid lets
// us pass "" → NULL or a valid uuid string → uuid in the same parameter.
func (a *MergeApplyTxAdapter) RestoreListingLink(ctx context.Context, listingID, prevMasterProductID, prevMasterVariantID string) error {
	if listingID == "" {
		return errors.New("merge_apply: listing_id required")
	}
	_, err := a.client.pool.Exec(ctx, `
		UPDATE catalog.products
		SET master_product_id = NULLIF($1,'')::uuid,
		    master_variant_id = NULLIF($2,'')::uuid,
		    updated_at = NOW()
		WHERE id = $3::uuid`,
		prevMasterProductID, prevMasterVariantID, listingID)
	if err != nil {
		return fmt.Errorf("restore listing link: %w", err)
	}
	return nil
}
