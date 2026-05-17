# Catalog v2 — Test Specification

Companion to `/Users/starknight/.claude/plans/structured-discovering-lollipop.md`
addendum "Catalog v2 hardening" (2026-05-15).

This document lists every test that must pass before catalog v2 ships to a
real Shopify merchant. Three layers:

1. **Automated tests** (Go) — run on every CI pass.
2. **Manual end-to-end scenarios** — Vlad runs once before go-live.
3. **Go-live readiness checklist** — one-page sign-off.

---

## Part 1 — Automated tests

All test files live under
`project_admin/backend/internal/usecases/`. Run with
`go test ./...` from the admin backend directory.

| File | What it covers | Status |
|---|---|---|
| `match_key_test.go` | `NormalizeMatchKey` — lowercase, trim, non-alnum strip, multi-space collapse, unicode (cyrillic), empty inputs, emoji, hyphen handling. Includes a `TestNormalizeMatchKey_MatchesSQLBackfill` regression that pins the Go output to three real SQL-backfilled keys from `catalog.master_products`. | **green** |
| `classify_vertical_test.go` | `ClassifyVertical` cascade: alias hit, alias miss → rule hit, both miss → "unknown", alias priority over rule, error on lookup falls through to rules. Plus `TestMatchesRule` for the When DSL (`product_type contains`, `brand =`, `name contains`, `tag =`, unknown syntax, unquoted literal, double quotes). | **green** |
| `apply_v2_transforms_test.go` | Transform engine: lowercase, trim, split:, ml_from_string, g_from_string, bool_from_yesno, int, numeric. Idempotency on `[]string` input. Unknown transform pass-through. `getPath` regression (top-level, dotted, `variants[0].sku` array indexing, missing keys). Plus `asString` / `asInt` / `asStringSlice` coercion tables. | **green** |
| `apply_v2_test.go` | `ApplyForTenant` end-to-end against in-memory fakes (InboxPort + MappingArtifactV2Port + CatalogV2WriterPort + TenantActionLogPort). Covers per-row vertical classification, branch selection (exact vertical, fallback to "unknown"), tier3 fallback for `cosmetics.*` rules emitted in a non-cosmetics branch, rejection of (no-sku + no-name) rows, synthetic SKU only used when name is present, GTIN digit-strip, listing fields, alias-beats-rule precedence, action-log status, mark-applied. Master immutability on bind is also asserted here (pre-seeded master + new tenant row with same SKU). | **green** |
| `discovery_v2_artifact_compat_test.go` | v2 artifact lifts to v3 with one branch via `domain.LiftV2ToV3`. v3 JSON round-trips through Marshal/Unmarshal. `MappingArtifactV3.FindBranch` falls back to "unknown" branch when exact vertical match misses. `PeekArtifactVersion` dispatch on raw bytes. | **green** |
| `update_orchestrator_test.go` | `OnWebhook` rate-limit window (24h gap absorbed_only vs. beyond window → applied). First-event-no-prior path. `ManualSync` bypasses rate limit. `OnMappingMiss` rejects when discovery is nil-wired. Validation rejects empty tenant/external_id. | **green** |

`apply_v2_match_test.go` was folded into `apply_v2_test.go` (2026-05-17). The
match cascade lives in `catalog_v2_writer_adapter.go` (the postgres adapter),
not in the usecase layer, so its real SQL behaviour requires a live DB and
is covered by the `smoke_catalog_v2` runner. The unit suite verifies what
the usecase ASKS of the writer: deterministic SKU/GTIN/normalized_match_key
fill-in, master_immutable on bind, listing always upserted.

### Test data fixtures

`internal/usecases/testdata/` already holds JSON inbox samples used by the
existing smoke test. The new automated suites reuse the same library used by
`cmd/smoke_catalog_v2/main.go` (cosmeticsLibrary, electronicsLibrary,
furnitureLibrary, skiLibrary) — extract into a shared package if duplication
becomes painful.

### Coverage target

Every public function in:
- `match_key.go`
- `classify_vertical.go`
- `apply_v2.go`
- `apply_v2_transforms.go`
- `domain/mapping_artifact_v3.go`

