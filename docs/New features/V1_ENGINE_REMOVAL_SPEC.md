# V1 Engine Removal — Refactoring Spec

Дата: 2026-03-30
Статус: Planned
Тип: Рефакторинг

---

## Контекст

На Railway задеплоен V2 движок (`ENGINE_VERSION=v2`). V1 код — мертвый балласт ~4100 LOC, который не вызывается в production. Удаление упростит кодовую базу и уберет путаницу между двумя путями исполнения.

---

## Критический баг: Pipeline использует V1 Agent2

`PipelineExecuteUseCase` (`usecases/pipeline_execute.go`, line 85) хардкодит V1 Agent2 внутри конструктора:

```go
agent2UC: NewAgent2ExecuteUseCase(llm, statePort, toolRegistry, log),  // V1!
```

Main.go создает V2 `agent2UC` (line 175), но pipeline его **игнорирует** и создает свой V1. Это значит:
- Tool registry V2 работает (visual_assembly V2 schema + executeV2)
- Но **промпт Agent2 — V1** (Agent2SystemPrompt вместо Agent2ToolSystemPromptV2)
- Agent2 видит V2 tool schema, но получает V1 инструкции

**Fix**: Принимать `agent2UC` как параметр вместо создания внутри конструктора.

---

## Этап 1: Fix PipelineExecuteUseCase (критический, можно деплоить отдельно)

### Файлы:
- `usecases/pipeline_execute.go` — изменить конструктор: принимать `*Agent2ExecuteUseCase` как параметр
- `cmd/server/main.go` — передавать уже созданный V2 agent2UC в pipeline

### Было:
```go
func NewPipelineExecuteUseCase(llm, statePort, cachePort, tracePort, catalogPort, toolRegistry, presetRegistry, log) {
    agent2UC: NewAgent2ExecuteUseCase(llm, statePort, toolRegistry, log),  // hardcoded V1
}
```

### Стало:
```go
func NewPipelineExecuteUseCase(agent2UC, statePort, cachePort, tracePort, catalogPort, toolRegistry, presetRegistry, log) {
    agent2UC: agent2UC,  // injected, V1 or V2 based on ENGINE_VERSION
}
```

---

## Этап 2: Удалить V1 backend код

### Файлы для полного удаления:

| Файл | LOC | Причина |
|------|-----|---------|
| `tools/tool_visual_assembly_v1.go` | ~540 | definitionV1(), executeV1() — V1-only |
| `tools/tool_render_preset.go` | ~241 | Мертвый код, не зарегистрирован ни в одном registry |
| `tools/tool_render_preset_test.go` | ~100 | Тест мертвого кода |
| `engine/constraints.go` | ~200 | V1-only constraints, V2 использует rules.go |
| `engine/layout.go` | ~150 | V1-only CalculateZones(), V2 использует auto_layout.go |
| `presets/product_presets.go` | ~120 | V1-only пресеты |
| `presets/service_presets.go` | ~120 | V1-only пресеты |
| `presets/visual_assembly_presets.go` | ~114 | V1-only пресеты |

### Файлы для модификации:

**`tools/tool_visual_assembly.go`**:
- Убрать `NewVisualAssemblyTool()` конструктор (V1)
- Убрать V1 ветки в `Definition()` и `Execute()` (if v2/else — оставить только V2)
- Переименовать `NewVisualAssemblyToolV2` → `NewVisualAssemblyTool`

**`tools/tool_registry.go`**:
- Убрать `NewRegistry()` (V1)
- Переименовать `NewRegistryV2()` → `NewRegistry()`
- Убрать V1 fallback в NewRegistryV2 (line 80)

**`usecases/agent2_execute.go`**:
- Убрать `NewAgent2ExecuteUseCase()` (V1 конструктор)
- Переименовать `NewAgent2ExecuteUseCaseV2()` → `NewAgent2ExecuteUseCase()`

**`prompts/prompt_compose_widgets.go`**:
- Убрать `Agent2SystemPrompt` (V1 prompt, lines 14-75)
- Убрать `BuildAgent2ToolPrompt()` (V1 builder)
- Переименовать V2 варианты → основные имена

**`cmd/server/main.go`**:
- Убрать `ENGINE_VERSION` env var и V1 fallback ветки (lines 139-152, 174-181)
- Всегда инициализировать V2: FieldDefinitionAdapter, PresetV2Registry, NewRegistry, NewAgent2ExecuteUseCase

