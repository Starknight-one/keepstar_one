# Admin Catalog — M1-M3 schema/types/parser + Shopify OAuth productionised

- **Branch:** `main`
- **Date (UTC):** 2026-04-26 10:28
- **Commits in this session (oldest first):**
  - `c0e776c` — docs(admin-catalog): final design for catalog management
  - `54b2a66` — docs: refresh status — DONE banners, drop stale specs, sync tracker with git
  - `c99589f` — feat(admin-shopify): env-driven scopes + API version, ready for prod OAuth
  - `d87a676` — fix(admin-shopify): install endpoint returns JSON, frontend fetches+redirects
  - `ad2386a` — feat(admin-catalog): M1 — additive schema migrations for catalog spec
  - `8065fcc` — feat(admin-catalog): M2 — domain types, ports, skeleton adapters
  - `71d5d7c` — feat(admin-catalog): M3 — units parser (deterministic, English-only)

## Context

Two parallel tracks converged today:

1. **Production Shopify OAuth.** Existed as scaffolding from `b8adcd1` (admin onboarding MVP) but never finished — hardcoded API version (2024-07), hardcoded narrow scope set, no `PUBLIC_BASE_URL` env on Railway, install endpoint did a 302 redirect that browsers stripped the bearer header from, and the Shopify App's own configuration was wrong (Redirect URL pointed to `/oauth/callback` while backend listens on `/callback`). Net effect: nothing worked end-to-end. We made it work end-to-end against a real dev store (`keepstar-neaqpan1.myshopify.com`) by wiring env-driven config, fixing the install endpoint to return JSON instead of 302, and correcting the dashboard URL.

2. **Admin Catalog spec implementation, milestones 1-3.** Following the implementation plan at `docs/New features/admin_catalog_implementation_plan_2026-04-26.md`. M1 lays down all new tables additively (no breaking changes). M2 brings domain types, ports and pgx adapters with compile-time interface conformance. M3 ships a deterministic state-machine units parser (tests pass on 40 cases) and seeds global unit aliases.

The combination means: the wiring to pull a real catalog from a real Shopify dev store works against the OLD schema today (17 fashion products synced through the legacy importer), AND the NEW schema is in place + ready to be filled by M4 (new metadata-first import path).

## Shopify OAuth — what was wrong, what was fixed

| Symptom | Root cause | Fix |
|---|---|---|
| `/integrations/shopify/install` returned blank page | `ADMIN_ENCRYPTION_KEY` not set on Railway → `secretBox=nil` → `integrationsAdapter=nil` → handler not registered → SPA fallback served `index.html` | Generated 32-byte base64 key, set `ADMIN_ENCRYPTION_KEY` on Railway Admin |
| `{"error":"missing token"}` after deploy | `window.location.href` to install URL doesn't carry `Authorization: Bearer` header (lives in localStorage) | Backend `HandleInstall` now returns `{install_url}` JSON; frontend `api.get` with bearer, then `window.location.href = res.install_url` |
| OAuth scopes hardcoded to `read_products,read_product_listings,read_inventory` | `client.go` had `q.Set("scope", "read_products,...")` literal | Scopes come from `SHOPIFY_SCOPES` env (default preserved). Persisted scopes in integration record use new `client.Scopes()` getter — no more stale string |
| API version hardcoded to `2024-07` | `client.go` had `const apiVersion = "2024-07"` | Now `c.apiVersion`, defaults to `2026-04`, env-overridable via `SHOPIFY_API_VERSION` |
| Redirect URL in Shopify App config wrong | I wrongly told the user to enter `/admin/api/integrations/shopify/oauth/callback`; backend route is `/admin/api/integrations/shopify/callback` | User updated the Shopify App version Redirect URL in dev dashboard |

Final OAuth flow proven on 2026-04-26 ~07:59 UTC: install → consent → callback → token exchange → encrypted persist → webhook registration → initial sync. 17 products imported in 3 seconds (`shopify_initial_sync_completed total=17`).

## Catalog M1 — schema (commit `ad2386a`)

13 new tables, all idempotent (`IF NOT EXISTS`), additive. Old tables untouched.

