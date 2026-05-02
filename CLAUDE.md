# Keepstar One Ultra — Project Context

> **IMPORTANT**: Information in this file may be stale. Before implementing any task, ALWAYS re-read the relevant source files and check the current state of the code. Don't blindly trust what's described below — these notes give you the big picture, but the details may have moved.

## What this is

AI-powered SaaS B2B2C chat widget for e-commerce. The user types in chat — instead of text replies, the bot generates interactive widgets: product cards, galleries, comparisons, detail views. Everything is composed dynamically on the backend through a two-agent LLM pipeline.

**Key value prop**: a merchant embeds a single `<script>` tag on their site and gets an AI assistant with visual answers, with no in-house dev work.

## Architecture (high level)

```
User → Chat Widget (React, Shadow DOM)
            ↓ REST API
       V4 Chat Backend (Go, port 8082)
            ↓
   ┌────────────────────────────┐
   │  Agent 1 (NLU/Data)        │  ← catalog_search, state_filter, history_lookup
   │  Agent 2 (Render)          │  ← visual_assembly tool → engine_v4
   └────────────────────────────┘
            ↓
   Formation JSON → Frontend renders
```

**Backend-first**: the frontend is a "dumb renderer" of JSON. All logic, layout, and constraints live on the backend.

## Three-level widget hierarchy

```
Formation (mode: grid, list, single, carousel, comparison, table, composed)
  └── Widget (card / row / block of atoms)
      └── Atom (6 types: text, number, image, icon, video, audio)
           ├── subtype (currency, rating, url, date, ...)
           ├── display/wrapper (h1, badge, tag, price, button, ...)
           ├── format (currency, stars, percent, date, ...)
           └── slot (hero, title, price, primary, secondary, badge, ...)
```

## Project layout

```
project_v4/backend/         — Go 1.24, hexagonal — V4 chat engine (PRODUCTION)
  cmd/server/               — Entry point, DI, graceful shutdown
  internal/
    domain/                 — Atom, Widget, Formation, Preset, Span, Trace, Tool
    ports/                  — Interfaces (LLM, Catalog, State, Trace, Embedding)
    adapters/               — Postgres (pgx), Anthropic, OpenAI
    usecases/               — pipeline_execute, agent1_execute, agent2_execute, navigation, state
    handlers/               — HTTP routes (pipeline, chat, navigation, testbench, debug)
    tools/                  — Tool executors (catalog_search, state_filter, history_lookup, visual_assembly)
    prompts/                — Agent1/Agent2 system prompts
    engine_v4/              — Ops-driven UI assembly engine (engine.go, ops.go, presets_*.go, binding.go, ...)

project/frontend/           — React 19 + Vite 7 chat widget (Shadow DOM, IIFE bundle)
  entities/                 — atom/, widget/, formation/, message/
  features/                 — chat/, catalog/, navigation/, overlay/, actions/
  shared/                   — api/, theme/, hooks/, config/

project_admin/              — Catalog management, auth, billing, KeepstarCanvas
  backend/                  — Go, hexagonal (auth, billing, canvas, catalog write side, Shopify)
  frontend/                 — React admin SPA (catalog UI, canvas editor, settings, traces)

curator/                    — Standalone Go service for catalog curation
  backend/                  — single binary; auth, candidates queue, junk, audit, tenants, master catalog
  frontend/                 — React + React Router 7 operator dashboard

Keepstar_one_landing/       — Public landing site (keepstar.one) + blog admin
docs/                       — Specs, dev logs, audits
ADW/                        — SDLC orchestrator + dev-inspector
AI_docs/                    — Manifesto, architecture rules, agent principles
scripts/                    — start.sh, stop.sh, start_admin.sh, stop_admin.sh, start_all.sh, stop_all.sh
```

> **Legacy `project/backend/` was deleted on 2026-04-29.** V4 is the only chat backend. Anything referencing the old V1/V2 engine is stale.

## Dev servers & tools

| Service | Path | Port |
|--------|------|------|
| V4 Chat backend | `project_v4/backend/` | 8082 |
| Chat widget (dev) | `project/frontend/` | 5173 |
| Admin backend | `project_admin/backend/` | 8081 |
| Admin frontend | `project_admin/frontend/` | 5174 |
| Curator | `curator/` | 8082 (separate Railway service) |
| Dev Inspector | `ADW/dev-inspector/` | 3457 |

