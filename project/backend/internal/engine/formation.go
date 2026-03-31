package engine

import (
	"strings"

	"keepstar/internal/domain"
)

// FieldGetter extracts field value from an entity
type FieldGetter func(fieldName string) interface{}

// CurrencyGetter extracts currency from an entity
type CurrencyGetter func() string

// IDGetter extracts entity ID
type IDGetter func() string

// EntityGetterFunc returns field getter, currency getter, and ID getter for entity at index i
type EntityGetterFunc func(i int) (FieldGetter, CurrencyGetter, IDGetter)

// ProductToMap converts a Product to a generic map for use with GenericFieldGetter.
func ProductToMap(p domain.Product) map[string]interface{} {
	m := map[string]interface{}{
		"id":          p.ID,
		"name":        p.Name,
		"description": p.Description,
		"brand":       p.Brand,
		"category":    p.Category,
		"productForm": p.ProductForm,
		"texture":     p.Texture,
		"routineStep": p.RoutineStep,
		"marketingClaim": p.MarketingClaim,
	}
	if p.Price != 0 {
		m["price"] = p.Price
	}
	if len(p.Images) > 0 {
		m["images"] = p.Images
	}
	if p.Rating != 0 {
		m["rating"] = p.Rating
	}
	if p.StockQuantity != 0 {
		m["stockQuantity"] = p.StockQuantity
	}
	if len(p.Tags) > 0 {
		m["tags"] = p.Tags
	}
	if len(p.SkinType) > 0 {
		m["skinType"] = strings.Join(p.SkinType, ", ")
	}
	if len(p.Concern) > 0 {
		m["concern"] = strings.Join(p.Concern, ", ")
	}
	if len(p.KeyIngredients) > 0 {
		m["keyIngredients"] = strings.Join(p.KeyIngredients, ", ")
	}
	if len(p.TargetArea) > 0 {
		m["targetArea"] = strings.Join(p.TargetArea, ", ")
	}
	if len(p.Benefits) > 0 {
		m["benefits"] = strings.Join(p.Benefits, ", ")
	}
	// Merge extra fields (tenant-defined custom fields)
	for k, v := range p.Extra {
		if _, exists := m[k]; !exists {
			m[k] = v
		}
	}
	// Remove empty strings to keep nil semantics
	for k, v := range m {
		if s, ok := v.(string); ok && s == "" {
			delete(m, k)
		}
	}
	return m
}

// ServiceToMap converts a Service to a generic map for use with GenericFieldGetter.
func ServiceToMap(s domain.Service) map[string]interface{} {
	m := map[string]interface{}{
		"id":           s.ID,
		"name":         s.Name,
		"description":  s.Description,
		"duration":     s.Duration,
		"provider":     s.Provider,
		"availability": s.Availability,
	}
	if s.Price != 0 {
		m["price"] = s.Price
	}
	if len(s.Images) > 0 {
		m["images"] = s.Images
	}
	if s.Rating != 0 {
		m["rating"] = s.Rating
	}
	if len(s.Tags) > 0 {
		m["tags"] = s.Tags
	}
	if len(s.Attributes) > 0 {
		m["attributes"] = s.Attributes
	}
	// Merge extra fields
	for k, v := range s.Extra {
		if _, exists := m[k]; !exists {
			m[k] = v
		}
	}
	// Remove empty strings
	for k, v := range m {
		if str, ok := v.(string); ok && str == "" {
			delete(m, k)
		}
	}
	return m
}

// GenericFieldGetter creates a FieldGetter from an arbitrary map.
// This replaces ProductFieldGetter/ServiceFieldGetter with a single universal function.
func GenericFieldGetter(data map[string]interface{}) FieldGetter {
	return func(fieldName string) interface{} {
		v, ok := data[fieldName]
		if !ok {
			return nil
		}
		return v
	}
}

// NonEmpty returns nil if string is empty, otherwise returns the string
func NonEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
