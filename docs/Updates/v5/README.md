# V5 Session Logs

Each implementation session on the `v5` branch leaves a log here.

## File naming

`v5_<YYYY-MM-DD>_<HH-MM>.md`

Date and time = the moment of the commit that closes the session (UTC).

## Required sections

```markdown
# <short title>

**Branch**: v5
**Date (UTC)**: 2026-XX-XX HH:MM
**Commit**: <sha>
**Parent**: <sha>

## Context
Why this session happened. What gap or task is being closed. Reference to the chunk plan
in `~/.claude/plans/` if relevant.

## Approach
What we changed and why this approach.

## Files changed
| File | Status | Notes |
|---|---|---|
| ... | added/modified/deleted | ... |

## Verification
How we verified locally (commands run, tests passing). What to watch on prod once deployed.

## Known gaps / caveats
What's NOT closed yet. Deferred details. Follow-ups.
```

## Per-chunk plans

Each implementation chunk gets a detailed plan document in `plans/`, written
in plan mode and approved before execution. Naming:
`plans/chunk-<N>-<short-name>.md` (e.g. `plans/chunk-1-engine-port.md`).

Plan + log are paired: the plan describes what we set out to do; the log
describes what actually happened (commit sha, files changed, gaps).

## See also

- High-level plan: `../../v5-engine-plan.md`
- Known-gaps registry: `../../v5-known-gaps.md`
- V4 reference logs: `../feature-engine-v4_*.md`

## Index of session logs (newest first)

