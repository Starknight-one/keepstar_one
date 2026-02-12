# Done: Logging — Full Project Coverage

**Дата:** 2026-02-12
**Ветка:** `main`
**Статус:** Реализовано, компилируется (chat + admin backends), frontend без ошибок

## Проблема

Проект готовится к пилоту. Без логов в проде — слепой полёт. Нужно видеть: что ищут, что находят, где тупят, что ломается, кто кому что передал, какие задержки на каждом стыке. Pipeline traces были (SpanCollector), но только для pipeline запросов — остальные запросы летели вслепую.

## Что сделано

### Phase 1: Postgres таблица `request_logs` + LogAdapter

**Миграция** (`log_migrations.go`, оба бэкенда):
```sql
CREATE TABLE IF NOT EXISTS request_logs (
    id TEXT PRIMARY KEY,
    timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    service TEXT NOT NULL DEFAULT 'chat',
    method TEXT NOT NULL,
    path TEXT NOT NULL,
    status INTEGER NOT NULL,
    duration_ms BIGINT NOT NULL,
    session_id TEXT,
    tenant_slug TEXT,
    user_id TEXT,
    error TEXT,
    spans JSONB,
    metadata JSONB
);
```
Индексы: `timestamp`, `session_id`, errors (`status >= 400`), `service + timestamp`.

**LogAdapter** (`postgres_logs.go`):
- `RecordRequestLog(ctx, *RequestLog) error` — INSERT с JSONB для spans/metadata
- Вызывается fire-and-forget из middleware (горутина, `context.Background()`)

### Phase 2: Logger Extension — With() + FromContext()

**Chat backend** (`internal/logger/logger.go`):
- `With(args ...any) *Logger` — child logger с доп. полями
- Context keys: `request_id`, `session_id`, `tenant_slug`
- `WithRequestID / WithSessionID / WithTenantSlug` — context setters
- `RequestIDFrom / SessionIDFrom / TenantSlugFrom` — context getters
- `FromContext(ctx) *Logger` — auto-enriches logger с полями из context

**Admin backend** (`internal/logger/logger.go`):
- Те же `With()` + `FromContext()`, ключи: `request_id`, `user_id`, `tenant_id`
- `AdminAction(action, userID, tenantID, detail)` — structured admin action log

### Phase 3: HTTP Middleware — request logging + SpanCollector + persistence

**Chat backend** (`handlers/middleware_logging.go`):
- Каждый HTTP запрос получает: UUID `request_id` → context + `X-Request-ID` header
- `SpanCollector` → в context (переиспользуется из `domain/span.go`)
- `responseWriter` wrapper для capture status code
- По завершении: stdout log (JSON) + async persist в Postgres
- Health endpoints → Debug level, errors → Error level, rest → Info level

**Admin backend** (`handlers/middleware_logging.go`):
- Аналогично, `service: "admin"`, извлечение `user_id` / `tenant_id` из JWT context
- Собственный `domain/span.go` (копия SpanCollector API для отдельного Go module)

### Phase 4: Span инструментация всех слоёв

**Handlers** (chat backend, 5 файлов):
- `handler.pipeline`, `handler.expand`, `handler.back`, `handler.session_init`, `handler.session_get`, `handler.chat`, `handler.catalog_list`
- Добавлен `log *logger.Logger` в PipelineHandler, NavigationHandler, SessionHandler
- SessionID → context для pipeline/navigation handlers

**UseCases** (3 файла):
- `usecase.expand`, `usecase.back`, `usecase.send_message`

**Adapters** (4 файла):

| Adapter | Spans |
|---------|-------|
| `postgres_state.go` | `db.create_state`, `db.get_state`, `db.update_state`, `db.update_data`, `db.update_template`, `db.update_view`, `db.push_view`, `db.pop_view` |
| `postgres_catalog.go` | `db.get_tenant`, `db.list_products`, `db.get_product`, `db.vector_search`, `db.get_catalog_digest`, `db.list_services`, `db.get_service`, `db.vector_search_services` |
| `postgres_cache.go` | `db.get_session`, `db.save_session` |
| `anthropic_client.go` | (уже были: `{stage}.llm`, `{stage}.llm.ttfb`, `{stage}.llm.body`) |

**slog.Warn cleanup:** 16 raw `slog.Warn()` вызовов → `a.log.Warn()` через injected logger (StateAdapter, CatalogAdapter, AnthropicClient).

### Phase 5: Retention — cleanup каждые 3 дня

`retention.go`:
- Новое поле `RequestLogMaxAge time.Duration` в `RetentionConfig` (default: 72h)
- `cleanupRequestLogs()` — `DELETE FROM request_logs WHERE timestamp < $1`
- Вызывается в `runCleanup()` рядом с `cleanupTraces()` и `cleanupDeadSessions()`

### Phase 6: Admin backend — logger + spans + persistence

