# feature/engine-v4 — #3 multi-widget composition: Phase 2 (per-widget preset)

**Branch**: `feature/engine-v4`
**Date**: 2026-04-07 03:00 MSK (2026-04-07 00:00 UTC)
**Commit**: `4ad4343`
**Parent**: `87dbc3e` (Phase 1 update log)

---

## Context

Phase 2 of the multi-widget composition rollout. Phase 1 (commit `01fcc15`) put the engine plumbing in place: `Widget.ReplicateConfig`, `expandReplicatedWidgets`, group-aware constraints, `autoDetectEntityRefs`. But there was no surface yet for Agent2 to actually express «insert two product cards with their own preset» — `insertWidget` only read `size` from props, and the only preset entry point was the legacy top-level `preset` parameter on the `visual_assembly` tool which expanded refs into a single global namespace and so could not safely run twice in one batch.

Phase 2 closes graby 5 (refs namespace global) и часть граблей 1/2 (per-widget replicate в ops).

Реальная цель: дать Agent2 синтаксис вида

```json
{"op":"insert","parent":"formation","props":{"type":"widget","preset":"product_card","replicate":true,"replicateLimit":6}}
```

…и чтобы он мог это сделать дважды (или вместе с literal hero/cta) в одном `ops` массиве — фундамент для презентационной композиции.

Phase 5 потом закроет prompt и tool validation; Phase 3 — auto-sections post-process. Phase 2 — чисто механика без UX surface на стороне tool/promprt.

---

## Approach

### `ExpandInlinePresets` — pre-pass в ApplyOps

`project_v4/backend/internal/engine_v4/presets.go`:

Новый helper. Walk через `[]Op`. Для каждого `OpInsert` с `props.preset = "<name>"`:

1. **Lookup preset** через `GetPreset(name)`. Unknown → warning + fall-through (op остаётся без `preset`, нормальный insertWidget → пустой виджет, не crash).
2. **Validate** что первый op пресета — `OpInsert` с `props.type = "widget"`. Если нет — warning + fall-through.
3. **Allocate per-widget prefix** `pN` из counter (`p0`, `p1`, `p2`...).
4. **Build rename map**:
   - Если у user op есть `op.Ref` (например `"card1"`) — первый ref пресета (`"w"`) переименовывается в это имя. Это позволяет user override ops продолжать использовать `$card1` для таргетинга widget root.
   - Если user op без `Ref` — генерируется `pN_w`.
   - Все остальные refs пресета (`root`, `info`, `meta`, `tags`...) — префиксуются `pN_root`, `pN_info` и т.д.
5. **Rewrite каждый op пресета** через rename map:
   - `op.Ref` → `renames[op.Ref]`
   - `op.Parent` если начинается с `$` → `$<renamed>` (через `renamePresetRef`)
   - `op.After` — то же самое
   - `op.Props` deep-copy через `deepCopyOpProps` (preset Build() возвращает статический slice — мутация props сломает следующие вызовы)
6. **Special-case первый op пресета** (widget insert):
   - `op.Parent` берётся из user op (обычно `"formation"`)
   - `op.After` — из user op
   - `op.Props` мерджатся (user wins on conflicts), `preset` исключается
   - Это значит user может override `size` из preset, передать `replicate/replicateLimit/dataIndex`, и эти поля попадут в первый widget insert

Output — `[]Op` готовый к обычному `ApplyOps` loop'у. Не-preset ops проходят без изменений.

### `ApplyOps` integration

```go
func ApplyOps(formation *FormationWithData, ops []Op) []string {
    // ...
    expandedOps, expandWarnings := ExpandInlinePresets(ops)
    ops = expandedOps
    warnings := append([]string(nil), expandWarnings...)
    // ... existing main loop ...
}
```

Pre-pass до построения idIndex — преимущество в том что warnings поднимаются на одном уровне.

### `insertWidget` — `readReplicateConfigFromProps`

`project_v4/backend/internal/engine_v4/ops.go`:

```go
func insertWidget(op Op, idx *idIndex, formation *FormationWithData) (string, string) {
    size := WidgetSizeMedium
    if v, ok := op.Props["size"].(string); ok && v != "" {
        size = WidgetSize(v)
    }
    w := Widget{Size: size, Layout: ..., Atoms: []Atom{}}
    if rcfg := readReplicateConfigFromProps(op.Props); rcfg != nil {
        w.ReplicateConfig = rcfg
    }
    // ... append + index
}
```

