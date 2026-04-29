# Pencil Convergence — Эволюция Visual Assembly Engine к универсальному UI-движку

> **Статус**: Спецификация / исследование
> **Дата**: 2026-03-24
> **Контекст**: MCP Pencil — инструмент для декларативной сборки UI через LLM. Анализ показал глубокое архитектурное сходство с Keepstar V2 Engine. Данная спецификация фиксирует сходства, различия и дорожную карту эволюции.

---

## 1. Общая парадигма

Оба движка решают одну задачу:

> **Декларативная сборка UI из дерева узлов с constraints, управляемая LLM**

```
LLM решает ЧТО → Движок решает КАК → Рендерер отображает
```

| Аспект | Pencil | Keepstar V2 |
|--------|--------|-------------|
| **Домен** | Произвольный UI (сайты, дашборды, приложения) | E-commerce виджеты (карточки, каталоги, сравнения) |
| **Управление** | Claude через MCP tools | Agent2 через visual_assembly tool |
| **Формат** | .pen файл (зашифрованный AST) | Formation JSON (в state.template) |
| **Рендер** | Pencil Editor (Electron/browser) | React (Shadow DOM widget) |
| **Модель** | Imperative DSL (Insert/Update/Delete) | Declarative config (show/hide/layout/size) |

---

## 2. Архитектурное сравнение — Node Tree

### 2.1 Иерархия узлов

| Уровень | Pencil | Keepstar V2 | Семантика |
|---------|--------|-------------|-----------|
| Корень | `Document` | `FormationWithData` | Контейнер всего UI |
| Секция | `Frame` (top-level) | `FormationSection` | Группа виджетов |
| Компонент | `Frame` (reusable) / `ref` | `Widget` + `PresetV2` | Переиспользуемый блок |
| Контейнер | `Frame` (layout: vertical/horizontal) | `LayoutNode` (row/column/flow/span) | Flexbox контейнер |
| Лист | `text`, `rectangle`, `image`, `icon_font` | `AtomV2` (text, number, image, icon, video, audio) | Единица контента |

### 2.2 Layout

| Свойство | Pencil | Keepstar V2 |
|----------|--------|-------------|
| Направление | `layout: "vertical" / "horizontal"` | `LayoutNodeType: row / column` |
| Wrap | Неявно через layout тип | `LayoutNodeFlow` + `wrap: true` |
| Full-width | Нет специального типа | `LayoutNodeSpan` |
| Gap | `gap: 16` (пиксели) | `gap: "sm"` (semantic tokens) |
| Padding | `padding: 32` (пиксели) | `padding: "md"` (semantic tokens) |
| Border radius | `cornerRadius: [8,8,8,8]` | `borderRadius: "md"` (token → "8px") |
| Shadow | Нет встроенного | `shadow: "sm" / "md" / "lg"` (tokens) |
| Alignment | Через x/y позиционирование | `align`, `distribution`, `selfAlign` |
| Sizing | `width: "fill_container"` / fixed px | `sizing: "hug" / "fill" / "fixed"` + `grow` |
| Min/Max | Нет | `minWidth`, `maxWidth`, `minHeight`, `maxHeight` |

### 2.3 Leaf Nodes (Atoms)

| Pencil тип | Keepstar AtomType | Различия |
|------------|-------------------|----------|
| `text` | `text` | Pencil: content + fontSize + fontWeight + color. Keepstar: Value + TextStyle + Format + Wrapper |
| `rectangle` / `frame` + fill | `image` | Pencil: image = fill на frame. Keepstar: первоклассный тип с MediaStyle |
| `icon_font` | `icon` | Pencil: font icons. Keepstar: name / emoji / svg + IconStyle |
| — | `number` | Pencil не выделяет числа в отдельный тип |
| — | `video` | Pencil рендерит как embed frame, не как отдельный тип |
| — | `audio` | Нет аналога в Pencil |
| `connection`, `line`, `polygon`, `path` | — | Графические примитивы для диаграмм, нет аналога в Keepstar |
| `note` | — | Аннотации, нет аналога |
| `group` | — | Визуальная группировка без layout |

### 2.4 Стилизация

| Аспект | Pencil | Keepstar V2 |
|--------|--------|-------------|
| Текст | `fontSize`, `fontWeight`, `fontFamily`, `color` на ноде | `TextStyle { FontSize, FontWeight, Color, LineClamp, TextTransform, LetterSpacing, LineHeight, TextDecoration }` |
| Обёртка | Нет концепции — стили на самой ноде | `WrapperConfig { Type, Variant, Color }` — badge / tag / pill / button / alert / avatar / tooltip / link / progress |
| Форматирование | Нет — значение как есть | `AtomFormat` — currency / stars / stars-compact / percent / date |
| Subtype | Нет — тип определяет всё | `AtomSubtype` — string / date / url / email / currency / rating / etc. |
| Slot | Нет — позиция = layout | `AtomSlot` — hero / title / price / primary / secondary / badge / tags / specs |
| Rigidity | Нет | `Rigidity` — locked / preferred / flexible (для constraint resolution) |
| Priority | Нет | `Priority int` — для culling при overflow |

---

## 3. Операции — сравнение API

### 3.1 CRUD операции

| Операция | Pencil | Keepstar V2 | Статус |
|----------|--------|-------------|--------|
| **Insert** | `foo=I(parent, {type:"text", content:"Hello"})` | `buildTypedAtoms()` — автоматически из данных | Keepstar не позволяет агенту вставлять произвольные ноды |
| **Update** | `U(foo+"/child", {fontSize:24})` | `applyAtomOverrides(atoms, instructions.Atoms)` | Keepstar: только через AgentInstructions (show/hide/order/atoms map) |
| **Delete** | `D("nodeId")` | `hide: ["fieldName"]` | Keepstar: семантическое скрытие, не удаление |
| **Replace** | `R("path", {type:"text", ...})` | Нет прямого аналога | Нельзя заменить атом другим типом |
| **Move** | `M("nodeId", "newParent", index)` | `order: ["field1", "field2"]` | Keepstar: только переупорядочивание, не перемещение между родителями |
| **Copy** | `C("nodeId", "parent", {overrides})` | Нет аналога | Каждый виджет строится из данных |
| **Image Gen** | `G("nodeId", "ai"/"stock", "prompt")` | Нет — изображения из каталога | Keepstar берёт URL из данных, не генерирует |

### 3.2 Binding System

**Pencil** — мощная система биндингов:
```javascript
hero=I("parentId", {type:"frame", width:400, height:300})
title=I(hero, {type:"text", content:"Hello", fontSize:24})
U(hero+"/subtitle", {content:"World"})
```

Ключевые свойства:
- Каждая Insert/Copy/Replace операция возвращает ID через binding
- Bindings можно конкатенировать: `foo+"/child/nested"`
- Path addressing: `instanceId/slotId/childId` — произвольная глубина
- Rollback при ошибке любой операции в batch

