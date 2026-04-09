package ports

import (
	"context"
	"keepstar_v4/internal/domain"
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

	// SampleFieldValues returns up to `limit` sample values per field for the given
	// tenant+entity type. Keys are field names; values are raw sample values pulled
	// from real rows (strings for text, numbers for price/rating, []interface{} for
	// JSONB arrays like images/tags). Used by Agent2 to populate the <fields> block
	// in its system prompt so the LLM can match atom slots against real data shapes.
	//
	// Returns a possibly-partial map — fields with no samples are omitted rather
	// than returning an error. Callers should treat this as best-effort enrichment.
	SampleFieldValues(ctx context.Context, tenantID string, entityType domain.EntityType, limit int) (map[string][]interface{}, error)
}
