# Visual Assembly Engine

Спецификация движка генерации визуала под запрос пользователя.

**Status:** Design phase (обновлено 2026-02-16 после code review)
**Priority:** Critical — центральная часть продукта

---

## 1. Business Context

### Что такое Keepstar

Встраиваемый AI-чат для e-commerce. Пользователь спрашивает — система отвечает не текстом, а интерактивным визуалом: карточки товаров, списки, детализации, сравнения, любые композиции. Один `<script>` тег на сайте клиента.

### Проблема текущей реализации

Текущая система работает через **пресеты** — 7 захардкоженных шаблонов виджетов (product_grid, product_card, product_compact, product_detail, service_card, service_list, service_detail). Agent2 (LLM) выбирает пресет по имени, опционально переопределяет поля.

Это работает, но масштабирование идёт через **добавление новых пресетов**. Это тупиковый путь:
- 100 кейсов = 100 пресетов = неуправляемый зоопарк
- Пресеты не покрывают кастомные запросы ("покажи только фотки и цены крупно с бейджами скидок")
- Нет гибкости внутри пресета — чтобы добавить одно поле, нужно переспецифицировать все
- Mode (grid/list/single) залочен в пресете — нельзя менять layout отдельно
- Поля статичны (10 имён), атрибуты из JSONB недоступны по отдельности

### Что нужно вместо этого

**Движок визуальной сборки (Visual Assembly Engine)** — система из двух агентов в цикле, которая из любого запроса пользователя генерирует визуал. Пресеты не исчезают — они становятся **saved configs** поверх движка.

### Критические требования

- **Время до ответа:** 1-2с на типовой запрос. Детерминированные операции <16ms. Минимум токенов, максимум кода.
- **Качество:** информационное (показано ровно то что нужно) + визуальное (не сломанные карточки).
- **Контекст:** система помнит что было раньше, что на экране, что пользователь уже видел.
- **Стоимость:** 1-5 параметров на типовой запрос от Agent2 (~30-120 output tokens). Бэкенд делает остальное бесплатно.
- **Гибкость:** любой запрос обрабатывается. 90% = простые, гибкость для 10% не ломает скорость.

### Архитектурные принципы

1. **Backend-First.** Фронтенд = тупая рендерилка. Бэкенд отдаёт FormationWithData JSON. Фронт получает и рисует. Подтверждено code review: фронт реально не принимает решений.
2. **Code handles expected, Agent handles unexpected.** Детерминированные задачи = код. Неоднозначные = агент.
3. **Два агента, чёткое разделение.** Agent1 = данные и интерпретация. Agent2 = отображение и сборка. Между ними — микроконтекст.

---

## 2. Полная система: Agent1 + Agent2 в цикле

### Почему это важно

Предыдущая версия спеки рассматривала Agent2 в вакууме — "три измерения" контекста, материалов и операций. Но реальная система — это **цикл** двух агентов, где данные путешествуют от хода к ходу. Нельзя проектировать Agent2 без понимания что делает Agent1 и что между ними.

### Пайплайн (как есть в коде)

```
Пользователь: "покажи кроссовки Nike покрупнее"
         │
         ▼
┌─── Pipeline (pipeline_execute.go) ───────────────────────┐
│                                                           │
│  1. Создаёт TurnID                                       │
│                                                           │
│  2. Agent1.Execute(query)                                 │
│     ├─ LLM читает: system prompt + CatalogDigest (кэш)  │
│     │               + ConversationHistory (кэш, растёт)   │
│     │               + user query                          │
│     ├─ LLM решает: вызвать catalog_search? другой тул?   │
│     ├─ Тул кладёт результат в стейт (zone: data)         │
│     └─ Возвращает: productCount, fields, toolName         │
│                                                           │
│  3. ──── МИКРОКОНТЕКСТ (сейчас примитивный) ────         │
│     Pipeline берёт мету из стейта + дельту текущего хода  │
│     Передаёт Agent2 как часть промпта                     │
│                                                           │
│  4. Agent2.Execute(sessionID, turnID, userQuery)          │
│     ├─ LLM читает: system prompt + tools (кэш)           │
│     │               + мета данных (productCount, fields)  │
│     │               + current_formation (что на экране)   │
│     │               + последние 4 user-сообщения          │
│     │               + user query                          │
│     ├─ LLM решает: какой render-тул вызвать               │
│     ├─ Тул строит FormationWithData (zone: template)      │
│     └─ Возвращает: formation для фронта                   │
│                                                           │
│  5. Pipeline отдаёт фронту:                               │
│     formation + adjacentTemplates + entities               │
│                                                           │
└───────────────────────────────────────────────────────────┘
```

### Что у каждого агента

| | Agent1 (данные) | Agent2 (отображение) |
|---|---|---|
| **Кэш (стабильно)** | Tools + System prompt + CatalogDigest | Tools + System prompt |
| **Кэш (растёт)** | ConversationHistory (user + assistant + tool_use + tool_result) | Ничего — каждый ход с чистого листа |
| **Каждый ход** | User query | Мета данных + current formation + user query + микроконтекст |
| **Помнит историю?** | ДА — полная conversation history | НЕТ — только текущий ход + последние 4 user-сообщения |
| **Тулы сейчас** | catalog_search (1 тул) | render_product_preset, render_service_preset, freestyle (3 тула, freestyle зарезервирован) |
| **Output tokens** | 30-80 (tool call params) | 30-120 (tool call params) |

