# feature/engine-v4 — #3 multi-widget composition: Phase 3 (auto-sections post-process)

**Branch**: `feature/engine-v4`
**Date**: 2026-04-07 03:27 MSK (2026-04-07 00:27 UTC)
**Commit**: `3be7187`
**Parent**: `40f80e0`

---

## Context

Phase 3 из 6-фазного плана multi-widget composition (`docs/Updates/feature-engine-v4_2026-04-07_02-46.md` для Phase 1, `feature-engine-v4_2026-04-07_03-00.md` для Phase 2). Закрывает архитектурное решение #12 из обсуждения 2026-04-07: **auto-sections в engine post-process, frontend не трогаем**.

Phase 1 заложил инфраструктуру (per-widget `ReplicateConfig`, in-place expand, group-aware constraints). Phase 2 дал Agent2 возможность вставлять multiple widgets с per-widget пресетами через `props.preset` и `props.replicate`. Но между ними оставался пробел: после `expandReplicatedWidgets` мы получаем плоский `formation.Widgets` вида `[hero, c1, c2, c3, cta]`, а frontend `FormationRenderer` рендерит этот массив через **один** layout-контейнер. Hero + 3 cards + cta получили бы одинаковую CSS сетку — галерея сломалась бы.

Альтернативное решение из handoff #6 (новый mode `composed` + `.formation-multi` CSS + ветка в `getLayoutClass()`) стэкало бы всё вертикально и убивало бы grid у галереи. Решение #12 — backend post-process: движок группирует widgets по `ReplicateConfig.GroupID` в `formation.Sections[]`, каждая секция со своим mode (replicate group → grid, literal → single). Frontend `FormationRenderer` уже умеет рекурсивно рендерить секции (проверено в audit Phase 1), `.formation-composed` CSS уже есть. **Frontend diff = 0**.

Критическое требование — **backwards compat для legacy single-widget flows**. "Покажи кремы" (1 preset → 6 cards в одной replicate group) и "покажи деталь" (1 entity widget) сейчас работают через flat `formation.Widgets`. Phase 3 не должен ломать `TestReplicationPreservesLayout` (assertive: ожидает 3 widgets в `formation.Widgets`) и `TestPresetBehavior_*` (non-assertive, но любой рефакторинг рендера сломал бы их визуально на проде). Решение — **single-section rollback**: если grouping даёт ровно одну секцию → откат к flat formation, mode/grid берём из секции.

Phase 3 = backend-only, semantically backwards-compatible. Pipeline 2-4с не меняется (одна O(N) проходка), $0.10/turn ceiling сохранён (никаких extra LLM calls).

---

## Approach

### 1. `domain.FormationTypeComposed` enum

`project_v4/backend/internal/domain/formation_entity.go` — добавлен:

```go
FormationTypeComposed FormationType = "composed"
```

в существующий const блок `FormationType` (рядом с `single/grid/list/carousel/comparison/table`). Используется только в результате multi-section grouping. Frontend `Formation.css:130-148` уже определяет `.formation-composed { display: flex; flex-direction: column; gap: 24px }`, а `FormationRenderer.jsx:17-38` уже идёт по рекурсивному пути `if (sections?.length > 0)`. Никакого frontend кода не добавлено.

### 2. `groupIntoSections` post-process — новый файл `sections.go`

`project_v4/backend/internal/engine_v4/sections.go` (NEW, 113 строк):

- **Алгоритм** — single-pass O(N) по `formation.Widgets`:
  - Иду по виджетам в порядке исходного массива
  - `gid := widget.ReplicateConfig?.GroupID` (literals → "")
  - Если `gid == ""` (literal): `flush()` накопленный bucket (если есть открытая replicate group), потом `bucket = [w]`, потом сразу `flush()` — каждый literal становится своей single-секцией
  - Если `gid != ""` (replicate group widget):
    - Если `gid != currentGroup` → `flush()`, обновить `currentGroup`
    - `bucket = append(bucket, w)`
  - В конце — финальный `flush()`

