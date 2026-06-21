package domain

// Theme is the design-system token payload that rides on every rendered
// Document as the top-level `theme` field. The widget shadow-root injector
// (project_v5/frontend) reads it and emits a `:host{ --kw-*: ... }` style
// block AFTER the static widget.css so tenant overrides win. It is an INERT
// passenger to the engine — no engine struct reads a top-level singular
// `theme`, so it round-trips untouched to the wire.
//
// IsDefault is true when no v5_themes row exists for the tenant: the tokens
// are then the canonical defaults below, which are byte-identical to the
// hardcoded widget.css values, giving ZERO visual change.
type Theme struct {
	Tokens    ThemeTokens `json:"tokens"`
	IsDefault bool        `json:"isDefault"`
}

// ThemeTokens is the canonical --kw-* token set. The shape mirrors the
// frontend CSS variable groups one-to-one (see the CSS variable mapping in
// the Phase 1 contract and widget.css:16-46).
type ThemeTokens struct {
	Colors     map[string]string `json:"colors"`
	Radius     map[string]int    `json:"radius"`
	Gap        map[string]int    `json:"gap"`
	FontSize   map[string]int    `json:"fontSize"`
	FontWeight map[string]int    `json:"fontWeight"`
}

// DefaultThemeTokens returns the canonical default token set.
//
// INVARIANT (byte-identical): every value here MUST equal the hardcoded
// widget.css fallback at project_v5/frontend/src/widget.css:16-46. If those
// CSS values change, update these constants too (and TestDefaultThemeTokens
// will fail until you do). Cross-link:
//
//	colors.blue       -> --kw-blue        widget.css:17  (#5BA4D9)
//	colors.orange     -> --kw-orange      widget.css:18  (#F0924A)
//	colors.bg         -> --kw-bg          widget.css:19  (#ffffff)
//	colors.text       -> --kw-text        widget.css:20  (#1a1a1a)
//	colors.text_muted -> --kw-text-muted  widget.css:21  (#666666)
//	colors.border     -> --kw-border      widget.css:22  (#e5e7eb)
//	colors.line       -> --kw-line        widget.css:23  (#e5e7eb)
//	radius.base       -> --kw-radius       widget.css:24  (8px)
//	gap.{k}           -> --kw-gap-{k}      widget.css:26-30 (4/8/16/24/32)
//	fontSize.{k}      -> --kw-fs-{k}       widget.css:32-39 (11/13/14/15/18/22/28/36)
//	fontWeight.{k}    -> --kw-fw-{k}       widget.css:41-45 (300/400/500/600/700)
//
// A fresh map is returned on every call so callers can mutate (merge) freely.
func DefaultThemeTokens() ThemeTokens {
	return ThemeTokens{
		Colors: map[string]string{
			"blue":       "#5BA4D9",
			"orange":     "#F0924A",
			"bg":         "#ffffff",
			"text":       "#1a1a1a",
			"text_muted": "#666666",
			"border":     "#e5e7eb",
			"line":       "#e5e7eb",
		},
		Radius: map[string]int{
			"base": 8,
		},
		Gap: map[string]int{
			"xs": 4,
			"sm": 8,
			"md": 16,
			"lg": 24,
			"xl": 32,
		},
		FontSize: map[string]int{
			"xs":   11,
			"sm":   13,
			"base": 14,
			"md":   15,
			"lg":   18,
			"xl":   22,
			"2xl":  28,
			"3xl":  36,
		},
		FontWeight: map[string]int{
			"light":    300,
			"normal":   400,
			"medium":   500,
			"semibold": 600,
			"bold":     700,
		},
	}
}

// DefaultTheme returns the canonical default Theme (IsDefault=true). Used at
// every attach point when no tenant v5_themes row exists, so the document
// always carries a COMPLETE, explicit token set.
func DefaultTheme() Theme {
	return Theme{Tokens: DefaultThemeTokens(), IsDefault: true}
}

// MergeThemeTokensOverDefaults overlays a partial tenant token set on top of
// the canonical defaults, key by key, so the result is always COMPLETE — a
// tenant that only set colors.blue still gets every other --kw-* token. A nil
// override returns a fresh copy of the defaults.
func MergeThemeTokensOverDefaults(override ThemeTokens) ThemeTokens {
	out := DefaultThemeTokens()
	mergeStr(out.Colors, override.Colors)
	mergeInt(out.Radius, override.Radius)
	mergeInt(out.Gap, override.Gap)
	mergeInt(out.FontSize, override.FontSize)
	mergeInt(out.FontWeight, override.FontWeight)
	return out
}

func mergeStr(dst, src map[string]string) {
	for k, v := range src {
		dst[k] = v
	}
}

func mergeInt(dst, src map[string]int) {
	for k, v := range src {
		dst[k] = v
	}
}