Goal: 70% statement coverage on the above files. The adapter
(`catalog_v2_writer_adapter.go`) is exercised by the smoke test (real DB),
not by unit tests.

---

## Part 2 — Manual end-to-end scenarios

Each scenario lists: **setup**, **expected DB state** with concrete SQL
queries Vlad can paste into psql, **pass criteria**. Vlad runs through this
list once before go-live.

### Scenario 1 — Fresh cosmetics tenant

**Setup**: run smoke with tenant A only (50-product variant) OR connect a
real Shopify dev-store with cosmetics catalog. Wait for OAuth + bulk ingest
+ initial apply.

**SQL**:
```sql
-- 1.1 — discovery produced a one-branch artifact.
SELECT jsonb_array_length(mapping_artifact -> 'branches') AS branches,
       (mapping_artifact -> 'branches' -> 0 ->> 'vertical') AS vertical
FROM catalog.tenant_catalog_schema
WHERE tenant_id = (SELECT id FROM catalog.tenants WHERE slug = 'smoke-cosm-a-...');
-- expected: branches=1 vertical=cosmetics

-- 1.2 — all 10 products applied without misses.
SELECT action, status, payload
FROM admin.tenant_action_log
WHERE tenant_id = (SELECT id FROM catalog.tenants WHERE slug = 'smoke-cosm-a-...')
ORDER BY occurred_at;
-- expected: inbox_pull/ok, discovery_start/ok, discovery_done/ok, apply/ok with applied=10 misses=0

-- 1.3 — every master has a master_cosmetics row.
SELECT COUNT(*) FROM catalog.master_products mp
LEFT JOIN catalog.master_cosmetics mc ON mc.master_product_id = mp.id
WHERE mp.owner_tenant_id = (SELECT id FROM catalog.tenants WHERE slug = 'smoke-cosm-a-...')
  AND mc.master_product_id IS NULL;
-- expected: 0
```

**Pass criteria**:
- 1 branch, vertical=cosmetics
- 0 mapping_miss
- master_products count = inbox count
- master_cosmetics 1:1 with master_products
- catalog.products count = inbox count (one listing per inbox item)

---

### Scenario 2 — 70% overlap tenant

**Setup**: after scenario 1, run smoke tenant B (with overlap rules
matching the `cosm-b` builder in `smoke_catalog_v2/main.go`).

**SQL**:
```sql
-- 2.1 — match cascade outcomes.
WITH tids AS (
  SELECT
    (SELECT id FROM catalog.tenants WHERE slug = 'smoke-cosm-a-...') AS a,
    (SELECT id FROM catalog.tenants WHERE slug = 'smoke-cosm-b-...') AS b
)
SELECT
  (SELECT COUNT(DISTINCT p.master_product_id) FROM catalog.products p, tids
   WHERE p.tenant_id = tids.b AND p.master_product_id IN
     (SELECT id FROM catalog.master_products mp, tids WHERE mp.owner_tenant_id = tids.a)) AS bound_to_a,
  (SELECT COUNT(DISTINCT p.master_product_id) FROM catalog.products p, tids
   WHERE p.tenant_id = tids.b AND p.master_product_id IN
     (SELECT id FROM catalog.master_products mp, tids WHERE mp.owner_tenant_id = tids.b)) AS bnew;
-- expected: bound_to_a=7, bnew=3

-- 2.2 — master rows for overlapping products carry A's owner (untouched on bind).
SELECT mp.sku, mp.owner_tenant_id = (SELECT id FROM catalog.tenants WHERE slug = 'smoke-cosm-a-...') AS owned_by_a
FROM catalog.master_products mp
WHERE mp.sku IN ('ORD-HA-30','ORD-NIA-30','ORD-SQ-50');
-- expected: all 3 owned_by_a=true (B did NOT take ownership on bind)

-- 2.3 — B's master.name for overlapping products equals A's master.name (not B's variant).
SELECT mp.sku, mp.name FROM catalog.master_products mp WHERE mp.sku IN ('ORD-HA-30','ORD-NIA-30','ORD-SQ-50');
-- expected: names match A's library (apply_v2 didn't overwrite the master on bind)
```

**Pass criteria**:
- 7 binds to A across the 3 cascade stages (SKU=3, GTIN=2, key=2)
- 3 new B-owned masters
- master.name unchanged on bind

