# E2E Root Causes — Анализ багов Run 2 (2026-03-28)

Результат прогона: 6/25 OK, 14 багов, 2 delayed, 3 unclear.
Ниже — корневые причины каждой категории.

---

## B1 — Layout switching не работает / отстаёт на шаг

**Кейсы**: #2, #3, #10, #11, #12, #18, #22

Три проблемы в цепочке:

### 1. Agent2 читает стейл стейт
**Файл**: `usecases/agent2_execute.go:113`

Agent2 читает `currentConfig` один раз в начале. Видит СТАРЫЙ layout (grid), хотя Agent1 уже обновил данные. Когда юзер говорит "покажи списком", Agent2 всё ещё думает что на экране grid.

### 2. Конфликт в промпте Agent2
**Файл**: `prompts/prompt_compose_widgets.go:170-174`

Agent2 получает два противоречащих сигнала:
- `current_formation: grid` (стейл из БД)
- `screen_state: list` (что юзер реально видит)

LLM путается какому источнику верить. Правило #4 говорит "не меняй layout без keywords", правило #7 говорит "уважай screen_state".

### 3. Tool откатывает решение Agent2
**Файл**: `tools/tool_visual_assembly_v1.go:387-399`

```go
if currentConfig != nil && !layoutKeywords.MatchString(toolCtx.UserQuery) {
    layout = string(currentConfig.Mode) // откат к старому layout
}
```

Даже если Agent2 правильно послал `layout: "list"`, tool проверяет regex keywords в запросе юзера. Не нашёл → откат. Слишком консервативная защита, которая убивает работающее решение LLM.

**Фронтенд чист** — FormationRenderer корректно рендерит все layouts по CSS-классам.

---

## B2 — Show/Hide/Order сломаны

**Кейсы**: #4, #5, #6, #8

### 1. Show не аддитивный — нет "текущего" состояния
**Файл**: `engine/engine_v2.go:455-459`

```go
baseFields := resolved.Fields
if len(baseFields) == 0 {
    baseFields = fields
}
```

Движок не знает какие поля СЕЙЧАС видны на экране. Работает от `resolved.Fields` (дефолты пресета). Юзер видит `[image, name, price, description]`, говорит "добавь рейтинг" → движок пересчитывает с нуля от пресета → result: `[image, name, price, rating]` (description потерян).

### 2. Order не доходит до рендера
**Файл**: `engine/engine_v2.go:504-520`

- Order правильно переставляет fieldNames
- Но `AutoLayout` потом группирует атомы по типу/слоту → кастомный порядок уничтожается
- `AutoLayoutSequential` (который сохраняет порядок) вызывается только при наличии order, но layout tree всё равно не сохраняет порядок атомов

### 3. Hide не защищён от C1 нормализации
**Файл**: `engine/rules.go:159-162`

Show-поля защищены через `protectedFields`, а Hide-поля — нет. Скрытое поле может вернуться как пустой placeholder после C1 нормализации.

### 4. Atom indices ломаются при reorder
**Файл**: `frontend: LayoutTreeRenderer.jsx:63-69`

Layout tree ссылается на `atomIndex: 0`. Если backend переставил атомы, индекс указывает на другой атом. Frontend рендерит не то.

---

## B3 — Фильтрация и поиск отстают на шаг

**Кейсы**: #14, #16, #17, #20, #21, #25

### 1. catalog_search неполно обновляет meta
**Файл**: `tools/tool_catalog_search.go:472-477`

```go
stateMeta := domain.StateMeta{
    Count:   total,
    Fields:  fields,
    Aliases: state.Current.Meta.Aliases,
    // ProductCount и ServiceCount НЕ ЗАДАНЫ → 0 в БД
}
```

state_filter пишет все 5 полей корректно. catalog_search — только 3.

### 2. Agent1 читает стейл после zone-write
**Файл**: `usecases/agent1_execute.go:286-287`

```go
state, _ = uc.statePort.GetState(ctx, req.SessionID)
productsFound = state.Current.Meta.Count // может быть от предыдущего turn'а
```

После zone-write, Agent1 делает `GetState()` и из-за DB isolation timing может получить СТАРЫЙ count. Microcontext для Agent2 строится из стейл значения.

### 3. Agent2 не пересчитывает Count
**Файл**: `usecases/agent2_execute.go:119-120, 263`

```go
state.Current.Meta.ProductCount = len(state.Current.Data.Products)  // пересчитывает
state.Current.Meta.ServiceCount = len(state.Current.Data.Services)  // пересчитывает
// Но Count берёт как есть из meta (стейл!)
MetaCount: state.Current.Meta.Count,  // line 263
```

### Паттерн отставания

```
Turn N: tool пишет 10 items в БД
  → Agent1 GetState() → читает 50 (от Turn N-1)
  → microcontext = "50 items"
  → Agent2 рендерит 50

Turn N+1: теперь данные видны
  → Agent1 GetState() → читает 10 (наконец)
  → работает правильно, но с опозданием на 1 ход
```

---

## B4 — Визуальные модификаторы не работают

**Кейсы**: #13, #24

### Direction horizontal
Бэкенд может послать `direction`, но фронтенд не обрабатывает его — нет CSS-маппинга для horizontal direction на карточках.

### Тени и скругления
`widgetShadow` / `widgetBorderRadius` приходят в toolInput, но нет CSS-переменных / классов которые их применяют на фронтенде.

---

## B5 — Кнопка корзины

**Кейсы**: все карточки

Текущее: текстовая кнопка "Add cart".
Ожидание: круглая кнопка с плюсом (+), при нажатии → каунтер (−1+).

Отдельная UI-задача, не связана с пайплайном.

---

## Общая картина

Три системные проблемы объясняют большинство багов:

1. **Stale state** — Agent2 и Agent1 читают стейт до/после записи, получают данные от предыдущего хода. Это root cause для B1 + B3 (9 кейсов).

2. **Нет "текущего состояния виджета"** — движок не знает что СЕЙЧАС на экране. Show/Hide работают от пресет-дефолтов, а не от реального состояния. Root cause для B2 (4 кейса).

3. **Tool перезаписывает Agent2** — visual_assembly_v1 tool слишком консервативно откатывает layout решения LLM. Root cause для B1 layout switching.

Фронтенд в целом чист — проблемы на бэкенде в пайплайне и движке.
