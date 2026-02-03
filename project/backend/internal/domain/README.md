# Domain

Чистые бизнес-сущности. НОЛЬ зависимостей из проекта.

## Файлы

### UI Primitives
- `atom_entity.go` — Atom, AtomType (базовый элемент UI)
- `widget_entity.go` — Widget, WidgetType (композиция атомов)
- `formation_entity.go` — Formation (layout виджетов)

### Chat
- `message_entity.go` — Message (сообщение в чате)
- `session_entity.go` — Session (сессия пользователя), SessionTTL (5 min sliding expiration)
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
- `state_entity.go` — SessionState, Delta, StateData (state для pipeline)
- `tool_entity.go` — ToolDefinition, ToolCall, LLMMessage, LLMResponse
- `template_entity.go` — FormationTemplate, FormationWithData
- `preset_entity.go` — Preset, FieldConfig, SlotConfig (пресеты рендеринга)

### Errors
- `domain_errors.go` — Доменные ошибки

## Правила

- Никаких импортов из `internal/`
- Только стандартная библиотека Go
- Структуры + конструкторы + методы валидации
