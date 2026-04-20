# feature/engine-v4 — #3 multi-widget composition: Phase 4 (BuildTreeMap multi-widget schema)

**Branch**: `feature/engine-v4`
**Date**: 2026-04-07 03:49 MSK (2026-04-07 00:49 UTC)
**Commit**: TBD (this commit)
**Parent**: `e61121c`

---

## Context

Phase 4 из 6-фазного плана multi-widget composition. Closes грабля 4 из handoff (`docs/New features/multi_widget_handoff_2026-04-07.md`): `BuildTreeMap` в `tree_ids.go` возвращал single template (`widget_template: {...}`), что для multi-widget композиций показывало только первый виджет — Agent2 в modify mode не видел остальных и не мог их таргетить.

Phase 1 заложил `ReplicateConfig.GroupID`, Phase 2 дал per-widget preset expansion, Phase 3 сгруппировал widgets в `formation.Sections[]` и установил `mode = composed`. После Phase 3 мы получили правильную rendered-структуру (frontend рисует Pencil-style layout), но контекст для следующего turn'а Agent2 был усечён до первого виджета. Без Phase 4 модификация типа "сделай заголовок hero красным" в композиции hero+gallery+cta была бы невозможна — Agent2 даже не знал бы, что после hero есть gallery и cta.

Решение: переписать `BuildTreeMap` на multi-widget schema, который возвращает массив `widgets[]` с двумя видами entries:

```json
{
  "widgets": [
    {"kind":"literal",    "id":"w-s0-w0", "atoms":[...], "nodes":[...]},
    {"kind":"replicated", "count":6, "ids":[...], "template":{...}}
  ],
  "widget_count": N,
  "mode": "composed",
  "grid": {...}
}
```

Group-by алгоритм идентичен `groupIntoSections` из Phase 3, чтобы tree map и rendered formation описывали одну и ту же логическую структуру (литералы по одному, replicate clones свёрнуты в один template + count + ids). Backwards compat: legacy single-grid/single-detail flows возвращают `widgets[]` length=1 (replicated entry для grid, literal для detail).

Phase 4 = backend-only, без LLM или фронта. Pipeline 2-4с не меняется (один проход по widgets). $0.10/turn ceiling сохранён.

---

## Approach

### 1. Переписать `BuildTreeMap` (`engine_v4/tree_ids.go:77-186`)

**До Phase 4**:
```go
func BuildTreeMap(formation) map[string]interface{} {
    var allWidgets []domain.Widget
    allWidgets = append(allWidgets, formation.Widgets...)
    for _, s := range formation.Sections {
        allWidgets = append(allWidgets, s.Widgets...)
    }
    if len(allWidgets) == 0 { return nil }
    template := buildWidgetMap(allWidgets[0])  // ← single template!
    return map[string]interface{}{
        "widget_template": template,
        "widget_count":    len(allWidgets),
        // ...
    }
}
```

**После Phase 4**:
- **Source widgets**: после Phase 3 источник истины зависит от пути:
  - `len(formation.Sections) > 0` → multi-section composed → итерируем `Sections[].Widgets[]`
  - иначе → flat formation (legacy / single-section rollback) → `formation.Widgets[]`
  - Старый код брал ОБА (`Widgets + Sections`), что могло задублировать виджеты после Phase 3 (sections rollback оставляет `Widgets` непустым). Новый код выбирает ровно один источник → no double-count.
- **Группировка**: одиночный pass по allWidgets, накопление в `groups []widgetGroup{gid, widgets[]}`:
  - Literal (no GroupID) → новая группа из одного виджета
  - Replicated widget с тем же GroupID что и предыдущий → append в open группу
  - Replicated widget с новым GroupID → новая группа
- **Сборка entries**:
  - Literal group → `{kind: "literal", ...buildWidgetMap(w)}` (сплющиваем поля widget map в entry — backwards compat для consumers, ожидающих `id/atoms/nodes` на верхнем уровне entry)
  - Replicated group → `{kind: "replicated", count: N, ids: [...], template: buildWidgetMap(g.widgets[0])}`
