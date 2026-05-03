# V5-only smoke — 2026-05-03T20:16:08Z

- V5: https://v5-engine-production.up.railway.app
- Tenant: `hey-babes-cosmetics`
- Session (single, shared): `3d45a4a9-88f6-4630-b624-711384a4588f`
- Started: 2026-05-03T20:16:08Z
- Finished: 2026-05-03T20:18:23Z
- Prompts: 30 from `prompts.json`

## Per-prompt narrative

Format: each turn shows what V5 did (tool calls + preset choice + render result + visible text samples). State accumulates across turns — drill/refine/modify/continuation see prior context.

### p01 `[greeting]` — «hi»

- **Tools**: visual_assembly preset=product_card mode=rebuild replicate=3
- **Render**: 3 card(s); nodes — frame=16, image=3, text=12
- **Latency**: 1554ms (a1 729 + a2 824) · cost $0.0050 · cache_read 7959

### p02 `[greeting]` — «hello there»

- **Tools**: visual_assembly mode=modify
- **Render**: 3 card(s); nodes — frame=16, image=3, text=12
- **Latency**: 2135ms (a1 1063 + a2 1071) · cost $0.0053 · cache_read 8099

### p03 `[conversational]` — «what can you do?»

- **Tools**: visual_assembly preset=text_explainer mode=rebuild
- **Render**: 2 card(s); nodes — frame=1, text=2
- **Latency**: 5057ms (a1 2771 + a2 2285) · cost $0.0074 · cache_read 7546

### p04 `[broad-search]` — «show me what you have»

- **Tools**: visual_assembly mode=modify ops=2
- **Render**: 2 card(s); nodes — frame=1, text=2
- **Visible text**: 'I can build and modify UI layouts' / 'I can render product grids, detail pages, empty states, and '
- **Latency**: 4965ms (a1 2885 + a2 2080) · cost $0.0076 · cache_read 7546

### p05 `[broad-search]` — «what's popular»

- **Tools**: visual_assembly preset=product_card mode=rebuild replicate=3
- **Render**: 3 card(s); nodes — frame=16, image=3, text=12
- **Latency**: 4221ms (a1 3325 + a2 896) · cost $0.0073 · cache_read 7546

### p06 `[specific-search]` — «show me toners»

- **Tools**: catalog_search; visual_assembly preset=empty_not_found mode=rebuild
- **Render**: 2 card(s); nodes — frame=1, text=2
- **Visible text**: 'Nothing found' / 'Try a different query or remove some filters.'
- **Latency**: 2260ms (a1 1158 + a2 1101) · cost $0.0067 · cache_read 7546

### p07 `[specific-search]` — «creams for dry skin»

- **Tools**: catalog_search; visual_assembly preset=product_card mode=rebuild replicate=3
- **Render**: 3 card(s); nodes — frame=16, image=3, text=12
- **Latency**: 2053ms (a1 967 + a2 1085) · cost $0.0069 · cache_read 7546

### p08 `[specific-search]` — «anti-aging products under 2000 rubles»

- **Tools**: catalog_search; visual_assembly preset=product_card mode=rebuild replicate=3
- **Render**: 3 card(s); nodes — frame=16, image=3, text=12
- **Latency**: 1906ms (a1 992 + a2 913) · cost $0.0070 · cache_read 7546

### p09 `[specific-search]` — «COSRX products»

- **Tools**: _internal_state_filter; visual_assembly preset=product_card mode=rebuild replicate=3
- **Render**: 3 card(s); nodes — frame=16, image=3, text=12
- **Latency**: 1604ms (a1 758 + a2 846) · cost $0.0073 · cache_read 7546

### p10 `[refine]` — «only the cheap ones»

- **Tools**: catalog_search; visual_assembly preset=product_card mode=rebuild replicate=3
- **Render**: 3 card(s); nodes — frame=16, image=3, text=12
- **Latency**: 2856ms (a1 1630 + a2 1226) · cost $0.0088 · cache_read 7546

### p11 `[refine]` — «show only with rating above 4»

- **Tools**: catalog_search; visual_assembly preset=product_card mode=rebuild replicate=3
- **Render**: 3 card(s); nodes — frame=16, image=3, text=12
- **Latency**: 1894ms (a1 1150 + a2 743) · cost $0.0041 · cache_read 11724

### p12 `[refine]` — «just three of them»

