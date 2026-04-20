# State Flow Map — Pipeline Turn

> Полная карта чтения/записи session state за один pipeline turn.
> Каждый `READ` и `WRITE` — это поход в PostgreSQL (Neon serverless).

---

## Session State — структура

```
SessionState
├── Current
│   ├── Data          ← Products[], Services[]
│   ├── Meta          ← count, productCount, serviceCount, fields[], aliases{}
│   └── Template      ← {"formation": FormationWithData}
├── View
│   ├── Mode          ← grid | detail | list | carousel
│   └── Focused       ← {type, id} — текущий товар в detail
├── ViewStack         ← навигационная история (back/forward)
├── ConversationHistory ← LLM сообщения Agent1 (для кэша промпта)
├── Agent2History     ← LLM сообщения Agent2 (последние 2 turn'а)
├── Actions           ← likedIds[], cartItems[]
└── Step              ← int (автоинкремент при каждом delta)
```

**БД колонки**: `current_data`, `current_meta`, `current_template`, `view_mode`, `view_focused`, `view_stack`, `conversation_history`, `agent2_history`, `actions`, `step`

---

## Timeline одного turn'а

```
ЗАПРОС: "Покажи только COSRX"
═══════════════════════════════════════════════════════════════

PHASE 0 — Init (pipeline_execute.go)
  └─ READ session (кэш-порт, не state)
  └─ Генерация TurnID

───────────────────────────────────────────────────────────────

PHASE 1 — AGENT1 (agent1_execute.go)

  ①  READ  GetState()                          ← line 99
     │     Загружает ВСЁ: data, meta, template, view, history
     │     Используется: meta.aliases (tenant), productCount,
     │                   conversationHistory (для LLM контекста)
     │
     ├─ [Путь A: Детерминистический фильтр]
     │   Если data есть И query = фильтр:
     │   │
     │   │  EXECUTE  _internal_state_filter (tool)
     │   │  ├─ Читает products из памяти (не из БД)
     │   │  ├─ Фильтрует: 23 → 3
     │   │  └─ WRITE  UpdateData()              ← tool_state_filter.go:158
     │   │            pool.Exec(UPDATE current_data, current_meta)
     │   │            + AddDelta()
     │   │
     │   ②  WRITE  AppendConversation()         ← line 175
     │   │         (без delta, перезапись колонки)
     │   │
     │   └─ RETURN (пропуск LLM)
     │
     ├─ [Путь B: LLM вызов]
     │   │
     │   │  Строит промпт с enriched query
     │   │  Вызывает LLM → LLM отвечает tool_use
     │   │
     │   │  EXECUTE  catalog_search / state_filter / etc (tool)
     │   │  └─ WRITE  UpdateData()              ← в tool коде
     │   │
     │   ③  READ  GetState()                    ← line 287
     │   │        productsFound = len(products)
     │   │        ⚠️  МОЖЕТ БЫТЬ STALE — connection pool
     │   │
     │   └─ WRITE  AppendConversation()         ← line 320
     │             user → assistant:tool_use → user:tool_result
     │
     └─ RETURN Agent1Response { ProductsFound, ToolName, ... }

───────────────────────────────────────────────────────────────

PHASE 1.5 — Между агентами (pipeline_execute.go)

  ④  READ  GetState()                           ← line 177
     │     Для trace snapshot (не для Agent2!)
     │
  ⑤  READ  GetDeltas()                          ← line 178
     │     Для trace snapshot
     │
     └─ Строит microcontext из Agent1Response:
        microcontext = "filtered: {ProductsFound} items"
        ⚠️  Если ProductsFound stale → microcontext врёт

───────────────────────────────────────────────────────────────

PHASE 2 — AGENT2 (agent2_execute.go)

  ⑥  READ  GetState()                           ← line 113
     │     Загружает ВСЁ заново
     │     ⚠️  МОЖЕТ БЫТЬ STALE — те же products что Agent1 видел
     │
     │  Пересчитывает count из data:
     │  ProductCount = len(state.Current.Data.Products)
     │  ⚠️  Если data stale → count stale → промпт Agent2 врёт
     │
  ⑦  READ  GetDeltasSince(0)                    ← line 163
     │     Ищет data-delta для текущего TurnID
     │     (чтобы понять: данные изменились или нет)
     │
  ⑧  READ  GetDeltas()                          ← line 183
     │     Вся история delta для контекста
     │
     │  Строит промпт Agent2:
     │  - productCount (из ⑥, может быть stale)
     │  - microcontext (из phase 1.5, может быть stale)
     │  - current_formation (из template)
     │  - screen_state (из фронтенда)
     │  - Agent2History (последние 4 сообщения)
     │
     │  Вызывает LLM → LLM отвечает tool_use
     │
     │  EXECUTE  visual_assembly (tool)
     │  ├─ Читает products из state (в памяти после ⑥)
     │  ├─ Строит formation с виджетами
     │  └─ WRITE  UpdateTemplate()              ← tool_visual_assembly.go:358
     │            pool.Exec(UPDATE current_template)
     │            + AddDelta()
     │
  ⑨  WRITE  AppendAgent2History()               ← line 345
     │       assistant:tool_use → user:tool_result
     │       (без delta)
     │
  ⑩  READ  GetState()                           ← line 351
     │     Чтобы достать formation из template
     │
     └─ RETURN Agent2Response { Formation }

───────────────────────────────────────────────────────────────

PHASE 2.5 — После Agent2 (pipeline_execute.go)

  ⑪  READ  GetState()                           ← line 272
  ⑫  READ  GetDeltas()                          ← line 279
     └─ Для финального trace snapshot

───────────────────────────────────────────────────────────────

RETURN → Formation JSON → Frontend
```