- **Top-level fields** остаются: `widgets_count`, `mode`, `grid` (если есть).
- **Удалено**: `widget_template` поле. Grep по `project_v4/backend/` и `project/backend/` подтвердил, что никто на него не ссылается — он использовался только в промпте Agent2 как prose, и тот теперь описывает новую schema (Phase 5).

### 2. Без изменений

- `buildWidgetMap` (`tree_ids.go:187-216`) — вспомогательная функция, формирующая `{id, atoms, nodes}` для одного виджета. Используется как для `kind: literal` (через сплющивание), так и для `kind: replicated` (как `template`).
- `collectLayoutNodes` (`tree_ids.go:218-236`) — без изменений.
- `StampTreeIDs` (`tree_ids.go:11-74`) — без изменений (Phase 3 уже подтвердил, что он работает и для flat и для sectioned formations).
- `BuildTreeMap` вызывается из `engine.go Step 8` после `StampTreeIDs` — порядок не меняется.

---

## Behaviour delta

### Пример 1 — legacy single grid ("show me creams", 6 cards)

**Input**:
```go
out := NewEngine().Execute(ExecuteInput{
    Ops:        ProductCardGridOps(),
    Data:       sampleProducts(6),
    EntityType: "product",
    Layout:     "grid",
    Columns:    3,
    Replicate:  true,
})
```

**Pipeline trace**:
- ApplyOps → 1 widget template
- expandReplicatedWidgets → 6 clones with `GroupID="rg-1"` (одна группа)
- groupIntoSections → 1 grid section (cols=3) → single-section rollback → `formation.Widgets = [c1..c6]`, `Sections = nil`
- StampTreeIDs → IDs `w-s0-w0..w-s0-w5`
- **BuildTreeMap** → группировка по GroupID:
  - 6 widgets с одинаковым `GroupID="rg-1"` → 1 group
  - Group → replicated entry
- **Output tree**:
  ```json
  {
    "widgets": [
      {
        "kind": "replicated",
        "count": 6,
        "ids": ["w-s0-w0", "w-s0-w1", "w-s0-w2", "w-s0-w3", "w-s0-w4", "w-s0-w5"],
        "template": {"id": "w-s0-w0", "atoms": [...], "nodes": [...]}
      }
    ],
    "widget_count": 6,
    "mode": "grid",
    "grid": {"cols": 3, "rows": 2}
  }
  ```

Tested by `TestBuildTreeMap_SingleGrid_Replicated`.

### Пример 2 — legacy single detail ("show me detail")

**Input**:
```go
out := NewEngine().Execute(ExecuteInput{
    Ops:        ProductDetailOps(),
    Data:       sampleProducts(1),
    EntityType: "product",
})
```

**Pipeline trace**:
- 1 widget без replicate (single detail) → `autoDetectEntityRefs` ставит `EntityRef` из data[0], НЕ ставит `ReplicateConfig`
- groupIntoSections → 1 single section → rollback → flat
- **BuildTreeMap** → 1 widget без `ReplicateConfig` → literal entry
- **Output tree**:
  ```json
  {
    "widgets": [
      {
        "kind": "literal",
        "id": "w-s0-w0",
        "atoms": [{"id":"a-...","type":"image","field":"images"}, ...]
      }
    ],
    "widget_count": 1,
    "mode": "single"
  }
  ```

Tested by `TestBuildTreeMap_SingleDetail_Literal`.

### Пример 3 — multi-widget composition (target use case)

**Input**:
```go
formation := &domain.FormationWithData{
    Widgets: []domain.Widget{
        makeLiteralWidget("Hero"),
        makeReplicateTemplate(),
        makeLiteralWidget("Buy now"),
    },
}
expandReplicatedWidgets(formation, sampleProductData(3), "product")
groupIntoSections(formation)
StampTreeIDs(formation)
tree := BuildTreeMap(formation)
```

**Pipeline trace**:
- expandReplicatedWidgets → `[hero, c1, c2, c3, cta]`, clones share `GroupID="rg-1"`
- groupIntoSections → 3 sections (single hero / grid×3 / single cta), `mode = composed`, `Widgets = nil`
- StampTreeIDs → `w-s0-w0` (hero), `w-s1-w0..w-s1-w2` (clones), `w-s2-w0` (cta)
- **BuildTreeMap** → итерация `Sections[].Widgets[]` (источник = sections, потому что они populated):
  - hero (no GroupID) → literal group
  - 3 clones (same GroupID) → replicated group
  - cta (no GroupID) → literal group
