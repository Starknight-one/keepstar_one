# Done: Adjacent Templates — N formations to 1 template + entities

**Дата:** 2026-02-12
**Ветка:** `feature/instant-navigation`
**Статус:** Реализовано, Go компилируется, фронт билдится, визуально проверено

## Проблема

`buildAdjacentFormations` строил до 15 detail-формаций (по одной на каждый товар в гриде). Все имели одинаковую структуру (слоты, displays из `product_detail` пресета) — отличались только значения. 6 товаров = 6 почти одинаковых JSON.

**Payload:** 6 products x ~1.2KB = ~7.2KB adjacent data.
**CPU:** 6x `BuildFormation` на бэкенде.

## Решение: 1 template + raw entities

Бэкенд строит 1 шаблон на тип entity (product/service) и шлёт сырые данные entities. Фронт заполняет шаблон при клике.

**Scope:** Только детерминированные переходы (клик на карточку). Chat-запросы ("убери рейтинг") идут через Pipeline/Agent2 как раньше — это новый узел дерева, round-trip ожидаем.

## Ключевая механика

### Template Formation

Обычная формация (`FormationWithData`), но:
- Один widget с `id: "template"`
- Каждый atom: `value: null`, `fieldName: "price"` (имя поля entity)
- Все поля пресета включены (в обычной формации nil-поля пропускаются)
- Currency meta содержит sentinel `"__ENTITY_CURRENCY__"` вместо реальной валюты

### FieldName на Atom

Новое поле `FieldName string` в `domain.Atom`:
```go
type Atom struct {
    Type      AtomType    `json:"type"`
    Subtype   AtomSubtype `json:"subtype,omitempty"`
    Display   string      `json:"display,omitempty"`
    Value     interface{} `json:"value"`
    FieldName string      `json:"fieldName,omitempty"` // NEW: source field name
    Slot      AtomSlot    `json:"slot,omitempty"`
    Meta      map[string]interface{} `json:"meta,omitempty"`
}
```
`omitempty` — не ломает существующие формации. Появляется только в template-атомах.

### fillFormation (frontend)

Чистая функция `fillFormation(template, entity, entityType)`:
1. Итерирует `template.widgets[0].atoms`
2. Для каждого: `value = getField(entity, atom.fieldName)` — зеркало Go-геттеров
3. `value == null` → пропускаем (как бэкенд)
4. Заменяет currency sentinel `__ENTITY_CURRENCY__` → `entity.currency`
5. Генерирует уникальный widget ID, ставит entityRef

### Pipeline Response

Было:
```json
{
  "formation": {...},
  "adjacentFormations": {
    "product:abc": { "mode": "single", "widgets": [...] },
    "product:def": { "mode": "single", "widgets": [...] },
    ...6 formations...
  }
}
```

Стало:
```json
{
  "formation": {...},
  "adjacentTemplates": {
    "product": { "mode": "single", "widgets": [{ atoms with fieldName, value: null }] }
  },
  "entities": {
    "products": [{ id, name, price, ... }, ...],
    "services": [...]
  }
}
```

### Expand flow (frontend)

```
User clicks card (entityType="product", entityId="abc")
  → lookup adjacentTemplatesRef["product"] → found template
  → lookup entitiesRef.products.find(e => e.id === "abc") → found entity
  → fillFormation(template, entity, "product") → filled formation
  → instant render (<16ms)
  → syncExpand() fire-and-forget
```

Fallback: если template или entity не найдены → API call как раньше.

## Что сделано

### Backend (4 файла)

#### `domain/atom_entity.go`
- Добавлено поле `FieldName string` в Atom struct

#### `tools/tool_render_preset.go`
- Новая функция `BuildTemplateFormation(preset) *FormationWithData`:
  - Создаёт формацию с ВСЕМИ полями пресета (не пропускает nil)
  - Каждый atom: `Value: nil`, `FieldName: field.Name`
  - Корректные Type/Subtype/Display/Slot/Meta
  - Currency sentinel `"__ENTITY_CURRENCY__"` в meta
  - 1 widget, mode/size из preset

