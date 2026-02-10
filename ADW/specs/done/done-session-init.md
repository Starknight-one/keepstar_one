# Done: Session Init + Tenant Seed

**Дата:** 2026-02-10 19:30
**Ветка:** `feat/session-init`
**Статус:** Реализовано

## Проблема

Тенант определялся при первом запросе в pipeline, но state создавался без тенанта → tool fallback на "nike" при каждом pipeline вызове. Не было greeting при открытии чата — пустой экран до первого сообщения пользователя.

## Что сделано

### Step 1: Backend — HandleInitSession
**Файл:** `project/backend/internal/handlers/handler_session.go`
- `SessionHandler` расширен полем `statePort ports.StatePort`
- `NewSessionHandler(cache, statePort)` — обновлённый конструктор
- `HandleInitSession` (POST /api/v1/session/init):
  - Резолвит tenant из контекста (middleware)
  - Генерит sessionID (uuid)
  - Создаёт state через `statePort.CreateState()`
  - Сидит `tenant_slug` в `state.Current.Meta.Aliases`
  - Сохраняет state через `statePort.UpdateState()`
  - Создаёт session record через `cache.SaveSession()`
  - Возвращает `{ sessionId, tenant: { slug, name }, greeting }`
- Response types: `InitSessionResponse`, `InitTenantResponse`

### Step 2: Backend — роут с tenant middleware
**Файл:** `project/backend/internal/handlers/routes.go`
- `/api/v1/session/init` → `tenantMw.ResolveFromHeader(defaultTenant)(HandleInitSession)`
- Зарегистрирован перед `/api/v1/session/` (Go ServeMux longest-prefix match)

### Step 3: Backend — main.go wiring
**Файл:** `project/backend/cmd/server/main.go`
- `NewSessionHandler(cacheAdapter, stateAdapter)` — передан stateAdapter

### Step 4: Backend — CORS fix
**Файл:** `project/backend/internal/handlers/middleware_cors.go`
- `Access-Control-Allow-Headers: Content-Type, X-Tenant-Slug`

### Step 5: Frontend — API client
**Файл:** `project/frontend/src/shared/api/apiClient.js`
- `initSession()` — POST /api/v1/session/init, возвращает `{ sessionId, tenant, greeting }`

### Step 6: Frontend — ChatPanel init
**Файл:** `project/frontend/src/features/chat/ChatPanel.jsx`
- На mount: если есть кэш → restore (как раньше). Если нет → `initSession()`
- Получает sessionId + greeting, показывает приветствие как assistant message
- Graceful fallback: если init упал — чат работает, сессия создастся при первом запросе

## Дубликации нет

Pipeline и Agent1 работают по паттерну get-or-create:
- `pipeline_execute.go:96-111` — `GetSession` → если нет → `SaveSession` (init уже создал — skip)
- `agent1_execute.go:90-93` — `GetState` → если нет → `CreateState` (init уже создал — skip)
- `agent1_execute.go:104` — `Aliases["tenant_slug"] = slug` — идемпотентно

## Изменённые файлы
| Файл | Изменение |
|------|-----------|
| `handlers/handler_session.go` | +statePort, +HandleInitSession, +response types |
| `handlers/routes.go` | +route /api/v1/session/init |
| `handlers/middleware_cors.go` | +X-Tenant-Slug header |
| `cmd/server/main.go` | NewSessionHandler(cache, statePort) |
| `frontend/src/shared/api/apiClient.js` | +initSession() |
| `frontend/src/features/chat/ChatPanel.jsx` | +initSession on mount, +greeting message |
