# Feature: ADW-p3k8v2m Patch — Session & State Flow

**ADW-ID**: ADW-p3k8v2m
**Type**: patch
**Priority**: high
**Complexity**: complex
**Layers**: backend, frontend
**Based on**: `ADW/specs/bugfix-session-state-flow.md` (BUG-002)

---

## Feature Description

Атомарный патч для session & state flow pipeline. Устраняет:
1. Потерю sessionId на фронте (stale closure)
2. Обход зонированной архитектуры тулами (UpdateState blob вместо zone-writes)
3. Отделение дельт от обновления стейта (нарушение инварианта "мутация = дельта")
4. Delta транзит через pipeline (бессмысленный прокид дельты, которая уже в БД)

Дополнительно:
- Pipeline упрощается — Delta вырезается из цепочки Agent1 → Pipeline → Handler
- Agent2 получает UserQuery + view + data delta для осмысленного выбора стиля
- freestyle тул добавляется в инструменты Agent2

Все фиксы идут вместе — частичный патч не имеет смысла: без ToolContext тулы не могут создавать дельты, без зонированных методов дельты бесполезны.

---

## Архитектурный контекст

Этот патч работает в рамках трёхслойной архитектуры с зонированным стейтом. Имплементатор **обязан** понимать эти принципы — иначе фиксы будут механическими заменами без понимания "зачем".

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

### Мгновенный UI + асинхронные дельты

**Слой 1 может менять экран моментально** — юзер или система меняют UI на фронте сразу. Дельты летят в БД асинхронно (убирает latency сети).

```
Юзер нажал 5 кнопок → получил апдейт → начал писать в чат
→ тем временем дельты долетают до БД
```

Это значит:
- Zone-write на бэкенде должен быть **атомарным** (дельта + обновление стейта в одной транзакции) — чтобы при асинхронной записи не потерять консистентность
- Фронтенд НЕ ждёт подтверждения дельты — он получает ответ pipeline и рендерит сразу
- Navigation (expand/back) уже работает по этому принципу — **эталонная реализация**

### 4 зоны стейта

| Зона | Назначение | Кто пишет | Метод |
|------|-----------|-----------|-------|
| **Data** | Продукты/сервисы | Слой 1 (тулы) | `UpdateData` |
| **Template** | Результат рендера от Agent2 | Слой 3 (render тулы) | `UpdateTemplate` |
| **View** | Что на экране (mode, focus, nav stack) | Слой 1 (navigation, clicks) | `UpdateView` |
| **Conversation** | LLM history (append-only) | Слой 1 | `AppendConversation` |

**Ключевой инвариант:** каждый актор пишет **только свою зону**. Тул search_products не имеет права трогать Template. Render тул не имеет права трогать Data. Нарушение этого инварианта — root cause бага с перезаписью data при пустом результате.

### Дельта = причина, не следствие

```
Действие актора → Дельта создаётся → Стейт обновляется
```

Дельта — это **причина** обновления стейта. Не лог "что случилось", а команда "что должно измениться".

Две цели:
1. **Контроль обновления** — разные части стейта обновляются разными дельтами, контролируемо
2. **LLM кэш** — передавать мелкую дельту в LLM вместо всего стейта → экономия токенов

### Что видит Agent2 при триггере

Agent2 получает три вещи:
1. **Текущий view** (полный) — что сейчас на экране
2. **Запрос пользователя** — что он хочет (для выбора стиля/пресета)
3. **Дельта по data** — что изменилось (не весь стейт, а именно что изменил Agent1)

Этого достаточно. История уже в LLM кэше.

### Pipeline — тупой клей, не оркестратор

Pipeline (`pipeline_execute.go`) — временный синхронный клей. Он не принимает решений, не хранит состояние, не содержит бизнес-логику. Его задача:
1. Создать сессию (FK constraint)
2. Сгенерировать TurnID
3. Вызвать Agent1
4. Вызвать Agent2 (Agent2 сам решает — данные есть → рендерит, нет → выходит)
5. Прочитать formation из стейта
6. Вернуть `{sessionId, formation, timing}` хендлеру

