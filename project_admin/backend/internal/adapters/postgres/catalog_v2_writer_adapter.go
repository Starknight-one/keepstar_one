// Package postgres — CatalogV2WriterAdapter implements ports.CatalogV2WriterPort.
// All upserts are idempotent on natural keys so re-running apply_v2 over the
// same inbox state converges.
//
// master_cosmetics schema awareness:
//   The current production table uses the legacy master_variant_id PK (0 rows).
//   The new shape uses master_product_id PK with 17+ typed cosmetic columns.
//   UpsertCosmetics probes for the master_product_id column once per process
//   start (sync.Once); on legacy shape it returns ErrCosmeticsSchemaNotReady
//   so apply_v2 falls back to writing into master_products.tier3.
package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/ports"
)

type CatalogV2WriterAdapter struct {
	client *Client
	log    *logger.Logger

	once             sync.Once
	cosmeticsReady   bool
	cosmeticsProbeErr error
}

func NewCatalogV2WriterAdapter(client *Client, log *logger.Logger) *CatalogV2WriterAdapter {
	return &CatalogV2WriterAdapter{client: client, log: log}
}

var _ ports.CatalogV2WriterPort = (*CatalogV2WriterAdapter)(nil)

// UpsertMaster writes one master_products row keyed by SKU.
//
// vertical is stamped on insert and overwritten on conflict — multi-tenant
// shares of the same master can disagree about vertical only across re-runs;
// last write wins which is fine because apply_v2 always passes the artifact
// vertical (consistent within a tenant).
//
// images is written as a one-element JSONB array containing image_url, when
// present. The full media gallery (if any) is responsibility of tier3.images.
func (a *CatalogV2WriterAdapter) UpsertMaster(ctx context.Context, mp *ports.MasterProductUpsert) (string, error) {
	if mp == nil {
		return "", fmt.Errorf("upsert master: nil input")
	}
	if mp.SKU == "" || mp.Name == "" {
		return "", fmt.Errorf("upsert master: sku and name required (sku=%q name=%q)", mp.SKU, mp.Name)
	}

	imagesJSON := []byte("[]")
	if mp.ImageURL != "" {
		b, _ := json.Marshal([]string{mp.ImageURL})
		imagesJSON = b
	}

	var id string
	err := a.client.pool.QueryRow(ctx, `
		INSERT INTO catalog.master_products
			(sku, name, brand, description, vertical, images, owner_tenant_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, NOW(), NOW())
		ON CONFLICT (sku) DO UPDATE
		SET name        = EXCLUDED.name,
		    brand       = COALESCE(NULLIF(EXCLUDED.brand, ''), master_products.brand),
		    description = COALESCE(NULLIF(EXCLUDED.description, ''), master_products.description),
		    vertical    = COALESCE(NULLIF(EXCLUDED.vertical, ''), master_products.vertical),
		    images      = CASE WHEN EXCLUDED.images::text = '[]' THEN master_products.images ELSE EXCLUDED.images END,
		    updated_at  = NOW()
		RETURNING id::text
	`, mp.SKU, mp.Name, mp.Brand, mp.Description, mp.Vertical, string(imagesJSON), nullableUUID(mp.OwnerTenantID)).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("upsert master exec: %w", err)
	}
	return id, nil
}

