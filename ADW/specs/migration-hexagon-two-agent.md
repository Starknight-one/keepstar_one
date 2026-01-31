# Migration: Hexagonal Architecture + Two-Agent Pipeline

## Overview

Миграция с простого chat (query → response) на двухагентный пайплайн с гексагональной архитектурой.

**Цель на завтра:** Рабочий MVP где User Query → Agent 1 → Cache → Agent 2 → Widget Builder → JSON Response

---

## Architecture Context

```
┌─────────────────────────────────────────────────────────────────────┐
│                         PRODUCTS                                     │
│                                                                      │
│  ┌──────────────────┐    ┌──────────────────────────────────────┐  │
│  │   ADMIN PANEL    │    │   EMBEDDABLE WIDGET (like Intercom)  │  │
│  │   (для клиентов) │    │   (встраивается на сайты клиентов)   │  │
│  └────────┬─────────┘    └─────────────────┬────────────────────┘  │
│           │                                 │                        │
│           └─────────────┬───────────────────┘                        │
│                         │                                            │
│                         ▼                                            │
│              ┌─────────────────────┐                                 │
│              │   BACKEND (Go)      │                                 │
│              │   Hexagonal Arch    │                                 │
│              └─────────────────────┘                                 │
└─────────────────────────────────────────────────────────────────────┘
```

**MVP Scope (завтра):** Только Embeddable Widget flow. Админка — потом.

---

## Current State Analysis

### Что есть и работает
- `main.go` — старый рабочий endpoint
- `anthropic.go` — рабочий вызов API

### Что есть как стабы (TODO: implement)
| Файл | Статус | Нужно |
|------|--------|-------|
| `domain/*.go` | ✅ Готово | Минимальные доработки |
| `ports/llm_port.go` | ⚠️ Частично | Добавить Tools interface |
| `ports/cache_port.go` | ✅ Готово | - |
| `ports/search_port.go` | ✅ Готово | - |
| `adapters/anthropic/` | ❌ Стаб | Полная реализация + Tools |
| `adapters/memory/` | ❌ Стаб | Реализовать методы |
| `adapters/json_store/` | ❌ Стаб | Реализовать Search |
| `usecases/chat_*.go` | ❌ Стабы | Полная реализация |
| `handlers/handler_chat.go` | ❌ Стаб | Реализовать |

### Чего нет совсем
- `ChatOrchestrator` — центральный компонент пайплайна
- `WidgetBuilder` — сборка виджетов из шаблонов + данных
- `ToolExecutor` — выполнение tools для Agent 1
- `ToolRegistry` — регистрация доступных tools

---

## Data Flow (детальный)

```
1. HTTP Request
   POST /api/v1/chat { sessionId?, message }

2. ChatHandler
   - Парсит request
   - Вызывает Orchestrator.Process()

3. Orchestrator
   ┌──────────────────────────────────────────┐
   │ 3.1 Load/Create Session                  │
   │     cache.GetSession(sessionId)          │
   │                                          │
   │ 3.2 Build Agent 1 Context                │
   │     - User message                       │
   │     - Session state (что в кэше)         │
   │     - Available tools                    │
   │                                          │
   │ 3.3 Call Agent 1 (Query Analyzer)        │
   │     → LLM decides which tools to call    │
   │     → ToolExecutor runs tools            │
   │     → Results go to cache (NOT to LLM)   │
   │     → LLM gets only: {success, count}    │
   │                                          │
   │ 3.4 Build Agent 2 Context                │
   │     - User message                       │
   │     - Cache summary (count, 1 example)   │
   │     - Available widget types             │
   │                                          │
   │ 3.5 Call Agent 2 (Widget Composer)       │
   │     → Returns template + layout config   │
   │                                          │
   │ 3.6 Widget Builder                       │
   │     - Takes template from Agent 2        │
   │     - Takes real data from cache         │
   │     - Produces ready widgets JSON        │
   └──────────────────────────────────────────┘

4. HTTP Response
   { sessionId, widgets: [...], displayConfig }
```

---

## Component Breakdown

### Phase 1: Foundation (30%)

#### 1.1 Memory Cache Implementation
**File:** `internal/adapters/memory/memory_cache.go`

Реализовать:
```go
func (c *Cache) GetSession(ctx, id) (*Session, error)
func (c *Cache) SaveSession(ctx, session) error
func (c *Cache) CacheProducts(ctx, sessionID, products) error
func (c *Cache) GetCachedProducts(ctx, sessionID) ([]Product, error)
```

**Acceptance:** Unit test проходит

#### 1.2 JSON Store Implementation
**File:** `internal/adapters/json_store/json_product_store.go`

Реализовать:
- Загрузка products.json при старте
- Search с фильтрами (category, price range, etc.)
- GetByID

**Data file:** `data/products.json` — создать тестовые данные (10-20 products)

