package usecases

import (
	"fmt"
	"reflect"

	"github.com/google/uuid"
	"keepstar_v4/internal/domain"
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
// Generates atoms with type, subtype, textStyle/wrapper, and slot for V4-compatible rendering
func applyWidgetTemplate(wt domain.WidgetTemplate, product domain.Product, index int) (*domain.Widget, error) {
	widget := &domain.Widget{
		ID:       uuid.New().String(),
		Size:     wt.Size,
		Priority: index,
		Atoms:    make([]domain.Atom, 0),
	}

	// Hero slot: images
	if len(product.Images) > 0 {
		widget.Atoms = append(widget.Atoms, domain.Atom{
			Type:    domain.AtomTypeImage,
			Subtype: domain.SubtypeImageURL,
			Value:   product.Images, // Array of URLs for carousel
			Slot:    domain.AtomSlotHero,
			MediaStyle: &domain.MediaStyle{
				AspectRatio: "4/3",
				ObjectFit:   "cover",
			},
			Meta: map[string]interface{}{"size": "large"},
		})
	}

	// Title slot
	if product.Name != "" {
		widget.Atoms = append(widget.Atoms, domain.Atom{
			Type:    domain.AtomTypeText,
			Subtype: domain.SubtypeString,
			Value:   product.Name,
			Slot:    domain.AtomSlotTitle,
			TextStyle: &domain.TextStyle{
				FontSize:   "xl",
				FontWeight: "bold",
			},
		})
	}

	// Primary slot: brand as badge
	if product.Brand != "" {
		widget.Atoms = append(widget.Atoms, domain.Atom{
			Type:    domain.AtomTypeText,
			Subtype: domain.SubtypeString,
			Value:   product.Brand,
			Slot:    domain.AtomSlotPrimary,
			Wrapper: &domain.WrapperConfig{
				Type:    "tag",
				Variant: "subtle",
			},
		})
	}

	// Primary slot: rating
	if product.Rating > 0 {
		widget.Atoms = append(widget.Atoms, domain.Atom{
			Type:    domain.AtomTypeNumber,
			Subtype: domain.SubtypeRating,
			Format:  domain.FormatStarsCompact,
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
		Format:  domain.FormatCurrency,
		Value:   product.Price,
		Slot:    domain.AtomSlotPrice,
		Meta:    map[string]interface{}{"currency": currency},
	})

	// Secondary slot: category
	if product.Category != "" {
		widget.Atoms = append(widget.Atoms, domain.Atom{
			Type:    domain.AtomTypeText,
			Subtype: domain.SubtypeString,
			Value:   product.Category,
			Slot:    domain.AtomSlotSecondary,
			TextStyle: &domain.TextStyle{
				FontSize: "sm",
			},
			Meta: map[string]interface{}{"label": "Category"},
		})
	}

	// Secondary slot: description
	if product.Description != "" {
		widget.Atoms = append(widget.Atoms, domain.Atom{
			Type:    domain.AtomTypeText,
			Subtype: domain.SubtypeString,
			Value:   product.Description,
			Slot:    domain.AtomSlotSecondary,
			TextStyle: &domain.TextStyle{
				LineClamp: 3,
			},
			Meta: map[string]interface{}{"label": "Description"},
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
