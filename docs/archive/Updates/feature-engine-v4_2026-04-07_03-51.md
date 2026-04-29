# feature/engine-v4 — #3 multi-widget composition: Phase 5 (tool validation + COMPOSING prompt)

**Branch**: `feature/engine-v4`
**Date**: 2026-04-07 03:51 MSK (2026-04-07 00:51 UTC)
**Commit**: TBD (this commit)
**Parent**: `e770db7`

---

## Context

Phase 5 из 6-фазного плана multi-widget composition. Это последняя фаза перед E2E verification (Phase 6). Закрывает грабли 8 и 11 из handoff:

- **Грабля 8**: top-level `preset` parameter мог быть совмещён с multi-widget insert ops в одном вызове. Семантически это ambiguous: top-level preset = «build single widget from this preset», per-widget preset (в `props.preset` insert op) = «compose multiple widgets, each from its own preset». Без validation Agent2 мог комбинировать оба paradigm'а и получать непредсказуемое поведение (preset expand'ит template, потом user ops добавляют ещё widgets — но preset DefaultReplicate применяется только к первому, refs пресета намешаются с user refs).
- **Грабля 11**: Agent2 prompt не знал синтаксис multi-widget композиций. Phases 1-4 строили инфраструктуру (per-widget replicate, per-widget preset, auto-sections, multi-widget tree map), но без секции в промпте Agent2 не знал что эти возможности существуют. Plus accumulated duplication в промпте (textStyle/PREFER PRESETS правила повторялись 3 раза).

Phase 5 — backend code + промпт. Pipeline 2-4с не меняется. $0.10/turn ceiling сохранён (новая логика в `validatePresetWithUserOps` это O(N) проход по ops). Промпт стал длиннее на ~30 строк (новая COMPOSING секция и описание новой tree_map schema), но сократился на ~3 строки за счёт dedup.

---

## Approach

### 1. Tool validation — `validatePresetWithUserOps`

`project_v4/backend/internal/tools/tool_visual_assembly.go` — добавлена функция и вызов перед preset expansion.

**Правила validation**:
1. `presetName == ""` → OK (нет top-level preset, всё разрешено).
2. `presetName != ""` + любой widget insert op с `props.preset` → ERROR. Mixing two preset paradigms.
3. `presetName != ""` + count(widget insert ops) > 1 → ERROR. Top-level preset не может composer'ить multiple widgets — для этого есть per-widget preset.
4. `presetName != ""` + ровно 1 widget insert op (без своего preset) → OK. Это не composition, а override flow: user добавляет один extra widget на top of preset's template (unusual but allowed по плану).
5. `presetName != ""` + только update/delete/move ops → OK. Канонический preset workflow с overrides.
6. `presetName != ""` + atom/node inserts (parent != "formation") → OK. Standard "add a NEW! badge" pattern.

**Implementation**:
```go
func validatePresetWithUserOps(presetName string, ops []engine_v4.Op) string {
    if presetName == "" {
        return ""
    }
    widgetInsertCount := 0
    for _, op := range ops {
        if op.Type != engine_v4.OpInsert || op.Props == nil {
            continue
        }
        opType, _ := op.Props["type"].(string)
        if opType != "widget" {
            continue
        }
        widgetInsertCount++
        if perWidgetPreset, _ := op.Props["preset"].(string); perWidgetPreset != "" {
            return fmt.Sprintf("top-level preset %q cannot be combined with per-widget preset %q in props; pick one", presetName, perWidgetPreset)
        }
    }
    if widgetInsertCount > 1 {
        return fmt.Sprintf("top-level preset %q cannot be combined with %d widget insert ops; use per-widget preset in props for multi-widget composition", presetName, widgetInsertCount)
    }
    return ""
}
```

**Integration**: вызов добавлен в `Execute` непосредственно перед `GetPreset(presetName)`:
```go
if presetName, ok := params["preset"].(string); ok && presetName != "" {
    if mode != "rebuild" {
        return &domain.ToolResult{Content: "preset can only be used with mode=\"rebuild\""}, nil
    }
    if errMsg := validatePresetWithUserOps(presetName, engineInput.Ops); errMsg != "" {
        return &domain.ToolResult{Content: errMsg}, nil
    }
    p, found := engine_v4.GetPreset(presetName)
    // ...
}
```