**Acceptance:** Search возвращает отфильтрованные продукты

---

### Phase 2: Tools System (25%)

#### 2.1 Tool Definitions
**New file:** `internal/domain/tool.go`

```go
type Tool struct {
    Name        string
    Description string
    Parameters  []ToolParam
}

type ToolParam struct {
    Name        string
    Type        string
    Required    bool
    Description string
}

type ToolCall struct {
    Name   string
    Params map[string]any
}

type ToolResult struct {
    Success bool
    Count   int
    Meta    map[string]any  // минимум метаданных
}
```

#### 2.2 Tool Registry
**New file:** `internal/tools/registry.go`

```go
type ToolRegistry struct {
    tools map[string]ToolHandler
}

type ToolHandler func(ctx, params, cache) ToolResult

// Регистрация
func (r *ToolRegistry) Register(name string, handler ToolHandler)

// Получение списка tools для LLM
func (r *ToolRegistry) GetToolDefinitions() []Tool
```

#### 2.3 Tool Implementations
**New file:** `internal/tools/search_tools.go`

Реализовать:
- `search_products` — поиск в БД, сохранение в кэш
- `filter_cached` — фильтрация кэша
- `sort_cached` — сортировка кэша
- `get_cached_count` — сколько в кэше

**Key principle:** Каждый tool:
1. Выполняет операцию
2. Сохраняет результат в cache
3. Возвращает ТОЛЬКО {success, count, meta} — НЕ данные

#### 2.4 Tool Executor
**New file:** `internal/tools/executor.go`

```go
type ToolExecutor struct {
    registry *ToolRegistry
    cache    ports.CachePort
    search   ports.SearchPort
}

func (e *ToolExecutor) Execute(ctx, calls []ToolCall) []ToolResult
```

---

### Phase 3: Anthropic Adapter (20%)

#### 3.1 Update LLM Port
**File:** `internal/ports/llm_port.go`

Добавить:
```go
type LLMPort interface {
    // Existing
    AnalyzeQuery(ctx, req) (*AnalyzeQueryResponse, error)
    ComposeWidgets(ctx, req) (*ComposeWidgetsResponse, error)

    // New: для работы с tools
    ChatWithTools(ctx, req ChatWithToolsRequest) (*ChatWithToolsResponse, error)
}

type ChatWithToolsRequest struct {
    SystemPrompt string
    Messages     []Message
    Tools        []Tool
}

type ChatWithToolsResponse struct {
    Content   string
    ToolCalls []ToolCall
    StopReason string  // "end_turn" or "tool_use"
}
```

#### 3.2 Anthropic Client Implementation
**File:** `internal/adapters/anthropic/anthropic_client.go`

Реализовать:
- HTTP вызов к Anthropic API
- Tool Use format (по документации Anthropic)
- Парсинг tool_calls из ответа
- Обработка stop_reason

**Reference:** https://docs.anthropic.com/en/docs/tool-use

---

### Phase 4: Agents (15%)

#### 4.1 Agent 1: Query Analyzer
**File:** `internal/usecases/chat_analyze_query.go`

```go
type AnalyzeQueryUseCase struct {
    llm      ports.LLMPort
    executor *ToolExecutor
}

func (uc *AnalyzeQueryUseCase) Execute(ctx, query, session) (*AnalysisResult, error) {
    // 1. Build system prompt (tools available, session context)
    // 2. Call LLM with tools
    // 3. Loop while stop_reason == "tool_use":
    //    - Execute tool calls
    //    - Send results back (only meta, not data!)
    //    - Call LLM again
    // 4. Return final analysis
}
```

**System Prompt включает:**
- Available tools с описаниями
- Session context: "В кэше N товаров категории X"
- Instructions: когда искать vs когда фильтровать

#### 4.2 Agent 2: Widget Composer
**File:** `internal/usecases/chat_compose_widgets.go`

```go
type ComposeWidgetsUseCase struct {
    llm   ports.LLMPort
    cache ports.CachePort
}

func (uc *ComposeWidgetsUseCase) Execute(ctx, query, session) (*WidgetTemplate, error) {
    // 1. Get 1 example product from cache
    // 2. Build system prompt:
    //    - Available widget types
    //    - Example product structure
    //    - "В кэше N товаров"
    // 3. Call LLM (no tools, just generation)
    // 4. Parse response as WidgetTemplate
}
```

**Output:**
```go
type WidgetTemplate struct {
    Type       WidgetType
    Layout     LayoutConfig  // grid, list, carousel
    Fields     []string      // какие поля показывать
    MaxItems   int
}
```

---

### Phase 5: Widget Builder (5%)

#### 5.1 Widget Builder
**New file:** `internal/services/widget_builder.go`

