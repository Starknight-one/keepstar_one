package engine

import (
	"sort"
	"strings"

	"github.com/google/uuid"
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

// BuildFormation creates formation from preset and entities
func BuildFormation(preset domain.Preset, count int, getEntity EntityGetterFunc) *domain.FormationWithData {
	widgets := make([]domain.Widget, 0, count)

	// Sort fields by priority
	fields := make([]domain.FieldConfig, len(preset.Fields))
	copy(fields, preset.Fields)
	sort.Slice(fields, func(i, j int) bool {
		return fields[i].Priority < fields[j].Priority
	})

	for i := 0; i < count; i++ {
		fieldGetter, currencyGetter, idGetter := getEntity(i)
		atoms := BuildAtoms(fields, fieldGetter, currencyGetter)
		widget := domain.Widget{
			ID:       uuid.New().String(),
			Template: preset.Template,
			Size:     preset.DefaultSize,
			Priority: i,
			Atoms:    atoms,
			EntityRef: &domain.EntityRef{
				Type: preset.EntityType,
				ID:   idGetter(),
			},
		}
		widgets = append(widgets, widget)
	}

	return &domain.FormationWithData{
		Mode:    preset.DefaultMode,
		Widgets: widgets,
	}
}

// BuildTemplateFormation creates a template formation with all fields from the preset.
// Each atom has FieldName set and Value=nil -- the frontend fills values from entity data at render time.
// This is used for adjacent templates: 1 template per entity type instead of N formations per entity.
func BuildTemplateFormation(preset domain.Preset) *domain.FormationWithData {
	fields := make([]domain.FieldConfig, len(preset.Fields))
	copy(fields, preset.Fields)
	sort.Slice(fields, func(i, j int) bool {
		return fields[i].Priority < fields[j].Priority
	})

	atoms := make([]domain.Atom, 0, len(fields))
	for _, field := range fields {
		atom := domain.Atom{
			Type:      field.AtomType,
			Subtype:   field.Subtype,
			Format:    field.Format,
			Display:   string(field.Display),
			Value:     nil,
			FieldName: field.Name,
			Slot:      field.Slot,
		}
		// Add meta hints for special types
		switch field.AtomType {
		case domain.AtomTypeImage:
			atom.Meta = map[string]interface{}{"size": "large"}
		case domain.AtomTypeNumber:
			if field.Subtype == domain.SubtypeCurrency {
				// Sentinel -- frontend replaces with entity.currency
				atom.Meta = map[string]interface{}{"currency": "__ENTITY_CURRENCY__"}
			}
		}
		atoms = append(atoms, atom)
	}

	widget := domain.Widget{
		ID:       "template",
		Template: preset.Template,
		Size:     preset.DefaultSize,
		Atoms:    atoms,
	}

	return &domain.FormationWithData{
		Mode:    preset.DefaultMode,
		Widgets: []domain.Widget{widget},
	}
}

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

// BuildAtoms creates atoms from fields using generic field getter
// Now uses the new atom model with Type, Subtype, Display
func BuildAtoms(fields []domain.FieldConfig, getField FieldGetter, getCurrency CurrencyGetter) []domain.Atom {
	atoms := make([]domain.Atom, 0)

	for _, field := range fields {
		value := getField(field.Name)
		if value == nil {
			continue
		}

		// D7: validate image URLs
		if field.AtomType == domain.AtomTypeImage {
			value = ValidateImageURL(value)
			if value == nil {
				continue
			}
		}

		atom := domain.Atom{
			Type:      field.AtomType,
			Subtype:   field.Subtype,
			Format:    field.Format,
			Display:   string(field.Display),
			Value:     value,
			Slot:      field.Slot,
			FieldName: field.Name,
		}

		// Add meta for backward compatibility and additional data
		switch field.AtomType {
		case domain.AtomTypeImage:
			atom.Meta = map[string]interface{}{"size": "large"}
		case domain.AtomTypeNumber:
			if field.Subtype == domain.SubtypeCurrency {
				currency := getCurrency()
				if currency == "" {
					currency = "$"
				}
				atom.Meta = map[string]interface{}{"currency": currency}
			}
		}

		atoms = append(atoms, atom)
	}

	return atoms
}

// ProductFieldGetter returns a FieldGetter for Product
func ProductFieldGetter(p domain.Product) FieldGetter {
	return func(fieldName string) interface{} {
		switch fieldName {
		case "id":
			return p.ID
		case "name":
			return NonEmpty(p.Name)
		case "description":
			return NonEmpty(p.Description)
		case "price":
			if p.Price == 0 {
				return nil
			}
			return p.Price
		case "images":
			if len(p.Images) == 0 {
				return nil
			}
			return p.Images
		case "rating":
			return p.Rating
		case "brand":
			return NonEmpty(p.Brand)
		case "category":
			return NonEmpty(p.Category)
		case "stockQuantity":
			if p.StockQuantity == 0 {
				return nil
			}
			return p.StockQuantity
		case "tags":
			if len(p.Tags) == 0 {
				return nil
			}
			return p.Tags
		case "productForm":
			return NonEmpty(p.ProductForm)
		case "skinType":
			if len(p.SkinType) == 0 {
				return nil
			}
			return strings.Join(p.SkinType, ", ")
		case "concern":
			if len(p.Concern) == 0 {
				return nil
			}
			return strings.Join(p.Concern, ", ")
		case "keyIngredients":
			if len(p.KeyIngredients) == 0 {
				return nil
			}
			return strings.Join(p.KeyIngredients, ", ")
		default:
			return nil
		}
	}
}

// ServiceFieldGetter returns a FieldGetter for Service
func ServiceFieldGetter(s domain.Service) FieldGetter {
	return func(fieldName string) interface{} {
		switch fieldName {
		case "id":
			return s.ID
		case "name":
			return NonEmpty(s.Name)
		case "description":
			return NonEmpty(s.Description)
		case "price":
			if s.Price == 0 {
				return nil
			}
			return s.Price
		case "images":
			if len(s.Images) == 0 {
				return nil
			}
			return s.Images
		case "rating":
			return s.Rating
		case "duration":
			return NonEmpty(s.Duration)
		case "provider":
			return NonEmpty(s.Provider)
		case "availability":
			return NonEmpty(s.Availability)
		case "attributes":
			if len(s.Attributes) == 0 {
				return nil
			}
			return s.Attributes
		default:
			return nil
		}
	}
}

// NonEmpty returns nil if string is empty, otherwise returns the string
func NonEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
