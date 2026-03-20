# E2E Test Cases — V2 Engine

Последовательные запросы в одной сессии (как обычный пользователь).

## Как тестировать

1. Открыть виджет на сайте (новая сессия)
2. Отправлять запросы по порядку
3. После каждого запроса фиксировать: визуальный результат + trace

## Как проверять traces

```bash
# Найти сессию
psql $DATABASE_URL -c "
  SELECT session_id, count(*), max(timestamp)::text
  FROM pipeline_traces GROUP BY session_id
  ORDER BY max(timestamp) DESC LIMIT 5;"

# Посмотреть что Agent2 послал и что движок решил
psql $DATABASE_URL -c "
  SELECT row_number() over (order by timestamp) as n, query,
    trace_data->'agent2'->>'toolInput' as tool_input,
    trace_data->'agent2'->'toolBreakdown'->>'size' as size,
    trace_data->'agent2'->'toolBreakdown'->>'layout' as layout,
    trace_data->'agent2'->'toolBreakdown'->>'preset' as preset,
    trace_data->'formationResult'->>'widgetCount' as widgets, error
  FROM pipeline_traces WHERE session_id = '<SID>' ORDER BY timestamp;"

# Посмотреть реальные поля в карточках
psql $DATABASE_URL -c "
  SELECT query,
    (SELECT string_agg(a->>'fieldName', ', ')
     FROM jsonb_array_elements(trace_data->'formationResult'->'fullJson'->'widgets'->0->'atoms') t(a)
    ) as fields
  FROM pipeline_traces WHERE session_id = '<SID>' ORDER BY timestamp;"
```

---

## Блок 1: Базовый поиск + авторезолв

| # | Запрос | Ожидание | Проверка в trace |
|---|--------|----------|------------------|
| 1 | "Привет, покажи кремы для лица" | grid, small cards, 3 поля (image, name, price), ~23 товара | layout=grid, size=small, widgets=23 |
| 2 | "Покажи их списком" | list layout, горизонтальные карточки (image 120px слева, контент справа) | layout=list |
| 3 | "Покажи детально первый товар" | single, large card, 9-13 полей (description, ingredients, rating...) | layout=single, size=large, widgets=1 |

## Блок 2: Show / Hide (аддитивная логика)

**Контекст**: после блока 1, мы на detail view. Запрос множественный → агент должен вернуться в grid.

| # | Запрос | Ожидание | Проверка в trace |
|---|--------|----------|------------------|
| 4 | "Покажи только название и цены" | grid, ТОЛЬКО name+price, БЕЗ images | hide=[всё кроме name,price], fields=name,price |
| 5 | "Добавь рейтинг" | name+price+rating (images НЕ должны появляться) | show=[rating], fields=name,price,rating |
| 6 | "Убери цены" | name+rating (НЕ images+name) | hide=[price], fields=name,rating |
| 7 | "Покажи всё как было в самом начале" | дефолтный набор: image, name, price | toolInput={}, fields=images,name,price |

## Блок 3: Order

| # | Запрос | Ожидание | Проверка в trace |
|---|--------|----------|------------------|
| 8 | "Покажи цену первой, потом название" | Цена отображается ВЫШЕ названия в карточке | order=[price,name], визуально price сверху |

## Блок 4: Size / Layout переключения

| # | Запрос | Ожидание | Проверка в trace |
|---|--------|----------|------------------|
| 9 | "Покажи крупными карточками" | size=large, карточки крупнее (2 или 1 в ряд, не 3) | size=large, columns меняется |
| 10 | "Покажи каруселью" | carousel layout, карточки листаются горизонтально, image НЕ на весь экран | layout=carousel |
| 11 | "Покажи таблицей" | table layout | layout=table |
| 12 | "Покажи снова сеткой, маленькими" | grid, size=small, 3 в ряд | layout=grid, size=small |
| 13 | "Покажи горизонтальными карточками" | direction=horizontal: image слева, текст справа | direction=horizontal |

## Блок 5: Фильтрация + бренд

| # | Запрос | Ожидание | Проверка в trace |
|---|--------|----------|------------------|
| 14 | "Покажи только COSRX" | state_filter → только COSRX товары (3-5 штук), НЕ 23 | a1_tool=state_filter, widgets=3-5 |
| 15 | "Покажи их с составом и типом кожи" | текущие COSRX товары + поля keyIngredients, skinType | show=[keyIngredients,skinType] |
| 16 | "Покажи все кремы The Ordinary" | НОВЫЙ catalog_search, другие товары | a1_tool=catalog_search, widgets=новые |