### Ключевой вывод

**Agent1 помнит всё. Agent2 не помнит ничего.** Это значит:
- Любая задача требующая ПОНИМАНИЯ контекста (что было раньше, что имел в виду пользователь, fuzzy matching) — задача Agent1.
- Agent2 — чистый исполнитель. Он получает данные в стейте + инструкцию и строит визуал.
- Между ними нужен **микроконтекст** — переваренная Agent1 инструкция для Agent2.

---

## 3. Agent1: Данные и интерпретация

### Текущие возможности (из кода)

**1 тул: `catalog_search`** — гибридный поиск (keyword SQL + vector pgvector + RRF merge). Записывает результат в стейт (zone: data). Возвращает LLM-у "ok: found N products".

**ConversationHistory** — Agent1 получает полную историю переписки включая свои прошлые tool calls и результаты. Это в кэше (append-only, prefix match работает).

**CatalogDigest** — ужатый каталог тенанта в кэше (~3K tokens для ToPromptText()). Agent1 знает структуру каталога: категории, бренды, ценовые диапазоны, атрибуты.

### Что нужно добавить

**Тулы Agent1 расширяются** (микро-тулл-трейн первого агента):

| Тул | Что делает | Когда нужен |
|-----|-----------|-------------|
| `catalog_search` | Поиск в каталоге БД | "покажи кроссовки Nike" — новый поиск |
| `state_filter` (NEW) | Фильтрация по данным уже в стейте | "только красные" — данные уже загружены |
| `history_lookup` (NEW) | Поиск в истории сессии | "те кроссовки что были раньше" — нужно найти в прошлых ходах |

**`state_filter`** — не делает новый поиск, а фильтрует то что уже в `state.Current.Data`. Например: "покажи только те у которых рейтинг выше 4" → фильтр по текущим данным. Дёшево, мгновенно, без обращения к БД.

**`history_lookup`** — ищет в дельтах сессии. Дельты уже записывают: какой тул вызван, какие параметры поиска, сколько найдено. Agent1 видит дельты через ConversationHistory (там есть tool_result). Альтернатива: отдельный тул который читает дельты напрямую.

### Fuzzy matching уже работает

Пример: "бренд Pagani" → Agent1 знает из CatalogDigest какие бренды есть. Если Pagani нет, но есть "Paganiradjo de trocco" — vector search по эмбеддингу "Pagani кроссовки" найдёт правильный бренд. Это уже реализованный механизм.

### Микроконтекст: Agent1 → Agent2

**Сейчас** (pipeline_execute.go): Agent2 получает примитивную мету — `productCount`, `fields`, `data_change: {tool: "catalog_search", count: 5}`.

**Нужно**: Agent1 генерирует обогащённый контекст для Agent2:

```
Простой кейс (90%):
  "5 products loaded, new search. User wants: default display."

Модификация (8%):
  "Same 5 products on screen. User wants: show larger with descriptions."

Исторический запрос (2%):
  "1 product from history (turn 3, 'Paganiradjo de trocco' sneakers).
   Already in state. User wants: show in detail, large."
```

Это **20-60 токенов** дополнительного контекста. Agent2 получает чёткую задачу вместо того чтобы гадать.

**Реализация**: Agent1 уже возвращает `Agent1ExecuteResponse` в pipeline. Добавляем поле `ContextForAgent2 string`. Pipeline передаёт его в `BuildAgent2ToolPrompt()`.

---

## 4. Agent2: Отображение и сборка

### Три измерения (из оригинальной спеки, сохранены)

Agent2 оперирует тремя измерениями:

1. **Данные/Контекст** — что Agent2 знает для принятия решений
2. **Материал** — из чего строится визуал (атомы)
3. **Операции** — что можно делать с материалом (tool-train)

### Dimension 1: Данные/Контекст Agent2

#### Что Agent2 получает (целевое)

```
[Кэш BP1] Tools definitions (tool-train)
[Кэш BP2] System prompt + CatalogDisplayMeta (ужатый для отображения)
           + правила constraints + доступные пресеты
[Каждый ход] {
  микроконтекст от Agent1,     // "5 products, new search, default display"
  current_formation,            // что сейчас на экране (RenderConfig)
  history_summary,              // компактная история ходов из дельт
  user_request                  // оригинальный запрос
}
```

#### CatalogDisplayMeta (расширение CatalogDigest для Agent2)

Agent1 использует CatalogDigest для **поиска** (категории, фильтры, кардинальности).
Agent2 нужна **display taxonomy** — какие поля есть и как их показывать:

```
CatalogDisplayMeta:
  entity: product (cosmetics)
  fields:
    core: name, price, brand, category, images, rating    // всегда есть
    tags: skin_type(9), concern(19), product_form(22)     // enum → показывать как tag
    badges: key_ingredients(30)                            // enum → показывать как badge
    detail: description, how_to_use, benefits, volume      // text → только в detail view
    hidden: ingredients, active_ingredients                 // слишком длинные, не показывать
```

Оценка: **~250-400 токенов** в кэше. Строится автоматически из CatalogDigest + классификация полей (structured enum vs freetext).