**Keepstar V2** — нет bindings:
- Agent2 оперирует field names (`show: ["price", "rating"]`)
- Нет ссылок на конкретные атомы по ID
- Нет path addressing внутрь layout tree
- AutoLayout сам группирует — агент не контролирует дерево

### 3.3 Batch Operations

| Аспект | Pencil | Keepstar V2 |
|--------|--------|-------------|
| Atomicity | Да — rollback всего batch при ошибке | Нет batch — один вызов visual_assembly |
| Порядок | Sequential внутри batch | Нет — все параметры одновременно |
| Max ops | 25 на batch | N/A — один вызов |
| Chaining | Несколько batch_design подряд | Один вызов на рендер |

---

## 4. Component / Preset System

### 4.1 Pencil Components

```
Component (reusable: true):
  ├── Определяется как Frame с reusable: true
  ├── Instance через ref: "componentId"
  ├── Descendants override: U("instance/child", {newProps})
  ├── Slot replacement: R("instance/slot", {type:"text",...})
  └── Design System = Frame с набором reusable компонентов
```

Пример:
```javascript
// Определение
{id:"Button", type:"frame", reusable:true, children:[
  {id:"label", type:"text", content:"Click me"}
]}

// Использование
btn=I("form", {type:"ref", ref:"Button"})
U(btn+"/label", {content:"Submit"})
```

### 4.2 Keepstar PresetV2

```go
PresetV2 {
  Name:        "product_card_grid"
  EntityType:  "product"
  Template:    "GenericCard"
  DefaultMode: FormationTypeGrid
  DefaultSize: WidgetSizeMedium
  Fields: []PresetV2Field{
    {FieldName:"images", Slot:"hero", Priority:0, Rigidity:"preferred",
     TextStyle:nil, Wrapper:nil, MediaStyle:&MediaStyle{AspectRatio:"4:3"}},
    {FieldName:"name", Slot:"title", Priority:1, Rigidity:"preferred",
     TextStyle:&TextStyle{FontSize:"xl", FontWeight:"bold"}, Wrapper:nil},
    ...
  }
  Structure:   *LayoutNode  // optional override
}
```

### 4.3 Ключевые различия

| Аспект | Pencil Component | Keepstar PresetV2 |
|--------|------------------|-------------------|
| Мутабельность | Instance overrides отдельных children | Field overrides через instructions |
| Вложенность | Компонент внутри компонента (ref→ref) | Нет — один уровень пресета |
| Структура | Полное дерево nodes | Плоский список field→atom mappings + optional LayoutNode |
| Адресация | Path-based: `instance/slot/child` | Field-name-based: `atoms["price"]` |
| Runtime | Статичный — что создал, то и есть | Динамический — AutoLayout перестраивает дерево каждый раз |

---

## 5. Constraint / Validation System

### 5.1 Pencil

Pencil **не имеет встроенных constraint rules**. Валидация:
- `snapshot_layout(problemsOnly: true)` — находит clipped/overlapping элементы
- `get_screenshot` → LLM визуально оценивает результат
- Повторные batch_design для исправления

Это post-hoc подход: сначала сделай → потом проверь → потом почини.

### 5.2 Keepstar V2

4 уровня constraint rules (30+ правил), выполняются **в pipeline автоматически**:

```
Level 0: Data → Atoms      — sanitization (7 rules)
Level 1: Per-Atom          — visual quality (A1-A5, D5-D6)
  A1: Badge text > 20 chars → tag
  A2: Tag text > 40 chars → unwrap
  A4: Badge → capitalize
  A5: Rating < 3.0 → compact format
  D5: Text truncation by slot
  D6: Large heading > 60 chars → downgrade

Level 2: Per-Widget        — composition (W1-W8)
  W1: Max 2 badges → 3rd+ → tag
  W2: Max 5 tags → rest hidden
  W4: One large heading per widget
  W8: Tiny size → remove images

Level 3: Cross-Widget      — consistency (C1-C5)
  C1: Field in < 70% widgets → remove from all
  C2: Placeholder fill for missing common fields
  C3: Format consistency (same field = same format)
  C4: Size consistency
  C5: Style consistency
```

**Space Resolution** (BudgetDown / NeedsUp):
```
Viewport → BudgetDown() → space allocations per widget
           NeedsUp()    → actual space needs from atoms
           → if needs > budget → Junction Rules:
             Soft:     Compress (reduce gap/padding)
             Medium:   Overflow-switch (row→column)
             Hard:     Downgrade fonts, hide low-priority atoms
             Extreme:  Flatten nesting (max 3 levels)
           → repeat max 2x
```

### 5.3 Keepstar преимущество

Keepstar гарантирует что **результат всегда валиден** без дополнительных проходов LLM. Pencil полагается на LLM для обнаружения и исправления проблем → больше токенов, больше latency, менее надёжный результат.

---

## 6. Design Tokens / Variables

### 6.1 Pencil Variables

```json
{
  "primary": { "default": "#3B82F6", "dark": "#60A5FA" },
  "background": { "default": "#FFFFFF", "dark": "#1F2937" },
  "text-primary": { "default": "#111827", "dark": "#F9FAFB" }
}
```
- Привязка к темам (light/dark/custom)
- Переменные можно bound к свойствам нод
- `set_variables` меняет глобально
- `get_variables` читает для code generation

### 6.2 Keepstar DesignTokensV2

```go
DesignTokensV2 {
  FontSize:      {xs:10, sm:12, md:14, lg:18, xl:24, 2xl:30, 3xl:36}
  FontWeight:    {light:300, normal:400, medium:500, semibold:600, bold:700}
  Spacing:       {none:0, xs:2, sm:4, md:8, lg:12, xl:16, 2xl:24}
  Radius:        {none:0, sm:4, md:8, lg:12, full:9999}
  BorderRadius:  {none:"0", sm:"4px", md:"8px", lg:"12px", xl:"16px", full:"9999px"}
  Shadow:        {none:"none", sm:"...", md:"...", lg:"..."}
  LineHeight:    {tight:1.25, normal:1.5, relaxed:1.625, loose:2.0}
  LetterSpacing: {tight:"-0.025em", normal:"0", wide:"0.05em"}
  IconSize:      {sm:16, md:20, lg:24}
}
```

### 6.3 Различия

| Аспект | Pencil | Keepstar |
|--------|--------|----------|
| Theming | Multi-theme (light/dark/custom axes) | Одна тема (CSS variables на виджете) |
| Granularity | Произвольные имена переменных | Фиксированные категории (FontSize, Spacing...) |
| Scope | Document-level | Engine-level (hardcoded) |
| Runtime switching | Да — переключение тем | Нет — одна тема |
| Variable binding | Свойство ноды → variable ref | Нет — токены резолвятся в pipeline |

---

## 7. Где кто сильнее

### 7.1 Pencil сильнее