## Блок 6: Сложные комбинации

| # | Запрос | Ожидание | Проверка в trace |
|---|--------|----------|------------------|
| 17 | "Покажи топ-5 с рейтингом, брендом и описанием, крупно" | limit=5, 6 полей, size=large | limit=5, size=large, show=[...], widgets=5 |
| 18 | "Сравни первые 3" | comparison layout | layout=comparison |
| 19 | "Сравни два самых дешёвых — только цена, бренд и состав" | comparison, limit=2, 3 поля | layout=comparison, limit=2 |

## Блок 7: Новый поиск + drill-down

| # | Запрос | Ожидание | Проверка в trace |
|---|--------|----------|------------------|
| 20 | "Покажи все увлажняющие средства" | catalog_search, grid, новые результаты | a1_tool=catalog_search |
| 21 | "Покажи первые 3 крупными карточками с описанием" | limit=3, size=large, show=[description] | limit=3, size=large |
| 22 | "Покажи детально вторую" | expand, single large, 10+ полей | layout=single, widgets=1 |

## Блок 8: V2-специфичные фичи

| # | Запрос | Ожидание | Проверка в trace |
|---|--------|----------|------------------|
| 23 | "Покажи сеткой по 2 в ряд" | columns=2 | toolInput.columns=2 |
| 24 | "Покажи с тенями и скруглёнными карточками" | widgetShadow + widgetBorderRadius | toolInput содержит эти поля |
| 25 | "Покажи топ-3 сыворотки" | новый поиск, grid | a1_tool=catalog_search |

---

## Глобальные проверки (каждый trace)

- [ ] `engineVersion: "v2"` (не v1)
- [ ] `fieldDefCount: 13+` (field definitions загружаются)
- [ ] `error` пустой (нет 500)
- [ ] V2 atoms: `atomsV2` + `layout` присутствуют в fullJson виджетов

---

## Результаты тестирования

### Run 1 — 2026-03-20, session f618e070

| # | Статус | Проблема |
|---|--------|----------|
| 1 | OK | grid, small, 23 товара |
| 2 | VISUAL BUG | list layout работает, но карточки слишком широкие |
| 3 | OK | single, large, detail view |
| 4 | OK (спорно) | grid name+price, вопрос: должен ли detail ужаться? |
| 5 | BUG | Agent послал `show:[rating]` → fields стали `rating,images,name` (price пропал, images вернулись) |
| 6 | BUG | Agent послал `hide:[price]` → fields `images,name` (rating пропал — его уже не было после #5) |
| 7 | OK | Сброс к дефолтам |
| 8 | BUG | Agent послал правильный order, но визуально ничего не изменилось. Engine/frontend не применяет order |
| 9 | BUG | size=large применился (preset=product_card_detail), но grid остался 3 колонки. Нет авто-уменьшения columns при large |
| 10 | VISUAL BUG | carousel работает, но image на весь экран — нужен max-height |
| 11 | OK | table |
| 12 | OK | grid, small |
| 13 | BUG | direction=horizontal послан, но визуально ничего не изменилось. Frontend не обрабатывает direction |
| 14 | BUG | Agent1 сделал state_filter, но Agent2 показывает 23 виджета. Фильтр не применился к формации |
| 15 | PARTIAL | state_filter сработал (3 виджета), show добавил поля. Но это #14 наконец отработал |
| 16 | BUG | catalog_search вызван, но Agent2 послал `{}` и показывает 3 виджета от прошлого фильтра |

### Категории багов

**P0 — Show/Hide сломан (аддитивная логика):**
- `show:[rating]` не добавляет к текущим, а ЗАМЕНЯЕТ текущие поля. Images возвращаются, price теряется.
- Причина: движок не знает "текущие" поля. Show additive только к дефолтам пресета.

**P1 — state_filter не влияет на formation:**
- Agent1 фильтрует state.data, но Agent2 всё равно рендерит все 23 виджета.
- Причина: Agent2 берёт entityCount из meta, а meta не обновляется после state_filter.

**P2 — Order не работает визуально:**
- Engine получает order, но frontend не переставляет атомы.

**P3 — Direction horizontal не работает:**
- Frontend не обрабатывает direction на layout tree / карточках.

**P4 — Size large не уменьшает grid columns:**
- size=large + grid = по-прежнему 3 колонки. Нужно: large → 1-2 cols.

**P5 — Carousel image на весь экран:**
- Нет max-height на hero image в carousel mode.
