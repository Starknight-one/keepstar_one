# Compose — Multi-Section Formation

## Проблема

Сейчас V2 движок рендерит одну плоскую формацию: N виджетов в одном layout (grid/list/carousel/single). Но пользователи часто хотят **составные** ответы:

- "Покажи самый дорогой слева, а справа остальные каруселью" → detail + carousel
- "Топ-3 крупно, остальные списком" → grid large + list small
- "Сравни первый и третий, остальные покажи мелко внизу" → comparison + grid
- "Покажи этот товар детально, а похожие рядом" → single + carousel
- "Покажи по категориям: кремы сеткой, сыворотки списком" → grid + list (разные данные)

Это не экзотика — это стандартный UX e-commerce: hero product + recommendations, featured + catalog, comparison + alternatives.

## Текущее состояние

### Что уже есть

```
domain/template_entity.go:
  FormationSection { Mode, Grid, Widgets, Label }
  FormationWithData { Mode, Grid, Widgets, Sections, Pagination }

FormationRenderer.jsx:
  if (sections?.length > 0) → рекурсивный рендер каждой секции

engine/assembly.go (V1, не портировано в V2):
  BuildComposedFormation() — секции из compose[] параметра
```

Frontend **уже умеет** рендерить секции. Domain struct **уже содержит** Sections. Нужно:
1. Параметр в AgentInstructions
2. Логика в V2 engine
3. Agent2 промпт
4. CSS для layout секций

### Чего нет

- V2 engine не знает про compose
- Agent2 tool schema не содержит compose
- CSS `.formation-composed` есть но примитивный (вертикальный стек)
- Нет горизонтального layout секций (side-by-side)

---

## Архитектура

### 1. AgentInstructions — новое поле `Sections`

```go
// instructions.go
type AgentInstructions struct {
    // ... существующие поля ...

    // Compose: multi-section formation
    // Когда задано — движок создаёт FormationSection для каждого элемента
    // Каждая секция получает свой subset данных и свой layout
    Sections []SectionInstruction `json:"sections,omitempty"`
}

type SectionInstruction struct {
    Layout    string   `json:"layout"`              // grid/list/single/carousel
    Size      string   `json:"size,omitempty"`       // tiny/small/medium/large
    Limit     int      `json:"limit,omitempty"`      // Max items in this section
    Offset    int      `json:"offset,omitempty"`     // Skip N items
    Show      []string `json:"show,omitempty"`       // Field visibility override
    Hide      []string `json:"hide,omitempty"`
    Label     string   `json:"label,omitempty"`      // "Самый дорогой", "Похожие"
    Direction string   `json:"direction,omitempty"`  // vertical/horizontal
    Sort      string   `json:"sort,omitempty"`       // price_asc/price_desc/rating_desc/name_asc
}
```

### 2. Tool Schema — параметр `sections`

```json
"sections": {
  "type": "array",
  "description": "Multi-section layout. Each section gets its own layout and subset of data.",
  "items": {
    "type": "object",
    "properties": {
      "layout":    { "type": "string", "enum": ["grid","list","single","carousel"] },
      "size":      { "type": "string", "enum": ["tiny","small","medium","large"] },
      "limit":     { "type": "integer", "minimum": 1 },
      "offset":    { "type": "integer", "minimum": 0 },
      "show":      { "type": "array", "items": { "type": "string" } },
      "hide":      { "type": "array", "items": { "type": "string" } },
      "label":     { "type": "string" },
      "direction": { "type": "string", "enum": ["vertical","horizontal"] },
      "sort":      { "type": "string", "enum": ["price_asc","price_desc","rating_desc","name_asc"] }
    },
    "required": ["layout"]
  }
}
```

### 3. Formation Layout — `sectionDirection`

```json
"sectionDirection": {
  "type": "string",
  "enum": ["vertical", "horizontal"],
  "description": "How sections are arranged: vertical (stacked) or horizontal (side-by-side)"
}
```

Добавляется в `AgentInstructions` и прокидывается в formation metadata для frontend.

### 4. Engine Pipeline — `ExecuteComposed`

Когда `Instructions.Sections` не пустой, engine переключается на composed path:

```
ExecuteComposed(input):
  1. Сортировка данных (если sort в первой секции)
  2. Для каждой SectionInstruction:
     a. Slice products по limit/offset
     b. Создать sub-input с section-specific instructions
     c. Вызвать стандартный Execute() pipeline для секции
     d. Собрать FormationSection из результата
  3. Собрать FormationWithData с Sections[]
  4. Записать sectionDirection в formation meta
```

**Ключевой принцип**: каждая секция — полноценный прогон через тот же pipeline (AutoResolve, presets, layout tree, rules). Не дублируем логику.

