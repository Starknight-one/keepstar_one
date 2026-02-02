# PostgreSQL Adapter

Адаптер для Neon PostgreSQL. Реализует CachePort, EventPort, CatalogPort и StatePort.

## Файлы

- `postgres_client.go` — Connection pool (pgxpool)
- `postgres_cache.go` — Реализация CachePort
- `postgres_events.go` — Реализация EventPort
- `postgres_catalog.go` — Реализация CatalogPort с product merging
- `postgres_state.go` — Реализация StatePort для two-agent pipeline
- `migrations.go` — Миграции для chat таблиц
- `catalog_migrations.go` — Миграции для catalog схемы
- `state_migrations.go` — Миграции для state таблиц
- `catalog_seed.go` — Seed данные (tenants, categories, products)

## Схемы и таблицы

### public (chat)

| Таблица | Назначение |
|---------|------------|
| chat_users | Пользователи/посетители |
| chat_sessions | Сессии чата |
| chat_messages | Сообщения |
| chat_events | События аналитики |

### catalog

| Таблица | Назначение |
|---------|------------|
| tenants | Бренды, ритейлеры, реселлеры |
| categories | Категории товаров (дерево) |
| master_products | Канонические товары |
| products | Листинги товаров по тенантам |

### state

| Таблица | Назначение |
|---------|------------|
| session_states | Текущее состояние сессии (JSONB) |
| session_deltas | История дельт для replay |

## Использование

```go
// Создание клиента
client, err := postgres.NewClient(ctx, databaseURL)

// Запуск миграций
client.RunMigrations(ctx)
client.RunCatalogMigrations(ctx)
client.RunStateMigrations(ctx)

// Seed данных
postgres.SeedCatalogData(ctx, client)

// Создание адаптеров
cacheAdapter := postgres.NewCacheAdapter(client)
eventAdapter := postgres.NewEventAdapter(client)
catalogAdapter := postgres.NewCatalogAdapter(client)
stateAdapter := postgres.NewStateAdapter(client)
```

## Требования

- PostgreSQL 14+
- SSL required (`sslmode=require`)
- Для Neon: `channel_binding=require`

## ENV

```
DATABASE_URL=postgresql://user:pass@host/db?sslmode=require&channel_binding=require
```
