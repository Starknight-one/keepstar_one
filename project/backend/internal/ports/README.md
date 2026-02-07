# Ports

Интерфейсы (контракты) для внешних зависимостей.

## Файлы

- `llm_port.go` — LLMPort interface (для AI провайдеров)
- `cache_port.go` — CachePort interface (для сессий и кэширования)
- `event_port.go` — EventPort interface (для аналитики событий)
- `catalog_port.go` — CatalogPort interface (для каталога товаров + vector search)
- `state_port.go` — StatePort interface (для session state)
- `trace_port.go` — TracePort interface (для pipeline трейсинга)
- `embedding_port.go` — EmbeddingPort interface (для генерации vector embeddings)

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
DeleteSession(ctx, id) error // removes session and all related data
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

// Vector search (pgvector)
VectorSearch(ctx, tenantID, embedding []float32, limit) ([]Product, error)
SeedEmbedding(ctx, masterProductID, embedding []float32) error
GetMasterProductsWithoutEmbedding(ctx) ([]MasterProduct, error)
```

Types:
```go
type ProductFilter struct {
    CategoryID   string
    CategoryName string            // ILIKE matching (agent passes name, not UUID)
    Brand        string
    MinPrice     int
    MaxPrice     int
    Search       string
    SortField    string            // "price", "rating", "name"
    SortOrder    string            // "asc", "desc"
    Limit        int
    Offset       int
    Attributes   map[string]string // JSONB attribute filters (key → ILIKE value)
}
```

### TracePort
```go
Record(ctx, trace *PipelineTrace) error  // saves trace to DB + console
List(ctx, limit) ([]*PipelineTrace, error)
Get(ctx, traceID) (*PipelineTrace, error)
```

### EmbeddingPort
```go
Embed(ctx, texts []string) ([][]float32, error) // generates vector embeddings
```

### StatePort
```go
CreateState(ctx, sessionID) (*SessionState, error)
GetState(ctx, sessionID) (*SessionState, error)
UpdateState(ctx, state) error
AddDelta(ctx, sessionID, delta) (int, error) // step auto-assigned

// Zone writes — atomically update zone + create delta
UpdateData(ctx, sessionID, data, meta, info) (int, error)
UpdateTemplate(ctx, sessionID, template, info) (int, error)
UpdateView(ctx, sessionID, view, stack, info) (int, error)

// Append-only, no delta (for LLM cache continuity)
AppendConversation(ctx, sessionID, messages) error

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
