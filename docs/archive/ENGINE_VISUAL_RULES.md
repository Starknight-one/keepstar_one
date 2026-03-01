# Visual Assembly Engine — Visual Rules & Constraints

**Date:** 2026-02-22
**Status:** Draft — будет расширяться
**Purpose:** Конечный список детерминированных правил, которые движок применяет КОДОМ, чтобы фронт всегда выглядел красиво независимо от того, что скажет LLM.

---

## Философия

```
LLM выражает НАМЕРЕНИЕ (1-5 параметров)
КОД гарантирует КАЧЕСТВО (30+ правил)
```

Движок = аналог design system, но на бэкенде. В обычном фронтенде design system гарантирует что любая комбинация компонентов выглядит приемлемо. Здесь вместо разработчика — LLM, и движок должен гарантировать что **любая комбинация параметров** производит красивый результат.

Три слоя (из спеки):
1. **Defaults** — код задаёт разумные дефолты (уже есть: AutoResolve, fieldRanking)
2. **Deltas** — LLM задаёт изменения (уже есть: visual_assembly tool)
3. **Constraints** — код валидирует и корректирует результат ← **ЭТО ЗДЕСЬ**

Сейчас слой 3 = 2 правила (MaxAtomsPerSize, ValidateDisplay). Нужно ~30+.

---

## Уровень 0: Данные → Атомы (sanitize)

Правила очистки ДО того, как данные станут атомами. `buildAtoms` пропускает nil, но не чистит мусор.

### D1. Пустые строки → nil для ВСЕХ текстовых полей

**Сейчас:** `nonEmpty()` применяется к name/brand/description, но не ко всем.
**Правило:** Любое текстовое поле с `value == ""` → nil → атом не создаётся.
**Код:** Обернуть ВСЕ текстовые возвраты в `nonEmpty()`.

### D2. Rating 0 → nil

**Статус:** Уже работает. `if p.Rating == 0 { return nil }`.

### D3. Price 0 → nil

**Сейчас:** `p.Price` возвращается как есть, даже если 0.
**Правило:** `if p.Price == 0 { return nil }` — не показывать `$0.00`.
**Где:** `productFieldGetter`, case "price".

### D4. Массивы → разные стратегии по display

**Сейчас:** skinType/concern/keyIngredients делают `strings.Join(arr, ", ")` в одну строку.
**Правило:**
- Если display = `tag` → создать **отдельный атом на каждый элемент** массива (до лимита W2)
- Если display = `body`/`body-sm` → join через `", "`
- Если display = `badge` → первый элемент, остальные в tag
**Где:** `buildAtoms`, после определения display.

### D5. Длинный текст → truncate до рендеринга

**Сейчас:** Описания до 2363 символов идут как есть.
**Правило:**
- slot = `description` в layout `single`/`detail` → без лимита
- slot = `description` в layout `grid`/`list` → max 120 символов + `"…"`
- slot = `secondary` → max 120 символов + `"…"`
- slot = `primary`/`badge`/`tag` → max 80 символов + `"…"`
- slot = `title` → max 100 символов + `"…"`
**Где:** `buildAtoms`, после создания атома, перед упаковкой в виджет.

### D6. Длинное имя → автоматический downgrade display

**Сейчас:** h1 с 112 символами выглядит ужасно (3-4 строки огромного текста).
**Правило:**
- `display == "h1" && len(name) > 60` → `display = "h2"`
- `display == "h2" && len(name) > 80` → `display = "h3"`
- `display == "h3" && len(name) > 100` → `display = "body-lg"`
**Где:** constraint solver, после применения display overrides.

### D7. Невалидный image URL → nil

**Сейчас:** Нет проверки — битый URL создаёт атом, фронт показывает broken image icon.
**Правило:** `if !strings.HasPrefix(url, "http") { return nil }`
**Где:** `productFieldGetter`, case "images".

---

## Уровень 1: Атом (per-atom visual quality)

Правила применяются к каждому атому после создания, до упаковки в виджет.

### A1. Badge max length → downgrade

**Правило:** Badge текст > 20 символов → `display = "tag"`.
**Почему:** Badge — это акцентный элемент, длинный текст в badge выглядит как мусор.

### A2. Tag max length → downgrade

**Правило:** Tag текст > 40 символов → `display = "body-sm"`.
**Почему:** Tag = короткая метка. Длинный текст в tag расширяет карточку.

### A3. Color contrast (auto text color)

**Правило:** Если atom имеет background color → текст автоматически контрастный.
```
Dark backgrounds (red, blue, purple, green) → white text
Light backgrounds (orange, gray, yellow)    → dark text
Hex → вычислять luminance, порог 0.5
```
**Где:** AtomRenderer на фронте, или meta.textColor на бэкенде.

### A4. Badge text → capitalize

**Правило:** Первая буква badge текста — заглавная.
**Пример:** "крем" → "Крем" в badge.
**Где:** buildAtoms или AtomRenderer.

### A5. Rating display по значению

