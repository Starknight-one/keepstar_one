package tools

import (
	"keepstar/internal/domain"
)

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
		"description", "tags", "stockQuantity",
		"productForm", "skinType", "concern", "keyIngredients"},
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

// defaultFormatForTypeSubtype maps Type+Subtype → auto-inferred AtomFormat
var defaultFormatForTypeSubtype = map[domain.AtomType]map[domain.AtomSubtype]domain.AtomFormat{
	domain.AtomTypeNumber: {
		domain.SubtypeCurrency: domain.FormatCurrency,
		domain.SubtypeRating:   domain.FormatStarsCompact,
		domain.SubtypePercent:  domain.FormatPercent,
		domain.SubtypeInt:      domain.FormatNumber,
		domain.SubtypeFloat:    domain.FormatNumber,
	},
	domain.AtomTypeText: {
		domain.SubtypeDate:     domain.FormatDate,
		domain.SubtypeDatetime: domain.FormatDate,
		domain.SubtypeString:   domain.FormatText,
	},
}

// InferFormat returns the format for an atom: explicit override > auto from type+subtype
func InferFormat(explicit domain.AtomFormat, atomType domain.AtomType, subtype domain.AtomSubtype) domain.AtomFormat {
	if explicit != "" {
		return explicit
	}
	if subtypeMap, ok := defaultFormatForTypeSubtype[atomType]; ok {
		if f, ok := subtypeMap[subtype]; ok {
			return f
		}
	}
	return domain.FormatText
}

// allValidDisplays is the universal set of valid display values.
// Any display works with any data type (format handles the value transform).
// Only image-only and icon-only displays are restricted to their types.
var allValidDisplays = map[string]bool{
	// text wrappers
	"h1": true, "h2": true, "h3": true, "h4": true,
	"body-lg": true, "body": true, "body-sm": true, "caption": true,
	"badge": true, "badge-success": true, "badge-error": true, "badge-warning": true,
	"tag": true, "tag-active": true,
	"button-primary": true, "button-secondary": true, "button-outline": true,
	"divider": true, "spacer": true,
	// number-origin wrappers (now universal for formatted content)
	"price": true, "price-lg": true, "price-old": true, "price-discount": true,
	"rating": true, "rating-text": true, "rating-compact": true,
	"percent": true, "progress": true,
	// image-only wrappers
	"image": true, "image-cover": true, "avatar": true, "avatar-sm": true, "avatar-lg": true,
	"thumbnail": true, "gallery": true,
	// icon-only wrappers
	"icon": true, "icon-sm": true, "icon-lg": true,
}

// imageOnlyDisplays are displays that only make sense for image atoms
var imageOnlyDisplays = map[string]bool{
	"image": true, "image-cover": true, "avatar": true, "avatar-sm": true, "avatar-lg": true,
	"thumbnail": true, "gallery": true,
}

// iconOnlyDisplays are displays that only make sense for icon atoms
var iconOnlyDisplays = map[string]bool{
	"icon": true, "icon-sm": true, "icon-lg": true,
}

// ValidateDisplay checks if a display value is valid for the given atom type.
// Now universal: any known display is valid for any type, except image-only/icon-only restrictions.
func ValidateDisplay(fieldName string, atomType domain.AtomType, display string) string {
	if display == "" {
		return defaultDisplay[fieldName]
	}
	// Check image-only restriction
	if imageOnlyDisplays[display] && atomType != domain.AtomTypeImage {
		if d := defaultDisplay[fieldName]; d != "" {
			return d
		}
		return "body"
	}
	// Check icon-only restriction
	if iconOnlyDisplays[display] && atomType != domain.AtomTypeIcon {
		if d := defaultDisplay[fieldName]; d != "" {
			return d
		}
		return "body"
	}
	// Any known display is valid
	if allValidDisplays[display] {
		return display
	}
	// Unknown display — fallback
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

	// copyFields returns a safe copy to avoid slice aliasing with the global fieldRanking
	copyFields := func(src []string, maxF int) []string {
		if len(src) > maxF {
			src = src[:maxF]
		}
		cp := make([]string, len(src))
		copy(cp, src)
		return cp
	}

	switch {
	case entityCount == 1:
		return ResolvedDefaults{
			Layout:    "single",
			Size:      domain.WidgetSizeLarge,
			MaxFields: 10,
			Fields:    copyFields(ranking, len(ranking)),
		}
	case entityCount <= 6:
		// 2-6 items: grid with top 5 fields
		return ResolvedDefaults{
			Layout:    "grid",
			Size:      domain.WidgetSizeMedium,
			MaxFields: 5,
			Fields:    copyFields(ranking, 5),
		}
	case entityCount <= 12:
		// 7-12 items: grid with top 3 fields (compact cards)
		return ResolvedDefaults{
			Layout:    "grid",
			Size:      domain.WidgetSizeSmall,
			MaxFields: 3,
			Fields:    copyFields(ranking, 3),
		}
	default:
		// 13+ items: grid with top 3 fields (small size → 3 atoms: image, name, price)
		return ResolvedDefaults{
			Layout:    "grid",
			Size:      domain.WidgetSizeSmall,
			MaxFields: 3,
			Fields:    copyFields(ranking, 3),
		}
	}
}

