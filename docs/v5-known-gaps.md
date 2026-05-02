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

## Deferred V5 work (not bugs — scope decisions)

| Area | What's deferred | Why | Closes in chunk |
|---|---|---|---|
| Span tracing | `domain.SpanFromContext` blocks dropped from PG adapter (V4 had them on every method) | Tracer port not yet ported in chunk 1 | Chunk 6 (wire tracer port + re-add spans) |
| Migration runner DI | `RunStateMigrations` exists on `Client` but is not invoked from `cmd/server/main.go` (still a stub) | No HTTP server / handlers yet | Chunk 6 |
| Typed wrapper for `Current.Template` | Stored as `map[string]interface{}` (transport shape). Wanted: a typed `*engine.Document` accessor. | Binding layer (chunk 3) is the natural consumer | Chunk 3 |
| Mock-style unit tests for PG SQL builders | Only integration test (behind `//go:build integration`) and `scanDeltas` with fake `pgx.Rows`; no test that introspects raw SQL strings | Mock-testing SQL strings is busywork; integration test catches real regressions | Chunk 6 (revisit if we ever change SQL shape mid-flight) |
| LLMMessage cache-control hints | V4's adapter sets `cache_control` ephemeral hints on prompt blocks; we ported the struct verbatim but no hint plumbing | Anthropic adapter is chunk 6 | Chunk 6 |
| Catalog repository adapter (`CatalogPort`) | Not started — `StateData.Products` is hand-built in tests | Binding layer + first preset (chunks 3-4) drive the read shape | Chunk 5 (when we wire a real Agent1 query) |

## Risks worth re-checking

| Risk | Trigger | Mitigation today |
|---|---|---|
| V5 sessions write to Neon `v5_*` tables in the same DB as V4. If a frontend confuses session ids across the two engines and reads `current_template` expecting Formation, it will see scene-graph and break. | Cross-engine read in same env | We don't expose V5 over HTTP yet (chunk 6). When we do — separate route prefix and tenant gate. |
| `engine.Document` JSON shape might evolve mid-V5. If a Template stored at chunk 4 doesn't unmarshal at chunk 7, replays break. | Document version bumps | Document carries a `Version` field (chunk 1). Add a guard at unmarshal site once binding lands. |