// UpsertCosmetics writes per-vertical cosmetics columns keyed by
// master_product_id. Returns ErrCosmeticsSchemaNotReady when the table
// still has the legacy master_variant_id PK shape (apply_v2 routes those
// fields into tier3 in that case).
func (a *CatalogV2WriterAdapter) UpsertCosmetics(ctx context.Context, masterID string, fields *ports.MasterCosmeticsUpsert) error {
	if masterID == "" || fields == nil {
		return fmt.Errorf("upsert cosmetics: master_id and fields required")
	}
	a.once.Do(func() {
		a.cosmeticsReady, a.cosmeticsProbeErr = a.probeCosmeticsSchema(ctx)
	})
	if a.cosmeticsProbeErr != nil {
		return fmt.Errorf("upsert cosmetics probe: %w", a.cosmeticsProbeErr)
	}
	if !a.cosmeticsReady {
		return ports.ErrCosmeticsSchemaNotReady
	}

	extraJSON := []byte("{}")
	if len(fields.Extra) > 0 {
		b, _ := json.Marshal(fields.Extra)
		extraJSON = b
	}

	_, err := a.client.pool.Exec(ctx, `
		INSERT INTO catalog.master_cosmetics (
			master_product_id, skin_type, concern, key_ingredients, target_area,
			product_form, texture, routine_step, routine_time, application_method,
			free_from, scent, spf, marketing_claim, benefits, how_to_use,
			volume_ml, weight_g, unit_count, extra, updated_at
		) VALUES (
			$1, COALESCE($2::text[], '{}'::text[]), COALESCE($3::text[], '{}'::text[]), COALESCE($4::text[], '{}'::text[]), COALESCE($5::text[], '{}'::text[]),
			$6, $7, $8, $9, $10,
			COALESCE($11::text[], '{}'::text[]), $12, $13, $14, COALESCE($15::text[], '{}'::text[]), $16,
			$17, $18, $19, $20::jsonb, NOW()
		)
		ON CONFLICT (master_product_id) DO UPDATE
		SET skin_type           = COALESCE(EXCLUDED.skin_type, master_cosmetics.skin_type),
		    concern             = COALESCE(EXCLUDED.concern, master_cosmetics.concern),
		    key_ingredients     = COALESCE(EXCLUDED.key_ingredients, master_cosmetics.key_ingredients),
		    target_area         = COALESCE(EXCLUDED.target_area, master_cosmetics.target_area),
		    product_form        = COALESCE(EXCLUDED.product_form, master_cosmetics.product_form),
		    texture             = COALESCE(EXCLUDED.texture, master_cosmetics.texture),
		    routine_step        = COALESCE(EXCLUDED.routine_step, master_cosmetics.routine_step),
		    routine_time        = COALESCE(EXCLUDED.routine_time, master_cosmetics.routine_time),
		    application_method  = COALESCE(EXCLUDED.application_method, master_cosmetics.application_method),
		    free_from           = COALESCE(EXCLUDED.free_from, master_cosmetics.free_from),
		    scent               = COALESCE(EXCLUDED.scent, master_cosmetics.scent),
		    spf                 = COALESCE(EXCLUDED.spf, master_cosmetics.spf),
		    marketing_claim     = COALESCE(EXCLUDED.marketing_claim, master_cosmetics.marketing_claim),
		    benefits            = COALESCE(EXCLUDED.benefits, master_cosmetics.benefits),
		    how_to_use          = COALESCE(EXCLUDED.how_to_use, master_cosmetics.how_to_use),
		    volume_ml           = COALESCE(EXCLUDED.volume_ml, master_cosmetics.volume_ml),
		    weight_g            = COALESCE(EXCLUDED.weight_g, master_cosmetics.weight_g),
		    unit_count          = COALESCE(EXCLUDED.unit_count, master_cosmetics.unit_count),
		    extra               = master_cosmetics.extra || EXCLUDED.extra,
		    updated_at          = NOW()
	`,
		masterID,
		stringSliceArg(fields.SkinType), stringSliceArg(fields.Concern),
		stringSliceArg(fields.KeyIngredients), stringSliceArg(fields.TargetArea),
		nullableStr(fields.ProductForm), nullableStr(fields.Texture),
		nullableStr(fields.RoutineStep), nullableStr(fields.RoutineTime),
		nullableStr(fields.ApplicationMethod),
		stringSliceArg(fields.FreeFrom),
		nullableStr(fields.Scent), nullableInt(fields.SPF),
		nullableStr(fields.MarketingClaim), stringSliceArg(fields.Benefits),
		nullableStr(fields.HowToUse),
		nullableInt(fields.VolumeML), nullableInt(fields.WeightG), nullableInt(fields.UnitCount),
		string(extraJSON),
	)
	if err != nil {
		return fmt.Errorf("upsert cosmetics exec: %w", err)
	}
	return nil
}

