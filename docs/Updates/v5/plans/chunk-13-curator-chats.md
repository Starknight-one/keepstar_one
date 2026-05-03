# V5 — Chunk 13: Cross-tenant chat / trace inspection in Curator

## Context

Chunks 9–12 закрыли весь P0 + render polish. После live smoke и manual
теста стало очевидно: для дебага «почему карточки кривые / почему
modify-bias / почему дорогая турна» нужен **Curator UI**, который видит
все сессии across tenants с per-turn token + cost + spans waterfall.
Сейчас единственные способы посмотреть session:
- tail `/tmp/v5-logs/backend.log` (теряется на restart)
- ручной SQL по `v5_chat_session_deltas` (без spans / cost)
- открыть devtools на pipelineResponse.spans (один турн, теряется
  при закрытии вкладки)

Не масштабируется ни на dev box, ни тем более на production — где
сессий много, тенантов несколько, и нужно ловить паттерны
(modify-bias, field-binding mismatch, дорогие сессии).

Cross-tenant chats UI давно лежит в `docs/v5-known-gaps.md` (секция
«Cross-tenant chat / trace inspection in Curator (no UI yet)»). Чанк 13
её закрывает в production-ready виде сразу — потому что V5 движок
скоро тоже летит на Railway.

## Locked-in decisions (после разведки)

- **Persist spans в Neon** в новой таблице `v5_chat_session_traces`
  каждый pipeline-turn. Curator читает её напрямую (общая Neon база с
  V5 и V4 — Curator уже коннектится к тому же DATABASE_URL).
- **Trace identity = request_id** (уже генерируется в logging
  middleware на каждый HTTP-запрос). TurnID отдельно не вводим —
  request_id и так уникален per turn, и логически он и есть turn id.
- **Spans хранятся как JSONB** (variable-length, nested). Агрегаты
  (latency_ms, agent1_ms, agent2_ms, tokens, cost_usd, status) —
  отдельные columns, чтобы Curator queries (sort/filter/aggregate) шли
  по индексам, не по JSONB.
- **Persistence — best effort**, non-blocking. Если запись trace
  падает — пайплайн всё равно отвечает (юзер не должен видеть
  internal-storage error). Лог + continue.
- **Curator читает v5_chat_sessions / v5_chat_session_traces НАПРЯМУЮ**
  (общая БД). Никакого proxy через V5 backend — это лишний слой и
  кэш-инвалидация. Curator уже читает `catalog.tenants` + `catalog.products` тем же паттерном.
- **Sidebar entry**: новая секция «Tracing» (между «Operations» и
  «Curation queues»), пункт «Chat Sessions».
- **Trace retention**: на эту итерацию не trim'аем — Neon хранит
  всё. Cron-cleanup (TTL 30/60/90 days) — отдельный chunk если/когда
  объём начнёт раздражать.
- **Railway-ready с первого коммита**: миграция запускается на boot
  (тот же `Run*Migrations` pattern), Curator уже имеет Dockerfile +
  port=PORT env. Никаких deploy-only шагов вне миграций.

## Approach

### Part A — V5 backend: trace persistence

1. **New migration** `internal/adapters/postgres/migrations_trace.go`:
   ```sql
   CREATE TABLE IF NOT EXISTS v5_chat_session_traces (
       id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
       session_id UUID NOT NULL REFERENCES v5_chat_sessions(id) ON DELETE CASCADE,
       request_id TEXT NOT NULL,
       tenant_id TEXT NOT NULL,
       user_query TEXT NOT NULL,
       spans JSONB NOT NULL,
       span_count INTEGER NOT NULL,
       latency_ms BIGINT NOT NULL,
       agent1_ms BIGINT,
       agent2_ms BIGINT,
       tokens_input BIGINT,
       tokens_output BIGINT,
       tokens_cache_read BIGINT,
       tokens_cache_creation BIGINT,
       cost_usd NUMERIC(10,6),
       status VARCHAR(20) NOT NULL DEFAULT 'ok',
       error_message TEXT,
       created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
       UNIQUE(session_id, request_id)
   );
   CREATE INDEX IF NOT EXISTS idx_v5_traces_tenant_created
       ON v5_chat_session_traces(tenant_id, created_at DESC);
   CREATE INDEX IF NOT EXISTS idx_v5_traces_session_created
       ON v5_chat_session_traces(session_id, created_at DESC);
   CREATE INDEX IF NOT EXISTS idx_v5_traces_status_created
       ON v5_chat_session_traces(status, created_at DESC) WHERE status = 'error';
   ```