**GAP в коде**: `catalogExtractProductFields()` (tool_catalog_search.go:503) не извлекает ключи из `attributes` map. Agent2 не знает что skin_type, concern и т.д. существуют. **Фикс: 10 строк.**

#### History Summary (из дельт)

Дельты уже записывают всё что нужно (state_entity.go). Формат для Agent2:

```
turn 1: search → grid 5 products [images,name,price,rating]
turn 2: expand product #3 → detail
turn 3: back → grid restored
turn 4: user "покажи списком" → list + description added
```

~30-50 токенов на ход. 40 ходов = ~1500 токенов. Помещается.

**Реализация**: функция `BuildHistorySummary(deltas []Delta) string` в пакете prompts. Дельты уже загружаются в agent2_execute.go:108.

#### 4 контейнера данных (из оригинальной спеки)

Для сложных запросов Agent2 может обращаться к разным скопам данных:

| Контейнер | Что внутри | Объём | Доступ |
|-----------|-----------|-------|--------|
| **Current screen** | То что отображено сейчас | 6-30 элементов | Через current_formation |
| **Current state** | Все данные в стейте (результат последнего поиска) | 5-30 элементов | Через мету |
| **History** | Что было на предыдущих ходах | Потенциально большой | Через history_summary |
| **Last delta** | Самое последнее изменение | 1-few элементов | Через микроконтекст |

Но в большинстве случаев (90%+) Agent2 работает только с **current state** — то что Agent1 положил в стейт на текущем ходе. Контейнеры нужны для сложных кейсов.

### Dimension 2: Материал (Атомы)

*(Обновлено 2026-02-16: полная модель свойств атома, позиционирование, layers-based виджет)*

#### Иерархия (обновлённая)

```
Atom (неделимый кирпичик информации)
  = данные + display + декорация + позиция
  ↓ атомы размещаются на
Layers (слои с z-index, каждый слой = свой flow)
  ↓ слои собираются в
Widget (контейнер из слоёв)
  ↓ виджеты раскладываются в
Formation (layout на экране: grid, list, carousel, single, composite, canvas)
```

#### Полная модель атома

```
Atom = {
  // ДАННЫЕ
  type: "text" | "number" | "image" | "icon" | "video" | "audio",
  subtype: "string" | "currency" | "rating" | "url" | ...,
  value: any,

  // ОТОБРАЖЕНИЕ (как данные рендерятся)
  display: "badge" | "h1" | "price-lg" | "body" | "image-cover" | ...,

  // ДЕКОРАЦИЯ (визуальная обёртка поверх)
  decoration: {
    color: string,          // цвет фона/текста/обводки
    size: "sm" | "md" | "lg" | "xl",
    shape: "pill" | "rounded" | "square" | "circle",
    emphasis: "primary" | "secondary" | "muted",
  },

  // ПОЗИЦИЯ (где атом живёт)
  position: {
    flow: "auto" | "absolute" | "relative",
    anchor: "parent" | "image" | "viewport" | "atom:<id>",
    x, y,                   // координаты относительно anchor
    z: number,              // слой (0 = базовый, 1+ = оверлеи)
  }
}
```

**Один и тот же атом** `{type: "text", value: "Organic"}`:
- display: "body" + без декора = строчка текста в отзыве
- display: "badge" + shape: pill, color: green, size: sm = бейджик на карточке
- display: "h2" + size: lg = заголовок секции

**Атом = данные. Display = как данные рендерятся. Декор = визуальная обёртка. Позиция = где на холсте.**

#### Три режима позиционирования

```
1. AUTO (90% сейчас)
   flow: "auto", атомы в потоке (vertical/horizontal stack)
   = то что пресеты делают сейчас
   = дёшево, быстро, предсказуемо, не нужны координаты

2. OVERLAY (маркетинг, бейджи)
   flow: "absolute", z > 0
   = бейджи на фото, "Хит!", скидки, акции
   = координаты по правилам (углы, края)

3. FREESTYLE (будущее, canvas/AI-placed)
   anchor: "image", region: AI-detected
   = "поставь текст где на фото есть свободное место"
   = нужен vision model, координаты от AI
```

Формат **один и тот же** — position object. Разница в том кто вычисляет координаты: правила, агент, или vision AI. Пресеты = saved config с flow: auto. Freestyle = тот же формат + absolute + z-layers.

#### 6 типов атомов (подтверждено code review)

| Тип | Подтипы | Примеры display (реализовано на фронте) |
|-----|---------|----------------|
| **text** | string, date, datetime, url, email, phone | h1-h4, body/body-lg/body-sm, caption, badge*, tag* |
| **number** | int, float, currency, percent, rating | price/price-lg/price-old/price-discount, rating*, percent, progress |
| **image** | url, base64 | image, image-cover, avatar*, thumbnail, gallery |
| **icon** | name, emoji, svg | icon/icon-sm/icon-lg |
| **video** | url, embed | video-inline, video-cover (минимальные) |
| **audio** | url | audio-player (минимальные) |

Плюс interactive (button-primary/secondary/outline/ghost, input) и layout (divider, spacer).

**Подтверждено**: фронт рендерит атомы по `display` string через switch/if-else. Добавить новый display = 1 JSX branch + CSS класс. Тривиально.

#### Виджет как слои (layers)

