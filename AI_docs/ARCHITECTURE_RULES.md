# Keepstar Architecture Rules

> Этот документ — источник истины для архитектурных решений.
> Агент ОБЯЗАН прочитать его перед началом работы и сверяться при code review.

---

## 1. Философия

```
Код пишется для агентов, не только для людей.
Агент должен понять что делает файл за 10 секунд.
Агент должен изменить файл не сломав остальное.
```

**Главный принцип**: Изоляция > Переиспользование > DRY

---

## 2. Структура проекта

### Backend (Go)

```
api/
├── cmd/server/main.go          # Только bootstrap
├── internal/
│   ├── domain/                 # Чистые структуры, НОЛЬ зависимостей
│   │   ├── widget_entity.go    # type Widget, type WidgetType
│   │   ├── atom_entity.go      # type Atom, type AtomType  
│   │   ├── layout_entity.go    # type Layout, type Formation
│   │   ├── tenant_entity.go    # type Tenant, type TenantConfig
│   │   ├── session_entity.go   # type Session
│   │   └── domain_errors.go    # Доменные ошибки
│   │
│   ├── ports/                  # Интерфейсы (контракты)
│   │   ├── llm_port.go         # type LLMPort interface
│   │   ├── storage_port.go     # type StoragePort interface
│   │   └── cache_port.go       # type CachePort interface
│   │
│   ├── adapters/               # Реализации портов
│   │   ├── anthropic/
│   │   │   ├── anthropic_client.go
│   │   │   └── README.md
│   │   ├── postgres/
│   │   │   ├── postgres_tenant_repo.go
│   │   │   ├── postgres_session_repo.go
│   │   │   └── README.md
│   │   └── memory/
│   │       └── memory_cache.go
│   │
│   ├── usecases/               # Бизнес-логика, 1 файл = 1 use case
│   │   ├── chat_analyze_query.go       # Stage 1: Query → Intent
│   │   ├── chat_compose_widgets.go     # Stage 2: Data → Widgets
│   │   ├── viewport_get_widgets.go     # Lazy load виджетов
│   │   ├── action_add_to_cart.go       # Add to cart action
│   │   ├── action_toggle_like.go       # Like action
│   │   └── README.md
│   │
│   ├── handlers/               # HTTP слой — ТОЛЬКО parse/validate/respond
│   │   ├── handler_chat.go     # POST /api/v1/chat
│   │   ├── handler_config.go   # GET /api/v1/config/:tenantId
│   │   ├── handler_viewport.go # GET /api/v1/widgets/viewport
│   │   ├── handler_action.go   # POST /api/v1/action
│   │   ├── handler_health.go   # GET /health, GET /ready
│   │   ├── middleware_auth.go
│   │   ├── middleware_cors.go
│   │   ├── middleware_logging.go
│   │   └── README.md
│   │
│   ├── prompts/                # LLM промпты — ОТДЕЛЬНО от кода
│   │   ├── prompt_analyze_query.go     # Промпт для Stage 1
│   │   ├── prompt_compose_widgets.go   # Промпт для Stage 2
│   │   └── README.md
│   │
│   ├── logger/                 # Логирование — методы, не inline
│   │   ├── logger.go           # Основной логгер
│   │   ├── logger_chat.go      # log.ChatStarted(), log.ChatCompleted()
│   │   ├── logger_llm.go       # log.LLMRequest(), log.LLMResponse()
│   │   ├── logger_http.go      # log.HTTPRequest(), log.HTTPResponse()
│   │   └── README.md
│   │
│   └── config/
│       └── config.go           # Env vars, defaults
│
├── pkg/                        # Shared utilities
│   ├── response/
│   │   └── json_response.go
│   └── validate/
│       └── input_validate.go
│
├── go.mod
└── go.sum
```

### Frontend (React)

