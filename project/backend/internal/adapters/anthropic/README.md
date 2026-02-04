# Anthropic Adapter

Клиент для Anthropic API (Claude).

## Файлы

- `anthropic_client.go` — Реализация LLMPort (Chat, ChatWithTools, ChatWithToolsCached, ChatWithUsage)
- `cache_types.go` — Типы для prompt caching (cacheControl, anthropicCachedRequest/Response)

## Реализует

- `ports.LLMPort`
  - `Chat(ctx, message)` — простой текстовый чат
  - `ChatWithTools(ctx, systemPrompt, messages, tools)` — чат с tool calling
  - `ChatWithToolsCached(ctx, systemPrompt, messages, tools, cacheConfig)` — чат с prompt caching
  - `ChatWithUsage(ctx, systemPrompt, userMessage)` — чат с usage статистикой

## Prompt Caching

`ChatWithToolsCached` добавляет `cache_control: {type: "ephemeral"}` в три точки:
1. Последний tool definition
2. System prompt block
3. Предпоследнее сообщение (конец истории, новое сообщение не кэшируется)

Helpers: `markMessageCacheControl(msg)`, `convertToAnthropicMessage(msg)`

## Конфигурация

Env vars:
- `ANTHROPIC_API_KEY` — API ключ
- `LLM_MODEL` — Модель (default: claude-haiku-4-5-20251001)

## Pricing

| Model | Input | Output | Cache Write | Cache Read |
|-------|-------|--------|-------------|------------|
| Haiku | $1/1M | $5/1M | $1.25/1M | $0.10/1M |
| Sonnet | $3/1M | $15/1M | $3.75/1M | $0.30/1M |
