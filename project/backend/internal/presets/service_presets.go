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
	Fields: []domain.FieldConfig{
		{Name: "images", Slot: domain.AtomSlotHero, AtomType: domain.AtomTypeImage, Priority: 1, Required: false},
		{Name: "name", Slot: domain.AtomSlotTitle, AtomType: domain.AtomTypeText, Priority: 2, Required: true},
		{Name: "provider", Slot: domain.AtomSlotPrimary, AtomType: domain.AtomTypeText, Priority: 3, Required: false},
		{Name: "duration", Slot: domain.AtomSlotPrimary, AtomType: domain.AtomTypeText, Priority: 4, Required: false},
		{Name: "price", Slot: domain.AtomSlotPrice, AtomType: domain.AtomTypePrice, Priority: 5, Required: true},
		{Name: "rating", Slot: domain.AtomSlotPrimary, AtomType: domain.AtomTypeRating, Priority: 6, Required: false},
	},
}

// ServiceListPreset for displaying services in compact list
var ServiceListPreset = domain.Preset{
	Name:        string(domain.PresetServiceList),
	EntityType:  domain.EntityTypeService,
	Template:    WidgetTemplateServiceCard,
	DefaultMode: domain.FormationTypeList,
	DefaultSize: domain.WidgetSizeSmall,
	Fields: []domain.FieldConfig{
		{Name: "name", Slot: domain.AtomSlotTitle, AtomType: domain.AtomTypeText, Priority: 1, Required: true},
		{Name: "duration", Slot: domain.AtomSlotPrimary, AtomType: domain.AtomTypeText, Priority: 2, Required: false},
		{Name: "price", Slot: domain.AtomSlotPrice, AtomType: domain.AtomTypePrice, Priority: 3, Required: true},
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
	Fields: []domain.FieldConfig{
		{Name: "images", Slot: domain.AtomSlotGallery, AtomType: domain.AtomTypeImage, Priority: 1, Required: false},
		{Name: "name", Slot: domain.AtomSlotTitle, AtomType: domain.AtomTypeText, Priority: 2, Required: true},
		{Name: "provider", Slot: domain.AtomSlotPrimary, AtomType: domain.AtomTypeText, Priority: 3, Required: false},
		{Name: "duration", Slot: domain.AtomSlotPrimary, AtomType: domain.AtomTypeText, Priority: 4, Required: false},
		{Name: "rating", Slot: domain.AtomSlotPrimary, AtomType: domain.AtomTypeRating, Priority: 5, Required: false},
		{Name: "price", Slot: domain.AtomSlotPrice, AtomType: domain.AtomTypePrice, Priority: 6, Required: true},
		{Name: "availability", Slot: domain.AtomSlotStock, AtomType: domain.AtomTypeText, Priority: 7, Required: false},
		{Name: "description", Slot: domain.AtomSlotDescription, AtomType: domain.AtomTypeText, Priority: 8, Required: false},
		{Name: "attributes", Slot: domain.AtomSlotSpecs, AtomType: domain.AtomTypeText, Priority: 9, Required: false},
	},
}
