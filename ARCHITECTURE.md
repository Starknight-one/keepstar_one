# v5-engine — handoff

> Canonical owner-handoff for the V5 chat engine (Go module `keepstar_v5`).
> Source: `/Users/starknight/Keepstar_project/Keepstar_one_ultra/project_v5`.
> Verified by reading code on 2026-06-02. Where this doc and older notes
> disagree, the code wins — re-verify any `file:line` claim before acting.

## Purpose & responsibilities

V5 is the **production chat engine** behind the Keepstar AI shopping
widget. A shopper types a natural-language query in an embedded chat; V5
answers with an **interactive scene-graph widget** (product cards,
grids, detail views) composed dynamically by a two-agent LLM pipeline.

It owns three things:

1. **The two-agent pipeline.** Agent1 (data) runs `catalog_search` /
   state-filter / history-lookup tools to fill the session's data zone;
   Agent2 (render) runs `visual_assembly` to produce a scene-graph
   document (template zone). Orchestrated by
   `internal/usecases/pipeline_execute.go`.
2. **The scene-graph engine.** A Go port of the v9 scene-graph engine
   (14 node types, 5 ops, presets, `$ref` resolution, binding, TreeMap,
   replicate) in `internal/engine/`. It materialises a preset + reusable
   components, resolves refs, binds catalog data, and injects default
   actions (like / cart_add).
3. **The widget frontend.** A React + Shadow-DOM IIFE bundle
   (`widget.js`) the merchant embeds with one `<script>` tag. It renders
   the scene-graph document and dispatches user actions/navigation back
   to the backend.

Explicit non-goals (owned elsewhere): catalog ingest, product editing,
master-data curation. V5 **reads** the shared catalog read-only and
never writes it.

## Architecture (frontend / backend, key dirs)

Single repo, two stacks, one Docker image (multi-stage build: widget →
Go binary → alpine runtime). The Go binary serves both the JSON API and
the built widget static assets from the same origin.

### Backend — Go (`backend/`, module `keepstar_v5`, Go 1.24)

Hexagonal/ports-and-adapters layout:

- `cmd/server/main.go` — entry point. Wires config → DB → migrations →
  adapters → ports → tools → use cases → handlers → `http.Server` with
  graceful shutdown.
- `internal/config/config.go` — env-driven config (`Load`/`MustLoad`).
- `internal/domain/` — typed entities: `Product`, `Tenant`,
  `SessionState`, `CatalogDigest`, `Span`, `Trace`, `Delta`,
  `UserActionKind`, scene-graph value objects, etc.
- `internal/engine/` — the v9 scene-graph port. Core flow:
  `Materialise` → `ResolveAndInline` (`$ref`) → `BindData` →
  `InjectDefaultActions`; plus `ExpandReplicates`, `apply_ops`
  (insert/update/delete/move/override), `tree_map`, `ProductToMap`.
  `engine/presets/` holds the embedded **system fallback registry**
  (`seed/*.json`: product_card, product_detail, empty_not_found, etc.)
  and the `SystemAdjacency` drill-target map.
- `internal/ports/` — interfaces: `CatalogPort`, `StatePort`,
  `PresetPort`, `ComponentPort`, `FieldDefinitionPort`, `EmbeddingPort`,
  `LLMPort`, `TracePort`.
- `internal/adapters/postgres/` — concrete ports over pgx/v5 + pgvector,
  plus the idempotent V5 migrations (state / preset / component / trace).
- `internal/adapters/anthropic/` — Anthropic chat client (the LLM),
  cost accounting, prompt-cache placement.
- `internal/adapters/openai/` — OpenAI embeddings client (optional).
- `internal/tools/` — LLM tools: `catalog_search`, `visual_assembly`
  (v5), `_internal_state_filter`, `_internal_history_lookup`, registry.
- `internal/usecases/` — `pipeline_execute`, `agent1_execute`,
  `agent2_execute`, prompt caches, prefetch builder, state
  reconstruct/rollback.
- `internal/handlers/` — HTTP handlers + middleware (logging → CORS →
  tenant) + routes + the pipeline rate/spend guard.
- `internal/prompts/`, `internal/measure/tokens/` — system prompts,
  `<fields>` block builder, token-budget measurement harness.

### Frontend — React 19 + Vite (`frontend/`)

- `src/widget.jsx` — IIFE entry. Creates a host `<div>`, attaches an
  **open Shadow DOM**, injects CSS inline, mounts `<WidgetApp>`. Derives
  the API base URL from `data-api`, else the embed script's origin
  (`{origin}/api/v1`), else dev fallback `http://localhost:8084/api/v1`.
