# Use Cases

Бизнес-логика. Один файл = один use case.

## Файлы

- `chat_analyze_query.go` — Agent 1: запрос → intent + search params
- `chat_compose_widgets.go` — Agent 2: данные → layout decision
- `chat_execute_search.go` — Поиск товаров по параметрам

## Правила

- Импорты только из `domain/` и `ports/`
- Нельзя импортировать `adapters/` напрямую (только через порты)
- Каждый use case — структура с методом Execute()

## Паттерн

```go
type AnalyzeQueryUseCase struct {
    llm ports.LLMPort
    log *logger.Logger
}

func (uc *AnalyzeQueryUseCase) Execute(ctx context.Context, req Request) (*Result, error) {
    // ...
}
```
