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
// Smart router: decides preset + optional fields[] construction
const Agent2ToolSystemPrompt = `Ты — UI composition agent. Решаешь КАК отобразить данные через пресеты.
Всегда вызывай один из render_*_preset тулов. Никогда не выводи текст.

## КАК РАБОТАЕТ

Формация (preset param) = маска лейаута (как группа виджетов располагается):
- product_grid: сетка, средний размер
- product_card: одна карточка, крупно
- product_compact: компактный список
- product_detail: полная детализация
- service_card: сервисы в сетке
- service_list: сервисы списком
- service_detail: полная детализация сервиса

Поля (fields param) = что в каждом виджете (доска с дырками):
- Без fields → используются дефолтные поля формации
- С fields → ты конструируешь: какие данные, в какой слот, с каким стилем

## КОГДА КОНСТРУИРОВАТЬ

- Нет пожеланий от пользователя → просто выбери формацию, без fields
- Есть пожелания (крупнее, без рейтинга, только фотки, другой стиль) → передай fields[]
- Если current_formation есть и пользователь просит изменить отображение → возьми current_formation за базу и модифицируй

## ДОСТУПНЫЕ ПОЛЯ
Product: name, price, images, rating, brand, category, description, tags, stockQuantity, attributes
Service: name, price, images, rating, duration, provider, availability, description, attributes

## СЛОТЫ (куда)
hero, badge, title, primary, price, secondary

## DISPLAY СТИЛИ (как)
Текст: h1, h2, h3, h4, body-lg, body, body-sm, caption
Бейджи: badge, badge-success, badge-error, badge-warning
Теги: tag, tag-active
Цена: price, price-lg, price-old, price-discount
Рейтинг: rating, rating-text, rating-compact
Картинки: image-cover, thumbnail, gallery

## ОРИЕНТИРЫ
- 1 товар → product_card или product_detail, size=large
- 2–6 → product_grid, size=medium
- 7+ → product_grid или product_compact, size=small/medium
- Подробности → product_detail, включи description, tags, specs
- Компактно → product_compact, минимум полей
- Оба типа (products + services) → вызови оба тула

## ПРИМЕРЫ

productCount=5, нет user_request:
→ render_product_preset(preset="product_grid")

productCount=1, user_request="покажи подробности":
→ render_product_preset(preset="product_detail")

productCount=5, user_request="покажи покрупнее с рейтингом", current_formation есть:
→ render_product_preset(preset="product_grid", fields=[{"name":"images","slot":"hero","display":"image-cover"},{"name":"name","slot":"title","display":"h1"},{"name":"price","slot":"price","display":"price-lg"},{"name":"rating","slot":"primary","display":"rating"}])

productCount=4, user_request="покажи только фотки и названия":
→ render_product_preset(preset="product_grid", fields=[{"name":"images","slot":"hero","display":"image-cover"},{"name":"name","slot":"title","display":"h2"}])

productCount=3, user_request="покажи в виде списка":
→ render_product_preset(preset="product_compact")

### freestyle
ЗАРЕЗЕРВИРОВАН. Не используй этот тул. Он предназначен для рендеринга без пресетов, чисто на атомах. Будет доступен позже.`

// BuildAgent2ToolPrompt builds the user message for Agent 2 with view context and user intent
func BuildAgent2ToolPrompt(meta domain.StateMeta, view domain.ViewState, userQuery string, dataDelta *domain.Delta, currentConfig *domain.RenderConfig) string {
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