**Delta НЕ прокидывается** через pipeline. Дельта создаётся атомарно внутри тула через zone-write и сразу в БД. Pipeline не знает про дельты и не должен. Debug page когда будет переделан — будет читать дельты из БД через `GetDeltas(sessionID)`.

В будущем pipeline может стать event-driven (дельта как триггер Agent2). Пока — синхронный клей.

---

## Objective

1. **Frontend sessionId стабильность** — useRef вместо closure capture, follow-up запросы в одной сессии
2. **ToolContext с TurnID** — тулы получают контекст для создания дельт
3. **Тулы → zone-writes** — search_products → `UpdateData`, render_preset/freestyle → `UpdateTemplate`
4. **Инвариант "мутация = дельта"** — дельта создаётся атомарно с обновлением стейта внутри тула
5. **Убрать дублирующие AddDelta** — агенты больше не создают дельты за тулы
6. **Упростить pipeline** — убрать Delta из цепочки, прокинуть UserQuery в Agent2
7. **Agent2 freestyle** — freestyle тул доступен Agent2 через обновлённый фильтр

---

## Expertise Context

Expertise used:
- **backend-pipeline**: ToolExecutor interface — текущая сигнатура `Execute(ctx, sessionID, input)` → добавить `ToolContext`. Registry.Execute тоже менять. **getAgent2Tools()** фильтрует по `render_*` — freestyle не попадает (баг)
- **backend-ports**: StatePort — зонированные методы `UpdateData`, `UpdateTemplate`, `UpdateView` уже существуют, `DeltaInfo` уже есть. Тулы вызывают `UpdateState` вместо них
- **backend-domain**: `DeltaInfo` struct и `ToDelta()` уже реализованы. TurnID уже в Delta struct
- **backend-usecases**: Agent1 создаёт delta через `AddDelta` (строка 171-193), Agent2 тоже (строка 168-184). Pipeline генерирует TurnID, но не прокидывает в тулы. Pipeline прокидывает `agent1Resp.Delta` (строка 154, 161) — бессмысленный транзит. `Agent1ExecuteResponse` не имеет поля `ToolCalled` — только `ToolName string`
- **backend-handlers**: handler_pipeline.go генерирует TurnID (строка 79), хранит Delta в metrics
- **frontend-features**: useChatSubmit.js — sessionId через closure capture (строка 33), `useCallback` deps содержит sessionId (строка 67)

---

## Relevant Files

### Existing Files (modify)

**Backend — Tools:**
- `project/backend/internal/tools/tool_registry.go` — ToolExecutor interface + Registry.Execute: добавить ToolContext
- `project/backend/internal/tools/tool_search_products.go` — заменить UpdateState (строки 139, 155) на UpdateData zone-write, сохранить Aliases
- `project/backend/internal/tools/tool_render_preset.go` — заменить UpdateState (строки 86, 162) на UpdateTemplate zone-write
- `project/backend/internal/tools/tool_freestyle.go` — заменить UpdateState (строка 140) на UpdateTemplate zone-write

**Backend — Usecases:**
- `project/backend/internal/usecases/agent1_execute.go` — убрать AddDelta (строки 171-193), убрать Delta из response, передавать ToolContext
- `project/backend/internal/usecases/agent2_execute.go` — убрать AddDelta (строки 168-184), передавать ToolContext, добавить UserQuery, исправить getAgent2Tools фильтр, убрать LayoutHint
- `project/backend/internal/usecases/pipeline_execute.go` — убрать Delta из response, прокинуть UserQuery, убрать мёрж Delta+Template

**Backend — Prompts:**
- `project/backend/internal/prompts/prompt_compose_widgets.go` — новая сигнатура BuildAgent2ToolPrompt(meta, view, userQuery, dataDelta)

**Backend — Handlers:**
- `project/backend/internal/handlers/handler_pipeline.go` — убрать Delta из metrics (или обнулить)

**Frontend:**
- `project/frontend/src/features/chat/useChatSubmit.js` — sessionIdRef вместо closure capture

