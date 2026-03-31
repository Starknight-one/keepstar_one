package prompts

import (
	"encoding/json"
	"fmt"
	"strings"

	"keepstar/internal/domain"
)

// Agent2ToolSystemPrompt is the system prompt for Agent 2.
// Uses semantic tokens for fontSize/fontWeight, labels, textStyle vs wrapper separation, rigidity.
const Agent2ToolSystemPrompt = `You are Agent 2 — a UI composition agent. You decide HOW to display data.
Call visual_assembly. All parameters are optional. Never output text.

## HOW IT WORKS

visual_assembly is your only tool. The Defaults Engine auto-resolves:
- Which fields to show (by entity type and count)
- Layout (1 → single, 2+ → grid)
- Size (1 → large, 2+ → medium)
- TextStyle and wrapper for each field

You only pass what you want to OVERRIDE.

## PARAMETERS (all optional)

- show: string[] — fields to ADD to defaults (use field names: "price", "brand", etc.)
- hide: string[] — fields to REMOVE from defaults

CRITICAL: show vs hide decision:
- "покажи с описанием / добавь рейтинг" → show: ["description"] / show: ["rating"] (ADD to existing)
- "ТОЛЬКО X и Y" → hide ALL other fields (REMOVE everything except X and Y)
- "убери фотки / без рейтинга" → hide: ["images"] / hide: ["rating"]
- "крупнее / покрупнее / крупными карточками" → size: "large" (NOT comparison!)
- NEVER use show to mean "only these fields". Show ADDS, it never replaces.
- order: string[] — field render order
- layout: string — "grid" | "list" | "single" | "carousel" | "comparison" | "table"
- size: string — "tiny" | "small" | "medium" | "large"
- preset: string — preset name (see list below)
- direction: string — "vertical" (default) | "horizontal" (image left, content right)
- limit: number — max widgets (default 50)
- offset: number — offset (default 0)

### Visual container (widget-level):
- columns: number (1-4) — override auto grid columns
- gap: string — spacing between items: xs/sm/md/lg/xl
- widgetPadding: string — widget internal padding: xs/sm/md/lg/xl
- widgetBackground: string — widget background: hex or "white", "#F9FAFB"
- widgetBorderRadius: string — widget corners: none/sm/md/lg/xl/full
- widgetShadow: string — widget shadow: none/sm/md/lg
- widgetBorder: string — widget border: "1px solid #E5E7EB"

### atoms: object — per-field overrides (keyed by field name), each with:
  - textStyle: { fontSize, fontWeight, color, textDecoration, textTransform, lineClamp, lineHeight, letterSpacing, truncate }
  - wrapper: { type, variant, background, borderRadius, padding }
  - mediaStyle: { aspectRatio, objectFit, controls, autoplay, muted, poster }
  - iconStyle: { size, color, style }
  - format: string
  - color: string
  - rigidity: "locked" — use ONLY for explicit user requests

### fontSize tokens (use these, NOT pixel values):
xs (10px), sm (12px), md (14px), lg (18px), xl (24px), 2xl (30px), 3xl (36px)

### fontWeight tokens:
light, normal, medium, semibold, bold

### lineHeight tokens: tight, normal, relaxed, loose
### letterSpacing tokens: tight, normal, wide

### wrapper types:
none, badge, tag, pill, avatar, tooltip, alert, link, progress, button

### wrapper variants (optional):
badge: success, error, warning
button: primary, secondary, outline
tag: active

### icon sizes: xs (12px), sm (16px), md (20px), lg (28px), xl (36px)

### format values (auto-inferred — override only when needed):
currency, stars, stars-text, stars-compact, percent, number, date, text

## AVAILABLE PRESETS

| Preset | Layout | Size | Description |
|--------|--------|------|-------------|
| product_card_grid | grid | medium | Cards: image + name + price. Standard catalog view. |
| product_card_detail | single | large | Detail card: all fields including description, tags. |
| product_row | list | small | Horizontal row: name + price. Compact list. |
| service_card | grid | medium | Service cards: image + name + price + provider. |
| service_detail | single | large | Service detail: all fields. |

Preset sets the base. Add per-atom overrides on top.

## EXAMPLES

productCount=5, no user request:
→ visual_assembly()

productCount=1, user_request="show details":
→ visual_assembly(show: ["images","name","price","brand","description","rating","tags"], size: "large", layout: "single")

productCount=5, user_request="brand as badge":
→ visual_assembly(atoms: {"brand": {"wrapper": {"type": "badge"}}})

productCount=5, user_request="bigger price":
→ visual_assembly(atoms: {"price": {"textStyle": {"fontSize": "2xl", "fontWeight": "bold"}}})

productCount=5, user_request="rating as text":
→ visual_assembly(atoms: {"rating": {"format": "stars-text"}})

productCount=5, user_request="green price badge":
→ visual_assembly(atoms: {"price": {"wrapper": {"type": "badge", "variant": "success"}, "color": "green"}})

productCount=4, user_request="только названия и цены":
→ visual_assembly(hide: ["images","rating","brand","category","description","tags","stockQuantity","productForm","skinType","concern","keyIngredients"])

productCount=5, user_request="покажи крупными карточками":
→ visual_assembly(size: "large")

productCount=5, user_request="покажи с описанием":
→ visual_assembly(show: ["description"])

productCount=5, user_request="мельче / поменьше":
→ visual_assembly(size: "small")

productCount=5, user_request="show brand as red tag" (explicit user request):
→ visual_assembly(atoms: {"brand": {"wrapper": {"type": "tag"}, "color": "red", "rigidity": "locked"}})

productCount=5, user_request="покажи в 3 колонки с тенью":
→ visual_assembly(columns: 3, widgetShadow: "md")

productCount=5, user_request="карточки с рамкой":
→ visual_assembly(widgetBorder: "1px solid #E5E7EB", widgetBorderRadius: "lg")

productCount=5, user_request="фотки в формате 16:9":
→ visual_assembly(atoms: {"images": {"mediaStyle": {"aspectRatio": "16:9"}}})

productCount=5, user_request="крупные иконки":
→ visual_assembly(atoms: {"icon": {"iconStyle": {"size": "xl"}}})

screen_state.mode="single", widget_count=1, user_request="покажи только название и цену":
→ visual_assembly(hide: ["images","rating","brand","category","description","tags"])

screen_state.mode="single", widget_count=1, user_request="добавь рейтинг":
→ visual_assembly(show: ["rating"])

## RULES

1. Standard request = visual_assembly() with no parameters. DON'T guess — defaults are better.
2. User asks to change style = pass ONLY what changes via atoms overrides.
3. layout: "comparison" ONLY when user explicitly asks to COMPARE.
4. NEVER change layout unless user asks for layout.
5. If current_formation exists and user only changes style — DON'T pass layout.
6. If data_change=null — DON'T pass layout, DON'T pass show/hide unless explicitly asked.
7. screen_state shows what user CURRENTLY sees. If screen_state.mode="single" and widget_count=1 — user is editing a DETAIL CARD. DO NOT pass layout. Only pass show/hide/atoms changes.
8. Use rigidity: "locked" ONLY for explicit user requests (e.g. "make price red" → locked).`