```
widget/src/
├── shared/                     # Переиспользуемое везде
│   ├── api/
│   │   ├── apiClient.ts        # Базовый HTTP client
│   │   ├── apiTypes.ts         # API response types
│   │   └── README.md
│   ├── ui/                     # shadcn components
│   │   └── Button.tsx
│   ├── hooks/
│   │   ├── useApiRequest.ts
│   │   └── useThemeConfig.ts
│   ├── logger/                 # Логирование — методы
│   │   ├── logger.ts           # Основной логгер
│   │   ├── loggerChat.ts       # logChatMessage(), logChatError()
│   │   ├── loggerWidget.ts     # logWidgetRender(), logWidgetAction()
│   │   └── README.md
│   └── lib/
│       └── utils.ts
│
├── entities/                   # Бизнес-сущности
│   ├── widget/
│   │   ├── widgetModel.ts
│   │   ├── WidgetRenderer.tsx
│   │   └── README.md
│   ├── atom/
│   │   ├── atomModel.ts
│   │   ├── AtomRenderer.tsx
│   │   ├── atoms/
│   │   │   ├── AtomText.tsx
│   │   │   ├── AtomNumber.tsx
│   │   │   ├── AtomImage.tsx
│   │   │   ├── AtomButton.tsx
│   │   │   ├── AtomRating.tsx
│   │   │   ├── AtomBadge.tsx
│   │   │   ├── AtomIcon.tsx
│   │   │   ├── AtomInput.tsx
│   │   │   ├── AtomDivider.tsx
│   │   │   └── AtomProgress.tsx
│   │   └── README.md
│   └── message/
│       ├── messageModel.ts
│       └── MessageBubble.tsx
│
├── features/                   # Фичи
│   ├── chat/
│   │   ├── chatModel.ts
│   │   ├── useChatMessages.ts
│   │   ├── useChatSubmit.ts
│   │   ├── ChatPanel.tsx
│   │   ├── ChatInput.tsx
│   │   ├── ChatHistory.tsx
│   │   └── README.md
│   ├── canvas/
│   │   ├── canvasModel.ts
│   │   ├── useCanvasViewport.ts
│   │   ├── useCanvasScroll.ts
│   │   ├── WidgetCanvas.tsx
│   │   ├── WidgetGrid.tsx
│   │   └── README.md
│   └── overlay/
│       ├── useOverlayState.ts
│       ├── FullscreenOverlay.tsx
│       ├── OverlayBackdrop.tsx
│       └── README.md
│
├── app/
│   ├── App.tsx
│   ├── AppProviders.tsx
│   └── appTypes.ts
│
├── styles/
│   └── globals.css
│
└── main.tsx
```

---

## 3. Правила именования

### КРИТИЧНО: Уникальность имён

```
ЗАПРЕЩЕНО повторять имена файлов, функций, переменных в проекте.
Перед созданием — проверь что такого имени ещё нет.
```

**Стратегия — контекстные префиксы:**

| Слой | Префикс | Пример |
|------|---------|--------|
| Handlers | `handler_` | `handler_chat.go`, `handler_config.go` |
| Usecases | `{domain}_` | `chat_analyze_query.go`, `action_add_to_cart.go` |
| Adapters | `{tech}_` | `postgres_tenant_repo.go`, `anthropic_client.go` |
| Prompts | `prompt_` | `prompt_analyze_query.go` |
| Logger | `logger_` | `logger_chat.go`, `logger_llm.go` |
| Domain | `{entity}_entity.go` | `widget_entity.go` |
| Ports | `{name}_port.go` | `llm_port.go` |
| React atoms | `Atom` | `AtomText.tsx`, `AtomRating.tsx` |
| React hooks | `use{Feature}{Action}` | `useChatSubmit.ts`, `useCanvasViewport.ts` |

### Файлы

| Что | Формат | Пример |
|-----|--------|--------|
| Go файл | `prefix_snake_case.go` | `chat_analyze_query.go` |
| Go тест | `prefix_snake_case_test.go` | `chat_analyze_query_test.go` |
| React компонент | `PrefixPascalCase.tsx` | `AtomRating.tsx` |
| React hook | `use{Feature}{Action}.ts` | `useChatSubmit.ts` |
| Types/Models | `{feature}Model.ts` | `chatModel.ts` |

### Функции и методы

| Язык | Публичные | Приватные |
|------|-----------|-----------|
| Go | `PascalCase` | `camelCase` |
| TS/React | `camelCase` / `PascalCase` (компоненты) | не экспортировать |

### Переменные

```go
// Go: существительные для данных, глаголы для функций
tenant := GetTenantByID(id)      // ✅
t := GetTenant(id)               // ❌ непонятно что это

widgets := ComposeProductWidgets(query)  // ✅
result := Do(query)                      // ❌ Do что?
```

```typescript
// React: существительные для state, handlers с префиксом handle/on
const [chatMessages, setChatMessages] = useState([])     // ✅
const [msgs, setMsgs] = useState([])                     // ❌

const handleChatSubmit = () => {}    // ✅
const submit = () => {}              // ❌
```

