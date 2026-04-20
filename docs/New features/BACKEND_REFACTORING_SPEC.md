# Backend Refactoring — Hexagonal Architecture Cleanup

Дата: 2026-04-03
Статус: Planned
Тип: Рефакторинг

---

## Контекст

`project/backend/internal/` заявлен как гексагональная архитектура, но за время активной разработки накопились проблемы:
- 2 файла выросли до 1300+ строк, ещё 9 файлов >500 строк
- 3 нарушения архитектуры в handlers/ (прямые импорты адаптеров и engine)
- Это затрудняет навигацию, повышает когнитивную нагрузку и приводит к ошибкам при изменениях

Промпты проверены — все в `internal/prompts/`, вынос не требуется.

Тестов в `internal/` нет. Верификация через `go build ./...` + `go vet ./...` + smoke test endpoints.

---

## Аудит: что в порядке

| Слой | Статус | Комментарий |
|---|---|---|
| `domain/` (23 файла) | PASS | Чистый домен, только stdlib imports |
| `ports/` (8 файлов) | PASS | Только `domain/` imports |
| `adapters/` (18 файлов) | PASS | Импортируют только `domain/` + `ports/` + external |
| `usecases/` (16 файлов) | PASS | Импортируют `domain/` + `ports/` + `tools/` + `engine/` + `prompts/` |
| `tools/` (7 файлов) | PASS | Импортируют `domain/` + `ports/` + `engine/` |
| `engine/` (15 файлов) | PASS | Только `domain/` imports, чистый алгоритмический слой |
| `prompts/` (2 файла) | PASS | Только `domain/` imports |
| `presets/` (1 файл) | PASS | Только `domain/` imports |
| `logger/`, `config/` | PASS | Инфраструктурные утилиты без internal imports |

---

## Нарушение 1: middleware_logging.go импортирует конкретный адаптер

**Файл**: `handlers/middleware_logging.go` (97 строк)
**Строка 9**: `import "keepstar/internal/adapters/postgres"`
**Строка 30**: `func LoggingMiddleware(log *logger.Logger, logAdapter *postgres.LogAdapter)`
**Строки 78-93**: создаёт `&postgres.RequestLog{...}` и вызывает `logAdapter.RecordRequestLog()`

**Почему плохо**: handlers не должны знать о конкретных адаптерах. Если заменить PostgreSQL — handler сломается.

**Порт `LogPort` отсутствует** — в `ports/` нет интерфейса для логирования запросов.

### Fix

1. Создать `domain/request_log_entity.go`:
```go
type RequestLogEntry struct {
    ID, Service, Method, Path string
    Status                     int
    DurationMs                 int64
    SessionID, TenantSlug      string
    UserID, Error              string
    Spans                      []Span
    Metadata                   map[string]any
}
```

2. Создать `ports/log_port.go`:
```go
type LogPort interface {
    RecordRequestLog(ctx context.Context, entry *domain.RequestLogEntry) error
}
```

3. `adapters/postgres/postgres_logs.go` — удалить `RequestLog` struct, метод принимает `*domain.RequestLogEntry`

4. `handlers/middleware_logging.go`:
   - Убрать import `adapters/postgres`
   - Сигнатура: `LoggingMiddleware(log *logger.Logger, logPort ports.LogPort)`
   - Создавать `&domain.RequestLogEntry{...}` вместо `&postgres.RequestLog{...}`

5. `cmd/server/main.go:341` — `logAdapter` уже удовлетворяет `ports.LogPort`, минимальное изменение

**Затронуто**: 4 файла + 1 новый

---

## Нарушение 2: handler_debug.go импортирует engine + presets

**Файл**: `handlers/handler_debug.go` (722 строк)
**Строки 12-14**: `import "keepstar/internal/engine"`, `"keepstar/internal/presets"`

Handler создаёт `engine.NewEngineV2()` и выполняет engine pipeline прямо в HTTP handler. Это бизнес-логика, она должна быть в usecase.

### Fix

1. Создать `usecases/testbench_execute.go` — `TestbenchExecuteUseCase`:
   - Принимает `ports.CatalogPort`, `ports.FieldDefinitionPort`, `*presets.PresetV2Registry`
   - Метод `Execute(ctx, tenantSlug, count, params)` — содержит логику engine execution
   - Usecase-слой уже импортирует `engine` (5 существующих файлов делают это)

2. `handlers/handler_debug.go` — `HandleSeedState` делегирует в usecase. Убрать imports `engine`/`presets`.

3. `cmd/server/main.go:215` — передать usecase в конструктор `DebugHandler`

**Затронуто**: 2 файла + 1 новый

---

## Нарушение 3: handler_testbench.go импортирует engine + presets

