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

// AtomFormat defines how the raw value is transformed into display text
type AtomFormat string

const (
	FormatCurrency     AtomFormat = "currency"      // "$329.00"
	FormatStars        AtomFormat = "stars"          // "★★★★☆"
	FormatStarsText    AtomFormat = "stars-text"     // "4.2/5"
	FormatStarsCompact AtomFormat = "stars-compact"  // "★ 4.2"
	FormatPercent      AtomFormat = "percent"        // "85%"
	FormatNumber       AtomFormat = "number"         // "329"
	FormatDate         AtomFormat = "date"           // "Feb 25, 2026"
	FormatText         AtomFormat = "text"           // as-is
)

// Atom is the smallest UI building block with type, subtype, format, and display
type Atom struct {
	Type      AtomType               `json:"type"`
	Subtype   AtomSubtype            `json:"subtype,omitempty"`
	Format    AtomFormat             `json:"format,omitempty"`    // Value transform (e.g., "currency", "stars-compact")
	Display   string                 `json:"display,omitempty"`   // Visual wrapper (e.g., "h1", "badge", "tag")
	Value     interface{}            `json:"value"`
	FieldName string                 `json:"fieldName,omitempty"` // Source field name (only in template atoms)
	Slot      AtomSlot               `json:"slot,omitempty"`      // Template slot hint
	Meta      map[string]interface{} `json:"meta,omitempty"`
}

// ============================================================================
// Engine V2 types
// ============================================================================

// Rigidity controls how the engine may adjust an atom/group/widget value.
type Rigidity string

const (
	RigidityLocked    Rigidity = "locked"    // User explicitly asked — engine never touches
	RigidityPreferred Rigidity = "preferred" // Preset set — engine can adjust if needed
	RigidityFlexible  Rigidity = "flexible"  // Default — engine freely adjusts
)

// TextStyle defines typography without visual container (engine v2).
// Separates "how text looks" from "what wraps it".
type TextStyle struct {
	FontSize       string `json:"fontSize,omitempty"`       // Semantic token: "xs","sm","md","lg","xl","2xl","3xl"
	FontWeight     string `json:"fontWeight,omitempty"`     // "light","normal","medium","semibold","bold"
	Color          string `json:"color,omitempty"`          // Semantic color token or hex
	TextDecoration string `json:"textDecoration,omitempty"` // "none","underline","line-through"
	TextTransform  string `json:"textTransform,omitempty"`  // "none","uppercase","lowercase","capitalize"
	LineClamp      int    `json:"lineClamp,omitempty"`      // Max visible lines (0 = unlimited)
	Truncate       int    `json:"truncate,omitempty"`       // Max chars (0 = unlimited)
	LineHeight     string `json:"lineHeight,omitempty"`     // "tight","normal","relaxed","loose"
	LetterSpacing  string `json:"letterSpacing,omitempty"`  // "tight","normal","wide"
}

// MediaStyle defines styling for image/video/audio atoms (engine v2).
type MediaStyle struct {
	AspectRatio string `json:"aspectRatio,omitempty"` // "1:1", "4:3", "16:9", "auto"
	ObjectFit   string `json:"objectFit,omitempty"`   // "cover", "contain", "fill"
	Controls    bool   `json:"controls,omitempty"`
	Autoplay    bool   `json:"autoplay,omitempty"`
	Muted       bool   `json:"muted,omitempty"`
	Poster      string `json:"poster,omitempty"` // URL for video poster
}

// IconStyle defines styling for icon atoms (engine v2).
type IconStyle struct {
	Size  string `json:"size,omitempty"`  // xs/sm/md/lg/xl
	Color string `json:"color,omitempty"` // semantic or hex
	Style string `json:"style,omitempty"` // "stroke" / "fill"
}

// WrapperConfig defines the visual container around an atom value (engine v2).
// Separates "what wraps it" from "how text looks".
type WrapperConfig struct {
	Type         string   `json:"type"`                   // "none","badge","tag","pill","avatar","tooltip","alert","link","progress","button"
	Variant      string   `json:"variant,omitempty"`      // Wrapper-specific: "success","error","warning","primary","secondary","outline"
	Rigidity     Rigidity `json:"rigidity,omitempty"`
	Background   string   `json:"background,omitempty"`   // CSS background value
	BorderRadius string   `json:"borderRadius,omitempty"` // Token: none/sm/md/lg/xl/full
	Padding      string   `json:"padding,omitempty"`      // Token: none/xs/sm/md/lg/xl
	ContentFit   string   `json:"contentFit,omitempty"`   // "hug" / "fill"
	Margin       string   `json:"margin,omitempty"`       // Token: xs/sm/md/lg
}

// AtomV2 is the engine v2 atom with separated textStyle and wrapper.
type AtomV2 struct {
	ID         string                 `json:"id,omitempty"`         // Stable addressable ID for tree operations
	Type       AtomType               `json:"type"`
	Subtype    AtomSubtype            `json:"subtype,omitempty"`
	Value      interface{}            `json:"value"`
	Label      string                 `json:"label,omitempty"`      // Human-readable field label ("Price", "Brand")
	Unit       string                 `json:"unit,omitempty"`       // "RUB", "USD", "min", "kg"
	Format     AtomFormat             `json:"format,omitempty"`     // Value transform: "currency", "stars-compact"
	TextStyle  *TextStyle             `json:"textStyle,omitempty"`  // Typography (separated from wrapper)
	Wrapper    *WrapperConfig         `json:"wrapper,omitempty"`    // Visual container (separated from textStyle)
	MediaStyle *MediaStyle            `json:"mediaStyle,omitempty"` // Image/video/audio styling
	IconStyle  *IconStyle             `json:"iconStyle,omitempty"`  // Icon styling
	Slot       AtomSlot               `json:"slot,omitempty"`
	Rigidity   Rigidity               `json:"rigidity,omitempty"`   // How freely engine can adjust this atom
	FieldName  string                 `json:"fieldName,omitempty"`
	Priority   int                    `json:"priority,omitempty"`
	Meta       map[string]interface{} `json:"meta,omitempty"`
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