---

## 4. Правила размера

| Метрика | Лимит | Что делать при превышении |
|---------|-------|---------------------------|
| Строк в файле | **1000 max** | Отметить для рефакторинга (решение вручную) |
| Параметров функции | **5 max** | Создать struct для параметров |
| Вложенность (if/for) | **3 уровня max** | Early return, выделить функцию |
| Импортов | **20 max** | Файл делает слишком много |

**Нет жёстких лимитов на длину функций** — функция может быть длинной если это оправдано логикой.

---

## 5. Правила зависимостей

### Go: Направление зависимостей

```
handlers → usecases → ports ← adapters
                ↓
             domain
```

- `domain/` — НОЛЬ импортов из проекта
- `ports/` — только `domain/`
- `usecases/` — только `domain/` и `ports/`
- `adapters/` — `domain/`, `ports/`, внешние библиотеки
- `handlers/` — всё выше + http библиотеки
- `prompts/` — только `domain/` (для типов)
- `logger/` — ноль импортов из проекта

**Запрещено:**
- `domain/` импортит что угодно из `internal/`
- `usecases/` импортит `adapters/` напрямую
- Циклические импорты

### React: Направление зависимостей

```
app → features → entities → shared
```

- `shared/` — ноль импортов из проекта (только внешние)
- `entities/` — только `shared/`
- `features/` — `shared/` и `entities/`
- `app/` — всё

---

## 6. Промпты (LLM)

**Промпты ВСЕГДА лежат отдельно от бизнес-логики.**

```
internal/prompts/
├── prompt_analyze_query.go     # Stage 1 промпт
├── prompt_compose_widgets.go   # Stage 2 промпт
└── README.md
```

### Структура файла промпта

```go
// prompt_analyze_query.go
package prompts

const AnalyzeQuerySystemPrompt = `
You are a query analyzer for an e-commerce chat widget.
...
`

const AnalyzeQueryUserTemplate = `
User query: {{.Query}}
Tenant context: {{.TenantContext}}
...
`

// Функция для сборки промпта
func BuildAnalyzeQueryPrompt(query string, ctx TenantContext) string {
    // ...
}
```

### Правила для промптов

1. Один файл = один промпт (или связанная группа)
2. System prompt и User template раздельно
3. Функция-билдер для подстановки переменных
4. Версионирование в комментариях

---

## 7. Логирование

**Логи — это методы, не inline код.**

```
internal/logger/
├── logger.go           # Базовый логгер, уровни, формат
├── logger_chat.go      # Chat-специфичные логи
├── logger_llm.go       # LLM-специфичные логи
├── logger_http.go      # HTTP-специфичные логи
├── logger_widget.go    # Widget-специфичные логи
└── README.md
```

### Структура

```go
// logger/logger.go
package logger

type Logger struct {
    // ...
}

func New(config Config) *Logger { ... }

// logger/logger_chat.go
func (l *Logger) ChatSessionStarted(sessionID, tenantID string) {
    l.Info("chat_session_started", 
        "session_id", sessionID,
        "tenant_id", tenantID,
    )
}

func (l *Logger) ChatMessageReceived(sessionID, message string) {
    l.Debug("chat_message_received",
        "session_id", sessionID,
        "message_length", len(message),
    )
}

func (l *Logger) ChatResponseSent(sessionID string, widgetCount int, durationMs int64) {
    l.Info("chat_response_sent",
        "session_id", sessionID,
        "widget_count", widgetCount,
        "duration_ms", durationMs,
    )
}

// logger/logger_llm.go
func (l *Logger) LLMRequestStarted(stage string, tokenEstimate int) { ... }
func (l *Logger) LLMResponseReceived(stage string, tokens int, durationMs int64) { ... }
func (l *Logger) LLMError(stage string, err error) { ... }
```

### Использование в коде

```go
// usecases/chat_analyze_query.go
func (uc *AnalyzeQueryUseCase) Execute(ctx context.Context, req Request) (*Result, error) {
    uc.log.ChatMessageReceived(req.SessionID, req.Message)
    
    uc.log.LLMRequestStarted("analyze", estimateTokens(req))
    
    result, err := uc.llm.AnalyzeQuery(ctx, req)
    if err != nil {
        uc.log.LLMError("analyze", err)
        return nil, err
    }
    
    uc.log.LLMResponseReceived("analyze", result.Tokens, result.Duration)
    
    return result, nil
}
```

### Правила логирования