| Возможность | Описание | Почему важно |
|-------------|----------|--------------|
| **Binding DSL** | `foo=I(parent, {...})` + `U(foo+"/child")` | LLM может строить произвольные деревья с ссылками на только что созданные ноды |
| **Path addressing** | `instanceId/slotId/childId` | Точечная модификация глубоко вложенных элементов |
| **Copy with overrides** | `C("template", parent, {descendants: {...}})` | Быстрое создание вариаций из существующего |
| **Replace** | `R("old", {type:"newType",...})` | Полная замена узла другим |
| **Произвольные типы** | frame, text, rectangle, ellipse, line, polygon, path, icon, image, connection, note | Рисование чего угодно — диаграммы, иллюстрации, чарты |
| **Multi-theme variables** | Переключение light/dark/custom без перерендера | Гибкость для разных контекстов |
| **Image generation** | `G("nodeId", "ai", "prompt")` | Генерация визуалов по описанию |
| **Component instances** | `ref` → deep descendant override | Полноценная component system как в Figma |
| **Style guide system** | `get_style_guide_tags` → `get_style_guide` → visual guidelines | Дизайн-система с вдохновением и правилами |
| **Visual validation** | `get_screenshot` → LLM проверяет визуально | Catch-all для проблем, которые constraints не покрывают |

### 7.2 Keepstar V2 сильнее

| Возможность | Описание | Почему важно |
|-------------|----------|--------------|
| **Auto-resolve** | Layout/size/fields автоматически из count + entity type | Агент может вообще не указывать параметры — движок сам решит |
| **Constraint pipeline** | 30+ правил, 4 уровня, автоматически | Гарантированно валидный результат без дополнительных LLM-вызовов |
| **BudgetDown / NeedsUp** | Двухпроходный layout с junction rules | Адаптивный отклик на viewport без media queries |
| **Semantic tokens** | "sm"/"md"/"lg" вместо пикселей | Устойчивость к изменениям, целостность дизайна |
| **Rigidity** | locked/preferred/flexible | Гибкий приоритет: что можно менять, а что нельзя |
| **Data-driven atoms** | Field definitions из БД → автоматические AtomV2 | Не нужно описывать каждый атом — данные сами определяют структуру |
| **Format system** | currency/stars/stars-compact/percent/date | Умное форматирование значений по subtype |
| **Wrapper ≠ TextStyle** | Разделение "как выглядит текст" и "что его оборачивает" | Комбинаторная гибкость: bold text в badge ≠ bold badge |
| **Cross-widget consistency** | C1-C5 правила: единообразие полей, форматов, стилей | Несколько виджетов выглядят как единое целое |
| **Priority-based culling** | При overflow — скрытие по приоритету | Graceful degradation, а не битый UI |

---

## 8. Information Layer — Ключевое архитектурное различие

> Это **главное** отличие Keepstar от Pencil и от любых других AI-UI инструментов.
> Pencil работает с пикселями. Keepstar работает с **данными**.

### 8.1 Pencil: данных нет

Pencil оперирует **визуальными нодами**, не данными. Когда вставляется текст:

```javascript
title=I(parent, {type:"text", content:"iPhone 16 Pro", fontSize:24})
```

`content: "iPhone 16 Pro"` — просто строка. Pencil не знает:
- Что это название товара
- Что рядом должна быть цена
- Что есть ещё 49 таких товаров
- Что "999.00" надо отформатировать как "$999.00"
- Что "4.7" — это рейтинг и нужны звёзды

**Вся** ответственность за понимание данных лежит на LLM. Claude должен сам:
1. Понять что за данные перед ним
2. Решить как их визуально представить
3. Выбрать правильный формат отображения
4. Расставить по правильным местам в layout
5. Обеспечить единообразие между элементами

Это работает, но ненадёжно — LLM может забыть отформатировать цену, поставить рейтинг не в ту зону, использовать разные стили для одного типа данных в разных виджетах.

### 8.2 Keepstar: данные — корень всего

Keepstar начинает с **данных** и автоматически строит визуальное представление:

```
Сырые данные (Product/Service из каталога)
    ↓ FieldDefinitions (из БД catalog.field_definitions)
Типизированные поля
    ↓ buildTypedAtoms()
AtomV2[] (type + subtype + format + textStyle + wrapper + slot + priority)
    ↓ AutoLayout()
LayoutNode tree (семантическая группировка)
    ↓ Constraints (30+ правил)
Гарантированно валидный UI
```

Пример автоматической классификации:

```
Product {
  name: "iPhone 16 Pro"    → AtomV2{type:text,   slot:title,  textStyle:{fontSize:xl, fontWeight:bold}}
  price: 999.00            → AtomV2{type:number, subtype:currency, format:currency, slot:price}
  rating: 4.7              → AtomV2{type:number, subtype:rating,   format:stars-compact}
  images: ["url1","url2"]  → AtomV2{type:image,  slot:hero, mediaStyle:{aspectRatio:4:3}}
  brand: "Apple"           → AtomV2{type:text,   wrapper:{type:badge}}
  tags: ["flagship","5G"]  → AtomV2{type:text,   wrapper:{type:tag}} × N
  in_stock: true           → AtomV2{type:text,   slot:stock, wrapper:{type:badge, variant:success}}
}
```

Движок **автоматически** решает:
- `price: 999.00` → это деньги → `format:currency` → "$999.00"
- `rating: 4.7` → это рейтинг → `format:stars-compact` → "★ 4.7"
- `images` → это медиа → `slot:hero` → крупно сверху
- `brand: "Apple"` → короткий текст → `wrapper:badge`
- `in_stock: true` → boolean → badge-success "В наличии"

**LLM не участвует** в этом процессе. Классификация детерминированная.

### 8.3 Сравнение цепочек

```
Pencil:
  LLM понимает данные → LLM решает формат → LLM строит layout → LLM стилизует
       ↑ ненадёжно          ↑ ненадёжно         ↑ ненадёжно        ↑ ненадёжно
  Каждый шаг может провалиться. Нет гарантий. Починка через screenshot loop.

Keepstar:
  Данные → Classify (код) → Atomize (код) → AutoLayout (код) → Constraints (код)
              ↑ надёжно        ↑ надёжно        ↑ надёжно          ↑ надёжно
  LLM нужен только для intent: "покажи как каталог" / "покажи детально" / "сравни"
```

### 8.4 Текущее ограничение: только e-commerce

Сейчас цепочка `Data → AtomV2` работает только для `Product` и `Service` через:
- `FieldDefinitions` в БД (таблица `catalog.field_definitions`)
- `ProductToMap()` / `ServiceToMap()` — конвертация в generic map
- `GenericFieldGetter` — извлечение значений по имени поля
- Hardcoded field rankings: `["images","name","brand","price","rating",...]`

Для универсального UI нужно уметь работать с **любыми** данными.

### 8.5 Universal Field Classifier — предложение

Автоматический классификатор произвольных данных без LLM:

```go
// engine/field_classifier.go

type FieldClassifier struct{}

type ClassifiedField struct {
    Name      string            // Имя поля из JSON
    AtomType  AtomType          // text, number, image, icon, video, audio + новые
    Subtype   AtomSubtype       // currency, rating, url, date, percent...
    Semantic  FieldSemantic     // title, metric, list, nested, media, identifier...
    Priority  int               // Автоматически: title=0, media=1, price=2, body=5...
    IsList    bool              // true для массивов
    Children  []ClassifiedField // для вложенных объектов
    Confidence float64          // 0-1, уверенность классификации
}

type FieldSemantic string
const (
    SemanticTitle       FieldSemantic = "title"       // Заголовок (name, title, label)
    SemanticDescription FieldSemantic = "description" // Длинный текст
    SemanticMedia       FieldSemantic = "media"       // Изображение, видео
    SemanticPrice       FieldSemantic = "price"       // Цена, стоимость
    SemanticMetric      FieldSemantic = "metric"      // Числовая метрика
    SemanticStatus      FieldSemantic = "status"      // Статус, состояние
    SemanticTag         FieldSemantic = "tag"          // Тег, категория
    SemanticDate        FieldSemantic = "date"         // Дата, время
    SemanticLocation    FieldSemantic = "location"     // Координаты, адрес
    SemanticContact     FieldSemantic = "contact"      // Email, телефон, URL
    SemanticList        FieldSemantic = "list"         // Массив элементов
    SemanticNested      FieldSemantic = "nested"       // Вложенный объект
    SemanticIdentifier  FieldSemantic = "identifier"   // ID, SKU, код
    SemanticBoolean     FieldSemantic = "boolean"      // Да/нет
)

func (c *FieldClassifier) Classify(data map[string]interface{}) []ClassifiedField
```

#### Правила классификации (детерминированные, без LLM)

**По значению:**

```
Значение — строка?
  ├── Regex URL + расширение (.jpg/.png/.webp/.gif/.svg) → image
  ├── Regex URL + video расширение (.mp4/.webm)          → video
  ├── Regex URL (другие)                                  → text, subtype:url
  ├── Regex email (contains @)                            → text, subtype:email
  ├── Regex phone (+7, 8-xxx)                             → text, subtype:phone
  ├── Regex ISO date / date-like                          → text, subtype:date
  ├── Regex hex color (#xxx, #xxxxxx)                     → text, subtype:color
  ├── Длина < 30 символов                                 → text (short → candidate title)
  ├── Длина 30-100                                        → text (medium → body)
  └── Длина > 100                                         → text (long → description)

Значение — число?
  ├── Имя: price|cost|amount|total|sum|fee                → number, subtype:currency
  ├── Имя: rating|score|stars|review                      → number, subtype:rating
  ├── Имя: percent|rate|change|growth|discount            → number, subtype:percent
  ├── Значение 0-5 + имя похоже на score                  → number, subtype:rating
  ├── Значение 0-100 + имя похоже на rate                 → number, subtype:percent
  └── Иначе                                               → number, subtype:int/float

Значение — boolean?
  ├── Имя: active|enabled|available|in_stock|verified     → badge (success/error)
  └── Иначе                                               → badge (neutral)

Значение — массив?
  ├── Массив строк, средняя длина < 20                    → tags (AtomType:text, wrapper:tag)
  ├── Массив URL-изображений                              → gallery (multiple image atoms)
  ├── Массив объектов                                     → nested widgets (рекурсия)
  └── Массив чисел                                        → chart data candidate

Значение — объект?
  ├── Содержит lat/lng или latitude/longitude             → map (Phase B)
  ├── Содержит nested objects                             → рекурсивная классификация
  └── Иначе                                               → inline group

Значение — null/undefined?
  └── Скрыть (не создавать атом)
```

**По имени поля (семантические эвристики):**

```
Имя поля → Semantic → Priority

title|name|label|heading          → SemanticTitle       → priority 0
image|photo|picture|avatar|logo   → SemanticMedia       → priority 1
price|cost|amount|fee|total       → SemanticPrice       → priority 2
rating|score|stars|review_score   → SemanticMetric      → priority 3
status|state|condition|phase      → SemanticStatus      → priority 4
tag|category|type|kind|genre      → SemanticTag         → priority 5
description|summary|about|bio|text → SemanticDescription → priority 6
date|time|created|updated|due     → SemanticDate        → priority 7
email|phone|website|url|link      → SemanticContact     → priority 8
address|location|city|country     → SemanticLocation    → priority 9
id|sku|code|ref|number            → SemanticIdentifier  → priority 10 (обычно скрыт)
```

**Комбинированная уверенность:**

```
Совпадение имени + типа значения = high confidence (0.9+)
  Пример: поле "price" + значение число → currency (0.95)

Только тип значения = medium confidence (0.6-0.8)
  Пример: значение 4.5 без "rating" в имени → может быть rating, может быть нет

Только имя = low confidence (0.4-0.6)
  Пример: поле "score" + значение строка "A+" → вероятно метрика, но не число
```

### 8.6 Semantic Layout Strategies

Разные паттерны данных → разные layout стратегии. Классификатор определяет не только поля, но и **тип визуализации**:

```go
type DataPattern string

const (
    PatternCatalog    DataPattern = "catalog"    // Много однородных сущностей
    PatternDetail     DataPattern = "detail"     // Одна сущность, много полей
    PatternComparison DataPattern = "comparison" // 2-4 сущности для сравнения
    PatternDashboard  DataPattern = "dashboard"  // Метрики, KPI, графики
    PatternTimeline   DataPattern = "timeline"   // Хронология, шаги, история
    PatternNested     DataPattern = "nested"     // Древовидные данные
    PatternMixed      DataPattern = "mixed"      // Разнородные данные → sections
)

// DetectPattern анализирует структуру данных и определяет оптимальный паттерн
func DetectPattern(items []map[string]interface{}, fields []ClassifiedField) DataPattern
```

**Правила определения паттерна:**

```
Массив 5+ объектов с одинаковыми полями
  → PatternCatalog
  → layout: grid (5-12 items) / list (13+)
  → size: medium (5-8) / small (9+)
  → fields: title + media + 2-3 ключевых поля

Один объект с 10+ полями
  → PatternDetail
  → layout: single
  → size: large
  → sections: hero → основное → характеристики → описание

2-4 объекта + intent "сравни"
  → PatternComparison
  → layout: comparison
  → fields: выравнены для сравнения

Объект с 3+ числовыми полями + change/trend/period
  → PatternDashboard
  → layout: grid
  → size: large
  → atoms: number с emphasis + chart (Phase B)

Массив объектов с date/step/order полем
  → PatternTimeline
  → layout: list vertical
  → atoms: date → title → description (в этом порядке)

Объекты с children/items/sub-*
  → PatternNested
  → layout: sections
  → рекурсивная обработка children
```

**Маппинг Semantic → Layout Strategy:**

