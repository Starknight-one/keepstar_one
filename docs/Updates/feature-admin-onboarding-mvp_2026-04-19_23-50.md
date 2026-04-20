# Admin onboarding MVP — CSV + Shopify

- **Branch**: `feature/admin-onboarding-mvp`
- **Date (UTC)**: 2026-04-19 23:50
- **Parent commit**: `5231cb3`
- **Plan**: `/Users/starknight/.claude/plans/groovy-soaring-aho.md`
- **Worktree**: `.claude/worktrees/admin-onboarding`

## Context

The admin panel only accepted JSON uploads in Keepstar's internal shape, which
meant every new merchant required a bespoke export+transform project. This
change lands the MVP of a generic onboarding flow: a new **Integrations** area
with three sources (Shopify, CSV, Google Sheets) and the two MVP paths fully
wired — CSV with AI-assisted column mapping, and Shopify via OAuth Custom
Distribution with webhooks for live updates.

Phase 0 puts the shared foundation in place: AES-256-GCM at-rest encryption
for OAuth tokens, the `admin.tenant_integrations` / `admin.oauth_states`
tables, source-tracking columns on `catalog.master_products`
(`source_system`, `source_id`) with a partial unique index for idempotent
re-imports, and a `deleted_at` column on `catalog.products` so Shopify
`products/delete` webhooks soft-delete rather than orphan.

Phase 1 plugs a Haiku-backed mapping proposer onto a CSV uploader, confirmed
by the user in a dropdown UI, then transformed into `ImportItem`s and run
through the existing import pipeline.

Phase 2 is the Shopify flow: signed state nonce → OAuth consent → offline
token → encrypted persistence → webhook registration for
`products/{create,update,delete}`, `inventory_levels/update`, `app/uninstalled`
→ paginated initial sync. The webhook endpoint is HMAC-verified and runs
outside the auth middleware; the callback is protected by the signed state.

## Approach

### Encryption layer

`internal/crypto/secretbox` is a ~100-line stdlib-only AES-256-GCM wrapper
with versioned envelopes (`v1.{nonce}.{ct}`) so future key rotation can ship
as v2.+dual-read without re-encrypting rows. Fail-closed: absent or malformed
`ADMIN_ENCRYPTION_KEY` logs a warning and disables all integrations (Shopify
plus CSV-as-an-integration row), while the rest of the admin keeps working.

### Integration persistence

Single port (`IntegrationsPort`) with CRUD + OAuth state ops. The adapter
seals `Credentials` on write, opens on read; decryption failures flip the
returned record to `status='error'` rather than surfacing garbage. OAuth
state is single-use: `ConsumeOAuthState` does `DELETE … RETURNING` so a replay
of the same callback URL fails atomically.

### Post-import enrichment autotrigger

`ImportUseCase` grew a narrow `EnrichmentTrigger` interface and a
`SetEnrichmentTrigger` setter, wired in `main.go` after both usecases are
built. The trigger fires `EnrichFromDBIncrementalAsync(tenantID)` after every
import finishes digest — landing products become searchable immediately and
PIM-enriched within minutes, without blocking the import's ack.

`EnrichmentUseCase` gained `EnrichFromDBIncremental(ctx, tenantID)` which
filters to `enrichment_version = 0` via a new
`GetUnenrichedMasterProducts`; when the filter yields zero rows (already-
enriched re-import), the usecase short-circuits to a no-op completed job
rather than erroring.

### CSV mapping

`CSVMappingUseCase.SuggestMapping` posts headers + 5 sample rows to Haiku
with a strict JSON schema and a whitelist of internal field names; anything
the model returns that isn't on the whitelist gets silently rewritten to
`ignore`. `ApplyMapping` is a pure function (no LLM) that transforms
confirmed mappings into `ImportItem`s with tolerant parsers for price/stock
("$12.99" → 12, "1,299" → 1299).

### Shopify

`adapters/shopify/client.go` is a thin HTTP client covering only what the
onboarding flow needs: OAuth exchange, Link-header cursor pagination,
per-product metafields, webhook registration, HMAC verify for both install
query strings and webhook bodies. Mapping lives in
`usecases/shopify_mapper.go` rather than the adapter to avoid the adapter→
usecase cycle that `ImportItem` would otherwise force.

