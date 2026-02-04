# Feature: ADW-z8v4q1w Zone-based State Management

## Feature Description

Рефакторинг StatePort: замена единого `UpdateState(blob)` на 4 зонированных метода записи с автоматическим созданием дельт. Каждая мутация стейта гарантированно записывается как дельта — инвариант обеспечивается на уровне API порта. Дельты группируются в Turn'ы через `DeltaInfo`.

Связан с [ADW-k7x9m2p](feature-ADW-k7x9m2p-anthropic-prompt-caching.md) — шаги 14-18 из родительской спеки.

## Objective

1. **Инвариант «мутация = дельта»** — невозможно обновить стейт без записи дельты (обеспечено отсутствием blob-метода для обычных акторов)
2. **Зонированные записи** — каждый актор пишет только свою зону, SQL обновляет только нужные колонки
3. **Turn-группировка** — дельты одного логического хода объединены через `turn_id`
4. **Фикс протухшего стейта** — search_products при empty очищает данные вместо игнорирования
5. **Фикс отсутствующих дельт** — Agent2/render tools создают дельты через zone-write

## Expertise Context

Expertise used:
- **backend-ports**: StatePort interface — текущие методы `UpdateState`, `AddDelta`, `PushView`/`PopView`. Добавляются 4 zone-write метода + `DeltaInfo`
- **backend-domain**: Delta struct — добавляется `TurnID`. SessionState зоны: data, template, view, conversation
- **backend-adapters**: Postgres StateAdapter — `UpdateState` обновляет все колонки одним UPDATE. Zone-write будет обновлять только нужные колонки + INSERT delta атомарно
- **backend-pipeline**: Agent1 создаёт дельту через `AddDelta`, Agent2 не создаёт дельту вообще. Tools (search, render) вызывают `UpdateState` без дельты
- **backend-usecases**: Expand/Back — единственные акторы с корректным `AddDelta` + `UpdateState`. Rollback — единственный легитимный пользователь blob `UpdateState`

## Relevant Files

### Existing Files (modify)

- `project/backend/internal/domain/state_entity.go` — добавить `TurnID` в Delta, добавить `DeltaInfo` struct
- `project/backend/internal/ports/state_port.go` — добавить 4 zone-write метода + `DeltaInfo` тип в порт
- `project/backend/internal/adapters/postgres/postgres_state.go` — реализовать zone-write методы (UPDATE зоны + INSERT delta атомарно)
- `project/backend/internal/tools/tool_search_products.go` — перевести на `UpdateData`, обработать empty (total=0)
- `project/backend/internal/tools/tool_render_preset.go` — перевести `RenderProductPresetTool` и `RenderServicePresetTool` на `UpdateTemplate`
- `project/backend/internal/usecases/agent1_execute.go` — перевести на `AppendConversation` вместо `UpdateState` для conversation history, передавать `DeltaInfo` (с TurnID)
- `project/backend/internal/usecases/agent2_execute.go` — создавать дельту через zone-write на обоих путях (render tool path + empty path)
- `project/backend/internal/usecases/pipeline_execute.go` — генерировать TurnID, передавать в Agent1/Agent2
- `project/backend/internal/usecases/navigation_expand.go` — перевести на `UpdateView` + `UpdateTemplate`
- `project/backend/internal/usecases/navigation_back.go` — перевести на `UpdateView` + `UpdateTemplate`
- `project/backend/internal/usecases/state_rollback.go` — оставить `UpdateState` (единственный легитимный blob-write)
- `project/backend/internal/handlers/handler_pipeline.go` — генерировать TurnID в handler, передавать в pipeline
- `project/backend/internal/handlers/handler_debug.go` — отображать turn_id в дельтах на debug page (если есть рендер дельт)

### New Files
- Нет (все изменения в существующих файлах)

## Step by Step Tasks

IMPORTANT: Execute strictly in order. Each step MUST compile (`go build ./...`).

