# feature/engine-v4 — Ops-Only Pencil Engine

**Branch**: `feature/engine-v4` (deploy branch: `feature/engine-v4-pencil`)
**Date**: 2026-04-03 — 2026-04-04
**Commit**: `9ddb19a feat: ops-only Pencil engine — remove presets, add widget insert + replication`

---

## Summary

Полная перестройка V4 движка: пресеты удалены, ops (insert/update/delete/move) — единственный механизм построения UI. Agent2 строит ОДИН шаблон виджета через ops, движок копирует его на N элементов данных, data binding заполняет значения.

Ментальная модель: ops делают виджет → виджет становится шаблоном → повторяется N раз по количеству товаров → сохраняется в state → при следующем запросе Agent2 модифицирует существующий через update/delete/insert, а не строит заново.

---

## What was implemented

### 1. ops.go — Widget insert
- `insertWidget()` — создание виджета через ops: `{op:"insert", parent:"formation", props:{type:"widget", size:"medium"}}`
- Widget создаётся с root layout node (column) и пустым списком атомов
- Ref chaining: `ref:"w"` → `parent:"$w"` для вложенных элементов
- Pending ID (`__pending_widget_N`) регистрируется в индексе для последующих ops

### 2. engine.go — Пайплайн без пресетов + репликация
Новый пайплайн:
1. Existing formation OR empty `{}`
2. Apply formation-level settings (layout, columns, size)
3. Apply ops
4. **Replicate** — 1 widget template + N data items → N deep-copied widgets (JSON roundtrip)
5. Bind data (FieldName → Value)
6. Apply constraints
7. Stamp tree IDs
8. Build compact tree map

- Удалён `presets map[string]PresetFunc` из Engine struct
- Удалён `GetPresetNames()`
- Добавлены `replicateWidgets()` и `deepCopyWidget()` (JSON roundtrip для deep copy)
- Правило репликации: `len(widgets)==1 && len(data)>1 && len(sections)==0`

### 3. types.go — Убраны пресеты из типов
- Удалён `Preset string` из `ExecuteInput`
- Удалён `type PresetFunc func(...)`

### 4. presets.go — УДАЛЁН (358 LOC)
Весь файл с preset-функциями удалён.

### 5. constraints.go — Убраны domain-specific правила
Удалены:
- D5: slot-based truncation (domain-specific)
- W1: max 2 badges (arbitrary limit)
- W2: max 5 tags (arbitrary limit)

Оставлены generic правила:
- A1: badge >20 chars → tag
- A2: tag >40 chars → unwrap
- W8: tiny widget → remove images
- C1: field <70% coverage → remove
- C3: same field → same format

### 6. default_ops.go — Helper ops для fallback
- `ProductCardGridOps() []Op` — 9 ops строящих карточку товара (widget → column → image + info → name + price + rating + brand)
- `ProductDetailOps() []Op` — 13 ops строящих детальный просмотр
- `GridColumnsForCount(count int) int` — автоподбор колонок (1→1, 2→2, 3+→3)

### 7. tree_ids.go — Штамповка widget IDs
- Widgets получают ID: `w-s0-w0`, `w-s0-w1`, ...
- Pending IDs (`__pending_widget_*`) заменяются на стабильные

### 8. tool_visual_assembly.go — Убран preset из API
- Удалён `preset` property из InputSchema
- Добавлен `widget` в type description
- Убран preset handling из Execute
- Build-from-scratch detection: если первый op — widget insert, НЕ грузить existing formation
- Wildcard expansion для modification ops

### 9. prompt_compose_widgets.go — Полный переписан Agent2 промпт
Старый: "You are Agent 2 — a UI composition agent. Use preset for data display."
Новый: "You are Agent 2 — a UI builder. Build any UI using ops."

Ключевые секции:
- Widget template pattern: "Build ONE widget template. Engine clones for N data items."
- 3 примера build (product card grid, single detail, compact rows)
- Примеры modification (update, delete, insert, move)
- Decision rules: data_change + no tree → build; no change + tree → modify; data + tree → rebuild
- Anti-patterns: don't create N widgets, don't hardcode data values

### 10. 8 callers обновлены
Все вызовы `Preset: "product_card_grid"` / `Preset: "product_detail"` заменены на `Ops: engine_v4.ProductCardGridOps()` / `Ops: engine_v4.ProductDetailOps()`:
- `usecases/navigation_back.go` (×2)
- `usecases/navigation_expand.go`
- `usecases/action_view.go` (×2)
- `usecases/pipeline_execute.go` (×2)
- `handlers/handler_testbench.go`
- `handlers/handler_debug.go`

---

## Known issues

### DEPLOYMENT ISSUE (UNRESOLVED)
Railway сервис `fabulous-learning` настроен на:
- Branch: `feature/engine-v4-pencil`
- Root directory: `/project_v4`
- Коммит `9ddb19a` присутствует на remote ветке (подтверждено)
- Код в коммите содержит новый промпт (подтверждено через `git show`)

**Проблема**: после редеплоя трейсы показывают СТАРЫЙ системный промпт Agent2:
```
"You are Agent 2 — a UI composition agent. You decide HOW to display data."
```
Вместо нового:
```
"You are Agent 2 — a UI builder. You build and modify visual UI using ops."
```

**Причина неизвестна.** Проверено:
- Код на remote ветке правильный ✓
- Dockerfile корректный (copies `backend/`, builds `./cmd/server/`) ✓
- main.go wires `engine_v4.NewEngine()` и `Agent2ToolSystemPrompt` без env var switching ✓
- `agent2_execute.go:192` — промпт хардкодом, без переключателей ✓
- Env vars `ENGINE_VERSION`, `AGENT2_PROMPT_VERSION` — НЕ читаются кодом project_v4 (grep подтверждает) ✓

### Layout nesting bug (NOT FIXED)
`insertLayoutNode` в ops.go возвращает `__pending_node_{ptr}` но НЕ регистрирует этот ID в `idx.nodes`. Последующие ops с `parent:"$root"` (resolve → pending ID) не находят parent node, и атомы падают на root level вместо вложения в нужный layout node.

---

## Files changed

| Action | File | Change |
|--------|------|--------|
| EDIT | `engine_v4/ops.go` | +30 LOC (insertWidget) |
| REWRITE | `engine_v4/engine.go` | Remove presets, add replication (~60 LOC) |
| EDIT | `engine_v4/types.go` | -10 LOC (remove Preset/PresetFunc) |
| DELETE | `engine_v4/presets.go` | -358 LOC |
| EDIT | `engine_v4/constraints.go` | -50 LOC (remove D5, W1, W2) |
| CREATE | `engine_v4/default_ops.go` | +80 LOC (helper ops) |
| EDIT | `engine_v4/tree_ids.go` | +8 LOC (widget ID stamp) |
| EDIT | `tools/tool_visual_assembly.go` | ~30 LOC edits |
| EDIT | `usecases/navigation_back.go` | 4 lines |
| EDIT | `usecases/navigation_expand.go` | 2 lines |
| EDIT | `usecases/action_view.go` | 4 lines |
| EDIT | `usecases/pipeline_execute.go` | 4 lines |
| EDIT | `handlers/handler_testbench.go` | 8 lines |
| EDIT | `handlers/handler_debug.go` | 2 lines |
| REWRITE | `prompts/prompt_compose_widgets.go` | ~200 LOC (new Agent2 prompt) |

**Net**: ~300 LOC deleted, ~150 LOC added, ~200 LOC rewritten
