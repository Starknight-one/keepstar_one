package presets

import "keepstar/internal/domain"

// ============================================================================
// Visual Assembly Presets — used by visual_assembly tool
// All use GenericCard template for uniform rendering
// ============================================================================

// ProductCardGridPreset — grid of cards with image, name, price
var ProductCardGridPreset = domain.Preset{
	Name:        string(domain.PresetProductCardGrid),
	EntityType:  domain.EntityTypeProduct,
	Template:    "GenericCard",
	DefaultMode: domain.FormationTypeGrid,
	DefaultSize: domain.WidgetSizeMedium,
	Fields: []domain.FieldConfig{
		{Name: "images", Slot: domain.AtomSlotHero, AtomType: domain.AtomTypeImage, Subtype: domain.SubtypeImageURL, Display: domain.DisplayImageCover, Priority: 0},
		{Name: "name", Slot: domain.AtomSlotTitle, AtomType: domain.AtomTypeText, Subtype: domain.SubtypeString, Display: domain.DisplayH2, Priority: 1},
		{Name: "price", Slot: domain.AtomSlotPrice, AtomType: domain.AtomTypeNumber, Subtype: domain.SubtypeCurrency, Display: domain.DisplayPrice, Priority: 2},
	},
}

// ProductCardDetailPreset — single card with all fields
var ProductCardDetailPreset = domain.Preset{
	Name:        string(domain.PresetProductCardDetail),
	EntityType:  domain.EntityTypeProduct,
	Template:    "GenericCard",
	DefaultMode: domain.FormationTypeSingle,
	DefaultSize: domain.WidgetSizeLarge,
	Fields: []domain.FieldConfig{
		{Name: "images", Slot: domain.AtomSlotHero, AtomType: domain.AtomTypeImage, Subtype: domain.SubtypeImageURL, Display: domain.DisplayImageCover, Priority: 0},
		{Name: "name", Slot: domain.AtomSlotTitle, AtomType: domain.AtomTypeText, Subtype: domain.SubtypeString, Display: domain.DisplayH1, Priority: 1},
		{Name: "brand", Slot: domain.AtomSlotPrimary, AtomType: domain.AtomTypeText, Subtype: domain.SubtypeString, Display: domain.DisplayTag, Priority: 2},
		{Name: "category", Slot: domain.AtomSlotPrimary, AtomType: domain.AtomTypeText, Subtype: domain.SubtypeString, Display: domain.DisplayTag, Priority: 3},
		{Name: "price", Slot: domain.AtomSlotPrice, AtomType: domain.AtomTypeNumber, Subtype: domain.SubtypeCurrency, Display: domain.DisplayPriceLg, Priority: 4},
		{Name: "rating", Slot: domain.AtomSlotPrimary, AtomType: domain.AtomTypeNumber, Subtype: domain.SubtypeRating, Display: domain.DisplayRating, Priority: 5},
		{Name: "description", Slot: domain.AtomSlotSecondary, AtomType: domain.AtomTypeText, Subtype: domain.SubtypeString, Display: domain.DisplayBody, Priority: 6},
		{Name: "tags", Slot: domain.AtomSlotSecondary, AtomType: domain.AtomTypeText, Subtype: domain.SubtypeString, Display: domain.DisplayTag, Priority: 7},
		{Name: "stockQuantity", Slot: domain.AtomSlotSecondary, AtomType: domain.AtomTypeNumber, Subtype: domain.SubtypeInt, Display: domain.DisplayBodySm, Priority: 8},
	},
}