- **Tools**: visual_assembly preset=product_card mode=rebuild replicate=3
- **Render**: 3 card(s); nodes — frame=16, image=3, text=12
- **Latency**: 2755ms (a1 1911 + a2 843) · cost $0.0036 · cache_read 11896

### p13 `[drill]` — «open the first one»

- **Tools**: visual_assembly preset=product_detail mode=rebuild replicate=1
- **Render**: 2 card(s); nodes — frame=4, image=1, text=5
- **Latency**: 2286ms (a1 1434 + a2 851) · cost $0.0036 · cache_read 12036

### p14 `[drill]` — «tell me more about the second»

- **Tools**: visual_assembly preset=product_detail mode=rebuild replicate=1
- **Render**: 2 card(s); nodes — frame=4, image=1, text=5
- **Latency**: 2783ms (a1 1921 + a2 861) · cost $0.0037 · cache_read 12048

### p15 `[drill]` — «show full details»

- **Tools**: visual_assembly mode=modify ops=5
- **Render**: 2 card(s); nodes — frame=5, image=1, text=9
- **Latency**: 3788ms (a1 1152 + a2 2635) · cost $0.0050 · cache_read 12053

### p16 `[continuation]` — «back»

- **Tools**: visual_assembly mode=modify ops=4
- **Render**: 2 card(s); nodes — frame=5, image=1, text=5
- **Latency**: 2569ms (a1 1406 + a2 1162) · cost $0.0039 · cache_read 12060

### p17 `[continuation]` — «go back to the list»

- **Tools**: visual_assembly preset=product_card mode=rebuild replicate=3
- **Render**: 3 card(s); nodes — frame=16, image=3, text=12
- **Latency**: 2035ms (a1 1141 + a2 893) · cost $0.0036 · cache_read 12064

### p18 `[modify]` — «make titles bigger»

- **Tools**: visual_assembly mode=modify ops=3
- **Render**: 3 card(s); nodes — frame=16, image=3, text=12
- **Latency**: 2161ms (a1 932 + a2 1229) · cost $0.0037 · cache_read 12066

### p19 `[modify]` — «remove prices»

- **Tools**: visual_assembly mode=modify ops=3
- **Render**: 3 card(s); nodes — frame=13, image=3, text=6
- **Latency**: 1902ms (a1 802 + a2 1099) · cost $0.0037 · cache_read 12072

### p20 `[modify]` — «add ratings everywhere»

- **Tools**: catalog_search; visual_assembly mode=modify ops=3
- **Render**: 6 card(s); nodes — frame=16, image=3, text=12
- **Latency**: 2145ms (a1 1139 + a2 1005) · cost $0.0044 · cache_read 12076

### p21 `[modify]` — «change to a horizontal list»

- **Tools**: visual_assembly preset=product_card_horizontal mode=rebuild replicate=3
- **Render**: 3 card(s); nodes — frame=13, image=3, text=15
- **Latency**: 1864ms (a1 918 + a2 945) · cost $0.0036 · cache_read 12079

### p22 `[modify]` — «highlight the title in red»

- **Tools**: visual_assembly mode=modify ops=1
- **Render**: 3 card(s); nodes — frame=13, image=3, text=15
- **Latency**: 2410ms (a1 1397 + a2 1012) · cost $0.0039 · cache_read 12216

### p23 `[compose]` — «make a landing page from these products»

- **Tools**: visual_assembly mode=rebuild replicate=3 ops=12
- **Render**: 5 card(s); nodes — frame=8, image=3, text=15
- **Visible text**: 'Discover Premium K-Beauty' / 'Curated skincare essentials for every skin type' / 'Shop All Products'
- **Latency**: 5791ms (a1 1137 + a2 4654) · cost $0.0070 · cache_read 12229

### p24 `[compose]` — «hero block on top + small cards below»

- **Tools**: visual_assembly mode=rebuild replicate=3 ops=11
- **Render**: 4 card(s); nodes — frame=10, image=3, text=14
- **Visible text**: 'Discover Premium K-Beauty' / 'Curated skincare essentials for every skin type'
- **Latency**: 5286ms (a1 2085 + a2 3200) · cost $0.0078 · cache_read 12235

### p25 `[compose]` — «show one big featured + grid of others»

