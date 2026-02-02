# Updates

Лог изменений проекта Keepstar.

---

## 2026-02-02

### Two-Agent Pipeline - Frontend Rendering (Phase 4)
- FormationRenderer с режимами grid/carousel/single/list
- AtomRenderer со стилями для всех типов (text, number, price, image, rating, badge, button, icon, divider, progress)
- WidgetRenderer с размерами (tiny/small/medium/large)
- MessageBubble с поддержкой Formation (backward compatible)
- Pipeline API: `POST /api/v1/pipeline`
- Debug Console: `/debug/session/` с детальными метриками
- Метрики: время LLM/tool, токены in/out, стоимость USD, промпты, responses

### Two-Agent Pipeline - Template Builder (Phase 3)
- Agent2ExecuteUseCase: meta → LLM → FormationTemplate
- Agent2SystemPrompt с правилами выбора mode/size
- BuildAgent2Prompt() для генерации промпта из StateMeta
- PipelineExecuteUseCase: Agent 1 → Agent 2 → ApplyTemplate
- ApplyTemplate: шаблон + products → FormationWithData с widgets

### Two-Agent Pipeline - Tool Caller (Phase 2)
- Agent1ExecuteUseCase: query → LLM → tool call → state update
- Agent1SystemPrompt с правилами tool calling
- Tool Registry с search_products tool
- ChatWithTools в LLMPort для tool calling
- Delta creation и сохранение в state

### Two-Agent Pipeline - State Storage (Phase 1)
- StatePort interface для session state
- PostgreSQL adapter с JSONB storage
- Domain entities: SessionState, Delta, StateMeta, StateData
- Миграции для chat_session_state, chat_session_deltas
- Delta-based state management

### Multi-tenant Product Catalog
- Добавлены domain entities: Tenant, Category, MasterProduct
- Расширен Product с tenantId, masterProductId, priceFormatted
- Создан CatalogPort interface для операций с каталогом
- Реализован PostgreSQL adapter с merging master/tenant данных
- Добавлены миграции для catalog schema (tenants, categories, master_products, products)
- Seed data: Nike, Sportmaster + 8 кроссовок
- Use cases: ListProducts, GetProduct
- HTTP handlers с TenantMiddleware для резолва slug
- Frontend: getProducts(), getProduct() в apiClient
- Frontend: ProductGrid компонент с WidgetRenderer

### API Endpoints
```
GET /api/v1/tenants/{slug}/products
GET /api/v1/tenants/{slug}/products/{id}
```

---

## 2026-02-01

### Neon PostgreSQL Integration
- Добавлен PostgreSQL adapter (pgxpool)
- Реализован CachePort для сессий и сообщений
- Реализован EventPort для аналитики
- Auto-migrations для chat таблиц (users, sessions, messages, events)
- Session TTL 10 минут (sliding window)
- Graceful degradation если DATABASE_URL не задан

### Chat Hexagonal Migration
- Перенесён chat на hexagonal architecture
- SendMessageUseCase с persistence
- ChatHandler, SessionHandler
- Frontend: session persistence в localStorage
- Frontend: восстановление истории при загрузке

---

## 2025-01-29

### Архитектура
- Создана hexagonal структура backend (internal/domain, ports, adapters, usecases, handlers, prompts)
- Создана feature-sliced структура frontend (shared, entities, features, app)
- Старый рабочий код сохранён (main.go, App.jsx, Chat.jsx)
- Новая структура в stubs, готова к миграции

### Expert System
- Заполнены expertise.yaml для backend и frontend
- Обновлены self-improve.md с валидацией YAML и лимитами строк
- Обновлены question.md с примерами и контекстом
- Добавлен README для expert system (ACT → LEARN → REUSE)

### Инструменты
- Добавлен dev-inspector для отладки UI элементов

### Документация
- Создан драфт Product Manifesto (AI_docs/Manifesto)

---