#### `usecases/pipeline_execute.go`
- `PipelineExecuteResponse`: `AdjacentFormations` → `AdjacentTemplates` + `Entities`
- Новый метод `buildAdjacentTemplates(state)` → 1 template per entity type + raw entities
- Удалён `buildAdjacentFormations` (строки 322-365)

#### `handlers/handler_pipeline.go`
- `PipelineResponse`: `AdjacentFormations` → `AdjacentTemplates` + `Entities`
- Обновлена сериализация

### Frontend (4 файла)

#### `features/chat/model/fillFormation.js` (NEW)
- `fillFormation(template, entity, entityType)` → filled FormationWithData
- `getField(entity, fieldName)` — зеркало Go productFieldGetter/serviceFieldGetter
- ~90 строк кода

#### `features/chat/ChatPanel.jsx`
- `adjacentFormationsRef` → `adjacentTemplatesRef` + `entitiesRef`
- `onFormationReceived(formation, adjacentTemplates, entities)` — 3 аргумента
- `handleExpand`: lookup template по entityType → find entity по id → `fillFormation()` → render
- Fallback на API если template/entity нет
- Session cache save/restore включает templates + entities (instant expand переживает F5)
- Session clear обнуляет templates + entities

#### `features/chat/useChatSubmit.js`
- `onFormationReceived(response.formation, response.adjacentTemplates, response.entities)` — 3 аргумента

#### `features/chat/sessionCache.js`
- `saveSessionCache`: принимает + сохраняет `adjacentTemplates`, `entities`

### Bugfix: Expand → RenderConfig (найдено при проверке)

#### `usecases/navigation_expand.go`
- `buildDetailFormation` не ставил `Config` (RenderConfig) на formation
- Agent1 проверяет `f.Config != nil` → nil → не видит что мы на detail view → запускает поиск заново
- Фикс: добавлен `formation.Config` с Preset, Mode, Size, Fields после `buildDetailFormation`
- Теперь Agent1 видит `current_display: { preset: "product_detail", mode: "single" }` и `displayed_fields`
- Запросы "убери описание" маршрутизируются в Agent2, а не триггерят новый поиск

## Payload

| | До (6 products) | После |
|--|-----------------|-------|
| Adjacent formations | 6 x ~1.2KB = ~7.2KB | 0 |
| Adjacent template | 0 | 1 x ~0.5KB |
| Entity data | 0 | 6 x ~300B = ~1.8KB |
| **Итого** | **~7.2KB** | **~2.3KB (~68% reduction)** |

Backend CPU: 6x `BuildFormation` → 1x `BuildTemplateFormation`

## Архитектурное решение

**Сдвиг парадигмы:** Раньше "фронт = тупая рендерилка, бэкенд всё готовит". Теперь `fillFormation` — первая логика на клиенте. Это осознанный trade-off:
- Template конструкция (preset → slots → displays) остаётся на бэкенде
- Фронт только подставляет значения по fieldName — механическая операция
- Preset configs НЕ утекают на фронт — фронт не знает что такое preset
- Если template нет → fallback на API (бэкенд построит всё сам)

## Файлы

| Файл | Действие |
|------|----------|
| `project/backend/internal/domain/atom_entity.go` | MODIFY (FieldName field) |
| `project/backend/internal/tools/tool_render_preset.go` | MODIFY (BuildTemplateFormation) |
| `project/backend/internal/usecases/pipeline_execute.go` | MODIFY (templates + entities) |
| `project/backend/internal/handlers/handler_pipeline.go` | MODIFY (serialization) |
| `project/backend/internal/usecases/navigation_expand.go` | MODIFY (RenderConfig bugfix) |
| `project/frontend/src/features/chat/model/fillFormation.js` | NEW |
| `project/frontend/src/features/chat/ChatPanel.jsx` | MODIFY |
| `project/frontend/src/features/chat/useChatSubmit.js` | MODIFY |
| `project/frontend/src/features/chat/sessionCache.js` | MODIFY |

## Верификация

- [x] `go build ./...` — компилируется
- [x] `npm run build` — билдится
- [x] Grid → expand → instant → back → expand другую → instant
- [ ] F5 → back → expand → из sessionCache
- [ ] Fallback: без templates → expand через API
- [ ] Chat на detail view → Agent2 (не Agent1 search) — зависит от RenderConfig bugfix
