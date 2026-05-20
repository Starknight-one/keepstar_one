// Package postgres — MasterDigestAdapter implements ports.MasterDigestPort.
// Used by discovery_v2's master-side tools so the LLM agent can see the
// existing master catalog before proposing its mapping artifact.
//
// All queries are read-only and global (master_categories /
// master_products are shared across tenants). Costs are bounded by per-
// query LIMITs; the agent enforces its own call budget on top.
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

type MasterDigestAdapter struct {
	client *Client
	log    *logger.Logger
}

func NewMasterDigestAdapter(client *Client, log *logger.Logger) *MasterDigestAdapter {
	return &MasterDigestAdapter{client: client, log: log}
}

var _ ports.MasterDigestPort = (*MasterDigestAdapter)(nil)

// ListMasterCategories returns master_categories filtered by vertical,
// each annotated with the count of master_products linked to it via the
// M:N table. Empty vertical returns all rows. limit defaults to 200.
func (a *MasterDigestAdapter) ListMasterCategories(ctx context.Context, vertical string, limit int) ([]domain.MasterCategoryNode, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := a.client.pool.Query(ctx, `
		SELECT
			mc.id::text,
			mc.slug,
			mc.name,
			COALESCE(mc.parent_id::text, ''),
			mc.vertical,
			COALESCE((
				SELECT COUNT(*)::int FROM catalog.master_product_categories mpc
				WHERE mpc.master_category_id = mc.id
			), 0) AS product_count
		FROM catalog.master_categories mc
		WHERE ($1 = '' OR mc.vertical = $1)
		ORDER BY mc.vertical, COALESCE(mc.parent_id::text, ''), mc.name
		LIMIT $2
	`, vertical, limit)
	if err != nil {
		return nil, fmt.Errorf("list master categories: %w", err)
	}
	defer rows.Close()

	var out []domain.MasterCategoryNode
	for rows.Next() {
		var n domain.MasterCategoryNode
		if err := rows.Scan(&n.ID, &n.Slug, &n.Name, &n.ParentID, &n.Vertical, &n.ProductCount); err != nil {
			return nil, fmt.Errorf("scan master category: %w", err)
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// DigestMasterCategory builds the compact roll-up: name + vertical, total
// product count, top 10 brands by frequency, 10 most recent product names.
// All three reads use the same M:N join from master_product_categories so
// the function works whether or not master_products.category_id is set.
func (a *MasterDigestAdapter) DigestMasterCategory(ctx context.Context, categoryID string) (*domain.MasterCategoryDigest, error) {
	if categoryID == "" {
		return nil, fmt.Errorf("digest master category: empty category id")
	}

	d := &domain.MasterCategoryDigest{CategoryID: categoryID}

	// Category metadata.
	err := a.client.pool.QueryRow(ctx, `
		SELECT name, vertical FROM catalog.master_categories WHERE id = $1
	`, categoryID).Scan(&d.CategoryName, &d.Vertical)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("digest master category: %s not found", categoryID)
		}
		return nil, fmt.Errorf("digest master category metadata: %w", err)
	}

	// Total product count.
	if err := a.client.pool.QueryRow(ctx, `
		SELECT COUNT(DISTINCT mp.id)::int
		FROM catalog.master_products mp
		JOIN catalog.master_product_categories mpc ON mpc.master_product_id = mp.id
		WHERE mpc.master_category_id = $1
	`, categoryID).Scan(&d.ProductCount); err != nil {
		return nil, fmt.Errorf("digest master category count: %w", err)
	}

	// Top brands (up to 10).
	brandsRows, err := a.client.pool.Query(ctx, `
		SELECT mp.brand
		FROM catalog.master_products mp
		JOIN catalog.master_product_categories mpc ON mpc.master_product_id = mp.id
		WHERE mpc.master_category_id = $1
		  AND mp.brand IS NOT NULL AND mp.brand <> ''
		GROUP BY mp.brand
		ORDER BY COUNT(*) DESC
		LIMIT 10
	`, categoryID)
	if err != nil {
		return nil, fmt.Errorf("digest master category brands: %w", err)
	}
	for brandsRows.Next() {
		var b string
		if err := brandsRows.Scan(&b); err != nil {
			brandsRows.Close()
			return nil, err
		}
		d.TopBrands = append(d.TopBrands, b)
	}
	brandsRows.Close()

	// Recent product names (up to 10).
	namesRows, err := a.client.pool.Query(ctx, `
		SELECT mp.name
		FROM catalog.master_products mp
		JOIN catalog.master_product_categories mpc ON mpc.master_product_id = mp.id
		WHERE mpc.master_category_id = $1
		ORDER BY mp.created_at DESC
		LIMIT 10
	`, categoryID)
	if err != nil {
		return nil, fmt.Errorf("digest master category names: %w", err)
	}
	for namesRows.Next() {
		var n string
		if err := namesRows.Scan(&n); err != nil {
			namesRows.Close()
			return nil, err
		}
		d.SampleNames = append(d.SampleNames, n)
	}
	namesRows.Close()

	return d, nil
}

// FindMasterBySKU is case-insensitive. nil result + nil error = not found.
func (a *MasterDigestAdapter) FindMasterBySKU(ctx context.Context, sku string) (*domain.MasterRef, error) {
	if sku == "" {
		return nil, nil
	}
	var r domain.MasterRef
	var gtin, brand *string
	err := a.client.pool.QueryRow(ctx, `
		SELECT id::text, sku, name, brand, vertical, gtin
		FROM catalog.master_products
		WHERE LOWER(sku) = LOWER($1)
		LIMIT 1
	`, sku).Scan(&r.ID, &r.SKU, &r.Name, &brand, &r.Vertical, &gtin)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("find master by sku: %w", err)
	}
	if brand != nil {
		r.Brand = *brand
	}
	if gtin != nil {
		r.GTIN = *gtin
	}
	return &r, nil
}

// FindMasterByGTIN exact match. nil result + nil error = not found.
func (a *MasterDigestAdapter) FindMasterByGTIN(ctx context.Context, gtin string) (*domain.MasterRef, error) {
	if gtin == "" {
		return nil, nil
	}
	var r domain.MasterRef
	var dbgtin, brand *string
	err := a.client.pool.QueryRow(ctx, `
		SELECT id::text, sku, name, brand, vertical, gtin
		FROM catalog.master_products
		WHERE gtin = $1
		LIMIT 1
	`, gtin).Scan(&r.ID, &r.SKU, &r.Name, &brand, &r.Vertical, &dbgtin)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("find master by gtin: %w", err)
	}
	if brand != nil {
		r.Brand = *brand
	}
	if dbgtin != nil {
		r.GTIN = *dbgtin
	}
	return &r, nil
}

// ListMasterBrandsInCategory orders by product count desc; limit defaults to 30.
func (a *MasterDigestAdapter) ListMasterBrandsInCategory(ctx context.Context, categoryID string, limit int) ([]string, error) {
	if categoryID == "" {
		return nil, fmt.Errorf("list master brands: empty category id")
	}
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	rows, err := a.client.pool.Query(ctx, `
		SELECT mp.brand
		FROM catalog.master_products mp
		JOIN catalog.master_product_categories mpc ON mpc.master_product_id = mp.id
		WHERE mpc.master_category_id = $1
		  AND mp.brand IS NOT NULL AND mp.brand <> ''
		GROUP BY mp.brand
		ORDER BY COUNT(*) DESC
		LIMIT $2
	`, categoryID, limit)
	if err != nil {
		return nil, fmt.Errorf("list master brands: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var b string
		if err := rows.Scan(&b); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}
