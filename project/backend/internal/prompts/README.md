# Prompts

LLM промпты. Отдельно от бизнес-логики.

## Файлы

- `prompt_analyze_query.go` — Промпт для Agent 1 (query analysis)
- `prompt_compose_widgets.go` — Промпт для Agent 2 (widget composition)

## Правила

- System prompt и User template раздельно
- Функция-билдер для подстановки переменных
- Импорты только из `domain/` (для типов)

## Паттерн

```go
const AnalyzeQuerySystemPrompt = `...`
const AnalyzeQueryUserTemplate = `...`

func BuildAnalyzeQueryPrompt(query string, ctx Context) string {
    // ...
}
```
