# Handlers

HTTP слой. Только parse/validate/respond.

## Файлы

- `handler_chat.go` — POST /api/v1/chat
- `handler_session.go` — GET /api/v1/session/{id}
- `handler_health.go` — GET /health, GET /ready
- `routes.go` — Настройка роутов
- `middleware_cors.go` — CORS middleware

## API

```
POST /api/v1/chat           — Отправить сообщение
GET  /api/v1/session/{id}   — Получить историю сессии
GET  /health                — Health check
GET  /ready                 — Readiness check
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

### GET /api/v1/session/{id}
Response:
```json
{
  "id": "uuid",
  "status": "active|closed|archived",
  "messages": [...],
  "startedAt": "timestamp",
  "lastActivityAt": "timestamp"
}
```

## Правила

- Никакой бизнес-логики, только:
  1. Парсинг запроса
  2. Валидация
  3. Вызов use case
  4. Формирование ответа
- Request/Response типы рядом с handler
