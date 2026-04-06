# feature/engine-v4 — B2: Named presets as visual_assembly parameter

**Branch**: `feature/engine-v4`
**Date**: 2026-04-06 15:35 UTC
**Commit**: `cc4af5f` — "feat(v4): named presets as visual_assembly parameter (B2)"
**Parent**: `4a68094` (B3 session log)

---

## Context

До этой сессии в движке было ровно 2 набора ops (`ProductCardGridOps`, `ProductDetailOps`) — hardcoded в `default_ops.go`. Их вызывали usecases и handlers напрямую, **Agent2 к ним доступа не имел**. Каждый turn LLM конструировал ops с нуля:

- ~10 ops на каждую карточку ≈ 500+ токенов выхода впустую
- layout плавал между turn'ами (LLM каждый раз выбирала gap/order/textStyle по-разному)
- блокировала кастомизацию тенантами (AD1), session overrides (E1), multi-widget композиции (B4 — без пресета Agent2 физически не успел бы описать два сложных виджета)

Цель B2 — сделать пресеты первым классом `visual_assembly`: Agent2 передаёт `preset: "name"` + опциональные override ops, движок раскрывает в batch и применяет.

Адаптивность под произвольные каталоги (B7) **отложена** — обсудили, гипотеза "роли в field_definitions" принята, но реализация после. Пока все пресеты с hardcoded fieldName под косметику. Реестр, tool schema и промпт переживут переход к B7 без переделок — поменяются только тела функций-пресетов.

Список пресетов утверждён пользователем: 6 product card вариантов + 3 системных + 3 навигационных = 12 штук.

---

## Механика override через concat

Ключевая находка: `ApplyOps` (engine_v4/ops.go) обрабатывает один batch ops и держит `$ref` биндинги **в рамках этого batch**. Значит если движок просто сделает:

```go
engineInput.Ops = append(presetOps, userOps...)
```

— пользовательские ops увидят `$w`, `$root`, `$info`, `$meta` из пресета и смогут на них ссылаться в `parent`. Update по `target: "price"` (fieldName) тоже работает на получившейся formation как обычно. **Никаких новых механизмов не понадобилось** — ни отдельного override engine, ни двух проходов ApplyOps. Одна строка concat.

---

## Реестр пресетов

Новые файлы:

- `project_v4/backend/internal/engine_v4/presets.go` — структура `Preset{Name, Description, Category, Refs, DefaultReplicate, Build}` + `RegisterPreset/GetPreset/ListPresets/PresetNames`
- `project_v4/backend/internal/engine_v4/presets_product.go` — 6 product-пресетов + `init()`
- `project_v4/backend/internal/engine_v4/presets_system.go` — 3 системных + `init()`
- `project_v4/backend/internal/engine_v4/presets_nav.go` — 3 навигационных + `init()`

`default_ops.go` урезан: `ProductCardGridOps`/`ProductDetailOps` теперь thin wrappers над `GetPreset("product_card")`/`GetPreset("product_detail")`. Usecases (navigation_back, action_view, pipeline_execute, navigation_expand, handler_debug, handler_testbench) **не тронуты** — нулевой риск регрессии в навигации и fallback путях. `DefaultWidgetActions` и `GridColumnsForCount` остались на месте.

### Таблица пресетов

| Name | Category | DefaultReplicate | Ops | Назначение |
|---|---|---|---|---|
| `product_card` | product | true | 9 | Обычная карточка для грида 2–4 штук (medium, vertical column, image 4:3) |
| `product_card_compact` | product | true | 7 | Маленькая карточка для грида 5+ (small, image 1:1, только title + price) |
| `product_card_horizontal` | product | true | 9 | Фото слева 1:1 + info справа. Для carousel или отдельного размещения |
| `product_card_list_row` | product | true | 10 | Широкая строка для `layout: list`. Image + title + description (2 lines) + price + rating + brand |
| `product_detail` | product | false | 13 | Детальная вертикальная: hero 16:9, price row (xl), description 6 lines, category+tags |
| `product_detail_horizontal` | product | false | 13 | Детальная горизонтальная: hero 4:3 слева, info column справа, description 8 lines |
| `text_explainer` | system | false | 4 | Виджет с литеральным title + body. Для LLM-объяснений. Ref'ы `title`/`body` для override |
| `empty_not_found` | system | false | 5 | Empty state: search icon + headline + subtext. Литералы, override через ref'ы `headline`/`subtext` |
| `error_generic` | system | false | 5 | Error state: alert icon (red) + headline + subtext |
| `catalog_category_card` | nav | true | 6 | Карточка группы каталога: image + name + count. Биндится `images/name/count` (count пока placeholder — catalog entity появится позже) |
| `liked_grid` | nav | true | 9 | Алиас на `product_card` с пометкой nav — чтобы E2 pre-built навигация могла ссылаться по nav-specific имени |
| `cart_grid` | nav | true | 9 | Алиас на `product_card_horizontal`. **Без** виджета тотала — multi-widget композиция в B4 |

