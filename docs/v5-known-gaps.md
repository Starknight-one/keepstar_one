# V5 — Known gaps

Living registry of things V5 inherits from V4 as-is, things deferred to a
later chunk, and bug-for-bug ports we want to revisit. Each row should name
the chunk where it gets closed.

> **How to use**: every chunk should
> 1. Add new rows for anything ported as-is or deferred during the chunk.
> 2. Strike through (or remove) rows it closes, with the closing commit SHA
>    in parentheses.
> 3. Reference this file in the session log under "Known gaps / caveats".

## V4 bugs / limitations ported as-is

| Symptom | Source (V4) | Where it lives now (V5) | Closes in chunk |
|---|---|---|---|
| ~~`zoneWriteWithDelta` runs zone UPDATE and `AddDelta` as two separate `pool.Exec` calls (no `BEGIN`/`COMMIT`). If `AddDelta` fails, zone is updated but no delta written → state and audit log diverge.~~ | ~~`project_v4/.../postgres_state.go:418-430`~~ | ~~`project_v5/.../postgres_state.go` `zoneWriteWithDelta` now wraps zone UPDATE + addDeltaWithRetry + state.step sync in a single `pgx.Tx` (READ COMMITTED), rolls back on any error.~~ | **Closed in chunk 6d** |
| ~~`AddDelta` uses CTE `WITH next_step AS (SELECT MAX(step)+1 FROM ...)` then INSERT in the same statement. Under READ COMMITTED two concurrent writers per session can read the same MAX(step). `UNIQUE(session_id, step)` prevents corruption — one INSERT errors out, but the caller does not retry.~~ | ~~`project_v4/.../postgres_state.go:267-283`~~ | ~~`project_v5/.../postgres_state.go` `AddDelta` factored into `addDeltaOnce` + `addDeltaWithRetry`. Retry kicks in on SQLSTATE 23505 (`isUniqueViolation`); up to 3 attempts with randomised 10–50ms backoff.~~ | **Closed in chunk 6d** |
| `applyDelta` switch has stub branches for `DeltaTypePush`, `DeltaTypePop`, `DeltaTypeRollback`, `DeltaTypeRemove` — they only update `state.Step`. Reconstruction is correct only for add/update + template replay. | `project_v4/.../state_reconstruct.go:99-115` | `project_v5/.../state_reconstruct.go` `applyDelta` (comments mark each stub) | Chunk 7 (proper replay of viewstack + remove + nested rollbacks) |
| ~~Грабля #1: `__bound` flag set to `true` blindly after replicate, even when atom value didn't resolve. Result: empty cards considered "done", LLM never fills them.~~ | ~~`project_v4/.../engine_v4/expand.go` (V4 expand sets `__bound`)~~ | ~~`project_v5/.../engine/binding.go` `bindOneNode` — sets `__bound` only when value actually landed; tested in `binding_test.go` "Грабля #1" assertion~~ | **Closed in chunk 3 (`524d6c3`)** |
| ~~Грабля #2: `ComponentResolver.expandRef` recursive expansion of nested refs left `reusable:true` on inner inlined subtrees. Only top-level `ResolveAndInline` stripped the marker, so a component referencing another component would silently get skipped by `BindData` and all its leaves would go un-bound. Latent (no nested-ref seed exercised it).~~ | ~~`project_v5/backend/internal/engine/component_resolver.go` `expandRef` lines 86 + 113-117~~ | ~~`project_v5/.../component_resolver.go` `expandRef` strips `reusable` immediately after `cloneNode(source)`; `resolve_inline.go` strip is gone; covered by `TestResolveAndInlineStripsReusableFromNestedRefs` in `resolve_inline_test.go`~~ | **Closed in chunk 5.5** |

## Deferred V5 work (not bugs — scope decisions)