- **Output tree**:
  ```json
  {
    "widgets": [
      {"kind":"literal","id":"w-s0-w0","atoms":[...]},
      {"kind":"replicated","count":3,"ids":["w-s1-w0","w-s1-w1","w-s1-w2"],"template":{...}},
      {"kind":"literal","id":"w-s2-w0","atoms":[...]}
    ],
    "widget_count": 5,
    "mode": "composed"
  }
  ```

Agent2 в modify mode теперь видит все три entries. Может таргетить:
- `{"op":"update","target":"price","props":{...}}` → broadcast по fieldName, попадёт в gallery template (атомы с FieldName в hero/cta нет)
- `{"op":"update","target":"a-s0-w0-text","props":{...}}` → surgical, только hero text
- `{"op":"update","target":"a-s2-w0-text","props":{...}}` → surgical, только cta text

Tested by `TestBuildTreeMap_MultiWidgetComposition`.

### Пример 4 — empty formation

```go
BuildTreeMap(nil) → nil
BuildTreeMap(&domain.FormationWithData{}) → nil
```

Early return. Tested by `TestBuildTreeMap_EmptyFormation`.

---

## Files changed

| File | Change |
|---|---|
| `project_v4/backend/internal/engine_v4/tree_ids.go` | Rewrite `BuildTreeMap`: source-of-truth select (sections > flat), group-by `ReplicateConfig.GroupID`, output `widgets[]` array of literal/replicated entries; +25 docstring lines describing schema |
| `project_v4/backend/internal/engine_v4/composition_behavior_test.go` | +152: 4 Phase 4 tests (`TestBuildTreeMap_SingleGrid_Replicated`, `TestBuildTreeMap_SingleDetail_Literal`, `TestBuildTreeMap_MultiWidgetComposition`, `TestBuildTreeMap_EmptyFormation`) |

Итого: 2 modified, 0 new. **~239 insertions(+), ~14 deletions(-)** в диапазоне Phase 4 (без файлов Phase 5).

---

## Verification

### Локально

```bash
cd project_v4/backend
go build ./...                                                                # clean
go test ./internal/engine_v4/ -run TestBuildTreeMap                          # 4 PASS
go test ./internal/engine_v4/...                                              # all green
```

**Phase 4 tests** (4 новых):
- `TestBuildTreeMap_SingleGrid_Replicated` — full Engine.Execute pipeline для legacy "show me creams" → tree.widgets[0].kind=replicated count=6
- `TestBuildTreeMap_SingleDetail_Literal` — full pipeline для product_detail → tree.widgets[0].kind=literal
- `TestBuildTreeMap_MultiWidgetComposition` — composition hero+gallery×3+cta → tree.widgets length=3, IDs совпадают со stamped (`w-s0-w0`, `w-s1-w0..w-s1-w2`, `w-s2-w0`), widget_count=5, mode=composed
- `TestBuildTreeMap_EmptyFormation` — nil + empty → nil

**Регрессия** — все существующие тесты по-прежнему зелёные:
- 7 Phase 1 tests (composition_behavior_test.go)
- 6 Phase 2 tests (composition_behavior_test.go)
- 7 Phase 3 tests (composition_behavior_test.go) — в т.ч. `TestStampTreeIDs_Composed`, который dependency для Phase 4 ID assertions
- 7 legacy replicate behavior tests (replicate_behavior_test.go)
- 6 PresetBehavior tests (presets_behavior_test.go)
- 3 ref-chaining/replication tests (ops_test.go)

### На проде (после деплоя)

Phase 4 — изменение **формата контекста** для Agent2. Это не меняет рендеринг (Phase 3 + frontend уже умеют composed), но влияет на промпт следующего turn'а:

1. **Regression — pure rebuild flows**: «покажи крема» → grid рендерится как раньше; tree map теперь показывает 1 replicated entry с count=6 вместо single template. Никакой регрессии для пользователя — это меняется только в `state.Current.Template.formation_tree`, который Agent2 читает в следующем turn'е.

