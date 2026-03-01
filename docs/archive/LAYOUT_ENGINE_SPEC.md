# Layout Engine — Spec (v1, implemented)

**Branch:** `feature/layout-engine`
**Date:** 2026-02-26
**Status:** Implemented, fuzz-tested (327K+ combinations, 17 invariants, 0 failures)

---

## Что это

Backend после constraints вычисляет `zones[]` для каждого виджета. Фронт получает готовый рецепт и маппит зоны на 6 CSS-классов. Zero логики на фронте.

### Проблема (до)

GenericCardTemplate рендерил все атомы в `flex-direction: column` — каждый атом на отдельной строке. Теги стопкой, цена и рейтинг друг под другом, бейджи вертикально. Бэкенд знал всё об атомах (тип, display, slot), но фронт это игнорировал.

### Решение (после)

Бэкенд классифицирует каждый атом в bucket по display/type/slot, затем собирает зоны в фиксированном визуальном порядке. Фронт рендерит `<div class="zone-{type}">` — 6 CSS-классов покрывают все кейсы.

---

## Архитектура

### Pipeline (полный, с layout engine)

```
Данные (продукты/услуги)
  ↓
AutoResolve (товары × поля → layout + size)
  ↓
Field Configs + Format/Display
  ↓
Data Constraints (truncate, badges, tags, headings...)
  ↓
Conditional Styling
  ↓
Layout Engine — CalculateZones()     ← NEW
  ↓
Post-processing (color, meta, pagination)
  ↓
Formation JSON (widgets[] с zones[])
  ↓
Frontend — zone-based рендер (или legacy fallback)
```

### Точки интеграции

Layout engine вызывается в 3 местах:
1. **tool_visual_assembly.go** — стандартный путь (после constraints, перед post-processing)
2. **tool_visual_assembly.go** — composed formation (после buildVisualWidgets для каждой секции)
3. **handler_testbench.go** — testbench (после constraints для каждого виджета)

---

## Domain Types

**Файл:** `domain/widget_entity.go`

```go
type ZoneType string

const (
    ZoneHero      ZoneType = "hero"      // Full-width image
    ZoneRow       ZoneType = "row"       // Horizontal flex
    ZoneStack     ZoneType = "stack"     // Vertical flex
    ZoneFlow      ZoneType = "flow"      // Inline wrap (tags, badges)
    ZoneGrid      ZoneType = "grid"      // CSS grid N-col
    ZoneCollapsed ZoneType = "collapsed" // Hidden + fold toggle
)

type Zone struct {
    Type        ZoneType `json:"type"`
    AtomIndices []int    `json:"atomIndices"`
    Columns     int      `json:"columns,omitempty"`
    MaxVisible  int      `json:"maxVisible,omitempty"`
    FoldLabel   string   `json:"foldLabel,omitempty"`
}
```

Widget struct получил поле `Zones []Zone`.

---

## Правила классификации

**Файл:** `tools/layout_engine.go`

### CalculateZones(atoms, tokens) → zones[]

Каждый атом попадает ровно в один bucket:

| # | Условие | Bucket |
|---|---------|--------|
| 1 | `atom.Type == image` | heroIndices |
| 2 | `display ∈ {h1, h2, h3, h4}` | headingIndices |
| 3 | `atom.Slot == price` ИЛИ `display starts with "price"` | priceIndices |
| 4 | `display starts with "rating"` | ratingIndices |
| 5 | `display starts with "tag"` ИЛИ `display starts with "badge"` | flowIndices |
| 6 | `display ∈ {body-lg, body, body-sm, caption, divider, spacer, percent, progress}` | bodyIndices |
| 7 | `display starts with "button"` | buttonIndices |
| 8 | Всё остальное | otherIndices |

### Сборка зон (фиксированный порядок)

```
hero (если есть)         → Zone{type: "hero"}
headings (если есть)     → Zone{type: "stack"}
price + rating           → Zone{type: "row"}
body text                → Zone{type: "stack"}
flow (tags/badges):
  ≤ FoldMaxVisible       → Zone{type: "flow"}
  > FoldMaxVisible       → Zone{type: "flow", maxVisible: N}
                          + Zone{type: "collapsed", foldLabel: "+X ещё"}
buttons                  → Zone{type: "row"}
other                    → Zone{type: "stack"}
```

