# Feature: Agent Tool Isolation

**ADW-ID**: ADW-t5f2k8n
**Type**: bugfix
**Priority**: critical
**Complexity**: simple
**Layers**: backend
**Based on**: Runtime log analysis — Agent1 вызывает render тулы, freestyle ни разу не вызван

---

## Feature Description

Agent1 получает ВСЕ тулы из реестра (search_*, render_*, freestyle), хотя по архитектуре должен видеть только data-тулы (search_*). Из-за этого:

1. При style-запросе ("покажи с большими заголовками") Agent1 сам вызывает `render_product_preset` вместо того чтобы пропустить ход и отдать рендер Agent2
2. Agent2 запускается после Agent1, видит что formation уже есть, и рендерит заново тем же `product_grid` — двойной рендер одним и тем же пресетом
3. Freestyle тул ни разу не вызван в продакшене — Agent2 всегда выбирает `render_product_preset`

**Доказательство из логов:**
```
query="А можешь показать только фотки кроссовок?"
→ agent1_completed tool_called="render_product_preset"   ← НЕПРАВИЛЬНО: Agent1 = data layer
→ agent2 → render_product_preset(product_grid)           ← перезапись тем же
```

Дополнительно: лог `tool_registry_initialized` в main.go захардкожен и не показывает freestyle.

---

## Архитектурный контекст

```
Agent1 = Слой 1 (Data) → ТОЛЬКО search_* тулы
Agent2 = Слой 3 (Render) → render_* + freestyle тулы
```

Agent2 уже имеет фильтр `getAgent2Tools()` (строка 187 agent2_execute.go). Agent1 фильтра НЕ имеет — `GetDefinitions()` возвращает все тулы (строка 96 agent1_execute.go).

При style-запросе (данные уже есть, юзер хочет изменить отображение):
- Agent1 **не должен** вызывать тул — данные не меняются
- Agent1 без data-тулов → LLM возвращает stop_reason="end_turn" → Agent1 возвращает пустой response
- Agent2 получает UserQuery + существующие данные → выбирает freestyle

---

## Objective

1. **Agent1 tool filter** — Agent1 видит только `search_*` тулы
2. **Динамический лог реестра** — main.go показывает реальный список тулов
3. **Agent2 prompt: явный сигнал "data не менялась"** — если data_change отсутствует, Agent2 знает что это style-запрос и должен использовать freestyle
4. **Agent1 prompt: явно разрешить "не вызывать тул"** — если запрос не про поиск, Agent1 может не вызывать тул

---

## Expertise Context

Expertise used:
- **backend-pipeline**: `GetDefinitions()` возвращает все тулы + padding тулы. `getAgent2Tools()` фильтрует render_* — паттерн для копирования в Agent1
- **backend-usecases**: Agent1 flow: get state → build messages → call LLM → execute tool → append conversation. Если LLM не вызвал тул — логируется warn `no_tool_call`, response возвращается с пустым ToolName. Pipeline продолжает к Agent2 в любом случае
- **backend-usecases**: Agent2 flow: get state → check count → short-circuit if 0 → build prompt → call LLM → execute tool. Short-circuit на `count == 0` — если данные есть от предыдущего запроса, Agent2 будет рендерить

---

## Relevant Files

### Existing Files (modify)

- `project/backend/internal/usecases/agent1_execute.go` — добавить `getAgent1Tools()`, заменить `GetDefinitions()` на фильтрованный список (строка 96)
- `project/backend/internal/prompts/prompt_analyze_query.go` — обновить Agent1SystemPrompt: разрешить не вызывать тул при style-запросах
- `project/backend/internal/prompts/prompt_compose_widgets.go` — усилить Agent2ToolSystemPrompt: добавить сигнал `no_data_change` в decision flow, добавить примеры на русском
- `project/backend/cmd/server/main.go` — динамический лог с реальным списком тулов (строка 108)