---

### Scenario 3 — Electronics-only tenant

**Setup**: smoke tenant C (or real Shopify with laptop catalog).

**SQL**:
```sql
-- 3.1 — artifact has electronics branch, no cosmetics.
SELECT (mapping_artifact -> 'branches' -> 0 ->> 'vertical') AS vertical
FROM catalog.tenant_catalog_schema
WHERE tenant_id = (SELECT id FROM catalog.tenants WHERE slug = 'smoke-electro-...');
-- expected: electronics

-- 3.2 — all masters routed to tier3 (no master_cosmetics row).
SELECT COUNT(*) FROM catalog.master_products mp
WHERE mp.owner_tenant_id = (SELECT id FROM catalog.tenants WHERE slug = 'smoke-electro-...')
  AND mp.vertical = 'electronics'
  AND mp.tier3 <> '{}'::jsonb;
-- expected: 10

SELECT COUNT(*) FROM catalog.master_cosmetics mc
JOIN catalog.master_products mp ON mp.id = mc.master_product_id
WHERE mp.owner_tenant_id = (SELECT id FROM catalog.tenants WHERE slug = 'smoke-electro-...');
-- expected: 0

-- 3.3 — tier3 contains the per-vertical attributes (cpu, ram_gb, storage_gb, screen_size).
SELECT mp.sku, jsonb_object_keys(mp.tier3) AS k
FROM catalog.master_products mp
WHERE mp.owner_tenant_id = (SELECT id FROM catalog.tenants WHERE slug = 'smoke-electro-...')
LIMIT 5;
-- expected: keys include 'cpu', 'ram_gb', 'storage_gb', 'screen_size'
```

**Pass criteria**:
- 10 masters vertical=electronics
- 0 master_cosmetics rows
- tier3 populated with cpu/ram/storage/screen on every master

---

### Scenario 4 — Multi-vertical mix

**Setup**: smoke tenant D (cosmetics + electronics + furniture + ski).

**SQL**:
```sql
-- 4.1 — artifact has 4 branches.
SELECT jsonb_array_elements(mapping_artifact -> 'branches') ->> 'vertical'
FROM catalog.tenant_catalog_schema
WHERE tenant_id = (SELECT id FROM catalog.tenants WHERE slug = 'smoke-mixed-...');
-- expected: 4 rows: cosmetics, electronics, furniture, ski

-- 4.2 — per-row vertical classification routed each product correctly.
SELECT mp.vertical, COUNT(*) FROM catalog.master_products mp
JOIN catalog.products p ON p.master_product_id = mp.id
WHERE p.tenant_id = (SELECT id FROM catalog.tenants WHERE slug = 'smoke-mixed-...')
GROUP BY mp.vertical;
-- expected: cosmetics=3 (all bound to A), electronics=3 (all bound to C),
--           furniture=2 (new), ski=2 (new)

-- 4.3 — owned-vs-bound breakdown matches expected.
SELECT mp.owner_tenant_id = (SELECT id FROM catalog.tenants WHERE slug = 'smoke-mixed-...') AS owned_here,
       COUNT(DISTINCT mp.id)
FROM catalog.master_products mp
JOIN catalog.products p ON p.master_product_id = mp.id
WHERE p.tenant_id = (SELECT id FROM catalog.tenants WHERE slug = 'smoke-mixed-...')
GROUP BY 1;
-- expected: owned_here=true → 4, owned_here=false → 6
```

**Pass criteria**:
- 4 branches in artifact
- Vertical distribution matches expectation
- 4 owned + 6 bound

---

### Scenario 5 — Webhook product update (real Shopify dev-store)

**Setup**: a real Shopify dev-store installed via OAuth, initial apply done.

**Steps**:
1. In Shopify admin, edit any product (change price or add a metafield).
2. Watch the admin backend logs for `webhook_received`.
3. Inside the 24h rate-limit window: log should show `absorbed_only`.
4. Click "Sync now" in curator. Verify apply runs immediately.

**SQL**:
```sql
SELECT action, status, payload FROM admin.tenant_action_log
WHERE tenant_id = '<your-tenant-id>'
ORDER BY occurred_at DESC LIMIT 5;
-- expected: webhook_received entries with payload.verb='updated';
--           manual_sync entry on Sync now click followed by apply/ok
```

