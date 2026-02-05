package presets

import "keepstar/internal/domain"

// ProductGridPreset for displaying multiple products in grid
var ProductGridPreset = domain.Preset{
	Name:        string(domain.PresetProductGrid),
	EntityType:  domain.EntityTypeProduct,
	Template:    domain.WidgetTemplateProductCard,
	DefaultMode: domain.FormationTypeGrid,
	DefaultSize: domain.WidgetSizeMedium,
	Displays: map[domain.AtomSlot]domain.AtomDisplay{
		domain.AtomSlotHero:    domain.DisplayImageCover,
		domain.AtomSlotTitle:   domain.DisplayH2,
		domain.AtomSlotBadge:   domain.DisplayBadge,
		domain.AtomSlotPrimary: domain.DisplayTag,
		domain.AtomSlotPrice:   domain.DisplayPrice,
	},
	Fields: []domain.FieldConfig{
		{Name: "images", Slot: domain.AtomSlotHero, AtomType: domain.AtomTypeImage, Subtype: domain.SubtypeImageURL, Display: domain.DisplayImageCover, Priority: 1, Required: true},
		{Name: "name", Slot: domain.AtomSlotTitle, AtomType: domain.AtomTypeText, Subtype: domain.SubtypeString, Display: domain.DisplayH2, Priority: 2, Required: true},
		{Name: "brand", Slot: domain.AtomSlotPrimary, AtomType: domain.AtomTypeText, Subtype: domain.SubtypeString, Display: domain.DisplayTag, Priority: 3, Required: false},
		{Name: "price", Slot: domain.AtomSlotPrice, AtomType: domain.AtomTypeNumber, Subtype: domain.SubtypeCurrency, Display: domain.DisplayPrice, Priority: 4, Required: true},
		{Name: "rating", Slot: domain.AtomSlotPrimary, AtomType: domain.AtomTypeNumber, Subtype: domain.SubtypeRating, Display: domain.DisplayRatingCompact, Priority: 5, Required: false},
	},
}

// ProductCardPreset for displaying a single product in detail
var ProductCardPreset = domain.Preset{
	Name:        string(domain.PresetProductCard),
	EntityType:  domain.EntityTypeProduct,
	Template:    domain.WidgetTemplateProductCard,
	DefaultMode: domain.FormationTypeSingle,
	DefaultSize: domain.WidgetSizeLarge,
	Displays: map[domain.AtomSlot]domain.AtomDisplay{
		domain.AtomSlotHero:      domain.DisplayImageCover,
		domain.AtomSlotTitle:     domain.DisplayH1,
		domain.AtomSlotBadge:     domain.DisplayBadgeSuccess,
		domain.AtomSlotPrimary:   domain.DisplayTag,
		domain.AtomSlotPrice:     domain.DisplayPriceLg,
		domain.AtomSlotSecondary: domain.DisplayBody,
	},
	Fields: []domain.FieldConfig{
		{Name: "images", Slot: domain.AtomSlotHero, AtomType: domain.AtomTypeImage, Subtype: domain.SubtypeImageURL, Display: domain.DisplayImageCover, Priority: 1, Required: true},
		{Name: "name", Slot: domain.AtomSlotTitle, AtomType: domain.AtomTypeText, Subtype: domain.SubtypeString, Display: domain.DisplayH1, Priority: 2, Required: true},
		{Name: "brand", Slot: domain.AtomSlotPrimary, AtomType: domain.AtomTypeText, Subtype: domain.SubtypeString, Display: domain.DisplayTag, Priority: 3, Required: false},
		{Name: "category", Slot: domain.AtomSlotPrimary, AtomType: domain.AtomTypeText, Subtype: domain.SubtypeString, Display: domain.DisplayTag, Priority: 4, Required: false},
		{Name: "price", Slot: domain.AtomSlotPrice, AtomType: domain.AtomTypeNumber, Subtype: domain.SubtypeCurrency, Display: domain.DisplayPriceLg, Priority: 5, Required: true},
		{Name: "rating", Slot: domain.AtomSlotPrimary, AtomType: domain.AtomTypeNumber, Subtype: domain.SubtypeRating, Display: domain.DisplayRating, Priority: 6, Required: false},
		{Name: "description", Slot: domain.AtomSlotSecondary, AtomType: domain.AtomTypeText, Subtype: domain.SubtypeString, Display: domain.DisplayBody, Priority: 7, Required: false},
	},
}

