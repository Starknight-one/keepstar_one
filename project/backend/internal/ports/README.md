# Ports

Интерфейсы (контракты) для внешних зависимостей.

## Файлы

- `llm_port.go` — LLMPort interface (для AI провайдеров)
- `search_port.go` — SearchPort interface (для поиска товаров)
- `cache_port.go` — CachePort interface (для кэширования)

## Правила

- Только интерфейсы, никакой реализации
- Импорты только из `domain/`
- Адаптеры реализуют эти интерфейсы
