package usecases

import (
	"fmt"
	"reflect"

	"github.com/google/uuid"
	"keepstar/internal/domain"
)

// ApplyTemplate applies a FormationTemplate to products, producing FormationWithData
func ApplyTemplate(template *domain.FormationTemplate, products []domain.Product) (*domain.FormationWithData, error) {
	if template == nil {
		return nil, fmt.Errorf("template is nil")
	}

	formation := &domain.FormationWithData{
		Mode: template.Mode,
		Grid: template.Grid,
	}

	// Apply widget template to each product
	for i, product := range products {
		widget, err := applyWidgetTemplate(template.WidgetTemplate, product, i)
		if err != nil {
			return nil, fmt.Errorf("apply widget template for product %d: %w", i, err)
		}
		formation.Widgets = append(formation.Widgets, *widget)
	}

	return formation, nil
}

// applyWidgetTemplate creates a Widget from template and product data
func applyWidgetTemplate(wt domain.WidgetTemplate, product domain.Product, index int) (*domain.Widget, error) {
	widget := &domain.Widget{
		ID:       uuid.New().String(),
		Type:     domain.WidgetTypeProductCard,
		Size:     wt.Size,
		Priority: index,
		Atoms:    make([]domain.Atom, 0, len(wt.Atoms)),
	}

	// Apply each atom template
	for _, atomTpl := range wt.Atoms {
		value := getFieldValue(product, atomTpl.Field)
		if value == nil {
			continue // Skip if field not found
		}

		atom := domain.Atom{
			Type:  atomTpl.Type,
			Value: value,
			Meta:  make(map[string]interface{}),
		}

		// Add style/format metadata
		if atomTpl.Style != "" {
			atom.Meta["style"] = atomTpl.Style
		}
		if atomTpl.Format != "" {
			atom.Meta["format"] = atomTpl.Format
		}
		if atomTpl.Size != "" {
			atom.Meta["size"] = atomTpl.Size
		}

		widget.Atoms = append(widget.Atoms, atom)
	}

	return widget, nil
}

// getFieldValue extracts a field value from product using reflection
func getFieldValue(product domain.Product, fieldName string) interface{} {
	// Map template field names to Product struct fields
	fieldMap := map[string]string{
		"id":          "ID",
		"name":        "Name",
		"description": "Description",
		"price":       "Price",
		"currency":    "Currency",
		"images":      "Images",
		"image_url":   "Images", // First image
		"rating":      "Rating",
		"brand":       "Brand",
		"category":    "Category",
		"stock":       "StockQuantity",
	}

	structField, ok := fieldMap[fieldName]
	if !ok {
		structField = fieldName // Try direct match
	}

	v := reflect.ValueOf(product)
	field := v.FieldByName(structField)
	if !field.IsValid() {
		return nil
	}

	value := field.Interface()

	// Special handling for images - return first image URL
	if fieldName == "image_url" || fieldName == "images" {
		if images, ok := value.([]string); ok && len(images) > 0 {
			return images[0]
		}
	}

	return value
}
