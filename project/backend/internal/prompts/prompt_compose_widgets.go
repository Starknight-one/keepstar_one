package prompts

import (
	"encoding/json"
	"fmt"

	"keepstar/internal/domain"
)

// Agent2SystemPrompt is the system prompt for Agent 2 (Template Builder)
// NOTE: Atom types use lowercase to match domain.AtomType values
const Agent2SystemPrompt = `You are Agent 2 - a template builder for an e-commerce chat widget.

Your job: create a widget template based on metadata. You do NOT see actual data.

Input you receive:
- count: number of items
- fields: available field names (e.g., ["name", "price", "rating", "images"])
- layout_hint: suggested layout (optional)

Output: JSON template with this structure:
{
  "mode": "grid" | "carousel" | "single" | "list",
  "grid": {"rows": N, "cols": M},  // only for grid mode
  "widgetTemplate": {
    "size": "tiny" | "small" | "medium" | "large",
    "atoms": [
      {"type": "image", "field": "images", "size": "medium"},
      {"type": "text", "field": "name", "style": "heading"},
      {"type": "number", "field": "price", "format": "currency"},
      {"type": "rating", "field": "rating"}
    ]
  }
}

Rules:
1. ONLY output valid JSON. No explanations.
2. Use fields that exist in the input.
3. Choose appropriate widget size based on atom count.
4. Size constraints:
   - tiny: max 2 atoms
   - small: max 3 atoms
   - medium: max 5 atoms
   - large: max 10 atoms
5. Choose mode based on count:
   - 1 item → "single"
   - 2-6 items → "grid" (2 cols)
   - 7+ items → "carousel" or "grid" (3 cols)

Atom types (lowercase):
- text: for strings (style: heading/body/caption)
- number: for numbers (format: currency/percent/compact)
- price: for prices with currency
- image: for image URLs (size: small/medium/large)
- rating: for 0-5 ratings
- badge: for status labels (variant: success/warning/danger)
- button: for actions (label, action)
`

// BuildAgent2Prompt builds the user message for Agent 2
func BuildAgent2Prompt(meta domain.StateMeta, layoutHint string) string {
	input := map[string]interface{}{
		"count":  meta.Count,
		"fields": meta.Fields,
	}
	if layoutHint != "" {
		input["layout_hint"] = layoutHint
	}

	jsonBytes, _ := json.Marshal(input)
	return fmt.Sprintf("Create a widget template for this data:\n%s", string(jsonBytes))
}

// Legacy prompts (kept for backward compatibility)

// ComposeWidgetsSystemPrompt is the legacy system prompt for widget composition
const ComposeWidgetsSystemPrompt = `You are a UI composer for an e-commerce chat widget.
Your job is to decide how to display products to the user.

Output JSON only.`

// ComposeWidgetsUserTemplate is the legacy user template for widget composition
const ComposeWidgetsUserTemplate = `User query: {{.Query}}
Products found: {{.ProductCount}}
Product names: {{.ProductNames}}

Decide:
- widget_type: product_card | product_list | comparison_table
- formation: grid | list | carousel
- columns: 1-4 (for grid)

JSON response:`

// BuildComposeWidgetsPrompt builds the prompt for widget composition (legacy)
func BuildComposeWidgetsPrompt(query string, productCount int, productNames []string) string {
	// TODO: implement template substitution
	return ""
}