```
Widget = {
  layers: [
    // z:0 — базовый слой, auto-flow (основной контент)
    {
      z: 0,
      flow: "auto-vertical",
      atoms: [image, name, price, rating, description]
    },

    // z:1 — оверлеи (маркетинг, бейджи)
    {
      z: 1,
      atoms: [
        { atom: badge("Хит!"), anchor: "image", x: 8, y: 8 },
        { atom: badge("-30%"), anchor: "image", x: "right-8", y: 8 },
      ]
    },

    // z:2 — будущее: AI-placed контент
    {
      z: 2,
      atoms: [
        { atom: text("описание"), anchor: "image", region: "safe-zone" }
      ]
    }
  ],
  style: { gap, padding, radius, background, shadow, ... }
}
```

**Template mechanism сохраняется**: Agent2 строит/модифицирует ОДИН виджет-шаблон, который применяется ко всем сущностям. Экономия токенов.

**Совместимость**: текущие 4 template компонента (ProductCard, ServiceCard, ProductDetail, ServiceDetail) = виджеты с одним слоем (z:0, flow: auto). Оверлеи (z:1+) = расширение, не замена.

**Пресеты = saved configs**: `product_card` = `{layers: [{z:0, flow: "auto-vertical", atoms: [image, name, price, rating]}], style: {gap: 8, padding: 12, radius: 8}}`. Не отдельный механизм — именованная конфигурация того же формата.

#### Адресация tenant-specific атрибутов

**Проблема**: `attributes` = `map[string]interface{}` — один blob. Agent2 не может адресовать отдельные атрибуты.

**Решение**: CatalogDisplayMeta в кэше Agent2 перечисляет все атрибуты по имени. Бэкенд по имени атрибута достаёт значение из attributes map. Уже почти работает — нужно расширить `catalogExtractProductFields()` и fieldTypeMap.

### Dimension 3: Операции

*(Обновлено 2026-02-16: 12 примитивов в трёх группах + разделение операции vs код)*

#### Ключевое разделение: операции vs код

**Операция** = что меняется (свойство атома/виджета). Задаётся агентом или дефолтом.
**Код** = как это вычисляется (размеры, подгонка, constraints). Работает автоматически.

Пример: `name + icon + price` → нужно собрать виджет.
```
order(name, 0), order(icon, 1), order(price, 2)  — ОПЕРАЦИЯ (порядок)
"догадаться что так лучше"                         — КОД (дефолт по ранжированию)
"прикинуть сколько места займёт"                   — КОД (layout engine)
"понять куда влезает на экране"                     — КОД (знает viewport)
"подогнать размеры если не влезает"                 — КОД (constraints)
```

Агент указывает **намерение**. Код делает **всё остальное**.

#### 12 примитивных операций

##### Группа 1: Информация (что показано)

| # | Примитив | Что меняет | Пример |
|---|----------|-----------|--------|
| 1 | **show**(atom) | visibility → вкл | "добавь описание" |
| 2 | **hide**(atom) | visibility → выкл | "убери рейтинг" |

##### Группа 2: Дизайн (как выглядит)

| # | Примитив | Что меняет | Пример |
|---|----------|-----------|--------|
| 3 | **display**(atom, format) | формат рендеринга данных | "body" → "badge", "price" → "price-lg" |
| 4 | **color**(atom, value) | цвет (фон/текст/обводка) | "бренд красным" |
| 5 | **size**(atom, value) | масштаб: sm/md/lg/xl | "фотку побольше" |
| 6 | **shape**(atom, value) | форма обёртки: pill/rounded/square/circle | "бренд как бейдж-пилюля" |

Примитивы 4-6 можно объединять: `decorate(atom, {color, size, shape})`. Но каждый — независимое свойство.

##### Группа 3: Положение (где находится)

**Атом внутри виджета:**

| # | Примитив | Что меняет | Пример |
|---|----------|-----------|--------|
| 7 | **order**(atom, index) | позиция в последовательности | "цену наверх" = order(price, 0) |
| 8 | **layer**(atom, z) | слой: 0=основной, 1+=оверлей | "бейдж на фото" = layer(badge, 1) |
| 9 | **anchor**(atom, target) | к чему привязан | anchor("parent") vs anchor("image") |

**Виджет / экран:**

| # | Примитив | Что меняет | Пример |
|---|----------|-----------|--------|
| 10 | **direction**(widget, dir) | поток: vertical/horizontal | "в строчку" |
| 11 | **place**(widget, location) | позиция виджета на экране | "этот блок справа" |
| 12 | **layout**(formation, type) | layout формации: grid/list/carousel/single/composite | "покажи списком" |

Будущее: canvas layout (свободное размещение как в Miro).

#### Что делает КОД (не операции)

Всё что НЕ является явным намерением агента/пользователя — вычисляется кодом:

| Задача кода | Что делает | Когда |
|-------------|-----------|-------|
| **Defaults** | Дефолтные значения всех 12 примитивов | Когда агент/пользователь не указал |
| **Field ranking** | Порядок полей по приоритету (name > price > rating > ...) | Для order() по умолчанию |
| **Size calculation** | Расчёт сколько места занимает виджет | После применения операций |
| **Viewport fitting** | Проверка что всё влезает в экран | После расчёта размеров |
| **Constraint enforcement** | Image ≤ viewport, max atoms per widget, конфликты | После всех операций |
| **Auto-adjustment** | Подгонка если не влезает (уменьшить, скрыть, перенести) | При нарушении constraints |

