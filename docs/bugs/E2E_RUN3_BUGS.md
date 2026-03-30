# E2E Run 3 — Найденные баги (2026-03-30)

**Сессия**: `d792aff2-36d2-48fe-b7dd-3d8319eff716`
**Результат**: 5 PASS / 20 FAIL из 25 тестов
**Скриншоты**: `tests/e2e_screenshots/`
**Трейсы**: `pipeline_traces WHERE session_id = 'd792aff2-...'`

---

## BUG-1: Фронтенд отстаёт на 1 шаг (P0)

**Симптом**: Визуальный результат соответствует ПРЕДЫДУЩЕМУ запросу, а не текущему.

**Доказательства из трейсов + скриншотов**:

| Шаг | Запрос | Backend вернул | Фронт показал |
|-----|--------|---------------|---------------|
| #8 | "Покажи цену первой, потом название" | order:[price,name,...] | Тот же grid (порядок не изменился) |
| #9 | "Покажи крупными карточками" | size не указан в трейсе | Grid с ценой СВЕРХУ (order от #8!) |
| #10 | "Покажи каруселью" | layout:"carousel" | Grid (тот же что в #9) |
| #11 | "Покажи таблицей" | — | Carousel (от #10!) |
| #12 | "Покажи сеткой маленькими" | layout:"grid", size:"small" | Carousel (от #10, 2 шага) |
| #20 | "Покажи увлажняющие" | layout:"grid", 50 widgets | Comparison (от #19) |
| #21 | "Первые 3 крупными" | — | Grid увлажняющих (данные от #20) |

### Результат исследования кода

**Race condition подтверждён** в `useChatSubmit.js`.

**Цепочка вызовов**:
1. Пользователь вводит текст → `ChatPanel.jsx` вызывает `rawSubmit(text)` из `useChatSubmit.js`
2. `useChatSubmit.js:26` — guard: `if (submittingRef.current) return` (блокирует параллельные отправки)
3. `useChatSubmit.js:40-51` — строит `screenContext` из `lastFormationRef.current`
4. `useChatSubmit.js:53` — `await sendPipelineQuery()` API call
5. `useChatSubmit.js:63-64` — `onFormationReceived(response.formation)` → callback в `ChatPanel.jsx:54-65`
6. `ChatPanel.jsx:54-65` — pushes to history, обновляет `lastFormationRef`, вызывает `setActiveFormation` из `WidgetApp.jsx:108`
7. `WidgetApp.jsx:80-84` — `FormationRenderer` рендерит `activeFormation`

**Проблема**: Guard `submittingRef` защищает от одновременных ОТПРАВОК, но **не от out-of-order ОТВЕТОВ**. Нет request versioning — если ответ запроса A приходит после ответа запроса B, он перезапишет корректный результат.