`ShopifyUseCase` owns the lifecycle: `StartOAuth` mints a 10-minute state
row; `CompleteOAuth` verifies HMAC + state, exchanges the code, seals the
token, persists the integration, registers all webhook topics, and kicks off
the initial sync in a goroutine; `HandleWebhook` dispatches by topic —
`products/{create,update}` fan into `UpsertSingle` (no job row noise for
per-product changes), `products/delete` calls `SoftDeleteProductBySource`,
`inventory_levels/update` logs-and-skips (no product_id in the payload —
6h resync covers it), `app/uninstalled` flips status to `disconnected`.

### Frontend

`useJobPolling` extracts the polling loop that was inline in `ImportPage`
and now drives both CSV and future integrations. Three new route screens
(`/integrations`, `/integrations/csv`, `/integrations/shopify`) plus a
shared `MappingEditor` for the column picker. `papaparse` parses CSVs in
the browser and ships sample rows for the AI suggestion. Settings gained a
currency dropdown so the tenant can set the default (USD/EUR/GBP/…/BRL) that
`ImportUseCase.resolveCurrency` reads.

## Files changed

### New — Backend

| File | Purpose |
|---|---|
| `project_admin/backend/internal/crypto/secretbox/secretbox.go` | AES-256-GCM wrapper with v1. envelope |
| `project_admin/backend/internal/domain/integration.go` | `Integration`, `OAuthState`, kind/status enums |
| `project_admin/backend/internal/ports/integrations_port.go` | Persistence contract |
| `project_admin/backend/internal/adapters/postgres/integrations_adapter.go` | pgx impl with seal/open on credentials |
| `project_admin/backend/internal/usecases/integrations.go` | List/Get/Disconnect + OAuth state sweeper |
| `project_admin/backend/internal/handlers/handler_integrations.go` | `GET /integrations`, `GET|DELETE /integrations/{id}` |
| `project_admin/backend/internal/usecases/csv_mapping.go` | Haiku-backed mapping proposer + `ApplyMapping` |
| `project_admin/backend/internal/handlers/handler_integrations_csv.go` | `POST /integrations/csv/{suggest-mapping,import}` |
| `project_admin/backend/internal/adapters/shopify/client.go` | OAuth + REST + HMAC client |
| `project_admin/backend/internal/usecases/shopify.go` | StartOAuth/CompleteOAuth/InitialSync/HandleWebhook/FullResync |
| `project_admin/backend/internal/usecases/shopify_mapper.go` | `ShopifyProduct → ImportItem` (in usecases/ to break adapter cycle) |
| `project_admin/backend/internal/handlers/handler_integrations_shopify.go` | install/callback/webhook/resync routes |

### New — Frontend

| File | Purpose |
|---|---|
| `project_admin/frontend/src/shared/hooks/useJobPolling.js` | Reusable import-job polling hook |
| `project_admin/frontend/src/features/integrations/IntegrationsPage.jsx` + `.css` | Source cards + connected list |
| `project_admin/frontend/src/features/integrations/MappingEditor.jsx` | Per-column field picker with AI confidence badges |
| `project_admin/frontend/src/features/integrations/CSVUploadPage.jsx` | Drop-zone → AI suggest → confirm → import |
| `project_admin/frontend/src/features/integrations/ShopifyConnectPage.jsx` | Shop-domain input → full-page install redirect |

### Modified — Backend

| File | Change |
|---|---|
| `cmd/server/main.go` | Wire secretBox, integrationsAdapter, CSV/Shopify usecases+handlers, OAuth sweeper, unauthenticated webhook route, enrichment trigger attachment |
| `internal/config/config.go` | `EncryptionKey`, `ShopifyAPIKey/Secret`, `PublicBaseURL` |
| `internal/adapters/postgres/admin_migrations.go` | `admin.tenant_integrations`, `admin.oauth_states` |
| `internal/adapters/postgres/catalog_migrations.go` | `source_system`, `source_id`, unique index; `deleted_at` on products |
| `internal/adapters/postgres/catalog_adapter.go` | source tracking on upsert; `GetUnenrichedMasterProducts`, `SoftDeleteProductBySource` |
| `internal/ports/catalog_port.go` | Two new method signatures |
| `internal/domain/product.go` | `SourceSystem`, `SourceID` on `MasterProduct` |
| `internal/usecases/import.go` | `EnrichmentTrigger` hook, `UpsertSingle`, `UploadWithJobName`, per-tenant currency resolution, source tracking on `ImportItem` |
| `internal/usecases/enrichment.go` | `EnrichFromDBIncremental{,Async}` filtered by `enrichment_version=0` |
| `internal/usecases/auth.go` | Default currency RUB → USD |