### Existing Files (verify only)
- `project/backend/internal/ports/state_port.go` — zone-write методы уже существуют ✅
- `project/backend/internal/domain/state_entity.go` — DeltaInfo, TurnID уже существуют ✅
- `project/backend/internal/usecases/navigation_expand.go` — эталон zone-write (не трогаем) ✅
- `project/backend/internal/usecases/navigation_back.go` — эталон zone-write (не трогаем) ✅

### New Files (создать)
- `project/backend/internal/tools/tool_search_products_test.go` — unit-тесты zone isolation
- `project/backend/internal/tools/tool_render_preset_test.go` — unit-тесты zone isolation

---

## Step by Step Tasks

IMPORTANT: Execute strictly in order. Each step MUST compile (`go build ./...`).

### 1. Frontend: sessionId ref (stale closure fix)

**File:** `project/frontend/src/features/chat/useChatSubmit.js`

Добавить `useRef` для sessionId чтобы callback не зависел от React re-render cycle:

```jsx
import { useCallback, useRef, useEffect } from 'react';

// Внутри хука:
const sessionIdRef = useRef(sessionId);
useEffect(() => { sessionIdRef.current = sessionId; }, [sessionId]);

const submit = useCallback(async (text) => {
  // Использовать sessionIdRef.current вместо sessionId
  const response = await sendPipelineQuery(sessionIdRef.current, text);
  if (response.sessionId && response.sessionId !== sessionIdRef.current) {
    sessionIdRef.current = response.sessionId;
    localStorage.setItem(SESSION_STORAGE_KEY, response.sessionId);
    setSessionId(response.sessionId);
  }
  // ... rest unchanged
}, []); // Стабильная ссылка, без sessionId в deps
```

**Что проверить:** `sessionId` больше не в deps массиве `useCallback` (строка 67). `sessionIdRef.current` используется во всех местах внутри submit. Другие deps (`addMessage`, `setLoading`, `setError`, `setSessionId`, `onFormationReceived`) — оставить или тоже вынести в ref.

---

### 2. Backend: ToolContext в tool_registry.go

**File:** `project/backend/internal/tools/tool_registry.go`

2.1. Добавить struct `ToolContext`:
```go
type ToolContext struct {
    SessionID string
    TurnID    string
    ActorID   string
}
```

2.2. Обновить интерфейс `ToolExecutor`:
```go
type ToolExecutor interface {
    Definition() domain.ToolDefinition
    Execute(ctx context.Context, toolCtx ToolContext, input map[string]interface{}) (*domain.ToolResult, error)
}
```

2.3. Обновить `Registry.Execute` (строка 69):
```go
func (r *Registry) Execute(ctx context.Context, toolCtx ToolContext, toolCall domain.ToolCall) (*domain.ToolResult, error) {
    // ...
    result, err := tool.Execute(ctx, toolCtx, toolCall.Input)
    // ...
}
```

**Compile check:** После этого шага будет много ошибок компиляции — все тулы и все вызовы Execute сломаются. Следующие шаги их чинят.

---

### 3. Backend: tool_search_products.go → UpdateData

**File:** `project/backend/internal/tools/tool_search_products.go`

3.1. Обновить сигнатуру Execute:
```go
func (t *SearchProductsTool) Execute(ctx context.Context, toolCtx ToolContext, input map[string]interface{}) (*domain.ToolResult, error)
```

3.2. Заменить `sessionID` на `toolCtx.SessionID` во всех вызовах (GetState строка 102, CreateState строка 104).

3.3. **Пустой результат (строка 131-142)** — НЕ трогаем Data zone, только дельта-маркер:

> **Примечание:** Здесь используется standalone `AddDelta` вместо zone-write `UpdateData`.
> Это осознанное исключение из инварианта "мутация = дельта атомарно": при пустом результате
> данные НЕ мутируются (предыдущие продукты сохраняются), поэтому атомарность не требуется.
> Дельта нужна только как маркер "поиск выполнен, результатов 0" для LLM кэша и debug page.

