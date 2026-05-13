# V5 prod smoke — 2026-05-03T18:47:03Z

- V5: https://v5-engine-production.up.railway.app
- V4: https://v4-engine-production.up.railway.app
- Tenant: `hey-babes-cosmetics`
- Started: 2026-05-03T18:47:03Z
- Prompts: 25 from `docs/v5-smoke/2026-05-03_18-47/prompts.json`
- **Run from**: MacBook → Railway prod.
- **⚠️ Region caveat**: at the time of this run V5 was deployed to
  Railway's **Singapore** region while Anthropic API + Neon DB sit in
  **us-east-1**. Every LLM call and every Postgres roundtrip from V5
  was trans-Pacific (~150-200ms RTT × multiple roundtrips per call).
  After the run Vlad migrated v5-engine to US region and a 4-prompt
  re-test showed V5 `latencyMs` drop from ~8000ms (this report) to
  **1881-2423ms** warm cache, **3446ms** cold — i.e. V5 latency was
  ~75% network and ~25% architecture. Numbers below remain a useful
  baseline of «V5 в плохом регионе vs V4 в правильном» but should NOT
  be read as steady-state V5 performance. Re-run suite after the
  region migration would replace these as the canonical baseline.

## Per-prompt

| # | tag | prompt | V5 | V5 lat | V5 doc | V5 tools | V5 in/out | V5 cache | V5 cost | V4 | V4 lat | V4 widgets | V4 in/out | V4 cache | V4 cost |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| p01 | search | покажи 3 продукта | ✅ | 7939 | 1 | visual_assembly(product_card/rebuild) | 3685/281 | 7546 | $0.0064 | ✅ | 4109 | 1 | 3792/157 | 0 | $0.0046 |
| p02 | search | тонер для жирной кожи | ✅ | 8365 | 1 | catalog_search(-/-), visual_assembly(product_card/rebuild) | 3990/197 | 8107 | $0.0058 | ✅ | 4350 | 3 | 4468/242 | 6893 | $0.0064 |
| p03 | search | крем для сухой кожи | ✅ | 8757 | 1 | catalog_search(-/-), visual_assembly(product_card/rebuild) | 4199/189 | 7546 | $0.0071 | ✅ | 2867 | 0 | 4559/200 | 6893 | $0.0070 |
| p04 | search | что у вас есть для глаз | ✅ | 9260 | 1 | catalog_search(-/-), visual_assembly(product_card/rebuild) | 4342/180 | 7546 | $0.0072 | ✅ | 3286 | 16 | 5221/213 | 7460 | $0.0072 |
| p05 | search | недорогая сыворотка | ✅ | 10254 | 1 | catalog_search(-/-), visual_assembly(product_card/rebuild) | 4484/220 | 7546 | $0.0076 | ✅ | 2739 | 17 | 782/254 | 6893 | $0.0089 |
| p06 | search | новинки | ✅ | 8166 | 1 | catalog_search(-/-), visual_assembly(product_card/rebuild) | 376/150 | 7546 | $0.0084 | ✅ | 4413 | 50 | 772/184 | 11103 | $0.0039 |
| p07 | search | show me COSRX products | ✅ | 7285 | 1 | _internal_state_filter(-/-), visual_assembly(product_card/rebuild) | 384/135 | 11817 | $0.0036 | ✅ | 2244 | 2 | 1084/186 | 11264 | $0.0042 |
| p08 | search | очищающее средство до 1500 рублей | ✅ | 5875 | 1 | _internal_state_filter(-/-), visual_assembly(product_card/rebuild) | 313/75 | 7546 | $0.0027 | ✅ | 690 | 0 | 597/55 | 6893 | $0.0025 |
| p09 | drill | открой первый | ✅ | 11401 | 1 | catalog_search(-/-), visual_assembly(product_detail/rebuild) | 370/207 | 11922 | $0.0040 | ✅ | 2234 | 50 | 446/221 | 11349 | $0.0037 |
| p10 | drill | детали третьего | ✅ | 5913 | 1 | visual_assembly(product_detail/rebuild) | 269/165 | 12041 | $0.0037 | ✅ | 2206 | 1 | 1030/126 | 11466 | $0.0038 |
| p11 | drill | расскажи подробнее | ✅ | 10957 | 1 | visual_assembly(product_detail/rebuild) | 269/659 | 12175 | $0.0059 | ✅ | 3974 | 1 | 684/303 | 11583 | $0.0042 |
| p12 | drill | show me details | ✅ | 7123 | 1 | visual_assembly(-/modify) | 412/259 | 12188 | $0.0048 | ✅ | 2381 | 1 | 674/209 | 11596 | $0.0037 |
| p13 | modify | сделай заголовок красным | ✅ | 6583 | 1 | visual_assembly(-/modify) | 440/95 | 12197 | $0.0043 | ✅ | 2224 | 1 | 686/123 | 11605 | $0.0033 |
| p14 | modify | уменьши карточки | ✅ | 8483 | 1 | visual_assembly(-/modify) | 436/397 | 12201 | $0.0051 | ✅ | 3255 | 1 | 682/246 | 11609 | $0.0039 |
| p15 | modify | убери цены | ✅ | 7072 | 1 | visual_assembly(-/modify) | 430/192 | 12211 | $0.0040 | ✅ | 2470 | 1 | 676/140 | 11619 | $0.0033 |
| p16 | modify | добавь рейтинги | ✅ | 6324 | 1 | visual_assembly(-/modify) | 413/153 | 12219 | $0.0038 | ✅ | 3381 | 1 | 667/269 | 11627 | $0.0040 |
| p17 | modify | make titles bigger | ✅ | 6570 | 1 | visual_assembly(-/modify) | 424/144 | 12224 | $0.0037 | ✅ | 2561 | 1 | 657/156 | 11632 | $0.0034 |
| p18 | compose | сделай лендинг с этими продуктами | ✅ | 8900 | 1 | visual_assembly(product_card/rebuild) | 446/333 | 12233 | $0.0047 | ✅ | 4870 | 12 | 679/443 | 11641 | $0.0049 |
| p19 | compose | покажи как landing page | ✅ | 9684 | 3 | visual_assembly(-/rebuild) | 223/929 | 12237 | $0.0074 | ✅ | 2608 | 0 | 721/154 | 11645 | $0.0035 |
| p20 | compose | карточка крупно + остальные мелко | ✅ | 7959 | 3 | visual_assembly(-/modify) | 376/365 | 12252 | $0.0055 | ✅ | 7071 | 0 | 3925/1013 | 11660 | $0.0110 |
| p21 | compose | hero блок + три карточки снизу | ✅ | 8868 | 2 | visual_assembly(-/rebuild) | 372/628 | 12259 | $0.0071 | ✅ | 6038 | 0 | 1175/747 | 11667 | $0.0075 |
| p22 | edge | покажи квантовые компьютеры | ✅ | 8134 | 1 | visual_assembly(empty_not_found/rebuild) | 489/256 | 12273 | $0.0053 | ✅ | 2166 | 50 | 859/192 | 11681 | $0.0051 |
| p23 | edge | ... | ✅ | 8454 | 1 | visual_assembly(product_card/rebuild) | 184/269 | 12285 | $0.0057 | ✅ | 1564 | 0 | 958/79 | 11693 | $0.0042 |
| p24 | edge | aaaaa | ✅ | 8650 | 1 | visual_assembly(-/modify) | 369/393 | 12298 | $0.0052 | ✅ | 2002 | 50 | 363/151 | 11786 | $0.0031 |
| p25 | edge | привет | ✅ | 3715 | 1 | visual_assembly(-/modify) | 367/83 | 12300 | $0.0033 | ✅ | 1809 | 0 | 953/92 | 11795 | $0.0034 |

