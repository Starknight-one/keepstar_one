package presets

import "keepstar/internal/domain"

// WidgetTemplateServiceCard is the template name for service cards
const WidgetTemplateServiceCard = "ServiceCard"

// ServiceCardPreset for displaying services in grid
var ServiceCardPreset = domain.Preset{
	Name:        string(domain.PresetServiceCard),
	EntityType:  domain.EntityTypeService,
	Template:    WidgetTemplateServiceCard,
	DefaultMode: domain.FormationTypeGrid,
	DefaultSize: domain.WidgetSizeMedium,
	Displays: map[domain.AtomSlot]domain.AtomDisplay{
		domain.AtomSlotHero:    domain.DisplayImageCover,
		domain.AtomSlotTitle:   domain.DisplayH2,
		domain.AtomSlotPrimary: domain.DisplayCaption,
		domain.AtomSlotPrice:   domain.DisplayPrice,
	},
	Fields: []domain.FieldConfig{
		{Name: "images", Slot: domain.AtomSlotHero, AtomType: domain.AtomTypeImage, Subtype: domain.SubtypeImageURL, Display: domain.DisplayImageCover, Priority: 1, Required: false},
		{Name: "name", Slot: domain.AtomSlotTitle, AtomType: domain.AtomTypeText, Subtype: domain.SubtypeString, Display: domain.DisplayH2, Priority: 2, Required: true},
		{Name: "provider", Slot: domain.AtomSlotPrimary, AtomType: domain.AtomTypeText, Subtype: domain.SubtypeString, Display: domain.DisplayCaption, Priority: 3, Required: false},
		{Name: "duration", Slot: domain.AtomSlotPrimary, AtomType: domain.AtomTypeText, Subtype: domain.SubtypeString, Display: domain.DisplayCaption, Priority: 4, Required: false},
		{Name: "price", Slot: domain.AtomSlotPrice, AtomType: domain.AtomTypeNumber, Subtype: domain.SubtypeCurrency, Display: domain.DisplayPrice, Priority: 5, Required: true},
		{Name: "rating", Slot: domain.AtomSlotPrimary, AtomType: domain.AtomTypeNumber, Subtype: domain.SubtypeRating, Display: domain.DisplayRatingCompact, Priority: 6, Required: false},
	},
}

// ServiceListPreset for displaying services in compact list
var ServiceListPreset = domain.Preset{
	Name:        string(domain.PresetServiceList),
	EntityType:  domain.EntityTypeService,
	Template:    WidgetTemplateServiceCard,
	DefaultMode: domain.FormationTypeList,
	DefaultSize: domain.WidgetSizeSmall,
	Displays: map[domain.AtomSlot]domain.AtomDisplay{
		domain.AtomSlotTitle:   domain.DisplayH3,
		domain.AtomSlotPrimary: domain.DisplayCaption,
		domain.AtomSlotPrice:   domain.DisplayPrice,
	},
	Fields: []domain.FieldConfig{
		{Name: "name", Slot: domain.AtomSlotTitle, AtomType: domain.AtomTypeText, Subtype: domain.SubtypeString, Display: domain.DisplayH3, Priority: 1, Required: true},
		{Name: "duration", Slot: domain.AtomSlotPrimary, AtomType: domain.AtomTypeText, Subtype: domain.SubtypeString, Display: domain.DisplayCaption, Priority: 2, Required: false},
		{Name: "price", Slot: domain.AtomSlotPrice, AtomType: domain.AtomTypeNumber, Subtype: domain.SubtypeCurrency, Display: domain.DisplayPrice, Priority: 3, Required: true},
	},
}

// WidgetTemplateServiceDetail is the template name for service detail view
const WidgetTemplateServiceDetail = "ServiceDetail"

// ServiceDetailPreset for displaying a single service in full detail view
var ServiceDetailPreset = domain.Preset{
	Name:        string(domain.PresetServiceDetail),
	EntityType:  domain.EntityTypeService,
	Template:    WidgetTemplateServiceDetail,
	DefaultMode: domain.FormationTypeSingle,
	DefaultSize: domain.WidgetSizeLarge,
	Displays: map[domain.AtomSlot]domain.AtomDisplay{
		domain.AtomSlotGallery:     domain.DisplayGallery,
		domain.AtomSlotTitle:       domain.DisplayH1,
		domain.AtomSlotPrimary:     domain.DisplayTag,
		domain.AtomSlotPrice:       domain.DisplayPriceLg,
		domain.AtomSlotStock:       domain.DisplayCaption,
		domain.AtomSlotDescription: domain.DisplayBody,
		domain.AtomSlotSpecs:       domain.DisplayBodySm,
	},
	Fields: []domain.FieldConfig{
		{Name: "images", Slot: domain.AtomSlotGallery, AtomType: domain.AtomTypeImage, Subtype: domain.SubtypeImageURL, Display: domain.DisplayGallery, Priority: 1, Required: false},
		{Name: "name", Slot: domain.AtomSlotTitle, AtomType: domain.AtomTypeText, Subtype: domain.SubtypeString, Display: domain.DisplayH1, Priority: 2, Required: true},
		{Name: "provider", Slot: domain.AtomSlotPrimary, AtomType: domain.AtomTypeText, Subtype: domain.SubtypeString, Display: domain.DisplayTag, Priority: 3, Required: false},
		{Name: "duration", Slot: domain.AtomSlotPrimary, AtomType: domain.AtomTypeText, Subtype: domain.SubtypeString, Display: domain.DisplayTag, Priority: 4, Required: false},
		{Name: "rating", Slot: domain.AtomSlotPrimary, AtomType: domain.AtomTypeNumber, Subtype: domain.SubtypeRating, Display: domain.DisplayRating, Priority: 5, Required: false},
		{Name: "price", Slot: domain.AtomSlotPrice, AtomType: domain.AtomTypeNumber, Subtype: domain.SubtypeCurrency, Display: domain.DisplayPriceLg, Priority: 6, Required: true},
		{Name: "availability", Slot: domain.AtomSlotStock, AtomType: domain.AtomTypeText, Subtype: domain.SubtypeString, Display: domain.DisplayCaption, Priority: 7, Required: false},
		{Name: "description", Slot: domain.AtomSlotDescription, AtomType: domain.AtomTypeText, Subtype: domain.SubtypeString, Display: domain.DisplayBody, Priority: 8, Required: false},
		{Name: "attributes", Slot: domain.AtomSlotSpecs, AtomType: domain.AtomTypeText, Subtype: domain.SubtypeString, Display: domain.DisplayBodySm, Priority: 9, Required: false},
	},
}
