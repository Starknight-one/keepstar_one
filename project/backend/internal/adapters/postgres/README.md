# PostgreSQL Adapter

Адаптер для Neon PostgreSQL. Реализует CachePort и EventPort.

## Файлы

- `postgres_client.go` — Connection pool (pgxpool)
- `postgres_cache.go` — Реализация CachePort
- `postgres_events.go` — Реализация EventPort
- `migrations.go` — Автоматические миграции

## Таблицы

| Таблица | Назначение |
|---------|------------|
| chat_users | Пользователи/посетители |
| chat_sessions | Сессии чата |
| chat_messages | Сообщения |
| chat_events | События аналитики |

## Использование

```go
// Создание клиента
client, err := postgres.NewClient(ctx, databaseURL)

// Запуск миграций
client.RunMigrations(ctx)

// Создание адаптеров
cacheAdapter := postgres.NewCacheAdapter(client)
eventAdapter := postgres.NewEventAdapter(client)
```

## Требования

- PostgreSQL 14+
- SSL required (`sslmode=require`)
- Для Neon: `channel_binding=require`

## ENV

```
DATABASE_URL=postgresql://user:pass@host/db?sslmode=require&channel_binding=require
```