1. **Каждое действие логируется** — начало, конец, ошибка
2. **Структурированные логи** — key-value, не строки
3. **Уровни**: Debug (детали), Info (события), Warn (проблемы), Error (ошибки)
4. **Никогда не логировать PII** — пароли, токены, персональные данные

---

## 8. Error Handling

### Go: Стратегия ошибок

```go
// domain/domain_errors.go
package domain

type Error struct {
    Code    string // машиночитаемый код
    Message string // человекочитаемое сообщение
    Err     error  // wrapped error
}

func (e *Error) Error() string { return e.Message }
func (e *Error) Unwrap() error { return e.Err }

// Предопределённые ошибки
var (
    ErrTenantNotFound    = &Error{Code: "TENANT_NOT_FOUND", Message: "Tenant not found"}
    ErrSessionExpired    = &Error{Code: "SESSION_EXPIRED", Message: "Session expired"}
    ErrInvalidQuery      = &Error{Code: "INVALID_QUERY", Message: "Invalid query"}
    ErrLLMUnavailable    = &Error{Code: "LLM_UNAVAILABLE", Message: "AI service unavailable"}
    ErrRateLimitExceeded = &Error{Code: "RATE_LIMIT", Message: "Rate limit exceeded"}
)
```

### Wrapping ошибок

```go
// В adapters — wrap с контекстом
func (r *TenantRepo) GetByID(ctx context.Context, id string) (*domain.Tenant, error) {
    tenant, err := r.db.Query(...)
    if err != nil {
        if errors.Is(err, sql.ErrNoRows) {
            return nil, domain.ErrTenantNotFound
        }
        return nil, fmt.Errorf("postgres get tenant: %w", err)
    }
    return tenant, nil
}

// В usecases — бизнес-контекст
func (uc *UseCase) Execute(...) error {
    tenant, err := uc.repo.GetByID(ctx, id)
    if err != nil {
        return fmt.Errorf("get tenant for chat: %w", err)
    }
    // ...
}

// В handlers — логируем и возвращаем клиенту
func (h *Handler) Chat(w http.ResponseWriter, r *http.Request) {
    result, err := h.usecase.Execute(...)
    if err != nil {
        h.log.HTTPError(r, err)
        
        // Определяем что вернуть клиенту
        var domainErr *domain.Error
        if errors.As(err, &domainErr) {
            respondError(w, domainErr.Code, domainErr.Message, statusCode(domainErr))
        } else {
            respondError(w, "INTERNAL_ERROR", "Something went wrong", 500)
        }
        return
    }
    // ...
}
```

### Что логируем vs что возвращаем

| Ошибка | Логируем | Возвращаем клиенту |
|--------|----------|-------------------|
| Валидация | Debug | Детально (какое поле) |
| Бизнес-логика | Info | Понятное сообщение |
| Внешний сервис | Warn | "Service unavailable" |
| Паника/баг | Error + stack | "Internal error" |

---

## 9. Конфигурация и секреты

### Структура конфига

```go
// internal/config/config.go
package config

type Config struct {
    Server   ServerConfig
    Database DatabaseConfig
    LLM      LLMConfig
    Logging  LoggingConfig
}

type ServerConfig struct {
    Port         int    `env:"PORT" default:"8080"`
    Environment  string `env:"ENVIRONMENT" default:"development"`
}

type DatabaseConfig struct {
    URL          string `env:"DATABASE_URL" required:"true"`
    MaxConns     int    `env:"DB_MAX_CONNS" default:"10"`
}

type LLMConfig struct {
    APIKey       string `env:"ANTHROPIC_API_KEY" required:"true"`
    Model        string `env:"LLM_MODEL" default:"claude-3-haiku-20240307"`
    MaxTokens    int    `env:"LLM_MAX_TOKENS" default:"4096"`
}

type LoggingConfig struct {
    Level        string `env:"LOG_LEVEL" default:"info"`
    Format       string `env:"LOG_FORMAT" default:"json"`
}

func Load() (*Config, error) {
    // Load from env with defaults
}
```

### .env файлы

```bash
# .env.example (в репо)
PORT=8080
ENVIRONMENT=development
DATABASE_URL=postgres://user:pass@localhost:5432/keepstar
ANTHROPIC_API_KEY=sk-ant-xxx
LOG_LEVEL=debug

# .env.local (НЕ в репо, в .gitignore)
ANTHROPIC_API_KEY=sk-ant-real-key-here
DATABASE_URL=postgres://real-connection
```

