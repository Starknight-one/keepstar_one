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
| `zoneWriteWithDelta` runs zone UPDATE and `AddDelta` as two separate `pool.Exec` calls (no `BEGIN`/`COMMIT`). If `AddDelta` fails, zone is updated but no delta written → state and audit log diverge. | `project_v4/.../postgres_state.go:418-430` | `project_v5/.../postgres_state.go` `zoneWriteWithDelta` (NOTE comment in code) | Chunk 6 (when LLM adapter wires the pipeline and we know retry semantics) |
| `AddDelta` uses CTE `WITH next_step AS (SELECT MAX(step)+1 FROM ...)` then INSERT in the same statement. Under READ COMMITTED two concurrent writers per session can read the same MAX(step). `UNIQUE(session_id, step)` prevents corruption — one INSERT errors out, but the caller does not retry. | `project_v4/.../postgres_state.go:267-283` | `project_v5/.../postgres_state.go` `AddDelta` (NOTE comment in code) | Chunk 6 (add caller-side retry, or move to advisory lock) |
| `applyDelta` switch has stub branches for `DeltaTypePush`, `DeltaTypePop`, `DeltaTypeRollback`, `DeltaTypeRemove` — they only update `state.Step`. Reconstruction is correct only for add/update + template replay. | `project_v4/.../state_reconstruct.go:99-115` | `project_v5/.../state_reconstruct.go` `applyDelta` (comments mark each stub) | Chunk 7 (proper replay of viewstack + remove + nested rollbacks) |
| ~~Грабля #1: `__bound` flag set to `true` blindly after replicate, even when atom value didn't resolve. Result: empty cards considered "done", LLM never fills them.~~ | ~~`project_v4/.../engine_v4/expand.go` (V4 expand sets `__bound`)~~ | ~~`project_v5/.../engine/binding.go` `bindOneNode` — sets `__bound` only when value actually landed; tested in `binding_test.go` "Грабля #1" assertion~~ | **Closed in chunk 3 (`524d6c3`)** |

## Deferred V5 work (not bugs — scope decisions)

