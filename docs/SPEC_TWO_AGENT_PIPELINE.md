# Two-Agent Pipeline

Микроспека двухагентной системы для chat.

## Концепция

```
User Query
    │
    ▼
┌─────────────────┐
│  Agent 1        │  Tool Caller
│  (Query→Tools)  │  Чистый оркестратор
└────────┬────────┘
         │
         ├──→ tool_call ──→ [Tool] ──→ State (пишет delta)
         │                     │
         │←── "ok" / "empty" ──┘
         │
         ▼
    trigger event
         │
         ▼
┌─────────────────┐
│  Agent 2        │  Template Builder
│  (Meta→Template)│  Получает meta, не сырые данные
└────────┬────────┘
         │
         ├──→ template ──→ State (пишет delta)
         │
         ▼
    Backend рендерит template + data → Frontend
```

## Ключевые принципы

1. **State — центр всего**. Агенты — просто tool callers, оркестраторы.
2. **Агенты не смотрят в State напрямую**. Получают данные через tool calls или принудительные триггеры.
3. **Дельты, не снапшоты**. Каждое действие = дельта. Можно откатить на любой шаг.
4. **Agent 2 не видит сырые данные**. Получает meta: count, fields, aliases. Экономия токенов.

## Agent 1: Tool Caller

**Задача**: из свободного текста понять какие tools вызвать. Супер быстро.

**Ключевое**: агент НЕ видит данные из tools. Tools пишут в State. Агент получает только "ok" / "empty".

### Input/Output

- **Input**: user query (свободная форма)
- **Output**: tool calls
- **Tools**: search_products, filter, sort, compare, set_layout...
- **Retry**: до 3 попыток при фейле (validation hooks)

### Что агент знает

Агент может вызвать tool чтобы узнать meta текущего state:
- Какие данные есть (count, fields)
- История предыдущих tool calls
- НЕ сами данные

### Экономия токенов

- Минимальный system prompt
- Никаких рассуждений, только tool calls
- Не читает результаты tools (только "ok"/"empty")
- Anthropic prompt caching для system prompt + tools definitions

## Agent 2: Template Builder

**Задача**: создать template виджета для frontend.

### Input (через триггер)

- Meta из State:
  - `count`: сколько элементов
  - `fields`: какие поля есть (aliases)
  - `layout_hint`: подсказка по layout
- Viewport info:
  - Размер экрана
  - Какой участок свободен

### Output

- Widget template (JSON)
- Template идёт в State, затем backend заполняет данными

### Не видит

- Сырые данные (products, prices, etc.)
- Только структуру и количества

## State

### Структура

State = история дельт + материализованное текущее состояние.

```
Session State
├── current/                 # Материализованный текущий state
│   ├── data/                # Сырые данные (products, etc.)
│   ├── meta/                # Метаданные для Agent 2
│   │   ├── count: int
│   │   ├── fields: []string
│   │   └── aliases: {}
│   └── template/            # Текущий template от Agent 2
│
└── deltas/                  # История изменений
    └── [{step, trigger, action, result, template?}, ...]
```

### Delta формат

```
Delta {
  step:        int           // порядковый номер
  trigger:     enum          // USER_QUERY | WIDGET_ACTION | SYSTEM

  // что произошло
  action: {
    type:      enum          // SEARCH | FILTER | SORT | LAYOUT | ROLLBACK
    tool:      string?       // какой tool вызван
    params:    {}            // параметры
  }

  // результат (meta, не сырые данные)
  result: {
    count:     int
    fields:    []string      // макс 30 полей пока
    aliases:   {}
  }

  // что Agent 2 сгенерил
  template:    {}?
}
```

### Текущий State

**Определение**: текущий state = состояние после завершения Agent 2.

При следующем запросе:
1. User query → Agent 1 → tools → дельта
2. Trigger → Agent 2 → template → дельта
3. State обновлён

### Интерактивность между запросами

Виджеты могут быть интерактивными. Пользователь кликает/фильтрует:
- Это генерирует дельту с `trigger: WIDGET_ACTION`
- Агенты не участвуют
- State обновляется напрямую

### Откаты

Любой шаг можно откатить:
```
State(step=N) = apply(Delta[0], Delta[1], ..., Delta[N])
```

Для скорости — чекпоинты каждые X шагов:
```
State(step=N) = checkpoint[K] + apply(Delta[K+1], ..., Delta[N])
```

## Хранение (PostgreSQL)

| Таблица | Что хранит |
|---------|------------|
| `chat_session_state` | Текущий материализованный state (JSONB) |
| `chat_session_deltas` | История дельт |
| `chat_session_data` | Сырые данные (products), state ссылается по ID |

## Кэширование (Anthropic)

- System prompt + tools definitions кэшируются
- История сообщений (дельты в компактном формате) — в prefix
- TTL 5 мин (продлевается), extended 1 час
- Порог ~2048 токенов для активации

## UI Composition: Atoms → Widgets → Formations

### Иерархия