- **Run everything**: `scripts/start_all.sh`
- **psql**: `/opt/homebrew/Cellar/libpq/18.1_1/bin/psql` or `/opt/homebrew/Cellar/postgresql@15/15.15_1/bin/psql`
- **Config**: `project_v4/.env` (DATABASE_URL, ANTHROPIC_API_KEY, OPENAI_API_KEY, TENANT_SLUG, LLM_MODEL)
- **DB**: Neon PostgreSQL (serverless, AWS). Schemas: `catalog`, `admin`, `logs` + `chat_*` tables in public.
- **Tests**: `cd project_v4/backend && go test ./...`

## Two-agent pipeline

1. **Agent 1** (NLU + data) — analyses the query, calls one of:
   - `catalog_search` — hybrid search (SQL keyword + pgvector + RRF merge)
   - `state_filter` — filter already-loaded data without a DB hit
   - `history_lookup` — search the conversation history
   - Writes results to `state.data` + `state.meta`.

2. **Agent 2** (render) — calls the `visual_assembly` tool:
   - Reads `state.meta` (count, fields, entity type)
   - Picks a preset, layout, columns, size
   - Emits `ops` (insert/update/delete/move/replace) on the formation tree
   - Constraints normalise (badge length, tag length, tiny-size strips images, group-scoped C1/C3)
   - Result lands in `state.template["formation"]`

3. **Frontend** renders the Formation JSON via `FormationRenderer` → `WidgetRenderer` → `AtomV2Renderer`.

## V4 Engine (current — `feature/engine-v4` branch deploys to v4-engine-production.up.railway.app)

Ops-driven engine — Agent2 builds and modifies the UI by emitting ops (insert/update/delete/move/replace) on a widget tree. Lives in `project_v4/backend/internal/engine_v4/`.

**Main tool — `visual_assembly`**, parameters:
- `ops` — array of ops on the formation tree
- `preset` — named preset (12 in the global registry). Concatenated with `ops` in one batch so user override-ops can reference `$ref` slots exposed by the preset (`$w` / `$root` / `$info` / `$meta`).
- `replicate` — explicit replicate flag (B3). Inherited from `preset.DefaultReplicate` if omitted.
- `limit` — cap on data items used for replication
- `layout` — grid / list / single / carousel, plus `columns`, `size`

**Execute pipeline** (`engine.go`):
1. Init formation (or load `Input.Formation`)
2. Apply formation-level settings (Layout → Mode, Columns → Grid.Cols, Size → all top-level widgets)
3. `ApplyOps` — preset + user ops in one batch
4. Limit data → handle Replicate flag (legacy bridge for single-widget; per-widget `ReplicateConfig` for multi-widget compositions)
4a. `expandReplicatedWidgets` — clone templates per data item with shared GroupID
4b. `autoDetectEntityRefs` — for single non-replicated entity widgets
4.5. Inject `DefaultWidgetActions` (like, add_to_cart) for entity-bound widgets
5. `BindData` — atoms with `FieldName` get values from `data[i]`
6. `ApplyConstraints` — per-atom (A1/A2) → per-widget (W8) → group cross-widget (C1/C3)
6.5. `groupIntoSections` — collapse consecutive same-GroupID widgets into sections; single-section rollback for legacy flat flows
7. `StampTreeIDs` — deterministic IDs (`w-s0-w0`, `a-s0-w0-name`, ...)
8. `BuildTreeMap` — compact context for Agent2's next turn

**12 presets** (`engine_v4/presets_*.go`):
- product (6): `product_card`, `product_card_compact`, `product_card_horizontal`, `product_card_list_row`, `product_detail`, `product_detail_horizontal`
- system (3): `text_explainer`, `empty_not_found`, `error_generic`
- nav (3): `catalog_category_card`, `liked_grid`, `cart_grid`

Presets currently hardcode cosmetics fieldNames (images/name/price/rating/brand/...). Wave B7 will switch to role-based slot resolution via `catalog.field_definitions`.

