# Handlers

HTTP слой. Только parse/validate/respond.

## Файлы

- `handler_chat.go` — POST /api/v1/chat
- `handler_session.go` — GET /api/v1/session/{id} (checks SessionTTL on read)
- `handler_catalog.go` — GET /api/v1/tenants/{slug}/products
- `handler_pipeline.go` — POST /api/v1/pipeline (two-agent pipeline)
- `handler_navigation.go` — POST /api/v1/navigation/expand, /back (drill-down navigation)
- `handler_debug.go` — Debug console for pipeline metrics + POST /debug/seed
- `handler_trace.go` — Pipeline trace list/detail (HTML/JSON) + kill-session + waterfall visualization
- `handler_health.go` — HealthHandler struct, GET /health, GET /ready
- `routes.go` — SetupRoutes(), SetupNavigationRoutes(), SetupCatalogRoutes()
- `middleware_cors.go` — CORS middleware
- `middleware_tenant.go` — Tenant resolution middleware
- `response.go` — JSON response helper

## API

```
POST /api/v1/chat                        — Отправить сообщение
GET  /api/v1/session/{id}                — Получить историю сессии
GET  /api/v1/tenants/{slug}/products     — Список товаров тенанта
GET  /api/v1/tenants/{slug}/products/{id} — Один товар
POST /api/v1/pipeline                    — Two-agent pipeline
POST /api/v1/navigation/expand           — Expand widget to detail view
POST /api/v1/navigation/back             — Navigate back from detail view
GET  /debug/session/                     — Debug console (all sessions)
GET  /debug/session/{id}                 — Session detail (HTML/JSON)
POST /debug/seed                         — Create session with mock products (no LLM)
GET  /debug/api                          — Debug API (JSON)
GET  /debug/traces/                      — Pipeline trace list (HTML/JSON)
GET  /debug/traces/{id}                  — Trace detail (HTML/JSON)
POST /debug/kill-session                 — Kill session (delete all data)
GET  /health                             — Health check
GET  /ready                              — Readiness check
```

### POST /api/v1/chat
Request:
```json
{ "sessionId?": "uuid", "tenantId?": "string", "message": "string" }
```
Response:
```json
{ "sessionId": "uuid", "response": "string", "latencyMs": 1234 }
```

### GET /api/v1/tenants/{slug}/products
Query: `?category=&brand=&search=&minPrice=&maxPrice=&limit=&offset=`
Response:
```json
{ "products": [...], "total": 8 }
```

### GET /api/v1/tenants/{slug}/products/{id}
Response:
```json
{
  "id": "uuid",
  "name": "Nike Air Max 90",
  "price": 1299000,
  "priceFormatted": "12 990 ₽",
  "brand": "Nike",
  "images": [...],
  ...
}
```

### POST /api/v1/pipeline
Request:
```json
{ "sessionId?": "uuid", "query": "string" }
```
Response:
```json
{
  "sessionId": "uuid",
  "formation": { "mode": "grid", "grid": { "cols": 2 }, "widgets": [...] },
  "agent1Ms": 234,
  "agent2Ms": 156,
  "totalMs": 390
}
```

### Debug Console

`GET /debug/session/` — HTML страница со списком всех сессий
`GET /debug/session/{id}` — Детали сессии (HTML, или JSON с `?format=json`)
`GET /debug/api?session={id}` — JSON API для дебага

AgentMetrics включает cache поля: `CacheCreationInputTokens`, `CacheReadInputTokens`, `CacheHitRate`

### Trace Handler (handler_trace.go)

Trace list включает колонку **TTFB** (max LLM time-to-first-byte из span'ов).

Trace detail содержит секцию **Waterfall** — интерактивная визуализация timeline span'ов:
- Горизонтальные полосы показывают timing каждого span'а относительно pipeline start
- Template funcs: `spanDepth` (indent по точкам), `spanLabel` (человекочитаемые названия), `spanColor` (цвет по типу операции), `spanPercent` (позиционирование), `maxTTFB`
- Цветовая схема: ttfb=cyan, llm=blue, body=dark blue, tool=green, embed=bright green, sql=yellow, vector=magenta, state=gray, pipeline=purple
- Легенда с перечислением всех типов span'ов

## Правила

- Никакой бизнес-логики, только:
  1. Парсинг запроса
  2. Валидация
  3. Вызов use case
  4. Формирование ответа
- Request/Response типы рядом с handler