```go
if total == 0 {
    // Standalone AddDelta — данные не мутируются, атомарность не нужна
    info := domain.DeltaInfo{
        TurnID:    toolCtx.TurnID,
        Trigger:   domain.TriggerUserQuery,
        Source:    domain.SourceLLM,
        ActorID:   toolCtx.ActorID,
        DeltaType: domain.DeltaTypeAdd,
        Path:      "data.products",
        Action:    domain.Action{Type: domain.ActionSearch, Tool: "search_products", Params: input},
        Result:    domain.ResultMeta{Count: 0},
    }
    if _, err := t.statePort.AddDelta(ctx, toolCtx.SessionID, info.ToDelta()); err != nil {
        return nil, fmt.Errorf("add empty delta: %w", err)
    }
    return &domain.ToolResult{Content: "empty: 0 results, previous data preserved"}, nil
}
```

3.4. **Успешный результат (строки 148-157)** — UpdateData zone-write. **ВАЖНО: сохранить Aliases** (tenant_slug):
```go
fields := extractProductFields(products[0])

data := domain.StateData{Products: products}
meta := domain.StateMeta{
    Count:   total,
    Fields:  fields,
    Aliases: state.Current.Meta.Aliases, // СОХРАНЯЕМ tenant_slug!
}

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
if _, err := t.statePort.UpdateData(ctx, toolCtx.SessionID, data, meta, info); err != nil {
    return nil, fmt.Errorf("update data: %w", err)
}
```

**Два ключевых изменения:**
1. Пустой результат — Data zone НЕ перезаписывается, предыдущие продукты сохраняются
2. Успешный результат — `Aliases: state.Current.Meta.Aliases` сохраняет tenant_slug (без этого tenant теряется на втором запросе)

---

### 4. Backend: tool_render_preset.go → UpdateTemplate

**File:** `project/backend/internal/tools/tool_render_preset.go`

4.1. Обновить обе сигнатуры Execute:
```go
func (t *RenderProductPresetTool) Execute(ctx context.Context, toolCtx ToolContext, input map[string]interface{}) (*domain.ToolResult, error)
func (t *RenderServicePresetTool) Execute(ctx context.Context, toolCtx ToolContext, input map[string]interface{}) (*domain.ToolResult, error)
```

4.2. Заменить `sessionID` на `toolCtx.SessionID` в GetState вызовах.

4.3. RenderProductPresetTool — заменить `UpdateState` (строка 86) на `UpdateTemplate`:
```go
info := domain.DeltaInfo{
    TurnID:    toolCtx.TurnID,
    Trigger:   domain.TriggerUserQuery,
    Source:    domain.SourceLLM,
    ActorID:   toolCtx.ActorID,
    DeltaType: domain.DeltaTypeUpdate,
    Path:      "template",
    Action:    domain.Action{Type: domain.ActionLayout, Tool: "render_product_preset"},
}
if _, err := t.statePort.UpdateTemplate(ctx, toolCtx.SessionID, state.Current.Template, info); err != nil {
    return nil, fmt.Errorf("update template: %w", err)
}
```

4.4. RenderServicePresetTool — аналогично (строка 162), `Tool: "render_service_preset"`.

---

### 5. Backend: tool_freestyle.go → UpdateTemplate

**File:** `project/backend/internal/tools/tool_freestyle.go`

5.1. Обновить сигнатуру Execute:
```go
func (t *FreestyleTool) Execute(ctx context.Context, toolCtx ToolContext, input map[string]interface{}) (*domain.ToolResult, error)
```

5.2. Заменить `sessionID` на `toolCtx.SessionID` в GetState.

5.3. Заменить `UpdateState` (строка 140) на `UpdateTemplate`:
```go
info := domain.DeltaInfo{
    TurnID:    toolCtx.TurnID,
    Trigger:   domain.TriggerUserQuery,
    Source:    domain.SourceLLM,
    ActorID:   toolCtx.ActorID,
    DeltaType: domain.DeltaTypeUpdate,
    Path:      "template",
    Action:    domain.Action{Type: domain.ActionLayout, Tool: "freestyle"},
}
if _, err := t.statePort.UpdateTemplate(ctx, toolCtx.SessionID, state.Current.Template, info); err != nil {
    return nil, fmt.Errorf("update template: %w", err)
}
```

---

### 6. Backend: agent1_execute.go — убрать AddDelta и Delta из response, передать ToolContext

**File:** `project/backend/internal/usecases/agent1_execute.go`

