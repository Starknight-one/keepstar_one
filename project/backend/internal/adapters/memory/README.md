# Memory Cache Adapter

In-memory кэш для сессий и данных.

## Файлы

- `memory_cache.go` — Реализация CachePort

## Реализует

- `ports.CachePort`

## Особенности

- Данные теряются при перезапуске
- Для MVP достаточно, потом заменить на Redis