- **`flush()`** — закрывает накопленный bucket в `FormationSection`:
  - Если первый виджет в bucket имеет `ReplicateConfig.GroupID != ""` → mode = `FormationTypeGrid`, `Grid.Cols = GridColumnsForCount(len(bucket))`
  - Иначе → mode = `FormationTypeSingle`
  - `sections = append(sections, sec); bucket = nil`

- **Single-section rollback**:
  ```go
  if len(sections) == 1 {
      sec := sections[0]
      if formation.Mode == "" { formation.Mode = sec.Mode }
      if formation.Grid == nil && sec.Grid != nil { formation.Grid = sec.Grid }
      formation.Widgets = sec.Widgets
      formation.Sections = nil
      return
  }
  ```
  — критично для backwards compat. `formation.Mode == ""` guard уважает `input.Layout` (если user явно передал `layout: "list"` для single template — не перетираем).

- **Multi-section composition**:
  ```go
  formation.Mode = domain.FormationTypeComposed
  formation.Widgets = nil
  formation.Sections = sections
  ```
  — sections становятся source of truth, flat array обнуляется. `StampTreeIDs` уже умеет идти и по `Sections[].Widgets[]`, и по плоскому `Widgets[]` (`tree_ids.go:17-33`), так что после Phase 3 stamp работает корректно для обоих путей.

