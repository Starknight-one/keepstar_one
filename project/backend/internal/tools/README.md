# Tools

Tool executors for LLM tool calling.

## Файлы

- `tool_registry.go` — Registry для всех tools
- `tool_catalog_search.go` — Hybrid search meta-tool: keyword SQL + vector pgvector + RRF merge (Agent1)
- `tool_search_products.go` — Legacy поиск товаров (не зарегистрирован в Registry)
- `tool_render_preset.go` — Рендеринг с пресетами (Agent2). Exports: BuildFormation(), FieldGetter, CurrencyGetter, IDGetter
- `tool_freestyle.go` — Freestyle рендеринг со стилевыми алиасами и кастомными display overrides (Agent2)
- `mock_tools.go` — Padding tools для достижения порога кэширования (4096 tokens)
- `tool_catalog_search_test.go` — Тесты CatalogSearchTool
- `tool_render_preset_test.go` — Тесты RenderPresetTool

## Registry

Центральный реестр инструментов:

```go
type Registry struct {
    tools          map[string]ToolExecutor
    statePort      ports.StatePort
    catalogPort    ports.CatalogPort
    presetRegistry *presets.PresetRegistry
    embeddingPort  ports.EmbeddingPort
}

// Создание с зависимостями
presetRegistry := presets.NewPresetRegistry()
registry := tools.NewRegistry(statePort, catalogPort, presetRegistry, embeddingPort)

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

## CatalogSearchTool (registered, Agent1)

Hybrid search meta-tool: keyword SQL + vector pgvector + RRF merge → state write.

Input schema:
- `vector_query` (required) — semantic search в ОРИГИНАЛЬНОМ языке пользователя
- `filters` — object с keyword filters: brand, category, min_price, max_price, color, material, storage, ram, size
- `sort_by` — price, rating, name
- `sort_order` — asc, desc
- `limit` — лимит (default: 10)

Flow:
1. Parse input, convert prices (rubles → kopecks x100)
2. Generate query embedding via EmbeddingPort (span: `{stage}.tool.embed`)
3. Keyword search via catalogPort.ListProducts (span: `{stage}.tool.sql`)
4. Vector search via catalogPort.VectorSearch (span: `{stage}.tool.vector`)
5. RRF merge: combine keyword + vector results (k=60, keyword weight 1.5× default, 2.0× with filters)
6. Write products to state via UpdateData zone-write

Возвращает: `"ok: found N products"` / `"empty: 0 results, previous data preserved"`
Metadata: embed_ms, sql_ms, vector_ms, keyword_count, vector_count, merged_count, search_type

## SearchProductsTool (legacy, NOT registered)

Legacy поиск товаров с записью в state (не зарегистрирован в Registry, заменён CatalogSearchTool):

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
- `GetCachePaddingTools()` — возвращает 20 dummy tools с prefix `_internal_`
- LLM не должен их вызывать (описание: "INTERNAL SYSTEM TOOL - DO NOT USE")
- ~5500 tokens (safely above 4096 min for Haiku 4.5)

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
