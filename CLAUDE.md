# Keepstar One Ultra

> Project context kept thin. Anything domain-specific (engine,
> pipeline, widget) lives in the experts — they
> self-update from code (see `.claude/commands/experts/README.md`).
> This file holds: what the project is, how to navigate, project-specific
> working rules.

## What this is

V5 is the generation engine of the **Keepstar interface runtime** —
data + operations + interface: anything can be visualized to resolve
a user's question the moment they ask. An LLM decides what to show
and which operation to run; a deterministic engine binds real tenant
data and executes — nothing is hallucinated. Chat (the embeddable
widget) is one door into the runtime; the headless API for AI agents
is the other. Commerce is one vertical, not the identity. The missing
piece today is the **operations layer** — a catalog of validated
operations with strict inputs/outputs — and that is the main
architecture front (canon: `../MANIFESTO.md`, course 2026-07-27).

**Current state (July 2026):** all services were taken off Railway in
July 2026 — the old prod URL `v5-engine-production.up.railway.app` is
dead. The Neon Postgres (flat-moon) is alive with data (3172 products,
35 tenants). Run locally via `scripts/start_all.sh`. **This monorepo
is the V5 engine core** (`project_v5/`) — Admin, Curator, and the
Landing were extracted to their own repos (`keepstar-admin`,
`keepstar-curator`, `keepstar-landing`); PIM / Connector / Price-Stock
are also separate repos.

## Routing — ask the expert first

| Question is about… | Ask |
|---|---|
| V5 scene-graph engine: 14 node types, 5 ops, presets, binding, TreeMap, `$ref` resolution | `/experts:engine-v5:question` |
| V5 pipeline: Agent1/Agent2, prompts, tools, prompt caching, span tracing, anthropic adapter | `/experts:pipeline-agents:question` |
| V5 widget frontend: Shadow DOM, SceneGraphRenderer, NodeRenderer, action dispatch, fillTemplate | `/experts:widget:question` |

> Admin / Curator / catalog ingest now live in their own repos
> (`keepstar-admin`, `keepstar-curator`) — not in this monorepo.

Reach for an expert BEFORE grep / Read. The expert returns `file:line`
refs and routes to a related expert when the answer crosses domains.

## Experts cycle (act → learn → reuse)

- After every session close, a `SessionEnd` hook spawns a headless
  `claude --print "/sync-experts --auto"` that diffs work since
  `origin/main`, maps changed files to experts via `_meta.yaml`, and
  refreshes only affected `expertise.yaml` files.
- Manual refresh: `/experts:<name>:self-improve true` or
  `/sync-experts --all`.
- Full system docs: `.claude/commands/experts/README.md`.

## Dev essentials

| Service | Path | Local port | Prod |
|---|---|---|---|
| V5 chat backend | `project_v5/backend/` | 8084 | — (taken off Railway July 2026) |
| V5 widget | `project_v5/frontend/` | 5173 | served by V5 backend |

- Run everything: `scripts/start_all.sh` (stop: `stop_all.sh`).
- `psql`: `/opt/homebrew/Cellar/libpq/18.1_1/bin/psql`.
- DB: shared Neon Postgres (flat-moon, owned by Admin). V5 reads `catalog.*` read-only via `DATABASE_URL` in `project_v5/.env`.
- Tests: `go test ./...` in each backend; `npm test` in each frontend.

## Project-specific rules

1. **Plan-mode → mandatory update log.** If `ExitPlanMode` was called
   and the plan approved, the final action of the session is
   `docs/Updates/<branch>_<YYYY-MM-DD>_<HH-MM>.md` with: context,
   approach, files changed, verification, known gaps. Even small
   changes — still a log.
2. **Time estimates in minutes, not sessions.** Concrete coding tasks
   are measured in minutes. «Session / hour» framing inflates ~10×.
3. **Theme — no purple.** Brand is light blue `#5BA4D9` + orange
   `#F0924A`. Strong aversion to purple anywhere.
4. **User-facing text in English only.** Every rendered string on
   landing/product/modals is English regardless of chat language.
   Dev docs (CLAUDE.md, READMEs, expert YAMLs) also English. Chat
   conversation with Vlad — Russian.
5. **Trackers:** `docs/CATALOG_GAPS.md` (catalog pre-launch gaps,
   live), `docs/v5-known-gaps.md` (V5 A-series gaps),
   `docs/PRE_LAUNCH_TASKS.md` (everything else).