---

## Этап 3: Рефакторинг V1-зависимых модулей на V2 engine

Эти модули строят formations через V1 path (`BuildFormation` + `PresetRegistry` + `ProductFieldGetter`). Перевести на `EngineV2.Execute()`.

### 3a. Navigation (expand/back)

**Файлы:**
- `usecases/navigation_expand.go` — принимает `*presets.PresetRegistry` (V1), вызывает `engine.BuildFormation + ProductFieldGetter`
- `usecases/navigation_back.go` — аналогично

**Fix**: Заменить `PresetRegistry` на `PresetV2Registry` + `FieldDefinitionPort`. Использовать `EngineV2.Execute()` вместо `BuildFormation`.

### 3b. Action View (liked/cart)

**Файл:** `usecases/action_view.go` — `BuildFieldConfigs + BuildFormation + ProductFieldGetter`

**Fix**: Аналогично navigation.

### 3c. Pipeline adjacentTemplates

**Файл:** `usecases/pipeline_execute.go`, lines 372-381 — `BuildTemplateFormation + BuildFieldConfigs`

**Fix**: Строить adjacentTemplates через V2 engine.

### 3d. Testbench

**Файл:** `handlers/handler_testbench.go` — полностью V1 (`PresetRegistry`, `BuildFieldConfigsWithFormat`, `CalculateZones`)

**Fix**: Переписать на V2 engine (принимать те же параметры, но строить через `EngineV2.Execute()`).

### 3e. Debug/Seed

**Файл:** `handlers/handler_debug.go` — V1 `PresetRegistry`

**Fix**: Переписать на V2 или убрать seed функционал.

---

## Этап 4: Удалить V1 frontend код

### Файлы для полного удаления (10 шт):

| Файл | LOC |
|------|-----|
| `templates/GenericCardTemplate.jsx` | 167 |
| `templates/GenericCardTemplate.css` | ~150 |
| `templates/ProductCardTemplate.jsx` | 207 |
| `templates/ProductCardTemplate.css` | ~200 |
| `templates/ServiceCardTemplate.jsx` | 177 |
| `templates/ServiceCardTemplate.css` | ~170 |
| `templates/ProductDetailTemplate.jsx` | 245 |
| `templates/ProductDetailTemplate.css` | ~250 |
| `templates/ServiceDetailTemplate.jsx` | 231 |
| `templates/ServiceDetailTemplate.css` | ~230 |

### Файлы для модификации:

**`templates/index.js`**:
- Убрать 5 V1 экспортов (GenericCardTemplate, ProductCardTemplate, ServiceCardTemplate, ProductDetailTemplate, ServiceDetailTemplate)

**`WidgetRenderer.jsx`**:
- Убрать import AtomRenderer (line 2)
- Убрать import V1 templates (line 3)
- Убрать switch cases для V1 templates (lines 66-80): GenericCard, ProductCard, ServiceCard, ProductDetail, ServiceDetail
- Убрать legacy functions (lines 82-120): ProductCard(), TextBlock(), QuickReplies(), DefaultWidget()

**`ComparisonTemplate.jsx`**:
- Убрать import AtomRenderer (line 1)
- Line 231: заменить `v2 ? <AtomV2Renderer> : <AtomRenderer>` на просто `<AtomV2Renderer>`

**`widget.jsx`**:
- Убрать 5 V1 CSS imports (lines 13-17): productCardCss, productDetailCss, serviceCardCss, serviceDetailCss, genericCardCss
- Убрать из ALL_CSS массива (line ~38)

**`preview.jsx`**:
- Аналогично widget.jsx — убрать 5 V1 CSS imports

### После проверки (опционально):
- `atom/AtomRenderer.jsx` + `Atom.css` — удалить если не используется после очистки ComparisonTemplate

---

## Этап 5: Удалить все backend тесты

> Подробности: `TEST_RESET_SPEC.md`

Все 27 `*_test.go` файлов удаляются вместе с V1 кодом. Тесты устарели, завязаны на V1, и будут заново покрываться через E2E в `tests/`. Makefile test targets (`test-unit`, `test-integration`, `test-usecase`, `test-llm`, `test-all`) — убрать.

### Ранее: Обновить тесты

