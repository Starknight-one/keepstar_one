# Handlers

HTTP слой. Только parse/validate/respond.

## Файлы

- `handler_chat.go` — POST /api/v1/chat
- `handler_health.go` — GET /health, GET /ready
- `routes.go` — Настройка роутов
- `middleware_cors.go` — CORS middleware
- `middleware_logging.go` — Logging middleware

## Правила

- Никакой бизнес-логики, только:
  1. Парсинг запроса
  2. Валидация
  3. Вызов use case
  4. Формирование ответа
- Request/Response типы рядом с handler (в том же файле или `handler_*_dto.go`)

## API

```
POST /api/v1/chat      — Отправить сообщение
GET  /health           — Health check
GET  /ready            — Readiness check
```