## Aggregates

| metric | V5 | V4 | Δ |
|---|---|---|---|
| Success rate | 25/25 | 25/25 | +0 |
| Latency p50 (ms) | 8166 | 2608 | +213.1% |
| Latency p95 (ms) | 10957 | 6038 | +81.5% |
| Latency mean (ms) | 8028 | 3100 | +158.9% |
| Total cost (USD) | $0.1323 | $0.1205 | - |
| Avg cost/turn (USD) | $0.0053 | $0.0048 | +9.8% |
| Avg input tokens | 1122 | 1484 | -24.4% |
| Avg cache_read tokens | 10909 | 10202 | +6.9% |

## Qualitative comparison (sample)

8 представительных prompts, ручной разбор V5 document vs V4 formation:

| # | prompt | V5 | V4 | Кто лучше |
|---|---|---|---|---|
| p01 | «покажи 3 продукта» | frame 3ch wrap (3 product cards) | `mode=single 1w "Ничего не найдено"` (no tool call) | **V5** — V4 даже catalog_search не вызвал |
| p06 | «новинки» | frame 3ch (3 cards) | grid 50w (вывалил все 50 продуктов) | **V5** — V4 заваливает экран |
| p09 | «открой первый» | frame 2ch (re-search, не drill) | grid 50w (тоже не drill) | **Tie** — оба не справились с drill (нет prior products) |
| p13 | «сделай заголовок красным» | frame 4ch (modify-mode) | mode=single 1w 8 atoms | **Tie** — оба что-то применили |
| p18 | «сделай лендинг с продуктами» | frame 1ch wrap (composed) | grid 12w | **V4** показал больше деталей; V5 сжал |
| p20 | «карточка крупно + остальные мелко» | 3 frames (hero + cards + extra) | **0 widgets** | **V5** — V4 fail |
| p22 | «квантовые компьютеры» | empty_not_found preset (graceful) | grid 50w (snowboard spam) | **V5** — V4 показывает мусор на empty match |
| p25 | «привет» | frame 3ch (greeting) | **0 widgets** | **V5** — V4 не обрабатывает greeting |

**Pattern**: V5 эмитит структурированный intentional output (правильный
preset под intent: empty_not_found, product_card replicate, composed
frames). V4 либо дампит сырой grid 50 widgets (без фильтра), либо
показывает 0 widgets на compose / greeting. На «нормальных»
search-промптах V4 быстрее и часто адекватнее, но edge cases / compose
он валит, а V5 нет.

**Net**: V5 latency ~3× хуже на server-side, **но quality на edge cases
и compose заметно лучше**. Два разных продуктовых профиля.
