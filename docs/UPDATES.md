# Updates

Лог изменений проекта Keepstar.

---

## 2026-02-02

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