### Existing Files (verify only)
- `project/backend/internal/usecases/agent2_execute.go` — `getAgent2Tools()` уже включает freestyle ✅
- `project/backend/internal/tools/tool_registry.go` — freestyle зарегистрирован в `NewRegistry()` ✅
- `project/backend/internal/usecases/pipeline_execute.go` — Agent2 всегда вызывается после Agent1, даже если Agent1 не вызвал тул ✅

### Existing Files (test update)
- `project/backend/internal/usecases/agent1_execute_test.go` — добавить тест что Agent1 не видит render тулы

---

## Step by Step Tasks

IMPORTANT: Execute strictly in order. Each step MUST compile (`go build ./...`).

### 1. Backend: Agent1 tool filter в agent1_execute.go

**File:** `project/backend/internal/usecases/agent1_execute.go`

1.1. Добавить метод `getAgent1Tools()` (по аналогии с `getAgent2Tools` из agent2_execute.go):

```go
// getAgent1Tools returns data tools only for Agent 1 (search_*)
func (uc *Agent1ExecuteUseCase) getAgent1Tools() []domain.ToolDefinition {
	allTools := uc.toolRegistry.GetDefinitions()
	var agent1Tools []domain.ToolDefinition
	for _, t := range allTools {
		if strings.HasPrefix(t.Name, "search_") {
			agent1Tools = append(agent1Tools, t)
		}
	}
	return agent1Tools
}
```

1.2. Добавить `"strings"` в imports (если нет).

1.3. Заменить строку 96:
```go
// БЫЛО:
toolDefs := uc.toolRegistry.GetDefinitions()

// СТАЛО:
toolDefs := uc.getAgent1Tools()
```

**Проверить:** Agent1 теперь видит только `search_products` (и будущие `search_services`). Render тулы и freestyle НЕ в списке.

**Важно:** `getAgent1Tools()` НЕ включает padding тулы (`_internal_*`). Padding тулы нужны для Anthropic prompt caching (порог 4096 токенов). Проверить что Agent1 toolDefs + system prompt + messages превышают 4096 токенов без padding. Если нет — добавить padding тулы в `getAgent1Tools()`:

```go
// Если нужен padding для кэша:
agent1Tools = append(agent1Tools, tools.GetCachePaddingTools()...)
```

---

### 2. Backend: Agent1 system prompt — разрешить не вызывать тул

**File:** `project/backend/internal/prompts/prompt_analyze_query.go`

2.1. Заменить `Agent1SystemPrompt` (строка 4):

```go
const Agent1SystemPrompt = `You are Agent 1 - a data retrieval agent for an e-commerce chat.

Your job: call search tools when user needs NEW data. If the user is asking about STYLE or DISPLAY (not new data), do nothing.

Rules:
1. If user asks for products/services → call search_products
2. If user asks to CHANGE DISPLAY STYLE (bigger, smaller, hero, compact, grid, list, photos only, etc.) → DO NOT call any tool. Just stop.
3. Do NOT explain what you're doing.
4. Do NOT ask clarifying questions - make best guess.
5. Tool results are written to state. You only get "ok" or "empty".
6. After getting "ok"/"empty", stop. Do not call more tools.

Available tools:
- search_products: Search for products by query, category, brand, price range

Examples:
- "покажи ноутбуки" → search_products(query="ноутбуки")
- "Nike shoes under $100" → search_products(query="Nike shoes", max_price=100)
- "дешевые телефоны Samsung" → search_products(query="телефоны", brand="Samsung", max_price=20000)
- "покажи с большими заголовками" → DO NOT call tool (style request, not data)
- "покажи только фотки" → DO NOT call tool (display change, not data)
- "сделай покрупнее" → DO NOT call tool (style request)
- "покажи в виде списка" → DO NOT call tool (layout change)
`
```