**Tracker**: `docs/PRE_LAUNCH_TASKS.md` (waves B2/B3/B4/B7/E1/E2/A2/AD1/UX1/...).
**Session dev logs**: `docs/Updates/<branch>_<YYYY-MM-DD>_<HH-MM>.md` — every session leaves a log with context, changes, tests, commit hash, known gaps.

## Data model (key entities)

- **SessionState**: `current` (data + meta + template), `view`, `viewStack`, `conversationHistory`, `step`
- **Delta**: source / actor / trigger / type + action / result (append-only history)
- **Atom**: type + subtype + display + format + value + slot + meta + fieldName
- **Preset**: Name, Description, Category, Refs[], DefaultLayout, DefaultColumns, DefaultSize, DefaultReplicate, Build()
- **Formation**: mode + grid + widgets[] + sections[] + pagination
- **Widget**: id + size + atoms[] + entityRef + ReplicateConfig + Actions

## API endpoints (V4 backend, port 8082)

- `POST /api/v1/pipeline` — main entry: query → Agent1 → Agent2 → Formation
- `POST /api/v1/chat` — alias / legacy entry
- `POST /api/v1/navigation/expand` — drill-down to detail
- `POST /api/v1/navigation/back` — navigate back
- `POST /api/v1/session/init` — create session
- `GET /api/v1/session/{id}` — restore session
- `POST /api/v1/actions` — like / cart sync
- `POST /api/v1/testbench` — visual_assembly without LLM
- `GET /debug/traces/` — pipeline waterfall traces
- `GET /debug/session/` — session inspector

## LLM & cost

- **Model**: Claude Haiku 4.5 (default; configurable via `LLM_MODEL`)
- **Embeddings**: OpenAI text-embedding-3-small (384 dim)
- **Prompt caching**: system + tools + conversation cached (5-min ephemeral TTL)
- **Haiku price**: $1 input / $5 output per 1M tokens; cache write 1.25×, cache read 0.1×
- **Cache invariants**: tool definitions sorted by name (byte-stable order); per-tenant catalog digest memoised in sync.Map; conversation trim at 20 messages

## Frontend (chat widget)

- **Deploy**: single IIFE bundle `widget.js`, embedded via `<script data-tenant="slug" data-api="url" src="https://v4-engine-production.up.railway.app/widget.js">`
- **Shadow DOM**: full style isolation from the host page (mode: open). All CSS imported as `?inline` and injected into shadow root.
- **Instant expand**: `adjacentTemplates` + `fillFormation()` — drill-down to detail with no round-trip; mirrors backend `productFieldGetter`/`serviceFieldGetter`.
- **Session cache**: localStorage, 30-min TTL, restored on F5
- **Theme**: CSS Variables. Brand palette: light blue `#5BA4D9` + orange `#F0924A` (gradient for action circles), white bg, text `#1a1a1a`. **No purple anywhere.**
- **Layout**: full-screen overlay when chat opens. `chat-area` — narrow 360px column on the right; `widget-display-area` — flex:1 on the left (max-width 1200, centers formation). Widgets render **beside** chat, not inside it.

## Admin panel

- **Catalog**: browse, search, edit products, master/listings split, master_variants
- **Import**: JSON / CSV / Shopify OAuth → harvester → discovery (LLM ~$0.40, 8-min budget) → mapping artifact → merge_apply → curator approval
- **Enrichment**: Claude Haiku extracts PIM attrs (skin_type, concern, ingredients, ...)
- **Widget**: embed-code generator (uses `WIDGET_BASE_URL` env)
- **KeepstarCanvas**: per-tenant preset / component / design-token overrides; published presets read by V4 via `tenant_preset_loader`
- **Crawler**: `cmd/crawler/` — scrapes JSON-LD from e-commerce sites
- **Sample data**: 967 heybabescosmetics products in `project_admin/Crawler_results/`

## Plan mode → mandatory update log

If a session used plan mode (ExitPlanMode was called and the plan approved), the **final action of the session** must be an update log in `docs/Updates/`.