| Area | What's deferred | Why | Closes in chunk |
|---|---|---|---|
| Span tracing | `domain.SpanFromContext` blocks dropped from PG adapter (V4 had them on every method) | Tracer port not yet ported in chunk 1 | Chunk 6 (wire tracer port + re-add spans) |
| Migration runner DI | `RunStateMigrations` exists on `Client` but is not invoked from `cmd/server/main.go` (still a stub) | No HTTP server / handlers yet | Chunk 6 |
| Typed wrapper for `Current.Template` | Stored as `map[string]interface{}` (transport shape). Wanted: a typed `*engine.Document` accessor. | Binding layer (chunk 3) is the natural consumer | Chunk 3 |
| Mock-style unit tests for PG SQL builders | Only integration test (behind `//go:build integration`) and `scanDeltas` with fake `pgx.Rows`; no test that introspects raw SQL strings | Mock-testing SQL strings is busywork; integration test catches real regressions | Chunk 6 (revisit if we ever change SQL shape mid-flight) |
| LLMMessage cache-control hints | V4's adapter sets `cache_control` ephemeral hints on prompt blocks; we ported the struct verbatim but no hint plumbing | Anthropic adapter is chunk 6 | Chunk 6 |
| ~~Catalog repository adapter (`CatalogPort`)~~ | ~~Not started — `StateData.Products` is hand-built in tests~~ | ~~Binding layer + first preset (chunks 3-4) drive the read shape~~ | **Closed in chunk 3 (`524d6c3`)** — `ports.CatalogPort` + `PostgresCatalogAdapter` shipped with `GetTenantBySlug`, `ListProducts`, `GetProduct`; live-Neon e2e test in chunk 4 already exercises it |
| Preset storage location | `v5_presets` / `v5_preset_versions` are the **final shape**, not a stopgap. The current `admin.tenant_presets` / `_versions` / `_components` / `_design_tokens` schemas in `project_admin/` are flagged for deletion (not migration) — user confirmed the existing canvas-in-admin is being torn out and replaced by a separate v9 microservice that will read/write V5's tables as a client. | Stream A defines the schema; the future canvas-microservice consumes it. | Canvas-microservice chunk (Stream B, post chunk 5) — wire the v9 microservice as the write-side client of `PresetPort`; tear down the legacy admin canvas tables |
| Agent2 prompt-builder with `<fields>` block | Originally scoped for chunk 4 alongside the first preset; bumped twice (4 → 5 → 6). | Chunk 5 became "micropresets via v9 RefNode components" — architectural validation of v9 reusables before LLM enters the picture. The prompt-builder's natural pair is the first real Agent2 turn (chunk 6) once Anthropic adapter, HTTP server, and tracer ports land. | Chunk 6 (first end-to-end Agent2 call against V5) |
| Multi-root v5 components | `Materialise` only appends `Document.Children[0]` from each component (single-root). | One-root is the natural shape for canvas-authored components; multi-root would need a separate "component pack" mode. | Re-evaluate when canvas microservice ships its first multi-root component |
| ID collisions across components | `Materialise` has no dedup; if two components define a node with the same id, `FindNodeByID` returns whichever is appended first. | Chunk-5 seeds use deterministically distinct IDs (`price-rating-root`, `brand-badge-root`); the failure mode is real but not exercised yet. | Canvas-microservice chunk — enforce uniqueness at write time, or namespace component IDs at read time |
| `ResolveAndInline` mints no fresh IDs on resolved subtrees | Multiple refs to the same component yield trees that share descendant IDs (only the resolved root is unique because `expandRef` line ~98 sets `clone.id = refNode.id`). | Each resolved tree lives under a distinct ref's id; inner-id collisions only matter for path-deep ops (which V5 does not support yet). | Constraints / path-deep ops chunk (TBD) |
| `BindData` skip semantics: subtree under `reusable:true` is fully skipped | Even legitimate inner refs inside a reusable wouldn't bind | Reusables are templates, not instances; binding inside them is meaningless until inlined via `ResolveAndInline`. The resolver strips the `reusable` marker on resolved instance roots so they DO bind — only top-level component definitions retain it. | No re-evaluation needed unless we change the materialisation model |
| Image-fill body shape | V5 writes `{type:"image", image:"<url>", mode:"fill"}` on image nodes. The `image` payload-key matches v9's discriminator-named-payload convention (cf. `FillTypeColor → color:{...}`) but is **not yet confirmed** against v9 TS source. `value_fill.go:1-62` declares only discriminators, no body schema. | Plan-agent flagged the gap; close before frontend renderer port. If v9 uses `url` or `src` instead, change `attrFillImageURL` constant in `binding.go`. | First frontend renderer port (or sooner via grep on v9 source) |
| Replicate `GroupID` not ported | V4 expand assigns `rg-{counter}` shared by all clones from the same template — used to scope cross-widget constraints (C1/C3). V5 does not stamp `GroupID` on replicate clones because constraints are deferred entirely. | Chunk 4 defers constraints; no consumer for `GroupID` yet. | Constraints chunk (TBD, post chunk 5) — re-add `GroupID` stamping in `ExpandReplicates` together with the constraint engine |
| `replicate:true` one-level deep only | `ExpandReplicates` honours the outer marker and skips any nested `replicate:true` inside an already-replicated subtree. Avoids combinatorial blow-up; not a real preset shape today. | No real-world preset needs nested replication right now. | Re-evaluate if/when a preset needs nested replication |

## Risks worth re-checking

| Risk | Trigger | Mitigation today |
|---|---|---|
| V5 sessions write to Neon `v5_*` tables in the same DB as V4. If a frontend confuses session ids across the two engines and reads `current_template` expecting Formation, it will see scene-graph and break. | Cross-engine read in same env | We don't expose V5 over HTTP yet (chunk 6). When we do — separate route prefix and tenant gate. |
| `engine.Document` JSON shape might evolve mid-V5. If a Template stored at chunk 4 doesn't unmarshal at chunk 7, replays break. | Document version bumps | Document carries a `Version` field (chunk 1). Add a guard at unmarshal site once binding lands. |
