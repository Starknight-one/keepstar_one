# Keepstar — System Reference

> Полная карта: где что лежит, как выглядят данные, кто читает, кто пишет.

---

## 1. Где физически БД

**Neon PostgreSQL** (serverless, AWS). Одна база, два схемы:
- `public` — чат, сессии, стейт, дельты, трейсы, логи
- `catalog` — продукты, сервисы, тенанты, ингредиенты

**Подключение**: `DATABASE_URL` из `project/.env`
**Драйвер**: pgx v5 (Go), connection pool (max 10 connections)
**Векторный поиск**: pgvector, HNSW индекс, 384 dimensions (OpenAI embeddings)

---

## 2. Все таблицы

### public (чат)

| Таблица | Что хранит | Ключевые поля |
|---------|-----------|---------------|
| `chat_sessions` | Сессии чата | id, tenant_id, status (active/closed), last_activity_at |
| `chat_users` | Посетители | id, tenant_id, fingerprint, ip_address |
| `chat_messages` | Сообщения чата | session_id, role (user/assistant), content, formation (JSONB) |
| `chat_events` | Клики, открытия | session_id, event_type, event_data |
| `chat_session_state` | **Стейт сессии** (1 запись на сессию) | session_id (UNIQUE), current_data, current_meta, current_template, view_mode, conversation_history, agent2_history, step |
| `chat_session_deltas` | **История изменений** (append-only) | session_id, step (auto), trigger, source, actor_id, delta_type, path, action, result, turn_id |
| `pipeline_traces` | Трейсы пайплайна | session_id, query, trace_data (JSONB), total_ms, cost_usd |
| `request_logs` | HTTP логи | method, path, status, duration_ms, session_id |

### catalog (товары)

| Таблица | Что хранит | Ключевые поля |
|---------|-----------|---------------|
| `catalog.tenants` | Тенанты | slug (UNIQUE), name, type, settings, catalog_digest |
| `catalog.categories` | Категории (иерархия) | name, slug, parent_id (self-ref) |
| `catalog.master_products` | Мастер-каталог продуктов | sku (UNIQUE), name, brand, images, embedding (vector 384), PIM поля |
| `catalog.products` | Продукты тенанта | tenant_id, master_product_id, price, stock_quantity, rating |
| `catalog.master_services` | Мастер-каталог услуг | sku, name, duration, provider, embedding |
| `catalog.services` | Услуги тенанта | tenant_id, master_service_id, price, availability |
| `catalog.stock` | Остатки | tenant_id + product_id (PK), quantity, reserved |
| `catalog.ingredients` | INCI ингредиенты | inci_name, name_ru, function |
| `catalog.product_ingredients` | Связь продукт↔ингредиент | master_product_id, ingredient_id, position, is_key |
| `catalog.field_definitions` | Определения полей для V2 engine | entity_type, field_name, atom_type, subtype, slot, tenant_id |

---

## 3. Session State — детально

**Таблица**: `chat_session_state`
**Одна запись на сессию** (UNIQUE session_id)

### Зона DATA (`current_data`, JSONB)

```json
{
  "products": [
    {
      "id": "abc-123",
      "name": "Осветляющий крем для лица с витамином C",
      "price": 139000,
      "priceFormatted": "1,390.00",
      "currency": "KRW",
      "brand": "COSRX",
      "category": "Skincare",
      "images": ["https://cdn.example.com/photo1.jpg"],
      "rating": 4.5,
      "stockQuantity": 100,
      "description": "Крем с витамином C для сияющей кожи",
      "tags": ["skincare", "brightening"],
      "productForm": "cream",
      "skinType": ["all"],
      "concern": ["dullness", "dark spots"],
      "keyIngredients": ["vitamin C", "niacinamide"]
    }
    // ... ещё продукты
  ],
  "services": []
}
```

**Кто пишет**: `catalog_search` tool, `state_filter` tool, `search_products` tool → через `UpdateData()`
**Кто читает**: Agent2 usecase (для рендеринга), visual_assembly tool (для построения виджетов)

---

### Зона META (`current_meta`, JSONB)

```json
{
  "count": 23,
  "productCount": 23,
  "serviceCount": 0,
  "fields": ["name", "price", "images", "rating", "brand", "category", "description"],
  "aliases": {
    "tenant_slug": "keepstart"
  }
}
```

**Кто пишет**: Пишется вместе с DATA через `UpdateData()`
**Кто читает**: Agent2 usecase (productCount для промпта), промпт Agent2 (поля для отображения)

---

### Зона TEMPLATE (`current_template`, JSONB)

