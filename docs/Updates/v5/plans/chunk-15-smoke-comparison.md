# V5 — Chunk 15: Prod smoke + V4-vs-V5 latency/cost parity

## Context

Chunk 14 закрыл V5 deploy на Railway. Single-prompt smoke прошёл, но
больше ничего конкретного по quality / latency / cost vs V4 не известно.
Items 16 + 17 из `docs/v5-engine-plan.md`:
- **Item 16** — V4 vs V5 smoke на 20-30 promптах
- **Item 17** — production-region latency baseline

Цель чанка — собрать **числа** для решения «прод-свапить V5 или нет»:
- Falls V5 на realistic prompt suite или нет?
- Насколько V5 медленнее / быстрее V4 на проде?
- Cost per turn V5 vs V4 — равно или X% больше?
- Quality call оставляем за глаза Vlad'а (per-prompt JSON dumps).

## Locked-in decisions

- **25 prompts × 5 тэгов**: search (8), drill (4), modify (5), compose
  (4), edge (4). Mixed RU + EN — реальное поведение пользователей
  hey-babes-cosmetics.
- **Bash + Python runner** в `scripts/v5-smoke.sh`. Bash для
  оркестрации curl-ов и file I/O, Python для JSON manipulation
  (cleaner чем jq на nested fields).
- **Output**: `docs/v5-smoke/<UTC-stamp>/` — `summary.md` +
  `prompts.json` + `meta.json` коммитятся. Per-prompt
  `pNN.{v5,v4,v4.trace}.json` дампы остаются локально через
  `.gitignore` rule.
- **Quality assessment вручную** Vlad'ом. Никакой автоматики diff'а.
  summary.md показывает objective metrics (status, latency, doc/widget
  count, tool calls, tokens, cache, cost). Quality судит человек по
  конкретным JSON dumps.
- **V4 cost discovery через `/debug/traces/?format=json`**. V4
  pipeline response не отдаёт usage; V4 metricsStore хранит per-trace
  cost / tokens / spans → его дёргаем сразу после каждого pipeline
  call'а и матчим по sessionId + query.

## Approach

### 1. Prompt suite — `scripts/v5-smoke-prompts.json`

25 prompts с тэгами и id (p01–p25). Тэги:
- `search` (8): «покажи 3 продукта», «тонер для жирной кожи», «крем
  для сухой кожи», «что у вас есть для глаз», «недорогая сыворотка»,
  «новинки», «show me COSRX products», «очищающее средство до 1500
  рублей»
- `drill` (4): «открой первый», «детали третьего», «расскажи
  подробнее», «show me details»
- `modify` (5): «сделай заголовок красным», «уменьши карточки», «убери
  цены», «добавь рейтинги», «make titles bigger»
- `compose` (4): «сделай лендинг с этими продуктами», «покажи как
  landing page», «карточка крупно + остальные мелко», «hero блок + три
  карточки снизу»
- `edge` (4): «покажи квантовые компьютеры» (empty match), «...»
  (junk), «aaaaa» (gibberish), «привет» (greeting)

### 2. Runner — `scripts/v5-smoke.sh`

Pipeline:
1. Init session на V5 + V4 (две отдельные sessionId через
   `/api/v1/session/init`).
2. Для каждого prompt:
   - POST V5 `/api/v1/pipeline` → capture full response
     (`pNN.v5.json`) + client wall-clock latency.
   - POST V4 `/api/v1/pipeline` → capture (`pNN.v4.json`) + wall-clock.
   - GET V4 `/debug/traces/?format=json` → найти trace по sessionId +
     query, save `pNN.v4.trace.json` для cost/tokens/cache.
   - Sleep 1s между промптами.
3. Render `summary.md`: per-prompt таблица + aggregates (success rate,
   latency p50/p95/mean, total cost, avg cost/turn, avg input tokens,
   avg cache_read).

### 3. V4 trace JSON field names — caveat

V4 trace JSON использует **сокращённые** field names: `cacheRead` /
`cacheWrite`, не `cacheReadInputTokens` / `cacheCreationInputTokens`
как у V5. Первая итерация скрипта искала длинные имена и получала
нули — fixed.

### 4. Dry run первый

Маленький `--limit 2` run на /tmp/ для верификации скрипта без
бюджетной нагрузки. После того как 2 строки нарисовались правильно —
полный 25-prompt run.

## Critical files

| File | Change |
|---|---|
| `scripts/v5-smoke-prompts.json` | NEW (25 prompts × 5 tags) |
| `scripts/v5-smoke.sh` | NEW (bash runner + python summary renderer) |
| `.gitignore` | MODIFY (add `docs/v5-smoke/**/p*.json` rule) |
| `docs/v5-smoke/2026-05-03_18-47/{summary,prompts,meta}.{md,json}` | OUTPUT (committed) |
| `docs/v5-smoke/2026-05-03_18-47/p*.json` | OUTPUT (gitignored, local-only) |

## Verification

- 2-prompt dry run на /tmp/v5-smoke-dry → summary правильно нарисован
- 25-prompt full run против deployed V5 + V4 → 6:34 wall-clock,
  $0.13 V5 + $0.12 V4 ≈ $0.25 total
- summary.md показывает все 25 строк + aggregates с правильными
  cache numbers (после fix V4 field names)

## Headline numbers (this run)

| metric | V5 | V4 | Δ |
|---|---|---|---|
| Success rate | 25/25 | 25/25 | +0 |
| Latency p50 | 8166ms | 2608ms | **+213%** |
| Latency p95 | 10957ms | 6038ms | +82% |
| Avg cost/turn | $0.0053 | $0.0048 | +10% |
| Avg input tokens | 1122 | 1484 | -24% |
| Avg cache_read | 10909 | 10202 | +7% |

V5 гораздо медленнее V4 на проде (3× p50, 2× p95) при сопоставимой
стоимости. Token efficiency лучше у V5 (-24% input thanks to tree_map),
но выходящие токены compose-промптов раздувают cost (e.g. p19 «landing
page» V5 outputs 929 tokens). Quality остаётся subjective — за
JSON-dumps eyeball'ом Vlad решает swap-or-not.

## Known gaps

- **V4 cost — in-memory metricsStore**. `/debug/traces` отдаёт что
  висит в RAM. V4 рестарт между прогонами стирает старые traces. На
  prod без user traffic это не проблема для этого smoke.
- **No quality auto-diff**. V4 эмитит Formation, V5 — scene-graph
  Document. Auto-diff бы потребовал schema-aware comparator, не
  делаем.
- **One tenant**. `hey-babes-cosmetics` only. Multi-tenant smoke —
  отдельный chunk если/когда добавим тенантов.
- **Anthropic cache state**. V5 cache_read зависит от того, кэширован
  ли prefix. Этот run был «warm» (после chunk-14 deploy + другие
  smokes), latency могла бы быть ещё выше на cold start.

## Recommendation для chunk 16

V5 latency 3× медленнее — это серьёзный аргумент **не свапить V5 в
prod как drop-in замену V4**. Варианты пути вперёд:

- (A) **V5 как opt-in beta**: deploy виджет-флаг для select tenants
  чтобы трафик попал на V5 контролируемо. Quality и speed валидируем
  на реальных запросах.
- (B) **Closing latency gap**: профилировать V5 LLM calls (сейчас
  Agent1 + Agent2 sequential = 2x roundtrip). Возможно parallelize
  или skip Agent1 на простых modify-promptах.
- (C) **Wait и не свапить**: V4 продолжает обслуживать prod, V5 уходит
  в Stream B (canvas-microservice) до тех пор пока не научимся
  closing latency gap.

Голосование за свап — за тобой.
