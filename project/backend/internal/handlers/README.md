# Handlers

HTTP слой. Только parse/validate/respond.

## Файлы

- `handler_chat.go` — POST /api/v1/chat
- `handler_session.go` — GET /api/v1/session/{id} (checks SessionTTL on read)
- `handler_catalog.go` — GET /api/v1/tenants/{slug}/products
- `handler_pipeline.go` — POST /api/v1/pipeline (two-agent pipeline)
- `handler_debug.go` — Debug console for pipeline metrics
- `handler_health.go` — GET /health, GET /ready
- `routes.go` — Настройка роутов
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
GET  /debug/session/                     — Debug console (all sessions)
GET  /debug/session/{id}                 — Session detail (HTML/JSON)
GET  /debug/api                          — Debug API (JSON)
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

## Правила

- Никакой бизнес-логики, только:
  1. Парсинг запроса
  2. Валидация
  3. Вызов use case
  4. Формирование ответа
- Request/Response типы рядом с handler
