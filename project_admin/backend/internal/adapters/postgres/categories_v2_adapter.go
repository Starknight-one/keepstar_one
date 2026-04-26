// Package postgres — master/tenant categories with M:N joins.
// Spec: docs/New features/admin_catalog_design_2026-04-23.md §3.2.
//
// Naming: this file is "_v2" because the legacy CatalogAdapter has a flat
// GetCategories method on the older catalog.categories table. The new spec
// uses catalog.master_categories + catalog.tenant_categories which are
// distinct tables with hierarchy and M:N joins.
package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/ports"
)

type CategoriesV2Adapter struct {
	client *Client
	log    *logger.Logger
}

func NewCategoriesV2Adapter(client *Client, log *logger.Logger) *CategoriesV2Adapter {
	return &CategoriesV2Adapter{client: client, log: log}
}

var _ ports.CategoriesPort = (*CategoriesV2Adapter)(nil)

// --- Master categories ---

// UpsertMasterCategory inserts new or updates by slug.
func (a *CategoriesV2Adapter) UpsertMasterCategory(ctx context.Context, mc *domain.MasterCategory) (string, error) {
	if mc.Slug == "" || mc.Name == "" || mc.Vertical == "" {
		return "", errors.New("master_category: slug + name + vertical required")
	}
	var parentArg interface{}
	if mc.ParentID != "" {
		parentArg = mc.ParentID
	}
	var id string
	err := a.client.pool.QueryRow(ctx, `
		INSERT INTO catalog.master_categories (parent_id, slug, name, vertical)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (slug) DO UPDATE SET
			parent_id = EXCLUDED.parent_id,
			name = EXCLUDED.name,
			vertical = EXCLUDED.vertical
		RETURNING id`,
		parentArg, mc.Slug, mc.Name, mc.Vertical).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("upsert master_category: %w", err)
	}
	return id, nil
}