**Файл**: `handlers/handler_testbench.go` (303 строки)
**Строки 10-12**: `import "keepstar/internal/engine"`, `"keepstar/internal/presets"`

Аналогичная проблема — handler создаёт engine и выполняет его.

### Fix

1. Использовать тот же `usecases/testbench_execute.go` (или метод на нём)
2. Перенести `buildTestbenchInstructions()` в usecase — функция строит `engine.AgentInstructions`
3. `handlers/handler_testbench.go` — тонкий HTTP adapter: parse request -> call usecase -> write response
4. `cmd/server/main.go:306` — передать usecase в конструктор `TestbenchHandler`

**Затронуто**: 2 файла (+ общий usecase из нарушения 2)

---

## Большие файлы: полная карта

Отсортировано по размеру, все файлы >400 строк:

| # | Файл | Строк | Tier |
|---|---|---|---|
| 1 | `adapters/postgres/postgres_catalog.go` | 1456 | Tier 2 |
| 2 | `engine/ops.go` | 1304 | Tier 2 |
| 3 | `tools/tool_visual_assembly.go` | 901 | Tier 3 |
| 4 | `engine/engine_v2.go` | 807 | Tier 3 |
| 5 | `handlers/handler_debug.go` | 722 | Tier 3 |
| 6 | `tools/tool_catalog_search.go` | 660 | Tier 3 |
| 7 | `adapters/anthropic/anthropic_client.go` | 648 | Tier 4 |
| 8 | `handlers/handler_trace.go` | 644 | Tier 3 |
| 9 | `adapters/postgres/postgres_state.go` | 620 | Tier 4 |
| 10 | `engine/rules.go` | 563 | Tier 4 |
| 11 | `usecases/pipeline_execute.go` | 516 | Tier 4 |
| 12 | `engine/defaults.go` | 415 | Tier 4 |
| 13 | `prompts/prompt_compose_widgets.go` | 410 | OK (промпты длинные по природе) |
| 14 | `usecases/agent2_execute.go` | 404 | Tier 4 |

---

## Tier 2 — Критичные сплиты (>1000 строк)

### 2A. postgres_catalog.go (1456 -> 4 файла)

21 метод. Содержит: product CRUD + service CRUD + vector search + digest + embeddings. Логика products/services ~дублирована.

Все файлы остаются в `package postgres` — чистый file-level split, 0 изменений import paths.

| Новый файл | Содержимое | ~строк |
|---|---|---|
| `postgres_catalog.go` | CatalogAdapter struct, constructor, GetTenantBySlug, GetAllTenants, categories, stock, shared helpers (formatPrice, masterProductRow, mergeProductWithMaster) | ~280 |
| `postgres_catalog_products.go` | ListProducts, GetProduct, GetMasterProduct, VectorSearch, SeedEmbedding, GetMasterProductsWithoutEmbedding | ~400 |
| `postgres_catalog_services.go` | ListServices, GetService, VectorSearchServices, SeedServiceEmbedding, GetMasterServicesWithoutEmbedding, masterServiceRow, mergeServiceWithMaster | ~320 |
| `postgres_catalog_digest.go` | GenerateCatalogDigest, SaveCatalogDigest, GetCatalogDigest | ~180 |

### 2B. engine/ops.go (1304 -> 4 файла)

28 функций. Содержит: парсинг операций, tree index, CRUD на дереве, property merging (11 хелперов), snapshot/customization система, post-op constraints.

Все файлы остаются в `package engine`.

| Новый файл | Содержимое | ~строк |
|---|---|---|
| `ops.go` | Op type, ParseOps, ApplyOps, resolveRefs, applyUpdate, applyDelete, applyMove, applyInsert (CRUD dispatch) | ~350 |
| `ops_tree.go` | idIndex, buildIndex, indexLayoutNode, insertAtom, insertLayoutNode, insertChildIntoNode, findWidgetForNode, nodeInTree, reindexWidget, removeAtomFromLayout, fixAtomIndices, removeChildNode | ~250 |
| `ops_merge.go` | mergeAtomProps, mergeNodeProps, mergeWidgetProps, mergeFormationProps, parseAtomFromProps, parseLayoutNodeFromProps, parseTextStyle, parseWrapper, parseMediaStyle, parseIconStyle, liftTextStyleShorthands, runPostOpsConstraints | ~300 |
| `ops_snapshot.go` | AtomOverrideSnapshot, FreestyleAtomSnapshot, FormationSnapshot, FormationOverrideSnapshot, SnapshotCustomizations, ApplySnapshot, ExpandWildcardOps, isTreeID, ResolveInsertedFieldValues + helper'ы | ~380 |

---

## Tier 3 — Средние сплиты (500-900 строк)

### 3A. engine/engine_v2.go (807 -> 2 файла)