- `src/WidgetApp.jsx`, `src/chat/` — chat panel + message list.
- `src/renderer/` — `SceneGraphRenderer`, `NodeRenderer`, node
  components (`Frame`/`Group`/`Image`/`Ref`/`Text`), `fillTemplate`,
  `format` (the `format`/`wrapper` → display-string step the backend
  deliberately does NOT do), `actionDispatch`.
- `src/api/client.js`, `src/api/actions.js` — fetch wrappers; always
  send `X-Tenant-Slug`.
- Build output: `dist/widget.js` (IIFE, ~206 KB / ~64 KB gz) + assets,
  copied into the image as `./static` and served by the Go server.

## Inputs & Outputs (endpoints + contracts)

All endpoints are plain **HTTP/JSON** (no OpenAPI/Swagger spec exists in
the repo). Tenant is resolved by the `WithTenant` middleware from the
`X-Tenant-Slug` request header, falling back to the `TENANT_SLUG` env
default. Endpoint catalog is documented in
`internal/handlers/routes.go`.

| Method & path | Purpose | Request | Response |
|---|---|---|---|
| `GET /healthz` | Liveness (process up). | — | `{"status":"ok"}` |
| `GET /readyz` | Readiness (DB ping). | — | 200 / 503 |
| `POST /api/v1/session/init` | Create a chat session. Inserts `v5_chat_sessions` row + shell state. | empty body; `X-Tenant-Slug` | `{sessionId, tenant:{slug,name}}` |
| `GET /api/v1/session/{id}` | Read session state (debug). | — | `{sessionId, step, current, view}` |
| `POST /api/v1/pipeline` | **The main turn.** Runs Agent1 → Agent2. Rate-limited + daily spend-capped. | `{sessionId, query}` | `{document, toolCalls, usage, latencyMs, agent1Ms, agent2Ms, prefetch?, spans?}` |
| `POST /api/v1/actions[?sync=true]` | Backend-resolved actions: `like`/`unlike`/`cart_add`/`cart_remove`. Other kinds → 400. | `{sessionId, kind, entity:{type,id}, params?}` | `{success, actions}` (or 204 on `sync=true`) |
| `POST /api/v1/navigation/expand` | Drill into detail preset (no LLM; uses `SystemAdjacency` map). | `{sessionId, entityType, entityId, turnId?}` | `{success, document, viewMode, focused, stackSize, canGoBack, presetInUse}` |
| `POST /api/v1/navigation/back` | Pop view stack, restore prior template. | `{sessionId, turnId?}` | same `navResponse` shape |
| `GET /` (+ static) | Serves the built widget bundle (`widget.js`, `widget.html`) from `STATIC_DIR` when present; SPA-style `index.html` fallback. | — | static files |

Guard behaviour on `/api/v1/pipeline`: 429 when per-IP rate limit
exceeded (`PIPELINE_RATE_PER_MIN`, default 30/min), 503 when the global
daily USD cap is hit (`PIPELINE_DAILY_USD_CAP`, default 10.0). The cap
is process-local — correct for the current single-instance deploy.

### Outbound calls V5 makes

- **Anthropic Messages API** (HTTP) — both agents, model from
  `LLM_MODEL` (default `claude-haiku-4-5`; `.env` pins
  `claude-haiku-4-5-20251001`). REQUIRED.
- **OpenAI Embeddings API** (HTTP) — `text-embedding-3-small` @ 384
  dims, used by `catalog_search` for the vector half. OPTIONAL — when
  `OPENAI_API_KEY` is blank, search degrades to FTS/keyword-only.

## Data (owns / reads)

This service deliberately **mixes two integration styles** — be honest
about which is which:

### Owns (read-write) — own `v5_*` tables in the shared Neon DB