| Паттерн | Layout | Size | AutoLayout группировка | Характерные поля |
|---------|--------|------|------------------------|------------------|
| Catalog | grid/list | small-medium | hero → title+price row → tags flow | title, image, price |
| Detail | single | large | hero span → headings column → specs column → description | title, image, description, specs |
| Comparison | comparison | medium | aligned fields across widgets | Одинаковые поля в каждом |
| Dashboard | grid | large | metric number (xl) → label (sm) → change badge | value, label, change, period |
| Timeline | list | medium | date badge → title → description column | date, title, description |
| Nested | sections | varies | parent section → child widgets | parent.children |

### 8.7 Presentation Intent Layer

Одни и те же данные могут быть представлены по-разному в зависимости от **намерения** пользователя:

```
Данные: 10 продуктов из каталога

Intent "покажи товары"          → PatternCatalog   → grid, compact cards
Intent "расскажи про первый"    → PatternDetail    → single, large, все поля
Intent "сравни первый и третий" → PatternComparison → comparison table
Intent "что дешевле?"           → PatternCatalog   → list sorted by price, price highlighted
Intent "покажи тренд продаж"    → PatternDashboard  → chart + KPI cards
Intent "сделай landing"         → PatternMixed      → hero section + grid + CTA
```

**Кто определяет intent:**

```
Agent1 (NLU) → анализирует запрос пользователя → записывает meta:
  {
    "intent": "compare",      // что хочет пользователь
    "focus_fields": ["price", "rating"],  // на чём акцент
    "sort_by": "price",       // порядок
    "limit": 3                // сколько показать
  }

Agent2 (Render) → получает meta + вызывает visual_assembly:
  {
    "layout": "comparison",   // из intent
    "show": ["price","rating","name","images"],  // focus_fields + defaults
    "atoms": {
      "price": {"textStyle": {"fontSize": "xl", "fontWeight": "bold"}}  // акцент
    }
  }

Engine → AutoResolve + Classify + AutoLayout + Constraints → Formation
```

**Intent не меняет данные — он меняет как данные представлены.** Классификатор определяет "что есть", intent определяет "что подчеркнуть".

### 8.8 Сравнительная таблица: Information Layer

| Возможность | Pencil | Keepstar V2 (сейчас) | Keepstar (цель) |
|-------------|--------|----------------------|-----------------|
| Источник данных | Нет — LLM вписывает контент руками | Product/Service из каталога | Любой JSON |
| Классификация полей | LLM (ненадёжно) | FieldDefinitions из БД (для e-commerce) | Universal Field Classifier (детерминированный) |
| Автоформатирование | Нет | Да (currency, stars, percent, date) | Да + расширенные форматы |
| Семантические слоты | Нет | Да (hero, title, price, primary, secondary...) | Да + динамические слоты |
| Layout по данным | Нет — LLM сам решает | Да (AutoResolve по count + entity type) | Да + DataPattern detection |
| Consistency | Нет гарантий | C1-C5 правила | Расширенные cross-pattern rules |
| Priority/Culling | Нет | Да (priority + rigidity) | Да + confidence-based |
| Nested data | Рекурсивные frames | Нет | Рекурсивная классификация |
| Intent awareness | LLM решает всё | Agent2 передаёт layout/size/show/hide | Structured intent от Agent1 → strategy selection |

### 8.9 Почему это уникально

> **Ни один другой AI-UI инструмент не начинает с данных.**

- **Pencil/Figma AI** — рисуют UI, контент вставляет LLM или пользователь
- **v0.dev** — генерирует React код с hardcoded данными
- **Vercel AI SDK** — рендерит React компоненты, но LLM решает какие
- **GPT-4 Canvas** — текстовый редактор, не UI engine

Keepstar — единственный, кто делает: `данные → автоклассификация → auto-layout → constraint validation → готовый UI`. Расширение классификатора за пределы e-commerce превращает это в **универсальный data-to-UI engine** — значительно более мощный, чем pixel-first подход Pencil.

---

## 9. Gap Analysis — Что нужно добавить для универсального UI (визуальный слой)

### 8.1 Текущие ограничения Keepstar V2

Сейчас движок ограничен e-commerce карточками. Для универсального UI нужно преодолеть:

1. **Только data-driven** — атомы создаются из данных каталога. Нет возможности вставить произвольный текст/изображение/кнопку.
2. **Фиксированные типы** — 6 atom types. Нет: divider, chart, map, code, table cell, progress bar (как первоклассный тип), embed.
3. **Один уровень вложенности** — Formation → Widgets → Atoms. Нет рекурсивных компонентов.
4. **Нет arbitrary layout** — AutoLayout группирует по правилам. Агент не может задать произвольное дерево.
5. **Нет CRUD на уровне нод** — нельзя Insert/Delete/Move отдельные ноды. Только show/hide/order.
6. **Нет component instances** — каждый виджет строится заново. Нет "возьми этот и измени".
7. **Нет theming** — одна тема, нет переключения.
8. **Нет image generation** — только URLs из каталога.
9. **Нет visual validation** — testbench, но нет screenshot → LLM → fix loop.

### 8.2 Roadmap: от e-commerce к универсальному UI

---

## Phase A: Foundation — Binding DSL & Path Addressing

**Цель**: Дать агенту возможность строить произвольные деревья, сохраняя constraint safety.

### A.1 Node Addressing (Path System)

```
Formation/Section[0]/Widget[1]/Layout/row-0/column-1/atom-3
                                         ↑ layout path
```

Добавить систему адресации:
```go
// domain/node_path.go
type NodePath string  // "widget:0/layout/row:0/atom:3"

func (p NodePath) Resolve(formation *FormationWithData) (interface{}, error)
func (p NodePath) Parent() NodePath
func (p NodePath) Child(segment string) NodePath
```

Использование агентом:
```json
{
  "update": {
    "widget:0/layout/row:0/atom:1": {
      "textStyle": {"fontSize": "xl", "fontWeight": "bold"}
    }
  }
}
```

### A.2 Binding References

Дать агенту возможность ссылаться на ноды по binding:
```json
{
  "operations": [
    {"op": "insert", "bind": "promo", "parent": "widget:0/layout", "node": {
      "type": "row", "gap": "md", "children": []
    }},
    {"op": "insert", "parent": "$promo", "node": {
      "type": "atom", "atomType": "text", "value": "Sale!",
      "textStyle": {"fontSize": "2xl", "color": "#EF4444"}
    }},
    {"op": "insert", "parent": "$promo", "node": {
      "type": "atom", "atomType": "image", "value": "https://...",
      "mediaStyle": {"aspectRatio": "1:1", "objectFit": "cover"}
    }}
  ]
}
```

**Важно**: операции проходят через те же constraints. Insert ноды валидируется W1-W8 + junction rules. Это ключевое отличие от Pencil — у нас safety net.

### A.3 CRUD Operations

```go
type NodeOperation struct {
    Op      string      // "insert", "update", "delete", "move", "replace"
    Bind    string      // Optional binding name
    Path    NodePath    // Target path (for update/delete/move)
    Parent  string      // Parent path or $binding (for insert/move)
    Index   *int        // Position among siblings (for insert/move)
    Node    interface{} // Node data (for insert/replace)
    Props   interface{} // Properties to update (for update)
}
```