2. **Migration registration** in `client.go`: new method
   `RunTraceMigrations(ctx)` reading `traceSchemaSQL` const.
3. **main.go**: добавить `{"trace", pgClient.RunTraceMigrations}` в
   migrations loop.
4. **TraceAdapter** (new `internal/adapters/postgres/postgres_trace.go`):
   - `SaveTrace(ctx, row TraceRow) error` — INSERT; на конфликт по
     `(session_id, request_id)` тихо игнорировать (idempotent retry).
   - Кошерные методы для Curator чтения вынесем в Curator-side,
     не в V5 backend (Curator читает напрямую).
5. **TracePort** (`internal/ports/trace_port.go`): single
   `SaveTrace(ctx, ...) error` interface для DI.
6. **TraceRow domain type** (`internal/domain/trace.go`): структура с
   полями таблицы + JSON-marshalable.
7. **Hook in handler_pipeline.go**: после `resp, err := h.pipeline.Execute()`
   и перед `writeJSON`, если `sc := domain.SpanFromContext(...)` есть,
   собрать TraceRow и зашипировать в TracePort асинхронно (через
   `go h.tracePort.SaveTrace(...)`) с recover-deferred-log на panics.
   Не ждём результат — UI важнее.
8. **NewPipelineHandler** + main.go DI: добавить TracePort деп.
9. Live smoke (`handler_pipeline_live_test.go`): после каждого turn
   делаем `SELECT count FROM v5_chat_session_traces WHERE session_id = $1`
   и проверяем что count == ожидаемое количество turn'ов.

### Part B — Curator backend: read endpoints

Все три endpoint'а под общим auth middleware (cookie session) — как
ListTenants и AuditList. Доступ к `v5_*` таблицам — через тот же
curator-backend's pgxpool.

