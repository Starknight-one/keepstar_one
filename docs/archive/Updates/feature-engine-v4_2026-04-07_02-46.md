# feature/engine-v4 — #3 multi-widget composition: Phase 1 (per-widget replicate)

**Branch**: `feature/engine-v4`
**Date**: 2026-04-07 02:46 MSK (2026-04-06 23:46 UTC)
**Commit**: `01fcc15`
**Parent**: `d3145e5`

---

## Context

Gap #3 из `docs/PRE_LAUNCH_TASKS.md` и `docs/Updates/feature-engine-v4_2026-04-04_04-07.md` — multi-widget composition. V4 engine сегодня умеет генерировать только «одну форму за turn»: либо N карточек одного пресета (replicate grid), либо один detail, либо один текст. Реальный продуктовый use-case — собрать «презентацию» из разных по форме блоков в одном ответе (hero + статы + галерея + explainer + CTA), потому что без этого чат не может «выдать всё нужное для принятия решения» = ядро продуктовой метафоры keepstar.

Handoff doc `docs/New features/multi_widget_handoff_2026-04-07.md` от того же дня предлагал короткий план — per-widget replicate flag + per-widget preset в props. При повторном глубоком чтении кода нашлись 6 критических граблей, которые в handoff не упоминались (см. memory `v4_multi_widget_analysis_2026_04_07.md`). По итогам обсуждения зафиксировано 11 архитектурных решений + 12-е «auto-sections в engine» (не frontend). Реализация разбита на 6 фаз — Phase 1 закрывает грабли 1, 2, 3, 7, 10:

1. **BindData позиционный** — `binding.go:14-19` маппил `widget[i] → data[i]`. В композиции literal+replicate данные сдвигаются.
2. **`replicateWidgets` затирает массив** — `engine.go:104-120` делал `formation.Widgets = widgets`, уничтожая literal виджеты.
3. **Cross-widget C1** — `constraints.go:84-101` удалял fields присутствующие в <70% виджетов; в композиции literals + 6 product cards threshold = 7.7, count = 6 → fields удалялись из cards. Композиция разваливалась тихо.
7. **EntityRef auto-detect** — single entity-bound widget без EntityRef → нужно гибридное решение (default data[0], override через DataIndex).
10. **Где хранить replicate/preset на виджете** — выделенное поле `Widget.ReplicateConfig *ReplicateConfig` с `json:"-"` (engine-internal).

