# Metadata-driven binding — Step A (conditional tree_map + Agent2 `<fields>` block)

- **Branch**: `feature/engine-v4`
- **Date (UTC)**: 2026-04-09 21:34
- **Commit**: `7e15cd9` (will be followed by a docs commit containing this log)
- **Parent**: `ba8f0e3` (docs log of previous cache audit session)
- **Plan**: `/Users/starknight/.claude/plans/snappy-singing-pascal.md`
- **Design doc**: `docs/New features/METADATA_DRIVEN_BINDING_2026-04-09.md`

## Context

Первая из трёх сессий по закрытию `PRE_LAUNCH_TASKS.md → 4.3 B7` (role-based
field resolution). Цель всей работы — сделать V4 пайплайн универсальным:
один и тот же preset `product_card` должен работать на любом каталоге
(косметика, ноуты, книги, мебель) без ручного маппинга полей и без role-
схемы в БД. Дизайн выбран metadata-driven: LLM (Agent2) матчит атом-слоты
на поля тенанта по данным, которые движок не видит.

Эта сессия делает **инфраструктурный слой** — реорганизует доставку данных
от движка в Agent2 и впрыскивает per-tenant словарь данных в system prompt,
но не меняет поведение LLM (правила матчинга идут в Сессии B). Поведение
hey-babes ожидается идентичным: те же 23 виджета, тот же preset, те же
биндинги — просто чуть-чуть другой формат tree_map в trace и новый
`<fields>` блок в system prompt.

## Approach

### 1. Conditional tree_map schema (`engine_v4/tree_ids.go`)

`buildWidgetMap` раньше отдавал для каждого атома одинаковый формат
`{id, type, field?}`. Для metadata-driven matching этого мало: LLM должна
видеть разницу между «атом уже связан, не трогай» и «атом пустой, найди
ему поле». Переписал на conditional output:

- **Bound атом** (FieldName заполнен + Value непустой/не nil/не `<UNKNOWN>`):
  ```json
  {"id": "a-s0-w0-title", "field": "name"}
  ```
  Минимум токенов — LLM видит «решено».
- **Open атом** (нет значения или нет fieldName):
  ```json
  {"id": "a-s0-w2-slot", "type": "text", "subtype": "string",
   "slot": "title", "format": "text", "open": true}
  ```
  Rich metadata — LLM может запросить `<fields>` и сматчить slot на поле.

Извлечён helper `isBoundAtom(a)` с явной проверкой "usable value"
(не nil, не `""`, не `"<UNKNOWN>"`, числа/объекты считаются bound при
non-nil). Если у open-атома есть intended `FieldName` (но без resolved
Value) — экспонируем его как `field` рядом с `open:true`, чтобы LLM могла
решать «оставить intent или override».

Поменял тип `atoms` с `[]map[string]string` на `[]map[string]interface{}`
чтобы `open: true` был bool.

### 2. `SampleFieldValues` port method + postgres adapter

Добавил в `FieldDefinitionPort` новый метод:
```go
SampleFieldValues(ctx, tenantIDOrSlug, entityType, limit) (map[string][]interface{}, error)
```

Возвращает `map[fieldName][]sampleValue` — реальные значения из N первых
продуктов для каждого из pre-seeded field definitions. Реализация в
`postgres/field_definition_adapter.go` делает один SELECT с JOIN'ами
`products → master_products → categories`, покрывающий все 13 hey-babes
product fields (images/name/price/rating/brand/category/description/tags/
stockQuantity/productForm/skinType/concern/keyIngredients).

Отдельный helper `appendSample` фильтрует пустые/нулевые значения —
пустой string хуже чем отсутствие сэмпла для LLM prompting'а. JSONB
колонки (images/tags) декодируются; массивные поля (skin_type/concern/
key_ingredients) конвертируются в `[]interface{}`.

Для `service` entity возвращает пустой map — дизайн doc помечает
multi-entity как out of scope для PoC.

