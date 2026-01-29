# Anthropic Adapter

Клиент для Anthropic API (Claude).

## Файлы

- `anthropic_client.go` — Реализация LLMPort

## Реализует

- `ports.LLMPort`

## Конфигурация

Env vars:
- `ANTHROPIC_API_KEY` — API ключ
- `LLM_MODEL` — Модель (default: claude-3-haiku-20240307)
