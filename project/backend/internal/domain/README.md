# Domain

Чистые бизнес-сущности. НОЛЬ зависимостей из проекта.

## Файлы

### UI Primitives
- `atom_entity.go` — Atom, AtomType (базовый элемент UI)
- `display_entity.go` — AtomDisplay, DisplayStyle (визуальное форматирование атомов)
- `widget_entity.go` — Widget, WidgetType (композиция атомов)
- `formation_entity.go` — Formation (layout виджетов)

### Chat
- `message_entity.go` — Message (сообщение в чате, поля: SentAt, ReceivedAt, Timestamp)
- `session_entity.go` — Session (сессия пользователя, поля: CreatedAt, UpdatedAt), SessionTTL (5 min sliding expiration)
- `user_entity.go` — ChatUser (пользователь чата)
- `event_entity.go` — ChatEvent (события аналитики)

### Catalog
- `entity_type.go` — EntityType (product, service)
- `product_entity.go` — Product (товар с tenant context)
- `service_entity.go` — Service (услуга с tenant context)
- `tenant_entity.go` — Tenant (бренд/ритейлер/реселлер)
- `category_entity.go` — Category (категория товаров)
- `master_product_entity.go` — MasterProduct (канонический товар)

### Pipeline
- `state_entity.go` — SessionState, Delta, DeltaInfo, StateData, ViewState, ViewSnapshot (state для pipeline). Delta.TurnID для группировки дельт по Turn'ам. DeltaInfo — лёгкая структура для zone-write, конвертируется в Delta через ToDelta(). SessionState содержит ConversationHistory для prompt caching
- `tool_entity.go` — ToolDefinition, ToolCall, LLMMessage, LLMResponse, LLMUsage (с cache полями: CacheCreationInputTokens, CacheReadInputTokens). CalculateCost() учитывает cache pricing
- `template_entity.go` — FormationTemplate, FormationWithData
- `preset_entity.go` — Preset, FieldConfig, SlotConfig (пресеты рендеринга)

### Tracing
- `trace_entity.go` — PipelineTrace (incl. Spans []Span), AgentTrace, StateSnapshot, DeltaTrace, FormationTrace (трейсинг pipeline)
- `span.go` — Span, SpanCollector (thread-safe timed span collector для waterfall визуализации). Context helpers: WithSpanCollector, SpanFromContext, WithStage, StageFromContext. Имена span'ов используют dot-separated иерархию: `pipeline`, `agent1.llm.ttfb`, `agent1.tool.embed`

### Errors
- `domain_errors.go` — Доменные ошибки

## Правила

- Никаких импортов из `internal/`
- Только стандартная библиотека Go
- Структуры + конструкторы + методы валидации
