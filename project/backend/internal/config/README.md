# Config

Конфигурация приложения через env vars.

## Файлы

- `config.go` — Загрузка конфигурации

## Env vars

```
PORT=8080
ENVIRONMENT=development
ANTHROPIC_API_KEY=sk-ant-xxx
LLM_MODEL=claude-haiku-4-5-20251001
LOG_LEVEL=debug
DATABASE_URL=postgresql://user:pass@host/db?sslmode=require
TENANT_SLUG=nike
OPENAI_API_KEY=sk-xxx
EMBEDDING_MODEL=text-embedding-3-small
```

## Helpers

- `HasDatabase()` — returns true if DATABASE_URL is configured
- `HasEmbeddings()` — returns true if OPENAI_API_KEY is configured

## Правила

- Все секреты через env vars
- Defaults для development
- Required для production
