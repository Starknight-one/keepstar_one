# Project Status — 2026-03-01

## Что работает

### Core Pipeline (end-to-end)
- User query → Agent1 (NLU, tool calling) → Agent2 (visual_assembly) → Formation JSON → Frontend render
- Hybrid search: SQL ILIKE + pgvector + RRF merge
- Session management: create, restore (localStorage, TTL 30 min)
- Navigation: expand (<16ms), back (<16ms), view stack — без LLM
- Prompt caching: system + tools + conversation (TTL 5 min)
- Model: Claude Haiku 4.5, Embeddings: OpenAI text-embedding-3-small

### Visual Assembly Engine
- 12 primitives: show, hide, display, format, color, size, shape, order, layer, anchor, direction, place
- Defaults Engine (AutoResolve): авто-выбор fields/layout/size по entity type + count
- Constraints: 30+ правил, 4 уровня
- 17 пресетов (9 visual assembly + 5 legacy product + 3 service)
- Layout Engine: zone-based (hero, stack, row, flow, buttons, collapsed)
- Тесты: 49+, 327K structural combos, 10K fuzz — всё зелёное

### Admin Panel
| Фича | Статус |
|------|--------|
| Auth (JWT) | Работает |
| Каталог товаров (CRUD) | Работает |
| Импорт JSON | Работает |
| Widget embed-код | Работает |
| Testbench | Работает |
| Обогащение (LLM PIM) | Только бэкенд, UI нет |
| Сервисы | Только бэкенд, UI нет |

### Frontend (Chat Widget)
- Shadow DOM, IIFE bundle, 7 widget templates
- FormationRenderer → WidgetRenderer → AtomRenderer
- Instant expand (adjacentTemplates), session cache

---

## visual_assembly — как работает тулла

Одна тулла у Agent2. Все параметры опциональные.

### Пример вызовов
```
visual_assembly()                                              — дефолты, движок сам решает
visual_assembly(layout: "list")                                — сменить на список
visual_assembly(show: ["description"], display: {"brand":"badge"}, color: {"price":"green"})
visual_assembly(preset: "product_card_detail")                 — пресет как база
```

### Что может принять
| Параметр | Тип | Описание |
|----------|-----|----------|
| preset | string | Один из 17 пресетов (поля + layout + size) |
| show | string[] | Добавить поля к текущим (append) |
| hide | string[] | Убрать поля |
| display | {field: style} | Визуальный враппер: badge, tag, h1, price, body-sm... |
| format | {field: fmt} | Формат значения: currency, stars, stars-text, percent... |
| layout | string | grid / list / single / carousel / comparison / table |
| size | string или object | "large" всем, или {"images":"xl","price":"lg"} per-field |
| order | string[] | Порядок полей |
| color | {field: color} | green, red, blue, orange, purple, gray + hex |
| direction | string | vertical / horizontal |
| shape | {field: shape} | pill / rounded / square / circle |
| layer | {field: z} | z-index |
| anchor | {field: pos} | top-left, top-right, bottom-left, bottom-right, center |
| place | string | sticky / floating / default |
| compose | array | Мульти-секции (код есть, не тестировано) |
| conditional | array | Условные стили (код есть, не тестировано) |
| limit/offset | number | Пагинация (код есть, не тестировано) |

### Пайплайн внутри тулы (19 шагов)
1. validateInput — санитизация
2. GetState из БД — берёт products/services
3. Определение entityType + count
4. AutoResolve — дефолты по count
5. Патч из currentConfig — если на экране уже что-то есть, сохраняет настройки
6. Загрузка пресета (если передан)
7. show/hide
8. display overrides
9. order
10. layout/size + **layout guard** (не меняет layout если юзер не просил)
11. Парсинг color/direction/shape/layer/anchor/place/format/pagination
12. BuildFieldConfigs с format inference
13. SlotConstraints + MaxAtomsPerSize
14. buildVisualWidgets — сборка Atom[] из данных
15. Constraints: atom → widget → cross-widget
16. conditional styling
17. CalculateZones
18. applyPostProcessing (color/size/shape/layer/anchor/direction/place/pagination)
19. writeFormation в стейт

