# Engine V2: Полная спецификация движка

> Цель: visual assembly engine работает без пресетов. Пресеты — шорткаты, не костыли.
> Агент 2 может собрать любой UI из любых данных через tool call.

---

## Часть 1: Полный пайплайн (как должно работать)

```
Данные (state.data)
  │
  ▼
ФАЗА A: Определение атомов
  Вход: N сущностей с M полями каждая
  Что: определить type/subtype каждого поля, создать atom для каждого
  Выход: список typed atoms
  │
  ▼
ФАЗА B: Операции над атомами (12 примитивов)
  Вход: typed atoms + параметры от Agent 2
  Что: к каждому атому применить:
    B1. format — трансформация значения (1500 → "1 500 ₽")
    B2. display — визуальная обёртка (badge, h2, price, tag, button...)
    B3. color, size, shape — стилизация обёртки
  Выход: оформленные атомы с известными визуальными размерами
  │
  ▼
ФАЗА C: Constraints Level 1 — валидация атомов
  Вход: оформленные атомы
  Что: badge>20chars→tag, tag>40→body-sm, h1>60chars→h2, rating<3→compact, truncation
  Выход: нормализованные атомы
  │
  ▼
ФАЗА D: Layout внутри виджета (КЛЮЧЕВОЕ ИЗМЕНЕНИЕ)
  Вход: нормализованные атомы + structure от Agent 2 (или auto-layout)
  Что: расположить атомы друг относительно друга:
    D1. Если structure указана → парсить в layout tree
    D2. Если structure НЕ указана → auto-layout (улучшенные zones → tree)
    D3. В обоих случаях → layout tree с пропорциями
  Выход: Widget с Layout tree
  │
  ▼
ФАЗА E: Constraints Level 2 — валидация виджета
  Вход: Widget с Layout tree
  Что:
    E1. Существующие: max badges/tags/headings
    E2. НОВОЕ: row overflow → column (используя viewport + approx sizes)
    E3. НОВОЕ: пустые группы → удалить
    E4. НОВОЕ: одиночный ребёнок → развернуть
    E5. НОВОЕ: глубина вложенности > 3 → flatten
  Выход: валидный Widget
  │
  ▼
ФАЗА F: Сборка формации
  Вход: Widget[] + layout параметр (grid/list/single/...)
  Что: расположить виджеты друг относительно друга
  Выход: FormationWithData
  │
  ▼
ФАЗА G: Constraints Level 4 — валидация формации
  Вход: FormationWithData
  Что: нормализация field set в grid/list (C1), пагинация
  Выход: финальная FormationWithData
  │
  ▼
ФАЗА H: Post-processing
  Вход: FormationWithData
  Что: color→atom.Meta, shape→atom.Meta, conditional styling
  Выход: финальный JSON → фронтенд
```

---

## Часть 2: Что сломано сейчас (по фазам)

### Фаза A: Определение атомов — СЛОМАНО

**Проблема 1: FieldTypeMap — 14 захардкоженных полей.**
Поле `warranty`, `color_options`, `ingredients_full` → движок не знает их тип. Fallback на text/string есть (строка 321 defaults.go), но проблема глубже.

**Проблема 2: ProductFieldGetter — switch-case на 14 полей.**
Если поля нет в switch — getter возвращает nil, атом не создаётся. Product struct не имеет generic-поля.

**Проблема 3: AutoResolve — хардкоженный fieldRanking.**
Знает только `["images", "name", "price", "rating", "brand", ...]`. Если данные другие (категории, рецепты, статьи) — не сможет выбрать поля.

**Проблема 4: Slot назначается по имени поля, а не по type/subtype.**
`defaultSlot["price"] = price` — это хардкод. Атом-как-абстракция не должен знать что он "price поле". Slot должен выводиться из type+subtype+display, а не из имени.

### Фаза B: Операции над атомами — РАБОТАЕТ

12 примитивов реализованы. show/hide/display/format/color/size/shape/order/layer/anchor/direction/place. Agent 2 может задать любую комбинацию.

### Фаза C: Constraints Level 1 — РАБОТАЕТ

Per-atom constraints работают (badge→tag, truncation, heading downgrade). 327K fuzz tests, 0 failures.

### Фаза D: Layout внутри виджета — СЛАБО

**Проблема 5: Zones — хардкоженная эвристика.**
`CalculateZones` классифицирует каждый атом НЕЗАВИСИМО по его display-типу:
- image → hero, h1/h2 → stack, price → row, tag → flow, body → stack
Нет способа сказать "image рядом с column из текстовых атомов". Нет вложенности. Нет пропорций.

**Проблема 6: Agent 2 не может управлять layout внутри виджета.**
Единственный примитив — `direction: horizontal/vertical`. Это меняет весь виджет целиком (CSS class). Невозможно описать "фото слева, инфа справа, кнопка на всю ширину".

**Проблема 7: Нет понятия viewport.**
Виджет строится без знания ширины экрана. Нельзя проверить "5 badge-ей в row влезают в 400px?". Нельзя рассчитать пропорции.

**Проблема 8: Нет приблизительных размеров display-обёрток.**
badge — примерно 60-120px в ширину. h1 — полная ширина. thumbnail — 60-80px. Без этих знаний невозможно валидировать layout.

### Фаза E: Constraints Level 2 — ЧАСТИЧНО

Существующие: max badges, max tags, max headings, direction limit — работают.
Нет: пространственной валидации (влезает ли layout в viewport).

### Фаза F: Сборка формации — РАБОТАЕТ

grid/list/single/carousel/comparison/table + grid config. Работает.

### Фаза G: Cross-Widget Constraints — РАБОТАЕТ

C1 (normalizeFieldSet) работает.

### Фаза H: Post-processing — РАБОТАЕТ

---

## Часть 3: Изменения (по приоритету)

---

