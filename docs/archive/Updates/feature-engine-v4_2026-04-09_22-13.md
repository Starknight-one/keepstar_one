# Metadata-driven binding — Step B (fieldName override + FIELD BINDING rule)

- **Branch**: `feature/engine-v4`
- **Date (UTC)**: 2026-04-09 22:13
- **Parent**: `333e918` (Session A — conditional tree_map + `<fields>` block)
- **Plan**: `/Users/starknight/.claude/plans/snappy-singing-pascal.md`
- **Design doc**: `docs/New features/METADATA_DRIVEN_BINDING_2026-04-09.md`

## Context

Второй из трёх коммитов по закрытию `PRE_LAUNCH_TASKS.md → 4.3 B7`. Session A
заложила инфраструктуру (conditional tree_map + `<fields>` блок в system
prompt), но LLM ещё не умел этим пользоваться: `mergeAtomProps` молча
игнорировал `props.fieldName`, так что override ops в стиле
`{op:"update", target:"name", props:{fieldName:"model"}}` не работали.
Session B делает две вещи:

1. Чинит engine — update ops с fieldName теперь реально перепривязывают атом
2. Учит Agent2 этим пользоваться — добавлена секция `## FIELD BINDING` в
   system prompt перед BUILDING/COMPOSING с правилами матчинга слот→поле,
   таблицей дефолтных fieldName по пресетам, и явным запретом копировать
   hey-babes имена из примеров

После session B Session A + B вместе дают LLM полный контракт:
- В system prompt: `<fields>` блок (данные) + FIELD BINDING rule (инструкции)
- Engine принимает override — fieldName меняется, Value пересчитывается из
  новых данных через BindData

Поведение hey-babes должно остаться identical: там фактические поля
совпадают с дефолтами пресетов (images/name/price/rating/brand) — LLM
увидит совпадение и не будет override'ить. Override'ы понадобятся
только Session C, когда появится test-electronics тенант с model/manufacturer/
cover_image.

## Approach

### 1. `mergeAtomProps` — handle `fieldName` + `slot` (`engine_v4/ops.go:717`)

Добавил в начало merger'а:
```go
if v, ok := props["fieldName"].(string); ok {
    atom.FieldName = v
    atom.Value = nil  // BindData re-bind with new field
}
if v, ok := props["slot"].(string); ok && v != "" {
    atom.Slot = domain.AtomSlot(v)
}
```

Почему `atom.Value = nil`: если атом раньше был связан со старым
fieldName и получил Value, смена fieldName без очистки оставит stale
value в новом атоме. `BindData` в engine pipeline Step 5 пройдётся и
перепривяжет из `data[i][newFieldName]`.

### 2. `applyUpdate` — fieldName fallback (`engine_v4/ops.go:189`)

Проблема: в rebuild mode свеже-вставленные атомы ещё не имеют tree ID
(IDs штампуются в `StampTreeIDs` — Step 7, ПОСЛЕ `ApplyOps` — Step 3).
Значит `idx.atoms["name"]` не находит свежий атом с `FieldName:"name"`,
и override `{target:"name"}` падает с warning `target "name" not found`.

`ExpandWildcardOps` (`ops.go:1188`) решает эту проблему, но вызывается
только в modify mode из `tool_visual_assembly.go:247`. В rebuild mode
wildcard expansion не применяется → preset + override в одной batch
тихо ломается.