### Modified — Frontend

| File | Change |
|---|---|
| `src/App.jsx` | Three new routes |
| `src/features/layout/DashboardLayout.jsx` | Integrations nav link with `Zap` icon |
| `src/features/import/ImportPage.jsx` | Replaced inline polling with `useJobPolling` |
| `src/features/settings/SettingsPage.jsx` | Currency dropdown (13 ISO-4217 codes) |
| `package.json`, `package-lock.json` | `papaparse` dependency |

## Verification

### Local build

- `go build ./...` in `project_admin/backend` — clean (status 0)
- `go vet ./...` — clean
- `npm run build` in `project_admin/frontend` — clean, papaparse bundled

### Manual smoke tests to run

1. **Phase 0 regression**: existing JSON import through `/import` still works
   and the refactored polling loop advances the progress bar correctly.
2. **Encryption fail-closed**: unset `ADMIN_ENCRYPTION_KEY`, confirm the
   Integrations area is gracefully disabled (sidebar link present, but no
   CSV suggest-mapping and no Shopify install).
3. **CSV path**: upload a 50-row CSV, verify AI mapping arrives with
   confidence > 0.7 on obvious columns, confirm, import runs, the new
   products show `source_system='csv'` and `source_id` = filename-prefixed
   row index; re-upload the same file → unique index absorbs duplicates.
4. **Shopify OAuth**: with a dev-store and valid `SHOPIFY_API_KEY/SECRET`,
   the install link reaches Shopify, the callback lands on
   `/integrations?connected=shopify&id=…`, and `admin.tenant_integrations`
   has a row with a sealed credential (starts with `v1.`).
5. **Shopify webhook HMAC**: curl the webhook with a known-good signature
   → 200; with a tampered body → 401 and nothing written.
6. **Enrichment autotrigger**: after a JSON import, an `enrichment_jobs`
   row appears within seconds; `enrichment_version` flips 0 → 2 on the
   imported products.

## Known gaps / caveats

- **CSV mapping cost model is flagged for rework** (user feedback this
  session): per-upload Haiku is wasteful. The post-MVP design is a one-shot
  tenant schema-discovery agent that reads the DB, builds a metadata
  snapshot similar to B7 `catalog.field_definitions`, and writes a mapping
  dictionary used for deterministic lookup on subsequent uploads. Captured
  as a feedback memory; don't tear this out until that design lands.
- **Master-catalog normalization is out of scope**. Bands stay raw strings,
  names only get `cleanText`, ingredients only INCI-slug. Source-tracking
  dedup gives idempotent re-imports from the same external system but does
  not merge across sources.
- **Shopify variants**: first-variant-wins. Multi-variant support is a
  separate epic with a schema migration on `catalog.products`.
- **Shopify `inventory_levels/update`** is logged-and-skipped — the payload
  has no product_id and mapping it back requires a second API round-trip.
  The 6-hour full resync (`StartPeriodicResync`) is the backup path.
- **`StartPeriodicResync` is a no-op placeholder**: needs
  `IntegrationsPort.ListByKindAndStatus` before it can iterate. Webhooks
  cover the live path so this isn't blocking.
- **Metafields** are pulled per-product on initial sync and on webhook
  updates. GraphQL bulk-query for large catalogs is post-MVP.
- **Google Sheets** source card is "Coming soon" only — no backend route.
- **B7 field_definitions seed for new tenants**: when a fresh Shopify
  merchant signs up in a non-cosmetics vertical, the cosmetics seed still
  runs. Needs the per-tenant schema-discovery agent above.
- **UI copy is English-only** per user preference (see memory
  `feedback_user_facing_text_english_only.md`).