Helper `readReplicateConfigFromProps`:
- `props["replicate"].(bool)` → `rcfg.Enabled = true`
- `props["replicateLimit"]` (int / float64 — JSON unmarshal даёт float64) → `rcfg.Limit`
- `props["dataIndex"]` (int / float64) → `rcfg.DataIndex`
- Возвращает nil если ничего не задано — для literal widgets `ReplicateConfig` остаётся pointer-nil.

Поля видны только engine: `expandReplicatedWidgets` (Phase 1) уже обрабатывает `Enabled/Limit/GroupID`, `autoDetectEntityRefs` (Phase 1) — `DataIndex`. Connect just works.

### Что НЕ тронуто в Phase 2

- `tool_visual_assembly.go` — tool schema всё ещё знает только top-level `preset`. Agent2 формально не может прислать `props.preset`, пока Phase 5 не обновит schema/prompt. Но если бы прислал — engine это уже корректно обработал бы.
- `prompt_compose_widgets.go` — без изменений. Phase 5.
- Top-level `preset` параметр — продолжает работать как раньше (через `tool_visual_assembly.go` он concat'ится с user ops перед `ApplyOps`, и не использует `ExpandInlinePresets`). Backwards compat 100%.
- `tree_ids.go` `BuildTreeMap` — single template. Phase 4.
- `engine.go` post-process для sections — Phase 3.
- Validation top-level preset + multi-widget inserts — Phase 5.

---

## Behaviour delta

### Пример 1 — два инлайн product_card в одном batch

```go
ops := []Op{
    {Type: OpInsert, Ref: "card1", Parent: "formation",
     Props: {"type": "widget", "preset": "product_card"}},
    {Type: OpInsert, Ref: "card2", Parent: "formation",
     Props: {"type": "widget", "preset": "product_card"}},
}
ApplyOps(formation, ops)
```

**До Phase 2**: невозможно. Top-level `preset` обрабатывался в `tool_visual_assembly.go`, мог быть только один на batch, использовал глобальные refs `w/root/info/meta`. Два preset → коллизия refs.

**После**:
- ExpandInlinePresets generates 2 prefixes: `p0`, `p1`
- card1 → first preset op ref `w` substituted to `card1`, остальные refs → `p0_root`, `p0_info`, `p0_meta`
- card2 → first preset op ref `w` substituted to `card2`, остальные refs → `p1_root`, `p1_info`, `p1_meta`
- 2 widgets в `formation.Widgets`, оба с полным product_card layout (5 атомов, nested $root → image + $info → name + $meta → price + rating, brand badge)

### Пример 2 — user override op после inline preset

```go
ops := []Op{
    {Type: OpInsert, Ref: "myCard", Parent: "formation",
     Props: {"type": "widget", "preset": "product_card"}},
    // override op уважает $myCard
    {Type: OpInsert, Parent: "$myCard",
     Props: {"type": "text", "value": "BADGE"}},
}
```

**До**: невозможно (single inline preset не поддерживался).

**После**:
- Preset's first op `Ref: "w"` → `Ref: "myCard"`, parent остаётся `"formation"`
- Preset's второй op `Parent: "$w"` → `renames["w"]` → `"$myCard"`
- User override op без preset обрабатывается напрямую, парcится `$myCard` через global refs map (user's `card1` ref был зарегистрирован в первый pass'е applyInsert через `op.Ref`)
- Виджет получает 6 атомов: 5 preset + 1 literal "BADGE"

### Пример 3 — per-widget replicate в props

```go
ops := []Op{
    {Type: OpInsert, Parent: "formation",
     Props: {"type": "widget", "preset": "product_card",
             "replicate": true, "replicateLimit": 3}},
}
ApplyOps(formation, ops)
// formation.Widgets[0] — template с ReplicateConfig{Enabled: true, Limit: 3}

data := sampleProductData(5)
expandReplicatedWidgets(formation, data, "product")
// formation.Widgets — 3 клона (limit=3 < data=5)
```

**До**: только top-level `Replicate: true` — один на весь batch, single-widget сценарий.

**После**: per-widget replicate. Можно 2 разные replicate группы в одном batch'е (нужен Phase 5 чтобы Agent2 это попросил).

### Пример 4 — DataIndex

```go
ops := []Op{
    {Type: OpInsert, Parent: "formation",
     Props: {"type": "widget", "preset": "product_card", "dataIndex": 2}},
}
ApplyOps(formation, ops)
// ReplicateConfig.DataIndex = 2, Enabled = false

autoDetectEntityRefs(formation, data, "product")
// EntityRef = data[2], inline bind from data[2]
```

Use case: comparison widgets, где каждый виджет показывает конкретный entity из data slice.

### Пример 5 — composition presentation (Phase 1 + 2 вместе)

```go
ops := []Op{
    // Hero literal
    {Type: OpInsert, Ref: "hero", Parent: "formation",
     Props: {"type": "widget", "size": "large"}},
    {Type: OpInsert, Parent: "$hero",
     Props: {"type": "text", "value": "New collection"}},
    // Gallery via preset + replicate
    {Type: OpInsert, Parent: "formation",
     Props: {"type": "widget", "preset": "product_card", "replicate": true}},
    // CTA literal
    {Type: OpInsert, Ref: "cta", Parent: "formation",
     Props: {"type": "widget", "size": "small"}},
    {Type: OpInsert, Parent: "$cta",
     Props: {"type": "text", "value": "Buy now"}},
}
ApplyOps(formation, ops)         // 3 widgets: hero, gallery template, cta
expandReplicatedWidgets(...)     // 6 widgets: hero, c1..c4, cta
autoDetectEntityRefs(...)        // hero/cta untouched, clones already had EntityRef
BindData(...)                    // skipped on __bound clones, hero/cta literal text preserved
ApplyConstraints(...)            // group-aware: clones validated as one group, hero/cta skipped
```

Это уже почти то что Agent2 будет генерировать. Нужны Phase 3 (auto-sections для frontend rendering) и Phase 5 (tool/prompt) чтобы сделать это полным flow.

### Пример 6 — unknown preset

```go
ops := []Op{
    {Type: OpInsert, Parent: "formation",
     Props: {"type": "widget", "preset": "no_such_preset"}},
}
warnings := ApplyOps(formation, ops)
// warnings: ["op[0]: unknown preset \"no_such_preset\""]
// formation.Widgets — 1 пустой widget (fall-through через stripPresetProp)
```

Fail-soft: всё-таки создаём виджет, чтобы остальные ops в batch'е не сломались каскадно.

---

## Files changed

| File | Change |
|---|---|
| `project_v4/backend/internal/engine_v4/presets.go` | `ExpandInlinePresets` (per-widget prefix `pN_*`, user ref substitution для первого preset op), `renamePresetRef`, `deepCopyOpProps`, `stripPresetProp`, `fmt` import |
| `project_v4/backend/internal/engine_v4/ops.go` | `ApplyOps` pre-pass через `ExpandInlinePresets`, `insertWidget` читает `replicate/replicateLimit/dataIndex` через `readReplicateConfigFromProps` |
| `project_v4/backend/internal/engine_v4/composition_behavior_test.go` | 6 новых assertive тестов (TwoWidgetsNoCollision, UserRefSubstituted, PerWidgetReplicate, PerWidgetDataIndex, UnknownPresetWarns, PresetMixedWithLiterals) + helper `containsString` |

Итого: 3 файла, **538 insertions(+), 2 deletions(-)**.

---

## Verification

### Локально

```bash
cd project_v4/backend
go build ./...                                                # clean
go test ./internal/engine_v4/ -v -run TestOps                 # 6 new PASS
go test ./internal/engine_v4/ -v -run TestExpandInPlace       # 3 Phase 1 PASS
go test ./internal/engine_v4/ -v -run TestBindData            # 1 Phase 1 PASS
go test ./internal/engine_v4/ -v -run TestConstraints         # 1 Phase 1 PASS
go test ./internal/engine_v4/ -v -run TestEntityRef           # 2 Phase 1 PASS
go test ./internal/engine_v4/ -v -run TestPreset              # 6 legacy preset PASS
go test ./internal/engine_v4/ -v -run TestBehavior            # 7 legacy replicate PASS
go test ./internal/engine_v4/ -v -run TestRefChaining         # 2 ref-chain PASS
go test ./internal/engine_v4/ -v -run TestReplication         # 1 ref-chain PASS
go test ./internal/engine_v4/...                              # all green
```

Сводка: 13 composition тестов (7 Phase 1 + 6 Phase 2) + все 16 существующих (6 preset + 7 replicate + 3 ref-chain) — все зелёные.

### На проде (после деплоя)

Phase 2 — backend-only, Agent2 пока не знает про новый синтаксис (Phase 5). Поведение для legacy путей identical:

1. **Regression**: «покажи крема» (top-level preset=product_card + replicate=true) → product_card grid из 6 cards → визуально как раньше. Top-level preset proceeds через `tool_visual_assembly.go` без `ExpandInlinePresets` (он не запускается на этих ops).
2. **Regression**: top-level preset + override ops («поставь цену красной на каждом») → preset ops + user ops концатятся на уровне tool, ApplyOps получает их единым batch'ом. ExpandInlinePresets walks через них — preset's first op уже без `props.preset` (top-level path не вставляет это поле), так что pre-pass проходит без рерайтов. Существующие 5 preset behavior тестов это подтверждают.
3. **Regression**: «покажи детали первого» → product_detail → как раньше.
4. **Regression**: «сделай цены красными» (modify mode) → modify ops применяются на existing formation → как раньше. ExpandInlinePresets walks через update/delete ops — никаких preset, fall-through.
5. **Multi-widget composition** ещё недоступна Agent2 — нужно Phase 5 (`props.preset` в tool schema + COMPOSING секция в промпте).

---

## Known gaps / caveats

1. **Tool/prompt не знают про новый синтаксис**. Agent2 не отправит `props.preset` сейчас — всё работает только если конструировать ops вручную (как в тестах). Phase 5 закроет это: добавит `props.preset/replicate/replicateLimit/dataIndex` в tool schema + COMPOSING секцию в промпт + dedup существующих повторов чтобы не раздуть промпт.
2. **`BuildTreeMap` всё ещё single-template** (грабля 4). После expand multi-widget composition, Agent2 в modify mode увидит только первый виджет в context. Phase 4. Это будет важно если Agent2 в следующем turn'е попробует модифицировать конкретный widget композиции.
3. **Auto-sections отсутствует** (грабля 6 / новое решение #12). Phase 3. Frontend сейчас отрендерит flat `widgets[]` через единый CSS контейнер — gallery cards и literal hero окажутся в одной grid'е. Composition будет визуально сломана пока Phase 3 не сгруппирует widgets в `formation.sections[]`.
4. **Top-level preset + multi-widget inserts** не валидируется. Если Agent2 ошибочно пришлёт top-level `preset: "product_card"` + ещё inline preset insert → top-level expand'нется через `tool_visual_assembly.go`, inline expand'нется через `ApplyOps`, оба добавят виджеты, refs не коллизятся (потому что top-level использует raw `w/root`, inline — `p0_*`). Получится больше виджетов чем хотел user, без явной ошибки. Phase 5 добавит explicit validation: rebuild + top-level preset + >1 widget insert ops = error.
5. **`insertWidget` рекогнайзит `replicate/replicateLimit/dataIndex` даже без preset**. Это feature, не баг — user может задать literal widget с replicate flag (без preset) и engine его всё равно правильно реплицирует. Используется в тесте `TestExpandInPlace_LimitRespected` где template — голый widget с FieldName атомами но без preset.
6. **Inline preset support только для widget inserts**. Если в будущем появится preset для атома или layout node — нужно расширять. Пока не нужно.
7. **deepCopyOpProps тривиальный** — копирует только map[string]interface{} вглубь. Атомные slice'ы / другие сложные значения копируются по reference. На текущих preset definitions (productCardOps и др.) это OK потому что они не содержат вложенных slice'ов; если в будущем добавятся — нужно расширить deep copy.

Phase 2 готова. Следующий шаг — Phase 3 (auto-sections post-process для группировки `formation.widgets[]` → `formation.sections[]` в engine, frontend не трогаем). После Phase 3 уже будет визуальный effect композиции, можно прогнать E2E через testbench.