Пустые группы пропускаются.

### Design Tokens

```go
type DesignTokens struct {
    FoldMaxVisible int // default: 9
}
```

Единственный токен на данный момент. Расширяется по мере необходимости.

---

## Zone CSS (фронт)

**Файл:** `GenericCardTemplate.css`

| Zone | CSS |
|------|-----|
| `zone-hero` | `position: relative; width: 100%; overflow: hidden; background: #E4E4E7` |
| `zone-row` | `display: flex; align-items: center; gap: 8px` |
| `zone-stack` | `display: flex; flex-direction: column; gap: 6px` |
| `zone-flow` | `display: flex; flex-wrap: wrap; gap: 6px` |
| `zone-grid` | `display: grid; grid-template-columns: repeat(2, 1fr); gap: 8px` |
| `zone-collapsed` | Контейнер для fold toggle |

Fold toggle — кнопка "показать ещё" / "скрыть" с React state.

---

## Frontend

**Файл:** `GenericCardTemplate.jsx`

Два режима:
- **ZoneLayout** — если `zones[]` есть в JSON. Hero зоны → ImageCarousel (media). Остальные зоны → `.generic-card-content` внутри `<div class="zone-{type}">`.
- **LegacyLayout** — если `zones[]` нет. Текущее поведение (image top, content column). Backward compat.

Collapsed зона: скрыта по дефолту, кнопка toggle показывает/скрывает.

---

## Инварианты (fuzz-tested)

6 zone-инвариантов добавлены к существующим 11:

| # | Инвариант | Описание |
|---|-----------|----------|
| I12 | Valid indices | Все индексы в zones: 0 ≤ idx < len(atoms) |
| I13 | Partition | Каждый атом ровно в одной зоне (нет пропущенных, нет дублей) |
| I14 | Valid types | Все zone types из enum {hero, row, stack, flow, grid, collapsed} |
| I15 | Image → hero | Image атомы только в hero зонах |
| I16 | Collapsed after flow | Collapsed зона только после flow зоны |
| I17 | No empty zones | Все зоны содержат хотя бы один атом |

Все 17 инвариантов проходят на:
- 315,000 exhaustive structural combinations
- 2,392 exhaustive atom transform combinations
- 10,000 random fuzz combinations

---

## Файлы

### Созданные
- `backend/internal/tools/layout_engine.go` — CalculateZones + DesignTokens

### Изменённые
- `backend/internal/domain/widget_entity.go` — ZoneType, Zone struct, Widget.Zones
- `backend/internal/tools/tool_visual_assembly.go` — вызов CalculateZones (стандартный + composed)
- `backend/internal/handlers/handler_testbench.go` — вызов CalculateZones
- `backend/internal/tools/formation_fuzz_test.go` — CalculateZones в pipeline + I12-I17
- `frontend/src/entities/widget/WidgetRenderer.jsx` — zones prop
- `frontend/src/entities/widget/templates/GenericCardTemplate.jsx` — ZoneLayout + LegacyLayout
- `frontend/src/entities/widget/templates/GenericCardTemplate.css` — 6 zone CSS-классов + fold toggle

---

## Ограничения (текущие)

1. **Нет zone overrides** — Agent 2 не может переопределить группировку (например "рейтинг под ценой, а не рядом"). Нужен новый примитив `group` в visual_assembly tool.
2. **order работает внутри зоны** — `order` управляет порядком атомов, но не может перемещать атомы между зонами. Классификация по display/type фиксирована.
3. **Один design token** — FoldMaxVisible. Остальные токены (responsive breakpoints, zone gaps) пока хардкожены в CSS.
4. **Нет responsive** — Спека предполагала responsive hints (breakpoint → fallback: stack). Не реализовано — ждёт реальных мобильных кейсов.
5. **ZoneGrid не используется** — CSS-класс готов, но ни одно правило классификации не генерирует grid зону. Зарезервирован для будущих кейсов (например, сетка ингредиентов).
