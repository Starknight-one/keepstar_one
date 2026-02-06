# Bugfix: Session & State Flow

**ADW-ID**: BUG-002
**Priority**: high
**Layers**: backend, frontend

---

## Summary

Системные проблемы в pipeline обнаружены при follow-up запросах:
1. Frontend теряет sessionId (stale closure)
2. Тулы обходят зонированную архитектуру — пишут через `UpdateState` вместо зонированных методов
3. Дельты создаются отдельно от обновления стейта (инвариант "Действие → Дельта → Стейт" нарушен)
4. Триггер слоя 2 — Agent2 вызывается последовательно, без проверки нужности

---

## Архитектурный контекст

### Три слоя

```
Слой 1 — Обработчик запросов:
  Agent1, user clicks, system
  → обрабатывают input → пишут дельты → стейт обновляется

Слой 2 — Состояние (State + Delta engine):
  State с 4 зонами + Delta log
  → хранит, обновляется дельтами, является источником правды

Слой 3 — Обработчик ответов:
  Agent2+ → читают стейт → формируют что увидит юзер
  → триггерятся когда слой 1 (агент) завершил работу
```

В будущем агентов может стать больше с обеих сторон.

**Слой 1 может менять экран моментально** — юзер или система меняют UI на фронте сразу. Дельты летят в БД асинхронно (убирает latency сети). Юзер нажал 5 кнопок → получил апдейт → начал писать в чат → тем временем дельты долетают до БД.

### 4 зоны стейта

| Зона | Назначение | Кто пишет |
|------|-----------|-----------|
| **Data** | Продукты/сервисы. Может обновляться разными акторами (агенты, система, юзер) — как добавляя, так и уменьшая | Слой 1 через порт |
| **Template** | Результат рендера от Agent2. Одномоментный снимок "что Agent2 выдал" | Слой 3 |
| **View** | Что реально на экране (mode, focused entity, navigation stack). Меняется кликами юзера | Слой 1 (navigation, user actions) |
| **Conversation** | LLM history (append-only) | Слой 1 |

### Дельты

```
Действие актора → Дельта создаётся → Стейт обновляется
```

Дельта = **причина** обновления стейта, не следствие.

Две цели:
1. **Контроль обновления** — разные части стейта обновляются разными дельтами, контролируемо
2. **LLM кэш** — передавать мелкую дельту в LLM вместо всего стейта → экономия токенов

### Триггер слоя 3

Упрощённая модель (текущая):
- Agent2 триггерится **только** когда Agent1 (слой 1, именно агент) завершил работу
- User clicks, navigation — **не триггерят** Agent2
- В будущем: multi-tool флаг, event-driven расширение

### Что видит Agent2 при триггере

Agent2 получает:
1. **Текущий view** (полный) — что сейчас на экране
2. **Запрос пользователя** — что он хочет
3. **Дельта по data** — что изменилось

Этого достаточно. История уже в LLM кэше. Кейсы с просмотром старых дельт (e.g. "покажи тот кроссовок что я листал") — будущее.

### Зонированные методы (существуют, тулами не используются)

```go
UpdateData(ctx, sessionID, data, meta, deltaInfo)      // Data zone
UpdateTemplate(ctx, sessionID, template, deltaInfo)     // Template zone
UpdateView(ctx, sessionID, view, stack, deltaInfo)      // View zone
AppendConversation(ctx, sessionID, messages)            // Conversation (no delta)
```

Навигация уже использует их правильно. Тулы — нет.

---

## Observed Behavior

```
User: "Привет, покажи кроссовки Найк, большие заголовки и цены"
→ Session 5860f83d, search_products(brand:Nike) → 7 products ✅
→ Agent2 renders grid ✅

User: "А можешь показать только Найк пегасус?"
→ Session 24aed98f (НОВАЯ!) ← frontend потерял sessionId
→ search_products(query:"Nike Pegasus") → 7 products
→ Agent2 renders ✅ (но другая сессия, без истории)

User: "А покажи теперь кроссовки только в названием"
→ search_products(query:"кроссовки") → 0 results
→ UpdateState удалил ВСЕ данные (включая template zone)
→ Agent2: count=0, exit early
→ UI не обновляется
```