### ИЗМЕНЕНИЕ 1: Generic Field Handling (Фаза A)

**Цель**: движок создаёт атомы из ЛЮБЫХ данных, не только из 14 захардкоженных полей.

#### 1.1 Добавить Extra в Product и Service

```go
// domain/product_entity.go
type Product struct {
    // ... все существующие поля ...
    Extra map[string]interface{} `json:"extra,omitempty"`
}

// domain/state_entity.go (Service аналогично)
type Service struct {
    // ... все существующие поля ...
    Extra map[string]interface{} `json:"extra,omitempty"`
}
```

#### 1.2 Расширить ProductFieldGetter — default case

```go
// engine/formation.go — в switch добавить default:
default:
    if p.Extra != nil {
        if v, ok := p.Extra[fieldName]; ok {
            return v
        }
    }
    return nil
```

Аналогично ServiceFieldGetter.

#### 1.3 Добавить InferFieldType — определение типа по значению

```go
// engine/field_types.go
func InferFieldType(fieldName string, value interface{}) FieldTypeEntry {
    // Сначала проверяем FieldTypeMap (known fields)
    if entry, ok := FieldTypeMap[fieldName]; ok {
        return entry
    }
    // Определяем по значению
    switch v := value.(type) {
    case float64, float32, int, int64:
        return FieldTypeEntry{domain.AtomTypeNumber, domain.SubtypeFloat}
    case string:
        if isURL(v) {
            return FieldTypeEntry{domain.AtomTypeImage, domain.SubtypeImageURL}
        }
        return FieldTypeEntry{domain.AtomTypeText, domain.SubtypeString}
    case []interface{}, []string:
        return FieldTypeEntry{domain.AtomTypeText, domain.SubtypeString}
    case bool:
        return FieldTypeEntry{domain.AtomTypeText, domain.SubtypeString}
    default:
        return FieldTypeEntry{domain.AtomTypeText, domain.SubtypeString}
    }
}

func isURL(s string) bool {
    return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}
```

#### 1.4 Обновить BuildFieldConfigsWithFormat — использовать InferFieldType

Строка 320-323 в defaults.go уже делает fallback на text/string. Заменить на:

```go
entry, known := FieldTypeMap[name]
if !known {
    // Пытаемся определить тип по первому значению из данных (если доступно)
    // Fallback: text/string
    entry = FieldTypeEntry{domain.AtomTypeText, domain.SubtypeString}
}
```

Для полноценного определения по значению нужно передать данные в BuildFieldConfigs. Но это потребует изменения сигнатуры. Минимальный вариант: оставить text/string fallback, InferFieldType использовать при создании атомов в BuildAtoms.

#### 1.5 Обновить AutoResolve — динамический field ranking

```go
func AutoResolve(entityType string, entityCount int) ResolvedDefaults {
    // ...существующая логика...
    ranking := fieldRanking[entityType]
    if ranking == nil {
        ranking = fieldRanking["product"] // fallback на product
    }
    // ...
}

// НОВАЯ функция: resolve с учётом реальных полей данных
func AutoResolveWithFields(entityType string, entityCount int, availableFields []string) ResolvedDefaults {
    defaults := AutoResolve(entityType, entityCount)

    // Если entity type известен И ranking покрывает доступные поля — ок
    ranking := fieldRanking[entityType]
    if ranking != nil {
        knownSet := make(map[string]bool, len(ranking))
        for _, f := range ranking {
            knownSet[f] = true
        }
        allKnown := true
        for _, f := range availableFields {
            if !knownSet[f] {
                allKnown = false
                break
            }
        }
        if allKnown {
            return defaults
        }
    }

    // Неизвестные поля или неизвестный entity type — используем availableFields как ranking
    // Приоритет: images first (если есть), затем как пришло
    fields := prioritizeFields(availableFields, defaults.MaxFields)
    defaults.Fields = fields
    return defaults
}

// prioritizeFields ставит images первым, ограничивает по maxFields
func prioritizeFields(available []string, maxFields int) []string {
    result := make([]string, 0, maxFields)
    // images first
    for _, f := range available {
        if f == "images" && len(result) < maxFields {
            result = append(result, f)
        }
    }
    for _, f := range available {
        if f != "images" && len(result) < maxFields {
            result = append(result, f)
        }
    }
    return result
}
```

#### 1.6 Обновить defaultDisplay и defaultSlot — fallback для неизвестных полей

Сейчас: неизвестное поле → display="" → ValidateDisplay возвращает "body", slot → primary.
Это ок. **Не менять** — Agent 2 может переопределить через display{}.

**Файлы**: `domain/product_entity.go`, `engine/field_types.go`, `engine/defaults.go`, `engine/formation.go`

---

### ИЗМЕНЕНИЕ 2: Layout Tree — structure параметр (Фаза D)

**Цель**: Agent 2 может описать расположение атомов внутри виджета как дерево. Когда structure не указана — auto-layout генерирует tree из улучшенной zone-логики.

#### 2.1 Четыре типа группировки

| Тип | CSS | Описание |
|-----|-----|----------|
| `row` | `display:flex; flex-direction:row` | Дети горизонтально |
| `column` | `display:flex; flex-direction:column` | Дети вертикально |
| `flow` | `display:flex; flex-wrap:wrap` | Дети с переносом |
| `span` | `width:100%` | Элемент на полную ширину родителя |

Это полный набор. Любой UI описывается комбинацией вложенных row/column/flow + span.

#### 2.2 Domain: LayoutNode

```go
// domain/widget_entity.go
type LayoutNode struct {
    Type       string       `json:"type"`                // "row", "column", "flow", "span", "atom"
    Children   []LayoutNode `json:"children,omitempty"`  // вложенные узлы
    FieldName  string       `json:"field,omitempty"`     // для type="atom": имя поля
    AtomIndex  int          `json:"atomIndex,omitempty"` // для type="atom": индекс в Widget.Atoms
    Proportion string       `json:"w,omitempty"`         // "40%", "auto", "fill" — ширина в родительском row
}
```