// ProductRowPreset — horizontal list row, 5 fields compact
var ProductRowPreset = domain.Preset{
	Name:        string(domain.PresetProductRow),
	EntityType:  domain.EntityTypeProduct,
	Template:    "GenericCard",
	DefaultMode: domain.FormationTypeList,
	DefaultSize: domain.WidgetSizeSmall,
	Fields: []domain.FieldConfig{
		{Name: "images", Slot: domain.AtomSlotHero, AtomType: domain.AtomTypeImage, Subtype: domain.SubtypeImageURL, Display: domain.DisplayThumbnail, Priority: 0},
		{Name: "name", Slot: domain.AtomSlotTitle, AtomType: domain.AtomTypeText, Subtype: domain.SubtypeString, Display: domain.DisplayH3, Priority: 1},
		{Name: "brand", Slot: domain.AtomSlotPrimary, AtomType: domain.AtomTypeText, Subtype: domain.SubtypeString, Display: domain.DisplayBodySm, Priority: 2},
		{Name: "price", Slot: domain.AtomSlotPrice, AtomType: domain.AtomTypeNumber, Subtype: domain.SubtypeCurrency, Display: domain.DisplayPrice, Priority: 3},
		{Name: "rating", Slot: domain.AtomSlotPrimary, AtomType: domain.AtomTypeNumber, Subtype: domain.SubtypeRating, Display: domain.DisplayRatingCompact, Priority: 4},
	},
}

// ProductSingleHeroPreset — single large hero card with all key fields
var ProductSingleHeroPreset = domain.Preset{
	Name:        string(domain.PresetProductSingleHero),
	EntityType:  domain.EntityTypeProduct,
	Template:    "GenericCard",
	DefaultMode: domain.FormationTypeSingle,
	DefaultSize: domain.WidgetSizeLarge,
	Fields: []domain.FieldConfig{
		{Name: "images", Slot: domain.AtomSlotHero, AtomType: domain.AtomTypeImage, Subtype: domain.SubtypeImageURL, Display: domain.DisplayImageCover, Priority: 0},
		{Name: "name", Slot: domain.AtomSlotTitle, AtomType: domain.AtomTypeText, Subtype: domain.SubtypeString, Display: domain.DisplayH1, Priority: 1},
		{Name: "brand", Slot: domain.AtomSlotPrimary, AtomType: domain.AtomTypeText, Subtype: domain.SubtypeString, Display: domain.DisplayBadge, Priority: 2},
		{Name: "category", Slot: domain.AtomSlotPrimary, AtomType: domain.AtomTypeText, Subtype: domain.SubtypeString, Display: domain.DisplayTag, Priority: 3},
		{Name: "price", Slot: domain.AtomSlotPrice, AtomType: domain.AtomTypeNumber, Subtype: domain.SubtypeCurrency, Display: domain.DisplayPriceLg, Priority: 4},
		{Name: "rating", Slot: domain.AtomSlotPrimary, AtomType: domain.AtomTypeNumber, Subtype: domain.SubtypeRating, Display: domain.DisplayRating, Priority: 5},
		{Name: "description", Slot: domain.AtomSlotSecondary, AtomType: domain.AtomTypeText, Subtype: domain.SubtypeString, Display: domain.DisplayBodyLg, Priority: 6},
		{Name: "tags", Slot: domain.AtomSlotSecondary, AtomType: domain.AtomTypeText, Subtype: domain.SubtypeString, Display: domain.DisplayTag, Priority: 7},
		{Name: "productForm", Slot: domain.AtomSlotSecondary, AtomType: domain.AtomTypeText, Subtype: domain.SubtypeString, Display: domain.DisplayTag, Priority: 8},
		{Name: "skinType", Slot: domain.AtomSlotSecondary, AtomType: domain.AtomTypeText, Subtype: domain.SubtypeString, Display: domain.DisplayTag, Priority: 9},
	},
}

// SearchEmptyPreset — empty state when no results found
var SearchEmptyPreset = domain.Preset{
	Name:        string(domain.PresetSearchEmpty),
	EntityType:  domain.EntityTypeProduct,
	Template:    "GenericCard",
	DefaultMode: domain.FormationTypeSingle,
	DefaultSize: domain.WidgetSizeMedium,
	Fields: []domain.FieldConfig{
		{Name: "name", Slot: domain.AtomSlotTitle, AtomType: domain.AtomTypeText, Subtype: domain.SubtypeString, Display: domain.DisplayH3, Priority: 0},
		{Name: "description", Slot: domain.AtomSlotSecondary, AtomType: domain.AtomTypeText, Subtype: domain.SubtypeString, Display: domain.DisplayBody, Priority: 1},
	},
}

