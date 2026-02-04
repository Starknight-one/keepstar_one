# Ports

Интерфейсы (контракты) для внешних зависимостей.

## Файлы

- `llm_port.go` — LLMPort interface (для AI провайдеров)
- `cache_port.go` — CachePort interface (для сессий и кэширования)
- `event_port.go` — EventPort interface (для аналитики событий)
- `catalog_port.go` — CatalogPort interface (для каталога товаров)
- `state_port.go` — StatePort interface (для session state)

## Интерфейсы

### LLMPort
```go
Chat(ctx, message) (string, error)
ChatWithTools(ctx, systemPrompt, messages, tools) (*LLMResponse, error)
ChatWithToolsCached(ctx, systemPrompt, messages, tools, cacheConfig) (*LLMResponse, error)
ChatWithUsage(ctx, systemPrompt, userMessage) (*ChatResponse, error)
```

Types:
```go
type ChatResponse struct {
    Text  string
    Usage domain.LLMUsage
}

type CacheConfig struct {
    CacheTools        bool // cache tool definitions
    CacheSystem       bool // cache system prompt
    CacheConversation bool // cache conversation history
}
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

### CatalogPort
```go
GetTenantBySlug(ctx, slug) (*Tenant, error)
GetCategories(ctx) ([]Category, error)
GetMasterProduct(ctx, id) (*MasterProduct, error)
ListProducts(ctx, tenantID, filter) ([]Product, int, error)
GetProduct(ctx, tenantID, productID) (*Product, error)
```

### StatePort
```go
CreateState(ctx, sessionID) (*SessionState, error)
GetState(ctx, sessionID) (*SessionState, error)
UpdateState(ctx, state) error
AddDelta(ctx, sessionID, delta) (int, error) // step auto-assigned
GetDeltas(ctx, sessionID) ([]Delta, error)
GetDeltasSince(ctx, sessionID, fromStep) ([]Delta, error)
GetDeltasUntil(ctx, sessionID, toStep) ([]Delta, error)
PushView(ctx, sessionID, snapshot) error
PopView(ctx, sessionID) (*ViewSnapshot, error)
GetViewStack(ctx, sessionID) ([]ViewSnapshot, error)
```

## Правила

- Только интерфейсы, никакой реализации
- Импорты только из `domain/`
- Адаптеры реализуют эти интерфейсы