6.1. **Убрать `Delta` из `Agent1ExecuteResponse`** (строка 26):
```go
type Agent1ExecuteResponse struct {
    // Delta     *domain.Delta  ← УБРАТЬ
    Usage     domain.LLMUsage
    LatencyMs int
    LLMCallMs      int64
    ToolExecuteMs  int64
    ToolName       string
    ToolInput      string
    ToolResult     string
    ProductsFound  int
    StopReason     string
}
```

6.2. Заменить вызов `Registry.Execute` (строка 155):
```go
result, err := uc.toolRegistry.Execute(ctx, tools.ToolContext{
    SessionID: req.SessionID,
    TurnID:    req.TurnID,
    ActorID:   "agent1",
}, toolCall)
```

6.3. **Удалить** блок создания дельты (строки 170-193):
```go
// УДАЛИТЬ ВСЁ ОТ:
// Create delta via DeltaInfo with TurnID
info := domain.DeltaInfo{...}
delta = info.ToDelta()
if _, err := uc.statePort.AddDelta(...)
// ДО конца if блока
```

6.4. Обновить productsFound — после тула читаем meta из стейта (строка 167-168, оставить как есть).

6.5. Убрать `Delta: delta` из return (строка 236).

6.6. `AppendConversation` (строка 219) — **оставить как есть**, это правильный zone-write.

---

### 7. Backend: agent2_execute.go — ToolContext, UserQuery, freestyle, убрать AddDelta

**File:** `project/backend/internal/usecases/agent2_execute.go`

7.1. Обновить request struct — **заменить LayoutHint на UserQuery**:
```go
type Agent2ExecuteRequest struct {
    SessionID string
    TurnID    string
    UserQuery string  // запрос пользователя (вместо LayoutHint)
}
```

7.2. Заменить вызов `Registry.Execute` (строка 154):
```go
result, err := uc.toolRegistry.Execute(ctx, tools.ToolContext{
    SessionID: req.SessionID,
    TurnID:    req.TurnID,
    ActorID:   "agent2",
}, toolCall)
```

7.3. **Удалить** блок создания дельты (строки 166-184) — render тулы сами создают дельты через zone-write.

7.4. **Удалить** блок создания дельты на empty path (строки 79-93) — Agent2 просто возвращает пустой response.

7.5. **Исправить `getAgent2Tools()`** — добавить freestyle (строка 211):
```go
func (uc *Agent2ExecuteUseCase) getAgent2Tools() []domain.ToolDefinition {
    allTools := uc.toolRegistry.GetDefinitions()
    var agent2Tools []domain.ToolDefinition
    for _, t := range allTools {
        if strings.HasPrefix(t.Name, "render_") || t.Name == "freestyle" {
            agent2Tools = append(agent2Tools, t)
        }
    }
    return agent2Tools
}
```

7.6. Получить data delta для Agent2:
> **TODO:** `GetDeltasSince(ctx, sessionID, 0)` тянет ВСЕ дельты сессии с начала. Для MVP допустимо,
> но при росте числа turns станет неэффективно. Добавить `GetDeltasByTurnID(ctx, sessionID, turnID)` в StatePort.

```go
// Получить последнюю дельту текущего turn'а по data zone
// TODO: заменить на GetDeltasByTurnID когда появится
var dataDelta *domain.Delta
if req.TurnID != "" {
    deltas, _ := uc.statePort.GetDeltasSince(ctx, req.SessionID, 0)
    for i := len(deltas) - 1; i >= 0; i-- {
        if deltas[i].TurnID == req.TurnID && strings.HasPrefix(deltas[i].Path, "data.") {
            dataDelta = &deltas[i]
            break
        }
    }
}
```

7.7. Обновить prompt building (строка 102) — заменить `BuildAgent2ToolPrompt(state.Current.Meta, req.LayoutHint)` на:
```go
userPrompt := prompts.BuildAgent2ToolPrompt(state.Current.Meta, state.View, req.UserQuery, dataDelta)
```

---

### 8. Backend: pipeline_execute.go — упростить, убрать Delta

**File:** `project/backend/internal/usecases/pipeline_execute.go`

