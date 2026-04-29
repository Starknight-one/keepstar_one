---
branch: feature/engine-v4
date: 2026-04-10 01:43 UTC
commits:
  - 2fa746e — fix(v4): read p.extra in VectorSearch for metadata-driven binding
  - 255585d — fix(v4): normalize session_id to lowercase + persist tenant_id
parent: 1321f19 — fix(v4): English-only test-electronics seed with real categories
plan: continuation of `docs/New features/METADATA_DRIVEN_BINDING_2026-04-09.md` Step C verification + admin UI fix surfaced during PoC verification
---

# Metadata-driven binding PoC verification + session aggregation fix

Two fixes in one session, both surfaced while verifying the
test-electronics tenant on prod:

1. **VectorSearch `p.extra` gap** — `Product.Extra` was nil for any
   product returned via embedding search, so atoms whose `fieldName`
   targets an extra-JSONB key (model/manufacturer/cover_image for
   test-electronics) had no value to bind to. This was the visible
   "data isn't applying on formations" symptom that ended the previous
   5-compaction session.
2. **Session aggregation case bug** — admin sessions list showed
   $0/0 tokens for some sessions. Root cause: `pipeline_traces.session_id`
   is TEXT (case-sensitive), `chat_sessions.id` is UUID (postgres
   stores lowercase), and a client sending an uppercase sessionId broke
   the JOIN `WHERE pt.session_id = cs.id::text`. As a bonus, all
   sessions had `tenant_id = ''` because pipeline_execute never set it.

## Context — fix 1 (p.extra in VectorSearch)

Sessions A (`7e15cd9` conditional tree_map + `<fields>` block), B
(`9261327` fieldName override + FIELD BINDING rule), and C (`5566439`
test-electronics seed + Product.Extra wiring) had landed all the engine
plumbing. Test-electronics seed populated `products.extra` with tenant
fields (model, manufacturer, cpu, ram, display_size, battery_life,
cover_image). `tools/tool_visual_assembly.go:ProductToMap` already
spreads `p.Extra` into the data map. Engine `BindData` already resolves
`entityData[a.FieldName]`. But on prod the rendered widgets had nil
values for `model`/`manufacturer`/`cover_image`.

`ListProducts` (`postgres_catalog.go:158`/`349`) reads
`COALESCE(p.extra, '{}'::jsonb)` correctly. `GetProduct` does too. But
`VectorSearch` (the embedding-search path used by Agent1's
catalog_search tool) was selecting 28 product columns and silently
omitting `p.extra`. So any product returned via vector search had
`p.Extra == nil` and the engine had nothing to bind.

For test-electronics specifically: query="laptops" hits the keyword
filter `ILIKE '%laptops%'` which misses "...Laptop" product names (the
trailing 's'), so keyword count = 0 and the engine falls back to vector
search → all 8 products come from VectorSearch → all 8 have `Extra=nil`
→ all atoms with electronics-specific fieldNames stayed unbound.

## Context — fix 2 (session aggregation)

While verifying the metadata fix on prod, the user noticed the admin
sessions list showed $0.0000 / 0 tokens / 0 requests for the sessions
my Python test scripts had created. Initial hypothesis: pipeline doesn't
write to `chat_messages`. Wrong — admin doesn't read `chat_messages`
for aggregations. It reads from `pipeline_traces` which IS written.

Direct DB query revealed the real bug:

```
session_id                           | cost_usd
71963687-7C25-492C-AA7D-932D3199630A | 0.0055443  ← UPPERCASE
A3A9A13B-CAEA-4C9C-B0E8-AB0D18E96636 | 0.013931
699FE43B-55E9-4EF4-B9A4-A57A3CD96974 | 0.0130145
561b472a-9893-4d25-998d-6a13064217ec | 0.0054798  ← lowercase
ba9f8bc8-d1ec-4fd6-b19c-07f745311aef | 0.003248
```

