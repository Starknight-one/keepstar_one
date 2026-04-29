# Trace Visibility — Спецификация доработок

> Что нужно докачать в админке чтобы видеть ВСЁ что происходит в пайплайне.

## Текущее состояние (Alpha 0.4.2)

Что видно сейчас в trace inline:
- Agent1: timing, tokens, cache HIT/MISS, cost breakdown, system prompt, enriched query, tool I/O
- Agent2: timing, tokens, cache HIT/MISS, cost breakdown, system prompt, prompt sent, tool I/O, raw response
- State after Agent1 и Agent2: counts, fields, deltas, meta, view, viewStack len, actions count, template summary
- Formation: mode, widgets, full JSON
- Waterfall: тайминги всех фаз с русскими подписями

---

## Слепые зоны

### Группа A — Данные теряются, нужно захватывать в trace

Эти данные существуют только в момент выполнения пайплайна. Если не записать в `trace_data` JSONB — они пропадают.

#### A1. Microcontext (Agent1 → Agent2 сигнал)

**Что это:** Строка-сигнал вроде `"new_search: 23 items found"`, `"filtered: 5 items"`, `"no_data_change"`. Строится `buildMicrocontext()` в `pipeline_execute.go` и передаётся Agent2.

**Почему важно:** Это мост между агентами. Без него непонятно какой контекст получил Agent2 для принятия решений.

**Что делать:**
- Добавить `Microcontext string` в `PipelineTrace`
- Сохранять в `pipeline_execute.go` после `buildMicrocontext()`
- Показывать в TraceInline между секциями Agent1 и Agent2

**Файлы:** `domain/trace_entity.go`, `usecases/pipeline_execute.go`, `TraceInline.jsx`

---

#### A2. ScreenContext (состояние экрана от фронтенда)

**Что это:** Фронтенд отправляет `{mode, widgetCount, fields}` — что сейчас видит пользователь. Влияет на решения Agent2.

**Почему важно:** Agent2 может решить "не менять layout" потому что на экране уже grid с 12 виджетами. Без этого контекста решение непонятно.

**Что делать:**
- Добавить `ScreenContext *ScreenContext` в `PipelineTrace`
- Сохранять из `req.ScreenContext` в `pipeline_execute.go`
- Показывать в TraceInline (компактно: "Экран: grid / 12 widgets / name,price,brand")

**Файлы:** `domain/trace_entity.go`, `usecases/pipeline_execute.go`, `TraceInline.jsx`

---

#### A3. Agent1 Raw Reasoning (текст LLM до tool_use)

**Что это:** Когда Agent1 решает вызвать инструмент, LLM сначала "думает" текстом, потом делает tool_use. Этот текст (`llmResp.Text`) сейчас только логируется.

**Почему важно:** Если Agent1 не вызвал tool или вызвал не тот — единственный способ понять почему.

**Что делать:**
- Добавить `RawReasoning string` в `AgentTrace`
- В `agent1_execute.go` сохранять `llmResp.Text` в response
- В `pipeline_execute.go` записывать в `trace.Agent1.RawReasoning`
- Показывать в TraceInline как схлопнутый JsonBlock "Agent1 Reasoning"

**Файлы:** `domain/trace_entity.go`, `usecases/agent1_execute.go`, `usecases/pipeline_execute.go`, `TraceInline.jsx`

---

#### A4. Navigation Traces (expand/back)

**Что это:** Хендлеры `HandleExpand` и `HandleBack` создают дельты и формации, но НЕ создают `pipeline_traces`. Навигация полностью невидима.

**Почему важно:** Expand — это переход к детальному просмотру. Back — возврат. Это ключевые взаимодействия, и их результат (какая формация сгенерировалась) нигде не записан.

**Что делать:**
- Создавать lightweight trace для expand/back в `handler_navigation.go`
- Структура: `{type: "navigation", action: "expand"/"back", entityRef, formation, totalMs, turnId}`
- Или расширить `pipeline_traces` с полем `traceType` ("pipeline" | "navigation" | "action")
- В админке показывать в timeline как отдельный тип записи (не "trace" и не "user action", а "navigation")

**Файлы:** `handlers/handler_navigation.go`, `domain/trace_entity.go`, `adapters/postgres/postgres_trace.go`, `TraceInline.jsx`, `SessionDetail.jsx`

**Примечание:** Это самое крупное изменение. Требует продумывания — писать в ту же таблицу `pipeline_traces` или в отдельную.

---

### Группа B — Данные есть в БД, нужно тянуть в админку

Эти данные хранятся в `chat_session_state` и `chat_session_deltas`. Нужны новые запросы в admin backend и отображение в UI.

#### B1. Conversation History (история диалога)

**Что это:** Полная история сообщений LLM: user queries + assistant reasoning + tool results. Хранится в `chat_session_state.conversation_history` (JSONB).

**Почему важно:** Без неё не видно что LLM "помнит" о предыдущих ходах. Это critical для multi-turn.