**Отдельная пометка:** При первом запросе юзер просил "большие заголовки и цены", но Agent2 использовал стандартный пресет (`display: "h2"`) вместо freestyle (`display: "h1"`). Возможные причины: freestyle tool не сработал / не был вызван, либо пресет недостаточно гибкий. Требует отдельного расследования.

---

## Root Causes

### 1. Frontend: stale closure на sessionId

**File:** `frontend/src/features/chat/useChatSubmit.js`

`submit` callback захватывает `sessionId` через closure. Между `setSessionId()` и следующим вызовом React может не пересоздать callback → второй запрос уходит с `sessionId = null` → бэкенд создаёт новую сессию.

### 2. Тулы обходят зонированную архитектуру

**Все 3 тула** используют `UpdateState` (полная перезапись всех зон):

| Тул | Вызовы UpdateState | Должен использовать |
|-----|-------------------|-------------------|
| `tool_search_products.go` | 2 (строки 139, 155) | `UpdateData` |
| `tool_render_preset.go` | 2 (строки 86, 162) | `UpdateTemplate` |
| `tool_freestyle.go` | 1 (строка 140) | `UpdateTemplate` |

**Навигация сделана правильно** — `UpdateView` + `UpdateTemplate`. Тесты навигации проверяют `UpdateStateCalls == 0`.

**Последствия:**
- `search_products` при пустом результате очищает Data zone → перезаписывает template zone тоже
- `render_product_preset` при записи template перезаписывает data zone
- Зоны не изолированы → каскадные поломки

### 3. Дельты создаются отдельно от обновления стейта

**Как задумано:**
```
Тул вызывает зонированный метод → дельта + обновление стейта атомарно
```

**Как реализовано:**
```
Тул пишет через UpdateState (без дельты)
→ Агент создаёт дельту через AddDelta (отдельно, потом)
```

Нарушает атомарность, инвариант "Mutation = Delta", и возможность использовать дельту как триггер.

### 4. Agent2 триггерится без проверки

Pipeline вызывает Agent2 всегда после Agent1. Нет проверки "нужен ли Agent2":
- Если Agent1 не изменил data → Agent2 может быть не нужен
- Если data пуста (из-за бага с UpdateState) → Agent2 выходит впустую

---

## Fixes

Все фиксы идут вместе — TurnID в тулы, зонированные методы, триггер. Частичный фикс не имеет смысла: без TurnID тулы не могут создавать дельты, без зонированных методов дельты бесполезны.

### Fix 1: sessionId ref (frontend)

**File:** `project/frontend/src/features/chat/useChatSubmit.js`

```jsx
const sessionIdRef = useRef(sessionId);
useEffect(() => { sessionIdRef.current = sessionId; }, [sessionId]);

const submit = useCallback(async (text) => {
  const response = await sendPipelineQuery(sessionIdRef.current, text);
  if (response.sessionId && response.sessionId !== sessionIdRef.current) {
    sessionIdRef.current = response.sessionId;
    localStorage.setItem(SESSION_STORAGE_KEY, response.sessionId);
    setSessionId(response.sessionId);
  }
  // ...
}, []);  // Стабильная ссылка, без sessionId в deps
```

### Fix 2: ToolContext с TurnID

**File:** `project/backend/internal/tools/tool_registry.go`

Тулы получают контекст для создания дельт:
```go
type ToolContext struct {
    SessionID string
    TurnID    string
    ActorID   string
}

// Execute сигнатура тулов меняется:
Execute(ctx context.Context, toolCtx ToolContext, input map[string]interface{}) (*domain.ToolResult, error)
```