8.1. **Убрать `Delta` из `PipelineExecuteResponse`** (строка 27):
```go
type PipelineExecuteResponse struct {
    Formation   *domain.FormationWithData
    // Delta       *domain.Delta  ← УБРАТЬ
    Agent1Ms    int
    Agent2Ms    int
    TotalMs     int
    Agent1Usage domain.LLMUsage
    Agent2Usage domain.LLMUsage
    // ... остальные поля оставить
}
```

8.2. Agent2 вызывается всегда, **добавляем** UserQuery (строки 116-122).
> **Примечание:** LayoutHint был объявлен в Agent2ExecuteRequest, но pipeline его никогда не передавал. Поэтому это не "замена", а добавление нового поля.

```go
// Agent2 всегда вызывается — он сам решает: данные есть → рендерит, нет → выходит
agent2Resp, err := uc.agent2UC.Execute(ctx, Agent2ExecuteRequest{
    SessionID: req.SessionID,
    TurnID:    turnID,
    UserQuery: req.Query,
})
if err != nil {
    return nil, fmt.Errorf("agent 2: %w", err)
}
```

8.3. **Удалить** мёрж Delta + Template (строки 153-157):
```go
// УДАЛИТЬ:
// if agent1Resp.Delta != nil && (agent2Resp.Template != nil || agent2Resp.Formation != nil) {
//     agent1Resp.Delta.Template = state.Current.Template
// }
```

8.4. **Убрать** `Delta: agent1Resp.Delta` из return (строка 161).

---

### 9. Backend: prompt_compose_widgets.go — view + UserQuery + data delta

**File:** `project/backend/internal/prompts/prompt_compose_widgets.go`

9.1. **Заменить** `BuildAgent2ToolPrompt` (строка 163) — новая сигнатура вместо старой:
```go
// BuildAgent2ToolPrompt builds the user message for Agent 2 with view context and user intent
func BuildAgent2ToolPrompt(meta domain.StateMeta, view domain.ViewState, userQuery string, dataDelta *domain.Delta) string {
    input := map[string]interface{}{
        "productCount": meta.ProductCount,
        "serviceCount": meta.ServiceCount,
        "fields":       meta.Fields,
    }

    // View context
    input["view_mode"] = string(view.Mode)
    if view.Focused != nil {
        input["focused"] = view.Focused
    }

    // User intent
    if userQuery != "" {
        input["user_request"] = userQuery
    }

    // Data change summary
    if dataDelta != nil {
        input["data_change"] = map[string]interface{}{
            "tool":   dataDelta.Action.Tool,
            "count":  dataDelta.Result.Count,
            "fields": dataDelta.Result.Fields,
        }
    }

    jsonBytes, _ := json.Marshal(input)
    return fmt.Sprintf("Render the data using appropriate tool:\n%s", string(jsonBytes))
}
```

9.2. Старая `BuildAgent2Prompt(meta, layoutHint)` (строка 76) — **оставить** для legacy, не ломать.

---

### 10. Backend: handler_pipeline.go — verify compile after Delta removal

**File:** `project/backend/internal/handlers/handler_pipeline.go`

10.1. После удаления `Delta` из `PipelineExecuteResponse` — убедиться что handler компилируется. Handler **не обращается** к `result.Delta` напрямую (metrics используют Formation, ToolCalled, Usage — но не Delta). Поэтому шаг сводится к проверке: `go build ./...` проходит без ошибок в handler_pipeline.go.

---

### 11. Backend: Создать тесты для тулов

**ВАЖНО: Тестов для тулов НЕ существует** — `project/backend/internal/tools/*_test.go` пусто. Нужно создать с нуля.

**File:** `project/backend/internal/tools/tool_search_products_test.go` — СОЗДАТЬ:
- Тест: UpdateData вызывается при успешном поиске (не UpdateState)
- Тест: пустой результат НЕ перезаписывает Data zone, AddDelta вызывается с count:0
- Тест: Aliases сохраняются при успешном поиске
- Тест: ToolContext.TurnID попадает в DeltaInfo

**File:** `project/backend/internal/tools/tool_render_preset_test.go` — СОЗДАТЬ:
- Тест: UpdateTemplate вызывается (не UpdateState)
- Тест: Data zone не затрагивается при рендере