**Pass criteria**:
- Webhook payload reaches inbox
- Within 24h: not auto-applied
- Manual sync forces apply
- Listing's `price` / `stock_quantity` reflects new Shopify values

---

### Scenario 6 — Webhook product delete

**Setup**: same tenant as scenario 5.

**Steps**:
1. In Shopify admin, delete a product.
2. Watch logs for `products/delete` webhook.

**SQL**:
```sql
SELECT id, name, deleted_at, source_id FROM catalog.products
WHERE source_id = 'gid://shopify/Product/<deleted-id>';
-- expected: one row, deleted_at IS NOT NULL

SELECT mp.id, mp.sku FROM catalog.master_products mp
WHERE mp.id = (SELECT master_product_id FROM catalog.products WHERE source_id = 'gid://shopify/Product/<deleted-id>');
-- expected: master row still present and untouched
```

**Pass criteria**:
- `catalog.products.deleted_at` set
- `master_products` row unchanged
- V5 chat search returns no results for that product

---

### Scenario 7 — Mapping miss → discovery cascade

**Setup**: real Shopify tenant with existing artifact.

**Steps**:
1. Add a brand-new metafield to one product (e.g. `metafields.skincare.spf_value=30`).
2. Save in Shopify; webhook fires.
3. Watch admin logs.

**SQL**:
```sql
SELECT action, status, payload FROM admin.tenant_action_log
WHERE tenant_id = '<id>' ORDER BY occurred_at DESC LIMIT 10;
-- expected: webhook_received → apply (status=warning, misses=1)
--                            → discovery_start (trigger=mapping_miss)
--                            → discovery_done
--                            → next apply succeeds

SELECT trigger, status, cost_usd FROM admin.agent_runs
WHERE tenant_id = '<id>' ORDER BY started_at DESC LIMIT 3;
-- expected: top row trigger=mapping_miss status=success
```

**Pass criteria**:
- Mapping miss recorded in action log
- Narrow discovery_v2 run triggered with `trigger='mapping_miss'`
- Subsequent apply succeeds (no more miss on that field)

---

### Scenario 8 — Unknown vertical fallback

**Setup**: real Shopify tenant with `product_type='Cookware'` (not in vertical_aliases).

**Steps**:
1. Create a product with `product_type='Cookware'` in Shopify.
2. Trigger sync.

**SQL**:
```sql
SELECT mp.sku, mp.vertical, mp.tier3 FROM catalog.master_products mp
WHERE mp.id = (SELECT master_product_id FROM catalog.products
               WHERE source_id = 'gid://shopify/Product/<cookware-id>');
-- expected: vertical IS NULL or 'unknown'; tier3 populated with whatever attributes the artifact mapped

SELECT action, status, payload FROM admin.tenant_action_log
WHERE tenant_id = '<id>' AND action IN ('apply', 'mapping_miss')
ORDER BY occurred_at DESC LIMIT 5;
-- expected: 'apply' entry succeeded (forgiving fallback absorbs the unknown vertical) OR
--           'mapping_miss' followed by a discovery_v2 run with trigger=unknown_vertical
```

**Pass criteria**:
- Product applied successfully
- Vertical recorded as `unknown` (or the new vertical the agent added)
- V5 search still finds the product

---

### Scenario 9 — Cost & latency budget

**Setup**: after running scenarios 1-4 with smoke.

**Numbers** (from a clean-DB smoke run on 2026-05-15 22:24):

| Tenant | Items | Discovery time | Discovery cost | Apply time | Apply cost |
|---|---|---|---|---|---|
| A (cosmetics) | 10 | 36.7s | $0.0477 | ~17s | $0 |
| B (cosmetics) | 10 | 32.4s | $0.0414 | ~11s | $0 |
| C (electronics) | 10 | 28.6s | $0.0369 | ~15s | $0 |
| D (multi-vertical) | 10 | 42.8s | $0.0588 | ~10s | $0 |
| **Total** | **40** | **140s** | **$0.185** | **~53s** | **$0** |