Агенты передают ToolContext при вызове:
```go
result, err := uc.toolRegistry.Execute(ctx, tools.ToolContext{
    SessionID: req.SessionID,
    TurnID:    req.TurnID,
    ActorID:   "agent1",
}, toolCall)
```

### Fix 3: Тулы → зонированные методы + дельты

**search_products** (Data zone):
```go
// Успешный поиск:
info := domain.DeltaInfo{
    TurnID:    toolCtx.TurnID,
    Trigger:   domain.TriggerUserQuery,
    Source:    domain.SourceLLM,
    ActorID:   toolCtx.ActorID,
    DeltaType: domain.DeltaTypeAdd,
    Path:      "data.products",
    Action:    domain.Action{Type: domain.ActionSearch, Tool: "search_products", Params: input},
    Result:    domain.ResultMeta{Count: total, Fields: fields},
}
t.statePort.UpdateData(ctx, toolCtx.SessionID, data, meta, info)

// Пустой результат — НЕ трогаем Data zone:
info := domain.DeltaInfo{
    // ...
    Result: domain.ResultMeta{Count: 0},
}
t.statePort.AddDelta(ctx, toolCtx.SessionID, info.ToDelta())
return &domain.ToolResult{Content: "empty: 0 results, previous data preserved"}, nil
```

**render_product_preset / freestyle** (Template zone):
```go
info := domain.DeltaInfo{
    TurnID:    toolCtx.TurnID,
    Trigger:   domain.TriggerUserQuery,
    Source:    domain.SourceLLM,
    ActorID:   toolCtx.ActorID,
    DeltaType: domain.DeltaTypeUpdate,
    Path:      "template",
    Action:    domain.Action{Type: domain.ActionLayout, Tool: toolName},
}
t.statePort.UpdateTemplate(ctx, toolCtx.SessionID, template, info)
```

### Fix 4: Убрать дублирующие AddDelta из агентов

**Files:** `agent1_execute.go`, `agent2_execute.go`

Тулы сами создают дельты через зонированные методы. Убрать `AddDelta` вызовы из агентов. Агент получает только отбивку от тула ("ok: found 7 products").

### Fix 5: Триггер Agent2

**File:** `project/backend/internal/usecases/pipeline_execute.go`

Agent2 триггерится **только** когда Agent1 (агент, не навигация) завершил работу:

```go
// После Agent1
if agent1Resp.ToolCalled {
    // Agent1 вызвал тул → тул создал дельту → стейт обновился
    // Триггерим слой 3
    agent2Resp, err := uc.agent2UC.Execute(ctx, Agent2ExecuteRequest{
        SessionID: req.SessionID,
        TurnID:    turnID,
        UserQuery: req.Query,  // запрос пользователя для Agent2
    })
}
```

### Fix 6: Agent2 input — view + data delta + query

**File:** `project/backend/internal/usecases/agent2_execute.go`

Agent2 получает:
1. Текущий view (полный)
2. Data дельту (что изменилось)
3. Запрос пользователя

```go
type Agent2ExecuteRequest struct {
    SessionID string
    TurnID    string
    UserQuery string  // NEW: запрос пользователя
}

// В Execute:
state, _ := uc.statePort.GetState(ctx, req.SessionID)

// Текущий view
currentView := state.View

// Проверяем данные — берём из state (Data zone сохранена)
productCount := len(state.Current.Data.Products)
serviceCount := len(state.Current.Data.Services)

// Строим промпт с view + data summary + user query
userPrompt := prompts.BuildAgent2ToolPrompt(state.Current.Meta, currentView, req.UserQuery)
```

---

## Estimate

