# Pencil Hybrid Engine — Spec

## Проблема

Текущий движок пересоздаёт formation с нуля каждый turn. Agent2 не может модифицировать дерево — только передавать параметры (show/hide/size/atoms) в пайплайн, который строит всё заново. Это делает невозможным:
- Точечные изменения ("добавь divider между полями")
- Составные страницы ("hero + grid + CTA")
- Freestyle контент ("заголовок + кнопка, без данных")

## Решение

Скопировать операционную модель Pencil MCP (I/U/D/M на дереве) и добавить data binding. Два режима в одном tool:

1. **Auto** — пайплайн создаёт дерево из данных + пресет (как сейчас, но дерево с ID и персистится)
2. **Ops** — agent шлёт операции над существующим деревом (как Pencil batch_design)

---

## 1. Модель дерева

Всё — нода с ID. Иерархия:

```
Formation (root)
 ├── Section "s1"
 │   ├── mode: "grid", columns: 3
 │   ├── dataSource: "products[0..N]"      ← data binding
 │   └── WidgetTemplate "wt1"              ← один шаблон, размножается по данным
 │       ├── LayoutGroup "hero" (span)
 │       │   └── Atom "a1" (image, bind:"images")
 │       ├── LayoutGroup "content" (column)
 │       │   ├── Atom "a2" (text, bind:"name", textStyle:{fontSize:"xl"})
 │       │   └── Atom "a3" (number, bind:"price", format:"currency")
 │       └── LayoutGroup "tags" (flow)
 │           └── Atom "a4" (text, bind:"brand", wrapper:{type:"badge"})
 │
 ├── Section "s2" (freestyle, без данных)
 │   └── Widget "w-cta"
 │       ├── Atom "a5" (text, value:"Лучшее для тебя", textStyle:{fontSize:"3xl"})
 │       └── Atom "a6" (text, value:"Купить", wrapper:{type:"button",variant:"primary"})
```

### Ключевые концепции:

**WidgetTemplate vs Widget**: WidgetTemplate содержит bind-атомы (ссылки на поля данных). Движок размножает шаблон × данные → рендеренные Widget-ы. Agent оперирует шаблоном — изменение в шаблоне применяется ко всем виджетам.

**Widget** (без Template): литеральные значения, не привязан к данным. Для freestyle контента (CTA, заголовки, кастомные блоки).

**Section**: группа виджетов с общим layout (mode/columns/gap). Formation = массив секций.

---

## 2. Структуры данных

### Node — базовая единица дерева

```go
// NodeID — стабильный идентификатор ноды (генерится при создании, не меняется)
type NodeID string

// NodeType — тип ноды в дереве
type NodeType string
const (
    NodeTypeFormation     NodeType = "formation"
    NodeTypeSection       NodeType = "section"
    NodeTypeWidgetTemplate NodeType = "widget_template"  // размножается по данным
    NodeTypeWidget        NodeType = "widget"             // литеральный (freestyle)
    NodeTypeLayoutGroup   NodeType = "layout_group"
    NodeTypeAtom          NodeType = "atom"
)
```

### Atom — лист дерева (отображает данные или литерал)

```go
type AtomNode struct {
    ID        NodeID                 `json:"id"`
    Type      AtomType               `json:"type"`       // text, number, image, icon, divider, spacer, ...
    Subtype   AtomSubtype            `json:"subtype,omitempty"`

    // Data binding (для WidgetTemplate) — ИЛИ Value (для freestyle)
    Bind      string                 `json:"bind,omitempty"`   // "name", "price", "images" — ссылка на поле данных
    Value     interface{}            `json:"value,omitempty"`  // литеральное значение (если bind пустой)

    // Стили (как текущий AtomV2)
    TextStyle  *TextStyle            `json:"textStyle,omitempty"`
    Wrapper    *WrapperConfig        `json:"wrapper,omitempty"`
    MediaStyle *MediaStyle           `json:"mediaStyle,omitempty"`
    IconStyle  *IconStyle            `json:"iconStyle,omitempty"`
    Format     AtomFormat            `json:"format,omitempty"`
    Label      string                `json:"label,omitempty"`
    Rigidity   Rigidity              `json:"rigidity,omitempty"`
    Meta       map[string]interface{} `json:"meta,omitempty"`
}
```

### LayoutGroup — контейнер с flex layout

