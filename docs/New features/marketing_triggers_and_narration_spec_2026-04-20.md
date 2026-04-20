# Микроспека: Marketing Triggers + Conversational Narration

Две связанные системы. Разрабатываются раздельно но используют общую инфраструктуру (state-aware hints в pipeline).

---

## Часть 1 — Marketing Triggers

### Проблема
Маркетолог хочет управлять что показывается в чате на основе действий/состояния пользователя ("при просмотре увлажняющих — продвигай новую сыворотку"). Чисто rule-based жёстко и ломает естественность генерации.

### Решение
**Two-stage LLM**: маркетолог описывает намерение на NL → meta-LLM компилирует в структуру → runtime matcher применяет в чат-пайплайне как hint.

### Compile-time (в админке, редко)
Маркетолог открывает раздел "Triggers" в админке, пишет:
```
Когда пользователь смотрит увлажняющие средства, мягко продвигай 
новую сыворотку с витамином C — они хорошо комплементарны.
```

При сохранении — Sonnet компилирует в:
```json
{
  "id": "trigger_42",
  "name": "vitamin-C cross-sell",
  "condition": {
    "type": "viewing_category",
    "params": { "category": "moisturizers" }
  },
  "action": {
    "type": "hint_to_agent",
    "strength": "soft",
    "hint": "Subtly feature vitamin-C serum (id:42), reason: complements moisturizers user is viewing",
    "promoted_items": ["serum_42"]
  }
}
```

Маркетолог видит **превью на 10-20 синтетических сценариев** перед публикацией. Если результат не нравится — правит NL-описание, перекомпилирует.

### Типы условий (condition.type)
- `query_contains: ["крем", "увлажн"]`
- `viewing_category: "skincare"`
- `viewing_product_in_category: "moisturizer"`
- `session_history_includes: <product_id>`
- `user_segment: "returning_buyer"`
- `cart_contains: <product_id>`
- `cart_value_below: 50`
- `time_window: "2026-05-01..2026-05-31"`
- `composite: { op: "AND" | "OR" | "NOT", clauses: [...] }`

### Strength levels (action.strength)
- `soft` — просто hint Agent2, агент свободен игнорировать. Дефолт.
- `recommended` — обязан учесть, сам выбирает форму подачи (карточка / упоминание в narration / мини-баннер)
- `must` — мимо агента, hard insert в фиксированный слот. **Только compliance/disclaimer**.

### Runtime
Перед Agent2 в pipeline:
1. **Matcher** (детерминированный код, без LLM): проверяет все активные триггеры тенанта против текущей сессии. Выбирает совпавшие.
2. Совпавшие складываются в system-prompt Agent2 секцией:
   ```
   MARKETER HINTS for this turn:
   - [soft] Promote vitamin-C serum (id:42), reason: complements moisturizers
   - [recommended] Mention free-shipping if cart < $50
   ```
3. Agent2 решает как интегрировать (или проигнорировать soft-hint если не вписывается).

### Хранение
Новая таблица `marketing_triggers`:
```sql
id UUID PRIMARY KEY,
tenant_id UUID NOT NULL,
name TEXT NOT NULL,
status TEXT DEFAULT 'draft', -- draft|active|paused
intent_text TEXT NOT NULL,    -- что написал маркетолог
compiled_rule JSONB NOT NULL, -- результат компиляции
preview_scenarios JSONB,       -- тестовые сценарии
created_at TIMESTAMPTZ,
updated_at TIMESTAMPTZ,
version INT DEFAULT 1
```

### Файлы для реализации
- `project_admin/frontend/src/features/triggers/` (новый раздел админки: список + редактор + превью)
- `project_admin/backend/internal/usecases/compile_trigger.go` (compile-time LLM-pass)
- `project_admin/backend/internal/adapters/postgres/triggers_adapter.go` (CRUD)
- `project_v4/backend/internal/triggers/matcher.go` (runtime matcher, новый пакет)
- `project_v4/backend/internal/usecases/pipeline_execute.go` (вызов matcher перед Agent2, передача hints в prompt)

### Открытые вопросы
- Triggers как отдельный раздел в админке или внутри v9 канвы как `doc-type: trigger`? Я бы делал **отдельный раздел** — другая ментальная модель (правила, не визуал).
- Превью-сценарии — синтетика (моки) или реальные исторические сессии? Начать с синтетики, добавить реальные потом.

---

## Часть 2 — Conversational Narration + Highlights

### Проблема
Юзер задаёт контекстный вопрос ("чем отличаются эти три ноута") → нужен **текст-объяснение + визуал + подсветка конкретных параметров на визуале**. Чисто LLM, контекстно. Отдельный третий агент — дорого.

### Решение
**Расширить output Agent2**, без третьего LLM-вызова. + дешёвый code-based fallback для тривиальных comparison'ов.

### Триггер
В Agent1 при разборе запроса — классификация. Если comparative/explainer → `state.needs_narration = true`.