2. **Modify mode на single grid**: «сделай цены красными» — Agent2 получает tree.widgets=[{kind:"replicated"}], видит template.atoms содержит `{field: "price"}`, шлёт `{"op":"update","target":"price",...}`. Targeting по fieldName broadcast'ит на все клоны → identical поведение к Phase 3. ✅

3. **Modify mode на composition** (новый use case): hero + gallery + cta уже на экране, юзер пишет «сделай заголовок hero крупнее». Agent2 теперь видит tree.widgets[0]={kind:"literal",id:"w-s0-w0",atoms:[{id:"a-s0-w0-0",type:"text"}]}, может таргетить по `a-s0-w0-0` или по позиционной семантике hero. Без Phase 4 он бы увидел только template первого виджета (hero), но не знал бы про gallery и cta — теперь знает.

4. **Backwards compat для consumers `widget_template`**: grep по обоим backend'ам показал ноль ссылок. Frontend tree map не использует — он рендерит из `formation.sections[]`/`formation.widgets[]`, не из tree_map. Tree map — чисто LLM-context.

### Pencil parity check

Если открыть Pencil .pen с layout «hero / gallery / footer», его компонентная иерархия концептуально матчит наш `widgets[]` array. Multi-widget tree map даёт Agent2 ту же ментальную модель, что и дизайнер видит в Pencil canvas. Это направление #2 из vision (data-to-any-UI): tree map становится bridge между «как агент видит UI» и «как дизайнер видит UI».

---

## Known gaps / caveats

1. **Empty groups**: если `expandReplicatedWidgets` не нашёл данных (data nil), template дропается, в `formation.Widgets` остаются только литералы. groupIntoSections и BuildTreeMap отработают корректно — никаких пустых entries. Проверено через `TestExpandInPlace_EmptyDataDropsTemplate` (Phase 1).

2. **Replicate group без `GroupID`**: теоретически возможно, если код вне `expandReplicatedWidgets` создаст widget с `ReplicateConfig{Enabled:true, GroupID:""}`. Тогда `groupIntoSections` положит его в literal группу (no GroupID = literal). BuildTreeMap пометит его как `kind: literal`. Это semantic drift, но defensive — на сегодня нет caller'а, который это делает (`expandReplicatedWidgets` всегда устанавливает GroupID).

3. **`buildWidgetMap` для literal entry — flatten approach**: я сплющиваю `{id, atoms, nodes}` в entry, чтобы entry выглядел как `{kind:"literal", id:"...", atoms:[...]}`. Альтернатива — обернуть в `{kind:"literal", widget: {id, atoms, nodes}}`. Выбрал flatten из соображений компактности промпта (один уровень меньше) и читаемости для Agent2. Если в будущем потребуется добавить поля к literal entry (например `meta`, `entity_ref`), они тоже будут на верхнем уровне — нет коллизии с существующими ключами `kind`/`count`/`ids`/`template` (последние три только у replicated).

4. **`buildWidgetMap` НЕ возвращает `entityRef`**: тесты Phase 4 не проверяют entity reference в литералах. Phase 7 (если будет) может расширить `buildWidgetMap` чтобы Agent2 видел какой entity привязан к single product detail виджету. Сейчас Agent2 узнаёт entity из top-level state context (data_change/screen_state), не из tree_map.

5. **Ordering of `widgets[]`** — preserved from `allWidgets` order, который preserved от `groupIntoSections` (sections в порядке исходных виджетов), который preserved от `expandReplicatedWidgets` (in-place splice). Цепочка ordering invariants держится через все фазы. Тест `TestBuildTreeMap_MultiWidgetComposition` проверяет порядок entries.

6. **Phase 5 ещё не сделан** — tool validation и промпт COMPOSING. Без Phase 5 Agent2 не знает синтаксис multi-widget композиций, и вызовы будут падать на backend validation (top-level preset + multi-widget) или возвращать single widget вместо composition. Phase 5 идёт следующим коммитом.

Phase 4 готова. Следующая — Phase 5 (tool validation + COMPOSING prompt section).