```
Formation (экран)
├── Widget 1 (карточка продукта)
│   ├── Atom: Image (фото)
│   ├── Atom: Text (название)
│   ├── Atom: Number (цена)
│   └── Atom: Rating (звёзды)
│
├── Widget 2 (карточка продукта)
│   └── ... те же атомы, другие данные
│
└── Widget 3 (метрика)
    ├── Atom: Text (label)
    └── Atom: Number (value)
```

### Atoms (неделимые)

Базовые UI-примитивы. Фронт знает как их рендерить.

| Atom | Описание | Параметры |
|------|----------|-----------|
| `Text` | Текст | value, style (heading/body/caption) |
| `Number` | Число | value, format (currency/percent/compact) |
| `Image` | Картинка | url, alt, size |
| `Icon` | Иконка/эмодзи | name |
| `Badge` | Статус | value, variant (success/warning/danger) |
| `Rating` | Звёзды | value (0-5) |
| `Button` | Кнопка | label, action |
| `Progress` | Прогресс-бар | value (0-100) |
| `Divider` | Разделитель | — |

### Widgets (составные)

Контейнер из атомов. Имеет размер и приоритет.

```
Widget {
  id:       string
  size:     enum        // tiny | small | medium | large
  priority: int         // для сортировки на экране
  atoms:    []Atom
}
```

**Size constraints** (строгие лимиты):
| Size | Ширина | Max atoms |
|------|--------|-----------|
| tiny | 80-110px | 2 |
| small | 160-220px | 3 |
| medium | 280-350px | 5 |
| large | 384-460px | 10 |

### Formations (экраны)

Набор виджетов + layout config.

```
Formation {
  mode:     enum        // grid | carousel | single | comparison
  grid:     {rows, cols}
  widgets:  []Widget
}
```

**Режимы:**
- `grid` — сетка N×M (карточки продуктов)
- `carousel` — горизонтальный скролл
- `single` — один продукт детально
- `comparison` — таблица сравнения

### Template vs Data

**Agent 2 создаёт template** — структуру без данных:

```json
{
  "mode": "grid",
  "grid": {"rows": 2, "cols": 3},
  "widgetTemplate": {
    "size": "medium",
    "atoms": [
      {"type": "Image", "field": "image_url"},
      {"type": "Text", "field": "name", "style": "heading"},
      {"type": "Number", "field": "price", "format": "currency"},
      {"type": "Rating", "field": "rating"}
    ]
  }
}
```

**Backend применяет template к данным:**

```
for each product in data:
  widget = applyTemplate(template.widgetTemplate, product)
  formation.widgets.append(widget)
```

**Результат — готовая formation с данными → фронт.**

### Similarity Routing

Когда продукты похожи (≥80% общих полей) → один template на всех.

```
products = [{name, price, rating, image}, {name, price, rating, image}, ...]
           ↑ одинаковая структура

→ Formation mode: 1 template, N применений
→ Экономия: Agent 2 генерит 1 раз, не N раз
```

---

## Roadmap: Инкрементальная разработка

### Фаза 1: State + Storage (фундамент)
**Цель**: данные сохраняются и читаются

- [ ] PostgreSQL миграция: `chat_session_state`, `chat_session_deltas`
- [ ] Go структуры: State, Delta
- [ ] CRUD операции: CreateState, GetState, AddDelta
- [ ] **Проверка**: тест записи/чтения state

### Фаза 2: Agent 1 + 1 Tool (минимальный пайплайн)
**Цель**: запрос → tool call → данные в state

- [ ] Tool: `search_products` (mock данные пока)
- [ ] Промпт Agent 1 (минимальный)
- [ ] Вызов Anthropic API
- [ ] Tool executor: вызов → запись в state → "ok"
- [ ] **Проверка**: "покажи ноутбуки" → state.data.products заполнен

### Фаза 3: Agent 2 + Template (визуализация)
**Цель**: state → template → JSON для фронта

- [ ] Триггер после Agent 1
- [ ] Промпт Agent 2
- [ ] Agent 2 получает meta (count, fields)
- [ ] Agent 2 возвращает template
- [ ] Backend применяет template к data
- [ ] **Проверка**: получаем Formation JSON

### Фаза 4: Frontend рендеринг
**Цель**: JSON → UI

- [ ] React компоненты: Atom, Widget, Formation
- [ ] Рендеринг grid/carousel
- [ ] **Проверка**: видим карточки на экране

### Фаза 5: Второй tool + дельты
**Цель**: цепочка запросов работает

- [ ] Tool: `filter_products`
- [ ] Дельты записываются
- [ ] State обновляется инкрементально
- [ ] **Проверка**: "только до 100к" → выборка уменьшилась

### Фаза 6: Логирование + метрики
**Цель**: видим что происходит

- [ ] Timing points
- [ ] Token usage
- [ ] Costs
- [ ] **Проверка**: лог с breakdown по шагам

---

## TODO (legacy)

- [ ] Validation hooks
- [ ] Откаты (rollback)
- [ ] Интерактивные виджеты
- [ ] Anthropic prompt caching
- [ ] Чекпоинты для быстрого replay
