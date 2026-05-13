# Keepstar One Ultra

> Project context kept thin. Anything domain-specific (catalog, engine,
> pipeline, widget, admin, curator) lives in the experts — they
> self-update from code (see `.claude/commands/experts/README.md`).
> This file holds: what the project is, how to navigate, project-specific
> working rules.

## What this is

AI-powered SaaS B2B2C chat widget for e-commerce. The user types in
chat — the bot answers with interactive widgets (product cards,
galleries, comparisons, detail views) composed dynamically by a
two-agent LLM pipeline. The merchant embeds a single `<script>` tag
and gets an AI assistant with visual answers, no in-house dev work.

V5 is the production chat engine (`project_v5/`, live at
`v5-engine-production.up.railway.app`). V4 (`project_v4/`) is being
phased out — touch only when the question is V4-specific.

## Routing — ask the expert first

| Question is about… | Ask |
|---|---|
| Catalog ingest, harvester, discovery agent, mapping artifact, merge_apply, Shopify integration, master_products / master_variants / master_cosmetics, V4 chat catalog read | `/experts:catalog:question` |
| V5 scene-graph engine: 14 node types, 5 ops, presets, binding, TreeMap, `$ref` resolution | `/experts:engine-v5:question` |
| V5 pipeline: Agent1/Agent2, prompts, tools, prompt caching, span tracing, anthropic adapter | `/experts:pipeline-agents:question` |
| V5 widget frontend: Shadow DOM, SceneGraphRenderer, NodeRenderer, action dispatch, fillTemplate | `/experts:widget:question` |
| Auth, billing, KeepstarCanvas, admin SPA, Resend/Google/Telegram/TOTP adapters | `/experts:admin:question` |
| Curator service: candidate queues, junk, audit, tenants dashboard, master catalog browse, MergeProxy | `/experts:curator:question` |

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
| V5 chat backend (production) | `project_v5/backend/` | 8084 | `v5-engine-production.up.railway.app` |
| V5 widget | `project_v5/frontend/` | 5173 | served by V5 backend |
| Admin backend | `project_admin/backend/` | 8081 | `admin-production-4ae4.up.railway.app` |
| Admin frontend | `project_admin/frontend/` | 5174 | (served by admin) |
| Curator | `curator/` | 8082 (separate) | `curator-production-46d7.up.railway.app` |

- Run everything: `scripts/start_all.sh` (stop: `stop_all.sh`).
- `psql`: `/opt/homebrew/Cellar/libpq/18.1_1/bin/psql`.
- DB: shared Neon Postgres. `DATABASE_URL` in `project_v4/.env` works for all services.
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
- `docs/Updates/` — session logs (chronological)
- `docs/catalog-audit-2026-05-07.md` — current catalog state snapshot
- `docs/v5-engine-plan.md` — V5 strategic plan + remaining work
- `docs/v5-known-gaps.md` — V5 A-series gap registry
- `AI_docs/Manifesto.md` — product vision
- `AI_docs/ARCHITECTURE_RULES.md` — architectural principles