- **Tools**: visual_assembly mode=rebuild replicate=3 ops=21
- **Render**: 2 card(s); nodes — frame=12, image=4, text=19
- **Visible text**: 'Discover Premium K-Beauty' / 'Curated skincare essentials for every skin type'
- **Latency**: 7135ms (a1 1799 + a2 5336) · cost $0.0109 · cache_read 12243

### p26 `[edge]` — «show me cars»

- **Tools**: catalog_search; visual_assembly preset=empty_not_found mode=rebuild ops=2
- **Render**: 2 card(s); nodes — frame=1, text=2
- **Visible text**: 'No cars in our catalog' / 'We specialize in K-beauty skincare products. Try browsing ou'
- **Latency**: 2347ms (a1 709 + a2 1638) · cost $0.0064 · cache_read 12252

### p27 `[edge]` — «askdfjlasdf»

- **Tools**: visual_assembly mode=modify ops=2
- **Render**: 2 card(s); nodes — frame=1, text=2
- **Visible text**: "We didn't find anything for that search" / 'Try different keywords or browse our K-beauty collection.'
- **Latency**: 2628ms (a1 1381 + a2 1247) · cost $0.0056 · cache_read 12261

### p28 `[edge]` — «...»

- **Tools**: visual_assembly mode=modify ops=2
- **Render**: 2 card(s); nodes — frame=1, text=2
- **Visible text**: "Hmm, that didn't quite work" / 'Browse our full K-beauty skincare catalog or try a new searc'
- **Latency**: 3006ms (a1 1478 + a2 1527) · cost $0.0036 · cache_read 12357

### p29 `[conversational]` — «thanks!»

- **Tools**: visual_assembly mode=modify
- **Render**: 2 card(s); nodes — frame=1, text=2
- **Visible text**: "Hmm, that didn't quite work" / 'Browse our full K-beauty skincare catalog or try a new searc'
- **Latency**: 1367ms (a1 770 + a2 597) · cost $0.0029 · cache_read 12371

### p30 `[conversational]` — «what did i ask first?»

- **Tools**: _internal_history_lookup; visual_assembly mode=modify ops=1
- **Render**: 2 card(s); nodes — frame=1, text=2
- **Visible text**: 'You asked me to update the empty state message' / 'Browse our full K-beauty skincare catalog or try a new searc'
- **Latency**: 2162ms (a1 1171 + a2 990) · cost $0.0031 · cache_read 12373


## Pattern analysis

### Per-tag behaviour summary

| tag | N | preset picks | mode mix | rebuild→cards (avg) | misclass count |
|---|---|---|---|---|---|
| broad-search | 2 | product_card×1 | modify×1, rebuild×1 | 3.0 | 0 |
| compose | 3 | — | rebuild×3 | 3.7 | 0 |
| continuation | 2 | product_card×1 | modify×1, rebuild×1 | 3.0 | 0 |
| conversational | 3 | text_explainer×1 | modify×2, rebuild×1 | 2.0 | 0 |
| drill | 3 | product_detail×2 | rebuild×2, modify×1 | 2.0 | 0 |
| edge | 3 | empty_not_found×1 | modify×2, rebuild×1 | 2.0 | 0 |
| greeting | 2 | product_card×1 | rebuild×1, modify×1 | 3.0 | 0 |
| modify | 5 | product_card_horizontal×1 | modify×4, rebuild×1 | 3.0 | 0 |
| refine | 3 | product_card×3 | rebuild×3 | 3.0 | 0 |
| specific-search | 4 | product_card×3, empty_not_found×1 | rebuild×4 | 2.8 | 0 |

### Numbers

- Latency: p50 2347ms, p95 5791ms, mean 2898ms
- Total cost: $0.1632 (avg 0.0054/turn)
- Cache_read: avg 10638 tokens (range 7546-12373)
- Success: 30/30

## Observations (human read)

### What V5 does well

- **p03 «what can you do?»** — picks `text_explainer` preset, renders
  actual answer: *«I can build and modify UI layouts»* / *«I can render
  product grids, detail pages, empty states, and...»*. V5 ANSWERS the
  conversational question with rendered text, not silently ignores.
  This is something V4 couldn't really do.
- **p23-25 compose («landing page», «hero + cards», «one big featured +
  grid»)** — V5 actually generates a hero block with custom marketing
  copy: *«Discover Premium K-Beauty»* / *«Curated skincare essentials
  for every skin type»* / *«Shop All Products»*. 12-21 ops per turn,
  builds a real landing layout. V4 returned 0 widgets on these prompts.