Widget получает новое поле:
```go
type Widget struct {
    // ... все существующие поля ...
    Layout []LayoutNode `json:"layout,omitempty"` // tree layout. Если есть — фронтенд рендерит tree, не zones.
}
```

**Zones остаются** для обратной совместимости. Если Layout != nil → фронт рендерит Layout. Если nil → рендерит Zones как сейчас.

#### 2.3 Формат параметра structure для Agent 2

Верхний уровень — неявный column (вертикальный стек). Каждый элемент:
- **string** → leaf node (имя поля → атом)
- **object с ключом row/column/flow** → группа, value = массив детей
- **object с ключом span** → элемент на полную ширину

```json
{
  "structure": [
    {"row": ["images", {"column": ["name", {"row": ["price", "rating"]}, {"flow": ["tags"]}]}]},
    {"span": ["buyButton"]}
  ]
}
```

Пропорции задаются через объект с полем `w`:
```json
{"row": [
  {"field": "images", "w": "40%"},
  {"column": ["name", "price", "rating"]}
]}
```

Если пропорция не указана — авто-расчёт (см. 2.6).

#### 2.4 Парсинг structure → LayoutNode tree

```go
// engine/layout.go — НОВАЯ функция
func ParseStructure(raw interface{}, atomFieldMap map[string]int) []LayoutNode {
    arr, ok := raw.([]interface{})
    if !ok {
        return nil
    }

    nodes := make([]LayoutNode, 0, len(arr))
    for _, item := range arr {
        node := parseNode(item, atomFieldMap)
        if node != nil {
            nodes = append(nodes, *node)
        }
    }
    return nodes
}

func parseNode(item interface{}, atomFieldMap map[string]int) *LayoutNode {
    // String → atom leaf
    if fieldName, ok := item.(string); ok {
        idx, exists := atomFieldMap[fieldName]
        if !exists {
            return nil // поле не в данных — пропустить
        }
        return &LayoutNode{Type: "atom", FieldName: fieldName, AtomIndex: idx}
    }

    // Object → group or atom-with-proportion
    obj, ok := item.(map[string]interface{})
    if !ok {
        return nil
    }

    // Atom with proportion: {"field": "images", "w": "40%"}
    if fieldName, ok := obj["field"].(string); ok {
        idx, exists := atomFieldMap[fieldName]
        if !exists {
            return nil
        }
        node := &LayoutNode{Type: "atom", FieldName: fieldName, AtomIndex: idx}
        if w, ok := obj["w"].(string); ok {
            node.Proportion = w
        }
        return node
    }

    // Group: {"row": [...]}, {"column": [...]}, {"flow": [...]}, {"span": [...]}
    for _, groupType := range []string{"row", "column", "flow", "span"} {
        if children, ok := obj[groupType].([]interface{}); ok {
            childNodes := make([]LayoutNode, 0, len(children))
            for _, child := range children {
                if cn := parseNode(child, atomFieldMap); cn != nil {
                    childNodes = append(childNodes, *cn)
                }
            }
            if len(childNodes) == 0 {
                return nil
            }
            return &LayoutNode{Type: groupType, Children: childNodes}
        }
    }

    return nil
}
```

`atomFieldMap` строится из Widget.Atoms: `{"images": 0, "name": 1, "price": 2, ...}`

#### 2.5 Auto-layout (когда structure НЕ указана)

Вместо текущих zones → генерируем LayoutNode tree из тех же правил но в формате дерева:

```go
// engine/layout.go — НОВАЯ функция
func AutoLayout(atoms []domain.Atom, direction string, size domain.WidgetSize, tokens DesignTokens) []LayoutNode {
    if len(atoms) == 0 {
        return nil
    }

    // Классификация (аналог текущих zones)
    var heroAtoms, headingAtoms, priceAtoms, ratingAtoms []int
    var flowAtoms, bodyAtoms, buttonAtoms, otherAtoms []int

    for i, a := range atoms {
        // ... та же логика что в CalculateZones ...
        // (image→hero, h1-h4→heading, price→price, rating→rating,
        //  tag/badge→flow, body→body, button→button, rest→other)
    }

    // ГОРИЗОНТАЛЬНЫЙ layout (image left + content right)
    if direction == "horizontal" && len(heroAtoms) > 0 {
        return buildHorizontalLayout(atoms, heroAtoms, headingAtoms, priceAtoms,
            ratingAtoms, flowAtoms, bodyAtoms, buttonAtoms, otherAtoms, tokens)
    }

    // ВЕРТИКАЛЬНЫЙ layout (стандарт) — собираем tree вместо flat zones
    var nodes []LayoutNode

    // Hero (images)
    for _, idx := range heroAtoms {
        nodes = append(nodes, LayoutNode{Type: "atom", AtomIndex: idx, FieldName: atoms[idx].FieldName})
    }

    // Headings (stack → column)
    for _, idx := range headingAtoms {
        nodes = append(nodes, LayoutNode{Type: "atom", AtomIndex: idx, FieldName: atoms[idx].FieldName})
    }

    // Price + Rating → row
    priceRating := append(priceAtoms, ratingAtoms...)
    if len(priceRating) > 0 {
        row := LayoutNode{Type: "row", Children: make([]LayoutNode, 0, len(priceRating))}
        for _, idx := range priceRating {
            row.Children = append(row.Children, LayoutNode{Type: "atom", AtomIndex: idx, FieldName: atoms[idx].FieldName})
        }
        nodes = append(nodes, row)
    }

    // Body (stack)
    for _, idx := range bodyAtoms {
        nodes = append(nodes, LayoutNode{Type: "atom", AtomIndex: idx, FieldName: atoms[idx].FieldName})
    }

    // Flow (tags/badges)
    if len(flowAtoms) > 0 {
        flow := LayoutNode{Type: "flow", Children: make([]LayoutNode, 0, len(flowAtoms))}
        for _, idx := range flowAtoms {
            flow.Children = append(flow.Children, LayoutNode{Type: "atom", AtomIndex: idx, FieldName: atoms[idx].FieldName})
        }
        nodes = append(nodes, flow)
    }

    // Buttons → row
    if len(buttonAtoms) > 0 {
        btnRow := LayoutNode{Type: "row", Children: make([]LayoutNode, 0, len(buttonAtoms))}
        for _, idx := range buttonAtoms {
            btnRow.Children = append(btnRow.Children, LayoutNode{Type: "atom", AtomIndex: idx, FieldName: atoms[idx].FieldName})
        }
        nodes = append(nodes, btnRow)
    }

    // Other → stack
    for _, idx := range otherAtoms {
        nodes = append(nodes, LayoutNode{Type: "atom", AtomIndex: idx, FieldName: atoms[idx].FieldName})
    }

    return nodes
}

func buildHorizontalLayout(atoms []domain.Atom, hero, headings, prices, ratings, flow, body, buttons, other []int, tokens DesignTokens) []LayoutNode {
    // Image left (40%), content right (60%)
    imageNode := LayoutNode{Type: "atom", AtomIndex: hero[0], FieldName: atoms[hero[0]].FieldName, Proportion: "40%"}

    // Right column — headings, price+rating row, body, flow
    var rightChildren []LayoutNode
    for _, idx := range headings {
        rightChildren = append(rightChildren, LayoutNode{Type: "atom", AtomIndex: idx, FieldName: atoms[idx].FieldName})
    }
    priceRating := append(prices, ratings...)
    if len(priceRating) > 0 {
        row := LayoutNode{Type: "row", Children: make([]LayoutNode, 0)}
        for _, idx := range priceRating {
            row.Children = append(row.Children, LayoutNode{Type: "atom", AtomIndex: idx, FieldName: atoms[idx].FieldName})
        }
        rightChildren = append(rightChildren, row)
    }
    for _, idx := range body {
        rightChildren = append(rightChildren, LayoutNode{Type: "atom", AtomIndex: idx, FieldName: atoms[idx].FieldName})
    }
    if len(flow) > 0 {
        flowNode := LayoutNode{Type: "flow", Children: make([]LayoutNode, 0)}
        for _, idx := range flow {
            flowNode.Children = append(flowNode.Children, LayoutNode{Type: "atom", AtomIndex: idx, FieldName: atoms[idx].FieldName})
        }
        rightChildren = append(rightChildren, flowNode)
    }

    rightCol := LayoutNode{Type: "column", Children: rightChildren, Proportion: "60%"}

    mainRow := LayoutNode{Type: "row", Children: []LayoutNode{imageNode, rightCol}}

    result := []LayoutNode{mainRow}

    // Buttons — span (full width under both columns)
    for _, idx := range buttons {
        result = append(result, LayoutNode{Type: "span", Children: []LayoutNode{
            {Type: "atom", AtomIndex: idx, FieldName: atoms[idx].FieldName},
        }})
    }

    return result
}
```

**Ключевое**: auto-layout теперь тоже выдаёт LayoutNode tree. Фронтенд всегда рендерит tree. Zones → deprecated (но остаются для обратной совместимости старых виджетов).

#### 2.6 Пропорции — дефолтные правила

Когда пропорция не указана, движок вычисляет:

```go
// engine/layout.go
func ApplyDefaultProportions(nodes []LayoutNode, atoms []domain.Atom, viewportWidth int) {
    for i := range nodes {
        node := &nodes[i]
        if node.Type == "row" && node.Proportion == "" {
            applyRowProportions(node, atoms, viewportWidth)
        }
        if len(node.Children) > 0 {
            ApplyDefaultProportions(node.Children, atoms, viewportWidth)
        }
    }
}

func applyRowProportions(row *LayoutNode, atoms []domain.Atom, viewportWidth int) {
    if len(row.Children) < 2 {
        return
    }
    // Проверить: есть ли image рядом с non-image
    hasImage := false
    for _, child := range row.Children {
        if child.Type == "atom" && child.AtomIndex >= 0 && child.AtomIndex < len(atoms) {
            if atoms[child.AtomIndex].Type == domain.AtomTypeImage {
                hasImage = true
                break
            }
        }
    }
    if hasImage && len(row.Children) == 2 {
        // Image + content → 40/60
        for i := range row.Children {
            if row.Children[i].Proportion != "" {
                continue // уже задана
            }
            if row.Children[i].Type == "atom" && atoms[row.Children[i].AtomIndex].Type == domain.AtomTypeImage {
                row.Children[i].Proportion = "40%"
            } else {
                row.Children[i].Proportion = "60%"
            }
        }
    }
    // Без image → equal split
    // (CSS flex:1 по умолчанию, Proportion остаётся "")
}
```

---

### ИЗМЕНЕНИЕ 3: Viewport и приблизительные размеры (Фаза D, E)

**Цель**: движок знает ширину экрана и примерные размеры display-обёрток. Constraints проверяют "влезает ли".

#### 3.1 Viewport

Chat widget имеет фиксированную ширину: ~380-420px. Определяем дефолт и передаём из фронтенда.

```go
// engine/layout.go
const DefaultViewportWidth = 400 // px
const WidgetPadding = 24         // 12px * 2 sides
const RowGap = 8                 // px between items in row
const ColumnGap = 6              // px between items in column
const FlowGap = 4               // px between items in flow
```

Фронтенд при инициализации сессии передаёт viewport ширину (если не передал — 400px).

#### 3.2 Приблизительные размеры display-обёрток

