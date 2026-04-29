# Pencil Engine V4 — Гипотеза параллельного проекта

> **Статус**: Гипотеза, готова к проверке
> **Дата**: 2026-04-03
> **Автор**: Vlad (product owner)
> **Контекст**: Решение принято после критического анализа текущего движка (V2 + ops/build)

---

## Зачем это нужно — проблема

Текущий движок (V2 + ops + build) **работает, но архитектурно нестабилен**:

1. **Ops/Build обходят V2 pipeline** — не проходят через constraints, пресеты, валидацию типов. Это корневая причина большинства багов.
2. **V1 fallback маскирует проблемы V2** — после каждой ops операции вызывается `WidgetV2ToLegacy()`, который конвертирует V2 → V1, теряя стили (например, цвет). Если фронт рендерит V1 — стили пропадают молча.
3. **Хардкод ограничивает гибкость** — field ranking, slot assignment, layout algorithm, MaxFields зашиты в код. Agent не может создать произвольный UI.
4. **Промпт противоречит сам себе** — правило 1 говорит "используй field names", правило 3 — "таргетируй по ID". Schema ops mode неполная (7 wrapper types вместо 10, 6 textStyle полей вместо 8).
5. **JSON roundtrip теряет данные** — `nil` → DB → `{}`, wrapper заменяется вместо мержа, нет оптимистичной блокировки.

Проблема **не в отдельных багах** — проблема в том, что ops/build были добавлены поверх V2 engine как надстройка, а не как часть единого pipeline. Латать каждый баг отдельно — бесконечная работа.

---

## Ключевая идея — перевёрнутая логика

### НЕ ТАК:
```
Keepstar V2 Engine (ограниченный, но быстрый)
  + добавляем гибкость от Pencil (ops, build, freestyle)
  = каша из двух архитектур
```

### А ТАК:
```
Pencil Engine (гибкий, может любой UI)
  + добавляем пресеты от Keepstar (мгновенная генерация)
  + добавляем data-binding от Keepstar (авто-заполнение данными)
  + добавляем constraints от Keepstar (авто-качество)
  + добавляем auto-resolve от Keepstar (LLM не думает о деталях)
  = движок уровня Pencil, но быстрее и дешевле
```

**Pencil — это фундамент.** Keepstar — это то, что делает Pencil быстрым, дешёвым и data-driven.

### Почему это важно понимать правильно

Pencil тратит десятки секунд и тысячи токенов потому что LLM строит каждую ноду вручную. Keepstar генерирует 23 карточки за миллисекунды. Но Keepstar ограничен e-commerce карточками.

**Пресеты и auto-resolve — это shortcut.** Когда есть пресет под задачу — используй его (мгновенно, дёшево). Когда нет — строй Pencil-стилем (гибко, но дороже). Комбинируй оба в одном запросе.

---

## Пример работы

```
Пользователь: "Покажи что есть для жирной кожи и сравни топ 3"

Agent1: находит 8 продуктов, фильтрует топ 3 отдельно

Agent2 смотрит на данные и решает:

  1. Сравнительная таблица (топ 3) — пресета нет
     → строим Pencil-стилем: I(frame), I(text, bind:"name"), I(number, bind:"price")...
     → data-binding заполняет значения из products[0:3]
     → constraints проверяют результат
     
  2. Карточки остальных (5 шт) — есть пресет product_card_grid
     → применяем пресет (мгновенно)
     → data-binding заполняет значения из products[3:8]  
     → Agent добавляет переопределение: badge "для жирной кожи"
     → constraints проверяют результат

Один tool call. < 1 секунда для пресетной части + LLM-время для freestyle части.
```

---

## Что конкретно от Pencil берётся как база

### Берём:
- **Generic node tree** — frame/text/image/icon как примитивы, не 6 фиксированных atom types
- **Flexbox layout** — gap, padding, justify, align управляются агентом, не хардкодом
- **Operations DSL** (I/U/D/M/R/C) — единый способ работы с деревом, не "auto vs ops vs build"
- **Binding system** — `foo=I(parent, {...})` → `U(foo+"/child", {...})` — ссылки на созданные ноды

### НЕ берём:
- .pen формат (зашифрованный) — используем свой JSON
- Pencil Editor — у нас React renderer
- Image generation (G() операция) — у нас данные из каталога
- Графические примитивы (polygon, path, line) — не нужны для UI виджетов

### Что от Keepstar прокачивает Pencil:

| Keepstar фишка | Зачем | Как работает |
|---|---|---|
| **Пресеты** | Мгновенная генерация без LLM-строительства | Preset = готовое поддерево с data-binding slots |
| **Data-binding** | Автоматическое заполнение данными | `bind: "price"` → движок подставляет из entity |
| **Auto-resolve** | LLM не думает о layout/size деталях | count=6 → grid 3x2, count=1 → single large |
| **Constraints** | Авто-качество без LLM-валидации | badge > 20 chars → tag, max 5 tags, consistency |
| **Field definitions** | Движок знает типы данных заранее | price = number/currency, images = image/url |
| **Entity mapping** | Один пресет = N виджетов с разными данными | WidgetTemplate × products[] → N виджетов |
| **Wildcard ops** | "Измени цену во ВСЕХ карточках" одной операцией | target: "price" → expand to all widgets |

---

## План проверки гипотезы

### Шаг 1: Параллельный проект (project_v4)

Создать `project_v4/backend/` рядом с `project/backend/`. Та же гексагональная архитектура.

**Копируется без изменений:**
- Agent1 (NLU/data) — usecases, tools (catalog_search, state_filter, history_lookup)
- Chat handlers, session management, API endpoints
- Adapters (Postgres, Anthropic, OpenAI)
- Domain entities (кроме Formation/Widget/Atom — они меняются)