```go
func (e *EngineV2) ExecuteComposed(input EngineV2Input) EngineV2Output {
    sections := make([]domain.FormationSection, 0, len(input.Instructions.Sections))

    // Sort all products if needed (first section's sort applies globally)
    products := sortProducts(input.Products, input.Instructions.Sections[0].Sort)

    offset := 0
    for _, si := range input.Instructions.Sections {
        // Determine data slice
        sectionOffset := si.Offset
        if sectionOffset == 0 {
            sectionOffset = offset // auto-advance
        }
        limit := si.Limit
        if limit == 0 {
            limit = len(products) - sectionOffset // rest
        }
        sectionProducts := sliceProducts(products, sectionOffset, limit)
        offset = sectionOffset + len(sectionProducts) // advance for next section

        // Build section-specific instructions
        sectionInstr := &AgentInstructions{
            Layout:    si.Layout,
            Size:      si.Size,
            Show:      si.Show,
            Hide:      si.Hide,
            Direction: si.Direction,
            Limit:     limit,
        }

        // Execute standard pipeline for this section
        sectionInput := EngineV2Input{
            EntityType:   input.EntityType,
            Products:     sectionProducts,
            FieldDefs:    input.FieldDefs,
            Instructions: sectionInstr,
            Preset:       input.Preset,
            Viewport:     input.Viewport,
        }
        sectionOutput := e.Execute(sectionInput)

        sections = append(sections, domain.FormationSection{
            Mode:    sectionOutput.Formation.Mode,
            Grid:    sectionOutput.Formation.Grid,
            Widgets: sectionOutput.Formation.Widgets,
            Label:   si.Label,
        })
    }

    return EngineV2Output{
        Formation: &domain.FormationWithData{
            Mode:     domain.FormationTypeGrid, // container mode
            Sections: sections,
        },
    }
}
```

### 5. Frontend — section layout

CSS для горизонтального layout секций:

```css
/* Вертикальный стек (default) */
.formation-composed {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

/* Горизонтальный (side-by-side) */
.formation-composed.direction-horizontal {
  flex-direction: row;
  align-items: flex-start;
}

.formation-composed.direction-horizontal > .formation-section {
  flex: 1;
  min-width: 0;
}

/* Фиксированная ширина для single-секций в horizontal */
.formation-composed.direction-horizontal > .formation-section.section-single {
  flex: 0 0 auto;
  width: 45%;
  max-width: 300px;
}

.formation-section-label {
  font-size: 14px;
  font-weight: 600;
  color: #64748b;
  padding: 4px 0 8px;
  text-transform: uppercase;
  letter-spacing: 0.5px;
}
```

`FormationRenderer.jsx` — добавить:
- `sectionDirection` из formation meta → класс `direction-horizontal`
- `section-${mode}` класс на каждую секцию для CSS targeting

### 6. Sorting

```go
func sortProducts(products []domain.Product, sort string) []domain.Product {
    sorted := make([]domain.Product, len(products))
    copy(sorted, products)

    switch sort {
    case "price_asc":
        slices.SortFunc(sorted, func(a, b domain.Product) int {
            return cmp.Compare(a.Price, b.Price)
        })
    case "price_desc":
        slices.SortFunc(sorted, func(a, b domain.Product) int {
            return cmp.Compare(b.Price, a.Price)
        })
    case "rating_desc":
        slices.SortFunc(sorted, func(a, b domain.Product) int {
            return cmp.Compare(b.Rating, a.Rating)
        })
    case "name_asc":
        slices.SortFunc(sorted, func(a, b domain.Product) int {
            return strings.Compare(a.Name, b.Name)
        })
    }
    return sorted
}
```

---

## Agent2 Prompt — когда использовать sections

```
## SECTIONS (multi-section layout)

Use `sections` when the user asks for DIFFERENT layouts for different subsets:
- "Покажи самый дорогой крупно, остальные каруселью"
- "Сравни первые два, остальные покажи сеткой"
- "Топ-3 крупными карточками, остальные списком"

Each section gets its own layout, size, and data slice.
Sections share the same data — use limit/offset to split.
Sort applies to ALL data before slicing.

Parameters:
  sections: array of { layout, size, limit, offset, show, hide, label, sort }
  sectionDirection: "vertical" (stacked) or "horizontal" (side-by-side)

Examples:

User: "Покажи самый дорогой слева, справа остальные каруселью"
→ sectionDirection: "horizontal"
  sections: [
    { layout: "single", limit: 1, size: "large", sort: "price_desc", label: "Самый дорогой" },
    { layout: "carousel", size: "small" }
  ]

User: "Топ-3 крупно, остальные мелко списком"
→ sections: [
    { layout: "grid", limit: 3, size: "large", label: "Лучшие" },
    { layout: "list", size: "small", label: "Остальные" }
  ]

User: "Сравни первые два, покажи остальные сеткой"
→ sections: [
    { layout: "comparison", limit: 2 },
    { layout: "grid", size: "small" }
  ]

DO NOT use sections for simple requests like "покажи сеткой" or "покажи крупнее".
Only use when user explicitly asks for DIFFERENT presentations of DIFFERENT subsets.
```