**Ключевые изменения:**
- Правило 1: явно — search если нужны НОВЫЕ данные
- Правило 2: явно — НЕ вызывать тул при style запросах
- 4 примера на русском когда НЕ вызывать тул

---

### 3. Backend: Agent2 prompt — сигнал "data не менялась" и русские примеры

**File:** `project/backend/internal/prompts/prompt_compose_widgets.go`

3.1. Заменить `Agent2ToolSystemPrompt` (строка 91) — усилить decision flow:

```go
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

2. user_request is about STYLE/DISPLAY + NO data_change → ALWAYS use freestyle
   Keywords: большие, крупнее, мелкие, hero, compact, фотки, картинки, список, карусель, красиво, заголовки, стиль
   This means user wants to RESTYLE existing data, not re-render with same preset.

3. Both products and services → call both tools

## EXAMPLES

State: { productCount: 5 }, no user_request
→ render_product_preset(preset="product_grid")

State: { productCount: 1 }, user_request: "покажи подробности"
→ render_product_preset(preset="product_detail")

State: { productCount: 7 }, user_request: "покажи с большими заголовками", no data_change
→ freestyle(entity_type="product", formation="grid", style="product-hero")

State: { productCount: 5 }, user_request: "покажи только фотки крупно", no data_change
→ freestyle(entity_type="product", formation="grid", overrides={"hero":"image-cover","title":"h3"})

State: { productCount: 3 }, user_request: "покажи в виде списка", no data_change
→ freestyle(entity_type="product", formation="list", style="product-compact")

State: { productCount: 4 }, user_request: "large titles and prices", no data_change
→ freestyle(entity_type="product", formation="grid", overrides={"title":"h1","price":"price-lg"})`
```

**Ключевые изменения:**
- Decision flow пункт 2: **явный сигнал** — если `no data_change` + style-related user_request → ALWAYS freestyle
- Русские примеры стилевых запросов
- Ключевые слова для распознавания style запросов

3.2. Обновить `BuildAgent2ToolPrompt` — добавить явный маркер `no_data_change` (строка 163):

```go
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
```

**Изменение:** `data_change: null` явно передаётся в промпт когда данные не менялись. Раньше поле просто отсутствовало — LLM мог это не заметить. Теперь `"data_change":null` — однозначный сигнал.

---

### 4. Backend: Динамический лог реестра в main.go

**File:** `project/backend/cmd/server/main.go`

4.1. Заменить строку 108:

```go
// БЫЛО:
appLog.Info("tool_registry_initialized", "tools", "search_products, render_product_preset, render_service_preset")

