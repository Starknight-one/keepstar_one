# ReplicateConfig persistence + Agent1 single-breakpoint cache

- **Branch**: `feature/engine-v4`
- **Date (UTC)**: 2026-04-09 02:05
- **Commit**: `64dc6aedaf182007eb404af476fd2b88ab81d225`
- **Parent**: `9926cc0` (docs log of previous cache audit session)

## Context

Вторая итерация по кэшу после деплоя предыдущего фикса (`403d1fe`). Пользователь
прислал две проблемы на сессии `8baf7cde-ad5d-47c7-afcc-2ce2c4168980`:

1. **Agent1 всё ещё 0% cache hit rate** — `systemPromptChars` вырос до ~5348
   как и ожидалось (дайджест встал в system prompt), но `cacheRead=0`,
   `cacheWrite=0` на всех turn'ах. Предыдущий фикс вернул дайджест, но кэш
   так и не активировался.
2. **Agent2 странное поведение на turn 2**: пользователь написал
   «Круто, а можешь только КОРСКС показать?» (опечатка COSRX) → deterministic
   `state_filter` bypass вернул 0 матчей → фронт показал 23 тайла с одним и
   тем же товаром вместо placeholder «не понял». Параллельно: Agent2
   `promptSent` раздулся до 25k chars на позднем turn'е — в `formation_tree`
   дампилось 50 полных копий одного literal-виджета.

## Findings

### Agent1 cache: split-breakpoint poisoning
В прошлой итерации `CacheConfig` для Agent1 был `CacheTools:true, CacheSystem:true, CacheConversation:true` — три breakpoint'а. Подсчёт:
- tools block (3 tools, ~1500 tokens) — **ниже 2048-token минимума Haiku 4.5**
- system+digest (~4000 tokens) — выше порога
- conversation — переменной длины

Гипотеза: Anthropic при первом невалидном breakpoint'е (tools < 2048) тихо
скипает всю цепочку — ни один блок не создаётся. Это объясняет 0/0 при
визуально «правильном» префиксе.

### ReplicateConfig lost in DB roundtrip
`domain.Widget.ReplicateConfig` имел `json:"-"`. В turn 1
`expandReplicatedWidgets` клонирует template N раз, каждому клону
проставляет `ReplicateConfig{Enabled:true, GroupID:"rg-N"}` + `Meta["__bound"]=true`.
Формация улетает в `state.Template["formation"]` → JSON → Postgres. На чтение
в turn 2 **`ReplicateConfig` теряется целиком**.

Последствия:
- `BuildTreeMap` (tree_ids.go) группирует клонов по `GroupID`. Без GroupID
  дедупа не происходит, каждый клон попадает в tree_map как отдельный
  `kind:"literal"` → Agent2 видит 50 идентичных literal-виджетов в
  `formation_tree` вместо одного `{kind:"replicated", count:50, template:...}`.
  Это и есть раздутие `promptSent` до 25k.
- `groupIntoSections` на modify-turn'е пересобирает 23 клона: т.к. у всех
  `ReplicateConfig=nil` → каждый виджет уходит в отдельную single-mode секцию
  (sections.go:64) → `formation.Mode = composed` → фронт рендерит 23
  вертикальные single-секции вместо одной grid-секции. Это же объясняет
  «все тайлы одного товара» — single-mode путь на фронте ведёт себя иначе
  чем grid и схлопывает визуал (верифицировано через trace: `a2_rawResp =
  "Formation rendered: 23 widgets, layout=composed"`, в `formation_tree` 23
  literal-виджета с одинаковой структурой без `ReplicateConfig`).

### Verification via trace
`curl .../debug/traces/?format=json&limit=50 | jq '... | .agent2'`:
- Turn 1 «покажи крема» → `{replicate:true, preset:"product_card", limit:23}`,
  `"Formation rendered: 23 widgets, layout=grid"` ✓
- Turn 2 «КОРСКС» → `{mode:"modify", ops:[{op:"update", props:{columns:2}, target:"formation"}]}`,
  `"Formation rendered: 23 widgets, layout=composed"` ✗ должен был остаться grid
- `formation_tree` в промпте turn 2: `widget_count:23`, все `kind:"literal"`
  — подтверждает что `GroupID` не переживает roundtrip

## Approach

1. **`ReplicateConfig` → `json:"replicate,omitempty"`** с полями `Enabled`,
   `Limit`, `DataIndex`, `GroupID` с json-тегами. Persist survives DB roundtrip.
   Комментарий над полем объясняет зачем (Agent2 BuildTreeMap dedupe +
   groupIntoSections rollback). Фронт игнорирует unknown props.

2. **Agent1 `CacheConfig`: single breakpoint.** Убрал `CacheTools:true`, оставил
   только `CacheSystem:true` + `CacheConversation`. С одним breakpoint'ом на
   конце system prompt он покрывает `[tools + system]` как один блок
   (~3700 tokens), уверенно выше 2048 минимума. Комментарий в коде объясняет
   почему split breakpoint ядовит.

3. Никаких изменений на пути Agent2 — у него tools+system+examples заведомо
   >2048, split breakpoint работает штатно.

## Files changed

| File | Change |
|---|---|
| `project_v4/backend/internal/domain/widget_entity.go` | `ReplicateConfig` теперь `json:"replicate,omitempty"` + tags на inner fields + док-комментарий |
| `project_v4/backend/internal/usecases/agent1_execute.go` | убран `CacheTools:true`, комментарий про split-breakpoint poisoning |

## Verification

**Локально**: `go build ./...` — чисто.

**На проде после деплоя**:
```bash
curl -s "https://v4-engine-production.up.railway.app/debug/traces/?format=json&limit=5" \
  | jq '.[0].agent1 | {sysChars: .systemPromptChars, write: .cacheWrite, read: .cacheRead}'
```
Ожидаемые изменения:
- На первом turn'е по тенанту в свежем процессе: `agent1.cacheWrite > 0`
  (раньше было 0).
- На втором+ turn'е в пределах 5 минут: `agent1.cacheRead > 0`.
- На multi-turn replicate grid'е (например «покажи крема» → «2 колонки»):
  turn 2 должен рендериться как `layout=grid`, а не `composed`. В
  `formation_tree` — один `{kind:"replicated", count:N, template:...}`.
- `promptSent` Agent2 на modify-turn'ах над replicate-гридом перестаёт
  скакать на 10k+ chars из-за дампа дубликатов.

## Known gaps / caveats

- **Old sessions in Redis/DB с pre-fix формациями останутся битыми** до TTL
  (30 мин) — их `ReplicateConfig=nil` прибит прошлой версией. Migration не
  делаем, естественно вымоются.
- **UX на пустом state_filter всё ещё неправильный**. Deterministic bypass
  возвращает `"empty: 0 results from 23 products, data preserved"`,
  микроконтекст для Agent2 формируется как `"filtered: N items"` (лжёт).
  Agent2 продолжает крутить тот же грид вместо `empty_not_found` preset.
  Отдельная задача на будущую сессию:
  - В `tool_state_filter.go` для 0-results возвращать отдельный маркер.
  - В pipeline микроконтекст-builder'е распознавать пустой результат и
    передавать `filter_empty` в Agent2.
  - В `Agent2ToolSystemPrompt` правило: `filter_empty` → `mode:"rebuild",
    preset:"empty_not_found"`.
- **Agent1 per-tenant cache memoization** — без инвалидации до рестарта
  процесса, унаследовано с прошлого фикса.