- **Guards**:
  - `formation == nil || len(formation.Widgets) == 0` → return (early)
  - `len(formation.Sections) > 0` → return (don't double-group, если upstream уже наложил sections)

### 3. Engine pipeline integration

`project_v4/backend/internal/engine_v4/engine.go` — добавлен Step 6.5 между `ApplyConstraints` и `StampTreeIDs`:

```go
// Step 6: Apply constraints (ALL atoms, no bypass)
ApplyConstraints(formation)

// Step 6.5: Group widgets into sections by ReplicateConfig.GroupID.
// Multi-widget compositions become formation.Sections; single-widget flows
// stay flat (rollback). Frontend FormationRenderer handles both paths.
groupIntoSections(formation)

// Step 7: Stamp stable IDs
StampTreeIDs(formation)
```

Порядок важен: grouping должен пройти **после** constraints (так что `cross-widget` валидация работает на flat array, как в Phase 1) и **до** stamping (так что IDs `w-s{N}-w{M}` отражают финальную section structure). После Phase 3:

```
1. Init formation
2. Apply input.Layout/Columns/Size
3. ApplyOps (with ExpandInlinePresets pre-pass — Phase 2)
4. Limit data slice
4a. expandReplicatedWidgets — Phase 1
4b. autoDetectEntityRefs — Phase 1
4.5. Inject default actions
5. BindData
6. ApplyConstraints — group-aware (Phase 1)
6.5. groupIntoSections — Phase 3 ← NEW
7. StampTreeIDs (works for both flat and sections paths)
8. BuildTreeMap
```

### Что НЕ тронуто в Phase 3

- `BuildTreeMap` всё ещё single-template (грабля 4) — Phase 4. После grouping он берёт `allWidgets[0]` через сплющивание `formation.Widgets + formation.Sections[].Widgets`. Это работает (allWidgets[0] = первая секция, первый виджет), но для multi-widget composition в Agent2 modify mode он покажет только первый widget. Phase 4 это переписывает на multi-widget schema.
- `ApplyOps` — не трогаем (Phase 2 уже добавил `ExpandInlinePresets` pre-pass).
- Frontend — никаких изменений. Проверено через регрессию `TestReplicationPreservesLayout` (assertive).
- Tool validation (top-level preset + multi-widget combo) — Phase 5.
- Промпт `prompt_compose_widgets.go` — Phase 5.

---

## Behaviour delta

### Пример 1 — legacy "show me creams" (single replicate group)

```go
out := NewEngine().Execute(ExecuteInput{
    Ops:        ProductCardGridOps(),
    Data:       sampleProducts(5),
    EntityType: "product",
    Layout:     "grid",
    Columns:    3,
    Replicate:  true,
})
```

**До Phase 3**: ApplyOps → 1 widget template. Bridge → `ReplicateConfig.Enabled=true`. `expandReplicatedWidgets` → 5 clones with `GroupID="rg-1"`. Cross-constraints применяются. StampTreeIDs → flat IDs. **Output**: `formation.Mode = "grid", Widgets = [5 clones], Sections = nil`.

**После Phase 3**: то же самое, плюс `groupIntoSections` собирает 5 clones в 1 grid section (cols=3 from `GridColumnsForCount(5)`). Single-section → rollback: `formation.Mode = "grid"` (уже было), `formation.Grid` уже `{Cols: 3}` из `input.Columns` — guard `if formation.Grid == nil` пропускает; `formation.Widgets = sec.Widgets`. **Output идентичный**: flat formation as before. ✅ Регрессия покрыта `TestEngineExecute_LegacyReplicateGridStaysFlat` + `TestReplicationPreservesLayout` (legacy assertive).

### Пример 2 — legacy "show me detail" (single entity widget)

```go
out := NewEngine().Execute(ExecuteInput{
    Ops:        ProductDetailOps(),
    Data:       sampleProducts(1),
    EntityType: "product",
})
```

**До Phase 3**: 1 widget с EntityRef из data[0], flat formation. **После**: `groupIntoSections` собирает 1 literal section (no GroupID — autoDetectEntityRefs не ставит GroupID), rollback → flat. `formation.Mode == ""` (input.Layout не был передан) → ставится `single` из section. Identical поведение. ✅

### Пример 3 — multi-widget composition (target use case)

```go
formation := &domain.FormationWithData{
    Widgets: []domain.Widget{
        makeLiteralWidget("Hero"),       // literal
        makeReplicateTemplate(),         // replicate template
        makeLiteralWidget("Buy now"),    // literal
    },
}
expandReplicatedWidgets(formation, sampleProductData(3), "product")
groupIntoSections(formation)
```

**Output**:
- `formation.Mode = "composed"`
- `formation.Widgets = nil`
- `formation.Sections = [`
  - `{Mode: "single", Widgets: [hero]}`
  - `{Mode: "grid", Grid: {Cols: 2}, Widgets: [c1, c2, c3]}` — cols=2 из `GridColumnsForCount(3)`
  - `{Mode: "single", Widgets: [cta]}`
- `]`

Frontend `FormationRenderer` идёт `if (sections?.length > 0)` → `.formation-composed` контейнер (flex column gap:24px) → каждая секция рендерится через recursive call с `mode/grid/widgets` → hero как single, gallery как 2-col grid, cta как single. Pencil-style "presentation" работает.

### Пример 4 — литералы only ("3 explanation blocks")

```go
formation := &domain.FormationWithData{
    Widgets: []domain.Widget{
        makeLiteralWidget("Hero"),
        makeLiteralWidget("Explainer"),
        makeLiteralWidget("CTA"),
    },
}
groupIntoSections(formation)
```

**Output**: 3 single-mode sections, composed. Каждый literal в своей секции — иначе они слились бы в один контейнер, что неправильно для разнотипных blocks (hero ≠ explainer ≠ cta).

### Пример 5 — TestStampTreeIDs_Composed

После grouping, `StampTreeIDs(formation)` идёт по `formation.Sections[]`:
- Section 0 (hero): `widgets[0].ID = "w-s0-w0"`
- Section 1 (gallery): `widgets[0..2].ID = "w-s1-w0", "w-s1-w1", "w-s1-w2"`
- Section 2 (cta): `widgets[0].ID = "w-s2-w0"`

Stable IDs позволяют Agent2 в modify mode таргетить конкретные виджеты по секциям. Phase 4 расширит `BuildTreeMap` чтобы Agent2 видел эту структуру.

### Пример 6 — order preservation

`[first, gallery_template, middle]` + `expandReplicatedWidgets(data=2)` + `groupIntoSections` →
- Section 0: literal "first"
- Section 1: gallery (2 clones, grid cols=2)
- Section 2: literal "middle"

Порядок source widgets сохранён. Tested в `TestGroupIntoSections_OrderPreserved`.

---

## Files changed

| File | Change |
|---|---|
| `project_v4/backend/internal/domain/formation_entity.go` | +1: `FormationTypeComposed FormationType = "composed"` |
| `project_v4/backend/internal/engine_v4/sections.go` | **NEW** — `groupIntoSections` (113 строк) |
| `project_v4/backend/internal/engine_v4/engine.go` | +5: Step 6.5 — `groupIntoSections(formation)` between Step 6 (ApplyConstraints) и Step 7 (StampTreeIDs) |
| `project_v4/backend/internal/engine_v4/composition_behavior_test.go` | +260: 7 новых тестов + helper `literalValue` + `fmt` import |

Итого: 3 modified + 1 new = 4 файла. **374 insertions(+), 0 deletions(-)**.

---

## Verification

### Локально

```bash
cd project_v4/backend
go build ./...                                                                # clean
go test ./internal/engine_v4/ -v -run TestGroupIntoSections                  # 5 PASS
go test ./internal/engine_v4/ -v -run TestStampTreeIDs_Composed              # 1 PASS
go test ./internal/engine_v4/ -v -run TestEngineExecute_LegacyReplicateGridStaysFlat  # 1 PASS
go test ./internal/engine_v4/...                                              # all green
```

**Phase 3 tests** (7 новых):
- `TestGroupIntoSections_SingleGroup_Flat` — 6 cards → 1 grid section → rollback → flat formation, mode=grid, cols=3
- `TestGroupIntoSections_LiteralsOnly_Composed` — 3 literals → 3 single-sections → mode=composed
- `TestGroupIntoSections_MixedComposition` — hero + 3-card gallery + cta → 3 sections (single/grid/single), gallery cols=2
- `TestGroupIntoSections_OrderPreserved` — `[first, gallery, middle]` → секции в том же порядке
- `TestGroupIntoSections_SingleEntityDetail_Flat` — 1 entity widget → 1 single section → rollback → flat, mode=single
- `TestStampTreeIDs_Composed` — после grouping, IDs в формате `w-s{N}-w{M}` для всех секций
- `TestEngineExecute_LegacyReplicateGridStaysFlat` — full Engine.Execute pipeline для legacy "show me creams" → flat formation сохраняется

**Регрессия** — все существующие тесты по-прежнему зелёные:
- 7 Phase 1 tests (composition_behavior_test.go)
- 6 Phase 2 tests (composition_behavior_test.go)
- 7 legacy replicate behavior tests (replicate_behavior_test.go) — non-assertive, наблюдаем через `t.Logf`. `TestBehavior_NoReplicate_MultiWidget` теперь логгирует `widgets=0 sections=2` вместо `widgets=2 sections=0` — поведение изменилось как и ожидалось (multi-literal → composed), но тест passes (нет assertions).
- 6 PresetBehavior tests (presets_behavior_test.go) — non-assertive. Все пресеты builds + executes; `t.Logf` выводы отражают новую rollback семантику.
- 3 ref-chaining/replication tests (ops_test.go) — `TestReplicationPreservesLayout` критически assertive (`expected 3 widgets after replication, got %d`) → проверено, rollback сохраняет flat array. ✅

### На проде (после деплоя)

Phase 3 — backend-only, semantically backwards-compatible. Поведение для legacy путей identical:

1. **Regression**: «покажи крема» → 6 product_card в grid → визуально как раньше (rollback path).
2. **Regression**: «покажи детали первого» → product_detail single → визуально как раньше (rollback).
3. **Regression**: modify mode на existing formation — modify ops применяются, sections grouping не трогает уже sectioned formations (`if len(formation.Sections) > 0 → return`).
4. **Multi-widget composition** теперь физически возможна через `Engine.Execute()`, но Agent2 пока не знает как её попросить через промпт — это Phase 5. Можно вручную через testbench с явными composed ops:
   ```json
   {"ops": [
     {"op":"insert","ref":"hero","parent":"formation","props":{"type":"widget"}},
     {"op":"insert","parent":"$hero","props":{"type":"text","value":"New collection"}},
     {"op":"insert","parent":"formation","props":{"type":"widget","preset":"product_card","replicate":true}},
     {"op":"insert","ref":"cta","parent":"formation","props":{"type":"widget"}},
     {"op":"insert","parent":"$cta","props":{"type":"text","value":"Buy now"}}
   ], "data": [...]}
   ```
   → engine produces `{mode: "composed", sections: [hero, gallery, cta]}` → frontend рендерит Pencil-like layout.

---

## Known gaps / caveats

1. **Phase 3 = backend-only механика без UX surface** — Agent2 ещё не знает синтаксис COMPOSING. Промпт обновляется в Phase 5. До тех пор multi-widget composition доступна только через ручной tool call (testbench) или programmatic ExecuteInput.

2. **`BuildTreeMap` всё ещё single-template** (грабля 4 из handoff) — после Phase 3 он берёт `allWidgets[0]` через сплющивание sections + flat. Технически работает (показывает первый widget композиции), но Agent2 в modify mode не видит остальные виджеты. Phase 4 переписывает schema на массив templates с группировкой literal/replicated.

3. **Single-section rollback может неожиданно изменить `formation.Mode`** — если upstream код вызвал `Engine.Execute()` без `input.Layout` (т.е. `formation.Mode == ""`) и data привела к одному widget, mode будет установлен в `single` (для literal) или `grid` (для replicate group). Это правильно, но если до Phase 3 какой-то caller рассчитывал на пустой `Mode` — сейчас он его не получит. Не нашёл таких callers (все usecases передают `input.Layout` явно или используют preset wrappers, которые в свою очередь работают с конкретными режимами).

4. **Empty bucket после filter** — если все виджеты после `expandReplicatedWidgets` исчезли (например `data == nil` для replicate template → drop), `formation.Widgets` пустой → groupIntoSections early returns. Sections не создаются, formation остаётся пустым. Это правильное поведение (нет данных = нет UI), но frontend должен корректно рендерить empty formation. Не тестировано в Phase 3 — предполагается, что existing code path для empty result уже работает (см. `TestExpandInPlace_EmptyDataDropsTemplate` в Phase 1).

5. **Cross-widget constraints**: они применяются в Step 6 (до grouping), на flat array. Это правильно — group-aware constraints из Phase 1 уже учитывают `ReplicateConfig.GroupID` и валидируют каждую группу отдельно. Grouping в Step 6.5 — чисто render-time post-process, без новых constraint passes.

6. **`describeFormation` в legacy тестах** теперь логгирует `widgets=0 sections=N` для multi-widget cases, что может сбивать с толку при чтении test output. Это не bug — legacy тесты non-assertive, они описывают наблюдаемое состояние. Можно расширить `describeFormation` чтобы он итерировал `formation.Sections[]` для лучшего вывода — но это касание legacy кода ради красоты, отложу до отдельной cleanup сессии.

7. **Update log time** — этот файл датирован MSK (UTC+3), commit Author Date `Tue Apr 7 03:27:xx 2026 +0300`.

Phase 3 готова. Следующая — Phase 4 (multi-widget tree map schema для Agent2 modify mode context).
