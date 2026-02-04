# Prompts

LLM промпты. Отдельно от бизнес-логики.

## Файлы

- `prompt_analyze_query.go` — Промпт для Agent 1 (Tool Caller)
- `prompt_compose_widgets.go` — Промпт для Agent 2 (Template Builder)

## Agent 1 (prompt_analyze_query.go)

```go
const Agent1SystemPrompt = `...`
```

Правила Agent 1:
- ВСЕГДА вызывает tool, никогда не отвечает текстом
- Без объяснений и уточняющих вопросов
- Tool возвращает "ok" или "empty"
- Останавливается после первого tool call

## Agent 2 (prompt_compose_widgets.go)

Два режима работы:

```go
// Text-based template building
const Agent2SystemPrompt = `...`
func BuildAgent2Prompt(meta StateMeta, layoutHint string) string

// Tool-based preset rendering
const Agent2ToolSystemPrompt = `...`
func BuildAgent2ToolPrompt(meta StateMeta, layoutHint string) string
```

Правила Agent 2:
- ТОЛЬКО валидный JSON, без объяснений
- Использует только поля из input
- Выбирает размер виджета по количеству атомов
- Выбирает mode по количеству items (1→single, 2-6→grid, 7+→grid preferred, carousel only if asked)

## Правила

- System prompt и User template раздельно
- Функция-билдер для подстановки переменных
- Импорты только из `domain/` (для типов)