`pipeline_traces.session_id` is `TEXT` (case-preserving). `chat_sessions.id`
is `UUID` (postgres normalizes to lowercase on insert). The admin
aggregation in `project_admin/.../postgres_trace.go:184` joins via
`WHERE pt.session_id = cs.id::text` — `cs.id::text` returns lowercase,
so any uppercase row in pipeline_traces silently aggregates to nothing.

Real frontend uses `crypto.randomUUID()` which is always lowercase, so
real users were unaffected. My Python test scripts (uppercased UUIDs)
hit the bug. But the case bug is a real defect — anyone scripting
against the API with a uppercase UUID would silently break the admin
view of their session.

Separately: `pipeline_execute.go:114` creates the chat_sessions row
with `session.TenantID` unset, so the admin's `tenant_id` column is
blank for all V4 sessions.

## Approach — fix 1 (p.extra)

Mirror the `ListProducts` pattern in `VectorSearch`
(`postgres_catalog.go:574-718`):

1. Add `COALESCE(p.extra, '{}'::jsonb) as extra` to the SELECT.
2. Add `extraJSON []byte` to the scan vars and append `&extraJSON` to
   the `rows.Scan(...)` call (right after `&mpBenefits`).
3. After the existing image/tags unmarshal block, add:
   ```go
   if len(extraJSON) > 0 {
       _ = json.Unmarshal(extraJSON, &p.Extra)
   }
   ```

`VectorSearchServices` (`postgres_catalog.go:1336`) was NOT changed —
`catalog.services` has no `extra` column and there's no test-services
tenant, so it's out of scope for the PoC.

## Approach — fix 2 (session aggregation)

1. **`handler_pipeline.go:91`** — `sessionID := strings.ToLower(req.SessionID)`
   instead of `sessionID := req.SessionID`. Normalize at the entry point.
2. **`postgres_trace.go:Record`** — defense-in-depth: lowercase
   `trace.SessionID` before insert. Future code paths that bypass the
   pipeline handler still get a clean DB.
3. **`pipeline_execute.go:114`** — set `TenantID: req.TenantSlug` when
   creating the session row.
4. **`postgres_cache.go:SaveSession`** — add `tenant_id =
   COALESCE(NULLIF(EXCLUDED.tenant_id, ''), chat_sessions.tenant_id)`
   to the ON CONFLICT SET clause. So a later SaveSession call without
   tenant (e.g. from action/navigation handlers) doesn't blank a
   previously-set value, AND the first save with tenant updates an
   existing row created without it.
5. **One-time backfill** for the 4 existing uppercase rows:
   ```sql
   UPDATE pipeline_traces SET session_id = LOWER(session_id)
   WHERE session_id ~ '[A-Z]';
   ```
   Skipped tenant_id backfill — would require inferring tenant from
   trace_data per row, not worth the complexity for cosmetic data.

## Files changed

| Commit  | File | Change |
|---------|------|--------|
| `2fa746e` | `project_v4/backend/internal/adapters/postgres/postgres_catalog.go` | `VectorSearch` reads `p.extra`, unmarshals into `p.Extra` |
| `255585d` | `project_v4/backend/internal/handlers/handler_pipeline.go` | `strings.ToLower(req.SessionID)` at entry |
| `255585d` | `project_v4/backend/internal/usecases/pipeline_execute.go` | Set `TenantID: req.TenantSlug` on session creation |
| `255585d` | `project_v4/backend/internal/adapters/postgres/postgres_cache.go` | COALESCE tenant_id in ON CONFLICT SET |
| `255585d` | `project_v4/backend/internal/adapters/postgres/postgres_trace.go` | Defense-in-depth lowercase in `Record` |

## Verification

### Metadata-driven binding (test-electronics PoC)

Local:
- `go build ./...` clean
- `go test ./...` green (handlers/usecases/postgres/engine_v4/tools)

