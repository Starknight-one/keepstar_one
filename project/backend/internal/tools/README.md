# Tools

Tool executors for LLM tool calling.

## Файлы

- `tool_registry.go` — Registry для всех tools
- `tool_search_products.go` — Поиск товаров (Agent1)
- `tool_render_preset.go` — Рендеринг с пресетами (Agent2). Exports: BuildFormation(), FieldGetter, CurrencyGetter, IDGetter
- `tool_freestyle.go` — Freestyle рендеринг со стилевыми алиасами и кастомными display overrides (Agent2)
- `mock_tools.go` — Padding tools для достижения порога кэширования (4096 tokens)
- `tool_search_products_test.go` — Тесты SearchProductsTool
- `tool_render_preset_test.go` — Тесты RenderPresetTool

## Registry

Центральный реестр инструментов:

```go
type Registry struct {
    tools          map[string]ToolExecutor
    statePort      ports.StatePort
    catalogPort    ports.CatalogPort
    presetRegistry *presets.PresetRegistry
}

// Создание с зависимостями
presetRegistry := presets.NewPresetRegistry()
registry := tools.NewRegistry(statePort, catalogPort, presetRegistry)

// Получение definitions для LLM
defs := registry.GetDefinitions()

// Выполнение tool call
toolCtx := tools.ToolContext{SessionID: sessionID, TurnID: turnID, ActorID: "agent1"}
result, err := registry.Execute(ctx, toolCtx, toolCall)
```

## ToolExecutor Interface

```go
type ToolExecutor interface {
    Definition() domain.ToolDefinition
    Execute(ctx context.Context, toolCtx ToolContext, input map[string]interface{}) (*ToolResult, error)
}
```

## SearchProductsTool

Поиск товаров с записью в state:

Input schema:
- `query` (required) — поисковый запрос
- `category` — фильтр по категории
- `brand` — фильтр по бренду
- `min_price` — минимальная цена
- `max_price` — максимальная цена
- `limit` — лимит (default: 10)

Возвращает:
- `"ok: found N products"` — товары найдены и записаны в state
- `"empty"` — ничего не найдено

## Padding Tools (mock_tools.go)

Временные инструменты для достижения минимального порога кэширования Anthropic (4096 tokens).
- `CachePaddingEnabled` — флаг включения (default: true)
- `GetCachePaddingTools()` — возвращает 8 dummy tools с prefix `_internal_`
- LLM не должен их вызывать (описание: "INTERNAL SYSTEM TOOL - DO NOT USE")
- ~3200 tokens, вместе с реальными tools ~4000

## FreestyleTool

Freestyle рендеринг с custom стилями (Agent2):

Input schema:
- `entity_type` (required) — "product" или "service"
- `formation` (required) — "grid", "list", "carousel", "single"
- `style` — стилевой алиас (product-hero, product-compact, product-detail, service-card, service-detail)
- `overrides` — slot→display map для явных display overrides
- `limit` — лимит entities (default: all)

Возвращает: `"ok: rendered N entity_types with freestyle style=X, formation=Y"`

## Правила

- Импорты: `domain/`, `ports/`
- Tools пишут результат в state, не возвращают данные напрямую
- Возвращают только статус: "ok" / "empty" / error
- `GetDefinitions()` автоматически включает padding tools