Добавил в `applyUpdate` fallback в самом конце (перед warn'ом):
```go
if idx.formation != nil && op.Target != "" && !IsTreeID(op.Target) {
    matched := 0
    updateInWidgets := func(widgets []domain.Widget) {
        for wi := range widgets {
            w := &widgets[wi]
            for ai := range w.Atoms {
                if w.Atoms[ai].FieldName == op.Target {
                    mergeAtomProps(&w.Atoms[ai], op.Props)
                    if w.Atoms[ai].Rigidity != domain.RigidityLocked {
                        w.Atoms[ai].Rigidity = domain.RigidityLocked
                    }
                    matched++
                }
            }
        }
    }
    updateInWidgets(idx.formation.Widgets)
    for si := range idx.formation.Sections {
        updateInWidgets(idx.formation.Sections[si].Widgets)
    }
    if matched > 0 { return "" }
}
```

Fallback активируется только если target не выглядит как tree ID
(`IsTreeID` — префиксы `a-`, `n-`, `w-`). Это не дублирует
`ExpandWildcardOps` — наоборот, делает его избыточным для rebuild, а
для modify они работают вместе (wildcard expansion вперёд → fallback
остаётся no-op потому что ID уже найден).

### 3. Unit test `TestApplyUpdate_FieldName` (`engine_v4/ops_test.go`)

Покрывает реальный сценарий Session C:
- 2 продукта-электроники с полями `model/manufacturer/price/cover_image`
- Препосет `product_card` (дефолты: `images/name/price/rating/brand`)
- 3 override ops: `name→model`, `brand→manufacturer`, `images→cover_image`
- Replicate=true, 2 виджета после прогона

Ассерции:
1. `seen["model"]` существует, `seen["name"]` не существует — fieldName
   реально ЗАМЕНЁН, не добавлен параллельно
2. `seen["model"] == data[i]["model"]` — BindData после override
   подтянул значение из data (проверяет что мы очищаем `atom.Value`)
3. Аналогично для manufacturer и cover_image

Добавил helper `keysOf(map)` для читаемых error-сообщений.

### 4. FIELD BINDING section в Agent2 prompt (`prompts/prompt_compose_widgets.go`)

Вставлена между `## PRESETS` (где заканчиваются примеры пресетов на
«When to go freestyle») и `## BUILDING FROM SCRATCH`. Содержание:

1. **Preamble** — объясняет что `<fields>` блок это source of truth
   для fieldName, обязательно consult перед каждым insert/update
2. **Matching slot → field** — 8 эвристик slot→тип поля
   (title→short text, hero→image, price→number+currency и т.д.)
3. **Preset default fieldNames table** — для всех 6 product пресетов
   перечислены их дефолтные fieldName'ы. LLM смотрит <fields>,
   сравнивает, решает нужен ли override
4. **Override examples** — 3 примера update ops для
   name→model / images→cover_image / brand→manufacturer
5. **formation_tree shapes** — объяснение Bound vs Open атомов
   (Session A формат) и что делать с каждым
6. **Rules** — 4 явных правила:
   - НИКОГДА не копировать fieldName из примеров BUILDING/COMPOSING
     (там везде hey-babes — images/name/price/brand/rating)
   - При building from scratch брать fieldName прямо из <fields>
   - Override'ить только то что отличается
   - Не re-bind'ить bound атомы без явного user request

Подход для hey-babes: так как preset defaults = hey-babes fields,
LLM увидит в <fields> точное совпадение и НЕ будет эмитить override
ops. Behavior identical baseline.

## Files changed

| File | Change |
|---|---|
| `project_v4/backend/internal/engine_v4/ops.go` | `mergeAtomProps` handles `fieldName`/`slot`, `applyUpdate` has fieldName fallback for rebuild mode |
| `project_v4/backend/internal/engine_v4/ops_test.go` | + `TestApplyUpdate_FieldName` (electronics override scenario) |
| `project_v4/backend/internal/prompts/prompt_compose_widgets.go` | + `## FIELD BINDING` section (~60 lines) between PRESETS and BUILDING |

## Verification

Локально выполнено:
- `go build ./...` — чисто
- `go test ./internal/engine_v4/...` — green (включая новый TestApplyUpdate_FieldName)
- `go test ./internal/usecases/...` — green (formatFieldsBlock тесты Session A не тронуты)
- `go test ./internal/...` — полный прогон без регрессий

Проверить на prod после деплоя:
1. **Hey-babes regression (critical)** — поведение должно быть identical
   baseline, потому что LLM увидит `<fields>` с полями совпадающими
   с preset defaults → не будет override'ить:
   - `curl POST /api/v1/pipeline` с `query:"покажи крема"` → 23 widgets
   - `trace.agent2.toolInput` НЕ должен содержать override ops для fieldName
   - `trace.agent2.systemPromptChars` вырос примерно на 2500-3000 chars
     относительно baseline 16103 (FIELD BINDING rule + таблица)
2. **Turn 2 cache** — `cacheRead > 0` (system prompt всё ещё byte-stable,
   мы добавили статический текст в константу)
3. **Modify на hey-babes** — «убери рейтинг» → delete op, остальные
   атомы не тронуты, fieldName не меняется
4. **Regression `TestApplyUpdate_FieldName` unit test** — не должен
   регрессировать при любом будущем изменении ops.go

## Known gaps / caveats

- **`applyDelete` не имеет fieldName fallback** — в rebuild mode удаление
  по fieldName не работает, только через ID. Это не мешает Session C
  (удаления не в сценарии), но может выстрелить если LLM попробует
  `{op:"delete", target:"rating"}` в rebuild. В modify это работает
  через `ExpandWildcardOps`. Можно добавить симметричный fallback
  в applyDelete отдельным мини-PR если понадобится.
- **Slot/subtype/type override не покрыты тестом** — `mergeAtomProps`
  теперь принимает `slot`, но тест только на `fieldName`. Это bonus
  поле и его runtime использование опционально, тест можно добавить
  позже.
- **Не проверено на проде с тенантом отличным от hey-babes** — Session C
  создаст test-electronics seed и тогда реально увидим как LLM выбирает
  override ops. Session B только подготавливает землю.
- **FIELD BINDING rule vs override accuracy** — промт написан исходя
  из паттернов которые я могу представить. Реальная эффективность
  зависит от того как Haiku 4.5 парсит таблицу preset defaults и
  `<fields>` блок. Если PoC покажет что LLM промахивается — нужна
  итерация промпта (добавить явный example «tenant has model, preset
  has name → emit update», или few-shot).
- **Preset defaults table захардкожена в промпте** — если мы добавим
  новые пресеты или изменим их fieldNames, таблица устареет.
  Долгосрочно можно генерировать таблицу из `presets_product.go`
  metadata в runtime, но для MVP хардкода достаточно.
- **Cache прожорливость не замерена** — FIELD BINDING rule это ~2500
  chars статики, это добавит ~600 tokens на каждый cache write
  на каждый тенант. На втором turn cache read покроет. Один раз
  нагреем и дальше tax = 0.

## Next session (C — Steps 4+5)

Сразу после merge этой сессии:
1. `scripts/seed_test_electronics.sql` — создать test-electronics
   тенант, 9 field_definitions, 8-10 продуктов с tenant-specific
   полями в `Product.Extra` (model, manufacturer, cpu, ram, …)
2. Проверить `ProductToMap` (`tools/tool_visual_assembly.go:311`) —
   нужно ли расширять чтобы `Product.Extra` попал в data map
3. Прогон pipeline на test-electronics → trace анализ:
   - `<fields>` содержит электронные поля с правильными samples
   - `toolInput.ops` содержит override ops
   - formation имеет 8-10 виджетов с не-nil Value в title/price/hero
4. (stretch) seed_test_books.sql + seed_test_furniture.sql для domain
   shift доказательства
5. Финал: закрыть 4.3 B7 с пометкой «решено через metadata-driven
   binding вместо role schema»