### 1. DeltaInfo struct + TurnID в Delta (domain)

Файл: `project/backend/internal/domain/state_entity.go`

- Добавить поле `TurnID string` в struct `Delta` (строка 62, после `Step`):
  ```go
  TurnID    string      `json:"turn_id,omitempty"`
  ```
- Добавить новый struct `DeltaInfo` (после Delta):
  ```go
  type DeltaInfo struct {
      TurnID    string      `json:"turn_id"`
      Trigger   TriggerType `json:"trigger"`
      Source    DeltaSource `json:"source"`
      ActorID   string      `json:"actor_id"`
      DeltaType DeltaType   `json:"delta_type"`
      Path      string      `json:"path"`
      Action    Action      `json:"action"`
      Result    ResultMeta  `json:"result"`
  }
  ```
- Добавить helper метод `DeltaInfo.ToDelta()` → `*Delta`:
  ```go
  func (di DeltaInfo) ToDelta() *Delta {
      return &Delta{
          TurnID:    di.TurnID,
          Trigger:   di.Trigger,
          Source:    di.Source,
          ActorID:   di.ActorID,
          DeltaType: di.DeltaType,
          Path:      di.Path,
          Action:    di.Action,
          Result:    di.Result,
          CreatedAt: time.Now(),
      }
  }
  ```

**Проверка**: `go build ./...` — чисто компилируется, DeltaInfo нигде ещё не используется.

### 2. Zone-write методы в StatePort (port)

Файл: `project/backend/internal/ports/state_port.go`

- Добавить `DeltaInfo` type alias (чтобы порт не импортировал domain напрямую для DeltaInfo — но в проекте порты уже импортируют domain, поэтому используем `domain.DeltaInfo` напрямую)
- Добавить 4 zone-write метода в интерфейс `StatePort`:
  ```go
  // Zone writes — update zone columns + create delta atomically
  UpdateData(ctx context.Context, sessionID string, data domain.StateData, meta domain.StateMeta, info domain.DeltaInfo) (int, error)
  UpdateTemplate(ctx context.Context, sessionID string, template map[string]interface{}, info domain.DeltaInfo) (int, error)
  UpdateView(ctx context.Context, sessionID string, view domain.ViewState, stack []domain.ViewSnapshot, info domain.DeltaInfo) (int, error)
  AppendConversation(ctx context.Context, sessionID string, messages []domain.LLMMessage) error
  ```
- НЕ удалять `UpdateState` — он остаётся для rollback

**Проверка**: `go build ./...` — ошибка компиляции: `StateAdapter` не реализует новые методы. Это ожидаемо, фиксим в шаге 3.

### 3. Zone-write реализация в Postgres adapter

Файл: `project/backend/internal/adapters/postgres/postgres_state.go`

Реализовать 4 метода. Каждый zone-write: UPDATE нужных колонок + INSERT delta + UPDATE step — всё в одной транзакции.

- **UpdateData**: обновляет `current_data`, `current_meta`, создаёт дельту с info, обновляет step
  ```go
  func (a *StateAdapter) UpdateData(ctx context.Context, sessionID string, data domain.StateData, meta domain.StateMeta, info domain.DeltaInfo) (int, error) {
      dataJSON, _ := json.Marshal(data)
      metaJSON, _ := json.Marshal(meta)
      delta := info.ToDelta()
      return a.zoneWriteWithDelta(ctx, sessionID, delta, `
          UPDATE chat_session_state
          SET current_data = $1, current_meta = $2, updated_at = NOW()
          WHERE session_id = $3
      `, dataJSON, metaJSON, sessionID)
  }
  ```

- **UpdateTemplate**: обновляет `current_template`, создаёт дельту
  ```go
  func (a *StateAdapter) UpdateTemplate(ctx context.Context, sessionID string, template map[string]interface{}, info domain.DeltaInfo) (int, error) {
      templateJSON, _ := json.Marshal(template)
      delta := info.ToDelta()
      return a.zoneWriteWithDelta(ctx, sessionID, delta, `
          UPDATE chat_session_state
          SET current_template = $1, updated_at = NOW()
          WHERE session_id = $2
      `, templateJSON, sessionID)
  }
  ```