Не pixel-perfect, а "достаточно для валидации". Нужно чтобы constraint мог сказать "5 badges в row на 400px — не влезает".

```go
// engine/layout.go
type DisplaySizeHint struct {
    MinWidth  int  // минимальная ширина в px
    MaxWidth  int  // максимальная ширина (0 = full width)
    Height    int  // примерная высота
    FullWidth bool // занимает всю ширину родителя (h1, body, image-cover)
}

var DisplaySizeHints = map[string]DisplaySizeHint{
    // Text — full width
    "h1":       {MinWidth: 200, MaxWidth: 0, Height: 44, FullWidth: true},
    "h2":       {MinWidth: 150, MaxWidth: 0, Height: 36, FullWidth: true},
    "h3":       {MinWidth: 120, MaxWidth: 0, Height: 28, FullWidth: true},
    "h4":       {MinWidth: 100, MaxWidth: 0, Height: 24, FullWidth: true},
    "body-lg":  {MinWidth: 100, MaxWidth: 0, Height: 24, FullWidth: true},
    "body":     {MinWidth: 80,  MaxWidth: 0, Height: 20, FullWidth: true},
    "body-sm":  {MinWidth: 60,  MaxWidth: 0, Height: 18, FullWidth: true},
    "caption":  {MinWidth: 40,  MaxWidth: 0, Height: 16, FullWidth: true},

    // Inline elements — variable width
    "badge":         {MinWidth: 40,  MaxWidth: 120, Height: 28, FullWidth: false},
    "badge-success": {MinWidth: 40,  MaxWidth: 120, Height: 28, FullWidth: false},
    "badge-error":   {MinWidth: 40,  MaxWidth: 120, Height: 28, FullWidth: false},
    "badge-warning": {MinWidth: 40,  MaxWidth: 120, Height: 28, FullWidth: false},
    "tag":           {MinWidth: 32,  MaxWidth: 100, Height: 24, FullWidth: false},
    "tag-active":    {MinWidth: 32,  MaxWidth: 100, Height: 24, FullWidth: false},

    // Price/Rating — fixed-ish width
    "price":          {MinWidth: 60,  MaxWidth: 150, Height: 28, FullWidth: false},
    "price-lg":       {MinWidth: 80,  MaxWidth: 200, Height: 36, FullWidth: false},
    "price-old":      {MinWidth: 50,  MaxWidth: 120, Height: 20, FullWidth: false},
    "price-discount": {MinWidth: 60,  MaxWidth: 150, Height: 28, FullWidth: false},
    "rating":         {MinWidth: 80,  MaxWidth: 140, Height: 24, FullWidth: false},
    "rating-compact": {MinWidth: 50,  MaxWidth: 80,  Height: 20, FullWidth: false},
    "rating-text":    {MinWidth: 40,  MaxWidth: 60,  Height: 20, FullWidth: false},

    // Images
    "image-cover": {MinWidth: 100, MaxWidth: 0, Height: 200, FullWidth: true},
    "thumbnail":   {MinWidth: 60,  MaxWidth: 80, Height: 60,  FullWidth: false},
    "avatar":      {MinWidth: 40,  MaxWidth: 48, Height: 40,  FullWidth: false},
    "avatar-sm":   {MinWidth: 32,  MaxWidth: 36, Height: 32,  FullWidth: false},
    "avatar-lg":   {MinWidth: 56,  MaxWidth: 64, Height: 56,  FullWidth: false},
    "gallery":     {MinWidth: 200, MaxWidth: 0,  Height: 300, FullWidth: true},

    // Buttons — variable
    "button-primary":   {MinWidth: 100, MaxWidth: 200, Height: 40, FullWidth: false},
    "button-secondary": {MinWidth: 100, MaxWidth: 200, Height: 36, FullWidth: false},
    "button-outline":   {MinWidth: 80,  MaxWidth: 180, Height: 36, FullWidth: false},

    // Utility
    "divider": {MinWidth: 0, MaxWidth: 0, Height: 1,  FullWidth: true},
    "spacer":  {MinWidth: 0, MaxWidth: 0, Height: 16, FullWidth: true},
}

func GetSizeHint(display string) DisplaySizeHint {
    if hint, ok := DisplaySizeHints[display]; ok {
        return hint
    }
    return DisplaySizeHint{MinWidth: 40, MaxWidth: 0, Height: 20, FullWidth: true}
}
```

#### 3.3 Оценка ширины row

```go
// engine/layout.go
func EstimateRowWidth(row LayoutNode, atoms []domain.Atom) int {
    total := 0
    for i, child := range row.Children {
        if i > 0 {
            total += RowGap
        }
        total += EstimateNodeMinWidth(child, atoms)
    }
    return total
}

func EstimateNodeMinWidth(node LayoutNode, atoms []domain.Atom) int {
    switch node.Type {
    case "atom":
        if node.AtomIndex < len(atoms) {
            hint := GetSizeHint(atoms[node.AtomIndex].Display)
            return hint.MinWidth
        }
        return 40
    case "row":
        return EstimateRowWidth(node, atoms)
    case "column", "flow", "span":
        // Ширина column/flow/span = ширина самого широкого ребёнка
        maxW := 0
        for _, child := range node.Children {
            w := EstimateNodeMinWidth(child, atoms)
            if w > maxW {
                maxW = w
            }
        }
        return maxW
    }
    return 40
}
```

---

### ИЗМЕНЕНИЕ 4: Layout Constraints (Фаза E)

**Цель**: валидация layout tree с использованием viewport и размеров.