| Table | Purpose |
|---|---|
| `catalog.master_variants` | Parent-child variant model (Option C). One row per SKU. `gtins TEXT[]` for primary match key. Dual-store unit fields (`weight_g` + `weight_raw` + `parse_status` JSONB). `variant_kind` enum (`real`/`addon`/`bundle`) |
| `catalog.master_cosmetics` | Per-vertical Tier 2 (Option B). PK = `master_variant_id`. Promotion targets land here |
| `catalog.master_categories` | Curator-owned global taxonomy with parent_id |
| `catalog.master_product_categories` | M:N (one master can be in many categories) |
| `catalog.tenant_categories` | Per-tenant copy of their collection tree, `kind` distinguishes `category`/`showcase`/`promo` |
| `catalog.category_mapping` | tenant_category → master_category (NULLable for showcase/promo) |
| `catalog.tenant_listing_categories` | M:N listing → tenant_categories |
| `catalog.master_attribute_candidates` | Staging with embedding for "search before promote". UNIQUE on (key, vertical) |
| `catalog.master_category_candidates` | Same idea for category staging |
| `catalog.tenant_variant_candidates_junk` | Gift-wrap / engraving / addon detection queue |
| `catalog.audit_log` | Append-only. System bulk = aggregate_meta; human edits = field_changes per-field |
| `catalog.tenant_catalog_schema` | Per-tenant `mapping_artifact` JSONB + status (active/stale/needs_human_review) |
| `catalog.unit_aliases` | Tenant-aware alias resolution (`tenant_id IS NULL` = global). 24 English seed rows |
| `catalog.api_keys` | Public REST API auth (bcrypt hash + revoke timestamp) |

Extensions to existing tables:
- `catalog.products`: `master_variant_id`, `original_name`, `display_name`, `raw_attributes JSONB`, `media JSONB`, `source_system`, `source_id`, `payload_hash`
- `catalog.master_products`: `vertical` (default `cosmetics`), `tier3 JSONB`, `confidence` enum

Verified via psql: `\dt catalog.*` shows 24 tables, `\d catalog.master_variants` shows correct columns/indexes/FK/CHECK constraints, `SELECT COUNT(*) FROM catalog.unit_aliases WHERE tenant_id IS NULL` → 24 (English-only after cleanup).

## Catalog M2 — domain types + ports + adapters (commit `8065fcc`)

| Layer | Files |
|---|---|
| Domain | `internal/domain/{master_variant,candidates,audit,mapping_artifact,master_category}.go` |
| Ports | `internal/ports/{master_variants,candidates,audit,mapping_artifact,categories}_port.go` |
| Adapters | `internal/adapters/postgres/{master_variants,candidates,audit,mapping_artifact,categories_v2}_adapter.go` |

Each adapter has a `var _ ports.X = (*Y)(nil)` compile-time conformance check. Adapters are functional CRUD — not stubs — so M4-M12 can wire them in directly without re-implementing basic INSERT/SELECT. Match cascade fuzzy/embedding queries are NOT here yet — those land in M5 alongside the cascade usecase.

Not wired in `cmd/server/main.go` yet — adapters compile in isolation. Existing `CatalogAdapter` / `AdminCatalogPort` untouched, so V4 chat and admin frontend keep working unchanged.

## Catalog M3 — units parser (commit `71d5d7c`)

`internal/units/` — pure-Go state machine, no DB calls in hot path, no LLM ever.

- `units.go` — canonical units (`mL`/`g`/`mm`/`pcs`) + Dimension classification + in-code authoritative conversion table (`rawTokenFactors`)
- `parser.go` — `Parse(input, opts) Result` with patterns: `<num><unit>` / `<num>x<num><unit>` (multi-pack incl. spaces and `×`) / `<num><unit>/<num><unit>` (dual-label, 2% tolerance) / bare `<num>` (ambiguous unless `ParseOpts.DefaultUnit` set) / anything else = failed. Multi-pack stores `unit_count` separately (`2x30ml` → `volume_ml=30, unit_count=2`, NOT 60).
- `aliases.go` — `AliasResolver` interface + `InMemoryResolver` (tests) + `PostgresAliasResolver` (per-tenant DB lookup, sync.Map cache, falls back to global table when no tenant override)
- `parser_test.go` — 40 subtests pass: Volume / Mass / Length / Count / Multi-pack / Dual-label / Bare number with default unit / Junk inputs / Tenant resolver override / normalize / dimension. English-only.

## Files changed (cumulative this session)

