# Metadata-driven atom binding — design doc

**Дата**: 2026-04-09
**Ветка**: `feature/engine-v4`
**Статус**: ✅ **DONE 2026-04-10** — Sessions A/B/C shipped: `7e15cd9` (tree_map + `<fields>` block), `9261327` (fieldName override + FIELD BINDING rule), `5566439` (Product.Extra wiring + test-electronics seed), `2fa746e` (VectorSearch reads p.extra). Verified: `7caeb8f`.
**Закрывает**: `docs/PRE_LAUNCH_TASKS.md` → 4.3 B7 role-based field resolution

---

## Что решаем

**Задача одним предложением**: сделать так, чтобы один и тот же V4 пайплайн (Agent1 → Agent2 → engine → рендер) работал на произвольном каталоге — косметика, ноутбуки, книги, мебель, что угодно — **без кода-изменений на стороне тенанта, без ручного маппинга полей, без ролевых схем в БД**.

**Текущая проблема**: все 12 пресетов в `engine_v4/presets_product.go` прибиты к конкретным ключам продукта хей-бейбс: `name/price/images/rating/brand/description/tags/category`. Для тенанта с полями `model/manufacturer/cpu/ram/cover_image` пресет создаст атомы с `fieldName:"name"` → `BindData` не найдёт такого поля в data → атомы останутся с `Value=nil` → грид из пустых карточек. Это главный блокер LAUNCH_CHECKLIST «любой каталог».

**Отвергнутые альтернативы**:
- **Role schema в `catalog.field_definitions`** (enum: title/price/image_primary/...) с resolver'ом в движке. Требует: миграцию БД, enum drift, ручной enrichment pass для каждого нового тенанта (или LLM-pass в админке, что добавляет целый пайплайн). Для одного разработчика без команды — непрактично. Плюс загрязняет движок semantic-знанием, регрессируя обратно в V2-эпоху «умный движок».
- **Переписать все 12 пресетов на slot-based refs** (`$title`, `$hero`, ...) где Agent2 всегда указывает fieldName. Удваивает output-токены, убивает cache-стабильность промпта, ломает работающую хей-бейбс механику ради симметрии. Плюс два параллельных паттерна (legacy и new) до завершения миграции — стандартный рецепт багов.

**Выбранное решение**: metadata-driven binding — одна точка интеллекта (LLM), один слой (prompt), нулевые изменения в движке по семантике. Симметрично тому что мы уже сделали для Agent1 catalog digest (commits `403d1fe`, `64dc6ae`): богатые метаданные кэшируются в system prompt, LLM использует их в runtime через tool ops.

---

## Модель — две стороны матча

**Atom = контейнер с известной формой.** Уже несёт полный паспорт в `domain/atom_entity.go`:
- `Type` — text/number/image/icon/video/audio
- `Subtype` — string/currency/rating/url/date/...
- `Slot` — title/hero/price/primary/secondary/badge/description/tags/specs/gallery/stock
- `Format` — currency/stars/percent/date/...
- `Wrapper` — badge/tag/button/link/...
- `TextStyle` — fontSize/fontWeight/color/...

