// Package usecases — wipe integration data (Phase A.0, 2026-04-29).
//
// "Disconnect" only drops the OAuth row in admin.tenant_integrations. That
// works for "I don't want to sync any more" but leaves catalog.products /
// merge_reports / discovery artifacts behind, so a subsequent re-install of
// the same Shopify store inherits stale state and the merge agent sees
// already_linked rows that don't actually exist on the source any more.
//
// WipeAndDisconnect is the destructive version: deletes all tenant-scoped
// catalog state in one transaction, then drops the OAuth row. Master
// products are intentionally NOT deleted — they may be linked by other
// tenants (heybabes is a seed catalogue), and orphan masters can be cleaned
// up separately. Audit log is also preserved (history > convenience).
package usecases

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/ports"
)

type IntegrationsWipeUseCase struct {
	pool         *pgxpool.Pool
	integrations ports.IntegrationsPort
	log          *logger.Logger
}

func NewIntegrationsWipeUseCase(pool *pgxpool.Pool, integrations ports.IntegrationsPort, log *logger.Logger) *IntegrationsWipeUseCase {
	return &IntegrationsWipeUseCase{pool: pool, integrations: integrations, log: log}
}

// WipeResult counts what we deleted, for the curator UI to show "wiped X
// listings, Y reports, Z categories" instead of a blank ack.
type WipeResult struct {
	IntegrationID  string `json:"integrationId"`
	TenantID       string `json:"tenantId"`
	Listings       int64  `json:"listings"`
	RawImports     int64  `json:"rawImports"`
	MergeReports   int64  `json:"mergeReports"`
	Categories     int64  `json:"categories"`
	SchemaArtifact bool   `json:"schemaArtifact"`
	IntegrationGone bool  `json:"integrationGone"`
}

// WipeAndDisconnect deletes every tenant-scoped catalog row tied to this
// integration in a single transaction, then deletes the integration row
// itself. Returns counts for UI confirmation.
//
// What we preserve:
//   - master_products / master_variants (may be referenced by other tenants)
//   - master_attribute_candidates (global pool, dedup'd by (key, vertical))
//   - audit_log (history)
//
// What we delete (filtered by tenant_id, all in one tx):
//   - catalog.merge_reports
//   - catalog.products WHERE source_system = integration.kind
//   - catalog.shopify_raw_imports (if shopify integration)
//   - catalog.tenant_categories
//   - catalog.tenant_catalog_schema (mapping_artifact)
//   - admin.tenant_integrations (the OAuth row itself)
func (uc *IntegrationsWipeUseCase) WipeAndDisconnect(ctx context.Context, tenantID, integrationID string) (*WipeResult, error) {
	if tenantID == "" || integrationID == "" {
		return nil, errors.New("wipe: tenant_id + integration_id required")
	}

	// Resolve integration first so we know its kind (filters the catalog.products
	// delete to "shopify" rows only — CSV-imported rows live in the same table
	// with source_system='csv' and shouldn't be wiped by a Shopify disconnect).
	integ, err := uc.integrations.Get(ctx, tenantID, integrationID)
	if err != nil {
		return nil, fmt.Errorf("load integration: %w", err)
	}
	if integ == nil {
		return nil, errors.New("wipe: integration not found")
	}

	res := &WipeResult{IntegrationID: integrationID, TenantID: tenantID}

	tx, err := uc.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// 1. merge_reports — tenant-keyed, JSONB proposals deleted with row.
	tag, err := tx.Exec(ctx, `DELETE FROM catalog.merge_reports WHERE tenant_id = $1`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("delete merge_reports: %w", err)
	}
	res.MergeReports = tag.RowsAffected()

	// 2. catalog.products — only this integration's source.
	sourceSystem := string(integ.Kind)
	tag, err = tx.Exec(ctx, `DELETE FROM catalog.products WHERE tenant_id = $1 AND source_system = $2`, tenantID, sourceSystem)
	if err != nil {
		return nil, fmt.Errorf("delete products: %w", err)
	}
	res.Listings = tag.RowsAffected()

	// 3. shopify_raw_imports — only relevant for shopify integrations.
	if integ.Kind == domain.IntegrationKindShopify {
		tag, err = tx.Exec(ctx, `DELETE FROM catalog.shopify_raw_imports WHERE tenant_id = $1`, tenantID)
		if err != nil {
			return nil, fmt.Errorf("delete shopify_raw_imports: %w", err)
		}
		res.RawImports = tag.RowsAffected()
	}

	// 4. tenant_categories.
	tag, err = tx.Exec(ctx, `DELETE FROM catalog.tenant_categories WHERE tenant_id = $1`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("delete tenant_categories: %w", err)
	}
	res.Categories = tag.RowsAffected()

	// 5. tenant_catalog_schema (mapping_artifact + validation_report).
	tag, err = tx.Exec(ctx, `DELETE FROM catalog.tenant_catalog_schema WHERE tenant_id = $1`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("delete tenant_catalog_schema: %w", err)
	}
	res.SchemaArtifact = tag.RowsAffected() > 0

	// 6. admin.tenant_integrations — the OAuth row itself.
	tag, err = tx.Exec(ctx, `DELETE FROM admin.tenant_integrations WHERE id = $1 AND tenant_id = $2`, integrationID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("delete tenant_integration: %w", err)
	}
	res.IntegrationGone = tag.RowsAffected() > 0

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	uc.log.Info("integration_wiped",
		"tenant_id", tenantID, "integration_id", integrationID, "kind", string(integ.Kind),
		"listings", res.Listings, "raw_imports", res.RawImports,
		"merge_reports", res.MergeReports, "categories", res.Categories,
		"schema", res.SchemaArtifact)

	return res, nil
}
