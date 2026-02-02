# Domain

Чистые бизнес-сущности. НОЛЬ зависимостей из проекта.

## Файлы

- `atom_entity.go` — Atom, AtomType (базовый элемент UI)
- `widget_entity.go` — Widget, WidgetType (композиция атомов)
- `formation_entity.go` — Formation (layout виджетов)
- `message_entity.go` — Message (сообщение в чате)
- `session_entity.go` — Session (сессия пользователя)
- `user_entity.go` — ChatUser (пользователь чата)
- `event_entity.go` — ChatEvent (события аналитики)
- `product_entity.go` — Product (товар с tenant context)
- `tenant_entity.go` — Tenant (бренд/ритейлер/реселлер)
- `category_entity.go` — Category (категория товаров)
- `master_product_entity.go` — MasterProduct (канонический товар)
- `state_entity.go` — SessionState, Delta (state для two-agent pipeline)
- `tool_entity.go` — ToolDefinition, ToolCall, LLMMessage, LLMResponse (LLM tool calling)
- `template_entity.go` — FormationTemplate, FormationWithData (шаблоны Agent 2)
- `domain_errors.go` — Доменные ошибки

## Правила

- Никаких импортов из `internal/`
- Только стандартная библиотека Go
- Структуры + конструкторы + методы валидации