**Handlers** (5 файлов):
- `AuthHandler`: logger, spans (`handler.signup`, `handler.login`), логирование signup/login success/failure с email
- `ProductsHandler`: logger, spans (`handler.products_list`, `handler.products_get`, `handler.products_update`, `handler.categories_list`), error logging
- `ImportHandler`: logger, spans (`handler.import_upload`, `handler.import_get_job`, `handler.import_list_jobs`), upload start log с items count
- `SettingsHandler`: logger, spans (`handler.settings_get`, `handler.settings_update`), settings updated log
- `StockHandler`: logger, spans (`handler.stock_bulk_update`), bulk update log с items count + affected

**Adapters** (2 файла):
- `catalog_adapter.go`: logger field, spans (`db.admin.list_products`, `db.admin.get_product`, `db.admin.update_product`, `db.admin.upsert_master_product`, `db.admin.upsert_product_listing`, `db.admin.bulk_update_stock`, `db.admin.generate_catalog_digest`, `db.admin.update_tenant_settings`)
- `import_adapter.go`: spans (`db.admin.create_import_job`, `db.admin.get_import_job`, `db.admin.list_import_jobs`, `db.admin.complete_import_job`)

### Phase 7: Frontend logging

**Widget frontend** (`shared/logger/index.js`):
- `log.debug/info/warn/error` — localStorage debug toggle (`debug=true`)
- `log.api(method, path, status, durationMs, error)` — API timing
- Prefix: `[keepstar]` / `[keepstar:api]`

**Admin frontend** (`shared/logger.js`):
- Same API, prefix `[admin]` / `[admin:api]`

**apiClient.js timing** (оба фронтенда):
- `performance.now()` до/после fetch
- `log.api()` при каждом вызове

**Console replacements:**
- `AtomRenderer.jsx:206`: `console.log` → `log.debug`
- `ChatPanel.jsx:88`: `console.error` → `log.error`
- `backgroundSync.js`: `.catch(() => {})` → `.catch(err => log.warn(...))`
- `ChatPanel.jsx:147,162`: silent catches → `log.warn` с описанием

## Пример waterfall (pipeline запрос)

```
http                                    0ms ████████████████████████████ 1250ms
  handler.pipeline                      1ms ███████████████████████████  1248ms
    usecase.pipeline                    2ms ██████████████████████████   1245ms
      db.get_session                    4ms ▌                               8ms
      agent1                           18ms ████████████                  810ms
        db.get_state                   19ms ▌                               4ms
        db.get_catalog_digest          25ms ▌                               8ms
        agent1.llm                     36ms ██████████                    680ms
        agent1.tool                   720ms ██                             95ms
          agent1.tool.embed           721ms ▌                              30ms
          agent1.tool.sql             752ms █                              55ms
        db.update_data                821ms ▌                               7ms
      agent2                          830ms ██████████                    380ms
        db.get_state                  831ms ▌                               4ms
        agent2.llm                    840ms ████████                      320ms
        db.update_template           1166ms ▌                               8ms
      pipeline.build_adjacent       1215ms ▌                              30ms
```

## Файлы

| Действие | Кол-во | Подробности |
|----------|--------|-------------|
| **Новые (chat backend)** | 3 | `log_migrations.go`, `postgres_logs.go`, `middleware_logging.go` |
| **Новые (admin backend)** | 3 | `log_migrations.go`, `postgres_logs.go`, `middleware_logging.go` + `domain/span.go` |
| **Новые (frontend)** | 2 | `logger/index.js` (widget), `logger.js` (admin) |
| **Изменённые (chat backend)** | 12 | `logger.go`, `main.go`, 5 handlers, `retention.go`, 3 adapters, `anthropic_client.go` |
| **Изменённые (admin backend)** | 7 | `logger.go`, `main.go`, 5 handlers, `catalog_adapter.go`, `import_adapter.go` |
| **Изменённые (frontend)** | 5 | `apiClient.js` x2, `backgroundSync.js`, `AtomRenderer.jsx`, `ChatPanel.jsx` |
| **Всего** | ~32 файла |

## Naming Convention

**Span names:** `dot.separated.hierarchy`
- `http` — весь HTTP request
- `handler.{name}` — handler method
- `usecase.{name}` — бизнес-логика
- `agent1`, `agent1.llm`, `agent1.tool` — pipeline agents
- `db.{operation}` — chat database calls
- `db.admin.{operation}` — admin database calls

**Log fields:** `request_id`, `session_id`, `tenant_slug`, `duration_ms`, `method`, `path`, `status`, `error`, `user_id`, `tenant_id`, `action`

## Верификация

1. `go build ./...` — оба бэкенда компилируются
2. При запуске: JSON логи с `request_id` на каждый HTTP запрос
3. `SELECT * FROM request_logs ORDER BY timestamp DESC LIMIT 5` — записи с spans JSONB
4. Ретенция: строки старше 3 дней удаляются автоматически
5. Frontend: `localStorage.setItem('debug', 'true')` → `[keepstar:api] POST /pipeline 200 1250ms`