**Правило:**
- Rating < 3.0 → `display = "rating-compact"` (только число, не звёзды)
- Rating >= 3.0 → display как указано (звёзды или compact)
**Почему:** Низкий рейтинг со звёздами (★★☆☆☆) — визуально шумно и негативно.

### A6. Null/undefined value → skip на фронте

**Сейчас:** AtomRenderer рендерит `String(null)` = буквальный текст `"null"`.
**Правило:** `if (!atom.value && atom.value !== 0) return null`
**Где:** AtomRenderer.jsx, первая строка render.

### A7. Fallback display для unknown

**Сейчас:** Неизвестный display → `<span>{String(atom.value)}</span>` без стилей.
**Правило:** `default: return <span className="atom-body">{String(atom.value)}</span>` — хотя бы стилизованный fallback.

---

## Уровень 2: Виджет (per-card composition)

Правила для одного виджета — как атомы компонуются внутри карточки.

### W1. Max 2 badge на виджет

**Правило:** Третий+ badge → `display = "tag"`.
**Почему:** Badge = акцент. Больше двух = визуальный шум, всё перестаёт выделяться.

### W2. Max 5 tag на виджет

**Правило:** Шестой+ tag → скрыть. Или показать 3 + `"+N ещё"`.
**Почему:** Стена тегов нечитаема.

### W3. No image → убрать media zone

**Сейчас:** nil-skip работает, но CSS media zone может рендериться пустой.
**Правило:** `if imageAtoms.length === 0` → не рендерить `.generic-card-media` вообще.
**Где:** GenericCardTemplate.jsx (частично уже есть: `images.length > 0 &&`).

### W4. Один h1/h2 на виджет

**Правило:** Если два text-поля с display h2 → второе автоматически h3.
**Почему:** Иерархия заголовков. Два h2 = нет иерархии = визуальный хаос.
**Где:** constraint solver, после BuildFieldConfigs.

### W5. Price рядом с name (мягкое)

**Правило:** Если order не переопределён пользователем, price идёт сразу после name.
**Реализация:** В fieldRanking price уже на позиции 3 (после image, name). OK если не сломано.

### W6. Image всегда первый (мягкое)

**Правило:** Если order не переопределён, image slot = index 0.
**Статус:** Работает через fieldRanking. Но если LLM задаст order без image — image должен остаться первым.

### W7. Horizontal mode + много атомов → запретить

**Правило:** `if direction == "horizontal" && atomCount > 4 { direction = "vertical" }`
**Почему:** Горизонтальный режим = image слева, контент справа. С >4 атомами контент не влезает.

### W8. Tiny size → no image

**Правило:** `if size == "tiny" { remove image atoms }`
**Почему:** Tiny = 100px высоты. Image media zone + контент не влезут. Только текст.

### W9. Consistent atom presence в grid

**Правило:** Все виджеты в одной formation показывают одинаковый набор полей.
**Реализация:** Два варианта:
1. Если поле отсутствует у одного товара → показать `"—"` placeholder
2. Если поле отсутствует у >30% товаров → убрать это поле из ВСЕХ карточек
**Связано с:** C1.

---

## Уровень 3: Formation (layout quality)

Правила для формации — как виджеты раскладываются на экране.

### F1. Grid: все карточки одной высоты

**Сейчас:** CSS grid stretch работает, но внутри карточки контент не прижимается.
**Правило:** `align-items: stretch` на grid + внутри карточки `flex: 1` на content zone.
**Где:** GenericCardTemplate.css + FormationRenderer CSS.

### F2. Grid column auto-select

**Сейчас:** Всегда 2 колонки (`visual_assembly` не ставит `grid.cols`).
**Правило:**
- `count == 1` → single (не grid)
- `count == 2` → cols=2
- `count == 3` → cols=3
- `count 4-8` → cols=2
- `count 9-12` → cols=3
- `count 13+` → cols=auto-fill
**Где:** `tool_visual_assembly.go`, при создании formation. Ставить `formation.Grid.Cols`.

### F3. Single item → detail автоматически

**Сейчас:** AutoResolve ставит layout=single, size=large, но template не переключается на detail.
**Правило:** 1 entity → GenericCard с large size + все доступные поля. Или переключение на ProductDetail template.
**Где:** Уже частично в AutoResolve. Доработать template selection.

### F4. Comparison max 4

**Статус:** Уже реализовано. OK.

### F5. Carousel: peek next card

**Сейчас:** `width: 100%` — carousel выглядит как список, не видно следующую карточку.
**Правило:** `carousel > * { width: 85%; flex-shrink: 0 }` + `scroll-snap-type: x mandatory`.
**Где:** Formation CSS.

### F6. List: alternating background

**Правило:** `nth-child(even) { background: var(--surface-secondary, #fafafa) }`.
**Почему:** Помогает визуально разделить элементы в длинных списках.
**Где:** Formation CSS.

### F7. Empty formation → сообщение