1. **`GET /api/chats?tenant=&status=&q=&limit=&offset=`** — список
   сессий. Default sort: `last_activity DESC`. Filters:
   - `tenant` — slug или uuid (приводим к id через
     `catalog.tenants` lookup, как в V5 adapter).
   - `status` — `active` (≤ 30 min с last_activity) | `closed` |
     `all` (default).
   - `q` — substring на `session_id` ИЛИ `tenant_slug` ИЛИ user_query
     (через trace's user_query).
   - `limit / offset` — pagination, default 50 / 0.
   Response: `{sessions: [{sessionId, tenantSlug, tenantName, createdAt,
   lastActivityAt, turnCount, totalCostUsd, status, latestQuery}],
   total, hasMore}`.

2. **`GET /api/chats/:sessionId`** — full timeline. Response:
   `{session: {sessionId, tenantSlug, ...}, turns: [{requestId,
   userQuery, latencyMs, agent1Ms, agent2Ms, tokensInput, tokensOutput,
   costUsd, status, createdAt}], totalCostUsd, totalTokens}`. Сортировка
   turns по `created_at ASC`.

3. **`GET /api/chats/:sessionId/turns/:requestId`** — full turn detail
   incl. spans waterfall. Response: `{turn: {<all turn fields>},
   spans: [<full Span objects>]}`. Spans уже в БД как JSONB — отдаём
   as-is.

4. **Adapter методы** (new `curator/backend/internal/adapters/v5_chats.go`):
   - `ListChats(ctx, ListChatsFilter) ([]ChatRow, int, error)` —
     LEFT JOIN v5_chat_sessions + aggregate from
     v5_chat_session_traces.
   - `GetChatTimeline(ctx, sessionID) (ChatTimeline, error)` —
     SELECT all traces for session.
   - `GetChatTurn(ctx, sessionID, requestID) (TurnDetail, error)` —
     SELECT one trace.

5. **Handlers** (extend `curator/backend/internal/handlers/`): new file
   `handler_chats.go` — три HandlerFunc.

6. **Routes**: register под `mux.HandleFunc("GET /api/chats", ...)`,
   `GET /api/chats/{sessionId}`, `GET /api/chats/{sessionId}/turns/{requestId}`.

### Part C — Curator frontend: ChatsPage + ChatDetailPage

1. **API helper extension** (`curator/frontend/src/api.js`): add
   `listChats(filters)`, `getChatTimeline(sessionId)`,
   `getChatTurn(sessionId, requestId)` thin wrappers around `api.get()`.

2. **`pages/ChatsPage.jsx`** (new ~150 lines, pattern от
   `TenantsPage.jsx` + `AuditPage.jsx`):
   - Top filter row: tenant dropdown, status select, search input.
   - Table: tenant_slug | session_id (short) | started_at |
     last_activity | turn_count | total_cost ($) | status badge.
   - Click row → navigate `/chats/:sessionId`.
   - Pagination: «Load more» button (offset stepped).

3. **`pages/ChatDetailPage.jsx`** (new ~200 lines):
   - Header: tenant + session_id + start time + total cost / tokens.
   - Per-turn timeline table: # | userQuery (truncated) | tokens
     in/out | cost ($) | latency_ms | status. Click row → expand
     spans waterfall (inline `<details>` или modal).
   - Spans waterfall — простая Gantt-like таблица: name | depth |
     start_offset_ms | duration_ms | status | attrs (JSON dump).
     Никаких SVG-чартов на этом шаге — простая текстовая визуализация.

4. **Routes + sidebar** in `App.jsx`:
   - Two new `<Route>` entries: `/chats` + `/chats/:sessionId`.
   - New sidebar section «Tracing» с NavLink на `/chats`.

### Part D — Tests + Railway readiness

1. **V5 backend**:
   - Unit test `postgres_trace_test.go` (integration tag):
     SaveTrace + двойной insert на same (session_id, request_id) —
     второй INSERT тихо ignore (UNIQUE constraint behaviour либо
     ON CONFLICT DO NOTHING).
   - Live smoke extension: после каждого turn в test verify
     `SELECT count FROM v5_chat_session_traces WHERE session_id = $1`.

2. **Curator backend**:
   - Не пишем mock-state тестов на этой итерации (Curator pattern —
     curl-via-running-process; existing handlers тоже без unit tests).
   - Smoke: запустить curator backend локально, дёрнуть три endpoint'а
     curl'ом, проверить shape ответа. (manual; задокументировано в
     log.)

3. **Curator frontend**:
   - Vitest не настроен в Curator. Smoke = npm run build не падает +
     визуальная проверка локально (manual).

4. **Railway readiness**:
   - **V5**: новая миграция запускается на boot существующим runner'ом.
     Никаких новых env vars. Существующий Dockerfile (если есть; иначе
     это P1 deploy chunk) подхватит без изменений.
   - **Curator**: уже имеет Dockerfile + multi-stage build + frontend
     bundled в backend's static. Чанк 13 не вводит новых env vars,
     поэтому никаких railway.toml правок не требуется.
   - **Cross-service конфликтов нет**: Curator коннектится к тому же
     DATABASE_URL — Neon один. V5 пишет в `v5_chat_session_traces`,
     Curator читает оттуда же. Read-write isolation на app level
     (Curator никогда не пишет в v5_* таблицы).

## Files changed (planned)

V5 backend:
| File | Status | Notes |
|---|---|---|
| `internal/adapters/postgres/migrations_trace.go` | added | DDL + RunTraceMigrations |
| `internal/adapters/postgres/postgres_trace.go` | added | TraceAdapter.SaveTrace |
| `internal/adapters/postgres/postgres_trace_test.go` | added | integration insert + idempotency |
| `internal/domain/trace.go` | added | TraceRow domain |
| `internal/ports/trace_port.go` | added | TracePort interface |
| `internal/handlers/handler_pipeline.go` | modified | + TracePort dep, write trace post-Execute |
| `internal/handlers/handler_pipeline_live_test.go` | modified | wire TracePort, assert traces persisted |
| `cmd/server/main.go` | modified | + RunTraceMigrations + NewTraceAdapter + DI into PipelineHandler |

Curator backend:
| File | Status | Notes |
|---|---|---|
| `curator/backend/internal/adapters/v5_chats.go` | added | ListChats / GetChatTimeline / GetChatTurn |
| `curator/backend/internal/handlers/handler_chats.go` | added | 3 routes |
| `curator/backend/cmd/server/main.go` | modified | wire 3 routes |