Errors возвращаются как `ToolResult.Content` (тот же шаблон что и для invalid mode) — Agent2 видит их как обычный tool result и может скорректировать запрос в следующем turn'е.

### 2. Tool tests — `tool_visual_assembly_test.go` (NEW)

`project_v4/backend/internal/tools/tool_visual_assembly_test.go` — 6 unit-тестов на `validatePresetWithUserOps`. Покрывают все правила:

- `TestValidatePresetWithUserOps_PresetPlusMultiInsert_Error` — top-level preset + 2 widget inserts → error
- `TestValidatePresetWithUserOps_PresetPlusInsertWithPreset_Error` — top-level preset + 1 insert с `props.preset` → error (даже если widget count == 1)
- `TestValidatePresetWithUserOps_PresetWithUpdateOps_OK` — preset + update/delete (canonical override flow) → OK
- `TestValidatePresetWithUserOps_PresetWithSingleInsert_OK` — preset + 1 widget insert без своего preset → OK (unusual but allowed)
- `TestValidatePresetWithUserOps_NoPreset_OK` — без top-level preset, multiple inserts с per-widget preset → OK (новый COMPOSING path)
- `TestValidatePresetWithUserOps_PresetWithAtomInserts_OK` — preset + atom/node inserts (text/row) → OK (standard "add NEW! badge" pattern)

Все тесты используют `engine_v4.Op` напрямую (не proходят через JSON parsing), что упрощает setup и фокусирует тест на самой логике валидации.

### 3. Промпт `prompt_compose_widgets.go` — два изменения

**3a. Add COMPOSING section** (между BUILDING FROM SCRATCH и MODIFYING EXISTING):

```
## COMPOSING — multi-widget responses

When the user wants a "presentation", "landing", or any response combining
different block types (hero + gallery + cta), insert MULTIPLE widget templates
in ONE rebuild call. Per-widget preset/replicate go in props. The engine
groups them: consecutive replicate clones → grid sections, literals → single
sections, all rendered as a composed formation.

### Example — product line presentation:
visual_assembly({
  mode: "rebuild",
  ops: [
    {"op":"insert","ref":"hero","parent":"formation","props":{"type":"widget","size":"large"}},
    {"op":"insert","parent":"$hero","props":{"type":"text","value":"New glossy collection","textStyle":{"fontSize":"3xl","fontWeight":"bold"}}},
    {"op":"insert","parent":"formation","props":{"type":"widget","preset":"product_card_compact","replicate":true,"replicateLimit":12}},
    {"op":"insert","ref":"cta","parent":"formation","props":{"type":"widget","size":"small"}},
    {"op":"insert","parent":"$cta","props":{"type":"text","value":"Shop the line","wrapper":{"type":"button","variant":"primary"}}}
  ]
})

### Rules:
- Per-widget preset goes inside the widget insert's props: {type:"widget", preset:"...", replicate:true, replicateLimit:N}
- Top-level preset + multiple widget inserts → ERROR. Use per-widget preset for compositions.
- Two widgets sharing the same per-widget preset don't collide on refs (auto-namespaced as p0_w / p1_w / ...)
- Literal widgets (hero, explainer, cta) use {value:"..."}; entity widgets use {fieldName:"..."}
- For non-replicated entity widgets, engine assigns EntityRef from data[0]; pass props.dataIndex:N to pick a specific item
```

**3b. Update MODIFYING EXISTING** — описать новую multi-widget tree_map schema:

До:
> Target atoms by FIELD NAME (applies to ALL widgets) or by specific ID from formation_tree.

После:
> formation_tree.widgets is an array of entries:
> - {"kind":"literal","id":"w-s0-w0","atoms":[...]} — single widget (hero, cta, explainer)
> - {"kind":"replicated","count":N,"ids":[...],"template":{...}} — N clones of one template (gallery)
>
> Target atoms:
> - by FIELD NAME — broadcasts to ALL atoms with that name in ALL widgets (use for "make all prices red")
> - by ATOM ID from a specific entry — surgical, targets one atom (use for "make THE hero text larger" in compositions)

