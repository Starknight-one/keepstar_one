package domain

// AtomType defines the type of atomic UI element
type AtomType string

const (
	AtomTypeText     AtomType = "text"
	AtomTypeNumber   AtomType = "number"
	AtomTypePrice    AtomType = "price"
	AtomTypeImage    AtomType = "image"
	AtomTypeRating   AtomType = "rating"
	AtomTypeBadge    AtomType = "badge"
	AtomTypeButton   AtomType = "button"
	AtomTypeIcon     AtomType = "icon"
	AtomTypeDivider  AtomType = "divider"
	AtomTypeProgress AtomType = "progress"
	AtomTypeSelector AtomType = "selector" // For multiple choice values (sizes, colors)
)

// AtomSlot defines where atom should be placed in template
type AtomSlot string

const (
	AtomSlotHero        AtomSlot = "hero"        // Main image/carousel
	AtomSlotBadge       AtomSlot = "badge"       // Badge overlay
	AtomSlotTitle       AtomSlot = "title"       // Product title
	AtomSlotPrimary     AtomSlot = "primary"     // Primary attributes (shown immediately)
	AtomSlotPrice       AtomSlot = "price"       // Price block
	AtomSlotSecondary   AtomSlot = "secondary"   // Secondary attributes (expandable)
	AtomSlotGallery     AtomSlot = "gallery"     // Full gallery (not just hero)
	AtomSlotStock       AtomSlot = "stock"       // Availability indicator
	AtomSlotDescription AtomSlot = "description" // Full description block
	AtomSlotTags        AtomSlot = "tags"        // Tags chips
	AtomSlotSpecs       AtomSlot = "specs"       // Specifications table
)

// Atom is the smallest UI building block
type Atom struct {
	Type  AtomType               `json:"type"`
	Value interface{}            `json:"value"`
	Slot  AtomSlot               `json:"slot,omitempty"` // Template slot hint
	Meta  map[string]interface{} `json:"meta,omitempty"`
}
