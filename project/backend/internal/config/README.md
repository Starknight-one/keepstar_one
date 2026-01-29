# Config

Конфигурация приложения через env vars.

## Файлы

- `config.go` — Загрузка конфигурации

## Env vars

```
PORT=8080
ENVIRONMENT=development
ANTHROPIC_API_KEY=sk-ant-xxx
LLM_MODEL=claude-3-haiku-20240307
LOG_LEVEL=debug
```

## Правила

- Все секреты через env vars
- Defaults для development
- Required для production