| Scope | File | New/Edit |
|---|---|---|
| Spec doc | `docs/New features/admin_catalog_design_2026-04-23.md` | New (commit `c0e776c`) |
| Plan doc | `docs/New features/admin_catalog_implementation_plan_2026-04-26.md` | New (this session, copied from `~/.claude/plans/`) |
| Doc refresh | `docs/PRE_LAUNCH_TASKS.md` + 5 SPEC banners + 5 deletions | Edit (commit `54b2a66`) |
| Shopify backend | `project_admin/backend/internal/adapters/shopify/client.go` | Edit (env-driven, JSON return) |
| Shopify backend | `project_admin/backend/internal/handlers/handler_integrations_shopify.go` | Edit |
| Shopify backend | `project_admin/backend/internal/usecases/shopify.go` | Edit |
| Shopify backend | `project_admin/backend/internal/config/config.go` | Edit |
| Shopify frontend | `project_admin/frontend/src/features/integrations/ShopifyConnectPage.jsx` | Edit |
| Catalog M1 | `project_admin/backend/internal/adapters/postgres/catalog_migrations.go` | Edit (added 292 lines additive) |
| Catalog M2 domain | `internal/domain/{master_variant,candidates,audit,mapping_artifact,master_category}.go` | New (5 files) |
| Catalog M2 ports | `internal/ports/{master_variants,candidates,audit,mapping_artifact,categories}_port.go` | New (5 files) |
| Catalog M2 adapters | `internal/adapters/postgres/{master_variants,candidates,audit,mapping_artifact,categories_v2}_adapter.go` | New (5 files) |
| Catalog M3 | `internal/units/{units,parser,aliases,parser_test}.go` | New (4 files) |

## Verification

- `cd project_admin/backend && go build ./... && go vet ./...` — clean.
- `go test ./internal/units/... -v` — 40 subtests pass.
- Railway Admin redeploy → `[INFO] catalog_migrations_completed`, `[INFO] encryption_initialized`, `[INFO] shopify_integration_enabled`.
- psql against Neon prod: 24 catalog tables present, master_variants schema correct (gin index on gtins, FK CASCADE to master_products, CHECK constraint on variant_kind), 24 unit_aliases seeded.
- Manual end-to-end OAuth on `keepstar-neaqpan1.myshopify.com`: install → consent → callback → token saved → 17 products synced in 3s → status `connected`.

## Known gaps / caveats

- **Old `catalog.unit_aliases` cyrillic seed rows** (мл/г/м/шт) inserted by the first deploy are still present in Neon. They won't match anything because the in-code `rawTokenFactors` map has no cyrillic, but they exist as junk rows. Harmless. Drop manually if curator wants a clean alias list — `DELETE FROM catalog.unit_aliases WHERE raw_token IN ('мл','г','м','шт')`.
- **17 dev-store products are still in OLD schema** (`catalog.master_products` + `catalog.products` without `master_variant_id`). M4 (or M7-style backfill) will migrate them.
- **New M2 adapters not wired in `main.go`.** They compile in isolation. Wiring happens in M4 onward as needed.
- **Custom App test detour was rejected.** I initially suggested a Custom App shortcut to avoid writing the OAuth callback; user pushed back ("делать сразу нормально") so we instead fixed the env + frontend bug and used the actual public-app OAuth flow. Better outcome.
- **Plan file lives in two places now.** Original at `~/.claude/plans/quizzical-sleeping-salamander.md` (auto-generated by plan mode), copy at `docs/New features/admin_catalog_implementation_plan_2026-04-26.md` (in repo, accessible from any future session). The repo copy is the source of truth — edit it, not the plan-mode file.
- **`Shopify Client Secret`** appeared in chat history during today's setup (value redacted from this log). It's stored on Railway env as `SHOPIFY_API_SECRET`. **Should be rotated before any non-dev exposure.** Track this for production hardening.
- **Encryption key just generated** (`ADMIN_ENCRYPTION_KEY` on Railway). If this key is ever lost, all encrypted Shopify access tokens become unrecoverable. Back it up to a secret manager before production.

## Next session entry point

Read these in order:
1. `docs/New features/admin_catalog_design_2026-04-23.md` (the spec — context)
2. `docs/New features/admin_catalog_implementation_plan_2026-04-26.md` (the plan — milestones)
3. This update log (state of M1-M3 done, M4 next)
4. `docs/PRE_LAUNCH_TASKS.md` (broader context; admin catalog tracked as 5.2 → in-progress)

Pick up at **M4 (new Shopify metadata-first import)**. Implementation strategy options were discussed but not chosen — see this log's "Final exchange" for the A/B/C decision.
