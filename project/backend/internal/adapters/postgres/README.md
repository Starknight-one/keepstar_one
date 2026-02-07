# PostgreSQL Adapter

Адаптер для Neon PostgreSQL. Реализует CachePort, EventPort, CatalogPort, StatePort и TracePort.

## Файлы

- `postgres_client.go` — Connection pool (pgxpool)
- `postgres_cache.go` — Реализация CachePort (incl. DeleteSession)
- `postgres_events.go` — Реализация EventPort
- `postgres_catalog.go` — Реализация CatalogPort с product merging + VectorSearch (pgvector cosine), SeedEmbedding, GetMasterProductsWithoutEmbedding
- `postgres_state.go` — Реализация StatePort для two-agent pipeline
- `postgres_trace.go` — Реализация TracePort: Record (DB + console printTrace с WATERFALL секцией для span'ов), List, Get
- `migrations.go` — Миграции для chat таблиц
- `catalog_migrations.go` — Миграции для catalog схемы + pgvector extension, embedding vector(384) column, HNSW index
- `state_migrations.go` — Миграции для state таблиц
- `trace_migrations.go` — Миграции для pipeline_traces таблицы
- `catalog_seed.go` — Seed данные (tenants, categories, products)
- `retention.go` — RetentionService: periodic cleanup (traces, dead sessions, conversation trim)
- `catalog_search_test.go` — Тесты CatalogPort (search)
- `postgres_state_test.go` — Интеграционные тесты StatePort (zone-write, deltas)

## Схемы и таблицы

### public (chat)

| Таблица | Назначение |
|---------|------------|
| chat_users | Пользователи/посетители |
| chat_sessions | Сессии чата |
| chat_messages | Сообщения |
| chat_events | События аналитики |
| chat_session_state | Текущее состояние сессии (JSONB), conversation_history |
| chat_session_deltas | История дельт для replay (включая turn_id) |
| pipeline_traces | Трейсы pipeline (timing, cost, tool breakdown) |

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