// СТАЛО:
toolNames := make([]string, 0)
for _, def := range toolRegistry.GetDefinitions() {
    if !strings.HasPrefix(def.Name, "_internal_") {
        toolNames = append(toolNames, def.Name)
    }
}
appLog.Info("tool_registry_initialized", "tools", strings.Join(toolNames, ", "), "count", len(toolNames))
```

4.2. Добавить `"strings"` в imports main.go (если нет).

**Проверить:** лог теперь показывает `"tools":"search_products, render_product_preset, render_service_preset, freestyle", "count":4`.

---

### 5. Backend: Тест — Agent1 не видит render тулы

**File:** `project/backend/internal/usecases/agent1_execute_test.go`

5.1. Добавить unit-тест (НЕ integration — не требует DB и LLM):

> **Примечание:** Полноценный unit-тест `getAgent1Tools()` требует mock'а `*tools.Registry` или рефакторинга чтобы принимать interface. Текущая реализация использует конкретный `*tools.Registry`, а `getAgent1Tools()` — приватный метод. Варианты:
>
> **Вариант A (простой):** Создать integration тест в существующем файле — использует реальный registry, проверяет через reflection или через новый exported метод.
>
> **Вариант B (правильный):** Добавить exported метод `GetAgent1ToolDefs()` в use case, тестировать его.
>
> **Рекомендация:** Вариант A — минимальные изменения, использовать setupIntegration из существующих тестов.

```go
func TestAgent1_OnlyGetsDataTools(t *testing.T) {
	ctx, cancel, stateAdapter, _, _, toolRegistry, llmClient, log := setupIntegration(t, 10*time.Second)
	defer cancel()

	agent1UC := usecases.NewAgent1ExecuteUseCase(llmClient, stateAdapter, toolRegistry, log)

	// Use exported method to verify tool filtering
	toolDefs := agent1UC.GetToolDefs(ctx)

	for _, td := range toolDefs {
		if strings.HasPrefix(td.Name, "_internal_") {
			continue // skip padding tools
		}
		if !strings.HasPrefix(td.Name, "search_") {
			t.Errorf("Agent1 should only see search_* tools, got: %s", td.Name)
		}
	}

	// Verify specific tools
	hasSearch := false
	hasRender := false
	hasFreestyle := false
	for _, td := range toolDefs {
		switch {
		case td.Name == "search_products":
			hasSearch = true
		case strings.HasPrefix(td.Name, "render_"):
			hasRender = true
		case td.Name == "freestyle":
			hasFreestyle = true
		}
	}

	if !hasSearch {
		t.Error("Agent1 should see search_products")
	}
	if hasRender {
		t.Error("Agent1 should NOT see render_* tools")
	}
	if hasFreestyle {
		t.Error("Agent1 should NOT see freestyle tool")
	}
}
```

5.2. Для этого теста нужен exported метод. Добавить в `agent1_execute.go`:

```go
// GetToolDefs returns the filtered tool definitions for Agent1 (exported for testing)
func (uc *Agent1ExecuteUseCase) GetToolDefs(ctx context.Context) []domain.ToolDefinition {
	return uc.getAgent1Tools()
}
```

---

### 6. Validation

```bash
cd project/backend && go build ./...
cd project/backend && go test ./internal/tools/... -v
cd project/backend && go test ./internal/usecases/... -v -run TestAgent1_OnlyGetsDataTools
```

---

## Validation Commands

```bash
# Backend build (required)
cd project/backend && go build ./...

# Backend tests
cd project/backend && go test ./...

# Frontend build (required — промпты не затрагивают фронт, но проверяем)
cd project/frontend && npm run build

# Frontend lint
cd project/frontend && npm run lint
```

---

## Acceptance Criteria

### Agent1 Tool Isolation
- [ ] `agent1_execute.go` содержит `getAgent1Tools()` с фильтром `search_*`
- [ ] Agent1 НЕ видит `render_product_preset`, `render_service_preset`, `freestyle`
- [ ] Agent1 видит `search_products`
- [ ] При style-запросе Agent1 НЕ вызывает тул (stop_reason="end_turn")

### Agent1 Prompt
- [ ] Agent1SystemPrompt содержит правило "DO NOT call tool" для style запросов
- [ ] Примеры на русском: "покажи с большими заголовками" → не вызывать тул
- [ ] 4+ примеров style-запросов без вызова тула

### Agent2 Prompt
- [ ] Decision flow содержит "NO data_change → ALWAYS use freestyle"
- [ ] Примеры на русском с freestyle вызовом
- [ ] `BuildAgent2ToolPrompt` передаёт `"data_change": null` когда дельты нет

### Лог реестра
- [ ] `tool_registry_initialized` показывает все 4 тула включая freestyle
- [ ] Лог динамический (не захардкожен)

### Tests
- [ ] Тест `TestAgent1_OnlyGetsDataTools` проходит
- [ ] Существующие тесты не сломаны
- [ ] `go build ./...` проходит
- [ ] `go test ./...` проходит

---

## Test Scenarios

```gherkin
Scenario: Style query — Agent1 does nothing, Agent2 uses freestyle
  Given session has 7 Nike products from previous search
  When user sends "покажи с большими заголовками"
  Then Agent1 does NOT call any tool (no data change needed)
  And Agent2 receives user_request="покажи с большими заголовками" and data_change=null
  And Agent2 calls freestyle(entity_type="product", formation="grid", style="product-hero")
  And UI shows 7 products with large titles