- **UpdateView**: обновляет `view_mode`, `view_focused`, `view_stack`, создаёт дельту
  ```go
  func (a *StateAdapter) UpdateView(ctx context.Context, sessionID string, view domain.ViewState, stack []domain.ViewSnapshot, info domain.DeltaInfo) (int, error) {
      viewFocusedJSON, _ := json.Marshal(view.Focused)
      viewStackJSON, _ := json.Marshal(stack)
      delta := info.ToDelta()
      return a.zoneWriteWithDelta(ctx, sessionID, delta, `
          UPDATE chat_session_state
          SET view_mode = $1, view_focused = $2, view_stack = $3, updated_at = NOW()
          WHERE session_id = $4
      `, view.Mode, viewFocusedJSON, viewStackJSON, sessionID)
  }
  ```

- **AppendConversation**: обновляет `conversation_history`. Без дельты (append-only для LLM кэша).
  ```go
  func (a *StateAdapter) AppendConversation(ctx context.Context, sessionID string, messages []domain.LLMMessage) error {
      historyJSON, _ := json.Marshal(messages)
      _, err := a.client.pool.Exec(ctx, `
          UPDATE chat_session_state
          SET conversation_history = $1, updated_at = NOW()
          WHERE session_id = $2
      `, historyJSON, sessionID)
      return err
  }
  ```

- **zoneWriteWithDelta**: приватный хелпер — выполняет zone UPDATE + AddDelta в одном вызове. Переиспользует существующий `AddDelta` для вставки дельты (auto-increment step уже реализован):
  ```go
  func (a *StateAdapter) zoneWriteWithDelta(ctx context.Context, sessionID string, delta *domain.Delta, zoneSQL string, zoneArgs ...interface{}) (int, error) {
      // 1. Execute zone update
      _, err := a.client.pool.Exec(ctx, zoneSQL, zoneArgs...)
      if err != nil {
          return 0, fmt.Errorf("zone update: %w", err)
      }
      // 2. Add delta (step auto-assigned, state.step synced)
      step, err := a.AddDelta(ctx, sessionID, delta)
      if err != nil {
          return 0, fmt.Errorf("add delta: %w", err)
      }
      return step, nil
  }
  ```

**Важно**: Не использовать pgx транзакции на этом этапе — `AddDelta` уже атомарно обновляет step через CTE. Двухшаговый подход (zone update + AddDelta) достаточен для текущих нагрузок. Транзакции можно добавить позже если потребуется.

**Важно**: Нужно добавить колонку `turn_id` в таблицу `chat_session_deltas` и обновить `AddDelta` SQL, чтобы вставлять `delta.TurnID`. Добавить в INSERT:
- Колонку `turn_id` в списке INSERT
- Параметр `delta.TurnID` в значения

Миграция: добавить `ALTER TABLE chat_session_deltas ADD COLUMN IF NOT EXISTS turn_id TEXT;` в auto-migration или через ручной SQL.

Также обновить `scanDeltas` — читать `turn_id` из row и записывать в `d.TurnID`.

**Проверка**: `go build ./...` — чисто компилируется.

### 4. Перевод search_products на UpdateData

Файл: `project/backend/internal/tools/tool_search_products.go`

Текущее поведение:
- `total > 0` → обновляет `state.Current.Data.Products` и `state.Current.Meta`, вызывает `UpdateState(blob)`, дельта **не создаётся** (создаётся в agent1)
- `total == 0` → return "empty", **стейт не трогается** (протухшие данные остаются)