**Пишется заново:**
- Engine — Pencil-based (generic nodes, flexbox, operations DSL)
- Visual assembly tool — новая schema, новый execute
- Agent2 prompt — под новый движок
- Formation model — дерево нод вместо atoms/widgets/zones
- Frontend renderer — рендерит новое дерево

**Адаптируется:**
- State management — тот же принцип, новый формат formation
- Presets — становятся готовыми поддеревьями с data-binding
- Constraints — переносятся, работают на новом дереве

### Шаг 2: MVP — "покажи крема"

Минимальный тест: пользователь пишет "покажи крема" → получает карточки товаров.
Но движок внутри — Pencil-based. Проверяем что можем сгенерировать любой UI.

**Ожидаемый результат шага 2:** можем сгенерировать любой UI, не обязательно подтягивая данные от Agent1.

### Шаг 3: Пресеты + data-binding + множественная генерация

Добавляем логику пресетов и наполнение данными. То, что подпадает под пресеты — делается по ним. Остальное — Pencil-стилем с переопределениями.

### Шаг 4: Метрики

Замеряем:
- Скорость (target: < 2 сек для пресетной генерации)
- Стоимость токенов (target: не дороже текущего)
- Гибкость (target: можем сделать то, что текущий движок не может)

Если метрики приемлемы и/или понятно как до них добежать — продолжаем. Если нет — останавливаемся, текущий движок остаётся.

---

## Чего НЕ делать (ловушки)

1. **Не "копировать Pencil 1:1"** — Pencil медленный и дорогой без пресетов. Нужна интеграция с Keepstar-логикой с первого дня.
2. **Не пытаться "починить текущий движок по пути"** — project_v4 живёт отдельно. Текущий project/ не трогаем.
3. **Не добавлять V1 compat** — нет V1, нет legacy, нет fallback. Только V4.
4. **Не усложнять хардкодом** — slots, ranking, maxFields НЕ зашивать в код. Всё в пресетах или конфигурации.
5. **Не разделять auto/ops/build** — один режим работы: операции над деревом. Пресет = макро-операция (вставить готовое поддерево). Freestyle = микро-операции (Insert нода за нодой).

---

## Связь с существующими спеками

- `PENCIL_CONVERGENCE_SPEC.md` — детальное сравнение Pencil и Keepstar V2. Полезно как справочник по различиям, но описывает подход "расширить Keepstar до Pencil" (не то что мы делаем).
- `PENCIL_HYBRID_ENGINE.md` — спека ops/build mode. Описывает то, что было реализовано и показало архитектурные проблемы. Полезно как анти-паттерн: не повторять разделение auto/ops/build.
- `V1_ENGINE_REMOVAL_SPEC.md` — план удаления V1. В project_v4 не нужен — V1 не копируется.

---

## Критический анализ текущего движка (для контекста)

Полный анализ проведён 2026-04-03. Ключевые находки:

### Ops mode обходит V2 pipeline:
- `executeOps()` в `tool_visual_assembly.go:673-740` не вызывает `EngineV2.Execute()`
- Пропускает: пресеты, atom overrides, budget/needs, viewport constraints (J1-J6)
- Inserted atoms получают `RigidityLocked` → все constraints пропускаются

### Schema mismatches (промпт vs схема vs движок):
- Wrapper types: промпт 10, ops schema 7, движок 10
- TextStyle fields: промпт 8, ops schema 6, движок 8
- Target field: промпт "use field names" AND "use IDs" (противоречие)
- Shorthand props: schema "MUST be nested", движок автоподнимает

### State/serialization:
- JSON roundtrip `nil` → `{}` (TextStyle, Wrapper, Grid)
- Wrapper заменяется при update, не мержится
- Нет оптимистичной блокировки — race condition на concurrent ops
- Widget-entity mapping позиционный — порядок изменился → данные от чужого продукта

### Хардкод ограничения:
- Field ranking (`defaults.go:16-22`) — images→name→price→rating навсегда
- Slot assignment (`defaults.go:45-64`) — price = AtomSlotPrice навсегда  
- Layout algorithm (`auto_layout.go:38-238`) — определяется автоматически, agent не управляет
- MaxFields (`defaults.go:67-72`) — tiny=2, small=3, medium=5, large=10

---

## Для следующего агента / сессии

**Контекст**: Vlad — product owner (не инженер), но глубоко понимает архитектуру и принимает технические решения. Текущий движок V2 + ops/build работает, но архитектурно нестабилен из-за того, что ops/build были надстройкой поверх V2, а не частью единого pipeline.

**Гипотеза**: взять архитектуру Pencil (generic nodes, flexbox, operations DSL) как фундамент и добавить инновации Keepstar (пресеты, data-binding, auto-resolve, constraints) как ускорители. НЕ наоборот. Это не "Keepstar + гибкость Pencil", а "Pencil + скорость/дешевизна Keepstar".

**Подход**: параллельный проект (project_v4), не рефакторинг существующего. Текущий project/ не трогается.

**Ключевая ошибка прошлого**: разделение на auto/ops/build mode создало три параллельных пути, которые по-разному обрабатывают данные. В V4 должен быть ОДИН режим: операции над деревом. Пресет = макро-операция.

**Справочные материалы**:
- Pencil MCP architecture: см. memory `pencil_mcp_reverse_engineering.md` или вызвать `get_guidelines(include_schema:true)` через MCP
- Текущий движок: `project/backend/internal/engine/`, `project/backend/internal/tools/tool_visual_assembly.go`
- Существующие спеки: `docs/New features/PENCIL_CONVERGENCE_SPEC.md`, `PENCIL_HYBRID_ENGINE.md`