#### Как кейсы пользователя раскладываются на примитивы

| Кейс | Какие примитивы | Сколько параметров |
|------|----------------|-------------------|
| Стандартный запрос ("покажи кроссовки") | **никакие** — код задаёт дефолты | 0 |
| Кастомный порядок ("цену наверх, бренд после") | **order** | 2-3 |
| Модификация дизайна ("крупнее, красным, как бейдж") | **size + color + display** | 3-5 |
| Тяжёлая модификация ("добавь 6 полей, убери одно, поменяй порядок, пошире") | **show + hide + order + size** | 5-10 |
| Несколько виджетов ("2 детально + остальные скроллом") | **layout + place** (каждый виджет = кейс 1-4) | 3-8 |
| Оверлей ("бейдж скидки на фото") | **layer + anchor + display + color + shape** | 4-6 |
| Ультра-кастом | комбинация всех | 10-15 |

90% запросов = 0-5 параметров. Операции **те же 12**, разница только в количестве.

#### Операции над данными (Agent1, не Agent2)

| Операция | Что делает | Пример | Кто выполняет |
|----------|-----------|--------|---------------|
| **filter** | Условный show/hide по значению | "только красные" | Agent1 (state_filter) |
| **sort** | Упорядочить сущности | "по цене от дешёвых" | Agent1 или код |
| **aggregate** | Вычислить новые данные | "средняя цена" | Agent1 |
| **relate** | Связь между сущностями | "сравни первый и третий" | Agent1 + Agent2 |

**Принцип**: операции над **данными** = Agent1. Операции над **отображением** (12 примитивов) = Agent2. Чёткое разделение.

#### Трёхслойная резолюция

```
Layer 1: КОД задаёт ДЕФОЛТЫ
  Все 12 примитивов имеют дефолтные значения.
  6 products, 1200px screen → layout(grid), size(md), order(по ranking), direction(vertical).
  Не нужен агент.

Layer 2: АГЕНТ задаёт ДЕЛЬТЫ
  User: "покажи описание крупнее"
  Agent2: show(description) + size(description, lg)
  1-5 параметров (90% кейсов), до 10-15 для сложных.

Layer 3: КОД применяет CONSTRAINTS
  Image не может превышать viewport.
  Нельзя показать 20 полей в tiny виджете.
  Конфликтующие операции резолвятся детерминированно.
  Если не влезает — auto-adjustment (уменьшить, скрыть, перенести).
```

#### Layer 1 конкретно: Defaults Engine

**Входы:**
- `entity_type` (product / service)
- `entity_count` (сколько сущностей в стейте)
- `screen_width` (размер экрана)

**Таблица 1: field_ranking** — приоритет полей по entity_type (конфиг, per-tenant)

```
field_ranking["product"] = [
  "image",           // 1 — без картинки нет карточки
  "name",            // 2 — что это
  "price",           // 3 — сколько стоит
  "rating",          // 4 — социальное доказательство
  "brand",           // 5 — бренд
  "category",        // 6 — тип продукта
  // ... tenant-specific атрибуты по приоритету ...
  "description",     // N — длинный текст, не для карточки
  "ingredients",     // last — скрыто по умолчанию
]
```

Можно генерировать автоматически из CatalogDigest (по частоте заполнения, длине значений, типу данных).

**Таблица 2: type_map** — atom type → default display

```
text              → "body"
number.currency   → "price"
number.rating     → "rating"
number.percent    → "percent"
image             → "image-cover"
icon              → "icon"
```

**Дефолтные значения всех 12 примитивов:**

```
Примитив       Дефолт                                    Источник
──────────────────────────────────────────────────────────────────
show/hide      top-N(field_ranking, shape.max_fields)     field_ranking + shape
display        type_map[atom.type]                        type_map таблица
color          нет (тема по умолчанию)                    константа
size           md                                         константа
shape          нет (без декоративной обёртки)             константа
order          field_ranking order                        field_ranking таблица
layer          0 (базовый слой)                           константа
anchor         "parent"                                   константа
direction      card→vertical, row→horizontal              shape
place          авто (layout engine)                       layout + entity_count
layout         1→single, 2-6→grid, 7+→list               entity_count
```

**Layout selection:**

```
entity_count = 1            → layout: single, shape: detail
entity_count = 2-6          → layout: grid, shape: card
entity_count = 7+           → layout: list, shape: row (или grid+card(small))
screen_width < 480          → layout: list (всегда, мобильный)
```

**Shape → max_fields (N для show/hide):**

```
detail: N = all (кроме hidden)
card:   N = 4-5
row:    N = 3-4
```

**Итого defaults engine**: 2 таблицы (field_ranking, type_map) + 3 if-а (layout, shape, direction). Всё остальное — константы.

#### Layer 3 конкретно: Constraints

**Жёсткие constraints (нарушение → автокоррекция):**

| Правило | Проверка | Автокоррекция |
|---------|----------|---------------|
| Widget ≤ viewport | widget_height ≤ screen_height | уменьшить size, или hide последних атомов по ranking |
| Atom ≤ widget | atom_size ≤ widget_size | уменьшить size атома |
| Max atoms per shape | card: ≤7, row: ≤5, detail: ∞ | hide по ranking снизу |
| Type compatibility | atom.type → allowed displays | fallback на type_map дефолт |
| Text overflow | text.length > threshold | truncate + toggle "показать ещё" |