Новое поведение:
- Tool **не** вызывает zone-write напрямую. Он по-прежнему модифицирует `state` и вызывает `UpdateState`. Причина: DeltaInfo создаётся в Agent1, не в tool.
- **НО**: при `total == 0` — tool должен очистить стейт. Заменить ранний return на:
  ```go
  if total == 0 {
      // Clear stale data from previous search
      state.Current.Data = domain.StateData{}
      state.Current.Meta = domain.StateMeta{
          Count:   0,
          Fields:  []string{},
          Aliases: state.Current.Meta.Aliases, // preserve tenant_slug
      }
      if err := t.statePort.UpdateState(ctx, state); err != nil {
          return nil, fmt.Errorf("update state: %w", err)
      }
      return &domain.ToolResult{Content: "empty"}, nil
  }
  ```

**Переосмысление**: Tool вызывается внутри Agent1. Agent1 создаёт дельту после tool execution. Поэтому search_products остаётся на `UpdateState` — дельта создаётся выше в agent1. Зона `data` будет записана через `UpdateData` на уровне Agent1 в шаге 7.

**Однако**: промежуточно (до шага 7) можно оставить `UpdateState` в tool, главное — фикс empty path. Шаг 7 потом переведёт Agent1 целиком.

**Проверка**: `go build ./...` — чисто.

### 5. Перевод render_preset tools на UpdateTemplate

Файл: `project/backend/internal/tools/tool_render_preset.go`

Текущее поведение (обе tools — `RenderProductPresetTool` и `RenderServicePresetTool`):
- Читает state, строит formation, записывает `state.Current.Template`, вызывает `UpdateState(blob)`, дельта **не создаётся**

Новое поведение:
- Tools получают `DeltaInfo` через tool context. Для этого нужен механизм передачи DeltaInfo в tool.
- **Подход**: Так же как search_products — render tools вызываются из Agent2. Agent2 должен создавать дельту. Tool остаётся на `UpdateState` для template записи, а Agent2 (шаг 8) создаёт дельту через zone-write **или** через отдельный `AddDelta`.

**Альтернативный подход (проще)**: render tools остаются как есть (пишут template через `UpdateState`), Agent2 после tool execution вызывает `AddDelta` с DeltaInfo. Это минимальное изменение.

**Решение**: Оставить render tools на `UpdateState` для template. Дельту будет создавать Agent2 (шаг 8). Render tools вносят минимум изменений.

**Нет изменений в этом шаге.** Переходим к шагу 6.

### 5 (revised). TurnID в Pipeline + Handler

Файл: `project/backend/internal/usecases/pipeline_execute.go`

- Добавить поле `TurnID string` в `PipelineExecuteRequest`
- Генерировать TurnID если не передан:
  ```go
  turnID := req.TurnID
  if turnID == "" {
      turnID = uuid.New().String()
  }
  ```
- Передать TurnID в Agent1 и Agent2:
  ```go
  agent1Resp, err := uc.agent1UC.Execute(ctx, Agent1ExecuteRequest{
      SessionID:  req.SessionID,
      Query:      req.Query,
      TenantSlug: req.TenantSlug,
      TurnID:     turnID,
  })
  ...
  agent2Resp, err := uc.agent2UC.Execute(ctx, Agent2ExecuteRequest{
      SessionID: req.SessionID,
      TurnID:    turnID,
  })
  ```
- Добавить import `"github.com/google/uuid"` (уже используется в handler_pipeline.go)

Файл: `project/backend/internal/handlers/handler_pipeline.go`

- Генерировать TurnID в handler и передавать в pipeline:
  ```go
  turnID := uuid.New().String()
  result, err := h.pipelineUC.Execute(r.Context(), usecases.PipelineExecuteRequest{
      SessionID:  sessionID,
      Query:      req.Query,
      TenantSlug: tenantSlug,
      TurnID:     turnID,
  })
  ```

Файл: `project/backend/internal/handlers/handler_debug.go` (navigation handler)

- Для Expand/Back — генерировать TurnID в handler. Добавить в `ExpandRequest` и `BackRequest` поле `TurnID string`. Navigation handler генерирует `turnID = uuid.New().String()` и передаёт.

