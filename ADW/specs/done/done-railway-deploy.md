# Done: Railway Deploy — Chat + Admin

**Дата:** 2026-02-11 00:30
**Ветка:** `main` (прямой пуш)
**Статус:** Реализовано и задеплоено

## Проблема

Railway не мог задеплоить проект:
1. Нет Dockerfile / railway.toml — Nixpacks не понимает монорепо (Go + Node в подпапках)
2. Go бэкенды — чисто API, не раздают фронтенд-статику
3. Фронтенды работают через Vite dev server (не подходит для прода)

## Архитектура на Railway

```
Railway Project
├── Service: keepstar-chat  (root: project/)
│   └── Go binary → :PORT
│       ├── /api/v1/*        → API handlers
│       ├── /health          → healthcheck
│       └── /*               → React SPA (из static/)
│
├── Service: keepstar-admin  (root: project_admin/)
│   └── Go binary → :PORT
│       ├── /admin/api/*     → Admin API handlers
│       ├── /health          → healthcheck
│       └── /*               → Admin React SPA (из static/)
│
└── PostgreSQL (Neon, внешний)
```

## Что сделано

### Step 1: Chat backend — SPA file server
**Файл:** `project/backend/cmd/server/main.go`
- Добавлен `"path/filepath"` в imports
- После всех API-роутов — catch-all `/` handler:
  - Проверяет `./static/` директорию
  - Если файл существует (JS, CSS, assets) — отдаёт как есть
  - Если нет — fallback на `index.html` (SPA routing)
- Логирует `spa_file_server_enabled`

### Step 2: Admin backend — SPA file server
**Файл:** `project_admin/backend/cmd/server/main.go`
- Тот же паттерн SPA-сервера что и для chat
- Использует `log` переменную (стиль существующего кода)

### Step 3: Admin PORT config
**Файл:** `project_admin/backend/internal/config/config.go`
- `Port: getEnv("PORT", getEnv("ADMIN_PORT", "8081"))`
- Railway всегда ставит `PORT`, локально — `ADMIN_PORT`

### Step 4: Dockerfile для Chat
**Файл:** `project/Dockerfile` (новый)
- Multi-stage build: Node 22 → Go 1.24 → Alpine 3.21
- Frontend: `npm ci && npm run build` с `VITE_API_URL=/api/v1`
- Backend: `CGO_ENABLED=0 GOOS=linux go build`
- Runtime: `./server` + `./static/` (frontend dist)

### Step 5: Dockerfile для Admin
**Файл:** `project_admin/Dockerfile` (новый)
- Тот же multi-stage паттерн
- Без `VITE_API_URL` (admin использует relative paths)

### Step 6: Debugging — embed error logging
**Файл:** `project/backend/internal/tools/tool_catalog_search.go`
- Embedding ошибка глоталась молча (`if err == nil && len(embeddings) > 0`)
- Исправлено: `meta["embed_error"] = embErr.Error()` — ошибка попадает в metadata

### Step 7: Trace logging — search diagnostics
**Файл:** `project/backend/internal/adapters/postgres/postgres_trace.go`
- Добавлен вывод в pipeline trace logs:
  - `embed: {ms}ms  ERROR: {error}` — время и ошибка embedding
  - `results: keyword={n} vector={n} merged={n} type={type}` — счётчики поиска

## Railway env vars

**Chat service:**
- `DATABASE_URL` — Neon PostgreSQL (полный URL с sslmode)
- `ANTHROPIC_API_KEY` — Claude API
- `OPENAI_API_KEY` — OpenAI embeddings
- `TENANT_SLUG` — дефолтный тенант (keepstart)
- `ENVIRONMENT=production`

**Admin service:**
- `DATABASE_URL` — та же Neon PostgreSQL
- `OPENAI_API_KEY` — для embeddings при импорте
- `JWT_SECRET` — секрет для JWT
- `ENVIRONMENT=production`

## Проблемы при деплое

1. **Railpack build fail** — не было root directory. Решение: настроить Root Directory в Railway settings
2. **Admin 502** — порт 8081 vs Railway PORT. Решение: добавить PORT=8081 в Railway Variables + смена региона Singapore → Netherlands
3. **Search 0 results** — OpenAI ключ в Railway отличался от рабочего. Решение: скопировать правильный ключ из .env
4. **Embed error invisible** — ошибка embedding глоталась молча. Решение: Steps 6-7 выше
5. **DATABASE_URL split** — Railway разбил URL на `&`. Решение: вставлять через JSON tab, не ENV tab

## Файлы изменены

| Файл | Действие |
|------|----------|
| `project/backend/cmd/server/main.go` | SPA file server |
| `project_admin/backend/cmd/server/main.go` | SPA file server |
| `project_admin/backend/internal/config/config.go` | PORT fallback |
| `project/Dockerfile` | Новый — multi-stage build |
| `project_admin/Dockerfile` | Новый — multi-stage build |
| `project/backend/internal/tools/tool_catalog_search.go` | embed_error logging |
| `project/backend/internal/adapters/postgres/postgres_trace.go` | search diagnostics в trace |

## Результат

- Chat: фронтенд + API на одном домене, поиск работает (hybrid: keyword + vector)
- Admin: фронтенд + API на одном домене, авторизация и каталог работают
- Pipeline: "покажи смартфоны" → 2 товара с карточками ✅
- session/init возвращает 500 (известная проблема, pipeline работает напрямую)
