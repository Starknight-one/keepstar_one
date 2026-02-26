# Layout Engine Spec (v0 — draft)

## Зачем это нужно

### Проблема

Visual Assembly Engine (backend) доказанно работает: 327,392 комбинации параметров, 12 инвариантов, zero failures. Данные всегда корректны — правильные поля, правильные форматы, правильные constraints.

Но **фронт не умеет это красиво показать**. Сейчас GenericCardTemplate — это 40 строк: "картинка сверху, остальное вертикально вниз". Нет адаптивности, нет колонок внутри карточки, нет fold/collapse, нет responsive. Пользователь видит "всё одинаковое" независимо от того, 3 поля или 30.

### Конкретные проблемы (из тестирования)

1. **Direction "horizontal" незаметен** — в узких grid-колонках карточка с картинкой слева выглядит так же как с картинкой сверху
2. **Нет адаптивных колонок по количеству полей** — 8 полей в medium grid → те же 2-3 колонки, карточки просто растягиваются
3. **Нет перехода grid → list при большом количестве полей** — пользователь просит "покажи все поля" → grid разваливается
4. **Нет fold/collapse** — 20 тегов показываются все, занимая экран
5. **Нет "micro-widgets"** — нельзя показать 50 ингредиентов компактной сеткой мини-тегов

### Решение

Перенести **все layout-решения на бэкенд**. Backend уже знает всё: количество атомов, их типы, размер виджета, layout формации. Он может рассчитать точный layout и отдать фронту готовый "рецепт". Фронт становится тупым рендерером — маппит зоны на CSS-классы, zero логики.

### Что это даёт

1. **Тестируемость** — layout rules фазятся так же как data constraints (327K+ комбинаций)
2. **Предсказуемость** — любая комбинация параметров даёт корректный layout
3. **Сменяемый визуал** — design tokens (10-15 чисел) определяют весь стиль. Сменил tokens = сменил дизайн-систему
4. **Тонкий фронт** — 6 zone-компонентов вместо сложной layout-логики

---

## Архитектура

### Pipeline (полный)

```
Данные (продукты/услуги)
  ↓
AutoResolve v2 (товары × поля → layout + size)
  ↓
Field Configs + Format/Display
  ↓
Data Constraints (текущие: truncate, badges, tags, headings...)
  ↓
Layout Engine (NEW) — зоны, колонки, fold, responsive hints
  ↓
Layout Spec (JSON) — готовый рецепт для фронта
  ↓
Frontend — тупой рендер zone-компонентов
```

### Ключевое изменение: AutoResolve v2

Текущий AutoResolve — одномерный (только по количеству товаров):
```
1 товар → single/large
2-6    → grid/medium
7-12   → list/small
13+    → grid/small
```

Новый — **двумерный** (товары × поля):
```
                    1-3 поля    4-8 полей     9+ полей
1 товар             single      single        single + fold
2-6 товаров         grid        grid 2-col    list детальный
7-12 товаров        grid sm     list          list + fold
13+ товаров         grid tiny   list compact  list compact + fold
```

Пользователь не просил "покажи списком" — он просил "покажи все поля". Движок сам понимает: 15 полей в grid не влезут → переключаю на list.

---

## Layout Spec (output формат)

Backend для каждого виджета генерирует layout spec:

```json
{
  "atoms": [...],
  "layout": {
    "zones": [
      {
        "type": "hero",
        "atomIndices": [0],
        "style": "full-width"
      },
      {
        "type": "row",
        "atomIndices": [1, 2, 3],
        "style": "space-between"
      },
      {
        "type": "flow",
        "atomIndices": [4, 5, 6, 7, 8],
        "columns": 3,
        "maxVisible": 6,
        "foldLabel": "ещё 14"
      },
      {
        "type": "collapsed",
        "atomIndices": [9, 10, 11, ...],
        "trigger": "show-more"
      }
    ],
    "responsive": {
      "breakpoint": 400,
      "fallback": "stack"
    }
  }
}
```

---

## Zone-примитивы (6 штук)

| Zone | Описание | CSS |
|------|----------|-----|
| `hero` | Картинка full-width | `width: 100%; object-fit: cover` |
| `row` | Атомы горизонтально в строку | `display: flex; gap: 8px` |
| `stack` | Атомы вертикально | `display: flex; flex-direction: column; gap: 8px` |
| `flow` | Inline wrap (tags, badges) | `display: flex; flex-wrap: wrap; gap: 6px` |
| `grid` | N колонок | `display: grid; grid-template-columns: repeat(N, 1fr)` |
| `collapsed` | Скрыто + кнопка | `display: none` + toggle |

Responsive: на мобиле (`< breakpoint`) все zones → stack.

---

## Design Tokens (базовые)

```
tag:        height 28px, padding 4px 12px, gap 6px, approx-width ~80px
badge:      height 24px, padding 2px 10px, approx-width ~70px
heading:    h1 28px, h2 22px, h3 18px, h4 15px, line-height 1.3
body:       14px, body-sm 12px
card:       padding 12px, content-gap 8px
zone-gap:   12px
columns:    gap 8px
fold:       max-visible-rows 3 (в flow), max-visible-atoms 9 (в grid)
```

Из этих ~15 чисел калькулятор выводит всё остальное арифметикой:
- `available_width ÷ (tag_width + gap) = tags_per_row`
- `total_tags ÷ tags_per_row = rows → если > max_visible_rows → fold`
- и т.д.

Сменив tokens (размеры, отступы, шрифты) → меняется вся дизайн-система без изменения кода.

---

## Правила зонирования (вход калькулятора)

```
1. image атомы          → zone "hero", full-width
2. h1/h2/h3 атомы       → zone "stack" (по одному в строку)
3. price + rating        → zone "row" (рядом горизонтально)
4. tag/badge атомы       → zone "flow" (inline wrap)
5. body/body-sm/caption  → zone "stack"
6. Если zone.count > fold_threshold → fold, кнопка "показать ещё"
7. Mobile (< breakpoint) → все zones в stack
```

7 правил. Не килотонны — семь. Всё остальное — арифметика из tokens.

---

## Переопределения пользователя

Пока не определены. Будут добавлены когда появятся реальные кейсы из чата. Возможные:
- "покажи в две колонки" → force columns=2
- "разверни всё" → disable fold
- "компактнее" → switch tokens to compact set

---

## Реализация (план)

1. Зафиксировать design tokens (базовые значения)
2. Layout calculator на бэкенде (`tools/layout_engine.go`)
   - Вход: atoms[], widget size, formation mode, tokens
   - Выход: layout spec (zones[])
3. Fuzz-тест layout calculator (аналог текущего — 300K+ комбинаций)
4. 6 zone-компонентов на фронте
5. Интеграция: widget JSON включает layout spec
6. Визуальная проверка в тестбенче

---

## Аналоги

- **Microsoft Adaptive Cards** — JSON schema → рендер на любой платформе. Близко по концепции, но не имеет layout calculator (человек сам пишет schema)
- **Slack Block Kit** — JSON blocks, но фиксированный layout
- **Flutter/SwiftUI** — Row/Column/Grid/Wrap/Stack примитивы — наша модель зон

Наш подход отличается тем, что **калькулятор сам выбирает примитивы** на основе данных. Человек не пишет layout — движок вычисляет его из количества атомов, их типов и доступного пространства.
