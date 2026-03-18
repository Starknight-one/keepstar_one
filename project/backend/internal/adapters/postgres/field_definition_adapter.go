package postgres

import (
	"context"
	"fmt"

	"keepstar/internal/domain"
	"keepstar/internal/ports"
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
func (a *FieldDefinitionAdapter) ListFieldDefinitions(ctx context.Context, tenantID string, entityType domain.EntityType) ([]ports.FieldDefinition, error) {
	query := `
		SELECT tenant_id, field_name, entity_type, atom_type, atom_subtype,
		       COALESCE(unit, ''), label, default_display, default_slot, priority
		FROM catalog.field_definitions
		WHERE tenant_id = $1 AND entity_type = $2
		ORDER BY priority ASC
	`

	rows, err := a.client.pool.Query(ctx, query, tenantID, string(entityType))
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
func (a *FieldDefinitionAdapter) GetFieldDefinition(ctx context.Context, tenantID string, entityType domain.EntityType, fieldName string) (*ports.FieldDefinition, error) {
	query := `
		SELECT tenant_id, field_name, entity_type, atom_type, atom_subtype,
		       COALESCE(unit, ''), label, default_display, default_slot, priority
		FROM catalog.field_definitions
		WHERE tenant_id = $1 AND entity_type = $2 AND field_name = $3
	`

	var d ports.FieldDefinition
	var et string
	err := a.client.pool.QueryRow(ctx, query, tenantID, string(entityType), fieldName).Scan(
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