// CategoryOverviewPreset — category cards in grid
var CategoryOverviewPreset = domain.Preset{
	Name:        string(domain.PresetCategoryOverview),
	EntityType:  domain.EntityTypeProduct,
	Template:    "GenericCard",
	DefaultMode: domain.FormationTypeGrid,
	DefaultSize: domain.WidgetSizeMedium,
	Fields: []domain.FieldConfig{
		{Name: "images", Slot: domain.AtomSlotHero, AtomType: domain.AtomTypeImage, Subtype: domain.SubtypeImageURL, Display: domain.DisplayImageCover, Priority: 0},
		{Name: "category", Slot: domain.AtomSlotTitle, AtomType: domain.AtomTypeText, Subtype: domain.SubtypeString, Display: domain.DisplayH2, Priority: 1},
		{Name: "name", Slot: domain.AtomSlotPrimary, AtomType: domain.AtomTypeText, Subtype: domain.SubtypeString, Display: domain.DisplayBodySm, Priority: 2},
	},
}

// AttributePickerPreset — attribute tags for filtering
var AttributePickerPreset = domain.Preset{
	Name:        string(domain.PresetAttributePicker),
	EntityType:  domain.EntityTypeProduct,
	Template:    "GenericCard",
	DefaultMode: domain.FormationTypeGrid,
	DefaultSize: domain.WidgetSizeSmall,
	Fields: []domain.FieldConfig{
		{Name: "name", Slot: domain.AtomSlotTitle, AtomType: domain.AtomTypeText, Subtype: domain.SubtypeString, Display: domain.DisplayTag, Priority: 0},
	},
}

// CartSummaryPreset — compact cart item row
var CartSummaryPreset = domain.Preset{
	Name:        string(domain.PresetCartSummary),
	EntityType:  domain.EntityTypeProduct,
	Template:    "GenericCard",
	DefaultMode: domain.FormationTypeList,
	DefaultSize: domain.WidgetSizeSmall,
	Fields: []domain.FieldConfig{
		{Name: "images", Slot: domain.AtomSlotHero, AtomType: domain.AtomTypeImage, Subtype: domain.SubtypeImageURL, Display: domain.DisplayThumbnail, Priority: 0},
		{Name: "name", Slot: domain.AtomSlotTitle, AtomType: domain.AtomTypeText, Subtype: domain.SubtypeString, Display: domain.DisplayH4, Priority: 1},
		{Name: "price", Slot: domain.AtomSlotPrice, AtomType: domain.AtomTypeNumber, Subtype: domain.SubtypeCurrency, Display: domain.DisplayPrice, Priority: 2},
		{Name: "stockQuantity", Slot: domain.AtomSlotSecondary, AtomType: domain.AtomTypeNumber, Subtype: domain.SubtypeInt, Display: domain.DisplayCaption, Priority: 3},
	},
}

// InfoCardPreset — informational card (e.g. tips, help)
var InfoCardPreset = domain.Preset{
	Name:        string(domain.PresetInfoCard),
	EntityType:  domain.EntityTypeProduct,
	Template:    "GenericCard",
	DefaultMode: domain.FormationTypeSingle,
	DefaultSize: domain.WidgetSizeMedium,
	Fields: []domain.FieldConfig{
		{Name: "name", Slot: domain.AtomSlotTitle, AtomType: domain.AtomTypeText, Subtype: domain.SubtypeString, Display: domain.DisplayH3, Priority: 0},
		{Name: "description", Slot: domain.AtomSlotSecondary, AtomType: domain.AtomTypeText, Subtype: domain.SubtypeString, Display: domain.DisplayBody, Priority: 1},
	},
}