### 3. `buildSystemPromptWithFields` helper в Agent2 (`usecases/agent2_execute.go`)

Зеркальный клон паттерна `Agent1ExecuteUseCase.buildSystemPromptWithDigest`:

- Новое поле `fieldsPromptCache sync.Map` в структуре — memoization
  per tenant slug, инвалидация только рестартом процесса. Это критично:
  без memoization byte-stability system prompt'а нарушается между
  turn'ами → Anthropic cache не срабатывает → cacheRead = 0.
- Helper `buildSystemPromptWithFields(ctx, tenantSlug)`:
  1. Fallback на базовый `Agent2ToolSystemPrompt` если tenantSlug
     пустой, port nil, ListFieldDefinitions вернул ошибку/пусто, или
     formatFieldsBlock вернул пусто.
  2. Загружает field definitions один раз.
  3. Вызывает `SampleFieldValues` (best-effort — ошибки и пусто
     игнорируются, block просто без samples).
  4. Формат через `formatFieldsBlock(fields, samples)` helper.
  5. Склейка `Agent2ToolSystemPrompt + "\n\n" + block + "\n"` →
     Store в cache → return.
- `formatFieldsBlock` — чистая функция, рендерит компактный блок:
  ```
  <fields entity="product">
  images          image/url          label="Images"         slot=hero     samples=["https://..."]
  name            text/string        label="Name"           slot=title    samples=["COSRX BHA","Laneige"]
  price           number/currency    label="Price"          unit="RUB"   slot=price    samples=[2490,3990]
  ...
  </fields>
  ```
  Поля сортируются по `Priority ASC`, type descriptor комбинируется
  как `atom_type/subtype` для информативности. Отсутствующие samples
  просто пропускаются.

Заменил в `Execute` строку `systemPrompt := prompts.Agent2ToolSystemPrompt`
на `systemPrompt := uc.buildSystemPromptWithFields(ctx, req.TenantSlug)`.
Существующий `loadFieldLabels` оставил как есть — он отдаёт labels
в user prompt (не в system), это другая работа которую не ломаем.

### 4. Unit тесты

Создал `internal/usecases/agent2_execute_test.go` (первый test-файл в
пакете usecases). Три теста для `formatFieldsBlock`:

- `TestFormatFieldsBlock_BasicShape` — полный happy path: 3 поля,
  2 с samples. Проверяет prefix/suffix тегов, наличие всех field
  names, priority ordering, JSON форму samples, compound type
  (`number/currency`), unit="RUB", slot hints.
- `TestFormatFieldsBlock_EmptyInput` — nil input → пустая строка.
- `TestFormatFieldsBlock_NoSamples` — поля без samples → блок
  без `samples=` маркеров.

## Files changed

| File | Change |
|---|---|
| `docs/New features/METADATA_DRIVEN_BINDING_2026-04-09.md` | + (design doc, закоммичен вместе с implementation) |
| `project_v4/backend/internal/engine_v4/tree_ids.go` | `buildWidgetMap` → conditional schema + `isBoundAtom` helper |
| `project_v4/backend/internal/ports/field_definition_port.go` | + `SampleFieldValues` method |
| `project_v4/backend/internal/adapters/postgres/field_definition_adapter.go` | + SQL implementation + `toInterfaceSlice` helper |
| `project_v4/backend/internal/usecases/agent2_execute.go` | + `sync` import, `sort` import, `fieldsPromptCache`, `buildSystemPromptWithFields`, `formatFieldsBlock` |
| `project_v4/backend/internal/usecases/agent2_execute_test.go` | + (новый, 3 теста на formatFieldsBlock) |

## Verification

Локально выполнено:
- `cd project_v4/backend && go build ./...` — чисто
- `go test ./internal/engine_v4/...` — все тесты проходят (включая
  composition_behavior_test, ops_test, replicate_behavior_test,
  presets_behavior_test)
- `go test ./internal/usecases/...` — новые formatFieldsBlock тесты
  зелёные
- `go test ./internal/...` — полный прогон без регрессий

