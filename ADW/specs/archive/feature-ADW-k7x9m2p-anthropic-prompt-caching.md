# Feature: ADW-k7x9m2p Anthropic Prompt Caching

## Feature Description

Внедрение Anthropic Prompt Caching для оптимизации стоимости и latency в двухагентном пайплайне. Кэширование позволит переиспользовать system prompts, tool definitions и историю разговора между запросами.

В процессе работы выявлена необходимость рефакторинга state management: переход от blob-апдейтов к зонированным записям с автоматическими дельтами и группировкой в Turn'ы.

## Objective

1. **Экономия токенов** — кэшированные токены стоят 10% от базовой цены
2. **Поддержка истории разговора** — LLM понимает контекст предыдущих сообщений
3. **Снижение latency** — кэшированный контент не перечитывается заново
4. **Метрики** — отслеживание cache hit rate для оценки экономики
5. **State zones** — зонированные апдейты вместо blob, автоматические дельты

---

## Implementation Status

### DONE (go build passes)

| # | Шаг | Статус | Файлы |
|---|------|--------|-------|
| 0 | Padding tools для порога кэширования | DONE | `tools/mock_tools.go`, `tools/tool_registry.go` |
| 1 | Cache поля в LLMUsage + CalculateCost | DONE | `domain/tool_entity.go` |
| 2 | Anthropic cache types | DONE | `adapters/anthropic/cache_types.go` |
| 3 | ChatWithToolsCached метод | DONE | `adapters/anthropic/anthropic_client.go` |
| 4 | CacheConfig + LLMPort interface | DONE | `ports/llm_port.go` |
| 5 | ConversationHistory в SessionState | DONE | `domain/state_entity.go` |
| 6 | Agent1 использует ChatWithToolsCached | DONE | `usecases/agent1_execute.go` |
| 7 | Agent2 использует ChatWithToolsCached | DONE | `usecases/agent2_execute.go` |
| 8 | Logger с cache metrics | DONE | `logger/logger.go` |
| 9 | Debug page — cache метрики | DONE | `handlers/handler_debug.go`, `handlers/handler_pipeline.go` |
| 10 | Cache integration test | DONE | `usecases/cache_test.go` |
| 11 | AddDelta auto-increment step (порт) | DONE | `ports/state_port.go` |
| 12 | AddDelta auto-increment (postgres) | DONE | `adapters/postgres/postgres_state.go` |
| 13 | Убран state.Step++ из tools/usecases | DONE | `tool_search_products.go`, `navigation_*.go`, `state_rollback.go`, `handler_debug.go` |

### TODO

| # | Шаг | Описание | Файлы |
|---|------|----------|-------|
| 14 | **Zone-based StatePort** | Заменить `UpdateState(blob)` на 4 зонированных метода. Каждый обновляет свои колонки + автоматически создаёт дельту | `ports/state_port.go`, `adapters/postgres/postgres_state.go` |
| 15 | **Turn (DeltaInfo)** | Добавить `turn_id` в Delta. Pipeline/handler создаёт Turn, передаёт в zone-write. Группировка дельт одного хода | `domain/state_entity.go`, `ports/state_port.go` |
| 16 | **Перевод акторов на zone-write** | search_products → `UpdateData`, render_preset → `UpdateTemplate`, Agent1 → `AppendConversation`, Expand/Back → `UpdateView`. Убрать все вызовы `UpdateState` кроме rollback | `tools/`, `usecases/`, `handlers/` |
| 17 | **search_products: empty = мутация** | При `total == 0` — очищать Data, обнулять Meta через `UpdateData`. Сейчас стейт протухший | `tools/tool_search_products.go` |
| 18 | **Agent2: дельта на обоих путях** | Нормальный путь (render tool) и пустой путь (empty) — оба создают дельту через zone-write | `usecases/agent2_execute.go` |
| 19 | **ConversationHistory в postgres** | Добавить колонку `conversation_history JSONB DEFAULT '[]'`, обновить adapter | `adapters/postgres/postgres_state.go` |
| 20 | **Beta header** | `anthropic-beta: prompt-caching-2024-07-31` в `ChatWithToolsCached` | `adapters/anthropic/anthropic_client.go` |
| 21 | **Padding tools 4096+** | Текущие ~2500 tok, нужно 4096+. Расширить описания или добавить tools | `tools/mock_tools.go` |
| 22 | **Перезапуск cache_test** | После фиксов 14-21 перезапустить `TestPromptCaching_Chain` | `usecases/cache_test.go` |

---

## Architectural Decision: State Zones

### Принцип