### 12. Backend: Обновить существующие тесты

**Files:**
- `project/backend/internal/usecases/agent1_execute_test.go` — убрать обращения к `agent1Resp.Delta` (строки 127-128, 145-150, 169-171, 444). Delta больше нет в response. **Также добавить TurnID в Agent1ExecuteRequest** во всех тестовых вызовах — текущие тесты передают только {SessionID, Query}, без TurnID. Без этого TurnID propagation не тестируется.
- `project/backend/internal/usecases/agent2_execute_test.go` — обновить `Agent2ExecuteRequest` (убрать LayoutHint, добавить UserQuery). Также добавить TurnID в тестовые вызовы.
- `project/backend/internal/usecases/navigation_test.go` — проверить что не использует ToolExecutor mock'и (если нет — не трогать)

---

### 13. Validation

```bash
cd project/backend && go build ./...
cd project/backend && go test ./...
cd project/frontend && npm run build
cd project/frontend && npm run lint
```

---

## Validation Commands

```bash
# Backend build (required)
cd project/backend && go build ./...

# Backend tests
cd project/backend && go test ./...

# Frontend build (required)
cd project/frontend && npm run build

# Frontend lint
cd project/frontend && npm run lint
```

---

## Acceptance Criteria

### Frontend
- [ ] `useChatSubmit.js` использует `useRef` для sessionId
- [ ] `useCallback` не содержит sessionId в deps
- [ ] Follow-up запрос идёт с тем же sessionId

### Backend — ToolContext
- [ ] `ToolContext` struct создан в `tool_registry.go`
- [ ] `ToolExecutor.Execute` принимает `ToolContext` вместо `sessionID string`
- [ ] `Registry.Execute` принимает `ToolContext`

### Backend — Zone Writes
- [ ] `search_products` использует `UpdateData` (не `UpdateState`)
- [ ] `search_products` при пустом результате НЕ перезаписывает Data zone
- [ ] `search_products` сохраняет `Aliases` (tenant_slug) при успешном поиске
- [ ] `render_product_preset` использует `UpdateTemplate` (не `UpdateState`)
- [ ] `render_service_preset` использует `UpdateTemplate` (не `UpdateState`)
- [ ] `freestyle` использует `UpdateTemplate` (не `UpdateState`)

### Backend — Atomic Deltas
- [ ] Тулы создают дельты через zone-write (атомарно с обновлением стейта)
- [ ] `agent1_execute.go` НЕ содержит `AddDelta` вызов
- [ ] `agent2_execute.go` НЕ содержит `AddDelta` вызов
- [ ] `Agent1ExecuteResponse` НЕ содержит поле `Delta`

### Backend — Pipeline Simplification
- [ ] `PipelineExecuteResponse` НЕ содержит поле `Delta`
- [ ] Pipeline НЕ мёржит Delta + Template (строки 153-157 удалены)
- [ ] Pipeline прокидывает `UserQuery` в Agent2

### Backend — Agent2 Input & Tools
- [ ] Agent2 получает `UserQuery` в request (вместо LayoutHint)
- [ ] Agent2 получает data delta текущего turn'а
- [ ] `getAgent2Tools()` включает freestyle (`|| t.Name == "freestyle"`)
- [ ] `BuildAgent2ToolPrompt` принимает `(meta, view, userQuery, dataDelta)`
- [ ] Промпт Agent2 содержит view state, user request, data change

### Tests
- [ ] `tool_search_products_test.go` создан — zone isolation, empty preserves, Aliases
- [ ] `tool_render_preset_test.go` создан — zone isolation
- [ ] Существующие тесты обновлены — нет обращений к `agent1Resp.Delta`
- [ ] `go test ./...` проходит

### Integration
- [ ] `go build ./...` проходит
- [ ] `go test ./...` проходит
- [ ] `npm run build` проходит
- [ ] `npm run lint` проходит

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

Scenario: Aliases preserved on successful search
  Given state.Meta.Aliases has tenant_slug="nike"
  When search_products finds 5 products
  Then state.Meta.Aliases still has tenant_slug="nike"

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
  And delta.TurnID matches pipeline TurnID

