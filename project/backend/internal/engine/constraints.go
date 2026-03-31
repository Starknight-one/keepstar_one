package engine

import (
	"net/url"
	"unicode/utf8"

	"keepstar/internal/domain"
)

// ============================================================================
// Level 0 -- Data Sanitization (applied in field getters / buildAtoms)
// ============================================================================

// D5: Truncate text by slot and layout
func TruncateBySlot(value string, slot domain.AtomSlot, layout string) string {
	maxLen := 0
	switch slot {
	case domain.AtomSlotSecondary:
		switch layout {
		case "grid":
			maxLen = 120
		case "list":
			maxLen = 200
		default:
			maxLen = 300
		}
	case domain.AtomSlotTitle:
		maxLen = 100
	case domain.AtomSlotPrimary:
		maxLen = 60
	}
	if maxLen > 0 && utf8.RuneCountInString(value) > maxLen {
		runes := []rune(value)
		return string(runes[:maxLen-1]) + "\u2026"
	}
	return value
}

// D7: Validate image URL -- return empty string if invalid
func ValidateImageURL(rawURL interface{}) interface{} {
	switch v := rawURL.(type) {
	case string:
		if !isValidImageURL(v) {
			return nil
		}
		return v
	case []interface{}:
		valid := make([]interface{}, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && isValidImageURL(s) {
				valid = append(valid, s)
			}
		}
		if len(valid) == 0 {
			return nil
		}
		return valid
	case []string:
		valid := make([]string, 0, len(v))
		for _, s := range v {
			if isValidImageURL(s) {
				valid = append(valid, s)
			}
		}
		if len(valid) == 0 {
			return nil
		}
		return valid
	default:
		return nil
	}
}

func isValidImageURL(s string) bool {
	if s == "" {
		return false
	}
	u, err := url.Parse(s)
	if err != nil {
		return false
	}
	return u.Scheme == "http" || u.Scheme == "https"
}
