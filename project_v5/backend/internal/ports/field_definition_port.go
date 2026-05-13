package ports

import (
	"context"

	"keepstar_v5/internal/domain"
)

// FieldDefinition describes one catalog field's metadata for the assembly
// engine. Lives in catalog.field_definitions (per-tenant). Replaces the
// hardcoded V4 maps.
type FieldDefinition struct {
	TenantID       string             `json:"tenantId"`
	FieldName      string             `json:"fieldName"`      // "price", "brand", "skinType", "cpu"
	EntityType     domain.EntityType  `json:"entityType"`     // "product" | "service"
	AtomType       domain.AtomType    `json:"atomType"`       // "text" | "number" | "image"
	AtomSubtype    domain.AtomSubtype `json:"atomSubtype"`    // "currency" | "rating" | "string"
	Unit           string             `json:"unit,omitempty"` // "RUB" | "USD" | "min"
	Label          string             `json:"label"`          // human-readable, "Price"
	DefaultDisplay string             `json:"defaultDisplay"` // "price" | "tag" | "h2"
	DefaultSlot    domain.AtomSlot    `json:"defaultSlot"`    // "price" | "primary" | "title"
	Priority       int                `json:"priority"`       // lower = more important
}

// FieldDefinitionPort serves field metadata + sample values to the
// promptbuilder. Catalog write side lives in admin; V5 only reads.
type FieldDefinitionPort interface {
	ListFieldDefinitions(ctx context.Context, tenantIDOrSlug string, entityType domain.EntityType) ([]FieldDefinition, error)
	GetFieldDefinition(ctx context.Context, tenantIDOrSlug string, entityType domain.EntityType, fieldName string) (*FieldDefinition, error)
	// SampleFieldValues returns up to `limit` real sample values per field
	// for the tenant's data, used by Agent2 to populate the <fields> block
	// in its system prompt. Best-effort enrichment — empty values omitted,
	// missing fields just don't appear in the result.
	SampleFieldValues(ctx context.Context, tenantIDOrSlug string, entityType domain.EntityType, limit int) (map[string][]interface{}, error)
}