**Мягкие constraints (предпочтения, не блокируют):**

| Правило | Что предпочитает |
|---------|-----------------|
| Image first in card | order(image, 0) если не переопределён |
| One primary emphasis | если 2 primary → второй станет secondary |
| Price near name | order: name и price соседствуют |

**5 жёстких + 3 мягких = 8 правил.** Конечный список, не комбинаторный взрыв.

#### Tool-Train: `visual_assembly`

*(Обновлено 2026-02-16: schema определена)*

**Один тул, все параметры опциональные.** Агент указывает только то что отличается от дефолта.

```
visual_assembly(
  // === Режим 1: единый конфиг (90% вызовов) ===
  preset?: string,              // shortcut: "product_card", "product_detail", ...
  show?: string[],              // добавить атомы: ["description", "brand"]
  hide?: string[],              // убрать атомы: ["rating"]
  display?: { atom: format },   // формат: { "brand": "badge", "price": "price-lg" }
  color?: { atom: value },      // цвет: { "brand": "green" }
  size?: { atom: value },       // масштаб: { "image": "xl" }
  shape?: { atom: value },      // форма: { "brand": "pill" }
  order?: string[],             // полный порядок: ["image", "name", "price"]
  layer?: { atom: z },          // слой: { "badge": 1 }
  anchor?: { atom: target },    // привязка: { "badge": "image" }
  direction?: string,           // "vertical" | "horizontal"
  layout?: string,              // "grid" | "list" | "single" | "carousel"

  // === Режим 2: композиция (10% вызовов) ===
  compose?: [                   // массив секций, каждая = мини-конфиг
    {
      ...те же параметры выше,
      count?: number            // сколько сущностей в этой секции
                                // без count = все остальные
    }
  ]
)
```

Если есть `compose` — плоские параметры игнорируются, используется массив секций.

**Примеры вызовов по кейсам:**

```
Стандарт (~5 tokens):
  visual_assembly(preset: "product_card")

Модификация (~20 tokens):
  visual_assembly(preset: "product_card", show: ["description"], size: {"image": "xl"})

Кастом (~50 tokens):
  visual_assembly(
    show: ["image","name","price","brand","skin_type"],
    display: {"brand": "badge"}, color: {"brand": "green"},
    shape: {"brand": "pill"}, size: {"image": "xl"},
    layout: "grid"
  )

Композиция (~30 tokens):
  visual_assembly(compose: [
    { preset: "product_detail", size: "md", count: 2 },
    { preset: "product_card", size: "sm", layout: "grid" }
  ])
```

**Бюджет: 5-50 tokens на вызов.** Вписывается в текущие 30-120, с запасом.

**Пресеты = именованные конфиги.** `"product_card"` = `{show: [image,name,price,rating], layout: grid, direction: vertical, size: md}`. Бэкенд резолвит пресет в набор значений 12 примитивов, агент переопределяет что нужно.

**Реализация**: расширение существующего `tool_freestyle.go`. Текущий input (entity_type, formation, style, overrides, limit) маппится на новый формат. Обратная совместимость с `render_product_preset` / `render_service_preset` через preset parameter.

---

## 5. Микроконтекст: мост между агентами

### Зачем

Agent1 помнит всё, Agent2 — ничего. Микроконтекст — это **переваренная инструкция** от Agent1 для Agent2, чтобы Agent2 не нужно было гадать.

### Примеры

```
Простой поиск:
  User: "покажи кроссовки Nike"
  Agent1: catalog_search → found 5
  Микроконтекст: "new_search: 5 products found"
  Agent2: product_grid (default)

Модификация:
  User: "покрупнее с описанием"
  Agent1: ничего не делает (данные уже в стейте)
  Микроконтекст: "no_data_change. display_request: larger + add description"
  Agent2: resize + show(description)

История:
  User: "те кроссовки с салатовыми шнурками, бренд Pagani"
  Agent1: history_lookup → нашёл в дельте turn 3 → catalog_search(уточнённый) → 1 product
  Микроконтекст: "history_match: 1 product from turn 3. display_request: detail, large"
  Agent2: product_detail, size: large

Фильтрация:
  User: "только красные"
  Agent1: state_filter(color=red) → 2 из 5 products
  Микроконтекст: "filtered: 2 products (from 5). display_request: keep current layout"
  Agent2: оставляет текущий пресет, формация пересобирается с 2 товарами
```

### Формат

Микроконтекст — это строка в промпте Agent2. Не JSON, не структура — просто текст который LLM легко парсит. ~20-60 токенов.

### Реализация

- `Agent1ExecuteResponse` добавляет поле `ContextForAgent2 string`
- Agent1 LLM генерирует эту строку как часть своего ответа (или она формируется кодом из мета)
- `pipeline_execute.go` передаёт её в `Agent2ExecuteRequest`
- `BuildAgent2ToolPrompt()` включает её в промпт

---

## 6. Что уже есть в коде (code review 2026-02-16)

### Бэкенд (Go)