### Pефы у product-пресетов

Все 6 product-вариантов экспортируют один и тот же набор `$w`, `$root`, `$info`, `$meta` → override ops предсказуемы независимо от того какую карточку выбрал Agent2. System-пресеты экспортируют дополнительные text-ref'ы (`title`/`body`/`headline`/`subtext`), чтобы Agent2 мог делать `{op: update, target: "headline", props: {value: "..."}}`.

---

## Tool schema delta

`project_v4/backend/internal/tools/tool_visual_assembly.go`:

```go
"preset": map[string]interface{}{
    "type":        "string",
    "enum":        engine_v4.PresetNames(),   // генерится из реестра
    "description": "Named preset — expands to a prebuilt ops batch...",
},
```

В `Execute()` добавлена обработка перед build-from-scratch детектором:

```go
_, replicatePassed := params["replicate"]
// ... replicate/limit парсинг ...
if presetName, ok := params["preset"].(string); ok && presetName != "" {
    p, found := engine_v4.GetPreset(presetName)
    if !found {
        return &domain.ToolResult{Content: fmt.Sprintf("unknown preset: %q", presetName)}, nil
    }
    engineInput.Ops = append(p.Build(), engineInput.Ops...)
    if !replicatePassed {
        engineInput.Replicate = p.DefaultReplicate
    }
}
```

Ключевая деталь: `replicatePassed` определяется **до** присвоения (через проверку наличия ключа в map), чтобы отличить "Agent2 явно передал replicate=false" от "Agent2 не передавал, унаследуй дефолт".

---

## Prompt delta

`project_v4/backend/internal/prompts/prompt_compose_widgets.go`:

1. Новая секция **PRESETS** между HOW IT WORKS и BUILDING FROM SCRATCH — список всех 12 имён с описанием + 3 примера (preset only / preset + overrides / system preset с литералами).
2. В **PARAMETERS** добавлен `preset` и обновлено описание `replicate` (объяснено что с preset наследуется DefaultReplicate).
3. В **DECISION RULES** — правило #11 "PREFER PRESETS".
4. В **ANTI-PATTERNS** — "Do NOT hand-roll ops when a preset covers the case."

---

## Tool call — до и после

### "покажи крема" (грид из поиска)

**До B2**:
```json
visual_assembly({
  ops: [
    {"op":"insert","ref":"w","parent":"formation","props":{...}},
    {"op":"insert","ref":"root","parent":"$w","props":{...}},
    {"op":"insert","parent":"$root","props":{"type":"image","fieldName":"images",...}},
    {"op":"insert","ref":"info","parent":"$root","props":{...}},
    {"op":"insert","parent":"$info","props":{"type":"text","fieldName":"name",...}},
    {"op":"insert","ref":"meta","parent":"$info","props":{...}},
    {"op":"insert","parent":"$meta","props":{"type":"number","fieldName":"price",...}},
    {"op":"insert","parent":"$meta","props":{"type":"number","fieldName":"rating",...}},
    {"op":"insert","parent":"$info","props":{"type":"text","fieldName":"brand",...}}
  ],
  layout: "grid", columns: 3, replicate: true, limit: 12
})
```
~500+ выходных токенов.

**После B2**:
```json
visual_assembly({
  preset: "product_card",
  layout: "grid",
  columns: 3,
  limit: 12
})
```
~40 выходных токенов. `replicate` не нужен — `product_card.DefaultReplicate=true`.

### "покажи крема с красной ценой"

```json
visual_assembly({
  preset: "product_card",
  ops: [
    {"op":"update","target":"price","props":{"textStyle":{"color":"red","fontWeight":"bold"}}}
  ],
  layout: "grid", columns: 3, limit: 12
})
```

### "ничего не найдено"

```json
visual_assembly({
  preset: "empty_not_found",
  ops: [
    {"op":"update","target":"headline","props":{"value":"No creams match your filters"}},
    {"op":"update","target":"subtext","props":{"value":"Try removing some of them."}}
  ]
})
```

---

## Тесты (behavior snapshots)

Новый файл `project_v4/backend/internal/engine_v4/presets_behavior_test.go`. Non-assertive, всё через `t.Logf`.

```bash
cd project_v4/backend && go test -v -run TestPresetBehavior_ ./internal/engine_v4/...
```

### Что проверяется