Стейт — центральная сущность. Акторы пишут в стейт, читают из стейта. Мутации стейта могут триггерить другие события. Каждая мутация записывается как дельта. Дельты группируются в Turn'ы.

**Инвариант: мутация стейта без дельты невозможна.** Обеспечивается на уровне API порта — нет метода "обновить стейт без дельты".

### 4 зоны

Зона — группа полей, которые всегда обновляются вместе одним актором.

```
SessionState
├── ZONE: data ──────────────────────────────────────
│   ├── Data.Products []Product
│   ├── Data.Services []Service
│   └── Meta (Count, Fields, Aliases)
│   Писатели: search tools
│   Метод: UpdateData(ctx, sessionID, data, meta, deltaInfo)
│   SQL: UPDATE SET current_data=$1, current_meta=$2 WHERE session_id=$3
│
├── ZONE: template ──────────────────────────────────
│   └── Template map[string]interface{} (formation)
│   Писатели: render tools
│   Метод: UpdateTemplate(ctx, sessionID, template, deltaInfo)
│   SQL: UPDATE SET current_template=$1 WHERE session_id=$2
│
├── ZONE: view ──────────────────────────────────────
│   ├── View.Mode (grid/detail/list/carousel)
│   ├── View.Focused *EntityRef
│   └── ViewStack []ViewSnapshot
│   Писатели: navigation (expand, back)
│   Метод: UpdateView(ctx, sessionID, view, stack, deltaInfo)
│   SQL: UPDATE SET view_mode=$1, view_focused=$2, view_stack=$3 WHERE session_id=$4
│
├── ZONE: conversation ──────────────────────────────
│   └── ConversationHistory []LLMMessage
│   Писатели: Agent1 (после каждого turn)
│   Метод: AppendConversation(ctx, sessionID, messages)
│   Append-only, для LLM кэша. Дельта опциональна.
│
└── step — не зона, управляется AddDelta автоматически
```

### Кто какую зону мутирует

```
                data    template    view    conversation
                ─────   ────────    ────    ────────────
search_prod       ✎
render_preset               ✎
Agent1                                          ✎
Agent2           (read)    (read)
Expand                      ✎        ✎
Back                        ✎        ✎
Rollback          ✎         ✎        ✎         ✎
```

### Turn

Turn — логическая группа дельт от одного внешнего события.

```go
type DeltaInfo struct {
    TurnID    string      // группирует дельты одного хода
    Trigger   TriggerType // USER_QUERY / WIDGET_ACTION / SYSTEM
    Source    DeltaSource // user / llm / system
    ActorID   string      // "agent1", "agent2", "user_expand"
    Action    Action      // что произошло
}
```

```
Turn 1: USER_QUERY "покажи Nike"           turn_id=t1
  ├── step=1  agent1 → UpdateData(7 products)
  └── step=2  agent2 → UpdateTemplate(grid)

Turn 2: WIDGET_ACTION expand prod_123      turn_id=t2
  └── step=3  user   → UpdateView(detail, push)

Turn 3: WIDGET_ACTION back                 turn_id=t3
  └── step=4  user   → UpdateView(grid, pop)

Turn 4: USER_QUERY "до 5000"               turn_id=t4
  ├── step=5  agent1 → UpdateData(0 products)
  └── step=6  agent2 → UpdateTemplate(empty)
```

Turn нужен для:
- **Rollback** — откат всех дельт Turn'а, а не одной
- **LLM Cache** — ConversationHistory строится из Turn'ов (USER_QUERY → messages pair)
- **Параллельные агенты** — 5 дельт с одним turn_id = один логический ход
- **Подписки** — агент может реагировать на "Turn completed"

### StatePort interface (целевой)

```go
type StatePort interface {
    // Reads
    GetState(ctx context.Context, sessionID string) (*domain.SessionState, error)
    CreateState(ctx context.Context, sessionID string) (*domain.SessionState, error)

    // Zone writes — каждый обновляет свои колонки + создаёт дельту
    UpdateData(ctx context.Context, sessionID string, data domain.StateData, meta domain.StateMeta, info DeltaInfo) (int, error)
    UpdateTemplate(ctx context.Context, sessionID string, template map[string]interface{}, info DeltaInfo) (int, error)
    UpdateView(ctx context.Context, sessionID string, view domain.ViewState, stack []domain.ViewSnapshot, info DeltaInfo) (int, error)
    AppendConversation(ctx context.Context, sessionID string, messages []domain.LLMMessage) error

    // Navigation helpers
    PushView(ctx context.Context, sessionID string, snapshot *domain.ViewSnapshot) error
    PopView(ctx context.Context, sessionID string) (*domain.ViewSnapshot, error)
    GetViewStack(ctx context.Context, sessionID string) ([]domain.ViewSnapshot, error)

    // Deltas
    AddDelta(ctx context.Context, sessionID string, delta *domain.Delta) (int, error)
    GetDeltas(ctx context.Context, sessionID string) ([]domain.Delta, error)
    GetDeltasSince(ctx context.Context, sessionID string, fromStep int) ([]domain.Delta, error)
    GetDeltasUntil(ctx context.Context, sessionID string, toStep int) ([]domain.Delta, error)
}
```