| # | Файл | Что | Строк |
|---|------|-----|-------|
| 1 | `useChatSubmit.js` | sessionId ref | ~10 |
| 2 | `tool_registry.go` | ToolContext + Execute signature | ~15 |
| 3 | `tool_search_products.go` | UpdateData + empty fix | ~35 |
| 4 | `tool_render_preset.go` | UpdateTemplate | ~30 |
| 5 | `tool_freestyle.go` | UpdateTemplate | ~15 |
| 6 | `agent1_execute.go` | Убрать AddDelta, прокинуть ToolContext | ~-15 |
| 7 | `agent2_execute.go` | Новый input (view + query), убрать AddDelta | ~30 |
| 8 | `pipeline_execute.go` | Триггер проверка + прокинуть query | ~10 |
| | **Total** | | **~150** |

Зонированная инфра (UpdateData, UpdateTemplate, zoneWriteWithDelta) уже существует и работает в навигации. Мы приводим тулы в соответствие.

---

## Files to Change

### Backend
- `project/backend/internal/tools/tool_registry.go` — ToolContext
- `project/backend/internal/tools/tool_search_products.go` — UpdateData
- `project/backend/internal/tools/tool_render_preset.go` — UpdateTemplate
- `project/backend/internal/tools/tool_freestyle.go` — UpdateTemplate
- `project/backend/internal/usecases/agent1_execute.go` — убрать AddDelta, ToolContext
- `project/backend/internal/usecases/agent2_execute.go` — новый input, убрать AddDelta
- `project/backend/internal/usecases/pipeline_execute.go` — триггер + query passthrough

### Frontend
- `project/frontend/src/features/chat/useChatSubmit.js` — sessionId ref

### Tests
- `project/backend/internal/tools/tool_search_products_test.go` — zone isolation, empty preserves
- `project/backend/internal/tools/tool_render_preset_test.go` — zone isolation
- `project/backend/internal/usecases/agent2_execute_test.go` — preserved data, new input
- Navigation tests — уже проверяют zone-writes ✅

---

## Test Scenarios

```gherkin
Scenario: Follow-up uses same session
  Given user sent "покажи кроссовки Nike" and got 7 results
  When user sends "покажи большими заголовками" within 2 seconds
  Then sessionId is the same
  And ConversationHistory contains both messages

Scenario: Empty search preserves Data zone
  Given state.Data has 7 products
  When search_products returns 0 results
  Then state.Data still has 7 products
  And delta created with count:0
  And Agent2 re-renders existing products

Scenario: Tool writes only its zone
  Given state has Data (7 products) and Template (grid formation)
  When search_products writes new data
  Then Template zone is unchanged
  When render_product_preset writes new template
  Then Data zone is unchanged

Scenario: Delta created atomically with zone write
  Given session exists
  When tool calls UpdateData with DeltaInfo
  Then delta is created in same transaction as data update
  And delta.Step matches state.Step

Scenario: Agent2 triggered only after Agent1
  Given user clicked expand (navigation)
  Then Agent2 is NOT triggered
  Given Agent1 finished with tool call
  Then Agent2 IS triggered

Scenario: Agent2 receives view + query + data delta
  Given Agent1 searched and found 7 products
  When Agent2 is triggered
  Then Agent2 receives current view state
  And Agent2 receives user query text
  And Agent2 receives data summary from state
```

---

## Open Questions

- **Freestyle vs Preset**: При запросе "большие заголовки" Agent2 использовал пресет вместо freestyle. Отдельное расследование — freestyle tool не вызван или пресет недостаточно гибкий?
- **Multi-tool future**: Когда Agent1 вызывает несколько тулов подряд — каждый создаёт дельту, триггер должен сработать после последнего. Нужен флаг / механизм. Пока один тул на вызов.

---

## Related Specs

- Zone-based state: `ADW/specs/feature-ADW-z8v4q1w-zone-based-state-management.md`
- Delta management: `ADW/specs/feature-x7k9m2p-delta-state-management.md`
- Design system: `ADW/specs/feature-design-system-integration.md`
- Domain model: `project/backend/internal/domain/README.md`
