# Use Cases

Бизнес-логика. Один файл = один use case.

## Файлы

- `chat_send_message.go` — Отправка сообщения с сохранением в БД
- `catalog_list_products.go` — Список товаров тенанта с фильтрацией
- `catalog_get_product.go` — Получение товара с merging master данных
- `agent1_execute.go` — Agent 1 (Tool Caller) для two-agent pipeline
- `agent1_execute_test.go` — Тесты Agent 1
- `agent2_execute.go` — Agent 2 (Template Builder) для two-agent pipeline
- `agent2_execute_test.go` — Тесты Agent 2
- `pipeline_execute.go` — Оркестратор: Agent 1 → Agent 2 → Formation
- `template_apply.go` — Применение шаблона к данным
- `state_reconstruct.go` — Реконструкция state на любой шаг
- `state_rollback.go` — Откат state на предыдущий шаг
- `state_rollback_test.go` — Интеграционные тесты rollback/reconstruct
- `navigation_expand.go` — Drill-down: expand widget to detail view
- `navigation_back.go` — Navigate back from detail view
- `navigation_test.go` — Navigation tests

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

## Agent2ExecuteUseCase

Agent 2 (Template Builder) для two-agent pipeline:
- Получает состояние сессии (после Agent 1)
- Проверяет наличие данных (count > 0)
- Вызывает LLM с meta данными (не сырыми данными!)
- Парсит JSON шаблон из ответа
- Сохраняет шаблон в state

```go
type Agent2ExecuteUseCase struct {
    llm       ports.LLMPort
    statePort ports.StatePort
}

func (uc *Agent2ExecuteUseCase) Execute(ctx, req) (*Agent2ExecuteResponse, error)
```

## PipelineExecuteUseCase

Оркестратор полного pipeline:
- Step 1: Agent 1 (Tool Caller) — query → tool call → state
- Step 2: Agent 2 (Template Builder) — meta → template → state
- Step 3: ApplyTemplate — template + data → FormationWithData

```go
type PipelineExecuteUseCase struct {
    agent1UC  *Agent1ExecuteUseCase
    agent2UC  *Agent2ExecuteUseCase
    statePort ports.StatePort
    log       *logger.Logger
}

func (uc *PipelineExecuteUseCase) Execute(ctx, req) (*PipelineExecuteResponse, error)
```

## ApplyTemplate

Функция применения шаблона к данным:
- FormationTemplate + []Product → FormationWithData
- Маппинг полей: name→Name, price→Price, images→Images[0]

```go
func ApplyTemplate(template *FormationTemplate, products []Product) (*FormationWithData, error)
```

## ReconstructStateUseCase

Реконструкция состояния сессии на любой шаг:
- Получает дельты до целевого шага (GetDeltasUntil)
- Строит базовое состояние (step 0)
- Последовательно применяет дельты
- Возвращает реконструированное состояние

```go
type ReconstructStateUseCase struct {
    statePort ports.StatePort
}

func (uc *ReconstructStateUseCase) Execute(ctx, req) (*ReconstructResponse, error)
```

## RollbackUseCase

Откат состояния на предыдущий шаг:
- Получает текущее состояние
- Валидирует целевой шаг (нельзя вперёд, нельзя < 0)
- Реконструирует состояние на целевой шаг
- Создаёт rollback delta (сохраняет историю)
- Обновляет текущее состояние

```go
type RollbackUseCase struct {
    statePort     ports.StatePort
    reconstructUC *ReconstructStateUseCase
}

func (uc *RollbackUseCase) Execute(ctx, req) (*RollbackResponse, error)
```

## ExpandUseCase

Drill-down: расширение виджета до детального просмотра:
- Сохраняет текущий view в ViewStack
- Устанавливает view в detail mode
- Рендерит detail preset для entity

```go
type ExpandUseCase struct {
    statePort      ports.StatePort
    presetRegistry *presets.PresetRegistry
}

func (uc *ExpandUseCase) Execute(ctx, req) (*ExpandResponse, error)
```

## BackUseCase

Навигация назад из детального просмотра:
- Pop view из ViewStack
- Восстанавливает предыдущее состояние view
- Перерендеривает предыдущую formation

```go
type BackUseCase struct {
    statePort      ports.StatePort
    presetRegistry *presets.PresetRegistry
}

func (uc *BackUseCase) Execute(ctx, req) (*BackResponse, error)
```

## Правила

- Импорты только из `domain/` и `ports/`
- Нельзя импортировать `adapters/` напрямую (только через порты)
- Каждый use case — структура с методом Execute()
- Graceful degradation если БД не настроена
- Agent2 работает только с meta, никогда не видит сырые данные
