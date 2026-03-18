package presets

import (
	"keepstar/internal/domain"
)

// PresetV2Registry stores v2 presets.
type PresetV2Registry struct {
	presets map[string]domain.PresetV2
}

// NewPresetV2Registry creates a registry with default v2 presets.
func NewPresetV2Registry() *PresetV2Registry {
	r := &PresetV2Registry{
		presets: make(map[string]domain.PresetV2),
	}
	r.registerDefaults()
	return r
}

// Get returns a v2 preset by name.
func (r *PresetV2Registry) Get(name string) (domain.PresetV2, bool) {
	p, ok := r.presets[name]
	return p, ok
}

// Register adds a v2 preset.
func (r *PresetV2Registry) Register(p domain.PresetV2) {
	r.presets[p.Name] = p
}

func (r *PresetV2Registry) registerDefaults() {
	// Product Card Grid — compact card for grid views
	r.Register(domain.PresetV2{
		Name:        "product_card_grid",
		EntityType:  domain.EntityTypeProduct,
		Template:    "GenericCard",
		DefaultMode: domain.FormationTypeGrid,
		DefaultSize: domain.WidgetSizeMedium,
		Fields: []domain.PresetV2Field{
			{FieldName: "images", Slot: domain.AtomSlotHero, Priority: 0, Rigidity: domain.RigidityPreferred},
			{FieldName: "name", TextStyle: &domain.TextStyle{FontSize: "2xl", FontWeight: "semibold"}, Slot: domain.AtomSlotTitle, Priority: 1, Rigidity: domain.RigidityPreferred},
			{FieldName: "brand", Wrapper: &domain.WrapperConfig{Type: "tag"}, Slot: domain.AtomSlotPrimary, Priority: 4, Rigidity: domain.RigidityFlexible},
			{FieldName: "price", Format: domain.FormatCurrency, TextStyle: &domain.TextStyle{FontSize: "lg", FontWeight: "bold"}, Slot: domain.AtomSlotPrice, Priority: 2, Rigidity: domain.RigidityPreferred},
			{FieldName: "rating", Format: domain.FormatStarsCompact, TextStyle: &domain.TextStyle{FontSize: "sm"}, Slot: domain.AtomSlotPrimary, Priority: 3, Rigidity: domain.RigidityFlexible},
		},
	})

	// Product Detail — full detail view
	r.Register(domain.PresetV2{
		Name:        "product_card_detail",
		EntityType:  domain.EntityTypeProduct,
		Template:    "GenericCard",
		DefaultMode: domain.FormationTypeSingle,
		DefaultSize: domain.WidgetSizeLarge,
		Fields: []domain.PresetV2Field{
			{FieldName: "images", Slot: domain.AtomSlotHero, Priority: 0, Rigidity: domain.RigidityPreferred},
			{FieldName: "name", TextStyle: &domain.TextStyle{FontSize: "2xl", FontWeight: "semibold"}, Slot: domain.AtomSlotTitle, Priority: 1, Rigidity: domain.RigidityPreferred},
			{FieldName: "brand", Wrapper: &domain.WrapperConfig{Type: "tag"}, Slot: domain.AtomSlotPrimary, Priority: 4, Rigidity: domain.RigidityFlexible},
			{FieldName: "category", Wrapper: &domain.WrapperConfig{Type: "tag"}, Slot: domain.AtomSlotPrimary, Priority: 5, Rigidity: domain.RigidityFlexible},
			{FieldName: "price", Format: domain.FormatCurrency, TextStyle: &domain.TextStyle{FontSize: "xl", FontWeight: "bold"}, Slot: domain.AtomSlotPrice, Priority: 2, Rigidity: domain.RigidityPreferred},
			{FieldName: "rating", Format: domain.FormatStarsCompact, TextStyle: &domain.TextStyle{FontSize: "sm"}, Slot: domain.AtomSlotPrimary, Priority: 3, Rigidity: domain.RigidityFlexible},
			{FieldName: "description", TextStyle: &domain.TextStyle{FontSize: "sm"}, Slot: domain.AtomSlotSecondary, Priority: 6, Rigidity: domain.RigidityFlexible},
			{FieldName: "tags", Wrapper: &domain.WrapperConfig{Type: "tag"}, Slot: domain.AtomSlotSecondary, Priority: 7, Rigidity: domain.RigidityFlexible},
			{FieldName: "stockQuantity", TextStyle: &domain.TextStyle{FontSize: "sm"}, Slot: domain.AtomSlotSecondary, Priority: 8, Rigidity: domain.RigidityFlexible},
		},
	})

	// Product Row — compact list item
	r.Register(domain.PresetV2{
		Name:        "product_row",
		EntityType:  domain.EntityTypeProduct,
		Template:    "GenericCard",
		DefaultMode: domain.FormationTypeList,
		DefaultSize: domain.WidgetSizeSmall,
		Fields: []domain.PresetV2Field{
			{FieldName: "name", TextStyle: &domain.TextStyle{FontSize: "md", FontWeight: "medium"}, Slot: domain.AtomSlotTitle, Priority: 0, Rigidity: domain.RigidityPreferred},
			{FieldName: "price", Format: domain.FormatCurrency, TextStyle: &domain.TextStyle{FontSize: "md", FontWeight: "bold"}, Slot: domain.AtomSlotPrice, Priority: 1, Rigidity: domain.RigidityPreferred},
		},
	})

	// Service Card Grid
	r.Register(domain.PresetV2{
		Name:        "service_card",
		EntityType:  domain.EntityTypeService,
		Template:    "GenericCard",
		DefaultMode: domain.FormationTypeGrid,
		DefaultSize: domain.WidgetSizeMedium,
		Fields: []domain.PresetV2Field{
			{FieldName: "images", Slot: domain.AtomSlotHero, Priority: 0, Rigidity: domain.RigidityPreferred},
			{FieldName: "name", TextStyle: &domain.TextStyle{FontSize: "2xl", FontWeight: "semibold"}, Slot: domain.AtomSlotTitle, Priority: 1, Rigidity: domain.RigidityPreferred},
			{FieldName: "provider", TextStyle: &domain.TextStyle{FontSize: "md"}, Slot: domain.AtomSlotPrimary, Priority: 4, Rigidity: domain.RigidityFlexible},
			{FieldName: "duration", TextStyle: &domain.TextStyle{FontSize: "md"}, Slot: domain.AtomSlotPrimary, Priority: 5, Rigidity: domain.RigidityFlexible},
			{FieldName: "price", Format: domain.FormatCurrency, TextStyle: &domain.TextStyle{FontSize: "lg", FontWeight: "bold"}, Slot: domain.AtomSlotPrice, Priority: 2, Rigidity: domain.RigidityPreferred},
			{FieldName: "rating", Format: domain.FormatStarsCompact, TextStyle: &domain.TextStyle{FontSize: "sm"}, Slot: domain.AtomSlotPrimary, Priority: 3, Rigidity: domain.RigidityFlexible},
		},
	})

	// Service Detail
	r.Register(domain.PresetV2{
		Name:        "service_detail",
		EntityType:  domain.EntityTypeService,
		Template:    "GenericCard",
		DefaultMode: domain.FormationTypeSingle,
		DefaultSize: domain.WidgetSizeLarge,
		Fields: []domain.PresetV2Field{
			{FieldName: "images", Slot: domain.AtomSlotHero, Priority: 0, Rigidity: domain.RigidityPreferred},
			{FieldName: "name", TextStyle: &domain.TextStyle{FontSize: "2xl", FontWeight: "semibold"}, Slot: domain.AtomSlotTitle, Priority: 1, Rigidity: domain.RigidityPreferred},
			{FieldName: "provider", TextStyle: &domain.TextStyle{FontSize: "md"}, Slot: domain.AtomSlotPrimary, Priority: 4, Rigidity: domain.RigidityFlexible},
			{FieldName: "duration", TextStyle: &domain.TextStyle{FontSize: "md"}, Slot: domain.AtomSlotPrimary, Priority: 5, Rigidity: domain.RigidityFlexible},
			{FieldName: "availability", TextStyle: &domain.TextStyle{FontSize: "md"}, Slot: domain.AtomSlotPrimary, Priority: 6, Rigidity: domain.RigidityFlexible},
			{FieldName: "price", Format: domain.FormatCurrency, TextStyle: &domain.TextStyle{FontSize: "xl", FontWeight: "bold"}, Slot: domain.AtomSlotPrice, Priority: 2, Rigidity: domain.RigidityPreferred},
			{FieldName: "rating", Format: domain.FormatStarsCompact, TextStyle: &domain.TextStyle{FontSize: "sm"}, Slot: domain.AtomSlotPrimary, Priority: 3, Rigidity: domain.RigidityFlexible},
			{FieldName: "description", TextStyle: &domain.TextStyle{FontSize: "sm"}, Slot: domain.AtomSlotSecondary, Priority: 7, Rigidity: domain.RigidityFlexible},
			{FieldName: "attributes", TextStyle: &domain.TextStyle{FontSize: "sm"}, Slot: domain.AtomSlotSecondary, Priority: 8, Rigidity: domain.RigidityFlexible},
		},
	})
}
