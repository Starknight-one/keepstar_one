package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"keepstar_v4/internal/domain"
	"keepstar_v4/internal/ports"
)

// FieldDefinitionAdapter implements ports.FieldDefinitionPort using PostgreSQL.
type FieldDefinitionAdapter struct {
	client *Client
}

// NewFieldDefinitionAdapter creates a new adapter.
func NewFieldDefinitionAdapter(client *Client) *FieldDefinitionAdapter {
	return &FieldDefinitionAdapter{client: client}
}

// ListFieldDefinitions returns all field definitions for a tenant+entity type, ordered by priority.
// tenantIDOrSlug can be either a UUID (tenant_id) or a slug string — both are supported.
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

	return defs, rows.Err()
}

// SampleFieldValues pulls `limit` rows from the tenant's products and extracts
// real sample values per field. Used by Agent2 to show the LLM what the data
// actually looks like (labels + types alone are ambiguous — a string field
// might be a product name, a model number, or a category slug; samples
// disambiguate).
//
// The SELECT joins products→master_products→categories to cover the 13
// hey-babes-style product field definitions (images, name, price, rating,
// brand, category, description, tags, stockQuantity, productForm, skinType,
// concern, keyIngredients). For tenants whose products do not follow this
// PIM shape, missing columns simply result in no samples for that field,
// and the caller falls back to label+type info only.
//
// Service entity type is not supported in this pass — returns an empty map.
// Design doc: docs/New features/METADATA_DRIVEN_BINDING_2026-04-09.md
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
		return nil, fmt.Errorf("sample field values: %w", err)
	}
	defer rows.Close()

	// Collect samples field-by-field. Skip empty/zero values so samples stay
	// informative (an empty string is worse than no sample at all for prompting).
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

		// JSONB arrays: decode and take the first element for images (hero),
		// or the full array for tags/skinType/etc (LLM needs to see array shape).
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

		// Tenant-specific extension fields: each key in the extra JSONB becomes
		// its own sample entry so tenants with custom catalog shapes (electronics,
		// books, furniture) get the same sample coverage as hey-babes-style fields.
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

// toInterfaceSlice converts a []string to []interface{} for JSON-safe sample storage.
func toInterfaceSlice(s []string) []interface{} {
	out := make([]interface{}, len(s))
	for i, v := range s {
		out[i] = v
	}
	return out
}

// GetFieldDefinition returns a single field definition by name.
// tenantIDOrSlug can be either a UUID (tenant_id) or a slug string — both are supported.
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