Scenario: Agent2 can use freestyle tool
  Given state has 5 products
  And user asked "покажи с большими заголовками"
  When Agent2 receives tools
  Then freestyle tool is in the list
  And Agent2 can call freestyle(style="product-hero")

Scenario: Agent2 receives view + query + data delta
  Given Agent1 searched and found 7 products
  And current view mode is "grid"
  When Agent2 is triggered
  Then Agent2 prompt contains current view mode
  And Agent2 prompt contains user query
  And Agent2 prompt contains data delta summary (count:7, tool:search_products)

Scenario: Pipeline response has no Delta
  When full pipeline executes
  Then response contains formation and timing
  And response does NOT contain Delta field
```

---

## Notes

### Gotchas обнаруженные при верификации

1. **ToolExecutor mock'и** — все тесты, которые mock'ают ToolExecutor, сломаются при смене сигнатуры Execute. Нужно обновить mock'и первым делом после изменения интерфейса.

2. **Порядок фиксов критичен** — шаг 2 (ToolContext) ломает компиляцию, шаги 3-7 её чинят. Нельзя делать `go build` между шагами 2 и 7.

3. **search_products Aliases** — текущий код на success path создаёт новый `StateMeta` с `Aliases: make(map[string]string)` — это **затирает tenant_slug**. Это предсуществующий баг. Шаг 3.4 его чинит.

4. **getAgent2Tools() фильтр** — текущий `strings.HasPrefix(t.Name, "render_")` отсекает freestyle. Без фикса Agent2 не может вызвать freestyle. Шаг 7.5 это чинит.

5. **Тестов для тулов нет** — `project/backend/internal/tools/*_test.go` не существует. Шаг 11 создаёт их с нуля.

6. **agent1Resp.Delta в тестах** — integration тесты обращаются к `agent1Resp.Delta.Result.Count` (строки 127-128 agent2_execute_test.go). После удаления Delta из response — NPE. Шаг 12 чинит.

7. **handler_pipeline.go metrics** — хранит `result.ToolCalled` (строка 110) из `PipelineExecuteResponse.ToolCalled` — это поле остаётся (оно про имя тула, не про Delta). Delta-related поля в metrics обнуляются.

### Scope

- **НЕ в scope**: multi-tool future (когда Agent1 вызывает несколько тулов)
- **НЕ в scope**: freestyle vs preset investigation (отдельный тикет)
- **НЕ в scope**: event-driven Agent2 trigger (будущее расширение)
- **НЕ в scope**: debug page рефакторинг (отдельная задача)

---

## Estimate

| # | Файл | Что | ~Строк |
|---|------|-----|--------|
| 1 | `useChatSubmit.js` | sessionId ref | ~10 |
| 2 | `tool_registry.go` | ToolContext + Execute signature | ~15 |
| 3 | `tool_search_products.go` | UpdateData + empty fix + Aliases | ~40 |
| 4 | `tool_render_preset.go` | UpdateTemplate (2 тула) | ~30 |
| 5 | `tool_freestyle.go` | UpdateTemplate | ~15 |
| 6 | `agent1_execute.go` | Убрать AddDelta + Delta из response, ToolContext | ~-20 |
| 7 | `agent2_execute.go` | ToolContext, UserQuery, freestyle, убрать AddDelta | ~35 |
| 8 | `pipeline_execute.go` | Убрать Delta, прокинуть UserQuery | ~-15 |
| 9 | `prompt_compose_widgets.go` | Новая сигнатура с view/query/delta | ~25 |
| 10 | `handler_pipeline.go` | Убрать Delta из metrics | ~-5 |
| 11 | Новые тесты тулов | Создать с нуля | ~80 |
| 12 | Существующие тесты | Убрать Delta обращения | ~20 |
| | **Total** | | **~230** |

---

## Related Specs

- Bugfix analysis: `ADW/specs/bugfix-session-state-flow.md`
- Zone-based state: `ADW/specs/feature-ADW-z8v4q1w-zone-based-state-management.md`
- Delta management: `ADW/specs/feature-x7k9m2p-delta-state-management.md`
- Design system: `ADW/specs/feature-design-system-integration.md`