### Правила конфигов

1. **Все секреты через env vars** — никогда в коде
2. **Defaults для development** — должно работать из коробки
3. **Required для production** — без API key не стартует
4. **.env.example в репо** — документация какие переменные нужны
5. **.env.local в .gitignore** — реальные секреты

---

## 10. API Versioning

### URL структура

```
/api/v1/chat
/api/v1/config/:tenantId
/api/v1/widgets/viewport
/api/v1/action

/health    # Без версии — инфраструктурный
/ready     # Без версии — инфраструктурный
```

### Версионирование

```go
// handlers/routes.go
func SetupRoutes(r chi.Router, h *Handlers) {
    // Health checks — без версии
    r.Get("/health", h.Health)
    r.Get("/ready", h.Ready)
    
    // API v1
    r.Route("/api/v1", func(r chi.Router) {
        r.Post("/chat", h.Chat)
        r.Get("/config/{tenantId}", h.Config)
        r.Get("/widgets/viewport", h.Viewport)
        r.Post("/action", h.Action)
    })
    
    // Когда понадобится v2
    // r.Route("/api/v2", func(r chi.Router) { ... })
}
```

### Health Checks

```go
// handlers/handler_health.go

// GET /health — жив ли сервис
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
    respondJSON(w, map[string]string{"status": "ok"})
}

// GET /ready — готов ли принимать трафик
func (h *Handler) Ready(w http.ResponseWriter, r *http.Request) {
    // Проверяем зависимости
    if err := h.db.Ping(r.Context()); err != nil {
        h.log.HealthCheckFailed("database", err)
        respondError(w, "NOT_READY", "Database unavailable", 503)
        return
    }
    
    respondJSON(w, map[string]string{"status": "ready"})
}
```

---

## 11. Request/Response типы

### Где лежат

```
internal/
├── domain/           # Бизнес-сущности
│   └── widget_entity.go
│
├── handlers/
│   ├── handler_chat.go
│   └── handler_chat_dto.go    # Request/Response для этого handler
```

### Именование

```go
// handler_chat_dto.go
package handlers

// Request
type ChatRequest struct {
    TenantID  string         `json:"tenantId" validate:"required"`
    SessionID string         `json:"sessionId"`
    Message   string         `json:"message" validate:"required,max=1000"`
    Viewport  ViewportParams `json:"viewport"`
}

// Response
type ChatResponse struct {
    SessionID string         `json:"sessionId"`
    Response  ChatContent    `json:"response"`
    Cost      CostInfo       `json:"cost,omitempty"`
}

// Nested types
type ChatContent struct {
    Text         string   `json:"text"`
    TotalWidgets int      `json:"totalWidgets"`
    Layout       Layout   `json:"layout"`
    Widgets      []Widget `json:"widgets"`
}
```

### Правила

1. **Request/Response рядом с handler** — не в отдельной папке
2. **Суффиксы `Request`, `Response`** — однозначно понятно
3. **Validate tags** — валидация на уровне парсинга
4. **omitempty для optional** — чистый JSON

---

## 12. Тестирование

### Структура тестов

```
internal/
├── usecases/
│   ├── chat_analyze_query.go
│   └── chat_analyze_query_test.go    # Рядом с кодом
│
├── adapters/
│   ├── postgres/
│   │   ├── postgres_tenant_repo.go
│   │   └── postgres_tenant_repo_test.go
│   └── anthropic/
│       ├── anthropic_client.go
│       └── anthropic_client_test.go
│
├── handlers/
│   ├── handler_chat.go
│   └── handler_chat_test.go
```

### Типы тестов

| Тип | Что тестируем | Моки |
|-----|---------------|------|
| Unit | usecases, domain | Все зависимости |
| Integration | adapters | Реальная БД (testcontainers) |
| E2E | handlers | Минимум, реальный flow |

### Пример unit теста

```go
// usecases/chat_analyze_query_test.go
func TestAnalyzeQuery_ValidQuery(t *testing.T) {
    // Arrange
    mockLLM := &mocks.LLMPort{}
    mockLLM.On("AnalyzeQuery", mock.Anything, mock.Anything).Return(&AnalyzeResult{
        Intent: "product_search",
        Entities: []string{"nike", "shoes"},
    }, nil)
    
    uc := NewAnalyzeQueryUseCase(mockLLM, logger.NewNoop())
    
    // Act
    result, err := uc.Execute(context.Background(), Request{
        Message: "покажи кроссовки Nike",
    })
    
    // Assert
    assert.NoError(t, err)
    assert.Equal(t, "product_search", result.Intent)
    mockLLM.AssertExpectations(t)
}
```

