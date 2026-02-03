package presets

import "keepstar/internal/domain"

// ProductGridPreset for displaying multiple products in grid
var ProductGridPreset = domain.Preset{
	Name:        string(domain.PresetProductGrid),
	EntityType:  domain.EntityTypeProduct,
	Template:    domain.WidgetTemplateProductCard,
	DefaultMode: domain.FormationTypeGrid,
	DefaultSize: domain.WidgetSizeMedium,
	Fields: []domain.FieldConfig{
		{Name: "images", Slot: domain.AtomSlotHero, AtomType: domain.AtomTypeImage, Priority: 1, Required: true},
		{Name: "name", Slot: domain.AtomSlotTitle, AtomType: domain.AtomTypeText, Priority: 2, Required: true},
		{Name: "brand", Slot: domain.AtomSlotPrimary, AtomType: domain.AtomTypeText, Priority: 3, Required: false},
		{Name: "price", Slot: domain.AtomSlotPrice, AtomType: domain.AtomTypePrice, Priority: 4, Required: true},
		{Name: "rating", Slot: domain.AtomSlotPrimary, AtomType: domain.AtomTypeRating, Priority: 5, Required: false},
	},
}

// ProductCardPreset for displaying a single product in detail
var ProductCardPreset = domain.Preset{
	Name:        string(domain.PresetProductCard),
	EntityType:  domain.EntityTypeProduct,
	Template:    domain.WidgetTemplateProductCard,
	DefaultMode: domain.FormationTypeSingle,
	DefaultSize: domain.WidgetSizeLarge,
	Fields: []domain.FieldConfig{
		{Name: "images", Slot: domain.AtomSlotHero, AtomType: domain.AtomTypeImage, Priority: 1, Required: true},
		{Name: "name", Slot: domain.AtomSlotTitle, AtomType: domain.AtomTypeText, Priority: 2, Required: true},
		{Name: "brand", Slot: domain.AtomSlotPrimary, AtomType: domain.AtomTypeText, Priority: 3, Required: false},
		{Name: "category", Slot: domain.AtomSlotPrimary, AtomType: domain.AtomTypeText, Priority: 4, Required: false},
		{Name: "price", Slot: domain.AtomSlotPrice, AtomType: domain.AtomTypePrice, Priority: 5, Required: true},
		{Name: "rating", Slot: domain.AtomSlotPrimary, AtomType: domain.AtomTypeRating, Priority: 6, Required: false},
		{Name: "description", Slot: domain.AtomSlotSecondary, AtomType: domain.AtomTypeText, Priority: 7, Required: false},
	},
}

// ProductCompactPreset for displaying many products in a compact list
var ProductCompactPreset = domain.Preset{
	Name:        string(domain.PresetProductCompact),
	EntityType:  domain.EntityTypeProduct,
	Template:    domain.WidgetTemplateProductCard,
	DefaultMode: domain.FormationTypeList,
	DefaultSize: domain.WidgetSizeSmall,
	Fields: []domain.FieldConfig{
		{Name: "name", Slot: domain.AtomSlotTitle, AtomType: domain.AtomTypeText, Priority: 1, Required: true},
		{Name: "price", Slot: domain.AtomSlotPrice, AtomType: domain.AtomTypePrice, Priority: 2, Required: true},
	},
}

// WidgetTemplateProductDetail is the template name for product detail view
const WidgetTemplateProductDetail = "ProductDetail"

// ProductDetailPreset for displaying a single product in full detail view
var ProductDetailPreset = domain.Preset{
	Name:        string(domain.PresetProductDetail),
	EntityType:  domain.EntityTypeProduct,
	Template:    WidgetTemplateProductDetail,
	DefaultMode: domain.FormationTypeSingle,
	DefaultSize: domain.WidgetSizeLarge,
	Fields: []domain.FieldConfig{
		{Name: "images", Slot: domain.AtomSlotGallery, AtomType: domain.AtomTypeImage, Priority: 1, Required: true},
		{Name: "name", Slot: domain.AtomSlotTitle, AtomType: domain.AtomTypeText, Priority: 2, Required: true},
		{Name: "brand", Slot: domain.AtomSlotPrimary, AtomType: domain.AtomTypeText, Priority: 3, Required: false},
		{Name: "category", Slot: domain.AtomSlotPrimary, AtomType: domain.AtomTypeText, Priority: 4, Required: false},
		{Name: "rating", Slot: domain.AtomSlotPrimary, AtomType: domain.AtomTypeRating, Priority: 5, Required: false},
		{Name: "price", Slot: domain.AtomSlotPrice, AtomType: domain.AtomTypePrice, Priority: 6, Required: true},
		{Name: "stockQuantity", Slot: domain.AtomSlotStock, AtomType: domain.AtomTypeNumber, Priority: 7, Required: false},
		{Name: "description", Slot: domain.AtomSlotDescription, AtomType: domain.AtomTypeText, Priority: 8, Required: false},
		{Name: "tags", Slot: domain.AtomSlotTags, AtomType: domain.AtomTypeText, Priority: 9, Required: false},
		{Name: "attributes", Slot: domain.AtomSlotSpecs, AtomType: domain.AtomTypeText, Priority: 10, Required: false},
	},
}