| Тест | Что делает | Наблюдаемый вывод (2026-04-06) |
|---|---|---|
| `TestPresetBehavior_AllBuildSuccessfully` | sub-тест на каждый из 12 пресетов: Build() → Execute() с sample data (1 или 5 продуктов) → лог formation + атомов первого виджета | Все 12 PASS, непустые formation, 0 warnings. Atom counts: product_card=5, compact=3, horizontal=5, list_row=6, detail=8, detail_horizontal=8, text_explainer=2, empty_not_found=3, error_generic=3, catalog_category_card=3, liked_grid=5, cart_grid=5 |
| `TestPresetBehavior_OverrideRedPrice` | product_card + `{update price color red}` | Все 3 виджета: `price.textStyle.Color = red`, `FontWeight = bold` |
| `TestPresetBehavior_OverrideDeleteField` | product_card + `{delete rating}` | Атомов с 5 → 4 у каждого виджета, rating пропал |
| `TestPresetBehavior_OverrideInsertChild` | product_card + insert text "NEW!" в `$info` | +1 литеральный text атом в каждом виджете |
| `TestPresetBehavior_SystemPreset` | text_explainer + update `title`/`body` literal values | Оба атома с новыми значениями |
| `TestPresetBehavior_UnknownPreset` | `GetPreset("no_such_preset")` | `ok=false` |

Все тесты PASS. Смотреть подробный вывод — `go test -v`.

---

## Files changed

| File | Type | Change |
|---|---|---|
| `project_v4/backend/internal/engine_v4/presets.go` | NEW | Реестр + тип Preset + helpers |
| `project_v4/backend/internal/engine_v4/presets_product.go` | NEW | 6 продуктовых пресетов |
| `project_v4/backend/internal/engine_v4/presets_system.go` | NEW | 3 системных пресета |
| `project_v4/backend/internal/engine_v4/presets_nav.go` | NEW | 3 навигационных пресета |
| `project_v4/backend/internal/engine_v4/default_ops.go` | MODIFIED | Урезан до thin wrappers + helpers |
| `project_v4/backend/internal/engine_v4/presets_behavior_test.go` | NEW | 6 behavior-тестов (non-assertive) |
| `project_v4/backend/internal/tools/tool_visual_assembly.go` | MODIFIED | `preset` в schema + parsing в Execute |
| `project_v4/backend/internal/prompts/prompt_compose_widgets.go` | MODIFIED | Секция PRESETS + decision rule #11 + anti-pattern |

Итого: 8 files, +650 / -29.

---

## Known gaps / caveats

1. **Hardcoded fieldName** (главное). Product-пресеты прибиты к `images/name/price/rating/brand/description/category/tags`. Для ноутбучного каталога `rating`/`brand` останутся пустыми атомами, `cpu`/`ram` не появятся вообще. Закрывается в **B7** (роли в field_definitions → slot resolver). Реестр и tool schema при этом не меняются.

2. **Cart без тотала**. `cart_grid` — только грид карточек, без виджета общей стоимости и скидок. Нужен multi-widget (**B4**) для композиции из двух виджетов.

3. **Catalog entity отсутствует**. `catalog_category_card` биндится на `count` поле которого нет у `Product` — в практике count будет 0 до появления реальной catalog entity.

4. **System пресеты в тестах получают EntityRef**. Артефакт теста — я передаю `sampleProducts(1)` в `TestPresetBehavior_AllBuildSuccessfully` и для `empty_not_found` движок ставит entity=product/p1 на widget. В реальном использовании Agent2 не передаёт data для freestyle — EntityRef не ставится.

5. **Prompt cache invalidation**. Обновление системного промпта инвалидирует Anthropic prompt cache → первый запрос после деплоя будет дороже.

6. **Backwards compat**. Старый Agent2 (до нового промпта) продолжит работать freestyle — никакого падения. Но без preset-пути и не почувствует экономию токенов.

---

## Verification

### Локально

```bash
cd project_v4/backend
go build ./...                                                  # ок
go test ./internal/engine_v4/...                                # all pass
go test -v -run TestPresetBehavior_ ./internal/engine_v4/...   # снапшоты в stdout
```

### На проде (после деплоя)

1. `/version` → новый build hash.
2. "покажи крема" → в трейсе Agent2 tool input содержит `preset: "product_card"` (не `ops: [...]` на 500 токенов). Получается грид как и сейчас.
3. "покажи крема компактно" → `preset: "product_card_compact"` → мелкие карточки.
4. "детали первого" → `preset: "product_detail"` → 1 большой виджет.
5. "покажи крема но цена красная" → preset + override op → красная цена на всех.
6. Пустой результат поиска → `preset: "empty_not_found"` с overridden headline/subtext.

### Как добавить новый пресет

1. Написать функцию-builder в одном из `presets_*.go` (возвращает `[]Op`).
2. Зарегистрировать в `init()` через `RegisterPreset`.
3. `PresetNames()` автоматически подхватит → в tool schema enum попадёт.
4. Обновить prompt: список + описание + пример если особый случай.
5. Добавить в `TestPresetBehavior_AllBuildSuccessfully` ничего не надо — он итерируется по всем зарегистрированным.