// MergeTier3 merges (top-level overwrite) a JSON patch into
// master_products.tier3. Existing keys not in patch survive.
func (a *CatalogV2WriterAdapter) MergeTier3(ctx context.Context, masterID string, patch map[string]any) error {
	if masterID == "" {
		return fmt.Errorf("merge tier3: empty master_id")
	}
	if len(patch) == 0 {
		return nil
	}
	body, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("merge tier3 marshal: %w", err)
	}
	_, err = a.client.pool.Exec(ctx, `
		UPDATE catalog.master_products
		SET tier3 = COALESCE(tier3, '{}'::jsonb) || $2::jsonb,
		    updated_at = NOW()
		WHERE id = $1
	`, masterID, string(body))
	if err != nil {
		return fmt.Errorf("merge tier3 exec: %w", err)
	}
	return nil
}

// UpsertListing writes a slim catalog.products row. Idempotency key:
// (tenant_id, master_product_id) — one listing per tenant per master.
//
// price is in cents; currency defaults to USD when empty. Legacy columns
// (name, description, images) are populated from related fields for
// backwards compat with code paths still reading them; once those code
// paths are deleted (admin catalog UI, V5 chat fallback) the writes here
// can be trimmed.
func (a *CatalogV2WriterAdapter) UpsertListing(ctx context.Context, lst *ports.ListingUpsert) (string, error) {
	if lst == nil || lst.TenantID == "" || lst.MasterProductID == "" {
		return "", fmt.Errorf("upsert listing: tenant_id and master_product_id required")
	}
	currency := lst.Currency
	if currency == "" {
		currency = "USD"
	}
	var id string
	err := a.client.pool.QueryRow(ctx, `
		INSERT INTO catalog.products
			(tenant_id, master_product_id, name, price, currency, stock_quantity,
			 source_system, source_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), NOW())
		ON CONFLICT (tenant_id, master_product_id) DO UPDATE
		SET name           = COALESCE(NULLIF(EXCLUDED.name, ''), products.name),
		    price          = EXCLUDED.price,
		    currency       = COALESCE(NULLIF(EXCLUDED.currency, ''), products.currency),
		    stock_quantity = EXCLUDED.stock_quantity,
		    source_system  = COALESCE(NULLIF(EXCLUDED.source_system, ''), products.source_system),
		    source_id      = COALESCE(NULLIF(EXCLUDED.source_id, ''), products.source_id),
		    updated_at     = NOW()
		RETURNING id::text
	`,
		lst.TenantID, lst.MasterProductID, lst.CustomTitle,
		lst.Price, currency, lst.Stock,
		lst.SourceSystem, lst.SourceID,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("upsert listing exec: %w", err)
	}
	return id, nil
}

// ----- internal helpers -----

// probeCosmeticsSchema returns (ready, err). ready=true when
// master_cosmetics has the master_product_id column (new shape).
func (a *CatalogV2WriterAdapter) probeCosmeticsSchema(ctx context.Context) (bool, error) {
	var exists bool
	err := a.client.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_schema = 'catalog'
			  AND table_name   = 'master_cosmetics'
			  AND column_name  = 'master_product_id'
		)
	`).Scan(&exists)
	return exists, err
}

// nullableStr returns *string for pgx (NULL when nil or empty).
func nullableStr(p *string) any {
	if p == nil || *p == "" {
		return nil
	}
	return *p
}

func nullableInt(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}

func nullableUUID(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// stringSliceArg returns nil for empty slices (pgx treats nil as NULL),
// otherwise the slice itself. The SQL uses COALESCE($n, '{}') so NULL
// gracefully maps to an empty Postgres text[] without overwriting the
// existing value on conflict update.
func stringSliceArg(s []string) any {
	if len(s) == 0 {
		return nil
	}
	return s
}