// DefaultSlotConstraints limits max atoms per slot
var DefaultSlotConstraints = map[domain.AtomSlot]int{
	domain.AtomSlotHero:      1,
	domain.AtomSlotTitle:     1,
	domain.AtomSlotPrice:     2,
	domain.AtomSlotBadge:     3,
	domain.AtomSlotPrimary:   4,
	domain.AtomSlotSecondary: 5,
}

// ApplySlotConstraints limits the number of atoms per slot
func ApplySlotConstraints(configs []domain.FieldConfig) []domain.FieldConfig {
	slotCounts := make(map[domain.AtomSlot]int)
	result := make([]domain.FieldConfig, 0, len(configs))
	for _, fc := range configs {
		maxForSlot, hasLimit := DefaultSlotConstraints[fc.Slot]
		if !hasLimit {
			result = append(result, fc)
			continue
		}
		if slotCounts[fc.Slot] < maxForSlot {
			result = append(result, fc)
			slotCounts[fc.Slot]++
		}
	}
	return result
}

// CalcGridConfig determines optimal grid columns from entity count and widget size
func CalcGridConfig(entityCount int, size domain.WidgetSize) *domain.GridConfig {
	var cols int
	switch {
	case entityCount <= 2:
		cols = entityCount
	case entityCount <= 4:
		cols = 2
	case entityCount <= 9:
		switch size {
		case domain.WidgetSizeLarge, domain.WidgetSizeMedium:
			cols = 2
		default:
			cols = 3
		}
	default:
		switch size {
		case domain.WidgetSizeLarge:
			cols = 3
		case domain.WidgetSizeTiny:
			cols = 4
		default:
			cols = 3
		}
	}
	return &domain.GridConfig{Cols: cols}
}

// GetDisplayMeta returns display hints for all known fields of an entity type
func GetDisplayMeta(entityType string) []domain.FieldDisplayHint {
	ranking := fieldRanking[entityType]
	if ranking == nil {
		ranking = fieldRanking["product"]
	}
	hints := make([]domain.FieldDisplayHint, 0, len(ranking))
	for _, name := range ranking {
		slot := defaultSlot[name]
		category := "detail_only"
		switch slot {
		case domain.AtomSlotHero:
			category = "media"
		case domain.AtomSlotTitle, domain.AtomSlotPrice:
			category = "primary"
		case domain.AtomSlotBadge:
			category = "badge"
		case domain.AtomSlotPrimary:
			category = "tag"
		case domain.AtomSlotSecondary:
			category = "detail_only"
		}
		hints = append(hints, domain.FieldDisplayHint{
			Name:     name,
			Category: category,
			Default:  defaultDisplay[name],
		})
	}
	return hints
}

// BuildFieldConfigs converts field names + display/format overrides into FieldConfig slice
func BuildFieldConfigs(fields []string, displayOverrides map[string]string) []domain.FieldConfig {
	return BuildFieldConfigsWithFormat(fields, displayOverrides, nil)
}

// BuildFieldConfigsWithFormat converts field names + display/format overrides into FieldConfig slice
func BuildFieldConfigsWithFormat(fields []string, displayOverrides map[string]string, formatOverrides map[string]string) []domain.FieldConfig {
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

		// Infer format: explicit override > auto from type+subtype
		var explicitFormat domain.AtomFormat
		if formatOverrides != nil {
			if f, ok := formatOverrides[name]; ok {
				explicitFormat = domain.AtomFormat(f)
			}
		}
		format := InferFormat(explicitFormat, entry.Type, entry.Subtype)

		configs = append(configs, domain.FieldConfig{
			Name:     name,
			Slot:     slot,
			AtomType: entry.Type,
			Subtype:  entry.Subtype,
			Format:   format,
			Display:  domain.AtomDisplay(display),
			Priority: i,
		})
	}

	return configs
}