**Оценка**: ~500 LOC backend, ~100 LOC frontend
**Приоритет**: Высокий — это foundation для всего остального

---

## Phase B: Extended Atom Types

**Цель**: Поддержка типов контента за пределами e-commerce.

### B.1 Новые AtomTypes

```go
const (
    // Существующие
    AtomTypeText   AtomType = "text"
    AtomTypeNumber AtomType = "number"
    AtomTypeImage  AtomType = "image"
    AtomTypeIcon   AtomType = "icon"
    AtomTypeVideo  AtomType = "video"
    AtomTypeAudio  AtomType = "audio"

    // Новые
    AtomTypeDivider   AtomType = "divider"    // Горизонтальная линия
    AtomTypeChart     AtomType = "chart"      // Графики (bar, line, pie, donut)
    AtomTypeMap       AtomType = "map"        // Карта с маркером
    AtomTypeCode      AtomType = "code"       // Код с подсветкой
    AtomTypeEmbed     AtomType = "embed"      // iframe / external widget
    AtomTypeProgress  AtomType = "progress"   // Progress bar (сейчас wrapper, станет тип)
    AtomTypeSpacer    AtomType = "spacer"     // Пустое пространство заданного размера
    AtomTypeRichText  AtomType = "rich_text"  // Markdown / formatted text
)
```

### B.2 Новые стили для новых типов

```go
// ChartStyle — для AtomTypeChart
type ChartStyle struct {
    ChartType  string                 `json:"chartType"`  // "bar", "line", "pie", "donut", "area"
    Data       []ChartDataPoint       `json:"data"`
    Colors     []string               `json:"colors,omitempty"`
    ShowLegend bool                   `json:"showLegend,omitempty"`
    ShowLabels bool                   `json:"showLabels,omitempty"`
    Height     string                 `json:"height,omitempty"` // token: "sm"/"md"/"lg"
}

// MapStyle — для AtomTypeMap
type MapStyle struct {
    Lat     float64 `json:"lat"`
    Lng     float64 `json:"lng"`
    Zoom    int     `json:"zoom,omitempty"`    // default 14
    Height  string  `json:"height,omitempty"`  // token
    Style   string  `json:"style,omitempty"`   // "streets", "satellite", "minimal"
}

// CodeStyle — для AtomTypeCode
type CodeStyle struct {
    Language   string `json:"language,omitempty"` // "go", "js", "python"...
    Theme      string `json:"theme,omitempty"`    // "dark", "light"
    ShowLines  bool   `json:"showLines,omitempty"`
    MaxLines   int    `json:"maxLines,omitempty"`
}

// DividerStyle — для AtomTypeDivider
type DividerStyle struct {
    Variant  string `json:"variant,omitempty"` // "solid", "dashed", "dotted", "gradient"
    Color    string `json:"color,omitempty"`
    Spacing  string `json:"spacing,omitempty"` // token: "sm"/"md"/"lg"
}

// EmbedStyle — для AtomTypeEmbed
type EmbedStyle struct {
    URL       string `json:"url"`
    Height    string `json:"height,omitempty"`
    Sandbox   bool   `json:"sandbox,omitempty"`   // iframe sandbox
    AllowList string `json:"allowList,omitempty"` // iframe allow
}
```

### B.3 Новые constraints для новых типов

```
A10: Chart без data → скрыть
A11: Map без lat/lng → скрыть
A12: Code > maxLines → truncate с "Show more"
A13: Embed URL → валидация allowlist (XSS protection)
A14: RichText → sanitize HTML (XSS)
W10: Max 1 chart per widget (space budget)
W11: Max 1 map per widget
W12: Chart/Map в tiny/small виджете → скрыть (не хватает места)
```

**Оценка**: ~400 LOC backend (types + constraints), ~600 LOC frontend (renderers), phased delivery
**Приоритет**: Средний — расширяет домен, но не блокирует core features

---

## Phase C: Recursive Components & Instances

**Цель**: Полноценная component system — определяй компоненты, создавай instances, override вглубь.

### C.1 ComponentDef — определение компонента

```go
type ComponentDef struct {
    ID          string          `json:"id"`
    Name        string          `json:"name"`          // "ProductHero", "PriceBlock", "ReviewCard"
    Description string          `json:"description"`
    EntityType  EntityType      `json:"entityType,omitempty"` // Привязка к типу данных
    Layout      *LayoutNode     `json:"layout"`        // Структура дерева
    Atoms       []AtomV2        `json:"atoms"`         // Атомы компонента
    Slots       []ComponentSlot `json:"slots"`         // Точки расширения
    Tokens      map[string]string `json:"tokens,omitempty"` // Локальные design tokens
}

type ComponentSlot struct {
    ID          string      `json:"id"`           // "actions", "footer", "sidebar"
    Name        string      `json:"name"`
    AcceptTypes []AtomType  `json:"acceptTypes,omitempty"` // Что можно вставлять
    MinChildren int         `json:"minChildren,omitempty"`
    MaxChildren int         `json:"maxChildren,omitempty"`
    Default     *LayoutNode `json:"default,omitempty"` // Дефолтное содержимое
}
```

### C.2 ComponentInstance — использование

```go
type ComponentInstance struct {
    Ref       string                 `json:"ref"`       // ID ComponentDef
    Overrides map[string]interface{} `json:"overrides"`  // path → props override
    SlotContent map[string]*LayoutNode `json:"slotContent,omitempty"` // slot → custom content
}
```

Использование агентом:
```json
{
  "operations": [
    {"op": "insert", "bind": "card", "parent": "widget:0/layout", "node": {
      "type": "component",
      "ref": "PriceBlock",
      "overrides": {
        "price-text": {"textStyle": {"color": "#EF4444"}},
        "currency-badge": {"wrapper": {"variant": "success"}}
      },
      "slotContent": {
        "actions": {
          "type": "row",
          "gap": "sm",
          "children": [
            {"atomType": "icon", "value": "cart", "iconStyle": {"size": "md"}}
          ]
        }
      }
    }}
  ]
}
```

### C.3 ComponentRegistry

```go
type ComponentRegistry struct {
    components map[string]*ComponentDef
}

func (r *ComponentRegistry) Register(def *ComponentDef)
func (r *ComponentRegistry) Get(id string) *ComponentDef
func (r *ComponentRegistry) Resolve(instance *ComponentInstance) (*LayoutNode, []AtomV2, error)
func (r *ComponentRegistry) ListByEntityType(et EntityType) []*ComponentDef
```

Resolve выполняет:
1. Загрузка ComponentDef по ref
2. Deep copy layout + atoms
3. Применение overrides по path
4. Вставка slotContent в slots
5. Валидация constraints (acceptTypes, min/maxChildren)
6. Возврат готового LayoutNode + AtomV2[] для рендера

### C.4 Отличие от Pencil Components

