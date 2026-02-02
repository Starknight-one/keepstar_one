# PostgreSQL Adapter

Адаптер для Neon PostgreSQL. Реализует CachePort, EventPort и CatalogPort.

## Файлы

- `postgres_client.go` — Connection pool (pgxpool)
- `postgres_cache.go` — Реализация CachePort
- `postgres_events.go` — Реализация EventPort
- `postgres_catalog.go` — Реализация CatalogPort с product merging
- `migrations.go` — Миграции для chat таблиц
- `catalog_migrations.go` — Миграции для catalog схемы
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

## Использование

```go
// Создание клиента
client, err := postgres.NewClient(ctx, databaseURL)

// Запуск миграций
client.RunMigrations(ctx)
client.RunCatalogMigrations(ctx)

// Seed данных
postgres.SeedCatalogData(ctx, client)

// Создание адаптеров
cacheAdapter := postgres.NewCacheAdapter(client)
eventAdapter := postgres.NewEventAdapter(client)
catalogAdapter := postgres.NewCatalogAdapter(client)
```

## Требования

- PostgreSQL 14+
- SSL required (`sslmode=require`)
- Для Neon: `channel_binding=require`

## ENV

```
DATABASE_URL=postgresql://user:pass@host/db?sslmode=require&channel_binding=require
```