`UpdateState` остаётся только для rollback (единственный кейс когда нужно перезаписать всё).

### Типизация остаётся

Go struct остаётся типизированным. Postgres колонки — те же JSONB. При добавлении нового типа данных (Orders, Bookings) — добавляем поле в `StateData` struct, зона `data` и метод `UpdateData` не меняются. Конечный набор файлов при расширении: `domain/` → `ports/` → `adapters/` → `usecases/` → `tools/`.

---

## Findings from First Test Run

Запустили `TestPromptCaching_Chain` — 10 запросов в одну сессию.

### Проблема 1: Кэш не активируется

**Симптом**: `cache_creation_input_tokens=0` и `cache_read_input_tokens=0` на всех 10 запросах.

**Причины**:
1. Нет beta header — API игнорирует `cache_control` без `anthropic-beta: prompt-caching-2024-07-31`
2. Ниже порога — Input tokens = ~2509, нужно минимум 4096 для Haiku

**Фикс**: TODO шаги 20-21.

### Проблема 2: Duplicate key на дельтах

**Симптом**: `ERROR: duplicate key value violates unique constraint "chat_session_deltas_session_id_step_key"`

**Причина**: `state.Step++` вызывался только при `total > 0`. При empty — step не рос → конфликт.

**Решение**: DONE шаги 11-13. `AddDelta` с auto-increment через `MAX(step)+1`.

### Проблема 3: ConversationHistory не сохраняется в БД

**Симптом**: `conversation_history: 0 msgs` на всех запросах.

**Причина**: Нет колонки в postgres. Поле есть в Go struct, но не персистится.

**Фикс**: TODO шаг 19.

### Проблема 4: Нарушен инвариант «мутация = дельта»

**Симптом**: Agent2 мутирует стейт, но никогда не создаёт дельту. search_products не обновляет стейт при empty.

**Аудит**:

| Актор | Мутирует стейт | Создаёт дельту | Статус |
|-------|----------------|----------------|--------|
| Expand | ✅ | ✅ | OK |
| Back | ✅ | ✅ | OK |
| Rollback | ✅ | ✅ | OK |
| Agent1 | ✅ | ✅ | Частично — tool не чистит стейт при empty |
| search_products | ✅ (count>0) | нет | Сломано |
| Agent2 | ✅ (через render) | никогда | Сломано |
| render_product_preset | ✅ | нет | Сломано |
| render_service_preset | ✅ | нет | Сломано |

**Фикс**: TODO шаги 14-18. Zone-based writes обеспечат инвариант на уровне API.

---

## How Anthropic Prompt Caching Works

### Структура кэширования
```
┌─────────────────────────────────────────┐
│ tools[]  (определения инструментов)     │ ← cache_control на последнем tool
├─────────────────────────────────────────┤
│ system   (system prompt как массив)     │ ← cache_control на последнем блоке
├─────────────────────────────────────────┤
│ messages (история разговора)            │ ← cache_control на предпоследнем сообщении
├─────────────────────────────────────────┤
│ Новое сообщение пользователя            │ ← НЕ кэшируется
└─────────────────────────────────────────┘
```

### Pricing (Claude Haiku 4.5)
| Тип | Цена за MTok |
|-----|--------------|
| Base input | $1.00 |
| Cache write (5 min) | $1.25 (×1.25) |
| Cache read | $0.10 (×0.1) |
| Output | $5.00 |

### Минимальный порог
- **Claude Haiku 4.5**: 4096 токенов минимум (верифицировать)
- **Claude Sonnet 4.5**: 1024 токена
- **Claude Opus 4.5**: 4096 токенов

### Требуемый header
```
anthropic-beta: prompt-caching-2024-07-31
```

### Связь с Turn

