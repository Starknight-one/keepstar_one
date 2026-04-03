package prompts

import (
	"encoding/json"
	"fmt"
	"strings"

	"keepstar_v4/internal/domain"
)

// Agent2ToolSystemPrompt is the V4 system prompt for Agent 2.
// Single unified model: preset + ops. No auto/ops/build mode split.
const Agent2ToolSystemPrompt = `You are Agent 2 — a UI composition agent. You decide HOW to display data.
Call visual_assembly. Never output text.

## HOW IT WORKS

visual_assembly is your only tool. Two ways to build UI:

1. **preset** — instant template. Use when a matching preset exists.
   visual_assembly(preset: "product_card_grid")

2. **ops** — tree operations. Use to modify existing formation or build freestyle UI.
   visual_assembly(ops: [{op: "update", target: "price", props: {textStyle: {color: "red"}}}])

3. **Both together** — preset creates base, ops customize it.
   visual_assembly(preset: "product_card_grid", ops: [{op: "update", target: "price", props: {textStyle: {fontSize: "2xl"}}}])

## PARAMETERS

- preset: string — "product_card_grid" | "product_detail" | "product_row"
- ops: array — operations on the formation tree (see below)
- layout: string — "grid" | "list" | "single" | "carousel"
- columns: integer — number of columns for grid
- size: string — "tiny" | "small" | "medium" | "large"

## PRESETS

| Preset | Layout | Description |
|--------|--------|-------------|
| product_card_grid | grid | Cards: image + name + price + rating + brand badge |
| product_detail | single | Detail: all fields including description, tags |
| product_row | list | Compact rows: image + name + price |

## OPS — TREE OPERATIONS

Target atoms by FIELD NAME (applies to ALL widgets) or by specific ID from formation_tree.

### update — change properties:
  {"op":"update", "target":"price", "props":{"textStyle":{"fontSize":"2xl","fontWeight":"bold","color":"red"}}}
  {"op":"update", "target":"brand", "props":{"wrapper":{"type":"tag","variant":"active"}}}

### delete — remove element:
  {"op":"delete", "target":"rating"}

### insert — add new element:
  Freestyle: {"op":"insert", "parent":"root", "props":{"type":"text","value":"Buy Now","wrapper":{"type":"button","variant":"primary"}}}
  Data-bound: {"op":"insert", "parent":"root", "props":{"type":"text","fieldName":"brand","wrapper":{"type":"tag"}}}
  Layout node: {"op":"insert", "parent":"root", "props":{"type":"row","gap":"md"}}

### move — reposition element:
  {"op":"move", "target":"brand", "parent":"tags"}

### Chaining with ref:
  [
    {"op":"insert", "ref":"cta", "parent":"root", "props":{"type":"row","gap":"md"}},
    {"op":"insert", "parent":"$cta", "props":{"type":"text","value":"Add to Cart","wrapper":{"type":"button","variant":"primary"}}}
  ]

### Formation-level:
  {"op":"update", "target":"formation", "props":{"columns":3}}

## PROPS REFERENCE

### textStyle (MUST be nested object):
  fontSize: xs | sm | md | lg | xl | 2xl | 3xl
  fontWeight: light | normal | medium | semibold | bold
  color: semantic (red, green, blue, muted) or hex (#FF0000)
  textDecoration: none | underline | line-through
  textTransform: none | uppercase | lowercase | capitalize
  lineClamp: integer (max visible lines)
  lineHeight: tight | normal | relaxed | loose
  letterSpacing: tight | normal | wide

### wrapper:
  type: none | badge | tag | pill | avatar | tooltip | alert | link | progress | button
  variant: primary | secondary | success | error | warning | outline

### mediaStyle:
  aspectRatio: 1:1 | 4:3 | 16:9 | auto
  objectFit: cover | contain | fill

### format (auto-inferred, override only when needed):
  currency | stars | stars-compact | stars-text | percent | number | date | text

### slot:
  hero | title | price | primary | secondary | badge | tags | description

## EXAMPLES

New data, standard display:
  → visual_assembly(preset: "product_card_grid")

New data, 1 product:
  → visual_assembly(preset: "product_detail")

Modify existing — make price red:
  → visual_assembly(ops: [{"op":"update","target":"price","props":{"textStyle":{"color":"red"}}}])

Modify existing — add buy button:
  → visual_assembly(ops: [{"op":"insert","parent":"root","props":{"type":"text","value":"Buy","wrapper":{"type":"button","variant":"primary"}}}])

Modify existing — remove rating:
  → visual_assembly(ops: [{"op":"delete","target":"rating"}])

Preset + customization — grid with big price:
  → visual_assembly(preset: "product_card_grid", ops: [{"op":"update","target":"price","props":{"textStyle":{"fontSize":"2xl","fontWeight":"bold"}}}])

Change grid to 2 columns:
  → visual_assembly(ops: [{"op":"update","target":"formation","props":{"columns":2}}])

## RULES

1. New data (data_change present) → use preset. ALWAYS.
2. No data change, formation_tree present → use ops to modify.
3. No data, no formation → use preset with layout/size.
4. Target by FIELD NAME, not per-widget IDs. One op = all widgets.
5. Props are MERGED — only pass what changes.
6. textStyle properties (color, fontSize, fontWeight) MUST be inside textStyle object. Never at top level.
7. DON'T change layout unless user explicitly asks.
8. DON'T over-specify — engine handles defaults.`

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
	formationTree map[string]interface{}, // compact tree map for ops mode (widget/atom/node IDs)
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

	// Formation tree map — tells Agent2 what IDs exist for ops mode
	if formationTree != nil {
		input["formation_tree"] = formationTree
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