Filename format: `<branch-name>_<YYYY-MM-DD>_<HH-MM>.md` (e.g. `feature-engine-v4_2026-04-07_14-30.md`). Date and time = moment of commit. The file must contain:
- Header: branch, date (UTC), commit sha, parent commit
- Context: why the change was made, what gap/task it closes
- Approach: what changed and why this way
- Files changed: a table
- Verification: how it was checked locally + what to watch in prod
- Known gaps / caveats: what is NOT closed, deferred nuances

This rule applies whenever plan mode was used, regardless of size. Even a small change → still a log. Format is established; see recent files in `docs/Updates/` for the template.

## Experts — when to use them

The codebase is covered by **6 vertical experts**. Each owns one business domain across all hexagonal layers. They give grounded answers with `file:line` refs faster than ad-hoc grep, plus they auto-route to other experts when an answer crosses domains.

**Routing — pick an expert FIRST, then read code only if the expert points you there:**

| User question is about… | Use this expert |
|---|---|
| Catalog ingest, harvester, discovery agent, mapping artifact, merge_apply, match cascade, tier1/tier2 attributes, catalog migrations, Shopify integration, V4 chat read of catalog, master_products / master_variants / listings | `/experts:catalog:question` |
| Engine ops (insert/update/delete/move/replace), presets, formation tree, atom binding, constraints, TreeMap, $ref bindings, replicate / GroupID, sections, tree IDs | `/experts:engine-v4:question` |
| Pipeline orchestration, Agent1 NLU, Agent2 rendering, system prompts, tool registry, prompt caching, span tracing, anthropic client, OpenAI embeddings, conversation history, retention | `/experts:pipeline-agents:question` |
| Chat widget, Shadow DOM, FormationRenderer modes, AtomV2Renderer, fillFormation (instant expand), sessionCache, ChatPanel, navigation history, cart/liked views, embed `<script>`, IIFE bundle | `/experts:widget:question` |
| Auth (email/password, magic link, 2FA, Google OAuth, telegram, invitations, sessions), billing, KeepstarCanvas (DRAFT/PUBLISHED), middleware (auth/api-key/internal-key), admin SPA, Resend mailer | `/experts:admin:question` |
| Curator standalone service, MergeProxy → admin internal endpoints, candidates queue, junk classification, audit log, tenants dashboard, master catalog browse, PromoteAttribute (ALTER TABLE) | `/experts:curator:question` |

**Default behaviour**: when a user asks a question about ANY of the above areas, your first action should be the matching `/experts:<X>:question`, NOT a grep / Read. The expert will tell you which file to read if it doesn't already know.

**When to call self-improve manually:**
- After a big refactor in one domain → `/experts:<X>:self-improve true`
- Before a heavy session in a domain (so context is fresh) → same
- Multiple domains touched at once → `/sync-experts --auto` (selective by diff) or `/sync-experts --all`

**Auto-sync at session close:**
- A `SessionEnd` hook spawns a headless `claude --print "/sync-experts --auto"` in a fresh context
- It diffs your work since `origin/main` + working tree, maps changed files to affected experts via `_meta.yaml`, refreshes only those
- Log: `.claude/.last_sync.log`. Lock: `.claude/.sync.lock.d/` (atomic mkdir).
- Skips when repo is fully clean or `reason == "clear"`

**Full system docs**: `.claude/commands/experts/README.md` (architecture, file layout, how to add a new expert).

## Documentation

- `.claude/commands/experts/README.md` — expert system (what / how / adding new ones)
- `docs/Updates/` — V4 session dev logs (most recent state, by date)
- `docs/PRE_LAUNCH_TASKS.md` — pre-release task tracker (waves B2/B3/B4/B7/AD1/...)
- `docs/CATALOG_GAPS.md` — live catalog gap tracker
- `docs/LAUNCH_ROADMAP.md` — launch roadmap (phases 1-7)
- `docs/archive/` — old specs (ARCHITECTURE, VISUAL_ASSEMBLY_ENGINE, LAYOUT_ENGINE_SPEC, GLOSSARY, SPEC_TWO_AGENT_PIPELINE, etc.)
- `AI_docs/Manifesto.md` — product vision
- `AI_docs/ARCHITECTURE_RULES.md` — architectural principles
