# Feature: Agent 2 Smart Render (Output Layer Upgrade)

## ADW ID: feature-agent2-smart-render

## Суть

Agent 2 (presentation layer) из жёсткого if/else роутера превращается в умный маршрутизатор. Получает полный контекст (что на экране, запрос пользователя, метаданные полей) и решает:
- **Нет пожеланий** — вызывает render_*_preset с дефолтным пресетом (как раньше)
- **Есть пожелания** — конструирует виджет через `fields[]`, полностью заменяя дефолтные поля пресета

Формация (preset) = маска лейаута (как виджеты располагаются).
Поля (fields) = что в каждом виджете (доска с дырками).

## Ключевые принципы

1. **Formation != Preset.** `product_grid` = формация (лейаут). Пресет = конфигурация одного виджета.
2. **Три тула раздельно:** `render_product_preset`, `render_service_preset`, `freestyle`. Микс — позже.
3. **Дефолт = нет пожеланий.** Любое пожелание от пользователя -> конструирование через `fields[]`.
4. **Freestyle отключён.** В промпте: "зарезервирован, не используй".
5. **Aliases = метаданные.** Компактное описание доступных наборов данных.

## Data Flow

```
User: "покажи покрупнее с рейтингом"
  -> Agent2 получает контекст:
     {
       productCount: 5,
       fields: ["name", "price", "images", "rating", "brand"],
       aliases: {"price": "Цена", "brand": "Бренд"},
       current_formation: {
         preset: "product_grid",
         mode: "grid",
         size: "medium",
         fields: [
           {"name":"images", "slot":"hero", "display":"image-cover"},
           {"name":"name", "slot":"title", "display":"h2"},
           {"name":"price", "slot":"price", "display":"price"}
         ]
       },
       user_request: "покажи покрупнее с рейтингом"
     }
  -> Agent2 видит: на экране grid из images+name+price.
     Пользователь хочет крупнее + рейтинг.
  -> Вызывает:
     render_product_preset(
       preset="product_grid",
       fields=[
         {"name":"images","slot":"hero","display":"image-cover"},
         {"name":"name","slot":"title","display":"h1"},
         {"name":"price","slot":"price","display":"price-lg"},
         {"name":"rating","slot":"primary","display":"rating"}
       ]
     )
  -> BuildFormation() строит виджеты с кастомными полями
  -> formation.Config = RenderConfig{...} (для следующего хода)
  -> State write: template.formation
```

## Relevant Files

### Modified Files

- `project/backend/internal/domain/template_entity.go` — +FieldSpec, +RenderConfig, +Config в FormationWithData
- `project/backend/internal/tools/tool_render_preset.go` — +fields[]/size в Definition/Execute обоих тулов, +fieldTypeMap, +parseFieldSpecs, +buildRenderConfig
- `project/backend/internal/prompts/prompt_compose_widgets.go` — новый Agent2ToolSystemPrompt, обогащённый BuildAgent2ToolPrompt
- `project/backend/internal/usecases/agent2_execute.go` — извлечение currentConfig, передача в prompt builder

### Read-Only (не тронуты)

- `project/backend/internal/tools/tool_freestyle.go` — зарезервирован, не удаляется
- `project/backend/internal/tools/tool_registry.go` — без изменений, все 3 тула зарегистрированы
- `project/backend/internal/presets/product_presets.go` — дефолтные конфигурации остаются
- `project/backend/internal/presets/service_presets.go` — дефолтные конфигурации остаются
- `project/backend/internal/usecases/pipeline_execute.go` — оркестратор, shape формации не меняется
- `project/backend/internal/usecases/navigation_expand.go` — использует BuildFormation напрямую
- `project/backend/internal/usecases/navigation_back.go` — использует BuildFormation напрямую
- Frontend — slot-based рендеринг уже работает

## Step by Step Tasks

### 1. Добавить FieldSpec, RenderConfig в domain

**Файл:** `project/backend/internal/domain/template_entity.go`

Новые типы:
```go
type FieldSpec struct {
    Name    string `json:"name"`    // "images", "name", "price"
    Slot    string `json:"slot"`    // "hero", "title", "price"
    Display string `json:"display"` // "image-cover", "h2", "price-lg"
}

type RenderConfig struct {
    EntityType string        `json:"entity_type"`
    Preset     string        `json:"preset,omitempty"`
    Mode       FormationType `json:"mode"`
    Size       WidgetSize    `json:"size"`
    Fields     []FieldSpec   `json:"fields,omitempty"`
}
```

Добавить `Config *RenderConfig json:"config,omitempty"` в `FormationWithData`.