```json
{
  "formation": {
    "mode": "grid",
    "grid": { "cols": 3 },
    "widgets": [
      {
        "id": "w-001",
        "type": "product_card",
        "template": "ProductCard",
        "size": "small",
        "atoms": [
          { "type": "image", "subtype": "url", "display": "hero", "value": "https://...", "slot": "hero", "fieldName": "images" },
          { "type": "text", "subtype": "string", "display": "h2", "value": "Крем COSRX", "slot": "title", "fieldName": "name" },
          { "type": "number", "subtype": "currency", "display": "price-lg", "value": 139000, "slot": "price", "fieldName": "price" }
        ],
        "entityRef": { "type": "product", "id": "abc-123" }
      }
      // ... ещё виджеты
    ],
    "config": {
      "entity_type": "product",
      "preset": "default_product",
      "mode": "grid",
      "size": "small",
      "fields": [
        { "name": "images", "slot": "hero", "format": "", "display": "hero" },
        { "name": "name", "slot": "title", "format": "", "display": "h2" },
        { "name": "price", "slot": "price", "format": "currency", "display": "price-lg" }
      ]
    }
  }
}
```

**Кто пишет**: `visual_assembly` tool, `render_product_preset` tool, navigation expand/back → через `UpdateTemplate()`
**Кто читает**: Agent2 usecase (current_formation для промпта), visual_assembly tool (currentFields, currentMode), фронтенд (formation JSON через API response)

---

### Зона VIEW (`view_mode` + `view_focused` + `view_stack`)

```
view_mode: "grid"                          ← VARCHAR колонка
view_focused: null                         ← JSONB (null в grid, {"type":"product","id":"abc-123"} в detail)
view_stack: [                              ← JSONB массив
  {
    "mode": "grid",
    "focused": null,
    "refs": [{"type":"product","id":"abc-123"}, ...],
    "step": 3,
    "created_at": "2026-03-30T12:00:00Z"
  }
]
```

**Кто пишет**: Navigation expand/back → через `UpdateView()`
**Кто читает**: Agent2 usecase (view mode для промпта), фронтенд (навигация)

---

### Зона CONVERSATION (`conversation_history`, JSONB)

```json
[
  { "role": "user", "content": "Привет, покажи кремы для лица" },
  { "role": "assistant", "content": "", "toolCalls": [{"id":"tc-1","name":"catalog_search","input":{"query":"кремы для лица"}}] },
  { "role": "user", "content": "{\"status\":\"ok\",\"count\":23}" },
  { "role": "user", "content": "Покажи только COSRX" },
  { "role": "assistant", "content": "", "toolCalls": [{"id":"tc-2","name":"_internal_state_filter","input":{"brand":"COSRX"}}] },
  { "role": "user", "content": "{\"status\":\"ok\",\"count\":3}" }
]
```

**Кто пишет**: Agent1 usecase → через `AppendConversation()` (без delta!)
**Кто читает**: Agent1 usecase (контекст для LLM, кэш промпта Anthropic)
**Не видит Agent2!** Agent2 не читает conversation_history.

---

### Зона AGENT2_HISTORY (`agent2_history`, JSONB)

```json
[
  { "role": "assistant", "toolCalls": [{"id":"tc-v1","name":"visual_assembly","input":{"layout":"grid","size":"small"}}] },
  { "role": "user", "content": "{\"mode\":\"grid\",\"widgets\":23}" },
  { "role": "assistant", "toolCalls": [{"id":"tc-v2","name":"visual_assembly","input":{"show\":[\"rating\"]}}] },
  { "role": "user", "content": "{\"mode\":\"grid\",\"widgets\":23}" }
]
```