| Area | What's deferred | Why | Closes in chunk |
|---|---|---|---|
| ~~Span tracing~~ | ~~`domain.SpanFromContext` blocks dropped from PG adapter (V4 had them on every method)~~ | ~~Tracer port not yet ported in chunk 1~~ | **Closed in chunk 6d** — `domain/span.go` ships `Span` + `SpanCollector` + `WithSpanCollector`/`SpanFromContext` helpers (V4-aligned). Logging middleware attaches a fresh collector per request. Spans on `postgres.GetState` / `UpdateData` / `UpdateTemplate` / `GetTenantBySlug` / `ListProducts` / `GetProduct` / `GetPublishedPreset` / `ListPublishedPresets` / `GetPublishedComponent` / `ListPublishedComponents` and on `agent2.execute` / `agent2.prompt` / `agent2.llm` / `agent2.tool.<name>`. |
| ~~Migration runner DI~~ | ~~`RunStateMigrations` exists on `Client` but is not invoked from `cmd/server/main.go` (still a stub)~~ | ~~No HTTP server / handlers yet~~ | **Closed in chunk 6c** — `cmd/server/main.go` now runs all three migrations (state / preset / component) on boot before adapter setup. |
| Typed wrapper for `Current.Template` | Stored as `map[string]interface{}` (transport shape). Wanted: a typed `*engine.Document` accessor. | Binding layer (chunk 3) is the natural consumer | Chunk 3 |
| Mock-style unit tests for PG SQL builders | Only integration test (behind `//go:build integration`) and `scanDeltas` with fake `pgx.Rows`; no test that introspects raw SQL strings | Mock-testing SQL strings is busywork; integration test catches real regressions | Chunk 6 (revisit if we ever change SQL shape mid-flight) |
| LLMMessage cache-control hints | V4's adapter sets `cache_control` ephemeral hints on prompt blocks; we ported the struct verbatim but no hint plumbing | Anthropic adapter is chunk 6 | Chunk 6 |
| ~~Catalog repository adapter (`CatalogPort`)~~ | ~~Not started — `StateData.Products` is hand-built in tests~~ | ~~Binding layer + first preset (chunks 3-4) drive the read shape~~ | **Closed in chunk 3 (`524d6c3`)** — `ports.CatalogPort` + `PostgresCatalogAdapter` shipped with `GetTenantBySlug`, `ListProducts`, `GetProduct`; live-Neon e2e test in chunk 4 already exercises it |
| Preset storage location | `v5_presets` / `v5_preset_versions` **and** `v5_components` / `v5_component_versions` are the **final shape**, not a stopgap. The current `admin.tenant_presets` / `_versions` / `_components` / `_design_tokens` schemas in `project_admin/` are flagged for deletion (not migration) — user confirmed the existing canvas-in-admin is being torn out and replaced by a separate v9 microservice that will read/write V5's tables as a client. | Stream A defines the schema; the future canvas-microservice consumes it. | Canvas-microservice chunk (Stream B, post chunk 5) — wire the v9 microservice as the write-side client of `PresetPort` + `ComponentPort`; tear down the legacy admin canvas tables |
| Agent2 prompt-builder with `<fields>` block | Originally scoped for chunk 4 alongside the first preset; bumped twice (4 → 5 → 6). | Chunk 5 became "micropresets via v9 RefNode components" — architectural validation of v9 reusables before LLM enters the picture. The prompt-builder's natural pair is the first real Agent2 turn (chunk 6) once Anthropic adapter, HTTP server, and tracer ports land. | Chunk 6 (first end-to-end Agent2 call against V5) |
| Multi-root v5 components | `Materialise` only appends `Document.Children[0]` from each component (single-root). | One-root is the natural shape for canvas-authored components; multi-root would need a separate "component pack" mode. | Re-evaluate when canvas microservice ships its first multi-root component |
| ID collisions across components | `Materialise` has no dedup; if two components define a node with the same id, `FindNodeByID` returns whichever is appended first. | Chunk-5 seeds use deterministically distinct IDs (`price-rating-root`, `brand-badge-root`); the failure mode is real but not exercised yet. | Canvas-microservice chunk — enforce uniqueness at write time, or namespace component IDs at read time |
| `ResolveAndInline` mints no fresh IDs on resolved subtrees | Multiple refs to the same component yield trees that share descendant IDs (only the resolved root is unique because `expandRef` line ~98 sets `clone.id = refNode.id`). | Each resolved tree lives under a distinct ref's id; inner-id collisions only matter for path-deep ops (which V5 does not support yet). | Constraints / path-deep ops chunk (TBD) |
| `BindData` skip semantics: subtree under `reusable:true` is fully skipped | Even legitimate inner refs inside a reusable wouldn't bind | Reusables are templates, not instances; binding inside them is meaningless until inlined via `ResolveAndInline`. The resolver strips the `reusable` marker on resolved instance roots so they DO bind — only top-level component definitions retain it. | No re-evaluation needed unless we change the materialisation model |
| ~~Image-fill body shape~~ | ~~V5 writes `{type:"image", image:"<url>", mode:"fill"}` on image nodes; key `"image"` was a reasoned guess against v9.~~ | ~~Confirmed in v9 source (`packages/domain/src/value-objects/fill.ts:35-42`): the canonical key is `url`, not `image`. V5 now writes `{type:"image", url:"<url>", mode:"fill"}`. Tests + integration assertions updated.~~ | **Closed in chunk 5.5** |
| Replicate `GroupID` not ported | V4 expand assigns `rg-{counter}` shared by all clones from the same template — used to scope cross-widget constraints (C1/C3). V5 does not stamp `GroupID` on replicate clones because constraints are deferred entirely. | Chunk 4 defers constraints; no consumer for `GroupID` yet. | Constraints chunk (TBD, post chunk 5) — re-add `GroupID` stamping in `ExpandReplicates` together with the constraint engine |
| `replicate:true` one-level deep only | `ExpandReplicates` honours the outer marker and skips any nested `replicate:true` inside an already-replicated subtree. Avoids combinatorial blow-up; not a real preset shape today. | No real-world preset needs nested replication right now. | Re-evaluate if/when a preset needs nested replication |
| Format / wrapper rendering | Leaf nodes carry `format` (currency/stars/percent/...) and `wrapper` (badge/tag/...) as pass-through string properties. Backend stores them; LLM ops change them; **`BindData` does NOT format** — `content` after binding holds the raw value (string, float, array). The actual `4.5 → "★ 4.5"` step is the frontend renderer's job. Vocabulary lives in `domain.AtomFormat` / `domain.AtomWrapper`. Identical model to V4 atoms. | Token efficiency + ops control — keeping format declarative on the node lets LLM toggle representation with a single `update("price", {format:"percent"})` op without re-binding data. | Frontend renderer port (later chunk) — that's where the rendering pipeline reads `content` + `format` + `wrapper` and produces the visible string + stylistic envelope |
| Vector search / EmbeddingPort | V5 `catalog_search` is keyword-only (V5 ListProducts multi-word AND-logic ILIKE). V4's hybrid pgvector + RRF path is not ported. Tool schema keeps `vector_query` so V4 prompt copies byte-identically; executor falls back to keyword. | Chunk 7 stayed lean — embeddings + vector index + RRF would have doubled the chunk size. Quality impact: V5 keyword search loses on semantic-only queries ("для сухой кожи" without category match) where V4 vector helped. | Future chunk — wire OpenAI EmbeddingPort + pgvector + RRF merge once we have real cost data showing keyword-only quality is insufficient |
| Services entity not in Agent1 | `catalog_search` schema dropped `entity_type=service` (V5 catalog has products only). V4 supports both. | V5's StateData schema does carry `Services []Service` for forward-compat; the executor branch is just absent. | When V5 grows services in the catalog — re-add the `entity_type` enum + service-side ListServices/VectorSearchServices and merge logic (~50 lines) |
| Catalog digest persistence | V4 stores digest in `tenants.catalog_digest` and refreshes via a background job. V5 builds on demand and caches in-process forever (no DB column, no background refresh). | First-call cost = the four aggregate SQL queries; subsequent calls hit `Agent1PromptCache.store`. | Add `/api/v1/admin/invalidate-cache?tenant=X` (or hook into tenant-version bumps) when digest staleness becomes a real product issue |
| Agent1 prefix below 4500-token stable-cache bar | Measured 1819 tokens (system + 3 tool defs, no digest); with digest ~2200. Below Vlad's 4500-token V4-prod stability bar. Live test: turn 1 misses cache, turn 2 reads 6244 cache tokens (combined Agent1+Agent2 prefix is being cached as one block once history grows). | V4 has the same gap — Agent1's prompt is short on purpose; growing it would invent rules that don't exist in V4. Conversation-history caching kicks in from turn 2 onward. | Future chunk — only act if real production cost data shows turn-1 cost is unacceptable. Otherwise leave as-is |
| `/debug/traces` waterfall UI | Spans now carry id / parent_id / status / structured attrs (tokens, cost, rows, tenant_id) + per-LLM-call breakdown — chunk 8 enriched the model. But the rendering side (a `/debug/traces` HTML page that draws a waterfall + lets you click into a span's attrs) is not built. Spans are surfaced via `pipelineResponse.spans` for clients to render. | Chunk 12 (or whenever traffic warrants a real debug surface). | Chunk 12 |
| Latency baseline / Railway deploy | All chunk-7 / chunk-8 latency numbers were measured locally (`httptest.NewServer` from macOS hitting US-east Neon + Anthropic). Local roundtrips ≈ 100-300ms × many = ~1.5-2s overhead per turn that disappears on Railway. We don't have actual production-region numbers. | Chunk 8 was about getting trace-level visibility right; deploy comes next. | Next chunk — deploy V5 to Railway + record baseline latency (turn 1 cold cache vs turn 2 warm cache). |
| `tree_map` injected on every turn that has a current Document | Chunk 9 builds the tree_map and prepends it as `<formation_tree>` to every Agent2 user message when state.Current.Template is non-empty. When the LLM is about to call a known preset, the tree_map is duplicative — the preset's shape is already implicit in the system prompt — so the per-turn input-token cost (~150-400 tokens for a typical view) is paid for nothing. Vlad flagged this; first measurement says it's well under «20k token» fear levels but still wasted on preset-only turns. | Chunk 9 prioritised correctness; the optimisation requires either tool-input prediction (skip tree_map when Agent2 is likely to use a preset) or a follow-up minimal-tree_map mode. | Future chunk — measure cost impact across realistic conversations; if material, add a "skip tree_map when preset_in_use is system-default" heuristic or split modify-mode into a separate tool. |

## Render-quality gaps surfaced by first manual test (2026-05-03)

Vlad opened the V5 widget locally for the first time after chunks 9 + 10
shipped and reported «тот ещё пиздец, скорее всего пресеты херовые».
Backend log (`/tmp/v5-logs/backend.log`, session
`083f7bd3-5d25-4cda-bcba-15b8b3f86985`) captured three turns:

| Turn | Mode | What Agent2 emitted | Vlad's complaint |
|---|---|---|---|
| 1 | preset=product_card replicate=3 (system fallback) | 3 product_card clones | «фотки во весь экран и название» |
| 2 | mode=modify ops=3 | three ops on top of turn-1 tree | «попросил сделать грид, получил кривой грид только с названиями» |
| 3 | mode=modify ops=14 | fourteen ops on top of turn-2 tree | «попросил лендинг, получил доп описание под ними как кусок лендинга» |

Diagnosis: not «one bad preset», but four overlapping architectural
gaps. None of them were caught in chunk-9 live tests because the live
test only asserted spans / tool calls, not visual quality.

| Gap | Symptom | Root cause | Closes in chunk |
|---|---|---|---|
| **No grid-layout mechanism on the formation root** | Top-level frames stack vertically, each occupying the full `kw-display-inner` width (max 1200 px). Replicate=3 → three full-width blocks one above the other instead of a 3-column grid. This is the «фотки во весь экран» complaint. | V4 carries `formation.layout = "grid"` + `formation.columns = N` and the V4 `FormationRenderer` honours it. V5 has no such concept — the engine `Document.Children` is just a list, the `SceneGraphRenderer` walks it as a flex column. The `visual_assembly` tool exposes no `layout` / `columns` parameter (V4 had both). | Chunk 11 (P0-B render polish) — pick one of: (a) add `layout` + `columns` params to `visual_assembly` and have the tool wrap replicate output in a grid frame; (b) add a `kw-grid` class on the renderer side and emit it when `Document` carries a top-level `layout: "grid"` hint; (c) bake a wrapper frame into every system card preset (`{type:"frame", layout:{direction:"row", wrap:true, gap:"md"}}` with cards as children — but that doesn't survive replicate fan-out cleanly). |
| **Tenant field mapping mismatch — system presets reference v9-style names, real catalog uses v4-style names** | «Кривой грид только с названиями» — images and prices don't render because their `fieldBinding` doesn't resolve. Only `name` and `brand` happen to match the v4 catalog field names so they survive. | V5 system presets in `internal/engine/presets/seed/*.json` bind to `heroImage` / `priceFormatted` (v9-canvas naming convention). The actual `domain.Product` exposed by `engine.ProductToMap` (and the underlying `catalog.products` table from V4) uses `images` (array) and `price` (number). No translation layer. V4 had the same hardcoded coupling but the names matched the schema; V5's pretty-renamed seeds don't. | Chunk 11 — two paths: (a) rename the system seeds to v4 field names (`images` / `price` etc.) — fast, hides the architectural gap; (b) introduce a `FieldAlias` map per tenant (or a fallback chain in `BindData`: try `priceFormatted`, then `price`) — proper but bigger. Recommendation: (a) for today, (b) when canvas microservice ships and tenants edit field names on their own. |
| **No `size` (small/medium/large) on cards** | Even if grid-layout lands, cards have no width hint — they fill whatever the grid cell gives them, with no aspect-ratio sanity. | V4 `widget.size` was both a layout hint (medium card → ~300 px wide) and a constraint trigger (`tiny` → strip images). V5 dropped the concept; the engine has no `size` attribute, the renderer has no width clamps. | Chunk 11 — add a `size` attribute on top-level frames; renderer translates to `min-width / max-width` constraints. Or rely entirely on grid `columns` parameter and CSS `grid-template-columns: repeat(N, 1fr)` — sizing falls out of the column count. |
| **Agent2 modify-bias once a tree_map exists** | Turns 2 + 3 went to `mode=modify` even though the user clearly asked for a different composition («сделай грид», «сделай лендинг» both imply rebuild, not edit). | V5 `visual_assembly` makes `mode` implicit: preset → fresh build, ops only → modify. When `state.Current.Template` is non-empty, the orchestrator injects a `tree_map` into the user prompt; Agent2 sees real ids, decides «I have a tree, I'll edit it» and emits ops-only — even when the prompt rules say «data_change present → fresh build with a preset». V4 had an explicit `mode: "rebuild" \| "modify"` parameter the LLM had to choose every turn, which forced the decision into the tool surface rather than leaving it to inference. | Chunk 11 — three options: (1) restore an explicit `mode` parameter in `visual_assembly`; (2) strengthen the prompt's DECISION RULES with a stronger «when in doubt, rebuild» heuristic + concrete examples for «show me a grid» / «make a landing» triggers; (3) suppress tree_map injection when the user query semantically implies new composition (would need an upstream classifier — too heavy). Recommend (1) — it worked in V4, the cost is one extra parameter. |

These four ride together: fix any one in isolation and the visual still
looks broken. Render polish ought to be one cohesive chunk that touches
the system seed JSONs, the tool schema (mode + layout + columns), the
prompt (rebuild bias + new params doc), and the renderer (kw-grid class
+ size clamps).

## Cross-tenant chat / trace inspection in Curator (no UI yet)

Today the only way to look at what a real session did is:

- tail `/tmp/v5-logs/backend.log` while the dev server runs (lost on
  restart), OR
- query Neon directly: `SELECT * FROM v5_chat_session_deltas WHERE
  session_id = '<uuid>' ORDER BY step` — gives you action.params per
  visual_assembly call but no token cost / latency / span breakdown,
  AND
- inspect `pipelineResponse.spans[]` in devtools the moment the response
  lands — gone the second the user closes the tab.

That's how the chunk-9/10 four-gap diagnosis above had to be done: by
hand, against an in-memory log file, with cross-referencing to a single
session id. Not scalable past a single dev box, impossible to do for
production traffic, no way to spot patterns across tenants.

What's needed is a **first-class «Chats» surface in the Curator app**
(`curator/`) — Curator already runs cross-tenant CRUD over the master
catalog and has the auth + tenant-list scaffolding. Adding a `/chats`
menu item there is the cheapest way to surface this without building a
fresh internal admin from scratch.

Concretely, this gap has three layers:

| Layer | What's missing | What lands | Closes in chunk |
|---|---|---|---|
| **DB persistence — trace + cost metadata** | Chunk-8 `Span` shape (id / parent_id / status / attrs with tokens, cost, rows, tenant_id) lives only in `pipelineResponse.spans[]`. Nothing writes it to Neon. Conversation + visual_assembly inputs ARE persisted via `v5_chat_session_deltas` (action.params), but spans / tokens / cost are not — so per-chat cost rollups are impossible today. | New table `v5_chat_session_traces`: one row per pipeline turn with FK to session, `request_id`, `agent1_ms` / `agent2_ms` / `total_ms`, `tokens_input` / `tokens_output` / `cache_read` / `cost_usd`, full `spans` JSONB. Written from the pipeline handler at the same point that `out.Spans` is built. | Future chunk («observability persistence») — backend-only, ~2-3 hours. |
| **Curator backend — read-side endpoints** | Curator backend (`curator/backend/`) talks to its own Postgres tables but has no read access to V5's `v5_chat_*` tables. Need a thin internal API: list sessions across tenants, list turns of a session, fetch one turn's spans. | New routes in Curator backend: `GET /api/chats?tenant=&active=&q=` (list with filters), `GET /api/chats/:sessionId` (turn-by-turn timeline), `GET /api/chats/:sessionId/turns/:turnId/spans` (waterfall data for one turn). MergeProxy → admin-internal-key style; respect Curator's existing auth. | Same chunk as the DB persistence above (writer + reader together). |
| **Curator frontend — `/chats` page + detail view** | Curator's `App.jsx` route table (login / tenants / master / candidates / junk / audit) has no chats entry. | New `pages/ChatsPage.jsx` listing rows: tenant slug, session id (short), started_at / last_activity_at, **active flag** (last activity within ~30 min ⇒ active, else closed), turn count, **total cost USD** rolled up from per-turn rows. Click row → `pages/ChatDetailPage.jsx`: full conversation timeline (user msgs + Agent2 tool calls + rendered Document preview if cheap enough), per-turn span waterfall (chunk 8 shape: id / parent_id / duration_ms / attrs), aggregate token + cost numbers. New menu item «Chats» in the sidebar between «Tenants» and «Master». | Chunk after the persistence one — frontend-only, ~3-4 hours. |

Vlad flagged this as the canonical home for the kind of detailed trace
analysis we just did manually. Without it, every iteration of the
diagnosis loop costs us a manual log scrape + DB query + memory
juggling. With it, we click into a session and see the trace + cost +
ops history in one place — and can spot cross-tenant patterns (which
prompts cause modify-bias, which tenants have field-binding mismatches,
which sessions are draining budget).

Order of attack: backend persistence first (writer + reader together),
frontend after. Persistence is also the foundation for the deferred
`/debug/traces` waterfall UI (currently scoped as chunk-12 inside V5
itself) — these two surfaces should share the same span schema.

## Risks worth re-checking

| Risk | Trigger | Mitigation today |
|---|---|---|
| V5 sessions write to Neon `v5_*` tables in the same DB as V4. If a frontend confuses session ids across the two engines and reads `current_template` expecting Formation, it will see scene-graph and break. | Cross-engine read in same env | We don't expose V5 over HTTP yet (chunk 6). When we do — separate route prefix and tenant gate. |
| `engine.Document` JSON shape might evolve mid-V5. If a Template stored at chunk 4 doesn't unmarshal at chunk 7, replays break. | Document version bumps | Document carries a `Version` field (chunk 1). Add a guard at unmarshal site once binding lands. |
| **V5 prompt+tool prefix below stable cache threshold** — chunk-5.5 measurement: V5 system+tools = 3001 tokens, V4 = 6449 tokens. Anthropic Haiku documented min is 2048, but Vlad's V4-prod experience says ≥ 4500 is needed for stable caching. At 3001 V5 may not get a cache hit reliably — and without cache, V5's −48.8% per-turn input-tokens win flips to +70.5% **higher** effective cost over a 10-turn conversation (V5 37050 vs V4 21725 USD-µ at Haiku 4.5 pricing). The measurement is in `internal/engine/tokens/measurement_test.go` (build tag `tokens`); re-run as V5 prompt evolves. | Chunk-6b prompt-builder ships an under-sized prompt | **HARD REQUIREMENT for chunk 6b**: V5 system + tools prefix must clear ≥ 4500 tokens before any LLM turn ships. Natural source: porting V4's BUILDING / COMPOSING / MODIFYING examples, FIELD BINDING rules, and decision rules into the V5 prompt — these are content we'll need anyway. The cache_control hint must be applied on the system + tools blocks (V4 pattern, see `prompt_compose_widgets.go` cache-control plumbing). |