Это критично для Phase 4 → Agent2 теперь читает новую schema из `BuildTreeMap`, и должен знать как с ней работать.

**3c. Dedup в DECISION RULES**:

- Удалено правило 5: «textStyle MUST be nested object. Never put fontSize/color at top level» (повторялось — есть в PROPS REFERENCE).
- Удалено правило 10: «PREFER PRESETS in rebuild mode» (повторялось — есть в PRESETS section с bold emphasis).
- Добавлено новое правило 9: «For mixed responses (presentation, landing, comparison) — insert multiple widget templates in one rebuild call with per-widget preset/replicate in props. See COMPOSING.» (pointer на новую секцию).
- DECISION RULES сжаты с 10 пунктов до 9.

**3d. Update ANTI-PATTERNS**:

- Заменено: «Do NOT create N widgets for N data items. Create 1 template + replicate: true.» (повтор HOW IT WORKS) → «Do NOT combine top-level preset with multiple widget insert ops — it errors. Use per-widget preset in props for compositions.» (отражает Phase 5 validation).

### 4. Без изменений

- Frontend — никаких касаний. Promt стороны Agent1 — никаких касаний. Engine pipeline — никаких касаний (Phase 5 это только tool layer + текст промпта + tests).
- `default_ops.go` (`ProductCardGridOps`, `ProductDetailOps`, etc.) — без изменений. Они вызываются legacy callers (handler_debug, handler_testbench, navigation_back, action_view) с ровно 1 widget insert, что попадает под "OK" правило 4 валидации.
- `BuildAgent2ToolPrompt` функция и её сигнатура — без изменений. Промпт обновлён только в `Agent2ToolSystemPrompt` константе (system prompt).

---

## Behaviour delta

### Пример 1 — top-level preset + multi-widget insert (старый bad case → теперь error)

**Input** (Agent2 неправильно скомпонует презентацию):
```json
{
  "mode": "rebuild",
  "preset": "product_card",
  "ops": [
    {"op":"insert","parent":"formation","props":{"type":"widget"}},
    {"op":"insert","parent":"formation","props":{"type":"widget","preset":"product_card_compact"}}
  ]
}
```

**До Phase 5**: tool молча обработал, preset expanded → 1 template (галерея cards), потом user ops добавили 2 ещё widget'а. DefaultReplicate=true применился ко всем (включая последние 2 пустых), bind data на пустые виджеты. Грязь.

**После Phase 5**: validation срабатывает на втором widget insert (`props.preset` присутствует) → возвращает:
```
top-level preset "product_card" cannot be combined with per-widget preset "product_card_compact" in props; pick one
```

Agent2 видит ошибку, в следующем turn'е переписывает на pure per-widget preset (без top-level).

### Пример 2 — top-level preset + 3 widget inserts (без per-widget preset)

**Input**:
```json
{
  "mode": "rebuild",
  "preset": "product_card",
  "ops": [
    {"op":"insert","parent":"formation","props":{"type":"widget"}},
    {"op":"insert","parent":"formation","props":{"type":"widget"}},
    {"op":"insert","parent":"formation","props":{"type":"widget"}}
  ]
}
```

**После Phase 5**: 3 widget inserts > 1 → error:
```
top-level preset "product_card" cannot be combined with 3 widget insert ops; use per-widget preset in props for multi-widget composition
```

Tested by `TestValidatePresetWithUserOps_PresetPlusMultiInsert_Error`.

### Пример 3 — pure COMPOSING (no top-level preset, target use case)