Curator frontend:
| File | Status | Notes |
|---|---|---|
| `curator/frontend/src/api.js` | modified | + listChats / getChatTimeline / getChatTurn |
| `curator/frontend/src/pages/ChatsPage.jsx` | added | list + filters |
| `curator/frontend/src/pages/ChatDetailPage.jsx` | added | timeline + spans waterfall |
| `curator/frontend/src/App.jsx` | modified | + 2 routes + sidebar Tracing section |

Docs:
| File | Status | Notes |
|---|---|---|
| `docs/v5-engine-plan.md` | modified | mark P1 «cross-tenant chats UI» closed; status row chunk 13 |
| `docs/v5-known-gaps.md` | modified | strike-through Curator UI section |
| `docs/Updates/v5/plans/chunk-13-curator-chats.md` | added | frozen plan |
| `docs/Updates/v5/v5_2026-05-03_<HHMM>_chunk-13.md` | added | session log |
| `docs/Updates/v5/README.md` | modified | + chunk 13 entry |
| `CLAUDE.md` | modified | mention chunk 13 |

## Verification

```sh
cd project_v5/backend
go build ./... && go build -tags=integration ./... && \
  go build -tags="integration live" ./... && go build -tags=tokens ./...
go vet ./... && go vet -tags=integration ./... && \
  go vet -tags="integration live" ./...
go test -count=1 ./...

# Live HTTP smoke (~$0.03)
TEST_DATABASE_URL=$DB ANTHROPIC_API_KEY=$KEY \
  go test -tags="integration live" -v -count=1 -timeout 12m \
  ./internal/handlers/... -run TestHTTPLiveSmoke

cd ../../curator/backend
go build ./... && go vet ./...
go test ./...

cd ../frontend
npm run build              # production bundle
```

Manual smoke (за Vlad'ом, на Railway после deploy):
1. Curator UI → sidebar → «Chat Sessions».
2. Видит таблицу сессий across all tenants с турнами и cost'ами.
3. Click сессию → timeline + per-turn spans waterfall.

## Known gaps after chunk 13

- **Trace retention / cron cleanup** — Neon хранит всё; cron-trim
  (TTL N days) откладывается до момента когда объём станет
  заметным.
- **Spans waterfall как SVG-Gantt** — на этой итерации текстовая
  таблица; визуальный Gantt — polish item.
- **Frontend testing** — Curator не имеет vitest setup; smoke =
  manual.
- **Cross-DB edge case** — если в будущем Curator переедет на
  отдельный Neon (разные DBs), потребуется replication / CDC между
  V5 и Curator. Сегодня одна общая БД, не проблема.
- **TurnID** — пока используем request_id как trace identity.
  Если фронт/бэк начнут вводить explicit turnId, добавим в
  v5_chat_session_traces столбец и фильтр.

## Quick reference for execution

- **Branch**: `v5`. **Last commit** before chunk-13: `68ddf27`.
- **Local dev**: V5 backend `:8084`, V5 frontend `:5173`, Curator
  backend `:8082` (clash with V4 prod-binary on dev — V4 нужно
  останавливать чтобы запустить Curator), Curator frontend `:5175`.
- **Common Neon DATABASE_URL** — same env in V5/V4/admin/curator.
- **First reads** during execution:
  1. This file.
  2. `project_v5/backend/internal/domain/span.go` (lines 34-45).
  3. `project_v5/backend/internal/adapters/postgres/state_migrations.go`
     (для образца DDL).
  4. `project_v5/backend/cmd/server/main.go` (migration loop).
  5. `project_v5/backend/internal/handlers/handler_pipeline.go` lines 96-98
     (где брать spans + где hook'аем persist).
  6. `curator/backend/cmd/server/main.go` (как router + middleware).
  7. `curator/backend/internal/handlers/handler_tenants.go` (handler
     pattern + auth).
  8. `curator/backend/internal/adapters/postgres.go` (как делать
     cross-tenant SQL — pattern ListTenants).
  9. `curator/frontend/src/App.jsx` (sidebar + routes).
  10. `curator/frontend/src/pages/TenantsPage.jsx` или
      `AuditPage.jsx` (fetch/table/filter pattern).