| Log | Chunk | Plan | What it shipped |
|---|---|---|---|
| `v5_2026-05-03_19-59_chunk-15.5-component-registry-and-manual-gaps.md` | 15.5 | (hotfix, no frozen plan) | `SystemComponentRegistry` — closes refs-not-resolved bug from chunk-9 oversight. price-rating-root + brand-badge-root now served as in-process fallback when `v5_components` DB has no row. Refs in system presets actually inline → cards render with image + name + price + rating + brand visible. Verified live: «тонер» turn returns 3 cards with «1 890 ₽» / «Dear, Klairs» / image fills. ALSO: registers 6 manual-test quality gaps vs V4 (greeting, modify-bias, pagination, back button, skip-Agent2, layout density) into `v5-known-gaps.md` for follow-up chunks. |
| `v5_2026-05-03_18-55_chunk-15.md` | 15 | `plans/chunk-15-smoke-comparison.md` | V5 vs V4 prod smoke comparison (closes items 16 + 17). New `scripts/v5-smoke-prompts.json` (25 prompts × 5 tags) + `scripts/v5-smoke.sh` (bash + python runner). For each prompt: hits V5 + V4 `/api/v1/pipeline`, pulls V4 cost from `/debug/traces?format=json`, writes per-prompt JSON dumps + `summary.md` with aggregates. First run: 25/25 success on both backends; V5 lat p50 8166ms vs V4 2608ms (**+213%** — V5 always fires Agent1+Agent2 sequentially); V5 avg cost $0.0053 vs V4 $0.0048 (+10%); V5 avg input tokens -24% via tree_map; cache_read parity (~10K both). Per-prompt JSON dumps gitignored, summary committed. |
| `v5_2026-05-03_16-30_chunk-14.md` | 14 | `plans/chunk-14-railway-deploy.md` | V5 backend deploy on Railway as service `v5-engine` in `selfless-tranquility/production`. New `project_v5/Dockerfile` (3-stage, V4-mirrored). New `/readyz` handler with `pgxpool.Ping`. Static fileserver on `/` so `widget.js` is served same-origin (V4 pattern). Live smoke green: healthz/readyz/widget.js + 1 pipeline turn (product_card replicate=3, cache_read=7546 tokens, $0.006, 7.2s). Domain: `https://v5-engine-production.up.railway.app`. |
| `v5_2026-05-03_15-28_chunk-13.md` | 13 | `plans/chunk-13-curator-chats.md` | Cross-tenant chat / trace inspection in Curator. New `v5_chat_session_traces` table (per-turn spans + tokens + cost), TracePort + TraceAdapter, async best-effort persist hook in pipeline handler. Curator backend: 3 SELECT-only endpoints reading shared Neon. Curator frontend: ChatsPage + ChatDetailPage + sidebar «Tracing» section. Railway-ready with first commit (no new env vars; existing Dockerfiles + boot-time migrations). Live smoke green, trace persist verified. |
| `v5_2026-05-03_14-58_chunk-12.md` | 12 | `plans/chunk-12-render-polish.md` | REQUIRED `mode: rebuild|modify` enum back on visual_assembly schema (defeats modify-bias). Card seeds wrapped in grid frame; Frame.jsx reads layout.wrap + width/maxWidth/minWidth → CSS flex-wrap + inline style. Agent2 prompt MODE section + DECISION RULES updated. Live HTTP smoke 5 turns green; mode emitted correctly on every call. |
| `v5_2026-05-03_13-56_chunk-11.md` | 11 | `plans/chunk-11-actions-and-navigation.md` | Closed action vocab (9 kinds), engine InjectDefaultActions pass, POST /actions, POST /navigation/{expand,back}, hardcoded SystemAdjacency, 1-level prefetch payload, frontend dispatcher + RenderContext + clickable cards + back button. Agent2 tool-filter fix. Live HTTP smoke green (5 turns + actions + nav). |
| `v5_2026-05-03_15-08_chunk-10.md` | 10 | `plans/chunk-10-frontend-renderer.md` | V5 frontend renderer at `project_v5/frontend/` (Vite + React 19 + Shadow DOM, 13/13 vitest jsdom). |
| `v5_2026-05-03_14-59_chunk-9.md` | 9  | `plans/chunk-9-tool-surface-and-system-presets.md` | Tool surface unblock (preset optional, multi-widget compose, modify-mode), 7 system presets via in-process registry, tree_map computation + injection. Live HTTP smoke 4-turn green. |
| `v5_2026-05-02_21-31.md` | 8  | `plans/chunk-8-trace-upgrade.md` | Span model upgrade — id / parent_id / status / structured attrs (LLM tokens + cost, postgres rows + tenant). |
| `v5_2026-05-02_21-07.md` | 7  | `plans/chunk-7-agent1.md` | Agent1: catalog_search / state_filter / history_lookup; Agent1 prompt; pipeline orchestrator (Agent1 → Agent2). |
| `v5_2026-05-02_20-13.md` | 6c+6d | `plans/chunk-6cd-http-cleanup.md` | HTTP server, handlers, DI, ops applier, tx wrap on zoneWriteWithDelta, retry on 23505, tracer port. |
| `v5_2026-05-02_19-51.md` | 6b | `plans/chunk-6b-agent2.md` | Tool registry, visual_assembly tool, Agent2 prompt-builder + first end-to-end Agent2 turn. |
| `v5_2026-05-02_19-26.md` | 6a | `plans/chunk-6a-anthropic-adapter.md` | LLMPort + Anthropic adapter (`ChatWithToolsCached`) + count_tokens. |
| `v5_2026-05-02_19-02.md` | 5.5 | `plans/chunk-5_5-hygiene.md` | Nested-ref reusable strip, image-fill `url`, format/wrapper props, first cache-aware token measurement. |
| `v5_2026-05-02_17-50.md` | 5  | `plans/chunk-5-micropresets.md` | v9 RefNode components, two presets sharing two reusable subtrees. |
| `v5_2026-05-02_17-28.md` | 4  | `plans/chunk-4-first-preset.md` | First product_card preset end-to-end + replicate fan-out + image binding. |
| `v5_2026-05-02_16-53.md` | 3  | `plans/chunk-3-binding.md` | Per-instance scene-graph binding, slot ↔ field, ProductToMap. |
| `v5_2026-05-02_16-00.md` | 2  | `plans/chunk-2-state-delta.md` | Sectional state + append-only delta-stream + reconstruct + rollback. |
| `v5_2026-05-02_15-26.md` | 1  | `plans/chunk-1-engine-port.md` | v9 → Go engine port (scene-graph + ops + components + variables). |