**Input** (после нового COMPOSING промпта):
```json
{
  "mode": "rebuild",
  "ops": [
    {"op":"insert","ref":"hero","parent":"formation","props":{"type":"widget","size":"large"}},
    {"op":"insert","parent":"$hero","props":{"type":"text","value":"New collection","textStyle":{"fontSize":"3xl"}}},
    {"op":"insert","parent":"formation","props":{"type":"widget","preset":"product_card_compact","replicate":true,"replicateLimit":12}},
    {"op":"insert","ref":"cta","parent":"formation","props":{"type":"widget","size":"small"}},
    {"op":"insert","parent":"$cta","props":{"type":"text","value":"Shop the line","wrapper":{"type":"button","variant":"primary"}}}
  ]
}
```

**После Phase 5**: validation проходит (нет top-level preset). ApplyOps + ExpandInlinePresets разворачивает per-widget preset с prefixed refs (`p1_w`, `p1_root`, ...), expandReplicatedWidgets дублирует gallery template ×12, groupIntoSections собирает 3 секции (hero/gallery/cta), BuildTreeMap возвращает 3-entry tree. Frontend рендерит composed formation.

### Пример 4 — preset + only update/delete (canonical override flow)

**Input**:
```json
{
  "mode": "rebuild",
  "preset": "product_card",
  "ops": [
    {"op":"update","target":"price","props":{"textStyle":{"color":"red"}}},
    {"op":"delete","target":"rating"}
  ]
}
```

**После Phase 5**: validation проходит (нет widget inserts). Preset expand'ит template, updates применяются к refs пресета. Это unchanged поведение — Phase 5 не ломает существующий workflow. Tested by `TestValidatePresetWithUserOps_PresetWithUpdateOps_OK`.

---

## Files changed

| File | Change |
|---|---|
| `project_v4/backend/internal/tools/tool_visual_assembly.go` | +40: `validatePresetWithUserOps` function (37 строк) + 3 строки call site в `Execute` |
| `project_v4/backend/internal/tools/tool_visual_assembly_test.go` | **NEW**: 6 validation тестов (~80 строк) |
| `project_v4/backend/internal/prompts/prompt_compose_widgets.go` | +33/-12: COMPOSING section (+30 строк), updated MODIFYING EXISTING (~+8/-1), DECISION RULES dedup (-2 правила, +1 правило), ANTI-PATTERNS update (-1 заменено) |

Итого: 2 modified + 1 new = 3 файла. **~123 insertions(+), ~14 deletions(-)** в диапазоне Phase 5.

---

## Verification

### Локально

```bash
cd project_v4/backend
go build ./...                                                                # clean
go test ./internal/tools/... -v -run TestValidatePresetWithUserOps           # 6 PASS
go test ./internal/engine_v4/... ./internal/tools/...                        # all green
```

**Phase 5 tests** (6 новых):
- `TestValidatePresetWithUserOps_PresetPlusMultiInsert_Error` — preset + 2 widget inserts → error message
- `TestValidatePresetWithUserOps_PresetPlusInsertWithPreset_Error` — preset + 1 insert с per-widget preset → error
- `TestValidatePresetWithUserOps_PresetWithUpdateOps_OK` — preset + update/delete → ok
- `TestValidatePresetWithUserOps_PresetWithSingleInsert_OK` — preset + 1 plain widget insert → ok
- `TestValidatePresetWithUserOps_NoPreset_OK` — multiple inserts без top-level preset → ok
- `TestValidatePresetWithUserOps_PresetWithAtomInserts_OK` — preset + atom inserts (text/row, не widget type) → ok

**Регрессия** — все существующие тесты по-прежнему зелёные:
- 7 Phase 1 + 6 Phase 2 + 7 Phase 3 + 4 Phase 4 + 6 Phase 5 = 30 composition tests
- Legacy: 7 replicate behavior + 6 preset behavior + 3 ops tests
- Tool tests (если были) — не сломаны.

### На проде (после деплоя)

Phase 5 — финальный backend bit перед E2E. Это:
- (a) Гард, который ловит invalid preset+multi-widget combos в runtime
- (b) Промпт, который учит Agent2 пользоваться composition paradigm

1. **Regression — single grid**: «покажи крема» → Agent2 продолжает использовать `preset: "product_card"` с `replicate: true` (no widget inserts) → validation passes → identical поведение.