Проверить на prod / runtime (следующий шаг для владельца):
1. Запустить V4 backend (`scripts/start.sh` или `cd project_v4/backend && go run ./cmd/server`)
2. Hey-babes регрессия: `curl POST /api/v1/pipeline` с `query:"покажи крема"`
   → ожидаем 23 виджета как раньше, preset `product_card`
3. Trace: `curl /debug/traces/?format=json&limit=3 | jq '.[0].agent2'`:
   - `.systemPrompt` должен содержать `<fields entity="product">` блок
     с name/price/brand/rating/images/... и реальными samples из БД
   - `.systemPromptChars` вырос примерно на 400-800 символов относительно
     baseline (`ba8f0e3`)
   - `.toolInput.formation_tree.widgets[].template.atoms[]` — bound
     атомы теперь `{id, field}` без type/slot, open атомы (если будут)
     — rich
4. Cache metric на 2+ turn: `.cacheRead > 0`. Доказывает что
   system prompt byte-stable — memoization через sync.Map работает.
5. Data integrity: атомы всё так же non-null Value после рендера,
   визуальный рендер на фронте не изменился.

## Known gaps / caveats

- **Nothing changes for hey-babes** — это feature, не баг. LLM пока не
  знает про `<fields>` блок как инструкцию (Сессия B добавит FIELD
  BINDING rule). Сейчас блок в system prompt есть, но LLM его
  игнорирует. Цель Сессии A — заложить data plumbing без риска
  ломать текущее поведение.
- **SampleFieldValues tied to hey-babes shape** — SQL жёстко ожидает
  `products JOIN master_products` с PIM колонками (skin_type, concern,
  key_ingredients, product_form). Для тенантов без master_products
  вернёт только flat-колонки (name, price, rating, images, tags,
  description) — остальные field_names получат empty samples,
  `<fields>` блок всё равно построится с label+type info. Тест на
  это придёт в Сессии C когда сделаем test-electronics seed.
- **Service entity type не поддержан** — `SampleFieldValues` для
  `entityType=service` возвращает пустой map. Services не теряют
  функциональность — loadFieldLabels по-прежнему загружает их labels
  в user prompt. Но service-тенанты не получат `<fields>` блок в
  system prompt до расширения SQL. Не блокер для PoC (products only).
- **ProductToMap ещё хардкодит hey-babes keys** (`tools/tool_visual_assembly.go:311`).
  Это отдельная проблема для Сессии C: чтобы test-electronics работал,
  нужно добавить `Product.Extra` в output map. Сессия A не трогала
  этот код.
- **`__bound` flag в `expand.go:87`** всё ещё ставится безусловно на
  клонах после replicate. Design doc помечает это как orthogonal mini-PR
  — теперь conditional schema в `buildWidgetMap` выводит bound из
  реального состояния Value, не из Meta-флага, так что LLM видит
  правильную картину независимо от этого бага. Если решим чинить —
  отдельным коммитом.
- **Cache invalidation** — `fieldsPromptCache` не знает про админские
  изменения field_definitions. До рестарта процесса блок stale. Design
  doc принимает этот trade-off для MVP (та же семантика что у Agent1
  digestCache).
- **Не проверено live** — unit-тесты и билд зелёные, но integration
  smoke test (curl → trace анализ) владелец запустит сам. В этой
  сессии backend не стартовал (нет активного инстанса на localhost:8080).

## Next session (B — Step 3)

Сразу после merge этой сессии:
1. `ops.go:mergeAtomProps` — добавить обработку `fieldName` (сейчас
   update-ops молча игнорируют его)
2. `ops_test.go` — `TestApplyUpdate_FieldName`
3. `prompt_compose_widgets.go` — добавить секцию `## FIELD BINDING`
   перед примерами + таблицу дефолтных fieldName по пресетам
4. Регрессия hey-babes (3 сценария — rebuild/modify columns/delete atom)
5. Dry run на каком-нибудь fake тенанте через curl (если удобно)
