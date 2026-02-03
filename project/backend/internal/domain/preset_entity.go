package domain

// FieldConfig defines how a field maps to an atom in a slot
type FieldConfig struct {
	Name     string   `json:"name"`     // field name: "price", "rating", "duration"
	Slot     AtomSlot `json:"slot"`     // target slot: hero, title, primary, etc.
	AtomType AtomType `json:"atomType"` // how to render: text, price, rating, image
	Priority int      `json:"priority"` // higher = show first
	Required bool     `json:"required"` // must include
}

// SlotConfig defines constraints for a slot
type SlotConfig struct {
	MaxAtoms     int        `json:"maxAtoms"`
	AllowedTypes []AtomType `json:"allowedTypes"`
}

// Preset defines how to render entities of a certain type
type Preset struct {
	Name        string                 `json:"name"`        // "product_grid", "service_card"
	EntityType  EntityType             `json:"entityType"`
	Template    string                 `json:"template"`    // widget template name
	Slots       map[AtomSlot]SlotConfig `json:"slots"`
	Fields      []FieldConfig          `json:"fields"`
	DefaultMode FormationType          `json:"defaultMode"` // grid, list, carousel
	DefaultSize WidgetSize             `json:"defaultSize"` // small, medium, large
}

// PresetName is the identifier for a preset
type PresetName string

const (
	PresetProductGrid    PresetName = "product_grid"
	PresetProductCard    PresetName = "product_card"
	PresetProductCompact PresetName = "product_compact"
	PresetServiceCard    PresetName = "service_card"
	PresetServiceList    PresetName = "service_list"
)