**Сейчас:** `if (!formation) return null` — пустота.
**Правило:** Показывать "Ничего не найдено" с нейтральной иллюстрацией.
**Где:** FormationRenderer.jsx.

### F8. Grid overflow protection

**Правило:** Если ширина контейнера < 300px → переключить grid на list.
**Почему:** 2 колонки по 150px = нечитаемые карточки.
**Где:** CSS `@media` или JS container query.

---

## Уровень 4: Cross-widget consistency

Правила согласованности между виджетами в одной формации.

### C1. Одинаковый набор полей

**Правило:** Все виджеты в grid/list показывают одни и те же поля.
**Реализация:**
1. Собрать union всех полей по всем entities
2. Для каждого поля: если заполнено у <70% entities → убрать из всех
3. Если заполнено у >=70% → показать у всех, у отсутствующих → placeholder `"—"`
**Где:** `tool_visual_assembly.go`, после buildAtoms, перед упаковкой в formation.

### C2. Одинаковый display per field

**Правило:** Если brand=badge у первого виджета, brand=badge у всех.
**Почему:** grid с brand как badge + brand как tag в соседних карточках = разнобой.
**Статус:** Уже работает (FieldConfigs применяются ко всем виджетам одинаково). Но нужно гарантировать что D4 (массивы) не создаёт разное количество атомов.

### C3. Price format consistency

**Правило:** Все цены в одной formation → одинаковый format и currency.
**Статус:** Уже работает через единый display. Но если одна цена 60₽, другая 5990₽ — визуальный дисбаланс. Возможное решение: min-width на price atom.
**Где:** CSS `.atom-price { min-width: 4em; text-align: right }`.

### C4. Image aspect ratio consistency

**Сейчас:** Нет `object-fit: cover` на carousel-image. Нет стиля для `.carousel-image` вообще.
**Правило:** Все images в grid → одинаковый aspect-ratio контейнера + `object-fit: cover`.
**Где:** GenericCardTemplate.css: `.carousel-image { width: 100%; height: 100%; object-fit: cover; }`.

### C5. Name truncation consistency

**Правило:** Все names в grid → одинаковое max-lines.
- size=medium → 2 строки
- size=small → 1 строка
- size=tiny → 1 строка
**Реализация:** CSS `-webkit-line-clamp` + `overflow: hidden` + `text-overflow: ellipsis`.
**Где:** AtomRenderer CSS или GenericCardTemplate CSS по display.

---

## Приоритизация

### Tier 1 — "поломано" → "работает"

| # | Правило | Критичность |
|---|---------|-------------|
| A6 | null value guard на фронте | Рендерит `"null"` буквально |
| D3 | price=0 → nil | Показывает `$0.00` |
| C4 | `object-fit: cover` на carousel-image | Нет стиля вообще, image не масштабируется |
| F5 | carousel width 85% | Carousel = сломанный список |
| W3 | пустая media zone CSS | Может рендерить пустой блок |

### Tier 2 — "работает" → "профессионально"

| # | Правило | Эффект |
|---|---------|--------|
| C1 | одинаковый набор полей в grid | Самое заметное улучшение в grid |
| C5 | line-clamp на name | Длинные имена ломают выравнивание |
| D5/D6 | truncate текстов / downgrade display | Переполнение карточек |
| F1 | одинаковая высота карточек | Рваный grid |
| F2 | авто-подбор колонок | Всегда 2 колонки, даже для 1 или 3 товаров |
| W9 | consistency в grid | Связано с C1 |

### Tier 3 — "профессионально" → "вау"

| # | Правило | Эффект |
|---|---------|--------|
| A1/A2 | badge/tag length limits | Чистота |
| A3 | color contrast | Читабельность |
| W1/W2 | max badges/tags | Визуальный порядок |
| D4 | массивы → отдельные tag-атомы | Правильные чипсы вместо строки |
| F6 | alternating backgrounds для list | Читабельность длинных списков |
| A4 | capitalize badge text | Полировка |
| A5 | rating display по значению | Субъективная чистота |
| W7 | horizontal mode atom limit | Предотвращение поломки |
| W8 | tiny → no image | Предотвращение поломки |

---

## Где реализовывать

```
Backend (Go) — constraint solver:
  tool_visual_assembly.go  → D1-D7, A1-A5, W1-W9, C1-C2, F2-F3
  defaults_engine.go       → расширить ValidateDisplay, добавить новые проверки

Frontend (React/CSS) — visual guards:
  AtomRenderer.jsx         → A6, A7, A3 (color contrast)
  GenericCardTemplate.jsx  → W3
  GenericCardTemplate.css  → C4, C5, F1, размеры
  FormationRenderer CSS    → F5, F6, F8
  FormationRenderer.jsx    → F7
```

Принцип: **максимум на бэкенде** (бэкенд отдаёт уже правильные данные), **CSS как safety net** (truncation, overflow, object-fit — на случай если данные всё-таки длинные).

---

*Создано: 2026-02-22*
*Статус: первая версия, будет расширяться*
