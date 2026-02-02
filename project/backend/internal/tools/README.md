# Tools

Tool executors for LLM tool calling.

## Файлы

- `tool_registry.go` — Registry для всех tools
- `tool_search_products.go` — Поиск товаров

## Registry

Центральный реестр инструментов:

```go
type Registry struct {
    tools       map[string]ToolExecutor
    statePort   ports.StatePort
    catalogPort ports.CatalogPort
}

// Создание с зависимостями
registry := tools.NewRegistry(statePort, catalogPort)

// Получение definitions для LLM
defs := registry.GetDefinitions()

// Выполнение tool call
result, err := registry.Execute(ctx, sessionID, toolCall)
```

## ToolExecutor Interface

```go
type ToolExecutor interface {
    Definition() domain.ToolDefinition
    Execute(ctx, sessionID, input) (*ToolResult, error)
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

## Правила

- Импорты: `domain/`, `ports/`
- Tools пишут результат в state, не возвращают данные напрямую
- Возвращают только статус: "ok" / "empty" / error