| Аспект | Pencil | Keepstar (предложение) |
|--------|--------|------------------------|
| Определение | В .pen файле как `reusable: true` frame | В ComponentRegistry (код + DB) |
| Инстанс | `ref: "componentId"` | `ref: "ComponentName"` |
| Override | Через path: `U(instance+"/child")` | Через path map: `overrides: {"child": {...}}` |
| Slots | Неявно — любой child можно заменить через R() | Явные слоты с constraints (acceptTypes, min/max) |
| Валидация | Нет | Constraints на слотах + engine pipeline |
| Рекурсия | Компонент может содержать ref на другой компонент | То же — ComponentDef.Layout может содержать ComponentInstance |

**Оценка**: ~800 LOC backend, ~300 LOC frontend
**Приоритет**: Средний-Высокий — ключевой для повторного использования дизайнов

---

## Phase D: Multi-Theme Variables

**Цель**: Поддержка тем (light/dark/brand) с runtime switching.

### D.1 ThemeDefinition

```go
type ThemeDefinition struct {
    Name      string                       `json:"name"`      // "light", "dark", "brand-cosmo"
    Variables map[string]string            `json:"variables"`  // token → value
    Extends   string                       `json:"extends,omitempty"` // base theme
}

type ThemeRegistry struct {
    themes  map[string]*ThemeDefinition
    active  string
}

// Resolve with inheritance
func (r *ThemeRegistry) ResolveToken(theme, token string) string
```

### D.2 Variable Binding

```go
// AtomV2 extension
type AtomV2 struct {
    // ... existing fields ...

    // Variable bindings — значение берётся из текущей темы
    TextStyleVar  map[string]string `json:"textStyleVar,omitempty"`  // {"color": "$text-primary", "fontSize": "$heading-size"}
    WrapperVar    map[string]string `json:"wrapperVar,omitempty"`    // {"color": "$accent"}
}
```

### D.3 Frontend Theme Switching

```jsx
// ThemeProvider wraps formation
<ThemeProvider theme={activeTheme} variables={resolvedVars}>
  <FormationRenderer formation={formation} />
</ThemeProvider>
```

CSS Variables injection:
```css
.formation-root {
  --text-primary: var(--theme-text-primary, #111827);
  --bg-primary: var(--theme-bg-primary, #FFFFFF);
  --accent: var(--theme-accent, #8B5CF6);
}
```

**Оценка**: ~300 LOC backend, ~200 LOC frontend
**Приоритет**: Средний — важно для white-label и B2B

---

## Phase E: Visual Validation Loop

**Цель**: Автоматическая проверка визуального результата через vision model.

### E.1 Screenshot Pipeline

```go
type VisualValidator struct {
    renderer    ScreenshotRenderer  // Headless browser (Playwright/Puppeteer)
    visionLLM   ports.LLMPort      // Claude with vision
}

func (v *VisualValidator) Validate(formation *FormationWithData) (*ValidationResult, error) {
    // 1. Render formation to HTML
    html := v.renderToHTML(formation)

    // 2. Take screenshot via headless browser
    screenshot := v.renderer.Screenshot(html, ViewportConfig{Width: 400, Height: 600})

    // 3. Send to vision model with validation prompt
    result := v.visionLLM.Analyze(screenshot, validationPrompt)

    // 4. Parse issues
    return parseValidationResult(result)
}

type ValidationResult struct {
    Pass     bool              `json:"pass"`
    Issues   []VisualIssue     `json:"issues"`
    Score    float64           `json:"score"` // 0-1
}

type VisualIssue struct {
    Type     string `json:"type"`     // "overlap", "truncation", "alignment", "contrast", "spacing"
    Severity string `json:"severity"` // "error", "warning", "info"
    Location string `json:"location"` // Описание где проблема
    Fix      string `json:"fix"`      // Предложение исправления
}
```

### E.2 Auto-fix Loop

```
Formation → Render → Screenshot → Vision LLM → Issues?
  ↑                                                │
  │                    NO: done                    │
  │                                                │
  └── YES: apply fixes (constraints) ─────────────┘
                    (max 2 iterations)
```

Fixes маппятся на существующие constraints:
- "overlap" → junction rule (compress/switch)
- "truncation" → D5/D6 (lineClamp/fontSize downgrade)
- "poor contrast" → theme variable adjustment
- "misalignment" → layout distribution fix

### E.3 Отличие от Pencil

| Аспект | Pencil | Keepstar (предложение) |
|--------|--------|------------------------|
| Trigger | Ручной — LLM вызывает get_screenshot | Автоматический — часть pipeline |
| Analysis | LLM "смотрит" на скриншот | Structured output от vision model |
| Fix | LLM сам решает что делать → batch_design | Маппинг issues → constraints |
| Iterations | Неограниченно (LLM в цикле) | Max 2 (как junction rules) |
| Cost | Высокий (LLM в цикле) | Контролируемый (optional, off by default) |

**Оценка**: ~400 LOC backend, ~100 LOC headless renderer, ~200 LOC validation prompts
**Приоритет**: Низкий — nice-to-have, дорого по токенам

---

## Phase F: Arbitrary Content (Free-form Atoms)

**Цель**: Агент может вставлять произвольный контент, не привязанный к данным каталога.

### F.1 Free-form Atom Source

Сейчас атомы создаются только из `buildTypedAtoms()` (данные каталога). Нужен второй путь:

```go
type AtomSource string

const (
    AtomSourceData    AtomSource = "data"    // Из каталога (существующий путь)
    AtomSourceAgent   AtomSource = "agent"   // Агент создал вручную
    AtomSourceSystem  AtomSource = "system"  // Системный (generated by engine)
)

type AtomV2 struct {
    // ... existing fields ...
    Source AtomSource `json:"source,omitempty"` // default: "data"
}
```

### F.2 Agent-created Atoms в instructions

```json
{
  "inject": [
    {
      "type": "text",
      "value": "Специальное предложение! Скидка 20% на всё",
      "textStyle": {"fontSize": "xl", "fontWeight": "bold", "color": "#EF4444"},
      "wrapper": {"type": "alert", "variant": "warning"},
      "position": "before:widget:0"
    },
    {
      "type": "divider",
      "position": "after:widget:0",
      "dividerStyle": {"variant": "gradient"}
    },
    {
      "type": "image",
      "value": "generate:promotional banner for cosmetics sale",
      "mediaStyle": {"aspectRatio": "21:9"},
      "position": "before:all"
    }
  ]
}
```

### F.3 Image Generation Integration

```go
type ImageGenerator interface {
    Generate(prompt string, style string, size ImageSize) (string, error) // Returns URL
    Stock(query string) (string, error) // Returns Unsplash URL
}
```

В pipeline:
```
if atom.Value starts with "generate:" → call ImageGenerator
if atom.Value starts with "stock:" → call Stock search
else → use as URL
```

**Оценка**: ~300 LOC backend, ~50 LOC frontend
**Приоритет**: Высокий — разблокирует промо-контент, landing pages, rich content

---

## Phase G: Style Guide & Design System Export