```go
// engine/constraints.go — НОВЫЕ функции

// ApplyLayoutConstraints validates and fixes layout tree
func ApplyLayoutConstraints(nodes []LayoutNode, atoms []domain.Atom, viewportWidth int) []LayoutNode {
    availableWidth := viewportWidth - WidgetPadding
    nodes = applyLayoutConstraintsRecursive(nodes, atoms, availableWidth)
    nodes = removeEmptyGroups(nodes)
    nodes = flattenSingleChild(nodes)
    return nodes
}

func applyLayoutConstraintsRecursive(nodes []LayoutNode, atoms []domain.Atom, availableWidth int) []LayoutNode {
    for i := range nodes {
        node := &nodes[i]

        // Рекурсия в детей
        if len(node.Children) > 0 {
            childWidth := availableWidth
            if node.Type == "row" && node.Proportion != "" {
                // Если row внутри другого row с пропорцией — доступная ширина уменьшается
                childWidth = parseProportionWidth(node.Proportion, availableWidth)
            }
            node.Children = applyLayoutConstraintsRecursive(node.Children, atoms, childWidth)
        }

        // L1: Row overflow → column
        if node.Type == "row" {
            estimatedWidth := EstimateRowWidth(*node, atoms)
            if estimatedWidth > availableWidth && len(node.Children) > 1 {
                // Попытка 1: если 2 детей и один image → задать 40/60 пропорции
                if len(node.Children) == 2 {
                    hasImage := false
                    for _, child := range node.Children {
                        if child.Type == "atom" && child.AtomIndex < len(atoms) && atoms[child.AtomIndex].Type == domain.AtomTypeImage {
                            hasImage = true
                        }
                    }
                    if hasImage {
                        continue // пропорции решат проблему
                    }
                }
                // Попытка 2: конвертировать в column
                node.Type = "column"
            }
        }

        // L5: Нет full-width элементов в row (h1, body, image-cover в row бессмысленны)
        if node.Type == "row" {
            for ci := range node.Children {
                child := &node.Children[ci]
                if child.Type == "atom" && child.AtomIndex < len(atoms) {
                    hint := GetSizeHint(atoms[child.AtomIndex].Display)
                    if hint.FullWidth && len(node.Children) > 1 {
                        // Full-width атом в row с другими → вынести в span
                        // Пока просто: оставить, CSS flex решит
                    }
                }
            }
        }

        // L6: Max nesting depth = 3
        if depthOf(*node) > 3 {
            node.Children = flattenDeep(node.Children, 3)
        }
    }
    return nodes
}

// removeEmptyGroups удаляет группы без детей (все атомы = nil)
func removeEmptyGroups(nodes []LayoutNode) []LayoutNode {
    result := make([]LayoutNode, 0, len(nodes))
    for _, node := range nodes {
        if len(node.Children) > 0 {
            node.Children = removeEmptyGroups(node.Children)
            if len(node.Children) > 0 {
                result = append(result, node)
            }
        } else {
            result = append(result, node) // leaf node — оставить
        }
    }
    return result
}

// flattenSingleChild разворачивает группы с одним ребёнком
func flattenSingleChild(nodes []LayoutNode) []LayoutNode {
    result := make([]LayoutNode, 0, len(nodes))
    for _, node := range nodes {
        if len(node.Children) > 0 {
            node.Children = flattenSingleChild(node.Children)
        }
        // Группа с одним ребёнком (кроме span) → развернуть
        if len(node.Children) == 1 && node.Type != "span" {
            result = append(result, node.Children[0])
        } else {
            result = append(result, node)
        }
    }
    return result
}
```

---

### ИЗМЕНЕНИЕ 5: Интеграция в pipeline (tool_visual_assembly.go)

#### 5.1 Добавить structure в tool definition

В `Definition()` добавить:
```go
"structure": map[string]interface{}{
    "type": "array",
    "description": "Widget internal layout. Top level = vertical stack. Items: field name (string) or group ({\"row\":[...]}, {\"column\":[...]}, {\"flow\":[...]}, {\"span\":[...]}). For proportion: {\"field\":\"images\", \"w\":\"40%\"}. Omit for auto-layout.",
    "items": map[string]interface{}{},
},
```

#### 5.2 Обновить Execute()

После Step 10 (BuildVisualWidgets) и Constraints (Level 1-2), ПЕРЕД zones:

```go
// Step 10.5: Build layout tree
structureRaw, hasStructure := input["structure"]
viewportWidth := DefaultViewportWidth
// TODO: получать viewport из toolCtx если фронтенд передал

for i := range formation.Widgets {
    w := &formation.Widgets[i]

    // Построить atomFieldMap
    atomFieldMap := make(map[string]int, len(w.Atoms))
    for ai, a := range w.Atoms {
        atomFieldMap[a.FieldName] = ai
    }

    if hasStructure {
        // Agent 2 указал structure → парсить
        layout := ParseStructure(structureRaw, atomFieldMap)
        if layout != nil {
            ApplyDefaultProportions(layout, w.Atoms, viewportWidth)
            layout = ApplyLayoutConstraints(layout, w.Atoms, viewportWidth)
            w.Layout = layout
        }
    }

    if w.Layout == nil {
        // Auto-layout → генерируем tree из zone-логики
        w.Layout = AutoLayout(w.Atoms, direction, size, DefaultDesignTokens())
        ApplyDefaultProportions(w.Layout, w.Atoms, viewportWidth)
        w.Layout = ApplyLayoutConstraints(w.Layout, w.Atoms, viewportWidth)
    }

    // Zones тоже заполняем для обратной совместимости
    w.Zones = CalculateZones(w.Atoms, DefaultDesignTokens())
}
```

#### 5.3 Сохранить structure в RenderConfig (для diff/patch)

```go
// domain/template_entity.go — добавить
type RenderConfig struct {
    // ... существующие поля ...
    Structure interface{} `json:"structure,omitempty"` // raw structure для повторного использования
}
```

В writeFormation: `formation.Config.Structure = input["structure"]` (если была).

При diff/patch (Step 2.5): если предыдущий Config имеет Structure → передать как базу, Agent 2 может модифицировать.