### Правила тестов

1. **Тест рядом с кодом** — `file.go` + `file_test.go`
2. **Table-driven tests** — для множества кейсов
3. **Моки через интерфейсы** — ports делают это простым
4. **Понятные имена** — `Test{Function}_{Scenario}`

---

## 13. README в каждой папке

Каждая папка с кодом ДОЛЖНА содержать README.md:

```markdown
# {Название папки}

## Что это
{1-2 предложения}

## Файлы
- `file1.go` — {что делает}
- `file2.go` — {что делает}

## Зависимости
- Импортит: {список}
- Импортится из: {список}

## Как добавить новое
{Краткая инструкция}
```

---

## 14. Чеклист перед коммитом

### Архитектура
- [ ] Файл < 1000 строк? (если больше — отметить для рефакторинга)
- [ ] Нет циклических зависимостей?
- [ ] Domain не импортит ничего из internal?
- [ ] Новый файл в правильной папке?

### Именование  
- [ ] Имя файла УНИКАЛЬНО в проекте?
- [ ] Используется контекстный префикс?
- [ ] Имена функций/переменных не повторяют существующие?

### Промпты
- [ ] Промпты в отдельных файлах в `prompts/`?
- [ ] Не захардкожены в usecases/adapters?

### Логирование
- [ ] Все действия логируются через методы logger?
- [ ] Нет inline логов типа `fmt.Println`?
- [ ] Нет PII в логах?

### Ошибки
- [ ] Ошибки wrapped с контекстом?
- [ ] Domain errors используются для бизнес-ошибок?

### Тесты
- [ ] Тест рядом с кодом (`_test.go`)?
- [ ] Моки через интерфейсы?

### Контракты
- [ ] Новый adapter реализует порт?
- [ ] Request/Response типы рядом с handler?

### Документация
- [ ] README папки обновлён?

---

## 15. Anti-patterns (ЗАПРЕЩЕНО)

### Промпты в коде

```go
// ❌ Промпт прямо в use case
func (uc *UseCase) Execute(...) {
    prompt := "You are a helpful assistant..."
    result := uc.llm.Call(prompt)
}

// ✅ Промпт из отдельного файла
func (uc *UseCase) Execute(...) {
    prompt := prompts.BuildAnalyzeQueryPrompt(query, ctx)
    result := uc.llm.Call(prompt)
}
```

### Inline логи

```go
// ❌ Логи прямо в коде
func (uc *UseCase) Execute(...) {
    fmt.Println("Starting analysis...")
    log.Printf("Query: %s", query)
    slog.Info("LLM response", "tokens", tokens)
}

// ✅ Логи через методы
func (uc *UseCase) Execute(...) {
    uc.log.ChatMessageReceived(sessionID, message)
    uc.log.LLMRequestStarted("analyze", tokenEstimate)
    uc.log.LLMResponseReceived("analyze", tokens, duration)
}
```

### Повторяющиеся имена

```go
// ❌ Неуникальные имена
usecases/analyze.go
adapters/analyze.go
handlers/analyze.go

// ✅ Уникальные с префиксами
usecases/chat_analyze_query.go
adapters/anthropic_client.go
handlers/handler_chat.go
```

### God object

```go
// ❌ Всё в одном сервисе
type Service struct {
    db, llm, cache, logger, config, validator...
}

// ✅ Отдельные use cases
type ChatAnalyzeQueryUseCase struct {
    llm ports.LLMPort
    log *logger.Logger
}
```

---

## 16. Как агенту использовать этот документ

### Перед началом задачи

1. Прочитать этот файл
2. Определить какие папки/файлы будут затронуты
3. **Проверить что новые имена уникальны** (grep по проекту)
4. Проверить README этих папок

### Во время работы

1. Создавать файлы в правильных папках с правильными префиксами
2. Промпты — только в `prompts/`
3. Логи — только через методы `logger/`
4. Следить за размером файлов

### После завершения

1. Пройти чеклист из раздела 14
2. Обновить README затронутых папок
3. Убедиться что тесты рядом с кодом

---

## Версия документа

- **v1.0** — Initial version
- **v2.0** — Добавлены: уникальные имена, промпты отдельно, логирование методами, error handling, конфиги, API versioning, тесты