| Компонент | Файл | Статус |
|-----------|------|--------|
| Agent2 execution | `usecases/agent2_execute.go` | Работает. ToolChoice: "any" — всегда вызывает тул. |
| Tool registry | `tools/tool_registry.go` | Работает. Чистый интерфейс ToolExecutor. |
| Render preset tool | `tools/tool_render_preset.go` | Работает. BuildFormation() собирает FormationWithData. |
| Freestyle tool | `tools/tool_freestyle.go` | Существует, зарегистрирован, но зарезервирован ("do not use"). |
| Preset registry | `presets/registry.go` | 7 пресетов. Чистая структура Preset{Name, Fields, DefaultMode, DefaultSize}. |
| State & deltas | `adapters/postgres_state.go` | Работает. Zones: data, meta, template, view. Delta audit log. |
| Formation stack | `state_entity.go` ViewStack | Работает для expand/back. Но хранит только mode+refs, не полную формацию. |
| CatalogDigest | `domain/catalog_digest_entity.go` | Работает для Agent1. ToPromptText() ~3K tokens. |
| Pipeline | `usecases/pipeline_execute.go` | Agent1 → Agent2 последовательно. Мета передаётся примитивно. |
| Atom/Widget/Formation | `domain/*_entity.go` | 6 atom types, 11 slots, ~30 display values, 5 display styles. |
| Navigation | `handlers/handler_navigation.go` | Instant expand/back через adjacent templates + fillFormation. |
| Agent1 CatalogSearch | `tools/tool_catalog_search.go` | Hybrid search: keyword + vector + RRF. Пишет в стейт. |
| BuildTemplateFormation | `tool_render_preset.go:399` | Существует но не используется. Строит формацию без данных (template-only). |

### Фронтенд (React)

| Компонент | Статус |
|-----------|--------|
| Shadow DOM widget shell | Работает. `widget.jsx` + inline CSS. |
| FormationRenderer | Работает. grid/list/carousel/single по mode. |
| WidgetRenderer | Работает. Template-based (ProductCard, ServiceCard, ProductDetail, ServiceDetail). |
| AtomRenderer | Работает. ~40 display values через switch/if-else. |
| fillFormation | Работает. Client-side template fill для instant navigation. 0ms. |
| Theme system | marketplace theme, ~80 CSS custom properties. |

**Вердикт фронта**: добавить новый display = 1 if-branch + CSS. Добавить новый template = ~50 LOC. Для tool-train фронт почти не меняется.

---

## 7. Конкретные GAPs

### Критические (блокируют прогресс)

1. **`catalogExtractProductFields()` не видит attributes** (tool_catalog_search.go:503). Agent2 не знает что skin_type, concern и т.д. существуют. Фикс: 10 строк — итерировать `p.Attributes` и добавлять ключи в fields.

2. **Микроконтекст не существует.** Pipeline передаёт Agent2 примитивную мету (productCount, fields). Нужно: обогащённый контекст от Agent1 с интерпретацией запроса.

3. **Agent1 не умеет искать в истории/стейте.** Только catalog_search. Нужны: state_filter, history_lookup.

4. **History summary для Agent2 не формируется.** Дельты есть, но не сериализуются в промпт Agent2. Нужна: BuildHistorySummary(deltas).

5. **Tool-train не спроектирован.** freestyle тул — зачаток, но нет: дельта-интерфейса, layout-отвязки, constraint solver.

### Важные (улучшат качество)

6. **Дельта-интерфейс для полей.** Сейчас fields override = полная перезапись. Нужно: {add, remove} дельта.

7. **Layout отвязан от пресета.** Сейчас product_grid = всегда grid. Нужно: layout как отдельный параметр.

8. **CatalogDisplayMeta для Agent2.** Другой формат чем CatalogDigest — display taxonomy вместо search taxonomy.

9. **Данные каталога — говнище.** У heybabes: active_ingredients = 2.82MB маркетингового текста вместо списка. volume содержит описания. Нужна нормализация на уровне импорта/обогащения.

10. **Formation не поддерживает дельты.** Каждый тул пересобирает формацию с нуля. Для tool-train нужно: читать текущую формацию → применить дельту → записать обновлённую.

### Обнаружены stress-test (2026-02-16, прогон 40 запросов)

**Закрыты по ходу анализа** (оказались не дырами — система поддерживает):
- ~~Comparison layout~~ → layout: "comparison" в visual_assembly, авто-подсветка = constraint правило
- ~~Table layout~~ → layout: "table", те же атомы, другая раскладка

**Новые GAPs:**

11. **Clarification mechanism (критическая).** Система не умеет переспрашивать при двусмысленности. "Стул для ребёнка" — школьный? для кормления? Agent1 должен уметь вернуть "нужно уточнить" вместо результата. Сейчас pipeline всегда идёт Agent1 → Agent2, нет ветки "переспросить".

12. **Compose spatial arrangement.** compose задаёт секции, но не описывает их взаимное расположение (справа/слева/над/под). "Лидера крупно справа, двух слева" — compose не может выразить горизонтальное расположение секций.

13. **Compose by filter/category.** compose группирует по count (первые N), но иногда нужно по категории: "закуски гридом, напитки списком, десерты каруселью" — каждая секция = фильтр по данным, не позиция в списке.

14. **Conditional styling.** "Выдели цветом где кто лучше" — color на основе сравнения значений между сущностями. Для comparison layout решается авто-правилом. Для произвольных случаев — не поддерживается.

15. **Empty state.** Что показывать когда 0 результатов. Agent2 не вызывается? Или вызывается с special case?