## Working rules

Bias: caution over speed on non-trivial work. Use judgment on trivial tasks.

### 1. Think before coding
State assumptions explicitly. If uncertain, ask rather than guess.
Present multiple interpretations when ambiguity exists. Push back when
a simpler approach exists. Stop when confused — name what's unclear.

### 2. Simplicity first
Minimum code that solves the problem. Nothing speculative. No features
beyond what was asked. No abstractions for single-use code. Test: would
a senior engineer say this is overcomplicated? If yes, simplify.

### 3. Surgical changes
Touch only what you must. Clean up only your own mess. Don't "improve"
adjacent code, comments, or formatting. Don't refactor what isn't
broken. Match existing style.

### 4. Goal-driven execution
Define success criteria. Loop until verified. Don't blindly follow
steps — define success and iterate.

### 5. Use the model only for judgment calls
Use LLM for: classification, drafting, summarization, extraction.
NOT for: routing, retries, deterministic transforms. If code can
answer, code answers.

### 6. Surface conflicts, don't average them
If two patterns contradict, pick one (more recent / more tested).
Explain why. Flag the other for cleanup. Don't blend conflicting
patterns.

### 7. Read before you write
Before adding code, read exports, immediate callers, shared utilities.
«Looks orthogonal» is dangerous. If unsure why code is structured
a way, ask.

### 8. Tests verify intent, not just behavior
Tests must encode WHY behavior matters, not just WHAT. A test that
can't fail when business logic changes is wrong.

### 9. Checkpoint after every significant step
Summarize what was done, what's verified, what's left. Don't continue
from a state you can't describe back. If you lose track, stop and
restate.

### 10. Match the codebase's conventions, even if you disagree
Conformance > taste inside the codebase. If you genuinely think a
convention is harmful, surface it. Don't fork silently.

### 11. Fail loud
«Completed» is wrong if anything was skipped silently. «Tests pass»
is wrong if any were skipped. Default to surfacing uncertainty, not
hiding it.

### 12. Stale notes — verify, don't trust
This file, expert YAMLs, and `docs/` are point-in-time. Before acting
on a claim about code (`file.go:123 does X`), check the current state.
If a memory/expert says something specific exists, grep before
recommending it.

---

*Working rules adapted from a CLAUDE.md template by @Mnilax.*

## Pointers

- `.claude/commands/experts/README.md` — expert system architecture
- `docs/Updates/` — session logs (chronological, append-only truth log)
- **`../MANIFESTO.md`** — **CANON**: Keepstar = interface runtime (course 2026-07-27; supersedes all earlier positioning)
- `../archive/CANVAS_MASTER_PLAN.md` — ⚠️ HISTORICAL canvas/engine wave plan (2026-06-11; moved to archive)
- `../archive/V5_VS_C1_PARITY.md` — ⚠️ HISTORICAL V5 vs Thesys C1 parity track (2026-06-11; moved to archive)
- `../LLM_VS_DETERMINISM.md` — scoring model: who decides, LLM vs engine (2026-06-11)
- `../archive/FINAL_PHASE_PLAN.md` — ⚠️ HISTORICAL plan (2026-05-30; superseded)
- `../archive/SESSION_HANDOFF_2026-05-30.md` — ⚠️ HISTORICAL resume snapshot (2026-05-30; superseded)
- `docs/PRE_LAUNCH_TASKS.md` — current pre-launch tracker
- `docs/v5-known-gaps.md` — V5 A-series gap registry (live)
- `docs/CATALOG_GAPS.md` + `docs/CATALOG_GROUP_D_SPEC.md` — current catalog status + spec
- `docs/catalog-audit-2026-05-07.md` — ⚠️ HISTORICAL audit (2026-05-07; schema debt since fixed by Group D)
- `docs/v5-engine-plan.md` — ⚠️ HISTORICAL V5 delivery plan (2026-05-03; superseded by FINAL_PHASE_PLAN.md)
- `AI_docs/Manifesto.md` — ⚠️ HISTORICAL Feb-2026 vision (superseded by ../MANIFESTO.md)
- `AI_docs/ARCHITECTURE_RULES.md` — architectural PRINCIPLES still useful; paths/ports/schema are pre-decomposition
