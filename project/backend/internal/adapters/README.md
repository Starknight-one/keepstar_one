# Adapters

Реализации портов (интерфейсов). Связь с внешним миром.

## Папки

- `anthropic/` — Клиент для Anthropic API (Claude) → LLMPort
- `postgres/` — PostgreSQL адаптер → CachePort, EventPort, CatalogPort, StatePort, TracePort
- `openai/` — Клиент для OpenAI Embeddings API → EmbeddingPort
- `json_store/` — Хранение товаров в JSON (MVP) → SearchPort
- `memory/` — In-memory кэш (устарел, заменён postgres)

## Статус

| Адаптер | Порт | Статус |
|---------|------|--------|
| anthropic | LLMPort | implemented |
| postgres | CachePort, EventPort, CatalogPort, StatePort, TracePort | implemented |
| openai | EmbeddingPort | implemented |
| json_store | SearchPort | stub |
| memory | CachePort | stub (deprecated) |

## Правила

- Каждый адаптер реализует порт из `ports/`
- Можно импортировать: `domain/`, `ports/`, внешние библиотеки
- Нельзя импортировать: `usecases/`, `handlers/`

## Как добавить новый адаптер

1. Создать папку `adapters/{name}/`
2. Реализовать интерфейс из `ports/`
3. Добавить README.md