**Цель**: Система стайл-гайдов для вдохновения + экспорт в CSS/Tailwind.

### G.1 Style Guide

По аналогии с Pencil `get_style_guide`:

```go
type StyleGuide struct {
    Name       string                    `json:"name"`
    Tags       []string                  `json:"tags"`       // "minimal", "bold", "e-commerce"
    Tokens     DesignTokensV2            `json:"tokens"`     // Кастомные токены
    Theme      ThemeDefinition           `json:"theme"`      // Цветовая палитра
    Components []ComponentStyleSnapshot  `json:"components"` // Примеры компонентов
    Typography TypographyGuide           `json:"typography"` // Шрифты, размеры, иерархия
}

type StyleGuideRegistry struct {
    guides map[string]*StyleGuide
    tags   map[string][]string // tag → guide names
}

func (r *StyleGuideRegistry) Search(tags []string) []*StyleGuide
func (r *StyleGuideRegistry) Get(name string) *StyleGuide
```

### G.2 Export

```go
type DesignExporter struct {}

func (e *DesignExporter) ToCSS(tokens DesignTokensV2, theme ThemeDefinition) string
func (e *DesignExporter) ToTailwindConfig(tokens DesignTokensV2) string
func (e *DesignExporter) ToFigmaTokens(tokens DesignTokensV2) map[string]interface{}
```

**Оценка**: ~400 LOC
**Приоритет**: Низкий — полезно для B2B white-label, не критично для core

---

## 10. Приоритизированная дорожная карта

```
┌─────────────────────────────────────────────────────────┐
│ CRITICAL PATH (для универсального UI)                    │
│                                                          │
│ Phase H: Universal Field Classifier        ~500 LOC     │
│   └── Любой JSON → ClassifiedField[] (без LLM)          │
│   └── Semantic detection (title/price/media/status...)   │
│   └── DataPattern detection (catalog/detail/dashboard)   │
│   └── Это ФУНДАМЕНТ — всё остальное строится на этом     │
│                                                          │
│ Phase A: Binding DSL & Path Addressing     ~600 LOC     │
│   └── Foundation для визуальных фаз                     │
│                                                          │
│ Phase F: Free-form Atoms (inject)          ~350 LOC     │
│   └── Агент может вставлять произвольный контент        │
│                                                          │
│ Phase B: Extended Atom Types               ~1000 LOC    │
│   └── chart, map, code, divider, embed, rich_text       │
│                                                          │
├─────────────────────────────────────────────────────────┤
│ HIGH VALUE (масштабируемость)                            │
│                                                          │
│ Phase C: Recursive Components              ~1100 LOC    │
│   └── ComponentDef, instances, slots, registry          │
│                                                          │
│ Phase D: Multi-Theme Variables             ~500 LOC     │
│   └── light/dark/brand themes, runtime switching        │
│                                                          │
├─────────────────────────────────────────────────────────┤
│ NICE TO HAVE (polish)                                    │
│                                                          │
│ Phase E: Visual Validation Loop            ~700 LOC     │
│   └── Screenshot → vision model → auto-fix              │
│                                                          │
│ Phase G: Style Guide & Export              ~400 LOC     │
│   └── Design inspiration + CSS/Tailwind export          │
│                                                          │
└─────────────────────────────────────────────────────────┘

Порядок реализации:
  H (classifier) → A (bindings) → F (inject) → B (types) → C (components) → D (themes) → E/G (polish)

Total estimated: ~5,150 LOC
```

---

## 11. Ключевой принцип эволюции

> **Pencil = свобода без гарантий. Keepstar = гарантии с ограниченной свободой.**
>
> Цель: **свобода С гарантиями.**

Каждая новая возможность (binding DSL, free-form atoms, components) проходит через constraint pipeline. Это наше конкурентное преимущество:

```
Pencil:   LLM → операции → рендер → screenshot → LLM проверяет → фиксит → ...
Keepstar: LLM → операции → constraints АВТОМАТИЧЕСКИ → рендер → ГОТОВО
```

LLM ненадёжен в precise layout. Constraints компенсируют это.
Pencil тратит дополнительные LLM-вызовы на валидацию.
Keepstar гарантирует результат за один проход.

Расширяя свободу агента (binding DSL, arbitrary content), мы сохраняем safety net constraints. Это делает наш движок уникальным: **мощность Pencil + надёжность constraint engine**.

---

## 12. Сводная таблица: текущее vs целевое состояние

| Возможность | Pencil | Keepstar V2 (сейчас) | Keepstar V3 (цель) | Фаза |
|-------------|--------|----------------------|---------------------|------|
| Binding DSL | ✅ | ❌ | ✅ | A |
| Path addressing | ✅ | ❌ | ✅ | A |
| CRUD операции | ✅ | ❌ (show/hide only) | ✅ | A |
| Free-form content | ✅ | ❌ | ✅ | F |
| Image generation | ✅ | ❌ | ✅ | F |
| Charts/Maps/Code | ❌ | ❌ | ✅ | B |
| Divider/Spacer | ❌ (manual frame) | ❌ | ✅ | B |
| Rich text | ✅ (text node) | ❌ | ✅ | B |
| Component instances | ✅ | ❌ (presets only) | ✅ | C |
| Slot system | ❌ (implicit) | ❌ | ✅ (explicit with constraints) | C |
| Multi-theme | ✅ | ❌ | ✅ | D |
| Variable binding | ✅ | ❌ | ✅ | D |
| Visual validation | ✅ (manual) | ❌ | ✅ (auto) | E |
| Style guides | ✅ | ❌ | ✅ | G |
| **INFORMATION LAYER** | | | | |
| Universal field classify | ❌ (LLM делает) | ❌ (только e-commerce) | ✅ (любой JSON) | H |
| Semantic detection | ❌ | ✅ (FieldDefinitions) | ✅ (auto + DB) | H |
| DataPattern detection | ❌ | ❌ (hardcoded layouts) | ✅ (catalog/detail/dashboard/timeline) | H |
| Presentation intent | ❌ (LLM решает) | ✅ (partial, через Agent2) | ✅ (structured intent) | H |
| Nested data handling | ✅ (recursive frames) | ❌ | ✅ (recursive classify) | H |
| Confidence scoring | ❌ | ❌ | ✅ (0-1 per field) | H |
| **EXISTING ADVANTAGES** | | | | |
| Auto-resolve | ❌ | ✅ | ✅ | — |
| Constraint pipeline | ❌ | ✅ | ✅ (расширенный) | — |
| BudgetDown/NeedsUp | ❌ | ✅ | ✅ | — |
| Semantic tokens | ❌ (px only) | ✅ | ✅ | — |
| Rigidity | ❌ | ✅ | ✅ | — |
| Cross-widget consistency | ❌ | ✅ | ✅ | — |
| Data-driven atoms | ❌ | ✅ (e-commerce) | ✅ (universal) | H |
| Format system | ❌ | ✅ | ✅ (расширенный) | — |