**Что делать:**
- Добавить метод в admin `TraceAdapter`: `GetSessionState(ctx, sessionID)` → тянет `conversation_history`, `agent2_history` из `chat_session_state`
- Добавить эндпоинт `/admin/api/sessions/{id}/state` → `{conversationHistory, agent2History}`
- В SessionDetail добавить схлопнутую секцию "Conversation History" с полным списком сообщений
- Рендер: role (user/assistant) + content (текст или tool_use JSON)

**Файлы:** `postgres_trace.go`, `handler_traces.go`, `main.go`, `SessionDetail.jsx`

---

#### B2. Full State Data (содержимое стейта)

**Что это:** Реальные продукты/сервисы в стейте — имена, цены, бренды, рейтинги. Хранится в `chat_session_state.current_data` (JSONB).

**Почему важно:** Сейчас видим "23 продукта, поля: name, price, brand". Но не видим КАКИЕ продукты. Agent2 рендерит данные которые мы не видим.

**Что делать:**
- В тот же эндпоинт `/admin/api/sessions/{id}/state` включить `current_data`
- В SessionDetail добавить секцию "State Data" — таблица продуктов (id, name, price, brand...)
- Схлопнутая по дефолту, разворачивается в компактную таблицу

**Файлы:** `postgres_trace.go`, `handler_traces.go`, `SessionDetail.jsx`

---

#### B3. ViewStack (навигационный стек)

**Что это:** Стек навигации: на каком экране был пользователь до expand. Хранится в `chat_session_state.view_stack` (JSONB).

**Почему важно:** Показывает навигационный путь пользователя: "список → деталь продукта A → назад → деталь продукта B".

**Что делать:**
- Включить в тот же эндпоинт `view_stack`
- Показать как breadcrumb или компактный список уровней (mode + focused entity)

**Файлы:** `postgres_trace.go`, `handler_traces.go`, `SessionDetail.jsx`

---

#### B4. Agent2 History (multi-turn рендеринг)

**Что это:** Последние 2 хода Agent2 (tool calls + results). Хранится в `chat_session_state.agent2_history` (JSONB).

**Почему важно:** Agent2 использует предыдущие ходы чтобы не повторять layout. Без этого непонятно почему он выбрал другой preset.

**Что делать:**
- Уже включён в B1 (тот же эндпоинт)
- Рендер: список tool_use → tool_result пар

**Файлы:** те же что B1

---

#### B5. Full Deltas (action + result JSONB)

**Что это:** Каждая дельта в `chat_session_deltas` содержит полный `action` (JSONB с типом, tool, params) и `result` (JSONB с count, fields). Сейчас в трейсе хранится только compact `DeltaTrace` без полных JSON.

**Почему важно:** Delta action содержит полные параметры инструмента и результат. Это audit trail.

**Что делать:**
- В `ListUserActions` уже тянется `action` JSONB — расширить чтобы тянуть `result` и `template` тоже
- Для LLM-дельт (source='llm') тоже тянуть и показывать в timeline
- Показывать в SessionDetail при раскрытии дельты полный JSON

**Файлы:** `postgres_trace.go`, `SessionDetail.jsx`, `UserActionItem` struct

---

### Группа C — Улучшения отображения

#### C1. Tool Definitions (какие инструменты доступны)

**Что это:** Список tool definitions отправляемых каждому агенту. Сейчас видим только `toolDefCount`.

**Что делать:**
- Добавить `ToolDefs []string` в `AgentTrace` (только имена, не полные схемы — они большие)
- В agent1/agent2 execute записать имена tools
- Показать как список тегов в секции агента

---

#### C2. Enriched Query читаемость

**Что это:** `enrichedQuery` для Agent1 — JSON с контекстом стейта. Сейчас показывается как raw JSON.

**Что делать:**
- Парсить JSON и показывать структурированно: count, fields, current_display, liked, cart
- Сделать компонент `EnrichedQueryView` вместо plain JsonBlock

---

## Приоритеты реализации

### P0 — Без этого не debuggable
1. **A1** Microcontext (5 мин, 3 строки бэкенд + 1 компонент фронт)
2. **A2** ScreenContext (5 мин, аналогично)
3. **A3** Agent1 raw reasoning (10 мин)
4. **B1** Conversation history (30 мин, новый эндпоинт + UI)

### P1 — Сильно помогает
5. **B2** Full state data (20 мин, тот же эндпоинт + таблица)
6. **A4** Navigation traces (1-2ч, требует архитектурного решения)
7. **B5** Full deltas (15 мин)

### P2 — Nice to have
8. **B3** ViewStack (10 мин)
9. **B4** Agent2 history (уже в B1)
10. **C1** Tool definitions (10 мин)
11. **C2** Enriched query view (15 мин)

---

## Архитектурная заметка

Группа A (захват в trace) требует изменений в **main backend** (`project/backend/`) — это пайплайн, который выполняется на chat backend порт 8080.

Группа B (pull из БД) требует изменений только в **admin backend** (`project_admin/backend/`) — новые запросы к тем же таблицам.

Оба бэкенда подключены к одной БД (Neon PostgreSQL), поэтому admin может читать всё что chat backend записал.