**Проверка**: `go build ./...` — ошибка: Agent1/Agent2 Request structs не имеют TurnID. Фиксим в шагах 6-7.

### 6. Agent1 — DeltaInfo + TurnID

Файл: `project/backend/internal/usecases/agent1_execute.go`

- Добавить `TurnID string` в `Agent1ExecuteRequest`
- Заменить ручное создание `delta` на создание через `DeltaInfo`:
  ```go
  info := domain.DeltaInfo{
      TurnID:    req.TurnID,
      Trigger:   domain.TriggerUserQuery,
      Source:    domain.SourceLLM,
      ActorID:   "agent1",
      DeltaType: domain.DeltaTypeAdd,
      Path:      "data.products",
      Action: domain.Action{
          Type:   domain.ActionSearch,
          Tool:   toolCall.Name,
          Params: toolCall.Input,
      },
      Result: domain.ResultMeta{
          Count:  state.Current.Meta.Count,
          Fields: state.Current.Meta.Fields,
      },
  }
  delta := info.ToDelta()
  ```
- `AddDelta` вызов остаётся тем же: `uc.statePort.AddDelta(ctx, req.SessionID, delta)`
- Заменить `UpdateState` для conversation history на `AppendConversation`:
  ```go
  // Было:
  state.ConversationHistory = append(state.ConversationHistory, ...)
  uc.statePort.UpdateState(ctx, state)

  // Стало:
  newMessages := append(state.ConversationHistory,
      domain.LLMMessage{Role: "user", Content: req.Query},
  )
  if len(llmResp.ToolCalls) > 0 {
      newMessages = append(newMessages,
          domain.LLMMessage{Role: "assistant", ToolCalls: llmResp.ToolCalls},
      )
  }
  uc.statePort.AppendConversation(ctx, req.SessionID, newMessages)
  ```
- **Важно**: Agent1 больше НЕ вызывает `UpdateState`. Tool (search_products) всё ещё вызывает `UpdateState` для data zone, но agent1 отвечает только за delta + conversation.

**Проверка**: `go build ./...`

### 7. Agent1 tools — перевод search_products на UpdateData (полный)

Файл: `project/backend/internal/tools/tool_search_products.go`

Полный перевод search_products на zone-write требует передачи `DeltaInfo` в tool. Но текущий tool interface:
```go
Execute(ctx context.Context, sessionID string, input map[string]interface{}) (*domain.ToolResult, error)
```
...не принимает DeltaInfo.

**Решение**: НЕ менять tool interface. Tool по-прежнему пишет данные через `UpdateState`. Дельта создаётся в Agent1 (шаг 6). Это сохраняет совместимость с tool registry и другими tools.

Единственное изменение в tool — фикс empty path (уже описан в шаге 4):
- При `total == 0` — очищать `state.Current.Data` и `state.Current.Meta` через `UpdateState`
- Сохранять `Aliases` (tenant_slug)

**Проверка**: `go build ./...`

### 8. Agent2 — дельта на обоих путях

Файл: `project/backend/internal/usecases/agent2_execute.go`

- Добавить `TurnID string` в `Agent2ExecuteRequest`
- После tool execution (render tool записал template в state), создать дельту:
  ```go
  // After tool execution — create delta for template zone
  templateInfo := domain.DeltaInfo{
      TurnID:    req.TurnID,
      Trigger:   domain.TriggerUserQuery,
      Source:    domain.SourceLLM,
      ActorID:   "agent2",
      DeltaType: domain.DeltaTypeUpdate,
      Path:      "template",
      Action: domain.Action{
          Type: domain.ActionLayout,
          Tool: response.ToolName,
      },
  }
  templateDelta := templateInfo.ToDelta()
  if _, err := uc.statePort.AddDelta(ctx, req.SessionID, templateDelta); err != nil {
      uc.log.Error("agent2_add_delta_failed", "error", err)
  }
  ```