func (a *CategoriesV2Adapter) ListMasterCategories(ctx context.Context, vertical string) ([]domain.MasterCategory, error) {
	rows, err := a.client.pool.Query(ctx, `
		SELECT id, COALESCE(parent_id::text, ''), slug, name, vertical, created_at
		FROM catalog.master_categories
		WHERE ($1 = '' OR vertical = $1)
		ORDER BY name`, vertical)
	if err != nil {
		return nil, fmt.Errorf("list master_categories: %w", err)
	}
	defer rows.Close()
	var out []domain.MasterCategory
	for rows.Next() {
		var c domain.MasterCategory
		if err := rows.Scan(&c.ID, &c.ParentID, &c.Slug, &c.Name, &c.Vertical, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// --- Tenant categories ---

func (a *CategoriesV2Adapter) UpsertTenantCategory(ctx context.Context, tc *domain.TenantCategory) (string, error) {
	if tc.TenantID == "" || tc.Name == "" || tc.Slug == "" {
		return "", errors.New("tenant_category: tenant_id + name + slug required")
	}
	if tc.Kind == "" {
		tc.Kind = domain.TenantCategoryKindCategory
	}
	var parentArg, externalArg interface{}
	if tc.ParentID != "" {
		parentArg = tc.ParentID
	}
	if tc.ExternalID != "" {
		externalArg = tc.ExternalID
	}

	// If external_id given, treat that as the upsert key. Otherwise insert new.
	if tc.ExternalID != "" {
		var id string
		err := a.client.pool.QueryRow(ctx, `
			INSERT INTO catalog.tenant_categories (tenant_id, parent_id, external_id, slug, name, kind)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (tenant_id, external_id) DO UPDATE SET
				parent_id = EXCLUDED.parent_id,
				slug = EXCLUDED.slug,
				name = EXCLUDED.name,
				kind = EXCLUDED.kind
			RETURNING id`,
			tc.TenantID, parentArg, externalArg, tc.Slug, tc.Name, tc.Kind).Scan(&id)
		if err != nil {
			return "", fmt.Errorf("upsert tenant_category: %w", err)
		}
		return id, nil
	}
	var id string
	err := a.client.pool.QueryRow(ctx, `
		INSERT INTO catalog.tenant_categories (tenant_id, parent_id, slug, name, kind)
		VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		tc.TenantID, parentArg, tc.Slug, tc.Name, tc.Kind).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("insert tenant_category: %w", err)
	}
	return id, nil
}

func (a *CategoriesV2Adapter) ListTenantCategories(ctx context.Context, tenantID string) ([]domain.TenantCategory, error) {
	rows, err := a.client.pool.Query(ctx, `
		SELECT id, tenant_id, COALESCE(parent_id::text, ''), COALESCE(external_id, ''),
			slug, name, kind, created_at
		FROM catalog.tenant_categories WHERE tenant_id = $1
		ORDER BY name`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list tenant_categories: %w", err)
	}
	defer rows.Close()
	var out []domain.TenantCategory
	for rows.Next() {
		var c domain.TenantCategory
		if err := rows.Scan(&c.ID, &c.TenantID, &c.ParentID, &c.ExternalID,
			&c.Slug, &c.Name, &c.Kind, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ListTenantCategoriesWithCounts returns the tenant tree with per-node counts
// of linked, non-deleted listings. Cheap query — no recursion, just LEFT JOIN
// on the M:N junction filtered by listing.deleted_at IS NULL.
func (a *CategoriesV2Adapter) ListTenantCategoriesWithCounts(ctx context.Context, tenantID string) ([]domain.TenantCategoryWithCount, error) {
	rows, err := a.client.pool.Query(ctx, `
		SELECT tc.id, tc.tenant_id, COALESCE(tc.parent_id::text, ''),
			COALESCE(tc.external_id, ''), tc.slug, tc.name, tc.kind, tc.created_at,
			COALESCE(cnt.n, 0)::int AS product_count
		FROM catalog.tenant_categories tc
		LEFT JOIN (
			SELECT tlc.tenant_category_id, COUNT(DISTINCT tlc.listing_id) AS n
			FROM catalog.tenant_listing_categories tlc
			JOIN catalog.products p ON p.id = tlc.listing_id
			WHERE p.tenant_id = $1 AND p.deleted_at IS NULL
			GROUP BY tlc.tenant_category_id
		) cnt ON cnt.tenant_category_id = tc.id
		WHERE tc.tenant_id = $1
		ORDER BY tc.name`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list tenant_categories_with_counts: %w", err)
	}
	defer rows.Close()
	var out []domain.TenantCategoryWithCount
	for rows.Next() {
		var c domain.TenantCategoryWithCount
		if err := rows.Scan(&c.ID, &c.TenantID, &c.ParentID, &c.ExternalID,
			&c.Slug, &c.Name, &c.Kind, &c.CreatedAt, &c.ProductCount); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// DeleteTenantCategory removes a tenant category. Children get re-parented to
// NULL (top-level) — the caller can re-link manually if needed. M:N junction
// rows are removed via ON DELETE CASCADE on the FK.
func (a *CategoriesV2Adapter) DeleteTenantCategory(ctx context.Context, tenantID, categoryID string) error {
	tx, err := a.client.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		UPDATE catalog.tenant_categories SET parent_id = NULL
		WHERE parent_id = $1 AND tenant_id = $2`, categoryID, tenantID); err != nil {
		return fmt.Errorf("reparent children: %w", err)
	}
	tag, err := tx.Exec(ctx, `
		DELETE FROM catalog.tenant_categories
		WHERE id = $1 AND tenant_id = $2`, categoryID, tenantID)
	if err != nil {
		return fmt.Errorf("delete tenant_category: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return errors.New("tenant_category not found")
	}
	return tx.Commit(ctx)
}

// --- Mapping ---

func (a *CategoriesV2Adapter) SetCategoryMapping(ctx context.Context, m *domain.CategoryMapping) error {
	if m.TenantCategoryID == "" {
		return errors.New("category_mapping: tenant_category_id required")
	}
	var masterArg interface{}
	if m.MasterCategoryID != "" {
		masterArg = m.MasterCategoryID
	}
	if m.Confidence == "" {
		m.Confidence = "unverified"
	}
	_, err := a.client.pool.Exec(ctx, `
		INSERT INTO catalog.category_mapping (tenant_category_id, master_category_id, confidence, mapped_by)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (tenant_category_id) DO UPDATE SET
			master_category_id = EXCLUDED.master_category_id,
			confidence = EXCLUDED.confidence,
			mapped_by = EXCLUDED.mapped_by`,
		m.TenantCategoryID, masterArg, m.Confidence, m.MappedBy)
	if err != nil {
		return fmt.Errorf("set category_mapping: %w", err)
	}
	return nil
}

func (a *CategoriesV2Adapter) GetCategoryMapping(ctx context.Context, tenantCategoryID string) (*domain.CategoryMapping, error) {
	var m domain.CategoryMapping
	err := a.client.pool.QueryRow(ctx, `
		SELECT tenant_category_id, COALESCE(master_category_id::text, ''),
			confidence, COALESCE(mapped_by, '')
		FROM catalog.category_mapping WHERE tenant_category_id = $1`, tenantCategoryID).Scan(
		&m.TenantCategoryID, &m.MasterCategoryID, &m.Confidence, &m.MappedBy)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get category_mapping: %w", err)
	}
	return &m, nil
}

// --- M:N joins ---

// LinkMasterProductToCategories replaces all category links for a master.
// Atomic: DELETE + bulk INSERT in one transaction.
func (a *CategoriesV2Adapter) LinkMasterProductToCategories(ctx context.Context, masterProductID string, masterCategoryIDs []string) error {
	tx, err := a.client.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM catalog.master_product_categories WHERE master_product_id = $1`, masterProductID); err != nil {
		return fmt.Errorf("delete master_product_categories: %w", err)
	}
	for _, mcID := range masterCategoryIDs {
		if _, err := tx.Exec(ctx, `
			INSERT INTO catalog.master_product_categories (master_product_id, master_category_id)
			VALUES ($1, $2) ON CONFLICT DO NOTHING`,
			masterProductID, mcID); err != nil {
			return fmt.Errorf("insert master_product_categories: %w", err)
		}
	}
	return tx.Commit(ctx)
}

func (a *CategoriesV2Adapter) LinkListingToCategories(ctx context.Context, listingID string, tenantCategoryIDs []string) error {
	tx, err := a.client.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM catalog.tenant_listing_categories WHERE listing_id = $1`, listingID); err != nil {
		return fmt.Errorf("delete tenant_listing_categories: %w", err)
	}
	for _, tcID := range tenantCategoryIDs {
		if _, err := tx.Exec(ctx, `
			INSERT INTO catalog.tenant_listing_categories (listing_id, tenant_category_id)
			VALUES ($1, $2) ON CONFLICT DO NOTHING`,
			listingID, tcID); err != nil {
			return fmt.Errorf("insert tenant_listing_categories: %w", err)
		}
	}
	return tx.Commit(ctx)
}

func (a *CategoriesV2Adapter) ListListingCategories(ctx context.Context, listingID string) ([]domain.TenantCategory, error) {
	rows, err := a.client.pool.Query(ctx, `
		SELECT tc.id, tc.tenant_id, COALESCE(tc.parent_id::text, ''),
			COALESCE(tc.external_id, ''), tc.slug, tc.name, tc.kind, tc.created_at
		FROM catalog.tenant_listing_categories tlc
		JOIN catalog.tenant_categories tc ON tc.id = tlc.tenant_category_id
		WHERE tlc.listing_id = $1
		ORDER BY tc.name`, listingID)
	if err != nil {
		return nil, fmt.Errorf("list listing_categories: %w", err)
	}
	defer rows.Close()
	var out []domain.TenantCategory
	for rows.Next() {
		var c domain.TenantCategory
		if err := rows.Scan(&c.ID, &c.TenantID, &c.ParentID, &c.ExternalID,
			&c.Slug, &c.Name, &c.Kind, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
