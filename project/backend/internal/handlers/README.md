# Handlers

HTTP слой. Только parse/validate/respond.

## Файлы

- `handler_chat.go` — POST /api/v1/chat
- `handler_session.go` — GET /api/v1/session/{id}
- `handler_catalog.go` — GET /api/v1/tenants/{slug}/products
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

## Правила

- Никакой бизнес-логики, только:
  1. Парсинг запроса
  2. Валидация
  3. Вызов use case
  4. Формирование ответа
- Request/Response типы рядом с handler