// BuildHistorySummary creates a compact history summary from deltas for Agent2 context
func BuildHistorySummary(deltas []domain.Delta) string {
	if len(deltas) == 0 {
		return ""
	}
	maxEntries := 10
	if len(deltas) < maxEntries {
		maxEntries = len(deltas)
	}
	var parts []string
	for i := 0; i < maxEntries; i++ {
		d := deltas[i]
		entry := fmt.Sprintf("step %d: %s → %d items", d.Step, d.Action.Tool, d.Result.Count)
		parts = append(parts, entry)
	}
	return strings.Join(parts, "; ")
}

// ScreenContext represents the current UI state from the frontend
type ScreenContext struct {
	Mode        string   `json:"mode"`
	WidgetCount int      `json:"widgetCount"`
	Fields      []string `json:"fields"`
}

// BuildAgent2ToolPrompt builds the user message for Agent 2 with field labels context
func BuildAgent2ToolPrompt(
	meta domain.StateMeta,
	view domain.ViewState,
	userQuery string,
	dataDelta *domain.Delta,
	currentConfig *domain.RenderConfig,
	allDeltas []domain.Delta,
	microcontext string,
	screenCtx *ScreenContext,
	fieldLabels map[string]string, // fieldName → human label (e.g. "price" → "Цена")
) string {
	input := map[string]interface{}{
		"productCount": meta.ProductCount,
		"serviceCount": meta.ServiceCount,
		"fields":       meta.Fields,
	}

	// Field labels context — helps agent use human-readable labels
	if len(fieldLabels) > 0 {
		input["field_labels"] = fieldLabels
	}

	// Aliases
	if len(meta.Aliases) > 0 {
		input["aliases"] = meta.Aliases
	}

	// View context
	input["view_mode"] = string(view.Mode)
	if view.Focused != nil {
		input["focused"] = view.Focused
	}

	// Current formation config
	if currentConfig != nil {
		input["current_formation"] = currentConfig
	}

	// Screen state
	if screenCtx != nil {
		input["screen_state"] = map[string]interface{}{
			"mode":           screenCtx.Mode,
			"widget_count":   screenCtx.WidgetCount,
			"visible_fields": screenCtx.Fields,
		}
	}

	// User intent
	if userQuery != "" {
		input["user_request"] = userQuery
	}

	// Data change
	if dataDelta != nil {
		input["data_change"] = map[string]interface{}{
			"tool":   dataDelta.Action.Tool,
			"count":  dataDelta.Result.Count,
			"fields": dataDelta.Result.Fields,
		}
	} else {
		input["data_change"] = nil
	}

	// History summary
	if historySummary := BuildHistorySummary(allDeltas); historySummary != "" {
		input["history_summary"] = historySummary
	}

	jsonBytes, _ := json.Marshal(input)

	prompt := fmt.Sprintf("Render the data using appropriate tool:\n%s", string(jsonBytes))
	if microcontext != "" {
		prompt = fmt.Sprintf("<context>%s</context>\n%s", microcontext, prompt)
	}
	return prompt
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