| Файл | Содержимое | ~строк |
|---|---|---|
| `engine_v2.go` | EngineV2 struct, types, NewEngineV2, Execute, buildWidgets, AutoSelectPreset, BuildTemplateFormationV2 | ~450 |
| `engine_v2_overrides.go` | applyInstructionOverrides, applyPresetV2Fields, applyAtomOverrides, applyMediaIconOverrides, applyDefaultMediaStyle, applyWidgetContainerOverrides, mergeTextStyle, DisplayToTextStyleWrapper, applyHorizontalDirection | ~350 |

### 3B. tools/tool_visual_assembly.go (901 -> 3 файла)

| Файл | Содержимое | ~строк |
|---|---|---|
| `tool_visual_assembly.go` | Struct, constructor, Definition() (JSON schema), Execute() auto mode | ~445 |
| `tool_visual_assembly_ops.go` | executeOps(), executeBuild() | ~220 |
| `tool_visual_assembly_parse.go` | parseV2Input(), parseAtomOverride(), inferLegacyDisplayFromAtomV2(), convertToFormation() | ~240 |

### 3C. tools/tool_catalog_search.go (660 -> 2 файла)

| Файл | Содержимое | ~строк |
|---|---|---|
| `tool_catalog_search.go` | Struct, constructor, Definition(), Execute() | ~505 |
| `tool_catalog_search_rrf.go` | rrfMerge(), rrfMergeServices(), catalogExtractProductFields(), catalogExtractServiceFields() | ~155 |

### 3D. handlers/handler_debug.go (722 -> 2 файла)

После выноса engine-логики в usecase (Tier 1), оставшееся:

| Файл | Содержимое | ~строк |
|---|---|---|
| `handler_debug.go` | Types (PipelineMetrics, AgentMetrics, FormationInfo, MetricsStore), DebugHandler struct, handler methods (thin) | ~260 |
| `handler_debug_templates.go` | templateFuncs, listTemplate, detailTemplate (html/template блоки) | ~340 |

### 3E. handlers/handler_trace.go (644 -> 2 файла)

| Файл | Содержимое | ~строк |
|---|---|---|
| `handler_trace.go` | TraceHandler struct, HandleKillSession, HandleTraces, handleList, handleDetail | ~120 |
| `handler_trace_templates.go` | traceFuncs, traceListTpl, traceDetailTpl | ~520 |

---

## Tier 4 — Отложено

Эти файлы >400 строк, но терпимы:

| Файл | Строк | Почему отложено |
|---|---|---|
| `adapters/anthropic/anthropic_client.go` | 648 | LLM client монолитен по природе (request/response/retry/cache) |
| `adapters/postgres/postgres_state.go` | 620 | 19 методов, но все простые SQL, низкая когнитивная сложность |
| `engine/rules.go` | 563 | Хорошо структурирован секциями внутри файла |
| `usecases/pipeline_execute.go` | 516 | Единый coherent pipeline, разбивать неестественно |
| `engine/defaults.go` | 415 | Много мелких функций, но все по одной теме |
| `usecases/agent2_execute.go` | 404 | Один coherent execution flow |

---

## Порядок выполнения

Каждый шаг — отдельный коммит. Зависимости отражены порядком.

| Шаг | Что | Blast radius | Верификация |
|---|---|---|---|
| 1 | LogPort (нарушение 1) | 4 файла + 1 новый | `go build`, любой endpoint логирует |
| 2 | Testbench usecase + fix debug + testbench handlers (нарушения 2-3) | 3 файла + 1 новый | `go build`, `POST /api/v1/testbench`, `POST /debug/seed` |
| 3 | Split postgres_catalog.go | 1 файл -> 4 | `go build` (same package, zero risk) |
| 4 | Split ops.go | 1 файл -> 4 | `go build` |
| 5 | Split engine_v2.go | 1 файл -> 2 | `go build` |
| 6 | Split tool_visual_assembly.go | 1 файл -> 3 | `go build` |
| 7 | Split tool_catalog_search.go | 1 файл -> 2 | `go build` |
| 8 | Split handler_debug.go + templates | 1 файл -> 2 | `go build` |
| 9 | Split handler_trace.go + templates | 1 файл -> 2 | `go build` |

---

## Ожидаемый результат

| Метрика | До | После |
|---|---|---|
| Файлов >1000 строк | 2 | 0 |
| Файлов >500 строк | 9 | ~3 (pipeline, anthropic, state — Tier 4) |
| Архитектурных нарушений | 3 | 0 |
| Максимальный файл | 1456 строк | ~505 строк |
| Новых port-интерфейсов | — | 1 (LogPort) |
| Новых usecase | — | 1 (TestbenchExecuteUseCase) |

Поведение системы не меняется. Чистый рефакторинг.