**Сигналы** (старт с keyword'ов, потом LLM-классификатор):
- "чем отличаются", "что лучше", "почему", "посоветуй", "сравни", "какой выбрать", "разница между"
- Маркетинговый триггер с `requires_narration: true` в action

### Output Agent2 расширяется
Было:
```json
{ "ops": [...] }
```

Станет (когда `needs_narration: true`):
```json
{
  "ops": [...],
  "narration": "MacBook выигрывает по RAM (32 vs 16), но Lenovo дешевле на $400. Если важна автономность — Asus впереди (12ч против 8).",
  "highlights": [
    { "atom_id": "ram_cell_macbook", "style": "win" },
    { "atom_id": "ram_cell_lenovo", "style": "loss" },
    { "atom_id": "price_cell_lenovo", "style": "win" },
    { "atom_id": "battery_cell_asus", "style": "win" }
  ],
  "narration_slot": "top"
}
```

Поля опциональные. Без флага — Agent2 их не возвращает, поведение как раньше.

### Расширение схемы атома (v9 nodes.ts)
```ts
highlight?: {
  style: 'win' | 'loss' | 'neutral_focus' | 'pulse';
  intensity?: number; // 0..1, default 1
}
```

Применяется на фронте через CSS-классы:
- `win` — зелёная обводка 2px + лёгкий glow
- `loss` — красная обводка 2px
- `neutral_focus` — синяя обводка + pulse animation
- `pulse` — золотая обводка + pulse animation

### Narration slot
В v9-формейшне зарезервированный слот `narration_slot` (top/bottom/sidebar/inline). Маркетолог в канве настраивает где он появится и стиль speech-bubble per-preset. Текст подставляется на runtime.

### Tier 1 — Code-based fallback (бесплатно)
Для тривиальных diff-сравнений — без LLM:
1. Pipeline видит `needs_narration: true` + intent `"diff"` (всё ещё классификация в Agent1)
2. Сравнивает массив товаров поле-в-поле
3. Программно генерит `highlights` (где значения отличаются → подсветить лучшее зелёным, худшее красным по типу поля — числовое больше=лучше, цена меньше=лучше)
4. Шаблон narration: `"{product_a} выигрывает по {field} ({val_a} vs {val_b})"`
5. Agent2 не зовётся для расширенного output — экономия

Покрывает ~60-70% comparison-кейсов. Tier 2 (Agent2 extended) — для запросов с reasoning ("если вам важна автономность...").

### Cost
- Tier 1: $0 (только код)
- Tier 2: +~300 output токенов = **+$0.0015/запрос на Haiku**
- Prompt caching: system + tools Agent2 в cache_control breakpoints → 10× дешевле input при cache hit

### Файлы для реализации
- `project_v4/backend/internal/usecases/agent1_execute.go` (добавить классификацию needs_narration + intent)
- `project_v4/backend/internal/prompts/agent2_v5.md` (новый блок промпта про narration + highlights когда флаг есть)
- `project_v4/backend/internal/engine_v5/highlights_diff.go` (Tier 1 — программный diff-engine, новый файл)
- `project_v9/packages/domain/src/entities/nodes.ts` (поле highlight? на ноде)
- `project/frontend/src/entities/pencil/PencilNodeRenderer.jsx` (применение highlight CSS-классов)
- `project/frontend/src/widgets/narration-bubble/` (компонент speech-bubble, новая папка)
- `project/frontend/src/shared/styles/highlights.css` (4 стиля подсветки)

### Открытые вопросы
- Классификатор `needs_narration` — keyword'ы достаточно или сразу LLM в Agent1? Старт на keyword'ах, проверить precision/recall, добавить LLM если нужно.
- Сколько highlight-стилей — 4 хватит? Можно начать с 2 (win/loss), добавить остальные по потребности.
- `narration_slot` — кто решает где? Маркетолог в канве или Agent2 динамически? Гибрид: дефолт от пресета, Agent2 может переопределить через `narration_slot` в output.

---

## Связь между двумя системами

Триггеры могут включать narration. Например в `compiled_rule.action`:
```json
{
  "type": "hint_to_agent",
  "strength": "recommended",
  "requires_narration": true,
  "hint": "User is comparing skincare. Mention our vitamin-C serum naturally in the explanation if relevant."
}
```

→ Agent2 видит и hint, и `needs_narration: true`, естественно встраивает упоминание в narration текст ("кстати, наша новая сыворотка отлично подходит к этому крему").

Та же инфраструктура hints в pipeline. Один путь данных state → Agent2 prompt.

---

## Порядок реализации (приоритезация на сегодня)

Если делать **только narration сегодня** (самое полезное и быстрое):
1. Поле `needs_narration` + классификатор keyword'ами в Agent1 (~1ч)
2. Расширить prompt Agent2 опциональным блоком narration + highlights (~1.5ч)
3. Поле `highlight?` на v9 ноде + 4 CSS-класса на фронте + рендер (~1.5ч)
4. Тестировать на запросе "сравни эти три"

**Триггеры — отдельная задача на 3-4 дня минимум** (новая админ-страница + compile-time LLM + matcher + БД-таблица + превью). Не на сегодня.

**Tier 1 highlights_diff** — можно отложить, первая итерация полностью через Agent2 extended.
