# feature/engine-v4 — B3: explicit `replicate` flag + `limit`

**Branch**: `feature/engine-v4`
**Date**: 2026-04-06 14:51 UTC
**Commit**: `cd504d8` — "feat(v4): explicit replicate flag + limit in visual_assembly (B3)"
**Parent**: `b642fde`

---

## Context

В деплой-сессии 2026-04-04 всплыли два ключевых бага V4 движка:

1. Запрос "лендинг из первых трёх" → Agent2 строил одну красивую композицию, а движок автоматически реплицировал её на все 50 продуктов из `state.Data` → мусор.
2. Нет способа ограничить количество данных для репликации.

Причина — авто-условие в `engine.go:54`: `if len(Data) > 1 && len(Widgets) == 1 && len(Sections) == 0 → replicate`. Никакого контроля снаружи.

Этот апдейт выносит решение о репликации **наружу из движка в tool schema**. Agent2 теперь явно решает: нужен грид (replicate: true) или freestyle композиция (ничего не передаёт). Это соответствует философии ops-движка: всё явно, никакой магии.

Задача **B3** из `docs/PRE_LAUNCH_TASKS.md`. B2 (пресеты как параметр) и B4 (multi-widget) — отдельно, следом.

---

## What changed

### 1. `ExecuteInput` (engine_v4/types.go)

Добавлены два поля:

```go
// Replication control (B3) — explicit, no magic.
Replicate bool // if true, replicate a single widget template across data items
Limit     int  // max data items to use (0 = all)
```

### 2. Engine pipeline (engine_v4/engine.go, Step 4)

**Было**: авто-репликация, если `data > 1 && widgets == 1 && sections == 0`.

**Стало**:
- Сначала применяется `Limit`: `Data = Data[:Limit]`.
- Репликация только если `Replicate == true` И 1 виджет И 0 секций И данные есть.
- Если `Replicate == false` и виджет один — берётся только `data[0]` для биндинга (trim до 1), чтобы `BindData` корректно отработал. Это делает "freestyle из первых N продуктов" возможным: Agent2 строит ровно то что хочет, движок ничего не копирует.
- Остальные ветки (multi-widget, 0 widgets) движок теперь не трогает — replicate на них не действует.

### 3. Tool schema (tools/tool_visual_assembly.go)

Две новые property в `visual_assembly`:
- `replicate: boolean` — "If true, replicate the single widget template across data items..."
- `limit: integer` — "Max number of data items to use (0 or omitted = all)..."

Парсинг: `params["replicate"].(bool)`, `params["limit"].(float64)` (JSON numbers → float64).

### 4. Agent2 prompt (prompts/prompt_compose_widgets.go)

- Grid-пример теперь включает `replicate: true, limit: 12`.
- Секция PARAMETERS описывает оба поля.
- DECISION RULES #9, #10 — явно объясняют когда флаг нужен, а когда нет.
- ANTI-PATTERNS обновлён: "Do NOT forget replicate: true on search-result grids".

---

## Tool call — до и после

### "покажи крема" (грид из результатов поиска)

**До**:
```json
visual_assembly({
  ops: [...широкий набор ops для одного widget template...],
  layout: "grid",
  columns: 3
})
```
→ движок сам реплицировал на все 50 продуктов.

**После**:
```json
visual_assembly({
  ops: [...тот же widget template...],
  layout: "grid",
  columns: 3,
  replicate: true,
  limit: 12
})
```
→ движок реплицирует на 12 карточек (или меньше, если данных меньше).

### "лендинг из первых трёх" (freestyle композиция)

**До**: Agent2 строит 1 красивый лендинг, движок копирует его 50 раз → сломано.

**После**:
```json
visual_assembly({
  ops: [...freestyle с литералами + $data[0..2] подстановками...]
  // replicate не передан → false
})
```
→ движок биндит data[0] в единственный виджет, не копирует. Лендинг остаётся как задумано.

---

## Tests (behavior snapshots, non-assertive)

Новый файл `project_v4/backend/internal/engine_v4/replicate_behavior_test.go`. Тесты **не падают** — они печатают наблюдаемое поведение через `t.Logf`. Используются так:

```bash
cd project_v4/backend && go test -v -run TestBehavior_ ./internal/engine_v4/...
```

### Сценарии и текущие наблюдения (2026-04-06)

| Тест | Вход | Фактический выход |
|------|------|-------------------|
| `TestBehavior_NoReplicate_MultiData` | 1 template, 5 products, `Replicate=false` | **1 widget**, `EntityRef=product/p1`, забиндились атомы `p1` |
| `TestBehavior_Replicate_NoLimit` | 1 template, 5 products, `Replicate=true` | **5 widgets**, EntityRef p1..p5 |
| `TestBehavior_Replicate_WithLimit` | 1 template, 10 products, `Replicate=true, Limit=3` | **3 widgets**, p1..p3 |
| `TestBehavior_Replicate_LimitLargerThanData` | 5 products, `Limit=100` | **5 widgets** (limit не падает при переполнении) |
| `TestBehavior_NoReplicate_NoData` | Freestyle ops, data=nil | **1 widget** с литералом "Hello world", EntityRef=nil |
| `TestBehavior_Replicate_ZeroWidgets` | ops=0, data=3, `Replicate=true` | **0 widgets** (ничего не реплицируется — нет шаблона) |
| `TestBehavior_NoReplicate_MultiWidget` | 2 widgets в ops, data=4, `Replicate=false` | **2 widgets**, никто не тронут, EntityRef=nil |

Также обновлён существующий `TestReplicationPreservesLayout` — добавлен `Replicate: true` (он полагался на старое авто-поведение).

Все тесты pass: `go test ./internal/engine_v4/... → ok`.

---

## Files changed

| File | Change |
|------|--------|
| `project_v4/backend/internal/engine_v4/types.go` | +2 поля в `ExecuteInput` |
| `project_v4/backend/internal/engine_v4/engine.go` | Step 4 переписан: limit slicing + explicit replicate |
| `project_v4/backend/internal/tools/tool_visual_assembly.go` | Schema: `replicate`, `limit`; parsing в Execute |
| `project_v4/backend/internal/prompts/prompt_compose_widgets.go` | Grid-пример, PARAMETERS, DECISION RULES #9/#10, ANTI-PATTERNS |
| `project_v4/backend/internal/engine_v4/replicate_behavior_test.go` | **NEW** — 7 поведенческих тестов |
| `project_v4/backend/internal/engine_v4/ops_test.go` | `TestReplicationPreservesLayout` — `Replicate: true` |

---

## Known gaps (остаются после этого апдейта)

Этот апдейт закрывает **только** B3 из `PRE_LAUNCH_TASKS.md`. Остаётся:

- **B2** — пресеты как именованный параметр (`preset: "product_card_grid"`). Сейчас Agent2 каждый раз городит ops вручную.
- **B4** — multi-widget composition (несколько разных виджетов в одном tool call, каждый со своим `replicate`).
- **E1** — session-scoped preset overrides.
- **State carry-over** — старый formation из прошлых turn'ов не чистится.
- **A2** — Agent1 → Agent2 для UI без данных (freestyle ветка).

---

## Verification

Локально:
```bash
cd project_v4/backend
go build ./...                                             # ок
go test ./internal/engine_v4/...                           # all pass
go test -v -run TestBehavior_ ./internal/engine_v4/...    # снапшоты в stdout
```

На проде (после деплоя):
1. `/version` — проверить новый build hash.
2. "покажи крема" → в трейсе Agent2 tool input содержит `replicate: true`; виджетов в ответе ≥2 (замены для 50 — `limit`).
3. "лендинг из первых трёх" → в tool input `replicate` отсутствует / false; в ответе **1** виджет. Баг из 04-04 закрыт.
4. "покажи 5 кремов" → `replicate: true, limit: 5` → 5 карточек.

### Backwards compat caveat

Старый Agent2 промпт (до этого апдейта) НЕ передаёт `replicate` → default `false` → любой грид из поиска выродится в 1 карточку. **Деплоить атомарно**: бэкенд + обновлённый промпт вместе. Anthropic prompt cache инвалидируется при изменении текста промпта — первый запрос будет чуть дороже.