---

### ИЗМЕНЕНИЕ 6: Фронтенд — LayoutRenderer

#### 6.1 Новый компонент LayoutRenderer

```jsx
// entities/widget/templates/LayoutRenderer.jsx
import { AtomRenderer } from '../../atom/AtomRenderer';
import './LayoutRenderer.css';

export function LayoutRenderer({ layout, atoms }) {
  if (!layout || layout.length === 0) return null;

  return (
    <div className="layout-root">
      {layout.map((node, i) => (
        <LayoutNode key={i} node={node} atoms={atoms} />
      ))}
    </div>
  );
}

function LayoutNode({ node, atoms }) {
  // Leaf: atom
  if (node.type === 'atom') {
    const atom = atoms[node.atomIndex];
    if (!atom) return null;
    return (
      <div className="layout-atom" style={node.w ? { flex: `0 0 ${node.w}` } : undefined}>
        <AtomRenderer atom={atom} />
      </div>
    );
  }

  // Group: row, column, flow, span
  const className = `layout-${node.type}`;
  const style = node.w ? { flex: `0 0 ${node.w}` } : undefined;

  return (
    <div className={className} style={style}>
      {node.children?.map((child, i) => (
        <LayoutNode key={i} node={child} atoms={atoms} />
      ))}
    </div>
  );
}
```

#### 6.2 CSS

```css
/* LayoutRenderer.css */
.layout-root {
  display: flex;
  flex-direction: column;
  gap: 6px;
  width: 100%;
}

.layout-row {
  display: flex;
  flex-direction: row;
  gap: 8px;
  align-items: flex-start;
}

.layout-column {
  display: flex;
  flex-direction: column;
  gap: 4px;
  flex: 1;
  min-width: 0; /* prevent overflow */
}

.layout-flow {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}

.layout-span {
  width: 100%;
}

.layout-atom {
  min-width: 0; /* allow shrinking */
}
```

#### 6.3 Обновить GenericCardTemplate

```jsx
export function GenericCardTemplate({ atoms = [], zones, layout, size = 'medium', direction, entityRef }) {
  // Новый путь: layout tree
  if (layout && layout.length > 0) {
    return (
      <div className={`generic-card size-${size}`}>
        <LayoutRenderer layout={layout} atoms={atoms} />
        {/* actions (like, cart) тут же */}
      </div>
    );
  }

  // Старый путь: zones
  if (zones && zones.length > 0) {
    return <ZoneLayout atoms={atoms} zones={zones} size={size} direction={direction} entityRef={entityRef} />;
  }

  return <LegacyLayout atoms={atoms} size={size} direction={direction} entityRef={entityRef} />;
}
```

---

### ИЗМЕНЕНИЕ 7: Agent 2 Prompt

#### 7.1 Добавить в системный промпт

```
## structure parameter (optional)

Controls how atoms are arranged INSIDE each widget. When omitted, auto-layout handles it (good for standard cards).
Use structure when user asks for a specific arrangement, or for non-standard views (categories, attributes, custom grids).

### Layout types:
- {"row": [...]}     — children side by side horizontally
- {"column": [...]}  — children stacked vertically
- {"flow": [...]}    — children with wrapping (ideal for tags, badges)
- {"span": [...]}    — full width of parent

### Syntax:
- String = field name (leaf atom): "name", "price", "images"
- Object = group: {"row": ["price", "rating"]}
- With proportion: {"field": "images", "w": "40%"}
- Top level is always an implicit column (vertical stack)
- Groups can nest: {"row": ["images", {"column": ["name", "price"]}]}

### When to use:
- User asks for specific arrangement: "фото слева, инфа справа" → structure with row
- Non-standard data display: categories list, attribute grid
- Complex layouts: image beside content with button spanning full width

### When NOT to use:
- Standard product cards → auto-layout handles it perfectly
- Only changing display/color/size → no structure needed

### Examples:

Horizontal card (image left, info right):
visual_assembly({
  structure: [
    {"row": [{"field":"images","w":"40%"}, {"column":["name",{"row":["price","rating"]},"description"]}]}
  ]
})

Category list:
visual_assembly({
  show: ["category", "subcategories"],
  display: {"category":"h3", "subcategories":"tag"},
  structure: ["category", {"flow": ["subcategories"]}]
})

Card with full-width button:
visual_assembly({
  structure: ["images", "name", {"row":["price","rating"]}, {"span":["buyButton"]}]
})
```

#### 7.2 Обновить BuildAgent2ToolPrompt — передать viewport

```go
if viewportWidth > 0 {
    input["viewport_width"] = viewportWidth
}
```

#### 7.3 Обновить display_meta — добавить type/subtype

```go
hints = append(hints, domain.FieldDisplayHint{
    Name:     name,
    Type:     entry.Type,     // "number"
    Subtype:  entry.Subtype,  // "currency"
    Category: category,
    Default:  defaultDisplay[name],
})
```

---

## Часть 4: Что НЕ меняется

- **Пресеты** — все 10 остаются, работают как раньше. Preset загружает базу → structure сверху если нужно.
- **Constraints Level 1** (per-atom) — без изменений.
- **Constraints Level 2** (per-widget: max badges/tags/headings) — без изменений.
- **Constraints Level 4** (cross-widget: normalizeFieldSet) — без изменений.
- **Formation modes** (grid/list/single/carousel/comparison/table) — без изменений.
- **PostProcessing** (color, shape, anchor, conditional) — без изменений.
- **Agent 1** — не затрагивается.
- **Diff/Patch** (Step 2.5) — работает как раньше для fields/layout/size. Structure добавляется в RenderConfig.
- **CalculateZones** — остаётся, заполняется для обратной совместимости. Но фронтенд предпочитает Layout.
- **Весь flow**: data → agent2 → tool → engine → state → frontend.