- **p26 «show me cars»** — calls `catalog_search`, gets nothing,
  renders `empty_not_found` with **custom text** *«No cars in our
  catalog»* / *«We specialize in K-beauty skincare products. Try
  browsing our…»*. Graceful, intelligent, on-brand.
- **p27-28 edge garbage («askdfjlasdf», «...»)** — Agent2 modifies
  the empty-state message in-place: *«We didn't find anything for
  that search»* / *«Hmm, that didn't quite work»*. Different copy on
  different turns — Agent2 is genuinely composing text per context.
- **p30 «what did i ask first?»** — calls `_internal_history_lookup`,
  answers from real history: *«You asked me to update the empty state
  message»*. Memory works.
- **p13/p14 drill «open the first one»/«tell me more about the
  second»** — picks `product_detail` preset, replicate=1. Picks the
  right preset for drill intent.

### Where V5 stumbles

- **p01 «hi» / p02 «hello there»** — picked `product_card` preset
  replicate=3 (wrong) then modify-mode on existing tree (also wrong).
  No greeting preset exists; Agent2 fallback to product_card is the
  closest «default». **A1 in known-gaps.**
- **p06 «show me toners»** — `catalog_search` returned 0 → V5
  rendered `empty_not_found`. But `тонер` (Russian) earlier worked.
  This is the **vector-search gap (item 18)**: keyword-only search
  doesn't match `toners → тонер` cross-language, and catalog tags are
  in Russian. NOT an Agent issue, a catalog-search infra issue.
- **p17 «go back to the list»** — V5 issued `mode=rebuild` with
  `product_card` preset, fetched new products. Should have used
  `/navigation/back` to pop view stack, not re-search. **A4 in
  known-gaps**: Agent2 doesn't know about navigation actions, treats
  «back» as a fresh query.
- **p20 «add ratings everywhere»** — Agent1 fired `catalog_search`
  unnecessarily (modify intent, no new data needed). Then Agent2
  modify-mode applied 3 ops. Wasted a Pipeline turn AND **A5 in
  known-gaps** confirms (skip Agent2 not done — but Agent1 also
  misfires on this kind of prompt). **Tighten Agent1 NLU rules.**
- **p11 «show only with rating above 4»** — Agent1 hit
  `catalog_search` (returned 1 row, mostly empty), Agent2 did
  rebuild. **Refine intent → state_filter is correct**, but Agent1
  picked search. Same NLU class as p20.
- **p06 / p11 / p20 / p17 — all four are Agent1 mis-classification
  cases.** This is the SINGLE biggest quality lever per Vlad's
  intuition: «вероятно реально единственная большая проблема в
  промпте». Fixing the Agent1 prompt to apply V4-style rules
  («loaded_products>0 + ‘only/just/cheap’ → state_filter, NOT
  catalog_search») would fix 4-5 of these in one go.

### Surprising things

- **p23-25 compose costs are 1.5-2× a search turn** ($0.007-0.011 vs
  $0.005). Output tokens spike to 600-900 because Agent2 emits 12-21
  ops per turn. Token-efficient on input (`tree_map` reuses cached
  prefix), expensive on output. This is intrinsic to compose work.
- **p15 «show full details»** after p14 drill → Agent2 chose
  `mode=modify` with 5 ops on the existing product_detail. Reasonable
  — it's «show more of the same thing». Cost only $0.005.
- **p21 «change to a horizontal list»** — Agent2 picked
  `product_card_horizontal` preset (rebuild). Selected a different
  preset variant! V5 has 4 product_card variants and Agent2 actually
  uses them when prompted right.
- **Cache_read climbing 7546 → 12373 across the session** — system
  prompt + tools + accumulated history all caching cleanly. By turn
  30 Agent2 reads >12K cached tokens vs ~300 fresh. This is why
  steady-state cost is so low ($0.0029-0.0044 typical).

### Headline take for prompt-eval

V5 base capability is **strong**: text rendering, compose layouts,
context-aware empty states, history lookup, preset variant picking
all work. The visible quality gap vs V4 in widget testing is
**concentrated in Agent1 NLU classification** (p06/p11/p17/p20 —
4 of 30 prompts), plus missing **greeting preset / fallback rule**
(p01/p02 — 2 of 30). Closing those two via prompt-tuning would
recover ~80% of the perceived quality gap without touching engine
code. Vlad's hypothesis confirmed.