**Field = источник значений с известными характеристиками.** Частично в `catalog.field_definitions` (имя, лейбл, тип), частично извлекаемо из самих продуктов (сэмплы значений, диапазоны, топ-значения enum'ов).

**Задача Agent2** — связать «контейнер с формой» с «источником с характеристиками» через op:
```json
{"op":"update","target":"<atomId>","props":{"fieldName":"<tenantKey>"}}
```

Для новых атомов через `insert` — то же самое внутри того же batch.

Движок остаётся 100% dumb: `BindData(atom, data)` смотрит `atom.FieldName` и берёт `data[fieldName]`. Никакой семантики, никаких ролей, никаких resolver'ов.

---

## Ключевой инсайт — conditional tree_map schema

Первоначальная идея была «давать LLM все свойства каждого атома одинаково» (`{id, type, subtype, slot, format, field, bound}`). Это избыточно.

**Правильная модель**: атом в tree_map отдаётся **в одном из двух режимов** в зависимости от состояния.

### Состояние 1 — bound (атом успешно получил значение)
```json
{"id":"a-s0-w0-title","field":"name"}
```
Только ID и имя связанного поля. LLM видит «я существую, я связан, меня трогать не надо». Если пользователь попросит «убери название» — таргетинг по `field` или по `id`. Всё.

### Состояние 2 — open (атом пустой, ждёт биндинга)
```json
{"id":"a-s0-w2-slot","type":"text","subtype":"string","slot":"title","open":true}
```
Rich metadata. LLM использует `type/subtype/slot` как запрос к `<fields>` блоку для поиска подходящего поля. `open:true` — явный сигнал «меня надо заполнить».

### Почему так

- **Экономия токенов**: замеры на replicate template с 5 атомами — `5 × 35 ≈ 175 символов` для bound vs `5 × 110 ≈ 550` для full. Dedupe один раз на template → экономия ~90-150 токенов на render.
- **Семантическая чистота**: bound атом для LLM — «решённая задача», open атом — «работа для меня». Разные формы = разные действия. Не нужно рассуждать «а что я с этим делаю» по однообразному блоку.
- **Меньше вероятность что LLM случайно тронет bound атом** — нет слота/типа который мог бы её сбить с толку.

---

## Критический fix — `__bound` должен не врать

В `expand.go:87` флаг `Meta["__bound"]=true` ставится **безусловно** на каждого клона после expandReplicatedWidgets, даже если `data[j][atom.FieldName]` не существует (то есть атом остался с `Value=nil`). Для хей-бейбс это работает потому что пресеты совпадают с данными. Для чужого тенанта → атомы «якобы связаны, но пустые» → `BindData` их скипает (из-за `__bound`) → грид пустой, и LLM не знает что есть работа.

**Fix**: ставить `__bound=true` только если **хотя бы один** атом реально получил значение в inline-bind loop. Или симметрично: в `buildAtomMap` выводить `bound:true` только если `Atom.Value != nil` **и** `Atom.FieldName != ""`. Второй вариант честнее, потому что реально отражает фактическое состояние, а не флаг.

Практически в `buildAtomMap`:
```go
isBound := a.FieldName != "" && a.Value != nil && a.Value != "" && a.Value != "<UNKNOWN>"
if isBound {
    am = {"id": a.ID, "field": a.FieldName}
} else {
    am = {"id": a.ID, "type": a.Type}
    if a.Subtype != "" { am["subtype"] = a.Subtype }
    if a.Slot != "" { am["slot"] = a.Slot }
    if a.Format != "" { am["format"] = a.Format }
    am["open"] = true
}
```

`Meta["__bound"]` в `BindData` остаётся своей отдельной вещью — это флаг «не трогай позиционным bind'ом», он делает другую работу. Fix в expand тоже стоит сделать (ставить только если реально связался) — но это ортогонально основной работе и может пойти отдельным мини-PR.

---

## `<fields>` блок — per-tenant словарь данных

**Куда кладём**: в Agent2 system prompt, через новый helper `buildAgent2SystemPromptWithFields(ctx, tenantSlug)` — симметрично тому что `Agent1ExecuteUseCase.buildSystemPromptWithDigest` делает для Agent1. Мемоизация в `sync.Map[tenantSlug]string`, инвалидация по рестарту процесса (как у Agent1).

**Почему система а не user prompt**: system кэшируется (byte-stable per tenant), cacheRead = input × 0.1. User prompt на каждом turn'е разный (меняется state, tree_map) → не кэшируется.

**Формат** (plain text, минимальный для парсинга LLM):

```
<fields entity="product">
name            text         label="Название"      samples=["COSRX BHA Blackhead Power Liquid","Laneige Water Bank Moisture Cream"]
price           number       label="Цена"          unit="RUB"  range=290..4990
brand           text         label="Бренд"         top=["COSRX","Laneige","The Saem","Missha"]
rating          number       label="Рейтинг"       range=3.8..5.0  null=12%
images          array<url>   label="Фото"          samples=["https://..."]  note="[0] for hero"
description     text         label="Описание"      avgLen=180
keyIngredients  array<text>  label="Ключевые ингредиенты"  samples=["ниацинамид","гиалуроновая кислота"]
skinType        array<text>  label="Тип кожи"      top=["сухая","жирная","комбинированная","нормальная"]
category        text         label="Категория"     top=["очищение","увлажнение","защита"]
tags            array<text>  label="Теги"          samples=[...]
</fields>
```

Ожидаемый размер — 400-800 токенов на типовой каталог. Первый turn — cache_write (~0.04¢ единоразово на process lifetime на тенанта), далее — cache_read × 0.1.

**Источник данных**:
- Labels + types из `catalog.field_definitions` через существующий `FieldDefinitionPort.ListFieldDefinitions`
- Сэмплы — один SQL `SELECT ... FROM products WHERE tenant_id=? LIMIT 3`, извлечение значений из JSONB или top-level колонок
- Range/top — агрегатный SQL (`MIN/MAX` для чисел, `GROUP BY` топ-5 для low-cardinality enums)
- Кешируется в sync.Map, выполняется один раз на тенант на process lifetime

**Решение по источнику сэмплов (PoC)**: вариант (A) — SQL на лету в `buildAgent2SystemPromptWithFields`. Проще реализовать, меньше movement. Если окажется медленным — миграция на (B) где сэмплы предгенерируются вместе с `CatalogDigest` в админке и хранятся в расширенной схеме digest'а.

---

## Пресеты не трогаем

Хей-бейбс пресеты пишут `fieldName:"name"` — это работает для хей-бейбс. Для ноутов Agent2 отправит в одном batch'е:
```json
{
  "mode":"rebuild",
  "preset":"product_card",
  "ops":[
    {"op":"update","target":"name","props":{"fieldName":"model"}},
    {"op":"update","target":"images","props":{"fieldName":"cover_image"}},
    {"op":"update","target":"brand","props":{"fieldName":"manufacturer"}}
  ]
}
```

Механика: `VisualAssemblyTool.Execute` в tool_visual_assembly.go:215 конкатенирует preset ops + user ops → `engine.Execute` → `ApplyOps` применяет их по порядку. Update-ops переписывают `fieldName` атомов ДО того как `expandReplicatedWidgets` клонирует template → клоны получают уже правильные fieldName → `BindData` биндит корректно.

**Надо проверить на этапе имплементации**: умеет ли `ApplyOps` update-op реально менять `fieldName` props (а не только atomic display properties). Если не умеет — минорный расширяющий фикс в `ops.go`. Если умеет — бесплатно.

**Таблица дефолтных fieldName по пресетам** — добавляется в Agent2 system prompt как справочник, чтобы LLM знала что таргетить в override-ops:
```
Preset defaults (fields used by each preset — override if your tenant doesn't have them):
  product_card:         name, price, images, rating, brand
  product_card_compact: name, price, images
  product_card_horizontal: name, price, images, description
  product_detail:       name, price, images, rating, brand, description, tags
  product_detail_horizontal: name, price, images, description
  ...
```

---

## Правило матчинга в Agent2 prompt

Новая секция в `Agent2ToolSystemPrompt` (prompt_compose_widgets.go):

```
## FIELD BINDING

Before writing props.fieldName on any atom (insert or update), consult <fields> below:

1. Read atom.slot and atom.type/subtype from formation_tree or your insert op
2. Scan <fields> for a field whose type is compatible AND whose label/samples match the slot semantics
3. Matching hints:
   - slot:title       → field with short human-readable text samples (product name, model, book title)
   - slot:hero        → field of type image or array<url>; for arrays take [0]
   - slot:price       → field of type number with currency-like unit
   - slot:description → field of type text with long samples (>100 chars avg)
   - slot:primary     → structured attributes (brand, category, key specs)
   - slot:secondary   → less-critical attributes (stock, rating)
   - slot:tags        → field of type array<text>
   - slot:badge       → short text with semantic meaning (discount, new, sale)

4. For formation_tree atoms marked open:true — they are empty slots waiting for binding.
   Pick a field and write {op:"update", target:<atomId>, props:{fieldName:<tenantField>}}.

5. For atoms where "field" key is present (already bound) — DO NOT re-bind unless user explicitly asks.

6. When using a preset, check its default fieldNames (table above). If your tenant has matching
   fields (e.g. "name" exists) — leave as is. If tenant uses different keys — override via
   update ops targeted at the default field names (e.g. {update, target:"name", props:{fieldName:"model"}}).

The examples in the COMPOSING section below use hey-babes cosmetics fields. Your tenant's
field names come from <fields> — DO NOT copy example fieldNames literally.
```

Существующие примеры композиции (lines 115-143 в prompt_compose_widgets.go) — не трогаем содержательно, но добавляем к каждому комментарий `// hey-babes example — override fieldNames for other tenants via <fields>`. Этого достаточно чтобы LLM не залипала на буквальных ключах.

---

## План имплементации (шаги, не даты)

Каждый шаг — самодостаточный PR, независим, откатывается отдельно.

### Шаг 1 — conditional tree_map schema + `__bound` fix
**Что**: переписать `buildAtomMap` в `engine_v4/tree_ids.go` на conditional output (bound → short, open → rich). Попутно — в `buildAtomMap` вычислять `bound` по реальному состоянию atom.Value + atom.FieldName, не по Meta-флагу. Флаг `__bound` в Meta остаётся для `BindData` как он работает сейчас (отдельный inner mechanism).

**Файлы**: `project_v4/backend/internal/engine_v4/tree_ids.go`

**Verification**:
- `go build ./... && go test ./internal/engine_v4/...` — чисто
- `curl /debug/traces/?format=json&limit=3 | jq '.[0].agent2.promptSent'` — проверить что formation_tree.widgets[].template.atoms имеет новую форму
- Регрессия hey-babes: «покажи крема» → 23 виджета, поведение идентично, `promptSent` немного меньше (из-за compact bound атомов)

### Шаг 2 — `buildAgent2SystemPromptWithFields` helper
**Что**: новый метод в `agent2_execute.go`, зеркальный `Agent1ExecuteUseCase.buildSystemPromptWithDigest`. Sync.Map мемоизация. Загрузка field_definitions + один SQL на сэмплы. Формирование `<fields>` блока. Склейка с `Agent2ToolSystemPrompt`. Использование в `ChatWithToolsCached`.

**Файлы**: 
- `project_v4/backend/internal/usecases/agent2_execute.go` (новый helper + вызов)
- `project_v4/backend/internal/adapters/postgres/postgres_catalog.go` или новый метод (SQL сэмплы)
- возможно `project_v4/backend/internal/ports/catalog_port.go` (новый метод `SampleProductFields`)

**Verification**:
- Build + unit test helper на mock данных
- Trace: `agent2.systemPrompt` содержит `<fields>` блок, `systemPromptChars` вырос
- Cache: `agent2.cacheRead > 0` на 2+ turn'е в пределах сессии (доказывает что блок стабилен per-tenant)
- Регрессия hey-babes: поведение идентично (prompt instructions ещё не знают про `<fields>`, просто добавили текст)

### Шаг 3 — FIELD BINDING rule в Agent2 system prompt
**Что**: добавить секцию FIELD BINDING в `prompt_compose_widgets.go` перед существующими примерами. Пометить примеры как hey-babes-specific комментариями. Добавить таблицу дефолтных fieldName по пресетам.

**Файлы**: `project_v4/backend/internal/prompts/prompt_compose_widgets.go`

**Verification**:
- Build
- Три регрессии на хей-бейбс:
  1. rebuild «покажи крема» → 23 продукта с правильными связями (name/price/images/rating/brand), `agent2.toolInput` содержит `preset:"product_card"` без лишних overrides
  2. modify «сделай 2 колонки» → `{op:update, target:"formation", props:{columns:2}}`, атомы не тронуты
  3. modify «убери рейтинг» → `{op:delete, target:"rating"}`, остальные атомы не тронуты
- Caсh: prompt всё ещё кэшируется, `cacheRead > 0`

### Шаг 4 — synthetic tenant test (electronics)
**Что**: SQL-сид (или тест-сценарий в админке) создающий тенант `test-electronics` с 8-10 продуктами-ноутбуками. Поля: `model, manufacturer, price, rating, cover_image, cpu, ram, display_size, battery_life`. Зарегистрировать field_definitions.

Затем: `curl POST /api/v1/pipeline` с tenant=test-electronics, query=«покажи ноутбуки». Разбор trace.

**Файлы**: `project_v4/backend/scripts/seed_test_electronics.sql` (новый) или миграция в admin-side.

**Verification (критерии успеха)**:
- `agent2.toolInput` содержит override ops вида `{update, target:"name", props:{fieldName:"model"}}` (или аналогичные)
- `agent2.toolInput` НЕ содержит буквальных хей-бейбс полей (`name/brand/description` не должны лететь в биндинг для этого тенанта)
- Формация отрендерилась с 10 виджетами, каждый с заполненными title/price/hero атомами
- Data integrity: tile[0] показывает product[0], tile[1] показывает product[1], ..., tile[9] показывает product[9] (проверка через trace formation_tree или DOM inspection)
- Если первая итерация не сработала — итерируем промпт до 3 раз, потом эскалируем

### Шаг 5 — widening: books + furniture (optional confidence test)
**Что**: те же сиды для `test-books` (поля `title/author/published_year/cover/pages/isbn`) и `test-furniture` (поля `name/material/dimensions/images/price/color`). Те же запросы. Успех = PoC доказан, 4.3 закрывается done.

**Verification**: тот же checklist что в Шаге 4.

---

## Verification strategy — как понять что всё правильно

### Уровень 1 — регрессия (доказывает "не сломали")
На каждом шаге, три жёстких сценария хей-бейбс:
1. rebuild «покажи крема» — 23 виджета, preset:product_card, атомы с правильными fieldName
2. modify «2 колонки» — grid.cols=2, атомы не тронуты
3. modify «убери рейтинг» — rating атом удалён, остальное не тронуто

Метрики pass/fail:
- `agent2.cacheRead > 0`
- `agent2.promptSent` не вырос более чем на 20% от baseline
- `formation.widgets_count == 23`
- Bound атомы имеют непустой Value после рендера

### Уровень 2 — синтетика (доказывает "работает на новом")
Тест-тенант ноутбуков. Бинарный критерий:
- Agent2 override-ops содержат `model/manufacturer/cover_image` (tenant-specific)
- Все 10 виджетов отрисованы с non-null Value в title/price/hero
- Data integrity: уникальные значения в каждом tile, соответствуют данным

### Уровень 3 — domain shift (доказывает "универсально")
Два ортогональных тенанта (books, furniture). Один запрос каждому. Тот же prompt, тот же движок. Оба работают — PoC полный, 4.3 done.

### Инструменты
- `/debug/traces/?format=json` + `jq` — разбор turn'ов
- `curl /api/v1/pipeline` с `tenant_slug` в query или заголовке — ручные запросы
- Go unit tests на engine_v4 — на уровне `TestBindData_GenericTenant`, `TestBuildAtomMap_BoundVsOpen`, `TestBuildAgent2SystemPromptWithFields`

---

## Risks и open questions

### Technical risks
- **`ApplyOps` update на fieldName**: не подтверждено что update-op реально переписывает `props.fieldName` существующего атома. Проверить в начале Шага 3. Фикс (если нужен) — минорный.
- **Сэмплы через on-the-fly SQL**: если тенант с 100k+ продуктов и field_definitions содержит 30+ полей — запрос «LIMIT 3 + агрегаты» может стать тяжёлым. Пока не актуально (heybabes 967 продуктов), но держим в голове. Fallback — вариант (B) с precomputed samples.
- **Ambiguous fields**: тенант с `title` и `name` оба подходят для title-slot. LLM выбирает один, возможно не детерминированно. Mitigation на будущее: добавить «suggested primary» флаг в field_definitions или сортировать по приоритету (поле с более короткими сэмплами → title, с длинными → description).
- **Cache invalidation**: sync.Map не знает про админ-изменения field_definitions. До рестарта процесса — stale. Acceptable для MVP.

### Open questions (решаем по ходу)
1. **Где хранить `<fields>` блок**: в Agent2 system prompt (как сейчас спроектировано) vs расширить Agent1 catalog digest до общего `<tenant_context>` блока который оба агента читают. Второй вариант дедупит данные между двумя system prompt'ами, первый — проще начать.
2. **Нужен ли `ref:` в пресетах**: альтернативно можно вместо override по fieldName делать override по ref (`$title`, `$hero`). Requires: пресет-билдеры присваивать `Ref` атомам, `ApplyOps` резолвить `target:"$title"` в соответствующий атом. Чище для LLM (референсы не зависят от случайных fieldName), но требует модификации 12 пресетов. Отложено до пост-PoC.
3. **Детерминизм на edge cases**: при температуре 0 и стабильном promptsize LLM должна давать стабильные mapping'и между запусками. Проверяем эмпирически на Шаге 4.

### Scope исключения
- **Ambiguous fields resolution с приоритетами** — не в этом PoC
- **Ручной override в админке** — отдельный тикет, не блокер
- **Auto-detection новых полей при импорте** — задача #9.2 (admin import), решается в рамках админки а не этого PoC
- **Multi-entity типы** (services одновременно с products в одном `<fields>` блоке) — scope TBD, для PoC фокус только на products

---

## Связанные задачи в трекере

- **4.3 B7: role-based field resolution** — этот дизайн **заменяет** её реализацию. Закрываем 4.3 с пометкой «решено через metadata-driven binding вместо role schema» после успешного Шага 5.
- **1.2 Agent1 tenant digest** — уже сделано (`2026-04-09_01-21`, `2026-04-09_02-05`). Agent2 fields block — симметричное решение для Agent2.
- **2.3 replicateLimit guidance** — независимая задача, не блокер.
- **2.8 freestyle widgets route** — этот дизайн делает freestyle более естественным (LLM уже умеет матчить слоты на поля), облегчает будущую работу над 2.8.
- **9.2 admin import auto-schema** — после имплементации этого дизайна для новых тенантов нужен автоматический генератор field_definitions при импорте. Не в scope PoC, но направление работы.

---

## Итоговые принципы

1. **Один pipeline, пофиг какой тенант** — LLM не знает и не должна знать что это (косметика, ноуты, книги). Она видит атомы как слоты, поля как источники, и матчит одно на другое по семантике.
2. **Движок остаётся dumb binder** — ноль семантической логики. `atom.FieldName → data[fieldName]`. Всё.
3. **Пресеты — шаблоны, не жёсткие артефакты** — работают из коробки для домена которому соответствуют дефолты (хей-бейбс = косметика), override'ятся через ops для всего остального.
4. **Интеллект в одной точке** — LLM, в prompt. Не в БД, не в движке, не в админке. Это делает решение тестируемым, дебагируемым, и переносимым.
5. **Cache-first** — весь новый контекст живёт в cacheable system prompt, симметрично тому что мы уже сделали для Agent1 digest. Incremental cost = почти ноль после первого turn'а.
