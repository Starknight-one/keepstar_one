package tools

import "keepstar/internal/domain"

// ResolvedDefaults holds auto-resolved layout, size, and field configuration
type ResolvedDefaults struct {
	Layout    string
	Size      domain.WidgetSize
	MaxFields int
	Fields    []string
}

// fieldRanking defines fields ordered by visual priority per entity type
var fieldRanking = map[string][]string{
	"product": {"images", "name", "price", "rating", "brand", "category",
		"description", "tags", "stockQuantity", "attributes"},
	"service": {"images", "name", "price", "rating", "duration", "provider",
		"availability", "description", "attributes"},
}

// defaultDisplay maps field name to its default display style
var defaultDisplay = map[string]string{
	"images":        "image-cover",
	"name":          "h2",
	"price":         "price",
	"rating":        "rating-compact",
	"brand":         "tag",
	"category":      "tag",
	"description":   "body-sm",
	"tags":          "tag",
	"duration":      "body",
	"provider":      "body",
	"availability":  "body",
	"stockQuantity":  "body-sm",
	"attributes":     "body-sm",
	"productForm":    "tag",
	"skinType":       "tag",
	"concern":        "tag",
	"keyIngredients": "body-sm",
}

// defaultSlot maps field name to its default slot
var defaultSlot = map[string]domain.AtomSlot{
	"images":        domain.AtomSlotHero,
	"name":          domain.AtomSlotTitle,
	"price":         domain.AtomSlotPrice,
	"rating":        domain.AtomSlotPrimary,
	"brand":         domain.AtomSlotPrimary,
	"category":      domain.AtomSlotPrimary,
	"description":   domain.AtomSlotSecondary,
	"tags":          domain.AtomSlotSecondary,
	"duration":      domain.AtomSlotPrimary,
	"provider":      domain.AtomSlotPrimary,
	"availability":  domain.AtomSlotPrimary,
	"stockQuantity":  domain.AtomSlotSecondary,
	"attributes":     domain.AtomSlotSecondary,
	"productForm":    domain.AtomSlotSecondary,
	"skinType":       domain.AtomSlotSecondary,
	"concern":        domain.AtomSlotSecondary,
	"keyIngredients": domain.AtomSlotSecondary,
}

// MaxAtomsPerSize limits how many atoms are shown per widget size
var MaxAtomsPerSize = map[string]int{
	"tiny":   2,
	"small":  3,
	"medium": 5,
	"large":  10,
}

// validDisplaysForType maps AtomType → set of valid display strings
var validDisplaysForType = map[domain.AtomType]map[string]bool{
	domain.AtomTypeText: {
		"h1": true, "h2": true, "h3": true, "h4": true,
		"body-lg": true, "body": true, "body-sm": true, "caption": true,
		"badge": true, "badge-success": true, "badge-error": true, "badge-warning": true,
		"tag": true, "tag-active": true,
		"button-primary": true, "button-secondary": true, "button-outline": true,
		"divider": true, "spacer": true,
	},
	domain.AtomTypeNumber: {
		"price": true, "price-lg": true, "price-old": true, "price-discount": true,
		"rating": true, "rating-text": true, "rating-compact": true,
		"percent": true, "progress": true,
		"body": true, "body-sm": true, "h3": true, "h4": true,
	},
	domain.AtomTypeImage: {
		"image": true, "image-cover": true, "avatar": true, "avatar-sm": true, "avatar-lg": true,
		"thumbnail": true, "gallery": true,
	},
	domain.AtomTypeIcon: {
		"icon": true, "icon-sm": true, "icon-lg": true,
	},
}

// ValidateDisplay checks if a display value is valid for the given atom type.
// Returns the display if valid, or the default display for the field if invalid.
func ValidateDisplay(fieldName string, atomType domain.AtomType, display string) string {
	if display == "" {
		return defaultDisplay[fieldName]
	}
	validSet, ok := validDisplaysForType[atomType]
	if !ok {
		return display // unknown type, allow anything
	}
	if validSet[display] {
		return display
	}
	// Fallback to default display for this field
	if d := defaultDisplay[fieldName]; d != "" {
		return d
	}
	return "body"
}

// AutoResolve determines layout, size, and max visible fields from entity count
func AutoResolve(entityType string, entityCount int) ResolvedDefaults {
	if entityCount == 0 {
		return ResolvedDefaults{}
	}

	ranking := fieldRanking[entityType]
	if ranking == nil {
		ranking = fieldRanking["product"] // fallback
	}

	switch {
	case entityCount == 1:
		return ResolvedDefaults{
			Layout:    "single",
			Size:      domain.WidgetSizeLarge,
			MaxFields: 10,
			Fields:    ranking,
		}
	case entityCount <= 6:
		// 2-6 items: grid with top 5 fields
		maxF := 5
		fields := ranking
		if len(fields) > maxF {
			fields = fields[:maxF]
		}
		return ResolvedDefaults{
			Layout:    "grid",
			Size:      domain.WidgetSizeMedium,
			MaxFields: maxF,
			Fields:    fields,
		}
	case entityCount <= 12:
		// 7-12 items: list with top 4 fields
		maxF := 4
		fields := ranking
		if len(fields) > maxF {
			fields = fields[:maxF]
		}
		return ResolvedDefaults{
			Layout:    "list",
			Size:      domain.WidgetSizeSmall,
			MaxFields: maxF,
			Fields:    fields,
		}
	default:
		// 13+ items: list with top 3 fields
		maxF := 3
		fields := ranking
		if len(fields) > maxF {
			fields = fields[:maxF]
		}
		return ResolvedDefaults{
			Layout:    "list",
			Size:      domain.WidgetSizeTiny,
			MaxFields: maxF,
			Fields:    fields,
		}
	}
}

// BuildFieldConfigs converts field names + display overrides into FieldConfig slice
func BuildFieldConfigs(fields []string, displayOverrides map[string]string) []domain.FieldConfig {
	configs := make([]domain.FieldConfig, 0, len(fields))

	for i, name := range fields {
		entry, known := fieldTypeMap[name]
		if !known {
			entry = fieldTypeEntry{domain.AtomTypeText, domain.SubtypeString}
		}

		display := defaultDisplay[name]
		if override, ok := displayOverrides[name]; ok {
			display = override
		}
		display = ValidateDisplay(name, entry.Type, display)

		slot := defaultSlot[name]
		if slot == "" {
			slot = domain.AtomSlotPrimary
		}

		configs = append(configs, domain.FieldConfig{
			Name:     name,
			Slot:     slot,
			AtomType: entry.Type,
			Subtype:  entry.Subtype,
			Display:  domain.AtomDisplay(display),
			Priority: i,
		})
	}

	return configs
}