**Хранит**: Последние 4 сообщения (2 turn'а) tool calls Agent2
**Кто пишет**: Agent2 usecase → через `AppendAgent2History()` (без delta!)
**Кто читает**: Agent2 usecase (мульти-turn контекст для LLM)
**Не видит Agent1!**

---

### Зона ACTIONS (`actions`, JSONB)

```json
{
  "likedIds": ["abc-123", "def-456"],
  "cartItems": [
    { "entityType": "product", "entityId": "abc-123", "quantity": 1, "addedAt": "2026-03-30T12:05:00Z" }
  ]
}
```

**Кто пишет**: User actions (like/cart) → через `UpdateActions()`
**Кто читает**: Agent1 usecase (контекст), фронтенд (UI состояние)

---

### Step (`step`, INTEGER)

Автоинкрементируемый счётчик. Каждый `AddDelta()` увеличивает на 1.
Позволяет отслеживать хронологический порядок всех изменений в стейте.

---

## 4. Дельты — история изменений

**Таблица**: `chat_session_deltas` (append-only, immutable)

```json
{
  "step": 1,
  "trigger": "USER_QUERY",
  "source": "llm",
  "actor_id": "agent1",
  "delta_type": "add",
  "path": "data.products",
  "action": {
    "type": "SEARCH",
    "tool": "catalog_search",
    "params": { "query": "кремы для лица" }
  },
  "result": {
    "count": 23,
    "fields": ["name", "price", "images", "rating", "brand"]
  },
  "turn_id": "turn-abc-123"
}
```

**Кто пишет**: Любой zone-write (UpdateData, UpdateTemplate, UpdateView, UpdateActions) автоматически создаёт delta через `AddDelta()`
**Кто читает**: Agent2 (контекст — что изменилось), Pipeline (trace snapshots), Debug handler

---

## 5. Фронтенд — что хранит клиент

### React State (в памяти, теряется при закрытии)

| Где | Что | Тип |
|----|-----|-----|
| `WidgetApp.jsx` | `activeFormation` — текущая формация на экране | useState |
| `WidgetApp.jsx` | `isChatOpen` — открыт ли чат | useState |
| `ChatPanel.jsx` | `lastFormationRef` — последняя формация (для screenContext) | useRef |
| `ChatPanel.jsx` | `adjacentTemplatesRef` — шаблоны для instant expand | useRef |
| `ChatPanel.jsx` | `entitiesRef` — продукты/сервисы (для instant expand) | useRef |
| `ChatPanel.jsx` | `lastQueryRef` — последний запрос | useRef |
| `useChatSubmit.js` | `requestIdRef` — monotonic counter (race condition guard) | useRef |
| `useChatSubmit.js` | `abortRef` — AbortController текущего запроса | useRef |
| `useChatMessages.js` | `messages` — массив сообщений чата | useState |
| `useChatMessages.js` | `sessionId` — ID текущей сессии | useState |
| `useChatMessages.js` | `isLoading` — ждём ответа | useState |
| `useFormationHistory.js` | `history` — массив формаций (back/forward) | useState |

### localStorage (переживает F5, TTL 30 мин)

| Ключ | Что хранит |
|------|-----------|
| `chatSessionId` | UUID сессии |
| `chatSessionCache` | Полный кэш: messages, formation, formationHistory, adjacentTemplates, entities, savedAt |
| `theme` | Название темы |
| `debug` | "true" если debug mode |

---

## 6. API Response — что бэкенд отдаёт фронтенду

`POST /api/v1/pipeline` → Response:

```json
{
  "sessionId": "uuid",
  "formation": { ... },          // FormationWithData — полная формация для рендеринга
  "adjacentTemplates": { ... },  // Шаблоны для instant expand (без round-trip)
  "entities": {                  // Сырые данные продуктов (для instant expand)
    "products": [...],
    "services": [...]
  },
  "agent1Ms": 450,               // Время Agent1
  "agent2Ms": 280,               // Время Agent2
  "totalMs": 730                  // Общее время
}
```

**Фронтенд НЕ получает**: state, deltas, conversation_history, agent2_history, view_stack, actions. Только formation + entities + templates.

---

## 7. Что НЕ записывается через delta

| Запись | Delta? | Почему |
|--------|--------|--------|
| `AppendConversation()` | Нет | Это LLM кэш, не бизнес-данные |
| `AppendAgent2History()` | Нет | Это LLM кэш, не бизнес-данные |
| `PushView()` / `PopView()` | Нет | Delta создаётся следующим `UpdateView()` |

---

## 8. Визуальная Timeline запроса

```
Юзер → "Покажи кремы"
          │
    ┌─────▼─────────────────────────────────────────────┐
    │  HANDLER (handler_pipeline.go)                    │
    │  Валидирует запрос, получает/создаёт session      │
    └─────┬─────────────────────────────────────────────┘
          │
    ┌─────▼─────────────────────────────────────────────┐
    │  AGENT1 USECASE                                   │
    │                                                   │
    │  📖 READ GetState()          ← БД: все зоны      │
    │     state = {data: {products: []}, meta: {...}}   │
    │                                                   │
    │  🤖 LLM Call → "catalog_search(кремы)"            │
    │                                                   │
    │  ⚙️  TOOL: catalog_search                         │
    │     SQL: keyword search + vector search + RRF     │
    │     Result: 23 products                           │
    │                                                   │
    │  📝 WRITE UpdateData()       ← БД: current_data   │
    │     {products: [23 items], meta: {count: 23}}     │
    │     + Delta: step=1, path="data.products"         │
    │                                                   │
    │  📖 READ GetState()          ← БД: ⚠️ stale?     │
    │     productsFound = 23 (или stale!)               │
    │                                                   │
    │  📝 WRITE AppendConversation() ← БД: conv history │
    │                                                   │
    │  Return: {ProductsFound: 23, ToolName: "catalog"} │
    └─────┬─────────────────────────────────────────────┘
          │
    ┌─────▼─────────────────────────────────────────────┐
    │  PIPELINE: buildMicrocontext()                    │
    │  microcontext = "new_search: 23 items found"      │
    │                                                   │
    │  📖 READ GetState() + GetDeltas()  ← для трейса   │
    └─────┬─────────────────────────────────────────────┘
          │
    ┌─────▼─────────────────────────────────────────────┐
    │  AGENT2 USECASE                                   │
    │                                                   │
    │  📖 READ GetState()          ← БД: все зоны      │
    │     ProductCount = len(products)  ⚠️ stale?       │
    │                                                   │
    │  📖 READ GetDeltasSince(0)   ← какой delta        │
    │  📖 READ GetDeltas()         ← вся история        │
    │                                                   │
    │  Строит промпт:                                   │
    │  - productCount: 23                               │
    │  - microcontext: "new_search: 23 items"           │
    │  - current_formation: (предыдущая или null)       │
    │  - screen_state: (от фронтенда)                   │
    │  - agent2_history: (последние 2 turn'а)           │
    │                                                   │
    │  🤖 LLM Call → "visual_assembly(layout: grid)"    │
    │                                                   │
    │  ⚙️  TOOL: visual_assembly                        │
    │     Берёт products из state (в памяти)            │
    │     Строит Formation + Widgets + Atoms            │
    │                                                   │
    │  📝 WRITE UpdateTemplate()   ← БД: current_template│
    │     {formation: {mode: "grid", widgets: [...]}}   │
    │     + Delta: step=2, path="template"              │
    │                                                   │
    │  📝 WRITE AppendAgent2History() ← БД: a2 history  │
    │                                                   │
    │  📖 READ GetState()          ← БД: достать formation│
    │                                                   │
    │  Return: {Formation: {...}}                       │
    └─────┬─────────────────────────────────────────────┘
          │
    ┌─────▼─────────────────────────────────────────────┐
    │  PIPELINE: финализация                            │
    │  📖 READ GetState() + GetDeltas() ← финальный трейс│
    │                                                   │
    │  Строит response:                                 │
    │  - formation (из Agent2)                          │
    │  - adjacentTemplates                              │
    │  - entities (products/services из state)           │
    └─────┬─────────────────────────────────────────────┘
          │
          ▼
    Фронтенд получает JSON → рендерит FormationRenderer
```

---

## 9. Счётчик операций за 1 turn

| Операция | Количество | Зачем |
|----------|-----------|-------|
| **GetState (SELECT)** | 5-6 | Agent1 (2), Pipeline trace (2), Agent2 (2) |
| **GetDeltas (SELECT)** | 3-4 | Agent2 (2), Pipeline trace (2) |
| **UpdateData (UPDATE)** | 1 | Tool записывает продукты |
| **UpdateTemplate (UPDATE)** | 1 | Tool записывает формацию |
| **AppendConversation (UPDATE)** | 1-2 | Agent1 пишет LLM историю |
| **AppendAgent2History (UPDATE)** | 1 | Agent2 пишет tool историю |
| **AddDelta (INSERT)** | 2 | По одному на UpdateData и UpdateTemplate |
| **ИТОГО** | **~14-16 queries** | На один запрос юзера |

---

## 10. Env переменные

| Переменная | Что | Пример |
|-----------|-----|--------|
| `DATABASE_URL` | PostgreSQL connection string | `postgresql://user:pass@host/db?sslmode=require` |
| `ANTHROPIC_API_KEY` | Ключ Claude API | `sk-ant-...` |
| `OPENAI_API_KEY` | Ключ OpenAI (embeddings) | `sk-...` |
| `TENANT_SLUG` | Тенант по умолчанию | `keepstart` |
| `LLM_MODEL` | Модель LLM | `claude-haiku-4-5-20251101` |
| `ENGINE_VERSION` | v1 или v2 engine | `v2` |
| `AGENT2_PROMPT_VERSION` | v1 или v2 промпт | `v2` |
| `BACKEND_PORT` | Порт бэкенда | `8080` |
