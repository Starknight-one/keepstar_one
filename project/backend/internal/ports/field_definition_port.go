package ports

import (
	"context"
	"keepstar/internal/domain"
)

// FieldDefinition describes a catalog field's metadata for the visual assembly engine.
// Replaces the hardcoded FieldTypeMap + defaultDisplay + defaultSlot maps.
type FieldDefinition struct {
	TenantID       string           `json:"tenantId"`
	FieldName      string           `json:"fieldName"`      // "price", "brand", "skinType"
	EntityType     domain.EntityType `json:"entityType"`     // "product" or "service"
	AtomType       domain.AtomType  `json:"atomType"`       // "text", "number", "image"
	AtomSubtype    domain.AtomSubtype `json:"atomSubtype"`   // "currency", "rating", "string"
	Unit           string           `json:"unit,omitempty"`  // "RUB", "USD", "min"
	Label          string           `json:"label"`           // Human-readable: "Цена", "Бренд"
	DefaultDisplay string           `json:"defaultDisplay"`  // "price", "tag", "h2"
	DefaultSlot    domain.AtomSlot  `json:"defaultSlot"`     // "price", "primary", "title"
	Priority       int              `json:"priority"`        // Lower = more important
}

// FieldDefinitionPort provides field metadata from the database.
type FieldDefinitionPort interface {
	// ListFieldDefinitions returns all field definitions for a tenant+entity type.
	// Results are ordered by priority ASC.
	ListFieldDefinitions(ctx context.Context, tenantID string, entityType domain.EntityType) ([]FieldDefinition, error)

	// GetFieldDefinition returns a single field definition by name.
	GetFieldDefinition(ctx context.Context, tenantID string, entityType domain.EntityType, fieldName string) (*FieldDefinition, error)
}