// ProductCompactPreset for displaying many products in a compact list
var ProductCompactPreset = domain.Preset{
	Name:        string(domain.PresetProductCompact),
	EntityType:  domain.EntityTypeProduct,
	Template:    domain.WidgetTemplateProductCard,
	DefaultMode: domain.FormationTypeList,
	DefaultSize: domain.WidgetSizeSmall,
	Displays: map[domain.AtomSlot]domain.AtomDisplay{
		domain.AtomSlotTitle: domain.DisplayH3,
		domain.AtomSlotPrice: domain.DisplayPrice,
	},
	Fields: []domain.FieldConfig{
		{Name: "name", Slot: domain.AtomSlotTitle, AtomType: domain.AtomTypeText, Subtype: domain.SubtypeString, Display: domain.DisplayH3, Priority: 1, Required: true},
		{Name: "price", Slot: domain.AtomSlotPrice, AtomType: domain.AtomTypeNumber, Subtype: domain.SubtypeCurrency, Display: domain.DisplayPrice, Priority: 2, Required: true},
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
	Displays: map[domain.AtomSlot]domain.AtomDisplay{
		domain.AtomSlotGallery:     domain.DisplayGallery,
		domain.AtomSlotTitle:       domain.DisplayH1,
		domain.AtomSlotBadge:       domain.DisplayBadgeSuccess,
		domain.AtomSlotPrimary:     domain.DisplayTag,
		domain.AtomSlotPrice:       domain.DisplayPriceLg,
		domain.AtomSlotStock:       domain.DisplayCaption,
		domain.AtomSlotDescription: domain.DisplayBody,
		domain.AtomSlotTags:        domain.DisplayTag,
		domain.AtomSlotSpecs:       domain.DisplayBodySm,
	},
	Fields: []domain.FieldConfig{
		{Name: "images", Slot: domain.AtomSlotGallery, AtomType: domain.AtomTypeImage, Subtype: domain.SubtypeImageURL, Display: domain.DisplayGallery, Priority: 1, Required: true},
		{Name: "name", Slot: domain.AtomSlotTitle, AtomType: domain.AtomTypeText, Subtype: domain.SubtypeString, Display: domain.DisplayH1, Priority: 2, Required: true},
		{Name: "brand", Slot: domain.AtomSlotPrimary, AtomType: domain.AtomTypeText, Subtype: domain.SubtypeString, Display: domain.DisplayTag, Priority: 3, Required: false},
		{Name: "category", Slot: domain.AtomSlotPrimary, AtomType: domain.AtomTypeText, Subtype: domain.SubtypeString, Display: domain.DisplayTag, Priority: 4, Required: false},
		{Name: "rating", Slot: domain.AtomSlotPrimary, AtomType: domain.AtomTypeNumber, Subtype: domain.SubtypeRating, Display: domain.DisplayRating, Priority: 5, Required: false},
		{Name: "price", Slot: domain.AtomSlotPrice, AtomType: domain.AtomTypeNumber, Subtype: domain.SubtypeCurrency, Display: domain.DisplayPriceLg, Priority: 6, Required: true},
		{Name: "stockQuantity", Slot: domain.AtomSlotStock, AtomType: domain.AtomTypeNumber, Subtype: domain.SubtypeInt, Display: domain.DisplayCaption, Priority: 7, Required: false},
		{Name: "description", Slot: domain.AtomSlotDescription, AtomType: domain.AtomTypeText, Subtype: domain.SubtypeString, Display: domain.DisplayBody, Priority: 8, Required: false},
		{Name: "tags", Slot: domain.AtomSlotTags, AtomType: domain.AtomTypeText, Subtype: domain.SubtypeString, Display: domain.DisplayTag, Priority: 9, Required: false},
		{Name: "attributes", Slot: domain.AtomSlotSpecs, AtomType: domain.AtomTypeText, Subtype: domain.SubtypeString, Display: domain.DisplayBodySm, Priority: 10, Required: false},
	},
}
