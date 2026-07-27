package presets

import (
	"fmt"
	"sort"
	"strings"
)

// SystemPresetDescriptions maps each system preset name to the one-line
// description shown in Agent2's system prompt.
// To add a new system preset: add the seed JSON to SystemPresetSeeds in
// presets.go AND add a description here. The prompt updates automatically.
var SystemPresetDescriptions = map[string]string{
	"product_card":              "standard product card for grids of 2–4 items",
	"product_card_compact":      "small product card for dense grids (5+ items)",
	"product_card_horizontal":   "image left, info right (carousels, single feature)",
	"product_card_list_row":     "wide row for list layouts",
	"product_carousel":          "horizontally scrollable card strip — browsing many items (5+)",
	"product_comparison":        "side-by-side columns to compare 2–4 items",
	"product_detail":            "full product detail (vertical, 16:9 hero)",
	"product_detail_accordion":  "product detail with collapsible description/details sections",
	"product_detail_horizontal": "product detail with image-left layout",
	"text_explainer":            "literal-text widget (title + body) for LLM explanations",
	"empty_not_found":           "empty state (\"nothing found\")",
	"error_generic":             "error state",
	"design_system_preview":     "brand design-token preview: color swatches + type scale (onboarding)",
	"uploader_card":             "universal catalog uploader (CSV/JSON, 20 MB); disarmed until the plan is confirmed",
	"operation_card":            "operation explainer card: input → does → output → why (replicates over library results)",
	"registration_form":         "owner account registration form (name / email / password, secure step submit)",
	"booking_form":              "book-a-showing form on a listing: preferred time + contact details",
	"lead_table":                "lead list rows for CRM chat: status, contact, listing, preferred time",
	"lead_detail":               "single lead detail with status-advance buttons",
	"success_plaque":            "operation success plaque, bound from the operation result",
	"surface_links":             "issued surface addresses (storefront + CRM URLs, replicates over surfaceLink)",
	"manifest_summary":          "assembly build receipt rows (replicates over manifestStep)",
}

// SystemPresetsBlock is the preset-catalog section injected into Agent2's
// system prompt. Generated once at startup from SystemPresetSeeds keys +
// SystemPresetDescriptions. Adding a seed entry + a description here is
// all that is needed for a new system preset to appear in the prompt.
var SystemPresetsBlock = buildSystemPresetsBlock()

func buildSystemPresetsBlock() string {
	names := make([]string, 0, len(SystemPresetSeeds))
	for n := range SystemPresetSeeds {
		names = append(names, n)
	}
	sort.Strings(names)

	var b strings.Builder
	for _, name := range names {
		desc := SystemPresetDescriptions[name]
		fmt.Fprintf(&b, "  %-32s— %s\n", name, desc)
	}
	return b.String()
}
