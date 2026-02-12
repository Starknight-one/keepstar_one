package domain

// AtomType defines the 6 base types of atomic data
type AtomType string

const (
	AtomTypeText   AtomType = "text"
	AtomTypeNumber AtomType = "number"
	AtomTypeImage  AtomType = "image"
	AtomTypeIcon   AtomType = "icon"
	AtomTypeVideo  AtomType = "video"
	AtomTypeAudio  AtomType = "audio"
)

// AtomSubtype defines the data format within a type
type AtomSubtype string

const (
	// text subtypes
	SubtypeString   AtomSubtype = "string"
	SubtypeDate     AtomSubtype = "date"
	SubtypeDatetime AtomSubtype = "datetime"
	SubtypeURL      AtomSubtype = "url"
	SubtypeEmail    AtomSubtype = "email"
	SubtypePhone    AtomSubtype = "phone"

	// number subtypes
	SubtypeInt      AtomSubtype = "int"
	SubtypeFloat    AtomSubtype = "float"
	SubtypeCurrency AtomSubtype = "currency"
	SubtypePercent  AtomSubtype = "percent"
	SubtypeRating   AtomSubtype = "rating"

	// image subtypes
	SubtypeImageURL    AtomSubtype = "url"
	SubtypeImageBase64 AtomSubtype = "base64"

	// icon subtypes
	SubtypeIconName  AtomSubtype = "name"
	SubtypeIconEmoji AtomSubtype = "emoji"
	SubtypeIconSVG   AtomSubtype = "svg"
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

// Atom is the smallest UI building block with type, subtype, and display
type Atom struct {
	Type      AtomType               `json:"type"`
	Subtype   AtomSubtype            `json:"subtype,omitempty"`
	Display   string                 `json:"display,omitempty"`   // Visual format (e.g., "h1", "price-lg", "badge-success")
	Value     interface{}            `json:"value"`
	FieldName string                 `json:"fieldName,omitempty"` // Source field name (only in template atoms)
	Slot      AtomSlot               `json:"slot,omitempty"`      // Template slot hint
	Meta      map[string]interface{} `json:"meta,omitempty"`
}

// Legacy type mappings for backward compatibility
// These map old atom types to new type + subtype combinations
var LegacyTypeMapping = map[string]struct {
	Type    AtomType
	Subtype AtomSubtype
	Display string
}{
	"price":    {AtomTypeNumber, SubtypeCurrency, "price"},
	"rating":   {AtomTypeNumber, SubtypeRating, "rating"},
	"badge":    {AtomTypeText, SubtypeString, "badge"},
	"button":   {AtomTypeText, SubtypeString, "button-primary"},
	"divider":  {AtomTypeText, SubtypeString, "divider"},
	"progress": {AtomTypeNumber, SubtypePercent, "progress"},
	"selector": {AtomTypeText, SubtypeString, "tag"},
}
