# Ports

Интерфейсы (контракты) для внешних зависимостей.

## Файлы

- `llm_port.go` — LLMPort interface (для AI провайдеров)
- `search_port.go` — SearchPort interface (для поиска товаров)
- `cache_port.go` — CachePort interface (для сессий и кэширования)
- `event_port.go` — EventPort interface (для аналитики событий)

## Интерфейсы

### LLMPort
```go
Chat(ctx, message) (string, error)
```

### CachePort
```go
GetSession(ctx, id) (*Session, error)
SaveSession(ctx, session) error
CacheProducts(ctx, sessionID, products) error
GetCachedProducts(ctx, sessionID) ([]Product, error)
```

### EventPort
```go
TrackEvent(ctx, event) error
GetSessionEvents(ctx, sessionID) ([]ChatEvent, error)
```

## Правила

- Только интерфейсы, никакой реализации
- Импорты только из `domain/`
- Адаптеры реализуют эти интерфейсы