ConversationHistory строится из Turn'ов типа USER_QUERY:
- Turn 1 → `{user: "покажи Nike", assistant: tool_call(search), tool_result: "7 found"}`
- Turn 2 → skip (WIDGET_ACTION — не LLM контекст) (Сообщение от проектировщика = на самом деле нужно, потому что мы можем показать пользователю кучу всего, он будет 5 минут возиться на странице, перебирать всё, смотреть, а потом попросит ЛЛМку что-то вроде "мне понравился кросовок который я вторым открывал, найди похожие. ЛЛМке потребуется понять что он делал чтобы точно подобрать тулл)
- Turn 4 → `{user: "до 5000", assistant: tool_call(search), tool_result: "0 found"}`

Кэш Anthropic видит стабильный prefix из предыдущих Turn'ов → cache hit на tools + system + messages prefix.

---

## Relevant Files

### Modified Files
- `adapters/anthropic/anthropic_client.go` — `ChatWithToolsCached`
- `ports/llm_port.go` — `ChatWithToolsCached` + `CacheConfig`
- `ports/state_port.go` — `AddDelta` возвращает `(int, error)`
- `domain/tool_entity.go` — cache поля в `LLMUsage`, `CalculateCost`
- `domain/state_entity.go` — `ConversationHistory`
- `adapters/postgres/postgres_state.go` — `AddDelta` с auto-increment
- `tools/tool_registry.go` — padding tools
- `tools/tool_search_products.go` — убран `state.Step++`
- `usecases/agent1_execute.go` — `ChatWithToolsCached`, conversation history
- `usecases/agent2_execute.go` — `ChatWithToolsCached`
- `usecases/pipeline_execute.go` — передаёт logger в Agent2
- `usecases/navigation_expand.go` — `AddDelta` с `(int, error)`
- `usecases/navigation_back.go` — `AddDelta` с `(int, error)`
- `usecases/state_rollback.go` — `AddDelta` с `(int, error)`
- `logger/logger.go` — `LLMUsageWithCache`
- `handlers/handler_debug.go` — cache метрики
- `handlers/handler_pipeline.go` — `cacheHitRate()`
- `cmd/server/main.go` — `NewAgent2ExecuteUseCase` с logger

### New Files
- `adapters/anthropic/cache_types.go` — cache request/response types
- `tools/mock_tools.go` — padding tools + `CachePaddingEnabled`
- `usecases/cache_test.go` — integration test

## Test Results (First Run)

```
Model: claude-haiku-4-5-20251001 | 10 queries | 1 session
Cache: not activated (no beta header, below token threshold)
Input tokens/request: ~2509
Cost/request: ~$0.0028 | Total: $0.0112
4/10 passed, 6 failed on duplicate delta key (fixed in steps 11-13)
Conversation history: 0 (not persisted)
```

## Validation Commands

```bash
cd project/backend && go build ./...
cd project/backend && go test -v -run TestPromptCaching_Chain -timeout 300s ./internal/usecases/
```

## Acceptance Criteria

### Done
- [x] Cache padding tools добавлены
- [x] Mock tools НЕ вызываются при обычных запросах (prefix `_internal_`)
- [x] `LLMUsage` содержит `cache_creation_input_tokens` и `cache_read_input_tokens`
- [x] `ChatWithToolsCached` метод реализован в Anthropic adapter
- [x] System prompt передаётся как массив с `cache_control`
- [x] Tool definitions имеют `cache_control` на последнем элементе
- [x] Cost calculation учитывает кэш-токены (write ×1.25, read ×0.1)
- [x] Логи показывают cache hit rate
- [x] Debug page показывает Cache Write/Read/Hit Rate
- [x] `AddDelta` auto-increment step (порт + postgres адаптер)
- [x] `CachePaddingEnabled` флаг позволяет отключить mock tools

### TODO
- [ ] Zone-based StatePort: `UpdateData`, `UpdateTemplate`, `UpdateView`, `AppendConversation`
- [ ] Turn (DeltaInfo с turn_id) в Delta struct и zone-write методах
- [ ] Все акторы переведены на zone-write (нет blob `UpdateState` кроме rollback)
- [ ] search_products чистит стейт при empty через `UpdateData`
- [ ] Agent2 создаёт дельту на обоих путях через zone-write
- [ ] Инвариант «мутация = дельта» соблюдён всеми акторами
- [ ] ConversationHistory персистится в postgres
- [ ] Beta header `anthropic-beta: prompt-caching-2024-07-31`
- [ ] Padding tools дают 4096+ токенов
- [ ] `cache_read_input_tokens > 0` при повторных запросах
- [ ] Conversation history кэшируется при последующих запросах

## Expertise Context

- **backend-adapters**: Anthropic client, `ChatWithToolsCached`. Postgres state adapter — zone-write рефакторинг.
- **backend-ports**: `LLMPort` + `CacheConfig`. `StatePort` → zone-based methods + `DeltaInfo`.
- **backend-domain**: `Delta` + `turn_id`. `SessionState` зоны.
- **backend-pipeline**: Agent1/Agent2 переведены на `ChatWithToolsCached`. Переход на zone-write.
