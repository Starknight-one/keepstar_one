# Use Cases

Бизнес-логика. Один файл = один use case.

## Файлы

- `chat_send_message.go` — Отправка сообщения с сохранением в БД
- `catalog_list_products.go` — Список товаров тенанта с фильтрацией
- `catalog_get_product.go` — Получение товара с merging master данных
- `agent1_execute.go` — Agent 1 (Tool Caller) для two-agent pipeline

## SendMessageUseCase

Основной use case для чата:
- Получает/создаёт сессию
- Проверяет TTL (10 мин sliding window)
- Отправляет сообщение в LLM
- Сохраняет сообщения в БД
- Трекает события (chat_opened, message_sent, message_received)

```go
type SendMessageUseCase struct {
    llm    ports.LLMPort
    cache  ports.CachePort
    events ports.EventPort
    sessionTTL time.Duration
}

func (uc *SendMessageUseCase) Execute(ctx, req) (*SendMessageResponse, error)
```

## ListProductsUseCase

Список товаров для тенанта:
- Резолвит тенант по slug
- Применяет фильтры (категория, бренд, цена, поиск)
- Возвращает merged данные (tenant + master)

```go
type ListProductsUseCase struct {
    catalog ports.CatalogPort
}

func (uc *ListProductsUseCase) Execute(ctx, req) (*ListProductsResponse, error)
```

## GetProductUseCase

Получение одного товара:
- Резолвит тенант по slug
- Получает товар с merging master данных

```go
type GetProductUseCase struct {
    catalog ports.CatalogPort
}

func (uc *GetProductUseCase) Execute(ctx, req) (*Product, error)
```

## Agent1ExecuteUseCase

Agent 1 (Tool Caller) для two-agent pipeline:
- Получает/создаёт state сессии
- Вызывает LLM с tools (ChatWithTools)
- Выполняет tool call через Registry
- Создаёт и сохраняет delta

```go
type Agent1ExecuteUseCase struct {
    llm          ports.LLMPort
    statePort    ports.StatePort
    toolRegistry *tools.Registry
    log          *logger.Logger
}

func (uc *Agent1ExecuteUseCase) Execute(ctx, req) (*Agent1ExecuteResponse, error)
```

## Правила

- Импорты только из `domain/` и `ports/`
- Нельзя импортировать `adapters/` напрямую (только через порты)
- Каждый use case — структура с методом Execute()
- Graceful degradation если БД не настроена
