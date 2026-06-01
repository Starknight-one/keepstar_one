package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgconn"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/ports"
)

// FieldDefinitionAdapter implements ports.FieldDefinitionPort against the
// shared catalog.* schema (owned by project_admin, not V5). Verbatim port
// from V4: field semantics didn't change.
type FieldDefinitionAdapter struct {
	client *Client
}

func NewFieldDefinitionAdapter(client *Client) *FieldDefinitionAdapter {
	return &FieldDefinitionAdapter{client: client}
}

// ListFieldDefinitions returns all field definitions for a tenant+entity
// type, ordered by priority. tenantIDOrSlug accepts either a UUID or the
// tenant slug.
func (a *FieldDefinitionAdapter) ListFieldDefinitions(ctx context.Context, tenantIDOrSlug string, entityType domain.EntityType) ([]ports.FieldDefinition, error) {
	query := `
		SELECT fd.tenant_id, fd.field_name, fd.entity_type, fd.atom_type, fd.atom_subtype,
		       COALESCE(fd.unit, ''), fd.label, fd.default_display, fd.default_slot, fd.priority
		FROM catalog.field_definitions fd
		JOIN catalog.tenants t ON t.id = fd.tenant_id
		WHERE (fd.tenant_id::text = $1 OR t.slug = $1) AND fd.entity_type = $2
		ORDER BY fd.priority ASC
	`
	rows, err := a.client.pool.Query(ctx, query, tenantIDOrSlug, string(entityType))
	if err != nil {
		if isUndefinedTable(err) {
			// catalog.field_definitions is optional per-tenant enrichment, not a
			// hard dependency: when it is absent the Agent2 <fields> block is
			// derived from real catalog data instead. Degrade, don't fail.
			slog.WarnContext(ctx, "catalog.field_definitions absent; <fields> will be data-derived", "tenant", tenantIDOrSlug)
			return nil, nil
		}
		return nil, fmt.Errorf("list field definitions: %w", err)
	}
	defer rows.Close()

	var defs []ports.FieldDefinition
	for rows.Next() {
		var d ports.FieldDefinition
		var et string
		err := rows.Scan(
			&d.TenantID, &d.FieldName, &et,
			&d.AtomType, &d.AtomSubtype,
			&d.Unit, &d.Label, &d.DefaultDisplay, &d.DefaultSlot, &d.Priority,
		)
		if err != nil {
			return nil, fmt.Errorf("scan field definition: %w", err)
		}
		d.EntityType = domain.EntityType(et)
		defs = append(defs, d)
	}
	if err := rows.Err(); err != nil {
		if isUndefinedTable(err) {
			slog.WarnContext(ctx, "catalog.field_definitions absent; <fields> will be data-derived", "tenant", tenantIDOrSlug)
			return nil, nil
		}
		return nil, fmt.Errorf("list field definitions: %w", err)
	}
	return defs, nil
}

// isUndefinedTable reports whether err is a Postgres "undefined_table" error
// (SQLSTATE 42P01). catalog.field_definitions is optional enrichment, so its
// absence degrades gracefully rather than failing the Agent2 turn.
func isUndefinedTable(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "42P01"
	}
	return false
}

func (a *FieldDefinitionAdapter) GetFieldDefinition(ctx context.Context, tenantIDOrSlug string, entityType domain.EntityType, fieldName string) (*ports.FieldDefinition, error) {
	query := `
		SELECT fd.tenant_id, fd.field_name, fd.entity_type, fd.atom_type, fd.atom_subtype,
		       COALESCE(fd.unit, ''), fd.label, fd.default_display, fd.default_slot, fd.priority
		FROM catalog.field_definitions fd
		JOIN catalog.tenants t ON t.id = fd.tenant_id
		WHERE (fd.tenant_id::text = $1 OR t.slug = $1) AND fd.entity_type = $2 AND fd.field_name = $3
	`
	var d ports.FieldDefinition
	var et string
	err := a.client.pool.QueryRow(ctx, query, tenantIDOrSlug, string(entityType), fieldName).Scan(
		&d.TenantID, &d.FieldName, &et,
		&d.AtomType, &d.AtomSubtype,
		&d.Unit, &d.Label, &d.DefaultDisplay, &d.DefaultSlot, &d.Priority,
	)
	if err != nil {
		return nil, fmt.Errorf("get field definition %q: %w", fieldName, err)
	}
	d.EntityType = domain.EntityType(et)
	return &d, nil
}

