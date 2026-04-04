# Pencil vs V4 Engine — Full Capability Comparison

**Date**: 2026-04-05
**Purpose**: Понять что Pencil умеет, что из этого есть у нас, чего нет и почему.

---

## 1. LAYOUT

| Capability | Pencil | V4 Engine | Gap |
|---|---|---|---|
| Flexbox (row/column) | `horizontal` / `vertical` | `row` / `column` | **Есть** |
| Flow (wrap) | Нет (вручную строить ряды) | `flow` (flex-wrap) | **V4 лучше** |
| Span (full-width block) | frame + fill_container | `span` | **Есть** |
| Absolute positioning | `layout: "none"` + x/y | Нет | **Нет у нас** — не нужно для виджетов |
| Gap | Числовое (px) | Токены (xs/sm/md/lg/xl) | **Есть**, токены удобнее |
| Padding | 1/2/4 значения (px) | Токены (xs/sm/md/lg/xl) | **Есть**, но только uniform |
| justifyContent | start/center/end/space_between/space_around | `distribution`: start/center/end/between/around/evenly | **Есть** |
| alignItems | start/center/end | `align`: start/center/end/stretch/baseline | **V4 лучше** (stretch, baseline) |
| Sizing: fill_container | Да, с fallback | `sizing: "fill"` на LayoutChild (flex: 1 1 0%) | **Есть** |
| Sizing: fit_content | Да, с fallback | `sizing: "hug"` на LayoutChild (flex: 0 0 auto) | **Есть** |
| Sizing: fixed | Числовое px | `sizing: "fixed"` + min/maxWidth/Height | **Есть** |
| Per-child flex control | layoutPosition: auto/absolute | grow, selfAlign, sizing, min/max | **V4 лучше** |
| Nesting | Неограниченное | Неограниченное (рекурсивный LayoutNode) | **Есть** |
| Grid (CSS Grid) | Нет (только flexbox) | Нет (только flexbox внутри виджета, grid на уровне formation) | **Оба нет** |

**Вывод**: Layout на уровне. V4 даже немного гибче (flow, stretch, baseline). Нет absolute positioning, но для виджетов не нужно.

---

## 2. STYLING & VISUAL PROPERTIES