```go
type WidgetBuilder struct {
    cache ports.CachePort
}

func (wb *WidgetBuilder) Build(ctx, template, sessionID) ([]Widget, error) {
    // 1. Get products from cache
    // 2. Apply template to each product
    // 3. Return ready widgets with real data
}
```

**Key:** Это НЕ LLM, это чистый Go код — быстро и предсказуемо.

---

### Phase 6: Orchestrator (5%)

#### 6.1 Chat Orchestrator
**New file:** `internal/usecases/chat_orchestrator.go`

```go
type ChatOrchestrator struct {
    cache          ports.CachePort
    analyzeQuery   *AnalyzeQueryUseCase
    composeWidgets *ComposeWidgetsUseCase
    widgetBuilder  *WidgetBuilder
}

func (o *ChatOrchestrator) Process(ctx, req ChatRequest) (*ChatResponse, error) {
    // 1. Get/Create session
    // 2. Run Agent 1
    // 3. Run Agent 2
    // 4. Build widgets
    // 5. Save session
    // 6. Return response
}
```

#### 6.2 Update Chat Handler
**File:** `internal/handlers/handler_chat.go`

- Парсинг request
- Вызов orchestrator
- Формирование response

---

## Implementation Order (для завтра)

```
┌─────────────────────────────────────────────────────────────┐
│ MORNING: Foundation                                          │
├─────────────────────────────────────────────────────────────┤
│ 1. Memory Cache (30 min)                                    │
│ 2. JSON Store + test data (30 min)                          │
│ 3. Tool definitions + registry (30 min)                     │
│ 4. Search tools implementation (45 min)                     │
├─────────────────────────────────────────────────────────────┤
│ MIDDAY: LLM Integration                                      │
├─────────────────────────────────────────────────────────────┤
│ 5. Anthropic client with tools (60 min)                     │
│ 6. Agent 1 usecase (45 min)                                 │
│ 7. Agent 2 usecase (30 min)                                 │
├─────────────────────────────────────────────────────────────┤
│ AFTERNOON: Assembly                                          │
├─────────────────────────────────────────────────────────────┤
│ 8. Widget Builder (30 min)                                  │
│ 9. Orchestrator (30 min)                                    │
│ 10. Handler + routes (20 min)                               │
│ 11. Wire up in main.go (15 min)                             │
│ 12. E2E test (30 min)                                       │
└─────────────────────────────────────────────────────────────┘
```

---

## Files to Create/Modify

### New Files
```
internal/
├── domain/
│   └── tool.go                    # NEW
├── tools/
│   ├── registry.go                # NEW
│   ├── executor.go                # NEW
│   └── search_tools.go            # NEW
├── services/
│   └── widget_builder.go          # NEW
└── usecases/
    └── chat_orchestrator.go       # NEW

data/
└── products.json                  # NEW (test data)
```

### Files to Implement (from stubs)
```
internal/
├── adapters/
│   ├── anthropic/anthropic_client.go
│   ├── memory/memory_cache.go
│   └── json_store/json_product_store.go
├── usecases/
│   ├── chat_analyze_query.go
│   └── chat_compose_widgets.go
├── handlers/
│   └── handler_chat.go
└── cmd/server/main.go             # Update wiring
```

### Files to Delete (after migration)
```
project/backend/
├── main.go        # старый entry point
├── anthropic.go   # старый client
└── gigachat.go    # старый client
```

---

## Validation Checkpoints

### Checkpoint 1: Cache works
```bash
# Unit test
go test ./internal/adapters/memory/...
```

### Checkpoint 2: Tools execute
```bash
# Manual test: search returns products, saves to cache
```

### Checkpoint 3: Agent 1 works
```bash
# Test: query → tool calls → cache populated
```

### Checkpoint 4: Agent 2 works
```bash
# Test: query + cache → widget template
```

### Checkpoint 5: E2E works
```bash
curl -X POST http://localhost:8080/api/v1/chat \
  -H "Content-Type: application/json" \
  -d '{"message": "покажи бензопилы до 500 долларов"}'

# Expected: JSON with widgets array
```

---

## Open Questions

1. **Session ID generation** — UUID или что-то специфичное?
2. **Error handling** — что возвращать фронту при ошибке LLM?
3. **Timeout** — сколько ждать ответа от Anthropic?
4. **Rate limiting** — нужен ли для MVP?

---

## Risk Mitigation

| Risk | Mitigation |
|------|------------|
| Anthropic Tool Use сложнее чем ожидалось | Сначала реализовать без tools, добавить позже |
| Agent 1 loop зависает | Ограничить max_iterations = 5 |
| Widget Builder не понимает template | Строгая JSON schema для template |

---

## Notes

- Старый код (`main.go`, `anthropic.go`) НЕ ТРОГАЕМ пока новый не заработает
- Новый код запускается из `cmd/server/main.go`
- Порты разные: старый 8080, новый 8081 (для параллельного тестирования)
