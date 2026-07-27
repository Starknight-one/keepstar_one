# Keepstar One Ultra

The generation engine of the **Keepstar interface runtime** — data +
operations + interface: anything can be visualized to resolve a user's
question the moment they ask. An LLM decides what to show and which
operation to run; a deterministic engine binds real tenant data and
executes — nothing is hallucinated. Chat (the embeddable `<script>`
widget) is one door into the runtime; the headless API for AI agents is
the other. Commerce is one vertical, not the identity. The missing
piece today is the **operations layer** — a catalog of validated
operations with strict inputs/outputs — the main architecture front.

**This repo is the V5 engine core** (`project_v5/`) — Admin, Curator,
and the Landing were extracted to their own repos (`keepstar-admin` /
`keepstar-curator` / `keepstar-landing`). All services share the same
Neon Postgres (flat-moon, owned by Admin), which is alive with data
(3172 products, 35 tenants). All Railway deployments were taken down in
July 2026 — the engine currently runs locally (see below).

> **New here?** Start with [`../MANIFESTO.md`](../MANIFESTO.md) —
> product canon (course 2026-07-27: Keepstar = interface runtime).
> Superseded plans live in `../archive/`. The project is decomposed
> into focused services (PIM · Connector · Price-Stock · v5-engine);
> this repo is slimmed to the v5 engine core. See `CLAUDE.md` for
> day-to-day navigation.

## Getting around

- **`CLAUDE.md`** — working context for AI assistants: routing to per-domain experts, dev essentials, 12 working rules.
- **`.claude/commands/experts/README.md`** — per-domain expertise (engine-v5, pipeline-agents, widget). Self-updates from code via a SessionEnd hook.
- **`docs/Updates/`** — chronological session dev logs.
- **`docs/CATALOG_GAPS.md`**, **`docs/v5-known-gaps.md`**, **`docs/PRE_LAUNCH_TASKS.md`** — live trackers.

## Run locally

```bash
./scripts/start_all.sh
```

Or per-service:

```bash
cd project_v5/backend     && go run ./cmd/server         # :8084
cd project_v5/frontend    && npm install && npm run dev  # :5173
```

DB: shared Neon Postgres. `DATABASE_URL` lives in `project_v5/.env`.

## Deployment status

All services (V5 engine, Admin, Curator, Landing) were taken off
Railway in July 2026 — the old URLs (e.g.
`v5-engine-production.up.railway.app`) are dead. The Neon Postgres
(flat-moon) remains alive with data. Current mode: local bring-up via
`scripts/start_all.sh`; next architecture front is the operations
layer (see `../MANIFESTO.md`).

## Embed widget

There is no public deployment right now. Against a running V5 backend
(locally `http://localhost:8084`):

```html
<script
  src="https://<v5-host>/widget.js"
  data-tenant="YOUR_TENANT_SLUG"
  data-api="https://<v5-host>/api/v1"
></script>
```

## License

Proprietary. All rights reserved.