| Capability | Pencil | V4 Engine | Gap |
|---|---|---|---|
| Solid colors | Hex (#RRGGBBAA) + variables | `background`, `borderColor`, `textStyle.color` — hex + семантика | **Частично** |
| Gradients (linear/radial/angular) | Полная система (stops, rotation, center) | Нет | **НЕТ** |
| Image fills | fill.type="image" на frame | Только через atom type="image" | **Другой подход** |
| Mesh gradients | Да (columns x rows, bezier) | Нет | **НЕТ** (экзотика) |
| Shadows | drop/inner shadow, blur, spread, offset | Токены: none/sm/md/lg | **Упрощённо** |
| Background blur | Да (glassmorphism) | Нет | **НЕТ** |
| Borders | stroke с align/thickness/dash/join/cap | `border`, `borderColor`, `borderWidth` | **Упрощённо** |
| Border radius | Per-corner (4 значения) | Токены: none/sm/md/lg/xl/full | **Есть**, но без per-corner |
| Opacity | 0-1 на любой ноде | `opacity` на LayoutNode (0-100) | **Есть** |
| Blend modes | 16 режимов | Нет | **НЕТ** |
| Overflow control | clip на frame | `overflow`: truncate/wrap/scroll/hide/visible | **Есть** |

**Вывод**: Pencil — полноценный графический редактор. V4 — токенизированная система для UI виджетов. Нет градиентов, blur, blend modes. Но для e-commerce карточек это не критично. **Если хотим генерировать лендинги — нужно добавлять.**

---

## 3. TYPOGRAPHY

| Capability | Pencil | V4 Engine | Gap |
|---|---|---|---|
| Font family | Любой + variables | Не контролируется (наследуется от темы) | **НЕТ** — фронт использует CSS theme |
| Font size | Числовое (px) | Токены: xs/sm/md/lg/xl/2xl/3xl | **Есть** |
| Font weight | Числовое (400/600) | Токены: light/normal/medium/semibold/bold | **Есть** |
| Text color | Hex + variables | Семантические + hex: red/green/blue/muted/error/success | **Есть** |
| Text alignment | left/center/right/justify | Нет отдельного свойства (через layout align) | **НЕТ** напрямую |
| Line height | Ratio (1.0, 1.5) | Токены: tight/normal/relaxed/loose | **Есть** |
| Letter spacing | Числовое | Токены: tight/normal/wide | **Есть** |
| Text decoration | underline/strikethrough | underline/line-through | **Есть** |
| Text transform | - | uppercase/lowercase/capitalize | **V4 лучше** |
| Line clamp | - | lineClamp (число строк) | **V4 лучше** |
| Rich text (inline styles) | Да (массив styled segments) | Нет (атом = один стиль) | **НЕТ** |
| Text growth modes | auto/fixed-width/fixed-width-height | Через sizing на LayoutChild | **Другой подход**, работает |
| Hyperlinks | href на text | wrapper.type="link" | **Есть** |

**Вывод**: Типографика достаточна для UI. Нет контроля fontFamily (но это тема), нет rich text (inline bold+regular). Rich text — gap если хотим описания с форматированием.

---

## 4. COMPONENT SYSTEM

| Capability | Pencil | V4 Engine | Gap |
|---|---|---|---|
| Reusable components | `reusable: true`, instance через `ref` | Нет | **КРИТИЧЕСКИЙ GAP** |
| Component instances | `type: "ref"`, descendant overrides | Нет | **НЕТ** |
| Slots (extension points) | `slot: ["recommended"]` | Нет | **НЕТ** |
| Variant overrides | descendants + property overrides | Нет | **НЕТ** |
| Design system library | Компоненты на canvas | Default ops bundles (2 штуки) | **Зачатки** |

**Вывод**: **Самый большой gap.** Pencil имеет полноценную компонентную систему: создал кнопку → переиспользуй 100 раз с вариациями. У нас — только 2 hardcoded ops bundle. Агент каждый раз генерирует с нуля.

**Но**: для нашего use case (LLM генерирует виджеты) компоненты работали бы иначе — это **Named Ops Bundles** (пресеты ops которые агент вызывает по имени). Это gap #2 из апдейта.

---

## 5. OPERATIONS (batch_design vs ops)

| Capability | Pencil | V4 Engine | Gap |
|---|---|---|---|
| Insert | `I(parent, {props})` | `{op:"insert", parent, props}` | **Есть** |
| Update | `U(id, {props})` | `{op:"update", target, props}` | **Есть** |
| Delete | `D(id)` | `{op:"delete", target}` | **Есть** |
| Move | `M(id, newParent, index)` | `{op:"move", target, parent, after}` | **Есть** |
| Replace | `R(oldId, {props})` | Нет (delete + insert) | **НЕТ** |
| Copy | `C(srcId, parent, {overrides})` | Нет | **НЕТ** |
| Image generation | `G(frameId, "ai"/"stock", prompt)` | Нет (images from data only) | **НЕТ** |
| Ref chaining | `foo=I(...)` → `I(foo, ...)` | `ref:"w"` → `parent:"$w"` | **Есть** |
| Binding DSL | `container+"/child"` | `"$w"` refs | **Есть** (проще) |
| Batch size | Max 25 per call | Без ограничений | **V4 лучше** |
| Atomic rollback | Да | Нет | **НЕТ** (но ок для наших целей) |
| Wildcard targeting | Нет | Да (field name → all widgets) | **V4 лучше** |

**Вывод**: Ops система на уровне. Copy и Replace — nice-to-have. Image generation — отдельная тема (у нас изображения из каталога). Wildcard targeting — наша уникальная фича.

---

## 6. DESIGN TOKENS & THEMES

| Capability | Pencil | V4 Engine | Gap |
|---|---|---|---|
| Color variables | `$variable-name`, type:color | Семантические токены (red, green, muted...) | **Упрощённо** |
| Spacing variables | `$spacing-md` | Фиксированные токены (xs=2px, sm=4px...) | **Есть** но не переменные |
| Number variables | Да | Нет | **НЕТ** |
| String variables | Да (font families) | Нет | **НЕТ** |
| Theme axes | `themes: {device: [mobile, desktop]}` | Нет | **НЕТ** |
| Theme switching | Переменные с per-theme values | Нет (единая тема, CSS variables на фронте) | **НЕТ** |
| Parametric styles | 10 стилей с параметрами | Нет | **НЕТ** |

**Вывод**: Pencil имеет полноценную токен-систему с темами. У нас — hardcoded token maps на фронте. **Для multi-tenant (разные бренды) — нужно.** Сейчас работает потому что один tenant.

---

## 7. CONTENT & DATA

| Capability | Pencil | V4 Engine | Gap |
|---|---|---|---|
| Text content | Строки + styled segments | Atom type=text с fieldName | **Есть** |
| Images | Fill на frame (url, stretch/fill/fit) | Atom type=image с mediaStyle | **Есть** |
| Icons | icon_font (5 семейств, weight) | Atom type=icon (emoji/name) | **Упрощённо** |
| Video | Нет нативного | Atom type=video с controls | **V4 лучше** |
| Audio | Нет нативного | Atom type=audio с controls | **V4 лучше** |
| AI image generation | G() operation | Нет | **НЕТ** |
| Data binding | Нет (всё статичное) | FieldName → entity data, автоматический bind | **V4 СИЛЬНО лучше** |
| Data replication | Нет (руками копировать) | 1 template → N widgets автоматически | **V4 СИЛЬНО лучше** |
| Entity references | Нет | EntityRef на каждом виджете | **V4 уникально** |

**Вывод**: **Data binding и replication — наше главное преимущество.** Pencil — статический дизайн, у нас — динамические данные. Это фундаментальное отличие.

---

## 8. INTERACTIVITY

| Capability | Pencil | V4 Engine | Gap |
|---|---|---|---|
| Click handlers | Нет (статический дизайн) | Widget onClick → expand (через entityRef) | **V4 лучше** |
| Like/favorite | Нет | Widget.Actions + frontend ActionContext | **V4 лучше** |
| Cart/purchase | Нет | Widget.Actions + CartOverlayButton | **V4 лучше** |
| Hover states | Через component variants (manual) | WidgetStates.Hover (CSS variables) | **Есть** |
| Animations | Нет | Нет | **Оба нет** |
| Navigation (expand/back) | Нет | adjacentTemplates + fillFormation | **V4 уникально** |
| Pagination | Нет | PaginationMeta + lazy loading | **V4 уникально** |

**Вывод**: **Интерактивность — наша территория.** Pencil — статика. У нас — клики, навигация, лайки, корзина, пагинация.

---

## 9. GUIDES & PATTERNS

| Capability | Pencil | V4 Engine | Gap |
|---|---|---|---|
| Design System guide | Composing screens, slots, patterns | Нет | **НЕТ** |
| Web App guide | 16 principles (purpose, density, hierarchy...) | Нет | **НЕТ** |
| Landing Page guide | Hero, conversion, rhythm, typography | Нет | **НЕТ** |
| Mobile App guide | Status bar, tab bar, touch targets | Нет | **НЕТ** |
| Table guide | Table→Row→Cell→Content | Нет (formation mode=table не реализован) | **НЕТ** |
| Parametric styles | 10 стилей × 10 палитр × шрифты | Нет | **НЕТ** |

**Вывод**: Pencil имеет **кодифицированные дизайн-знания** через guides и styles. У нас знания — только в Agent2 промпте (неформализованные). **Это второй критический gap после компонентов.**

---

## 10. DISCOVERY & VERIFICATION

| Capability | Pencil | V4 Engine | Gap |
|---|---|---|---|
| Schema introspection | get_editor_state(include_schema) | TreeMap (compact widget tree) | **Оба есть**, разный подход |
| Node search | batch_get(patterns) | Wildcard field targeting | **Оба есть** |
| Visual verification | get_screenshot() | Нет (только через браузер) | **НЕТ** |
| Layout debugging | snapshot_layout(problemsOnly) | Нет | **НЕТ** |
| Property search | search_all_unique_properties | Нет | **НЕТ** |

**Вывод**: У Pencil мощные средства отладки (скриншот, layout snapshot). У нас — только трейсы и DevTools.

---

## ИТОГО: КРИТИЧЕСКИЕ GAPS

### Tier 1 — Нужно для "генерить любой фронт"

| # | Gap | Pencil | Нужно нам | Сложность |
|---|---|---|---|---|
| 1 | **Named ops bundles (компоненты)** | Reusable + instances + slots | Registry ops bundles по имени, агент вызывает "product_card" а не 9 ops | Medium |
| 2 | **Design guides (кодифицированные знания)** | 8 guides с правилами layout | Правила в Agent2 промпте: когда grid, когда list, spacing rules | Medium |
| 3 | **Parametric styles (темизация)** | 10 стилей с палитрами | CSS variables per tenant, токены подхватываются | Medium |

### Tier 2 — Улучшит качество

| # | Gap | Что даст | Сложность |
|---|---|---|---|
| 4 | **Rich text** (inline styles) | Описания с **bold** и _italic_ | Low |
| 5 | **Gradients** | Красивые hero-секции, фоны | Medium |
| 6 | **Font family control** | Брендинг per-tenant | Low |
| 7 | **Per-corner border radius** | Более сложные формы | Low |
| 8 | **Text alignment** (text-align) | Центрирование текста в блоках | Low |
| 9 | **Background blur** (glassmorphism) | Модные overlay-эффекты | Low |

### Tier 3 — Nice to have

| # | Gap | Что даст |
|---|---|---|
| 10 | Copy operation | Быстрое дублирование секций |
| 11 | Replace operation | Замена без delete+insert |
| 12 | Visual verification (screenshot) | Автоматическая проверка рендера |
| 13 | Blend modes | Фотоэффекты |
| 14 | Mesh gradients | Экзотические фоны |

---

## НАШИ УНИКАЛЬНЫЕ ПРЕИМУЩЕСТВА (чего нет у Pencil)

1. **Data binding** — fieldName → автоматически подставляет данные из каталога
2. **Replication** — 1 шаблон → N виджетов за 0 токенов
3. **Entity references** — каждый виджет привязан к товару/услуге
4. **Navigation** — instant expand (detail), back, history
5. **Actions** — like, cart, compare прямо на виджетах
6. **Wildcard ops** — "измени price во ВСЕХ виджетах" одной операцией
7. **Constraints** — автоматическая нормализация (consistency across widgets)
8. **LLM-driven** — пользователь описывает что хочет текстом, не рисует

---

## РЕКОМЕНДУЕМЫЙ ПУТЬ

**Фаза 1: Убрать ограничения, доказать гибкость**
- Временно ослабить constraints (или сделать их configurable)
- Добавить text-align, padding per-side
- Проверить что агент может собирать разнообразные layout-ы

**Фаза 2: Named ops bundles**
- Registry: product_card, product_detail, comparison_table, hero_banner...
- Agent2 может вызвать по имени ИЛИ написать ops с нуля
- Экономит токены, повышает consistency

**Фаза 3: Design guides в промпт**
- Формализовать: когда grid vs list, spacing rules, hierarchy principles
- Из Pencil: Web App guide (16 principles) адаптировать для наших виджетов

**Фаза 4: Темизация**
- CSS variables per tenant
- Токены на фронте подхватывают tenant palette
- Агент работает с семантическими цветами (primary, secondary, accent)