---

## Точки WRITE за один turn

| # | Кто | Метод | Что пишет | Зона БД | Delta? |
|---|-----|-------|-----------|---------|--------|
| W1 | Tool (Agent1) | `UpdateData()` | Products, Services, Meta | current_data + current_meta | Да |
| W2 | Agent1 usecase | `AppendConversation()` | LLM history | conversation_history | Нет |
| W3 | Tool (Agent2) | `UpdateTemplate()` | Formation JSON | current_template | Да |
| W4 | Agent2 usecase | `AppendAgent2History()` | Tool call history | agent2_history | Нет |

---

## Точки READ за один turn

| # | Кто | Метод | Зачем | Line |
|---|-----|-------|-------|------|
| R1 | Agent1 | `GetState()` | Начальная загрузка | agent1:99 |
| R2 | Agent1 | `GetState()` | Count после tool | agent1:287 |
| R3 | Pipeline | `GetState()` | Trace snapshot | pipeline:177 |
| R4 | Pipeline | `GetDeltas()` | Trace snapshot | pipeline:178 |
| R5 | Agent2 | `GetState()` | Данные для рендеринга | agent2:113 |
| R6 | Agent2 | `GetDeltasSince()` | Data delta текущего turn | agent2:163 |
| R7 | Agent2 | `GetDeltas()` | Полная история | agent2:183 |
| R8 | Agent2 | `GetState()` | Извлечь formation | agent2:351 |
| R9 | Pipeline | `GetState()` | Финальный snapshot | pipeline:272 |
| R10 | Pipeline | `GetDeltas()` | Финальный snapshot | pipeline:279 |

**Итого: 1 WRITE data + 10 READ за один turn.**

---

## Stale Read проблема

```
WRITE W1:  pool.Exec(UPDATE) → Connection A → ОК
                                                    ← Neon может задержать
READ  R2:  pool.Query(SELECT) → Connection B → STALE! (видит старые 23)
READ  R5:  pool.Query(SELECT) → Connection C → STALE! (тоже 23)
```

**Почему**: `pgxpool.Pool` раздаёт connections из пула. Write идёт через connection A, read через B — Neon serverless может не гарантировать read-after-write consistency через разные connections.

**Затронутые операции**:
- R2 (agent1:287) → `ProductsFound` stale → microcontext врёт
- R5 (agent2:113) → `Products[]` stale → рендерит все вместо отфильтрованных

---

## Зоны БД и методы записи

| Зона | Колонка | Кто пишет | Через |
|------|---------|-----------|-------|
| Data | `current_data` | catalog_search, state_filter | `UpdateData()` |
| Meta | `current_meta` | catalog_search, state_filter | `UpdateData()` |
| Template | `current_template` | visual_assembly, render_preset, navigation | `UpdateTemplate()` |
| View | `view_mode` + `view_focused` + `view_stack` | navigation expand/back | `UpdateView()` |
| Conversation | `conversation_history` | Agent1 usecase | `AppendConversation()` |
| Agent2History | `agent2_history` | Agent2 usecase | `AppendAgent2History()` |
| Actions | `actions` | action_execute | `UpdateActions()` |
| Step | `step` | Любой AddDelta | `AddDelta()` |
| Deltas | `chat_session_deltas` (отдельная таблица) | Любой zone-write | `AddDelta()` |

---

## Connection Pool конфиг

```go
// postgres_client.go
MaxConns:          10
MinConns:          0
MaxConnLifetime:   1 hour
MaxConnIdleTime:   5 minutes
HealthCheckPeriod: 5 minutes
```

Транзакции **не используются** — каждый zone-write это отдельный `pool.Exec()` + `pool.QueryRow()` (AddDelta). Нет гарантии что следующий SELECT пойдёт через тот же connection.

---

## Кэширование

| Уровень | Что | Где |
|---------|-----|-----|
| LLM промпт | System prompt, tools, conversation | Anthropic API (TTL 5 мин) |
| Session metadata | Session объект | In-memory sync.Map |
| State | **НЕ кэшируется** | Всегда из PostgreSQL |
| Deltas | **НЕ кэшируются** | Всегда из PostgreSQL |