**Дополнительно**: 5 запросов из 25 **не попали в трейсы** (#6, #9, #13 и др.) — возможно фронтенд их НЕ ОТПРАВИЛ (guard отбросил как дубликат, или race condition в submit).

**Файлы для фикса**:
- `project/frontend/src/features/chat/useChatSubmit.js` — строки 26-91
- `project/frontend/src/features/chat/ChatPanel.jsx` — строки 54-65
- `project/frontend/src/WidgetApp.jsx` — строка 108

**Фикс**: Добавить request counter:
```javascript
const requestIdRef = useRef(0)

const submit = useCallback(async (text) => {
  const thisRequestId = ++requestIdRef.current
  // ... await sendPipelineQuery() ...
  if (thisRequestId !== requestIdRef.current) return // stale response — discard
  onFormationReceived(response.formation)
})
```
Или использовать `AbortController` для отмены предыдущих запросов.

---

## BUG-2: Widget count расхождение backend vs frontend (P1)

**Симптом**: Backend возвращает N виджетов, фронт показывает другое число.

| Шаг | Backend widgets | Frontend widgets |
|-----|----------------|-----------------|
| #1 | 23 | 12 |
| #5 | 23 | 23 (без фото) |
| #7 | 23 | 12 |
| #16 | 50 | 12 |
| #20 | 3 (сыворотки) | 12 |

**Гипотеза**: Фронтенд имеет пагинацию или хардкод-лимит на отображение (12?). Или formation JSON содержит все виджеты, но рендерер обрезает.

**Где копать**:
- `project/frontend/src/entities/formation/FormationRenderer.jsx` — есть ли limit/slice на widgets?
- `project/frontend/src/features/catalog/ProductGrid.jsx` — пагинация?
- Или это BUG-1 (показывает предыдущую формацию с 12 виджетами)

---

## BUG-3: Show/Hide не редактирует текущую карточку (P0)

**Симптом**: На detail view (#3) запрос "покажи только название и цены" (#4) не убирает лишние поля.

**Трейс #4**:
```json
{
  "hide": ["images","rating","brand","category","description","tags","stockQuantity","productForm","skinType","concern","keyIngredients"],
  "layout": "grid"
}
```

**Проблема Agent2**: Вместо того чтобы оставить `layout:"single"` и применить hide к текущей detail card, Agent2 переключил на `layout:"grid"`. Он не понимает что пользователь редактирует ТЕКУЩИЙ вид, а не просит новый.

**Трейс #5** ("Добавь рейтинг"):
```json
{"show": ["rating"]}
```
Agent2 послал правильно, но без layout — значит движок применил дефолт (grid). Контекст "мы на detail card" потерян.

### Результат исследования кода

**Agent2 ПОЛУЧАЕТ полный контекст экрана — проблема в том, что он его ИГНОРИРУЕТ.**

Agent2 получает в user prompt:
```json
{
  "screen_state": {
    "mode": "single",           // ← фронтенд говорит: сейчас single detail card
    "widget_count": 1,
    "visible_fields": ["images", "name", "price", "brand", "description"]
  },
  "view_mode": "detail",        // ← бэкенд подтверждает: detail mode
  "focused": { "type": "product", "id": "prod-123" },
  "current_formation": {...},   // ← текущая формация
  "user_request": "покажи только название и цены"
}
```

В системном промпте есть **Rule #7** (`prompt_compose_widgets.go`):
> "IMPORTANT: screen_state shows what the user CURRENTLY sees. If screen_state.mode="single" and widget_count=1 — user is on a DETAIL card. Apply changes TO THE DETAIL CARD (layout: "single"), DON'T switch back to grid."

**Но модель игнорирует Rule #7** и шлёт `layout:"grid"`. Почему:
1. Rule #7 далеко в промпте (после примеров), модель может не придавать ему достаточный вес
2. Примеры в промпте показывают grid-first поведение — нет примера show/hide на detail card
3. `show:["rating"]` без layout → движок применяет дефолт grid

**Файлы для фикса**:
- `project/backend/internal/prompts/prompt_compose_widgets.go` — Rule #7 + примеры
- `project/backend/internal/usecases/agent2_execute.go` — строки 186-207

**Варианты фикса**:
1. **Промпт**: Вынести Rule #7 в НАЧАЛО (до примеров), добавить explicit пример show/hide на detail
2. **Движок**: Если `screen_state.mode == "single"` и Agent2 не послал layout → подставлять `layout:"single"` автоматически (а не grid default)
3. **Оба**: промпт + fallback в движке

---

## BUG-4: Layout switch на list не работает (P1)

**Симптом**: #2 "Покажи их списком" — grid остаётся.

**Трейс #2**:
```json
{"layout": "list"}
```
Agent2 послал `layout:"list"` корректно. Backend вернул 23 виджета.

**Но фронт показывает grid**. Это может быть:
1. BUG-1 (задержка) — но #1 тоже grid, так что задержка ничего не меняет
2. Formation JSON не содержит layout:"list" — движок проигнорировал
3. Фронтенд FormationRenderer не поддерживает list для текущего пресета

**Где копать**:
- Проверить formation JSON в трейсе: `trace_data->'formationResult'->'fullJson'->>'mode'`
- `project/backend/internal/tools/tool_visual_assembly.go` — как обрабатывается layout:"list"
- `project/frontend/src/entities/formation/FormationRenderer.jsx` — поддержка mode:"list"

---

## BUG-5: state_filter не влияет на формацию (P1)

**Симптом**: #14 "Покажи только COSRX" — фильтр вызван, но все 12 товаров остались.

**Трейс #14**:
- Agent1: `_internal_state_filter` — вызван
- Agent2 toolInput: `{}` (пустой!)
- Backend widgets: 23

**Проблема**: Agent1 фильтрует state.data, но:
1. Agent2 получает `{}` — не знает что данные изменились
2. state.meta.entityCount не обновляется после фильтра
3. В итоге Agent2 рендерит ВСЕ товары, не отфильтрованные

**Трейс #15** ("Покажи с составом"):
- Agent1: `_internal_state_filter` (повторно)
- Agent2: `show:["keyIngredients","skinType"]`
- Backend widgets: 3

На #15 сработало — значит второй вызов state_filter обновил meta, и Agent2 увидел 3 товара.

**Где копать**:
- `project/backend/internal/tools/tool_state_filter.go` — обновляет ли meta.entityCount?
- `project/backend/internal/usecases/agent1_execute.go` — записывается ли результат фильтра в state?
- Порядок: Agent1 записывает → Agent2 читает. Есть ли гарантия что запись завершена до чтения?

---

## BUG-6: Limit не работает (P1)

**Симптом**: "Покажи топ-5" (#17), "первые 3 крупными" (#21) — показывает больше.

**Трейс #17**:
```json
{"limit": 5, "show": ["description"], "size": "large"}
```
Backend widgets: **5** (правильно!)

**Но фронт показал 3** (старые данные от #16). Это опять BUG-1 — задержка.

**Трейс #21**: Нет в выборке (пропущен запрос "Покажи первые 3 крупными карточками с описанием"). Фронт показал 12. Тоже BUG-1.

**Вывод**: Limit работает на бэкенде. Проблема в отображении (BUG-1).

---

## BUG-7: Size override не работает (P1)

**Симптом**: "Покажи крупными" (#9) — size остаётся small.

**Трейс #9**: Запрос "Покажи крупными карточками" — **нет в трейсах**! Между #8 (order) и #10 (carousel) нет записи для #9.

**Гипотеза**: Запрос #9 не дошёл до pipeline или ответ пришёл так быстро что тест не дождался. Или фронтенд отправил запрос но не получил ответ.

**Примечание**: Трейс содержит 20 записей, а тестов 25. Часть запросов пропущена в трейсах:
- Нет #6 ("Убери цены")
- Нет #9 ("Покажи крупными")
- Нет #13 ("Покажи горизонтальными")
- Нет #18 ("Сравни первые 3") — нет, есть, это #14 в трейсе
- Нет #21, #22 — в одном трейсе

**Где копать**: Проверить фронтенд — все ли запросы реально отправляются. Возможно некоторые запросы теряются (race condition в ChatPanel).

---

## BUG-8: Comparison → grid не переключается (P1)

**Симптом**: После comparison (#19) запрос "Покажи увлажняющие" (#20) показывает comparison.

**Трейс #20** ("Покажи увлажняющие"):
- Agent1: `catalog_search`
- Agent2: `layout:"grid", size:"medium"`
- Backend widgets: **50**

Backend вернул grid с 50 виджетами — правильно. Фронт показывает comparison от #19.

**Это BUG-1** — фронтенд задержка.

---

## BUG-9: Visual modifiers (shadow, accent) не работают (P3)

**Симптом**: #24 "С тенями и акцентным цветом" — тени не видны.

**Трейс #24**:
```json
{
  "atoms": {"price": {"color": "orange"}},
  "widgetShadow": "md"
}
```

Agent2 послал правильно. Но:
1. `widgetShadow:"md"` — поддерживает ли движок/фронтенд?
2. `atoms.price.color:"orange"` — поддерживает ли V2 рендерер цвет на атомах?

**Где копать**:
- `project/backend/internal/engine/engine_v2.go` — обработка widgetShadow
- `project/backend/internal/engine/tokens.go` — DesignTokensV2, shadow
- `project/frontend/src/entities/widget/templates/GenericCardV2Template.jsx` — применяется ли shadow из formation JSON?

---

## BUG-10: Direction horizontal не работает (P2)

**Симптом**: #13 "Покажи горизонтальными карточками" — карточки вертикальные.

**Трейс #13**: Нет в трейсах (пропущен запрос).

**Где копать**:
- Во-первых: доходит ли запрос до бэкенда?
- `project/backend/internal/engine/` — поддержка direction в EngineV2
- `project/frontend/src/entities/widget/templates/` — CSS для horizontal layout

---

## BUG-11: Тест — slots=[] всегда пустой (баг тестового фреймворка)

**Симптом**: `[data-slot]` атрибуты не находятся. Все поля fields_present/fields_absent проверки ложно FAIL.

**Причина**: V2 рендерер (`AtomV2Renderer.jsx`, `GenericCardV2Template.jsx`) вероятно не ставит `data-slot` атрибут на элементы. V1 использовал `<span class="atom" data-slot="title">`, V2 может использовать другую структуру.

**Фикс**: Добавить `data-slot` атрибут в V2 рендерер (не ломает ничего, помогает и тестам и debug).

**Файлы**:
- `project/frontend/src/entities/atom/AtomV2Renderer.jsx` — добавить `data-slot={atom.fieldName}`
- `project/frontend/src/entities/widget/templates/GenericCardV2Template.jsx` — проверить что slot пробрасывается

---

## Приоритизация

| Приоритет | Баг | Влияние |
|-----------|-----|---------|
| **P0** | BUG-1: Фронтенд задержка | 10+ тестов ломаются, пользователь видит старые данные |
| **P0** | BUG-3: Show/Hide не редактирует view | Вся аддитивная логика сломана |
| **P1** | BUG-2: Widget count mismatch | Пользователь видит не все товары |
| **P1** | BUG-4: List layout не работает | Layout switching |
| **P1** | BUG-5: state_filter meta не обновляется | Фильтрация по бренду |
| **P1** | BUG-7: Пропущенные запросы | 5 запросов не дошли до бэкенда |
| **P2** | BUG-10: Direction horizontal | Visual feature |
| **P3** | BUG-9: Shadow/accent | Visual feature |
| **Test** | BUG-11: slots=[] в тесте | Ложные FAIL |

---

## Agent2 — кэширование и размер промпта

### Текущее состояние (работает правильно)

| Компонент | Размер | Статический? | Кэшируется? |
|-----------|--------|-------------|-------------|
| System prompt V1 | 7,776 chars (~1,944 tokens) | Да | Да (ephemeral) |
| System prompt V2 | 6,180 chars (~1,545 tokens) | Да | Да (ephemeral) |
| Tool definition V1 | ~5,426 chars (~1,356 tokens) | Да | Да (ephemeral) |
| Tool definition V2 | ~7,427 chars (~1,857 tokens) | Да | Да (ephemeral) |
| User prompt | 500-2,000 chars | Нет | Нет |
| Agent2 history | 0-4 messages (2 turns) | Нет | Нет |

**Итого ~3,300 tokens кэшируется** (system + tools). Cache read = 0.1x стоимости → ~90% экономии на статике.

### Как устроен кэш
- Файл: `project/backend/internal/adapters/anthropic/anthropic_client.go` (строки 381-571)
- `CacheConfig{CacheTools: true, CacheSystem: true, CacheConversation: false}`
- System prompt и последний tool definition помечаются `cache_control: {type: "ephemeral"}`
- TTL кэша Anthropic: 5 минут

### Agent2 history (multi-turn память)
- Файл: `project/backend/internal/usecases/agent2_execute.go` (строки 209-223)
- Хранит последние 4 LLM-сообщения (2 turns): `assistant:tool_use` + `user:tool_result`
- Позволяет Agent2 видеть свои предыдущие решения
- **НЕ** видит Agent1 tool calls
- Персистится в `state.Agent2History` через `AppendAgent2History()`

### Возможные оптимизации (низкий приоритет)
1. Сократить V1 промпт: убрать 5-6 менее важных примеров → -300 tokens
2. V2 tool definition крупнее V1 (7,427 vs 5,426) — можно упростить atoms schema
3. Включить cache на conversation history (`CacheConversation: true`) — сейчас отключен
4. V2 system prompt на 20% компактнее V1 — переход на V2 сам по себе экономит

---

## Рекомендуемый порядок фикса

1. **BUG-1** (P0) — фронтенд задержка. Починив это, ~10 тестов могут стать PASS
2. **BUG-3** (P0) — Agent2 промпт: "сохраняй текущий layout при show/hide"
3. **BUG-11** (Test) — починить data-slot селекторы в тесте
4. **BUG-5** (P1) — state_filter meta update
5. **BUG-4** (P1) — list layout
6. Остальное по приоритету

---

## Как воспроизвести

```bash
# Запустить тесты
/Library/Developer/CommandLineTools/usr/bin/python3 tests/e2e_run.py

# Посмотреть трейсы для сессии
psql $DATABASE_URL -c "
  SELECT query,
    trace_data->'agent1'->>'toolName' as a1_tool,
    trace_data->'agent2'->>'toolInput' as a2_input,
    trace_data->'formationResult'->>'widgetCount' as widgets,
    error, total_ms
  FROM pipeline_traces
  WHERE session_id = '<SESSION_ID>'
  ORDER BY timestamp;"

# Посмотреть formation JSON
psql $DATABASE_URL -c "
  SELECT query,
    trace_data->'formationResult'->'fullJson'->>'mode' as mode,
    trace_data->'formationResult'->'fullJson'->>'grid' as grid
  FROM pipeline_traces
  WHERE session_id = '<SESSION_ID>'
  ORDER BY timestamp;"
```