Created by V5's own idempotent migrations at boot (`CREATE TABLE IF NOT
EXISTS`, run in `cmd/server/main.go`):

- `v5_chat_sessions` — session rows (`state_migrations.go`).
- `v5_chat_session_state` — current data/template/view/actions/history
  per session.
- `v5_chat_session_deltas` — append-only delta stream (audit + state
  reconstruct).
- `v5_chat_session_traces` — per-turn trace + cost/latency/spans
  (`migrations_trace.go`); read by the Curator "Chats" UI.
- `v5_presets` / `v5_preset_versions` and `v5_components` /
  `v5_component_versions` — preset + reusable-component storage
  (`preset_migrations.go`, `component_migrations.go`). This is the
  **final shape**; a future canvas microservice is intended to write
  these as a client. DB rows win over the embedded system fallback
  registry.

### Reads (READ-ONLY) — shared `catalog.*` schema ("flat-moon"), owned by Admin

V5 **never writes** `catalog.*`. This is a **direct shared-database
read**, not an HTTP/API call to Admin — V5 connects to the same Neon DB
via `DATABASE_URL` and queries these tables directly:

- `catalog.tenants` — tenant resolution by slug.
- `catalog.products`, `catalog.master_products`, `catalog.master_variants`,
  `catalog.master_categories`, `catalog.master_product_categories` — the
  product read model (two-path master resolution; typed attrs live in
  `master_products.tier2` jsonb after the "Group D" rework).
- `catalog.tenant_search_projection` — per-tenant FTS + embedding
  projection; the single read behind `SearchProjection` (the fused
  search the `catalog_search` tool uses).
- `catalog.field_definitions` — OPTIONAL per-tenant render vocabulary
  for Agent2's `<fields>` block. Reads tolerate a missing table
  (SQLSTATE 42P01) and fall back to data-derived fields. See gaps.
- `catalog.product_ingredients`, `catalog.stock`, `catalog.categories`
  — legacy/referenced in code; several dropped by Group D (queries
  degrade to empty when absent).

Because `catalog.*` is a shared DB read, **V5 is coupled to Admin's
schema**. Admin migrations that drop/rename catalog tables can silently
break V5 at runtime (see the `field_definitions` incident in gaps).

## Integrations (who ⇄ how)

| Counterparty | Direction | How (HTTP API vs shared DB) |
|---|---|---|
| **Anthropic** | V5 → Anthropic | HTTP API (Messages). The LLM for Agent1 + Agent2. |
| **OpenAI** | V5 → OpenAI | HTTP API (Embeddings). Optional; feeds vector search. |
| **Admin / catalog ("flat-moon")** | V5 → catalog | **SHARED DATABASE, read-only.** Same Neon DB via `DATABASE_URL`; direct SQL on `catalog.*`. No HTTP call to Admin. |
| **Curator** | Curator → V5 data | **SHARED DATABASE.** Curator's backend reads V5's `v5_chat_*` tables directly (its own pgxpool) for the "Chats"/tracing UI. No HTTP between them. |
| **Merchant storefront / shopper** | Browser → V5 | HTTP/JSON API + embedded `widget.js` served from the same origin. |
| **Future canvas microservice** | (planned) → V5 tables | Intended to write `v5_presets`/`v5_components` as a DB client. Not built. |

## Run locally

Prereqs: Go 1.24, Node 22, access to the Neon DB (and API keys) — local
`.env` already has working dev credentials.

Backend:

```sh
cd project_v5/backend
go build ./...
# Env (or use project_v5/.env): DATABASE_URL + ANTHROPIC_API_KEY required;
# OPENAI_API_KEY optional; PORT defaults 8082, dev uses 8084.
go run ./cmd/server/
```

Frontend (widget dev server, port 5173):

```sh
cd project_v5/frontend
npm install
npm run dev          # vite; widget served via src/, derives api from devConfig/fallback 8084
npm run build        # produces dist/ (widget.js) — what the Docker image serves
```

Tests:

```sh
cd project_v5/backend && go test ./...          # unit; some integration tests behind build tags / live DB
cd project_v5/frontend && npm test              # vitest
```

Repo-wide convenience: `Keepstar_one_ultra/scripts/start_all.sh` (stop:
`stop_all.sh`). `psql` on this box:
`/opt/homebrew/Cellar/libpq/18.1_1/bin/psql`.

No checked-in `.env.example` — `project_v5/.env` is the de-facto example
(contains live dev secrets; do not commit derivatives).

## Deploy (Railway service + required env vars)

- **Railway service:** `v5-engine`, live at
  `https://v5-engine-production.up.railway.app`.
- **Build:** repo-root `Dockerfile` (multi-stage). Railway service
  config uses `rootDirectory="/"` (build context is the monorepo root;
  the Dockerfile copies `project_v5/frontend` and `project_v5/backend`).
  No `railway.json`/`railway.toml` in the repo — service config lives in
  the Railway dashboard.
- **Listen port:** image `EXPOSE 8084`; `PORT` env overrides
  (`config.go` default `8082`).
- **DB:** shared Neon Postgres (flat-moon, owned by Admin), US East.

Environment variables (from `config.go` + `.env`):