```go
type LayoutGroupNode struct {
    ID       NodeID          `json:"id"`
    Type     LayoutNodeType  `json:"type"`     // row, column, span, flow
    Name     string          `json:"name,omitempty"`
    Children []NodeID        `json:"children"` // ID дочерних нод (atom или layout_group)

    // Flex props
    Gap          string `json:"gap,omitempty"`
    Align        string `json:"align,omitempty"`
    Distribution string `json:"distribution,omitempty"`
    Wrap         bool   `json:"wrap,omitempty"`

    // Visual container
    Background   string `json:"background,omitempty"`
    Padding      string `json:"padding,omitempty"`
    BorderRadius string `json:"borderRadius,omitempty"`
    Shadow       string `json:"shadow,omitempty"`
    Border       string `json:"border,omitempty"`
}
```

### WidgetTemplate — шаблон виджета (размножается по данным)

```go
type WidgetTemplateNode struct {
    ID       NodeID   `json:"id"`
    Root     NodeID   `json:"root"`       // ID корневого LayoutGroup
    Size     WidgetSize `json:"size,omitempty"`
    Template string   `json:"template,omitempty"` // "GenericCard", etc.
}
```

### Section — группа виджетов с layout

```go
type SectionNode struct {
    ID         NodeID        `json:"id"`
    Mode       FormationType `json:"mode"`            // grid, list, single, carousel
    Columns    int           `json:"columns,omitempty"`
    Gap        string        `json:"gap,omitempty"`
    Label      string        `json:"label,omitempty"`
    Background string        `json:"background,omitempty"`
    Padding    string        `json:"padding,omitempty"`

    // Источник данных
    DataSource string        `json:"dataSource,omitempty"` // "products[all]", "products[0]", "products[1-3]", "" (freestyle)

    // Содержимое — WidgetTemplate (data-driven) или Widget[] (freestyle)
    WidgetTemplate NodeID    `json:"widgetTemplate,omitempty"` // ID шаблона
    Widgets        []NodeID  `json:"widgets,omitempty"`        // freestyle виджеты (без binding)
}
```

### Formation — корень дерева

```go
type FormationNode struct {
    ID       NodeID   `json:"id"`
    Sections []NodeID `json:"sections"`

    // Плоский реестр всех нод (для быстрого доступа по ID)
    Nodes map[NodeID]interface{} `json:"nodes"` // AtomNode | LayoutGroupNode | WidgetTemplateNode | SectionNode | ...
}
```

---

## 3. Операции (Pencil-like)

Agent2 шлёт массив операций. Движок применяет последовательно, constraints валидируют после.

### Insert — создать ноду

```json
{"op": "I", "parent": "lg-content", "node": {"type": "atom", "atomType": "divider"}, "after": "a2"}
```
→ Создаёт divider в LayoutGroup "content", после атома "a2" (name). Применяется к шаблону → появляется во всех рендеренных виджетах.

### Update — изменить свойства

```json
{"op": "U", "id": "a3", "props": {"textStyle": {"fontSize": "2xl", "fontWeight": "bold"}}}
```
→ Делает price крупнее во всех виджетах.

### Delete — удалить ноду

```json
{"op": "D", "id": "s2"}
```
→ Удаляет секцию CTA и все её дочерние ноды.

### Move — переместить ноду

```json
{"op": "M", "id": "a4", "parent": "lg-content", "after": "a2"}
```
→ Перемещает brand badge из tags group в content group, после name.

### Replace — заменить ноду целиком

```json
{"op": "R", "id": "a4", "node": {"type": "atom", "atomType": "text", "bind": "brand", "textStyle": {"fontSize": "lg"}}}
```
→ Заменяет badge на обычный текст.

### Batch (как Pencil batch_design)

```json
{
  "ops": [
    {"op": "I", "parent": "lg-content", "node": {"type": "atom", "atomType": "divider"}, "after": "a2"},
    {"op": "U", "id": "a3", "props": {"textStyle": {"fontSize": "2xl"}}},
    {"op": "I", "parent": "formation-root", "node": {"type": "section", "mode": "single", "dataSource": "none", "widgets": [...]}}
  ]
}
```

---

## 4. Data Binding + Рендеринг

### Процесс:

```
FormationNode (дерево с bind-атомами)
    │
    ▼  Engine: для каждой секции с dataSource
    │
    ├── WidgetTemplate × products[0..N]
    │   └── Для каждого продукта:
    │       └── Клонировать шаблон, подставить bind → value
    │       └── atom.bind="name" → atom.value="Очищающий крем"
    │       └── atom.bind="price" → atom.value=119000, format="currency"
    │
    ▼  Constraints (валидация)
    │
    ▼  FormationWithData (рендеренный результат → frontend)
```

### Bind resolution:

