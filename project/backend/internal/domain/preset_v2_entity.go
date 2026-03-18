package domain

// PresetV2 is a v2 preset: a snapshot of atom configs + rules + optional structure.
type PresetV2 struct {
	Name        string         `json:"name"`
	EntityType  EntityType     `json:"entityType"`
	Template    string         `json:"template"`
	DefaultMode FormationType  `json:"defaultMode"`
	DefaultSize WidgetSize     `json:"defaultSize"`
	Fields      []PresetV2Field `json:"fields"`
	Structure   *LayoutNode    `json:"structure,omitempty"` // Optional layout override
}

// PresetV2Field defines a field in a v2 preset.
type PresetV2Field struct {
	FieldName  string         `json:"fieldName"`
	TextStyle  *TextStyle     `json:"textStyle,omitempty"`
	Wrapper    *WrapperConfig `json:"wrapper,omitempty"`
	Format     AtomFormat     `json:"format,omitempty"`
	Slot       AtomSlot       `json:"slot,omitempty"`
	Priority   int            `json:"priority"`
	Rigidity   Rigidity       `json:"rigidity,omitempty"`
}
