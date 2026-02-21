package prompts

import (
	"encoding/json"
	"fmt"
	"strings"

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
// Uses visual_assembly tool with smart defaults
const Agent2ToolSystemPrompt = `Ты — UI composition agent. Решаешь КАК отобразить данные.
Вызывай visual_assembly. Все параметры опциональные. Никогда не выводи текст.

## КАК РАБОТАЕТ

visual_assembly — единственный тул. Defaults Engine автоматически определяет:
- Какие поля показать (по типу и количеству сущностей)
- Layout (1→single, 2+→grid)
- Size (1→large, 2+→medium)

Ты передаёшь ТОЛЬКО то что хочешь изменить.

## ПАРАМЕТРЫ (все опциональные)

- show: string[] — какие поля показать (заменяет дефолтные)
- hide: string[] — какие поля убрать из дефолтных
- display: object — стиль отображения поля: {"brand":"badge","price":"price-lg"}
- layout: string — "grid" | "list" | "single" | "carousel" | "comparison"
- size: string — "tiny" | "small" | "medium" | "large"
- order: string[] — порядок полей
- color: object — цвет поля: {"brand":"red","price":"green"}. Именованные: green, red, blue, orange, purple, gray
- direction: string — "vertical" (по умолчанию) | "horizontal" (картинка слева, контент справа)
- preset: string — shortcut для точного набора полей (backward compat)

## ДОСТУПНЫЕ ПОЛЯ
Product: images, name, price, rating, brand, category, description, tags, stockQuantity, attributes
Service: images, name, price, rating, duration, provider, availability, description, attributes

## DISPLAY СТИЛИ
Текст: h1, h2, h3, h4, body-lg, body, body-sm, caption
Бейджи: badge, badge-success, badge-error, badge-warning
Теги: tag, tag-active
Цена: price, price-lg, price-old, price-discount
Рейтинг: rating, rating-text, rating-compact
Картинки: image-cover, thumbnail, gallery

## ПРАВИЛА

1. Стандартный запрос = visual_assembly() без параметров. НЕ угадывай — дефолты лучше.
2. Пользователь просит изменить отображение = передай ТОЛЬКО то что меняется.
3. layout: "comparison" ТОЛЬКО если пользователь явно просит СРАВНИТЬ ("сравни", "сравнение", "compare").
4. НИКОГДА не меняй layout если пользователь не просит layout. "покажи бренд бейджем" = display only, НЕ трогай layout.
5. Если current_formation уже задан и пользователь меняет только стиль (display/color/size) — НЕ передавай layout.

## ПРИМЕРЫ

productCount=5, нет user_request:
→ visual_assembly()

productCount=1, user_request="покажи подробности":
→ visual_assembly(show: ["images","name","price","brand","description","rating","tags"], size: "large", layout: "single")

productCount=5, user_request="покажи покрупнее":
→ visual_assembly(size: "large")

productCount=4, user_request="только фото и цена":
→ visual_assembly(show: ["images","price"])

productCount=3, user_request="покажи списком":
→ visual_assembly(layout: "list")

productCount=4, user_request="сравни эти товары":
→ visual_assembly(layout: "comparison")

productCount=5, user_request="без рейтинга":
→ visual_assembly(hide: ["rating"])

productCount=5, user_request="бренд как бейдж":
→ visual_assembly(display: {"brand":"badge"})

productCount=5, user_request="покажи покрупнее с описанием":
→ visual_assembly(show: ["images","name","price","brand","description"], size: "large")

productCount=5, user_request="покажи бренд красным":
→ visual_assembly(color: {"brand":"red"})

productCount=5, user_request="покажи горизонтально":
→ visual_assembly(direction: "horizontal")

productCount=5, user_request="бренд зелёным бейджем горизонтально":
→ visual_assembly(display: {"brand":"badge"}, color: {"brand":"green"}, direction: "horizontal")`

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

// BuildAgent2ToolPrompt builds the user message for Agent 2 with view context and user intent
func BuildAgent2ToolPrompt(meta domain.StateMeta, view domain.ViewState, userQuery string, dataDelta *domain.Delta, currentConfig *domain.RenderConfig, allDeltas []domain.Delta, microcontext string) string {
	input := map[string]interface{}{
		"productCount": meta.ProductCount,
		"serviceCount": meta.ServiceCount,
		"fields":       meta.Fields,
	}

	// Aliases — compact field metadata for Agent 2 context
	if len(meta.Aliases) > 0 {
		input["aliases"] = meta.Aliases
	}

	// View context
	input["view_mode"] = string(view.Mode)
	if view.Focused != nil {
		input["focused"] = view.Focused
	}

	// Current formation config — what is on screen now
	if currentConfig != nil {
		input["current_formation"] = currentConfig
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

	// History summary for multi-turn context
	if historySummary := BuildHistorySummary(allDeltas); historySummary != "" {
		input["history_summary"] = historySummary
	}

	jsonBytes, _ := json.Marshal(input)

	// Prepend microcontext if available
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
