# Keepstar One Ultra

AI-powered SaaS B2B2C chat widget for e-commerce. The user types in
chat — the bot answers with interactive widgets (product cards,
galleries, comparisons, detail views) composed dynamically by a
two-agent LLM pipeline. The merchant embeds a single `<script>` tag
and gets an AI assistant with visual answers, no in-house dev work.

V5 is the production chat engine (`project_v5/`, live at
`v5-engine-production.up.railway.app`). Admin SPA and curator service
share the same Neon Postgres.

## Getting around

- **`CLAUDE.md`** — working context for AI assistants: routing to per-domain experts, dev essentials, 12 working rules.
- **`.claude/commands/experts/README.md`** — per-domain expertise (catalog, engine-v5, pipeline-agents, widget, admin, curator). Self-updates from code via a SessionEnd hook.
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
cd project_admin/backend  && go run ./cmd/server         # :8081
cd project_admin/frontend && npm run dev                 # :5174
cd curator/backend        && go run ./cmd/server         # :8082
cd curator/frontend       && npm run dev                 # :5175
```

DB: shared Neon Postgres. `DATABASE_URL` lives in `project_v5/.env`.

## Deployed services

| Service | URL |
|---|---|
| V5 chat engine | `v5-engine-production.up.railway.app` |
| Admin | `admin-production-4ae4.up.railway.app` |
| Curator | `curator-production-46d7.up.railway.app` |
| Landing | `keepstar.one` |

## Embed widget

```html
<script
  src="https://v5-engine-production.up.railway.app/widget.js"
  data-tenant="YOUR_TENANT_SLUG"
  data-api="https://v5-engine-production.up.railway.app/api/v1"
></script>
```

## License

Proprietary. All rights reserved.