---

## Примеры (input → output)

### Кейс 1: Hero + carousel

**Запрос**: "Покажи самый дорогой слева, справа остальные каруселью"

**Agent2 toolInput**:
```json
{
  "sectionDirection": "horizontal",
  "sections": [
    { "layout": "single", "limit": 1, "size": "large", "sort": "price_desc" },
    { "layout": "carousel", "size": "small" }
  ]
}
```

**Formation output**:
```json
{
  "mode": "grid",
  "sections": [
    {
      "mode": "single",
      "label": "",
      "widgets": [{ /* самый дорогой, large, 10 полей */ }]
    },
    {
      "mode": "carousel",
      "widgets": [{ /* 2й */ }, { /* 3й */ }, ...]
    }
  ],
  "meta": { "sectionDirection": "horizontal" }
}
```

**Визуально**:
```
┌──────────────────┬──────────────────────────┐
│                  │  ← [product2] [product3] →│
│   САМЫЙ ДОРОГОЙ  │     carousel scroll       │
│   (detail card)  │                            │
│                  │                            │
└──────────────────┴──────────────────────────┘
```

### Кейс 2: Featured + catalog

**Запрос**: "Покажи топ-3 крупно с описанием, остальные мелко"

**Agent2 toolInput**:
```json
{
  "sections": [
    { "layout": "grid", "limit": 3, "size": "large", "show": ["description"], "label": "Лучшие" },
    { "layout": "grid", "size": "small", "label": "Все остальные" }
  ]
}
```

**Визуально**:
```
Лучшие
┌─────────┐ ┌─────────┐ ┌─────────┐
│ product1│ │ product2│ │ product3│
│ (large) │ │ (large) │ │ (large) │
│ +descr  │ │ +descr  │ │ +descr  │
└─────────┘ └─────────┘ └─────────┘

Все остальные
┌────┐ ┌────┐ ┌────┐ ┌────┐ ┌────┐
│ p4 │ │ p5 │ │ p6 │ │ p7 │ │ p8 │
└────┘ └────┘ └────┘ └────┘ └────┘
```

### Кейс 3: Comparison + alternatives

**Запрос**: "Сравни два самых дешёвых, остальные покажи каруселью"

```json
{
  "sections": [
    { "layout": "comparison", "limit": 2, "sort": "price_asc" },
    { "layout": "carousel", "size": "small", "label": "Другие варианты" }
  ]
}
```

---

## План реализации

### Фаза 1: Backend (engine + tool)
1. `SectionInstruction` struct в `instructions.go`
2. `sortProducts()` в `engine_v2.go`
3. `ExecuteComposed()` в `engine_v2.go` — вызывает `Execute()` для каждой секции
4. Роутинг в `Execute()`: если `len(Instructions.Sections) > 0` → `ExecuteComposed()`
5. `sectionDirection` в AgentInstructions + прокидывание в formation meta
6. `sections` параметр в `definitionV2()` tool schema
7. `parseSections()` в `parseV2Input()`

### Фаза 2: Frontend
8. `FormationRenderer.jsx` — `sectionDirection` → CSS класс
9. `Formation.css` — horizontal layout, section-single width cap
10. Section label styling

### Фаза 3: Agent2 prompt
11. Секция SECTIONS в `Agent2ToolSystemPromptV2`
12. 3-4 примера
13. Guardrail: "DO NOT use sections for simple requests"

### Фаза 4: Testing
14. Unit тесты: ExecuteComposed с 2-3 секциями
15. E2E тесткейсы в docs/E2E_TEST_CASES.md

### Оценка
- Backend: ~200 LOC
- Frontend: ~50 LOC CSS + ~20 LOC JSX
- Prompt: ~40 строк
- Тесты: ~100 LOC
- **Итого: ~400 LOC, 1 сессия работы**

---

## Ограничения и edge cases

1. **Max 4 секции** — больше не имеет смысла визуально
2. **Секции делят один набор данных** — нет cross-search (каждая секция из своего запроса). Для этого нужен Agent1 multi-tool call
3. **Sort global** — сортировка применяется ко всем данным до slicing. Нельзя сортировать внутри секции отдельно (пока)
4. **sectionDirection: horizontal** — только для 2 секций. 3+ секций → fallback на vertical
5. **Comparison в секции** — работает, но comparison template занимает много места. В horizontal layout может быть тесно
6. **Mobile** — horizontal sections → fallback на vertical (viewport < 480px)