- На empty path (no data), тоже создать дельту:
  ```go
  if state.Current.Meta.ProductCount == 0 && state.Current.Meta.ServiceCount == 0 {
      emptyInfo := domain.DeltaInfo{
          TurnID:    req.TurnID,
          Trigger:   domain.TriggerUserQuery,
          Source:    domain.SourceLLM,
          ActorID:   "agent2",
          DeltaType: domain.DeltaTypeUpdate,
          Path:      "template",
          Action: domain.Action{
              Type: domain.ActionLayout,
          },
          Result: domain.ResultMeta{Count: 0},
      }
      emptyDelta := emptyInfo.ToDelta()
      uc.statePort.AddDelta(ctx, req.SessionID, emptyDelta)

      return &Agent2ExecuteResponse{
          Template:  nil,
          LatencyMs: int(time.Since(start).Milliseconds()),
      }, nil
  }
  ```

**Проверка**: `go build ./...`

### 9. Navigation — Expand на zone-write

Файл: `project/backend/internal/usecases/navigation_expand.go`

- Добавить `TurnID string` в `ExpandRequest`
- Заменить ручной `AddDelta` + `UpdateState` на zone-write:
  ```go
  // Было:
  delta := &domain.Delta{...}
  uc.statePort.AddDelta(ctx, req.SessionID, delta)
  state.Current.Template = map[string]interface{}{"formation": formation}
  state.View.Mode = domain.ViewModeDetail
  state.View.Focused = &domain.EntityRef{...}
  uc.statePort.UpdateState(ctx, state)

  // Стало — два zone-write:
  // 1. UpdateView (view zone)
  viewInfo := domain.DeltaInfo{
      TurnID:    req.TurnID,
      Trigger:   domain.TriggerWidgetAction,
      Source:    domain.SourceUser,
      ActorID:   "user_expand",
      DeltaType: domain.DeltaTypePush,
      Path:      "view",
  }
  newView := domain.ViewState{
      Mode:    domain.ViewModeDetail,
      Focused: &domain.EntityRef{Type: req.EntityType, ID: req.EntityID},
  }
  stack, _ := uc.statePort.GetViewStack(ctx, req.SessionID)
  uc.statePort.UpdateView(ctx, req.SessionID, newView, stack, viewInfo)

  // 2. UpdateTemplate (template zone)
  templateInfo := domain.DeltaInfo{
      TurnID:    req.TurnID,
      Trigger:   domain.TriggerWidgetAction,
      Source:    domain.SourceUser,
      ActorID:   "user_expand",
      DeltaType: domain.DeltaTypeUpdate,
      Path:      "template",
  }
  template := map[string]interface{}{"formation": formation}
  uc.statePort.UpdateTemplate(ctx, req.SessionID, template, templateInfo)
  ```
- `PushView` вызов остаётся перед zone-write (snapshot записывается отдельно)
- Удалить вызов `UpdateState`
- Удалить ручной `AddDelta`

**Проверка**: `go build ./...`

### 10. Navigation — Back на zone-write

Файл: `project/backend/internal/usecases/navigation_back.go`

Аналогично Expand:
- Добавить `TurnID string` в BackRequest (через navigation handler)
- Заменить `AddDelta` + `UpdateState` на `UpdateView` + `UpdateTemplate`
- Удалить вызовы `AddDelta` и `UpdateState`

Файл: `project/backend/internal/handlers/` — navigation handler:
- Передавать `TurnID` из handler в Expand/Back requests

**Проверка**: `go build ./...`

### 11. Миграция: turn_id колонка

Файл: `project/backend/internal/adapters/postgres/postgres_state.go` (auto-migration секция, если есть)

Или выполнить SQL вручную:
```sql
ALTER TABLE chat_session_deltas ADD COLUMN IF NOT EXISTS turn_id TEXT;
```