| Тест | Что менять |
|------|-----------|
| `usecases/cache_test.go` | `NewRegistry` → новый `NewRegistry` (бывший V2) |
| `usecases/tool_execution_integration_test.go` | аналогично |
| `usecases/agent1_execute_test.go` | аналогично |
| `usecases/pipeline_mock_llm_test.go` | аналогично + agent2UC injection |
| `usecases/agent2_execute_test.go` | `NewAgent2ExecuteUseCase` → новый (бывший V2) |
| `usecases/navigation_test.go` | PresetRegistry → PresetV2Registry |
| `usecases/navigation_integration_test.go` | аналогично |
| `handlers/smoke_test.go` | PresetRegistry → PresetV2Registry |
| `tools/tool_render_preset_test.go` | удалить (вместе с tool_render_preset.go) |

---

## Этап 6: Финальный cleanup

- Убрать `ENGINE_VERSION` и `AGENT2_PROMPT_VERSION` из env vars (всегда V2)
- Переименовать все `*V2` суффиксы → основные имена:
  - `NewRegistryV2` → `NewRegistry`
  - `NewAgent2ExecuteUseCaseV2` → `NewAgent2ExecuteUseCase`
  - `Agent2ToolSystemPromptV2` → `Agent2ToolSystemPrompt`
  - `BuildAgent2ToolPromptV2` → `BuildAgent2ToolPrompt`
  - `NewVisualAssemblyToolV2` → `NewVisualAssemblyTool`
- Оценить `engine/compat.go` — `WidgetV2ToLegacy()` вызывается из `engine_v2.go:230`. Нужен пока фронтенд читает `widget.atoms`/`widget.zones` (legacy fields). Удалить когда фронтенд полностью на V2.
- Вычистить shared файлы (`formation.go`, `assembly.go`, `defaults.go`) от V1-only функций
- Обновить `CLAUDE.md` и `README.md` — убрать V1/V2 разделение, описать единый движок

---

## Порядок и группировка PR

```
PR 1: Этап 1 (fix pipeline bug)         — критический, деплоить сразу
PR 2: Этап 2 + 3 + 5 (backend cleanup)  — большой рефакторинг
PR 3: Этап 4 (frontend cleanup)          — отдельный PR
PR 4: Этап 6 (final cleanup)             — косметика
```

---

## Верификация

После каждого этапа:
- `cd project/backend && make test-unit`
- `cd project/backend && make test-all`
- Локальный запуск без `ENGINE_VERSION` env var — всё работает (V2 по дефолту)
- E2E: `cd tests && python e2e_run.py`
- Проверить на Railway после деплоя каждого PR

---

## Полная инвентаризация V1 кода

### Backend удаление (~2800 LOC):
```
tools/tool_visual_assembly_v1.go        — 540 LOC (целый файл)
tools/tool_render_preset.go             — 241 LOC (целый файл)
tools/tool_render_preset_test.go        — 100 LOC (целый файл)
engine/constraints.go                   — 200 LOC (целый файл, V1-only)
engine/layout.go                        — 150 LOC (целый файл, V1-only)
presets/product_presets.go              — 120 LOC (целый файл)
presets/service_presets.go              — 120 LOC (целый файл)
presets/visual_assembly_presets.go      — 114 LOC (целый файл)
+ ~200 LOC из модифицируемых файлов (V1 ветки, конструкторы, промпты)
```

### Frontend удаление (~2237 LOC):
```
templates/GenericCardTemplate.jsx+css   — ~317 LOC
templates/ProductCardTemplate.jsx+css   — ~407 LOC
templates/ServiceCardTemplate.jsx+css   — ~347 LOC
templates/ProductDetailTemplate.jsx+css — ~495 LOC
templates/ServiceDetailTemplate.jsx+css — ~461 LOC
atom/AtomRenderer.jsx + Atom.css        — ~210 LOC (опционально)
```

### Engine файлы — что остается (V2 + shared):
```
engine/engine_v2.go          — V2 orchestrator (KEEP)
engine/auto_layout.go        — V2 layout (KEEP)
engine/layout_pass.go        — V2 budget/needs (KEEP)
engine/rules.go              — V2 constraints (KEEP)
engine/tokens.go             — V2 design tokens (KEEP)
engine/instructions.go       — V2 agent instructions (KEEP)
engine/compat.go             — V2→V1 bridge (KEEP until frontend fully V2)
engine/formation.go          — shared (KEEP, clean V1 functions later)
engine/assembly.go           — shared (KEEP, clean V1 functions later)
engine/defaults.go           — shared (KEEP, clean V1 functions later)
engine/field_types.go        — shared fallback (KEEP)
```