16. **Graceful degradation.** Как отвечать на невыполнимые запросы (2% абсурда). Нужен механизм "не могу так, но могу предложить X".

17. **Pagination.** "Покажи все товары" при 1000 штук. Constraint solver должен ограничить, но нужен механизм "показать ещё" / infinite scroll.

---

## 8. План реализации

### Принцип: каждый шаг = рабочий продукт

### Step 0: Фундамент (не ломает ничего)
- [ ] Фикс extractFields — извлекать attribute ключи из JSONB
- [ ] BuildHistorySummary — сериализация дельт для Agent2
- [ ] CatalogDisplayMeta — display taxonomy формат для Agent2
- [ ] Нормализация данных каталога (на уровне обогащения)

### Step 1: Микроконтекст + тулы Agent1
- [ ] Поле ContextForAgent2 в Agent1ExecuteResponse
- [ ] Pipeline передаёт микроконтекст в Agent2
- [ ] state_filter тул для Agent1
- [ ] history_lookup тул для Agent1 (или расширение catalog_search)
- [ ] Обновление Agent1 промпта — генерировать микроконтекст

### Step 2: Улучшения Agent2 (в рамках пресетов)
- [ ] Дельта-интерфейс для полей: {add, remove} вместо полной перезаписи
- [ ] Layout как отдельный параметр (отвязка от пресета)
- [ ] Активация freestyle тула (расширенный)
- [ ] History summary в промпте Agent2
- [ ] CatalogDisplayMeta в кэше Agent2

### Step 3: Tool-Train (полноценный)
- [ ] Проектирование конкретного JSON schema tool-train
- [ ] Resolution engine: defaults → agent deltas → constraints
- [ ] Constraint solver (жёсткие правила: max atoms, viewport limits)
- [ ] Пресеты как saved configs поверх tool-train
- [ ] Formation diff/patch (дельты вместо полной пересборки)

### Step 4: Freestyle mode
- [ ] Composite layouts (detail + carousel, mixed sizes)
- [ ] Size calculator (display → expected px)
- [ ] Новые shapes: Comparison, Table
- [ ] Расширение constraint solver

### Следующие deep-dives (до начала реализации)

- [ ] **Agent1 deep-dive** — полный разбор: какие данные получает, как обновляет стейт, state_filter/history_lookup конкретно, как генерирует микроконтекст, edge cases
- [ ] **State deep-dive** — полный разбор: какие данные в каждой zone, когда обновляются, кем, жизненный цикл, дельты, что нужно менять для нового движка
- [ ] **Stress-test спеки** — прогнать 20-40 пользовательских запросов через описанную систему, для каждого проверить: получится или нет? Найти что не работает

### Что НЕ меняется

- Каталог, мультитенантность, embeddings, vector search
- Navigation (expand/back, instant nav, fillFormation)
- Widget shell (Shadow DOM)
- Chat, stepper, инфраструктура
- Admin panel
- Фронтенд (минимальные изменения — новые display/template при необходимости)

---

## Appendix A: Три оси поведения пользователя (из рефлексии)

Все действия пользователя раскладываются на три оси:

**Ось 1: Источник данных** — откуда информация
- Поиск в каталоге (Agent1, catalog_search) — ЕСТЬ
- Текущий экран (стейт) — ЕСТЬ
- Ранее просмотренное (история) — НЕТ → history_lookup
- Непродуктовая информация (FAQ, доставка) — НЕТ

**Ось 2: Формат отображения** — как показано (САМОЕ СЛАБОЕ МЕСТО)
- Выбор полей (on/off) — есть но rigid → дельта-интерфейс
- Стиль отображения — есть
- Layout (grid/list/single) — есть но залочен в пресете → отвязка
- Сравнение — НЕТ
- Сортировка/группировка — НЕТ
- Контроль плотности — НЕТ

**Ось 3: Действия** — что пользователь делает
- Навигация expand/back — ЕСТЬ
- Like/save — НЕТ
- Действия с виджетами (корзина, выбор варианта) — НЕТ
- Чат-запрос = комбинация осей

"Сложные" кейсы — это КОМБИНАЦИИ базовых операций по осям. "Покажи те красные сумки что понравились, покрупнее" = Ось 1 (liked) + Ось 2 (product_card, large). Оси конечны, комбинации кажутся бесконечными.

---

## Appendix B: Пять форм виджетов (Shapes)

| Shape | Описание | Статус |
|-------|----------|--------|
| **Card** | Одна сущность = один виджет, визуальный акцент | Есть |
| **Row** | Одна сущность = одна строка, компактный текст | Есть |
| **Detail** | Одна сущность, вся доступная информация | Есть |
| **Comparison** | N сущностей как столбцы, M полей как строки, авто-подсветка лучших значений | layout: "comparison" в visual_assembly |
| **Table** | N сущностей × M полей, обычная таблица | layout: "table" в visual_assembly |

Gallery, "только фотки" = Card/Row с field delta, не новая форма.

---

*Создано: 2026-02-13 (рефлексия)*
*Расширено: 2026-02-14 (Dimension 1-2)*
*Обновлено: 2026-02-16 (code review + полная система Agent1+Agent2 + микроконтекст + конкретные gaps)*
*Обновлено: 2026-02-16 вечер (полная модель атома, layers-based виджет, 12 примитивов, defaults engine, constraints, tool-train schema visual_assembly)*