**Pass criteria**:
- Per-tenant discovery_v2 cost ≤ $1 (×20 headroom)
- Per-tenant discovery wall-clock ≤ 5 min (×4 headroom)
- Full smoke (4 tenants) finishes in ≤ 10 min and costs ≤ $1

---

## Part 3 — Go-live readiness checklist

Print and tick.

- [x] **Unit tests green** (2026-05-17): `go test ./...` in `project_admin/backend/` returns ok. Catalog v2 layer is now covered by 5 test files (match_key, classify_vertical, apply_v2_transforms, apply_v2, discovery_v2_artifact_compat, update_orchestrator). Auth layer adds 5 more (auth, auth_sessions, auth_google, auth_2fa, auth_invitations) on top of the pre-existing auth_magic_link suite.
- [ ] **Smoke clean**: `go run ./cmd/smoke_catalog_v2` produces:
  - Distinct masters across 4 tenants: **27**
  - Total active listings: **40**
  - Distinct master_cosmetics: **13**
  - Distinct masters with tier3: **17**
  - Vertical distribution: cosmetics=13, electronics=10, furniture=2, ski=2
  - Per-tenant ownership: A=10/0, B=3/7, C=10/0, D=4/6
  - Total cost ≤ $0.50
- [ ] **Build clean**: `go build ./...` in both admin and curator backends.
- [ ] **DB migration applied to prod Neon**: `catalog.vertical_aliases` has 57+ rows, `master_products.normalized_match_key` populated for hey-babes (`SELECT COUNT(*) FROM catalog.master_products WHERE normalized_match_key IS NOT NULL`).
- [ ] **Real Shopify dev-store install** completes scenarios 5-8.
- [ ] **Curator UI** renders the 4 smoke tenants without 500s, shows their masters with linked listings (multi-tenant masters visible in MasterDetailPage's listings tab — when curator UI lands tomorrow).
- [ ] **Action log readable** for all 4 smoke tenants — no rows with `status='error'` other than expected mapping_miss warnings.
- [ ] **No mapping_miss > 5%** of apply totals across the smoke run.

---

## Known limitations to revisit post-launch

These are not blockers for go-live but should land in the next milestone.
Each one is "deferred — requires Vlad" because it needs a prod-migration,
an architecture decision, or a UI design that's out of scope today.

- **`listing.tenant_overrides`** JSONB column reserved (`catalog_migrations.go:837`)
  but no writer wired. Per-tenant marketing customization (custom tags, custom
  images, video URLs) currently goes to either tier3 (which we drop on bind)
  or is lost.
- **DROP COLUMN on 16 legacy PIM columns on `master_products`** (skin_type,
  concern, key_ingredients, …) — already migrated to `master_cosmetics`, but
  old columns still on the table. Destructive operation; needs Vlad +
  Neon point-in-time backup before running.
- **DROP `master_variants` table** — V5 chat still LEFT JOINs it (4 unused
  hey-babes rows). Sequence: remove JOIN in `project_v5/backend/internal/
  adapters/postgres/postgres_catalog{,_vector}.go`, deploy V5, then DROP.
- **Embedding seed for new masters** — `apply_v2` does not write
  `catalog.master_products.embedding`, so V5 vector search returns 0 hits
  for fresh masters until `cmd/rebuild-embeddings` is run. Long-term fix is
  to wire `EmbeddingPort` into `ApplyV2UseCase` — needs architectural
  decision on where the embedding client lives (admin backend vs cross-call
  to V5).
- **Curator UI for master edit / merge / vertical reassignment /
  vertical_aliases editor** — needed for ongoing operator work, not for
  initial install flow.
- **CSV / Google Sheets / manual ingest paths** — explicitly deferred per
  Shopify-only scope cut.
- **`catalog.shopify_raw_imports` table** — the CREATE block was removed
  from `catalog_migrations.go` on 2026-05-17 (no live writer references the
  table anymore). The prod row still exists; DROP TABLE pending the same
  prod-migration window as the legacy column drops.
- **Telegram legacy widget fallback** — `auth_telegram.go:174-197` plus the
  handler at `handler_auth_oauth.go:125-160` are dead in the new SignInPage
  flow (frontend only offers OIDC). Remove next time we touch
  `auth_telegram.go`.