// SampleFieldValues pulls up to `limit` rows from the tenant's products and
// extracts real sample values per field. Skips empty/zero values so samples
// stay informative — empty string is worse than no sample for prompting.
//
// JOIN covers heybabes-shape PIM fields (name/price/rating/brand/category/
// productForm/skinType/concern/keyIngredients/images/tags/stockQuantity);
// for non-PIM tenants those columns are NULL/empty and just produce no
// samples. Tenant-specific fields in products.extra get spread into samples
// individually so non-PIM tenants get coverage too.
//
// Service entity not implemented — returns empty map.
func (a *FieldDefinitionAdapter) SampleFieldValues(ctx context.Context, tenantIDOrSlug string, entityType domain.EntityType, limit int) (map[string][]interface{}, error) {
	result := make(map[string][]interface{})
	if limit <= 0 {
		limit = 3
	}
	if entityType != domain.EntityTypeProduct {
		return result, nil
	}

	query := `
		SELECT
		    COALESCE(p.name, '') AS name,
		    COALESCE(p.description, '') AS description,
		    p.price,
		    COALESCE(p.rating, 0) AS rating,
		    COALESCE(p.stock_quantity, 0) AS stock_quantity,
		    COALESCE(p.images, '[]') AS images,
		    COALESCE(p.tags, '[]') AS tags,
		    COALESCE(mp.brand, '') AS brand,
		    COALESCE(c.name, '') AS category,
		    COALESCE(mp.product_form, '') AS product_form,
		    COALESCE(mp.skin_type, '{}') AS skin_type,
		    COALESCE(mp.concern, '{}') AS concern,
		    COALESCE(mp.key_ingredients, '{}') AS key_ingredients,
		    COALESCE(p.extra, '{}'::jsonb) AS extra
		FROM catalog.products p
		LEFT JOIN catalog.master_products mp ON p.master_product_id = mp.id
		LEFT JOIN catalog.categories c ON mp.category_id = c.id
		JOIN catalog.tenants t ON t.id = p.tenant_id
		WHERE (p.tenant_id::text = $1 OR t.slug = $1)
		LIMIT $2
	`
	rows, err := a.client.pool.Query(ctx, query, tenantIDOrSlug, limit)
	if err != nil {
		if isUndefinedTable(err) {
			slog.WarnContext(ctx, "catalog tables absent; SampleFieldValues → empty samples", "tenant", tenantIDOrSlug)
			return result, nil
		}
		return nil, fmt.Errorf("sample field values: %w", err)
	}
	defer rows.Close()

	appendSample := func(field string, val interface{}) {
		if val == nil {
			return
		}
		switch v := val.(type) {
		case string:
			if v == "" || v == "<UNKNOWN>" {
				return
			}
		case int:
			if v == 0 {
				return
			}
		case int64:
			if v == 0 {
				return
			}
		case float64:
			if v == 0 {
				return
			}
		case []interface{}:
			if len(v) == 0 {
				return
			}
		case []string:
			if len(v) == 0 {
				return
			}
		}
		result[field] = append(result[field], val)
	}

	for rows.Next() {
		var (
			name, description, brand, category, productForm string
			price, stockQuantity                             int
			rating                                           float64
			imagesRaw, tagsRaw, extraRaw                     []byte
			skinType, concern, keyIngredients                []string
		)
		if err := rows.Scan(
			&name, &description, &price, &rating, &stockQuantity,
			&imagesRaw, &tagsRaw,
			&brand, &category, &productForm,
			&skinType, &concern, &keyIngredients,
			&extraRaw,
		); err != nil {
			return nil, fmt.Errorf("scan sample row: %w", err)
		}

		appendSample("name", name)
		appendSample("description", description)
		appendSample("price", price)
		appendSample("rating", rating)
		appendSample("stockQuantity", stockQuantity)
		appendSample("brand", brand)
		appendSample("category", category)
		appendSample("productForm", productForm)

		var images []interface{}
		if len(imagesRaw) > 0 {
			_ = json.Unmarshal(imagesRaw, &images)
		}
		if len(images) > 0 {
			appendSample("images", images[0])
		}

		var tags []interface{}
		if len(tagsRaw) > 0 {
			_ = json.Unmarshal(tagsRaw, &tags)
		}
		if len(tags) > 0 {
			appendSample("tags", tags)
		}

		if len(skinType) > 0 {
			appendSample("skinType", toInterfaceSlice(skinType))
		}
		if len(concern) > 0 {
			appendSample("concern", toInterfaceSlice(concern))
		}
		if len(keyIngredients) > 0 {
			appendSample("keyIngredients", toInterfaceSlice(keyIngredients))
		}

		var extra map[string]interface{}
		if len(extraRaw) > 0 {
			_ = json.Unmarshal(extraRaw, &extra)
		}
		for k, v := range extra {
			appendSample(k, v)
		}
	}
	return result, rows.Err()
}

func toInterfaceSlice(s []string) []interface{} {
	out := make([]interface{}, len(s))
	for i, v := range s {
		out[i] = v
	}
	return out
}