Обновить AddDelta SQL — добавить `turn_id` в INSERT и передать `delta.TurnID` как параметр.

Обновить `scanDeltas` — читать `turn_id` из результата.

**Проверка**: `go build ./...` + миграция применена.

### 12. Validation

- `cd project/backend && go build ./...`
- `cd project/backend && go test ./...` (unit tests)
- Проверить что `UpdateState` вызывается **только** из:
  - `state_rollback.go` (легитимный blob-write)
  - `tool_search_products.go` (промежуточно — tool не имеет DeltaInfo context)
- Все остальные акторы используют zone-write или `AppendConversation`
- Grep: `UpdateState` — убедиться что нет вызовов из agent1, agent2, expand, back

## Validation Commands

```bash
cd project/backend && go build ./...
cd project/backend && go test ./...
```

## Acceptance Criteria

- [ ] `DeltaInfo` struct добавлен в domain с `TurnID`, `Trigger`, `Source`, `ActorID`, `DeltaType`, `Path`, `Action`, `Result`
- [ ] `Delta.TurnID` поле добавлено в Delta struct
- [ ] `DeltaInfo.ToDelta()` helper создаёт Delta из DeltaInfo
- [ ] StatePort расширен: `UpdateData`, `UpdateTemplate`, `UpdateView`, `AppendConversation`
- [ ] Postgres adapter реализует 4 zone-write метода
- [ ] Zone-write методы атомарно обновляют зону + создают дельту
- [ ] `turn_id` колонка добавлена в `chat_session_deltas`
- [ ] `AddDelta` SQL вставляет `turn_id`
- [ ] `scanDeltas` читает `turn_id`
- [ ] Pipeline генерирует TurnID и передаёт в Agent1/Agent2
- [ ] Navigation handler генерирует TurnID и передаёт в Expand/Back
- [ ] Agent1 использует `AppendConversation` вместо `UpdateState` для conversation history
- [ ] Agent1 создаёт delta через `DeltaInfo.ToDelta()` с TurnID
- [ ] Agent2 создаёт дельту на render path через `AddDelta`
- [ ] Agent2 создаёт дельту на empty path через `AddDelta`
- [ ] search_products при `total == 0` очищает `state.Current.Data` и `state.Current.Meta`
- [ ] search_products сохраняет `Aliases` (tenant_slug) при очистке meta
- [ ] Expand использует `UpdateView` + `UpdateTemplate` вместо `UpdateState`
- [ ] Back использует `UpdateView` + `UpdateTemplate` вместо `UpdateState`
- [ ] `UpdateState` вызывается только из rollback и search_products tool
- [ ] `go build ./...` проходит
- [ ] `go test ./...` проходит

## Notes

### Почему tool interface не меняется
Tool interface (`Execute(ctx, sessionID, input)`) не принимает DeltaInfo. Менять его = менять все tools + registry + agent execute. Это избыточно: дельты создаются на уровне usecases (Agent1/Agent2), не на уровне tools. Tools пишут данные, usecases пишут дельты.

### Почему UpdateState остаётся для search_products
search_products вызывается внутри Agent1 через tool registry. Agent1 создаёт дельту после tool execution. Tool не знает про DeltaInfo. Поэтому tool пишет data zone через `UpdateState`, а Agent1 создаёт дельту отдельно. Это промежуточное решение — в будущем можно передавать DeltaInfo через context.

### Почему нет pgx транзакций в zone-write
`AddDelta` уже атомарно инкрементирует step через CTE. Zone UPDATE + AddDelta = два SQL запроса без транзакции. Race condition маловероятен: один session обрабатывается одним pipeline последовательно. Транзакции добавим когда появятся параллельные агенты.

### Порядок zone-write в Expand/Back
Expand делает 2 zone-write: UpdateView + UpdateTemplate. Это 2 дельты с одним TurnID. Порядок: сначала view (навигация), потом template (контент). При rollback обе откатываются по TurnID.