### visual_assembly() без параметров
При пустом вызове AutoResolve решает всё сам:
- 1 товар → single, large, ВСЕ поля
- 2-6 → grid, medium, 5 полей (images, name, price, rating, brand)
- 7-12 → grid, small, 3 поля (images, name, price)
- 13+ → grid, small, 3 поля

**Проблема**: кол-во показываемых полей жёстко привязано к кол-ву товаров. Нашли 10 — юзер видит только картинку, имя и цену. Без рейтинга, без бренда. Если Agent2 передаст show: ["brand"] — ограничение снимается, но по дефолту оно жёсткое.

### Контекст который получает Agent2
```json
{
  "productCount": 5,
  "serviceCount": 0,
  "fields": ["images","name","price","rating","brand"],
  "view_mode": "grid",
  "current_formation": { "mode": "grid", "size": "medium", "fields": [...] },
  "screen_state": { "mode": "grid", "widget_count": 5 },
  "display_meta": [{ "name": "images", "category": "media", "default": "image-cover" }, ...],
  "user_request": "покажи бренд красным",
  "data_change": null | { "tool": "catalog_search", "count": 5 },
  "history_summary": "step 1: catalog_search → 5 items",
  "microcontext": "new_search: 5 items found"
}
```

~2500 токенов input (при кеше ~90% = cache read = дёшево)

### Дельты
Append-only лог изменений стейта. Каждое действие Agent1/Agent2/навигации записывается.
Agent2 видит последнюю дельту как `data_change` + последние 10 как `history_summary`.

### Microcontext
Одна строка-сигнал: "new_search: 23 items found" / "filtered: 5 items" / "no_data_change".
Дублирует data_change — можно будет убрать одно из двух.

---

## Найденные баги в промпте Agent2

Файл: `project/backend/internal/prompts/prompt_compose_widgets.go`

1. **Правило 6**: "If data_change=null — DON'T pass layout, DON'T pass show/hide unless explicitly asked" — некорректно. Если юзер просит "покажи списком" и данные не менялись — data_change=null, но layout менять НУЖНО. "unless explicitly asked" может не спасти — LLM может проигнорировать.

---

## Что чиним (решено)

### 1. AutoResolve — переделать правила
Текущие: кол-во товаров → size → кол-во полей. Это плохо — 10 товаров = 3 поля.
Нужно: разумные дефолты независимо от количества. Layout зависит от count, поля — нет.

### 2. Фронтенд — чинить постоянно
- CSS хаос: карточки расползаются, нет max-width
- ComparisonTemplate сломан
- Нет fallback для битых картинок
- Мобильный layout тесный
- Тёмный фон — контраст

### 3. Рефакторинг — вынести engine из tools/ в usecases/
Пока движок не трогаем архитектурно, но код перенести чтобы не путалось.

### 4. Кнопки действий
Нет cart, like, share. Продумать и добавить.

### 5. Промпт Agent2
Починить правило с data_change=null. Убрать дублирование microcontext/data_change.

## Что НЕ трогаем (решено)

- Движок архитектурно не дорабатываем
- compose, conditional, pagination, table — оставляем как есть
- zone overrides, responsive hints — потом
- Локальные LLM, биллинг, аналитика — потом
- Frontend тесты — потом

## Ключевые файлы

| Файл | Что |
|------|-----|
| `project/backend/internal/tools/tool_visual_assembly.go` | Тулла (1044 LOC, 19 шагов) |
| `project/backend/internal/tools/defaults_engine.go` | AutoResolve, field ranking, display defaults |
| `project/backend/internal/tools/constraints.go` | 30+ правил, 4 уровня |
| `project/backend/internal/tools/tool_render_preset.go` | Сборка атомов из данных |
| `project/backend/internal/tools/layout_engine.go` | Зонирование виджетов |
| `project/backend/internal/presets/visual_assembly_presets.go` | 17 пресетов |
| `project/backend/internal/prompts/prompt_compose_widgets.go` | Промпт Agent2 (220 строк) |
| `project/backend/internal/usecases/agent2_execute.go` | Вызов Agent2, контекст |
| `project/backend/internal/usecases/pipeline_execute.go` | Оркестрация, microcontext |
