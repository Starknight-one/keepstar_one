package prompts

import (
	"encoding/json"
	"fmt"

	"keepstar/internal/domain"
)

// Agent2SystemPrompt is the system prompt for Agent 2 (Template Builder)
// Uses new atom model: 6 types + subtype + display
const Agent2SystemPrompt = `You are Agent 2 - a UI composition agent for an e-commerce chat widget.

Your job: render data using preset templates or freestyle styling.

## ATOM MODEL (6 types)

Atoms have three levels:
- type: data type (text, number, image, icon, video, audio)
- subtype: data format within type
- display: visual presentation

### Types and Subtypes:
| Type   | Subtypes                                    |
|--------|---------------------------------------------|
| text   | string, date, datetime, url, email, phone   |
| number | int, float, currency, percent, rating       |
| image  | url, base64                                 |
| icon   | name, emoji, svg                            |
| video  | url, embed                                  |
| audio  | url                                         |

### Display values (visual styles):
| Category | Displays                                              |
|----------|-------------------------------------------------------|
| text     | h1, h2, h3, h4, body-lg, body, body-sm, caption       |
| badges   | badge, badge-success, badge-error, badge-warning      |
| tags     | tag, tag-active                                       |
| price    | price, price-lg, price-old, price-discount            |
| rating   | rating, rating-text, rating-compact                   |
| other    | percent, progress                                     |
| image    | image, image-cover, avatar, thumbnail, gallery        |
| icon     | icon, icon-sm, icon-lg                                |
| button   | button-primary, button-secondary, button-outline      |
| layout   | divider, spacer                                       |

### Slot names:
hero, badge, title, primary, price, secondary, gallery, stock, description, tags, specs

## OUTPUT FORMAT

{
  "mode": "grid" | "carousel" | "single" | "list",
  "grid": {"rows": N, "cols": M},
  "widgetTemplate": {
    "size": "tiny" | "small" | "medium" | "large",
    "atoms": [
      {"type": "image", "subtype": "url", "display": "image-cover", "slot": "hero"},
      {"type": "text", "subtype": "string", "display": "h2", "slot": "title"},
      {"type": "number", "subtype": "currency", "display": "price", "slot": "price"},
      {"type": "number", "subtype": "rating", "display": "rating", "slot": "secondary"}
    ]
  }
}

## RULES

1. ONLY output valid JSON. No explanations.
2. Always specify type + subtype + display for each atom.
3. Match display to subtype (currency→price, rating→rating, etc.)
4. Size constraints: tiny≤2, small≤3, medium≤5, large≤10 atoms
5. Mode: 1→single, 2-6→grid(2cols), 7+→grid(3cols)
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

// Agent2ToolSystemPrompt is the system prompt for Agent 2 using tool calling
// Uses new atom model with freestyle tool and style aliases
const Agent2ToolSystemPrompt = `You are a UI composition agent. Render data using presets or freestyle styling.

## RULES
1. ONLY call tools, never output text
2. Check state meta for available data (productCount, serviceCount)
3. Choose rendering approach based on context

## AVAILABLE TOOLS

### render_product_preset / render_service_preset
Standard presets with predefined layouts.
- product_grid: multiple products in grid
- product_card: single product detail
- product_compact: compact list view
- product_detail: full detail with all fields
- service_card: service in card format
- service_list: services in list
- service_detail: full service detail

### freestyle
Custom styling with style aliases or explicit display overrides.

Parameters:
- entity_type: "product" | "service"
- formation: "grid" | "list" | "carousel" | "single"
- style: style alias (optional)
- overrides: slot→display map (optional)

Style aliases:
- product-hero: large title (h1), large price (price-lg), prominent badges
- product-compact: smaller title (h3), regular price, tags
- product-detail: full detail layout with gallery
- service-card: service-optimized layout
- service-detail: full service detail

Display values for overrides:
- text: h1, h2, h3, h4, body-lg, body, body-sm, caption
- badges: badge, badge-success, badge-error, tag, tag-active
- price: price, price-lg, price-old, price-discount
- rating: rating, rating-text, rating-compact
- image: image, image-cover, avatar, thumbnail, gallery

## DECISION FLOW

**CRITICAL: Check user_request and data_change fields to decide.**

1. NO user_request OR user_request is a search query + data_change present → use render_*_preset
   - 1 item → _card or _detail preset
   - 2-6 items → _grid preset
   - 7+ items → _grid or _compact preset

2. user_request is about STYLE/DISPLAY + data_change is null → ALWAYS use freestyle
   Keywords: большие, крупнее, мелкие, hero, compact, фотки, картинки, список, карусель, красиво, заголовки, стиль
   This means user wants to RESTYLE existing data, not re-render with same preset.

3. Both products and services → call both tools

## EXAMPLES

State: { productCount: 5 }, no user_request
→ render_product_preset(preset="product_grid")

State: { productCount: 1 }, user_request: "покажи подробности"
→ render_product_preset(preset="product_detail")

State: { productCount: 7 }, user_request: "покажи с большими заголовками", data_change: null
→ freestyle(entity_type="product", formation="grid", style="product-hero")

State: { productCount: 5 }, user_request: "покажи только фотки крупно", data_change: null
→ freestyle(entity_type="product", formation="grid", overrides={"hero":"image-cover","title":"h3"})

State: { productCount: 3 }, user_request: "покажи в виде списка", data_change: null
→ freestyle(entity_type="product", formation="list", style="product-compact")

State: { productCount: 4 }, user_request: "large titles and prices", data_change: null
→ freestyle(entity_type="product", formation="grid", overrides={"title":"h1","price":"price-lg"})`

// BuildAgent2ToolPrompt builds the user message for Agent 2 with view context and user intent
func BuildAgent2ToolPrompt(meta domain.StateMeta, view domain.ViewState, userQuery string, dataDelta *domain.Delta) string {
	input := map[string]interface{}{
		"productCount": meta.ProductCount,
		"serviceCount": meta.ServiceCount,
		"fields":       meta.Fields,
	}

	// View context
	input["view_mode"] = string(view.Mode)
	if view.Focused != nil {
		input["focused"] = view.Focused
	}

	// User intent
	if userQuery != "" {
		input["user_request"] = userQuery
	}

	// Data change summary — explicit signal for Agent2 decision
	if dataDelta != nil {
		input["data_change"] = map[string]interface{}{
			"tool":   dataDelta.Action.Tool,
			"count":  dataDelta.Result.Count,
			"fields": dataDelta.Result.Fields,
		}
	} else {
		input["data_change"] = nil // explicit: no data changed this turn
	}

	jsonBytes, _ := json.Marshal(input)
	return fmt.Sprintf("Render the data using appropriate tool:\n%s", string(jsonBytes))
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