```go
func resolveBindings(template WidgetTemplateNode, data map[string]interface{}, nodes map[NodeID]interface{}) Widget {
    // 1. Clone all atoms from template
    // 2. For each atom with bind != "":
    //    atom.Value = data[atom.Bind]
    //    atom.FieldName = atom.Bind
    // 3. Build Widget with resolved atoms
}
```

---

## 5. Auto Mode (создание начального дерева)

"Покажи крема" → Agent2 вызывает `visual_assembly()` без ops.

Движок:
1. Загружает field definitions из БД
2. Создаёт WidgetTemplate с bind-атомами по field definitions
3. Создаёт одну Section с dataSource="products[all]"
4. Применяет пресет (если есть) — меняет стили/порядок атомов
5. Размножает template × данные
6. Прогоняет constraints
7. Сохраняет FormationNode в state (для будущих ops)
8. Отдаёт FormationWithData на фронт

**Важно**: Auto mode создаёт то же самое что и текущий пайплайн, но результат — дерево с ID, которое можно модифицировать.

---

## 6. Tool Interface

```go
// visual_assembly tool принимает ДВА режима:

// Режим 1: Auto (как сейчас + совместимый)
{"preset": "product_card_grid", "size": "large", "show": ["description"], ...}

// Режим 2: Ops (Pencil-like)
{"ops": [
    {"op": "I", "parent": "lg-content", "node": {"type": "atom", ...}, "after": "a2"},
    {"op": "U", "id": "a3", "props": {"textStyle": {"fontSize": "2xl"}}},
    ...
]}

// Режим 3: Build (создать дерево с нуля, для лендингов)
{"build": {
    "sections": [
        {"mode": "single", "dataSource": "products[0]", "template": {...}},
        {"mode": "grid", "columns": 3, "dataSource": "products[1-5]"},
        {"mode": "none", "widgets": [{"atoms": [{"type": "text", "value": "CTA"}]}]}
    ]
}}
```

---

## 7. Что остаётся без изменений

- **Agent1** — NLU, catalog_search, state_filter. Не трогаем.
- **Frontend** — FormationRenderer, WidgetRenderer, AtomV2Renderer. Рендерят FormationWithData как сейчас. Sections рендеринг уже есть.
- **Типы атомов** — все 6 (text, number, image, icon, video, audio). Стили (TextStyle, MediaStyle, etc.) без изменений.
- **Field definitions** — catalog.field_definitions, FieldDefinitionPort. Без изменений.
- **Двухагентный пайплайн** — Agent1 → Agent2 → Tool. Без изменений.
- **State management** — SessionState, Delta, UpdateTemplate. Без изменений (formation пишется в state.template как и сейчас).

## 8. Что меняется

| Компонент | Сейчас | Станет |
|-----------|--------|--------|
| Дерево | Пересоздаётся с нуля каждый turn | Персистится, модифицируется операциями |
| Атомы | AtomV2 с Value | AtomNode с Bind или Value |
| Виджеты | Widget[] (плоский массив) | WidgetTemplate (шаблон) + рендеренные Widget[] |
| Formation | FormationWithData (плоский) | FormationNode (дерево с ID) → рендерится в FormationWithData |
| Tool input | show/hide/size/atoms overrides | Auto params ИЛИ ops batch ИЛИ build spec |
| Tool execute | Всегда пайплайн с нуля | Auto → пайплайн, Ops → modify tree, Build → construct tree |
| Engine | 10-step pipeline | Pipeline (для auto) + OpExecutor (для ops) + Renderer (binding → formation) |
| Agent2 prompt | "Передай параметры" | "Передай параметры ИЛИ операции" |
| Constraints | Часть пайплайна | Post-operation validator (после любого изменения) |

## 9. Порядок реализации

### Phase 1: Node Model + Auto Mode
- Новые структуры (FormationNode, AtomNode, etc.)
- Auto mode: текущий пайплайн → создаёт FormationNode вместо FormationWithData
- Renderer: FormationNode → FormationWithData (bind resolution)
- Дерево сохраняется в state
- **Результат**: работает как раньше, но formation — дерево с ID

### Phase 2: Operations
- OpExecutor: I/U/D/M/R на FormationNode
- Tool принимает ops массив
- Constraints после операций
- **Результат**: agent может модифицировать дерево

### Phase 3: Build Mode + Agent2 Prompt
- Build spec парсинг (создание дерева с нуля из JSON)
- Agent2 prompt обновлён с ops/build примерами
- **Результат**: лендинги, freestyle, кастом

### Phase 4: Presets (новый формат)
- Preset = сохранённый WidgetTemplate (JSON дерева)
- Auto mode подставляет preset как шаблон
- **Результат**: "покажи крема" → пресет × данные за миллисекунды
