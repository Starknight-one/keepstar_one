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
// Generates atoms with type, subtype, display, and slot hints for template-based rendering
func applyWidgetTemplate(wt domain.WidgetTemplate, product domain.Product, index int) (*domain.Widget, error) {
	widget := &domain.Widget{
		ID:       uuid.New().String(),
		Template: domain.WidgetTemplateProductCard,
		Size:     wt.Size,
		Priority: index,
		Atoms:    make([]domain.Atom, 0),
	}

	// Hero slot: images
	if len(product.Images) > 0 {
		widget.Atoms = append(widget.Atoms, domain.Atom{
			Type:    domain.AtomTypeImage,
			Subtype: domain.SubtypeImageURL,
			Display: string(domain.DisplayImageCover),
			Value:   product.Images, // Array of URLs for carousel
			Slot:    domain.AtomSlotHero,
			Meta:    map[string]interface{}{"size": "large"},
		})
	}

	// Title slot
	if product.Name != "" {
		widget.Atoms = append(widget.Atoms, domain.Atom{
			Type:    domain.AtomTypeText,
			Subtype: domain.SubtypeString,
			Display: string(domain.DisplayH2),
			Value:   product.Name,
			Slot:    domain.AtomSlotTitle,
		})
	}

	// Primary slot: brand as tag
	if product.Brand != "" {
		widget.Atoms = append(widget.Atoms, domain.Atom{
			Type:    domain.AtomTypeText,
			Subtype: domain.SubtypeString,
			Display: string(domain.DisplayTag),
			Value:   product.Brand,
			Slot:    domain.AtomSlotPrimary,
		})
	}

	// Primary slot: rating
	if product.Rating > 0 {
		widget.Atoms = append(widget.Atoms, domain.Atom{
			Type:    domain.AtomTypeNumber,
			Subtype: domain.SubtypeRating,
			Display: string(domain.DisplayRatingCompact),
			Value:   product.Rating,
			Slot:    domain.AtomSlotPrimary,
		})
	}

	// Price slot
	currency := product.Currency
	if currency == "" {
		currency = "$"
	}
	widget.Atoms = append(widget.Atoms, domain.Atom{
		Type:    domain.AtomTypeNumber,
		Subtype: domain.SubtypeCurrency,
		Display: string(domain.DisplayPrice),
		Value:   product.Price,
		Slot:    domain.AtomSlotPrice,
		Meta:    map[string]interface{}{"currency": currency},
	})

	// Secondary slot: category
	if product.Category != "" {
		widget.Atoms = append(widget.Atoms, domain.Atom{
			Type:    domain.AtomTypeText,
			Subtype: domain.SubtypeString,
			Display: string(domain.DisplayCaption),
			Value:   product.Category,
			Slot:    domain.AtomSlotSecondary,
			Meta:    map[string]interface{}{"label": "Category"},
		})
	}

	// Secondary slot: description
	if product.Description != "" {
		widget.Atoms = append(widget.Atoms, domain.Atom{
			Type:    domain.AtomTypeText,
			Subtype: domain.SubtypeString,
			Display: string(domain.DisplayBody),
			Value:   product.Description,
			Slot:    domain.AtomSlotSecondary,
			Meta:    map[string]interface{}{"label": "Description"},
		})
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