---

## Часть 5: Порядок реализации

### Этап 1: Domain (30 мин)
1. `LayoutNode` struct в `domain/widget_entity.go`
2. `Layout []LayoutNode` в `Widget`
3. `Extra map[string]interface{}` в `Product` и `Service`
4. `Structure interface{}` в `RenderConfig`

### Этап 2: Generic fields (45 мин)
1. `InferFieldType()` в `engine/field_types.go`
2. `default` case в `ProductFieldGetter` и `ServiceFieldGetter`
3. `AutoResolveWithFields()` в `engine/defaults.go`

### Этап 3: Layout engine (1.5 часа)
1. `ParseStructure()` в `engine/layout.go`
2. `AutoLayout()` (zones → tree) в `engine/layout.go`
3. `ApplyDefaultProportions()` в `engine/layout.go`
4. `DisplaySizeHints` + `EstimateRowWidth` + `EstimateNodeMinWidth` в `engine/layout.go`
5. Layout constraints (`ApplyLayoutConstraints`) в `engine/constraints.go`

### Этап 4: Tool integration (45 мин)
1. `structure` в tool Definition()
2. Pipeline step 10.5 в `tool_visual_assembly.go`
3. Structure в RenderConfig save

### Этап 5: Frontend (1.5 часа)
1. `LayoutRenderer.jsx` + CSS
2. Обновить `GenericCardTemplate.jsx` — if layout → LayoutRenderer
3. Обновить `WidgetRenderer.jsx` — передать layout prop

### Этап 6: Prompt + тест (1 час)
1. Обновить Agent 2 системный промпт (structure docs + examples)
2. Обновить `BuildAgent2ToolPrompt` (viewport, enhanced display_meta)
3. Тест через testbench: structure без пресета
4. Тест: auto-layout (обратная совместимость)
5. Тест: LLM запрос "фото слева инфа справа"
6. Тест: LLM запрос "покажи категории списком"

**Общее время: ~6 часов**

---

## Часть 6: Примеры — как это работает

### Пример 1: Стандартный запрос (без structure)

Юзер: "покажи сыворотки для сухой кожи"
Agent 1: catalog_search → 5 products
Agent 2: `visual_assembly({})`

Движок:
1. AutoResolve(product, 5) → fields=[images,name,price,rating,brand], layout=grid, size=medium
2. BuildFieldConfigs → 5 FieldConfigs
3. BuildAtoms для каждого продукта
4. Constraints Level 1 (per-atom)
5. AutoLayout → tree: [images, name, {row: [price, rating]}, brand]
6. ApplyDefaultProportions → нечего менять (нет row с image)
7. ApplyLayoutConstraints → ок (всё влезает)
8. Constraints Level 2 (per-widget)
9. Formation(grid, 2 cols)
10. Cross-widget constraints
→ Выглядит как обычные карточки. Обратная совместимость.

### Пример 2: Горизонтальная карточка (structure)

Юзер: "покажи фото слева, информацию справа"
Agent 2: `visual_assembly({direction: "horizontal", structure: [{"row": [{"field":"images","w":"40%"}, {"column":["name",{"row":["price","rating"]},"brand","description"]}]}]})`

Движок:
1. AutoResolve → defaults
2. Parse structure → tree с image 40%, column 60%
3. BuildAtoms → 6 атомов
4. Constraints Level 1
5. Layout = parsed tree
6. ApplyDefaultProportions → image уже 40%
7. ApplyLayoutConstraints → EstimateRowWidth(image 100px + gap + column 150px) = 258px < 376px → ок
8. Constraints Level 2
→ Карточка с фото слева, инфой справа.

### Пример 3: Список категорий (без пресета)

Юзер: "какие категории есть?"
Agent 1: catalog_search → 20 products. State meta fields includes "category".
Agent 2: `visual_assembly({show: ["category"], display: {"category":"h3"}, layout: "list", structure: ["category"]})`

Движок:
1. fields = ["category"]
2. BuildFieldConfigs: category → text/string, display=h3
3. BuildAtoms: для каждого продукта берём category → атом
4. Layout = [atom:category] (просто один атом в виджете)
5. Cross-widget constraints: normalizeFieldSet → все имеют category → ок
→ Список из 20 карточек, каждая с одним заголовком-категорией.

### Пример 4: Переопределение после отображения

Экран: стандартный grid 5 продуктов.
Юзер: "рейтинг покажи большим внизу"
Agent 2: `visual_assembly({display: {"rating":"h1"}, order: ["images","name","price","brand","rating"]})`

Движок:
1. Diff/patch: берёт предыдущий Config как базу (layout=grid, size=medium, fields=[images,name,price,rating,brand])
2. display override: rating → h1
3. order: rating в конце
4. AutoLayout → tree: [images, name, {row: [price, brand]}, rating]
   (rating = h1, FullWidth=true → не в row, а отдельным элементом внизу)
5. Constraints: h1 = один заголовок → h2 если name тоже h2? → name=h2, rating=h1 → W4: один h1/h2, второй→h3
   Решение: order ставит rating последним. name приоритетнее. Значит: name остаётся h2, rating = h3 (downgrade).
   Или: Agent 2 может hide name для акцента на rating.
→ Grid карточки с рейтингом внизу крупным.

### Пример 5: Сравнение без пресета

Юзер: "сравни первые два товара"
Agent 2: `visual_assembly({layout: "comparison", show: ["images","name","price","rating","brand","description"]})`
(structure не нужна — comparison это просто grid cols=2 с одинаковыми виджетами)

Движок:
1. layout=comparison, products[:2]
2. AutoLayout для каждого виджета: стандартный vertical layout
3. Formation(comparison, 2 cols)
4. C1: normalizeFieldSet → оба виджета имеют одинаковые поля → ок
→ Два виджета рядом с одинаковой структурой = сравнение.
