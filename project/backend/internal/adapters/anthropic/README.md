# Anthropic Adapter

Клиент для Anthropic API (Claude).

## Файлы

- `anthropic_client.go` — Реализация LLMPort

## Реализует

- `ports.LLMPort`
  - `Chat(ctx, message)` — простой текстовый чат
  - `ChatWithTools(ctx, systemPrompt, messages, tools)` — чат с tool calling
  - `ChatWithUsage(ctx, systemPrompt, userMessage)` — чат с usage статистикой

## Конфигурация

Env vars:
- `ANTHROPIC_API_KEY` — API ключ
- `LLM_MODEL` — Модель (default: claude-haiku-4-5-20251001)

## Pricing

| Model | Input | Output |
|-------|-------|--------|
| Haiku | $1/1M | $5/1M |
| Sonnet | $3/1M | $15/1M |