Prod (after `2fa746e` deploy):
- `POST /api/v1/pipeline {tenant: test-electronics, query: "laptops"}`
  → 8 widgets, all 40 atoms bound:
  - `cover_image` URLs resolved (e.g. apple.com images)
  - `model` strings ("MacBook Pro 14"...)
  - `manufacturer` strings ("Apple", "Lenovo", "Dell"...)
  - `price`, `rating` correct
- Trace `agent2.toolInput.ops` contains override ops
  (`name→model`, `brand→manufacturer`, `images→cover_image`)
- Hey-babes regression: `POST /api/v1/pipeline {tenant: hey-babes-cosmetics,
  query: "крема"}` → 23 widgets, 105/115 atoms bound (10 unbound rating
  = nullable products), Agent2 emits ZERO override ops (LLM correctly
  recognized preset defaults match the tenant fields).

### Session aggregation fix

Local:
- `go build ./...` clean
- `go test ./...` green

Prod (after `255585d` deploy):
- Test pipeline call with intentionally uppercase sessionId:
  ```
  curl -X POST .../pipeline -d '{"sessionId":"DEADBEEF-CAFE-BABE-FACE-FEED12345678",...}'
  ```
  Response echoes `sessionId: deadbeef-cafe-babe-face-feed12345678` ✓
- Direct DB query confirms:
  ```
  id                                  | tenant_id           | cost   | traces | tokens
  deadbeef-cafe-babe-face-feed12345678 | hey-babes-cosmetics | 0.0139 | 1      | 4640
  ```
  Both fixes working: case normalized, tenant set.
- Backfill verified: `count(*) FILTER (WHERE session_id ~ '[A-Z]')` from
  pipeline_traces returns 0 (was 4 before).
- Previously-empty sessions from screenshot now show:
  - `71963687-...` 2 traces, $0.018, 7266 tokens
  - `a3a9a13b-...` 1 trace, $0.014, 4633 tokens
  - `699fe43b-...` 1 trace, $0.013, 3714 tokens

What to check on admin UI after this lands:
- Sessions list shows non-zero COST/TOKENS/REQUESTS for the 4 backfilled
  sessions
- New sessions (from real frontend or tests) show populated `tenant_id`
- Old sessions still have empty tenant_id (cosmetic — won't be backfilled)

## Known gaps / caveats

- **Tenant_id backfill skipped** — old sessions still show empty tenant.
  Could backfill from `trace_data->'agent1'->'toolBreakdown'->>'tenant'`
  but it's cosmetic and risky without testing every case. New sessions
  going forward will be correct.
- **Other handlers don't normalize sessionId** — `handler_navigation.go`,
  `handler_action.go`, `handler_chat.go` still take sessionId as-is. In
  practice these are called with sessionIds returned from a previous
  pipeline call (already lowercase from the new normalization), so the
  bug doesn't manifest. But if a client manually calls navigation with
  an uppercase sessionId, the trace adapter's defense-in-depth catches
  it. Not a complete fix at every entry point but functionally covered.
- **`last_activity_at` not bumped on subsequent pipeline calls** — the
  pipeline only calls `SaveSession` on first `ErrSessionNotFound`,
  never on subsequent turns. So sessions are sorted by `started_at`
  not real activity. Out of scope, but worth a future fix for the
  admin UI's "most active sessions" view.
- **VectorSearchServices still missing extra** — services table has no
  `extra` column today, so it doesn't matter. If a future tenant adds
  service-side custom fields, this needs the same fix.
- **Hey-babes regression atom binding rate** — 105/115 not 115/115. The
  10 unbound atoms are `rating` slots on products where the source
  data has `rating: null`. Engine correctly skips nil values; this is
  expected, not a regression. (Pre-existing behavior, validated against
  baseline traces from before metadata-binding work.)
- **PoC scope** — test-electronics is one tenant with 8 products. For a
  production claim of "domain-shift works", we need test-books and
  test-furniture seeds too (mentioned in `METADATA_DRIVEN_BINDING_2026-04-09.md`
  Step 5). That's the next session. PoC closes 4.3 B7 on the trackerm,
  but the multi-tenant proof is still owed.