Phase 1 = чистый backend-only diff. Frontend не трогаем (это решение #12 — auto-sections, реализуется в Phase 3). Критические ограничения: pipeline 2-4с, ~$0.10/turn unit economics ceiling — никаких extra LLM calls, никаких self-validation hooks. Промпт остаётся английским.

---

## Approach

### Phase 1 — pipeline в 5 точках

#### 1. `domain.Widget.ReplicateConfig` (грабля 10)

`project_v4/backend/internal/domain/widget_entity.go`:

```go
type ReplicateConfig struct {
    Enabled   bool
    Limit     int
    DataIndex int   // для не-replicate widgets — какой data[i] использовать
    GroupID   string // ставится engine'ом после expansion для constraint scoping
}

// Widget:
ReplicateConfig *ReplicateConfig `json:"-"`
```

Type-safe, никогда не сериализуется на фронтенд (`json:"-"`), engine-internal. После expand widgets теряют флаг через JSON roundtrip → frontend получает «обычные» виджеты.

#### 2. `expandReplicatedWidgets` — in-place splice (грабли 1, 2)

`project_v4/backend/internal/engine_v4/expand.go` (новый файл):

- Итерация по `formation.Widgets` **backwards** — splice не сбивает индексы ещё необработанных виджетов.
- Для каждого widget с `ReplicateConfig.Enabled=true`:
  - `end := min(len(data), cfg.Limit)`
  - `end == 0` → drop template entirely (нет данных — нет clone'ов)
  - `groupCounter++; groupID = "rg-N"`
  - Deep copy template в N clone'ов через `deepCopyWidget` (JSON roundtrip)
  - Каждому клону: `EntityRef = &EntityRef{Type, ID: data[j]["id"]}`
  - **Inline bind**: для каждого атома клона с `FieldName` → `a.Value = data[j][a.FieldName]`. Это критично — обходит позиционный `BindData` полностью.
  - Default actions если `len(Actions) == 0`
  - `ReplicateConfig.GroupID = groupID` всем clone'ам
  - `Meta["__bound"] = true` — флаг для top-level `BindData`
  - Splice: `formation.Widgets = [...prefix, ...clones, ...suffix]`
- Literal widgets без `ReplicateConfig.Enabled` остаются на месте.

#### 3. `autoDetectEntityRefs` — гибридная гравля 7

Та же `expand.go`, отдельная функция:

- Iterate `formation.Widgets`
- Skip если `EntityRef` уже стоит, replicate-обработан, или нет ни одного атома с `FieldName`
- `dataIdx := 0` (default), override через `ReplicateConfig.DataIndex`
- `dataIdx >= len(data)` → fallback на `data[0]`
- Set `EntityRef` из `data[dataIdx]["id"]`
- **Inline bind атомы из data[dataIdx]** — критично для widget на позиции != 0 в композиции, иначе позиционный BindData возьмёт неправильный data[i]
- Set `Meta["__bound"] = true`

#### 4. `BindData` skip-guard (грабля 1)

`project_v4/backend/internal/engine_v4/binding.go` — в `bindWidgetAtoms`:

```go
if w.Meta != nil {
    if bound, _ := w.Meta["__bound"].(bool); bound {
        return
    }
}
```

Top-level `BindData` остаётся как safety net для legacy single-widget путей. Multi-widget composition виджеты помечены `__bound` → пропускаются.

#### 5. `ApplyConstraints` group-aware (грабля 3)

`project_v4/backend/internal/engine_v4/constraints.go`:

- Per-atom и per-widget constraints применяются ко всем widgets как раньше.
- Cross-widget constraints (C1 field-presence threshold, C3 format consistency) теперь буккетятся по `ReplicateConfig.GroupID`:
  - `groups[gid] = append(groups[gid], w)` где `gid := w.ReplicateConfig.GroupID`
  - **Literals (gid=="") полностью пропускаются** — не имеют peers для cross-validation
  - Для каждой группы с `len > 1` → `applyCrossWidgetConstraints(group)`

Семантика: hero literal с одним полем больше не «тянет» galleries cards под threshold C1. Каждая replicate группа валидируется внутри себя.

#### 6. Engine pipeline integration

`project_v4/backend/internal/engine_v4/engine.go` — обновлён Step 4:

```go
// Bridge legacy input.Replicate flag → ReplicateConfig
if input.Replicate && len(formation.Widgets) == 1 && len(formation.Sections) == 0 && len(input.Data) > 0 {
    formation.Widgets[0].ReplicateConfig = &domain.ReplicateConfig{Enabled: true, Limit: input.Limit}
}

// Step 4a: in-place expansion (multi-widget safe)
expandReplicatedWidgets(formation, input.Data, input.EntityType)

// Step 4b: auto-detect EntityRef for single non-replicated entity widgets
autoDetectEntityRefs(formation, input.Data, input.EntityType)
```

Старый `replicateWidgets` (массив-overwrite) удалён вместе с импортом `fmt`. Старый `else case` (single-widget single-bind) удалён — `autoDetectEntityRefs` обрабатывает это универсально + теперь корректно для multi-widget compositions.

### Что НЕ тронуто в Phase 1

- `ops.go` `insertWidget` — не читает `props.preset/replicate/dataIndex`. Это Phase 2.
- `tree_ids.go` `BuildTreeMap` — всё ещё single-template. Phase 4.
- `engine.go` post-process для sections — Phase 3.
- Frontend — никаких изменений.
- Tool validation (top-level preset + multi-insert) — Phase 5.
- Промпт `prompt_compose_widgets.go` — Phase 5.

---

## Behaviour delta

### Пример 1 — legacy single-widget replicate ("покажи крема")

**До**: `input.Replicate=true` → `replicateWidgets` overwrite массива → N clone'ов, EntityRef из data[i].
**После**: bridge в `engine.go` ставит `ReplicateConfig.Enabled=true` на template → `expandReplicatedWidgets` splice'ит в том же массиве (с учётом backwards iteration) → N clone'ов, EntityRef + inline bind, GroupID `rg-1`. Cross-constraints применяются ко всей группе как раньше.

**Контракт**: legacy `replicate_behavior_test.go` (5 тестов) проходит без изменений, видно в выводе — атомы получают `name=Product 1/2/3...`, EntityRef'ы `p1/p2/p3...`.

### Пример 2 — composition в Phase 1 (вручную через тесты)

Сейчас Agent2 ещё не умеет это конструировать (нужна Phase 2 для `props.replicate`), но из тестов:

```go
formation := &domain.FormationWithData{
    Widgets: []domain.Widget{
        makeLiteralWidget("Welcome"),       // hero
        makeReplicateTemplate(),            // gallery template
        makeLiteralWidget("Buy now"),       // cta
    },
}
expandReplicatedWidgets(formation, sampleData(3), "product")
```

→ 5 widgets: `[hero, c1, c2, c3, cta]`. Hero и cta literal, без EntityRef. Clone'ы с EntityRef + GroupID `rg-1`, атомы заполнены данными data[0..2].

`ApplyConstraints` после этого: hero и cta пропускаются (gid=""), clone'ы валидируются как группа из 3 — `name/price/image` остаются на месте (раньше threshold 0.7×5=3.5, count=3 < threshold → удаление).

### Пример 3 — single entity widget на позиции != 0

```go
formation := &domain.FormationWithData{
    Widgets: []domain.Widget{
        makeLiteralWidget("Header"),
        makeEntityWidget(),   // позиция 1, FieldName атомы
    },
}
autoDetectEntityRefs(formation, sampleData(2), "product")
BindData(formation, sampleData(2))
```

**До Phase 1**: single-widget switch обрабатывал только `len(formation.Widgets) == 1`, multi-widget pathway не имел entity binding → атомы пустые. Если был, BindData позиционно дал бы `data[1]` widget'у на позиции 1.
**После**: `autoDetectEntityRefs` ставит `EntityRef = data[0]` (default), inline bind атомы из data[0], `__bound = true`. Top-level BindData skip'ает widget. Результат: name=alpha, price=100 (data[0]).

### Пример 4 — DataIndex override

```go
w := makeEntityWidget()
w.ReplicateConfig = &ReplicateConfig{DataIndex: 2}
// formation = [w], data = [4 items]
```

→ `EntityRef = data[2]`, name=gamma, price=300. Используется в comparison сценариях (несколько entity widgets из одного data slice).

---

## Files changed

| File | Change |
|---|---|
| `project_v4/backend/internal/domain/widget_entity.go` | `ReplicateConfig` type + `Widget.ReplicateConfig *ReplicateConfig` (json:"-") |
| `project_v4/backend/internal/engine_v4/expand.go` | **NEW** — `expandReplicatedWidgets` (in-place splice + inline bind), `autoDetectEntityRefs` (default+DataIndex+inline bind), `widgetHasFieldNameAtoms` |
| `project_v4/backend/internal/engine_v4/engine.go` | bridge legacy `Replicate` flag, replace switch с expand+auto-detect, удалён старый `replicateWidgets` + `fmt` import |
| `project_v4/backend/internal/engine_v4/binding.go` | `bindWidgetAtoms` skip widgets с `Meta["__bound"]==true` |
| `project_v4/backend/internal/engine_v4/constraints.go` | `ApplyConstraints` group-aware: bucket по `ReplicateConfig.GroupID`, cross-constraints per-group, literals skip |
| `project_v4/backend/internal/engine_v4/composition_behavior_test.go` | **NEW** — 7 assertive тестов: ExpandInPlace_LiteralsPreserved, BindData_NoPositionalShift, Constraints_GroupAware_LiteralsSkipped, EntityRef_AutoDetect, EntityRef_DataIndexOverride, ExpandInPlace_EmptyDataDropsTemplate, ExpandInPlace_LimitRespected |

Итого: 4 modified + 2 new = 6 файлов. **535 insertions(+), 35 deletions(-)**.

---

## Verification

### Локально

```bash
cd project_v4/backend
go build ./...                                                    # clean
go test ./internal/engine_v4/ -v -run TestExpandInPlace          # 3 PASS
go test ./internal/engine_v4/ -v -run TestBindData               # 1 PASS
go test ./internal/engine_v4/ -v -run TestConstraints_GroupAware # 1 PASS
go test ./internal/engine_v4/ -v -run TestEntityRef              # 2 PASS
go test ./internal/engine_v4/ -v -run TestBehavior               # 5 legacy replicate PASS
go test ./internal/engine_v4/ -v -run TestPreset                 # 5 preset PASS
go test ./internal/engine_v4/ -v -run TestRefChaining            # 2 ref-chaining PASS
go test ./internal/engine_v4/...                                  # all green
```

Все 7 новых composition тестов + все существующие 12 (5 replicate + 5 preset + 2 ref-chaining) проходят без изменений.

### На проде (после деплоя)

Phase 1 — backend-only, semantically backwards-compatible. Поведение для legacy single-widget replicate identical:

1. **Regression**: «покажи крема» → product_card grid из 6 cards → визуально как раньше.
2. **Regression**: «покажи детали первого» → product_detail → визуально как раньше.
3. **Regression**: «сделай цены красными» (modify mode) → modify ops применяются на existing formation → как раньше.
4. **Multi-widget composition** ещё не доступна Agent2 — ждёт Phase 2 (`props.replicate` в insertWidget). Можно проверить вручную через testbench с явными `ReplicateConfig` на widgets, но это академично пока tool не поддерживает синтаксис.

---

## Known gaps / caveats

1. **Phase 1 = инфраструктура без UX surface**. Multi-widget composition нельзя сейчас попросить у Agent2 — нужна Phase 2 для `props.replicate/preset/dataIndex` в `insertWidget`. Текущий код только обеспечивает корректную обработку, если кто-то конструирует `ReplicateConfig` напрямую.
2. **`BuildTreeMap` всё ещё single-template** (грабля 4). После expand multi-widget composition, Agent2 в modify mode увидит только первый виджет в context. Phase 4.
3. **Refs namespace всё ещё global** (грабля 5). Если в Phase 2 expand'ить два preset'а в одном batch'e — refs `w/root/info/meta` коллизятся. Phase 2 решит через `ExpandPresetWithPrefix`.
4. **Frontend не знает про auto-sections** — в Phase 3 движок начнёт группировать widgets в `formation.sections[]` post-process, frontend `FormationRenderer` уже умеет рекурсивно рендерить секции (проверено в audit). Phase 1 не трогает sections.
5. **Cross-widget constraints semantics changed** — раньше literal multi-widget formation (не replicate, через top-level ops) мог получить cross-widget normalization. Теперь нет — literals не группируются. Это было намеренное решение #3 (handoff), не regression. Если найдётся legacy use case где это нужно — добавить «default group» для literal widgets.
6. **`replicate_behavior_test.go` non-assertive** — `t.Logf` only, физически не падают. Но семантически проверено: атомы заполнены правильно, EntityRef'ы стоят (visible в logs тестов).
7. **Update log time** — этот файл датирован MSK (UTC+3), commit Author Date `Tue Apr 7 02:46:18 2026 +0300`.

Phase 1 готова. Следующая — Phase 2 (per-widget preset + replicate в `insertWidget` props + `ExpandPresetWithPrefix` для prefix-namespacing refs).