### 2. Добавить fieldTypeMap и хелперы в tool_render_preset.go

**Файл:** `project/backend/internal/tools/tool_render_preset.go`

`fieldTypeMap` — 13 записей (product + service поля), резолвит field name -> AtomType/Subtype:
- Product: name, description, brand, category, price, rating, images, stockQuantity, tags, attributes
- Service: duration, provider, availability

`parseFieldSpecs(rawFields)` — парсит `[]interface{}` из LLM tool input в `[]FieldConfig` + `[]FieldSpec`. Неизвестные поля дефолтятся в text/string.

`buildRenderConfig(entityType, preset, size, fieldSpecs)` — строит RenderConfig. Если fieldSpecs пуст — берёт из preset defaults.

### 3. Расширить render_product_preset

**Файл:** `project/backend/internal/tools/tool_render_preset.go`

Definition(): +`fields` (array of {name,slot,display}, optional), +`size` (enum, optional).

Execute():
1. Загрузить пресет из registry
2. Если `fields[]` передан — `parseFieldSpecs` -> заменить `preset.Fields`
3. Если `size` передан — override `preset.DefaultSize`
4. `BuildFormation()` с модифицированным пресетом
5. `formation.Config = buildRenderConfig(...)` — записать что на экране
6. Zone-write в state

### 4. Расширить render_service_preset

**Файл:** `project/backend/internal/tools/tool_render_preset.go`

Аналогичные изменения. Service-специфичные поля в Description (duration, provider, availability). Merge с existing formation сохранён.

### 5. Переписать Agent2ToolSystemPrompt

**Файл:** `project/backend/internal/prompts/prompt_compose_widgets.go`

Жёсткий decision flow заменён на guidelines:
- Объясняет концепцию формации vs полей
- "Когда конструировать" — нет пожеланий/есть пожелания/модификация current_formation
- Доступные поля, слоты, display стили
- Ориентиры по количеству товаров
- Примеры с fields[]
- freestyle зарезервирован

### 6. Обогатить BuildAgent2ToolPrompt

**Файл:** `project/backend/internal/prompts/prompt_compose_widgets.go`

Новая сигнатура: `+currentConfig *domain.RenderConfig`

Добавлено в JSON контекст Agent 2:
- `current_formation` — из RenderConfig (что на экране сейчас)
- `aliases` — из StateMeta.Aliases (метаданные полей)

### 7. Обновить Agent2 orchestrator

**Файл:** `project/backend/internal/usecases/agent2_execute.go`

Перед вызовом LLM — извлечь `RenderConfig` из `state.Current.Template["formation"]` (type assertion к `*FormationWithData`, проверка Config != nil). Передать в `BuildAgent2ToolPrompt`.

## Acceptance Criteria

- [x] `FieldSpec`, `RenderConfig` — новые domain типы
- [x] `FormationWithData.Config` — заполняется при каждом рендере
- [x] `render_product_preset` Definition — `fields[]` и `size` optional параметры
- [x] `render_service_preset` Definition — `fields[]` и `size` optional параметры
- [x] `fieldTypeMap` — 13 полей (10 product + 3 service-specific)
- [x] Дефолтный режим (без fields) — работает как раньше, + Config записывается
- [x] Конструирование (с fields) — fields заменяют дефолтные поля пресета
- [x] Size override — опционально меняет размер виджета
- [x] Agent2 промпт — guidelines вместо rigid decision flow
- [x] freestyle — зарезервирован в промпте
- [x] Agent2 контекст содержит `current_formation` и `aliases`
- [x] Orchestrator извлекает RenderConfig из state перед LLM вызовом
- [x] `go build ./...` — OK

## Ограничения фронта (известные)

- Порядок слотов фиксирован в JSX: `hero -> badge -> title -> primary -> price -> secondary`
- Можно: добавлять/убирать поля (пустой слот не рендерится), менять display стиль
- Нельзя: менять порядок слотов, класть числа в hero слот (ожидает image)
- Будущее: generic slot renderer который рендерит атомы в порядке из бэкенда

## Hexagonal Architecture Compliance

- **Domain** — +2 типа (FieldSpec, RenderConfig), +1 поле (Config в FormationWithData)
- **Tools** — fields[]/size в Definition/Execute, fieldTypeMap, parseFieldSpecs, buildRenderConfig
- **Prompts** — новый системный промпт, обогащённый user prompt builder
- **Usecases** — извлечение currentConfig, передача в prompt
- **Ports** — без изменений
- **Adapters** — без изменений
- **Presets** — без изменений (остаются как дефолтные конфигурации)
