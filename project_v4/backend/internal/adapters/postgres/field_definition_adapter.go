package postgres

import (
	"context"
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