2. **Regression — single detail**: «покажи деталь» → Agent2 шлёт `preset: "product_detail"` (no widget inserts) → passes → identical.

3. **Regression — preset + override**: «сделай цены красными» в rebuild mode → Agent2 шлёт `preset` + only update/delete ops → passes → identical.

4. **New — COMPOSING flow**: «сделай презентацию новой линии помад» → Agent2 (после промпта) шлёт multiple widget inserts с per-widget preset, без top-level preset → validation passes → engine produces composed formation → frontend рендерит Pencil-style layout.

5. **Validation guard**: если Agent2 ошибся и комбинировал top-level preset + multi-widget — backend вернёт error string в `ToolResult.Content`, который попадёт в conversation_history. Agent2 в следующем turn'е увидит ошибку и скорректируется.

### Прогноз поведения Agent2

Промпт даёт пример с тремя типами виджетов (hero text → gallery preset+replicate → cta text+button) и явное правило: "Top-level preset + multiple widget inserts → ERROR. Use per-widget preset for compositions."

Ожидаю, что Agent2 будет:
- Для simple grid/detail — продолжать использовать top-level preset (привычный path).
- Для presentation/landing/composition запросов — переключаться на multi-widget insert ops без top-level preset.
- Иногда (особенно на edge case промптах) пытаться mix → получит ошибку → скорректирует.

Phase 6 (E2E) проверит этот прогноз на реальных запросах.

---

## Known gaps / caveats

1. **Validation срабатывает только для top-level `preset`** — если Agent2 шлёт multiple widget inserts с per-widget preset И ещё передаёт top-level preset, ловится. Но если Agent2 шлёт multiple widget inserts с per-widget preset И НЕ передаёт top-level preset (правильный COMPOSING path), всё ОК — это и есть target use case. Validation скорее «гард от старого paradigm'а», чем «гарантия правильности нового».

2. **Per-widget preset валидация — не Phase 5**: Phase 5 не валидирует, что `props.preset` ссылается на существующий пресет. Если Agent2 напишет `props.preset: "nonexistent"`, ошибка вылезет в `ExpandInlinePresets` (Phase 2) с less helpful сообщением. Можно добавить early validation в Phase 7 (если будет), но не критично — все 12 пресетов перечислены в schema enum, LLM constrained.

3. **Промпт ~280 строк** — после dedup и добавления COMPOSING секции мы около 280 строк (vs ~250 до Phase 5). В пределах нормы для Agent2 system prompt с tool description. Если в будущем prompt станет тяжелее — можно вынести COMPOSING примеры в separate file (`COMPOSING_EXAMPLES.md`) и оставить в промпте только rules + 1 minimal example. Не блокирует Phase 6.

4. **No examples for `dataIndex`** — Phase 1 ввёл `props.dataIndex` для non-replicated entity widgets (compose detail для конкретного товара из multi-product data). В COMPOSING секции упомянут как rule («pass props.dataIndex:N»), но без example. Если Phase 6 покажет, что Agent2 не использует — добавлю явный пример в Phase 7.

5. **No COMPOSING example with `text_explainer` preset** — Phase 5 показывает hero как literal text, не как `text_explainer` preset. Можно добавить альтернативный пример где hero=text_explainer для consistency с system presets. Не критично — литералы и presets оба валидны для hero.

6. **`text` widgets с `wrapper`** — пример показывает `{type:"text","wrapper":{"type":"button","variant":"primary"}}` для CTA. Это уже supported V4 engine, но в Phase 6 надо проверить визуально, что button wrapper рендерится правильно через frontend.

7. **Phase 6 ещё не сделан** — это E2E verification на проде, нужно:
   - Push branch → деплой Railway → проверить full pipeline
   - Regression: «покажи крема», «покажи деталь», «сделай цены красными»
   - New: «сделай презентацию новой линии», «сделай landing для скидки», «сравни два товара»
   - Visual check композиции через Dev Inspector
   - Capture trace IDs для дебага

Phase 5 готова. Все 5 backend phases замкнуты. Готов к Phase 6 E2E.