Scenario: Search query — Agent1 searches, Agent2 renders
  Given fresh session
  When user sends "покажи кроссовки Nike"
  Then Agent1 calls search_products(query="кроссовки", brand="Nike")
  And Agent2 receives data_change={tool:"search_products", count:7}
  And Agent2 calls render_product_preset(preset="product_grid")
  And UI shows 7 products in grid

Scenario: New search after style — Agent1 searches, Agent2 re-renders
  Given session has 7 Nike products displayed with freestyle hero style
  When user sends "покажи Jordan"
  Then Agent1 calls search_products(query="Jordan")
  And Agent2 receives data_change={tool:"search_products", count:N}
  And Agent2 calls render_product_preset(preset="product_grid")
  And UI shows Jordan products in default grid

Scenario: Agent1 tool list verification
  Given tool registry has search_products, render_product_preset, render_service_preset, freestyle
  When Agent1 gets tool definitions
  Then Agent1 sees only search_products
  And Agent1 does NOT see render_product_preset
  And Agent1 does NOT see freestyle
```

---

## Notes

### Padding тулы и prompt caching

`GetDefinitions()` включает ~20 padding тулов (`_internal_*`) для достижения порога 4096 токенов Anthropic prompt caching. `getAgent1Tools()` фильтрует по `search_*` — padding тулы НЕ попадут. Нужно проверить что Agent1 prompt + messages + search_products definition > 4096 токенов. Если нет — добавить padding тулы в `getAgent1Tools()`.

Из лога: Agent1 первый запрос `cache_creation_input_tokens=5815` — это включает padding. После фильтра может стать <4096. **Обязательно проверить cache_creation при первом запросе после деплоя.**

### Agent1 "no tool call" path

Когда Agent1 не вызывает тул (style-запрос), flow:
1. Agent1 возвращает response с `ToolName: ""`, `ProductsFound: 0`
2. Pipeline продолжает к Agent2 (нет early exit)
3. Agent2 видит `state.Current.Meta.ProductCount > 0` (данные от предыдущего запроса)
4. Agent2 НЕ short-circuits, рендерит
5. Agent2 получает `data_change: null` → freestyle

Этот path уже работает — Agent1 без тула не ломает pipeline. Проверено по коду: `agent1_execute.go:171-177` логирует warn и продолжает.

### Conversation history при style-запросе

Когда Agent1 не вызывает тул, `AppendConversation` добавляет только `user` message (без assistant:tool_use и user:tool_result). Это корректно — LLM не вызвал тул, нет tool_result для записи.

**Проверить:** строки 179-198 agent1_execute.go — `if len(llmResp.ToolCalls) > 0` gate. Если 0 tool calls — в историю идёт только user message.

### Scope

- **НЕ в scope:** рефакторинг `getAgent1Tools` / `getAgent2Tools` в общий метод с параметром фильтра
- **НЕ в scope:** добавление новых search тулов (search_services)
- **НЕ в scope:** изменение Agent2 tool execution logic
- **НЕ в scope:** frontend изменения (не нужны)

---

## Estimate

| # | Файл | Что | ~Строк |
|---|------|-----|--------|
| 1 | `agent1_execute.go` | getAgent1Tools() + замена GetDefinitions + GetToolDefs | ~15 |
| 2 | `prompt_analyze_query.go` | Новый Agent1SystemPrompt | ~20 (замена) |
| 3 | `prompt_compose_widgets.go` | Agent2ToolSystemPrompt + BuildAgent2ToolPrompt | ~30 (замена) |
| 4 | `main.go` | Динамический лог | ~5 |
| 5 | `agent1_execute_test.go` | TestAgent1_OnlyGetsDataTools | ~35 |
| | **Total** | | **~105** |