| Var | Required | Default | Notes |
|---|---|---|---|
| `DATABASE_URL` | yes | — | Neon Postgres URL; boot fails loud if missing. |
| `ANTHROPIC_API_KEY` | yes | — | LLM; boot fails loud if missing. |
| `OPENAI_API_KEY` | no | "" | Embeddings; blank → keyword/FTS-only search. |
| `LLM_MODEL` | no | `claude-haiku-4-5` | `.env` pins `claude-haiku-4-5-20251001`. |
| `TENANT_SLUG` | no | `hey-babes-cosmetics` | Fallback tenant when no `X-Tenant-Slug` header. |
| `PORT` | no | `8082` | Image exposes 8084. |
| `STATIC_DIR` | no | `./static` | Where the built widget bundle lives; empty disables static serving. |
| `LOG_LEVEL` | no | `info` | debug/info/warn/error (slog). |
| `PIPELINE_RATE_PER_MIN` | no | `30` | Per-IP token-bucket on `/api/v1/pipeline`; ≤0 disables. |
| `PIPELINE_DAILY_USD_CAP` | no | `10.0` | Global daily Anthropic spend ceiling (process-local); ≤0 disables. |

Migrations run automatically on boot (idempotent), so a fresh
environment self-provisions its `v5_*` tables.

## Existing docs (filenames)

In-repo (`project_v5/`):
- `project_v5/backend/README.md` — layout + dev commands. NOTE: stale
  header ("Not wired to HTTP/DB/LLM yet") — long since wired and
  deployed; trust the code, not this line.
- `project_v5/.env` — de-facto env example (live dev secrets).

Parent repo (`Keepstar_one_ultra/`):
- `CLAUDE.md` — project overview + routing to "experts".
- `docs/v5-known-gaps.md` — **live** A-series gap registry (the richest
  source of behavioral truth; see Gaps below).
- `docs/v5-engine-plan.md` — HISTORICAL delivery plan (2026-05-03,
  superseded).
- `docs/v5-port-from-openui.md`, `docs/v5-tool-format-decision.md`,
  `docs/v5-smoke/` — V5 design/port notes + smoke artifacts.
- `docs/CATALOG_GAPS.md`, `docs/CATALOG_GROUP_D_SPEC.md`,
  `docs/CATALOG_V2_TEST_SPEC.md`, `docs/catalog-audit-2026-05-07.md`
  (HISTORICAL) — the catalog schema V5 reads.
- Expert system: `.claude/commands/experts/` — `engine-v5`,
  `pipeline-agents`, `widget` experts self-update from code.

No OpenAPI/Swagger spec exists for the HTTP API.

## Gaps & known issues

Authoritative live list is `docs/v5-known-gaps.md`. Highlights an owner
must know:

- **Shared-DB coupling to Admin is the #1 operational risk.** Admin
  migrations that drop/rename `catalog.*` break V5 silently at runtime.
  Real incident (2026-05-30, fixed `405d740`): Admin dropped
  `catalog.field_definitions`; Agent2 `/pipeline` 500'd on **every**
  tenant. Fix made the field-definitions read fail-OPEN. Re-audit any
  other hard `catalog.*` dependency.
- **Open A-series parity gaps vs the old V4 engine** (independent, each
  its own fix): A1 greeting handling (renders `empty_not_found` on
  "hi"); A2 modify-vs-rebuild misclassification (new category sometimes
  skips `catalog_search`); A3 hard replicate=3 / no pagination; A4 no
  "← Back" button in widget despite the endpoint existing; A5 Agent2
  always runs even on Agent1 no-op (~1s + ~$0.001 wasted/turn); A6 card
  width/density narrower than V4.
- **Agent1 catalog digest is vertical-locked to cosmetics** (B1) —
  `BuildCatalogDigest` hardcodes skin_type/concern/ingredients/etc.;
  search quality degrades on non-cosmetics verticals.
- **Agent2 has no result-set digest** (B2) — only a 1-line count via
  `<microcontext>`; no category/price-range context for the render step.
- **`SampleFieldValues` is cosmetics-shaped** — reads `products.extra`
  but not `master_products.tier2`, so other-vertical specs miss the
  `<fields>` block.
- **Daily spend cap is process-local** — correct only for the current
  single instance; revisit if V5 scales horizontally.
- **Trace persistence is best-effort/async** — a failed background
  INSERT only logs a warning; traces can be lost without surfacing.
- **State reconstruct replay is partial** — `applyDelta` stubs
  push/pop/rollback/remove (only add/update + template replay are
  fully correct).
- **No OpenAPI spec / no checked-in `.env.example`** — contracts live in
  handler code; secrets live in `project_v5/.env`.
- **`backend/README.md` is stale** ("not wired yet"); ignore that line.
