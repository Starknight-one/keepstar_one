# feature/engine-v4 — KeepstarCanvas Phase 1 (backend CRUD)

- **Branch**: `feature/engine-v4`
- **Date (UTC)**: 2026-04-12 01:48 (MSK) / 2026-04-11 22:48 UTC
- **Parent commit**: `7caeb8f` (docs(updates): metadata binding PoC verified + session aggregation fix)
- **Commit sha**: `5401b89`

## Context

Kicking off the KeepstarCanvas initiative (survivable plan: `docs/New features/KEEPSTAR_CANVAS_PLAN.md`, canonical: `.claude/plans/mutable-launching-fiddle.md`). Phase 1 spec: stand up the storage + CRUD backend for tenant-scoped presets, components, design tokens, and the per-tenant design-context version counter that will bust V4 agent2 prompt cache in Phase 3.

The end-goal is a Pencil-clone editor in admin where tenants design preset tiles and publish them; the V4 chat backend picks up published ops on the next turn. This session lays the DB + CRUD foundation — no frontend, no V4 wiring yet.

## Approach

All work lives in `project_admin/backend/` (admin service, port 8081). V4 chat backend is untouched — that's Phase 2.

1. **Schema**: added 6 tables to `admin.*` with idempotent `CREATE TABLE IF NOT EXISTS`, wired into `RunAdminMigrations`. Preset/component rows are **headers** that point at an immutable version row; ops JSON lives only on version rows, so published versions stay byte-frozen for reproducibility of past chat sessions.
   - `admin.tenant_presets` + `admin.tenant_preset_versions`
   - `admin.tenant_components` + `admin.tenant_component_versions`
   - `admin.tenant_design_tokens`
   - `admin.tenant_design_context_version` (one row per tenant, bumped on every publish — this is the cache-bust key Phase 3 will read)
2. **Draft/publish lifecycle**: `SaveDraftOps` locks the preset row `FOR UPDATE`, then either overwrites the current version (if already draft) or forks a new version at `max(version)+1`. Published rows are never mutated; the next edit forks a fresh draft. `ForkPreset` provides an explicit "edit published" path for the UI.
3. **Design-context version bump on every publish/delete/token upsert**, via `BumpDesignContextVersion` in the usecase layer. Phase 3 will key the V4 agent2 prompt cache on this counter.
4. **Port/adapter split**: `ports.CanvasPort` is the repository boundary the usecase talks to; `postgres.CanvasAdapter` implements it. Compile-time interface assertion in the adapter guards against drift.
5. **Validation in usecase**: preset names are `[a-z0-9_-]{2,200}`, ops payloads must parse as a JSON array, category/name required for tokens.
6. **HTTP routes** (all behind `authMW`, tenant from JWT):
   - `GET/POST /admin/api/canvas/presets`
   - `GET/PUT/DELETE /admin/api/canvas/presets/{id}`
   - `POST /admin/api/canvas/presets/{id}/publish`
   - `POST /admin/api/canvas/presets/{id}/fork`
   - Mirrors for `/canvas/components` + `/canvas/components/{id}` + `/publish`
   - `GET/POST /admin/api/canvas/tokens`, `DELETE /admin/api/canvas/tokens/{id}`

## Files changed

| File | Kind | Notes |
|---|---|---|
| `project_admin/backend/internal/adapters/postgres/admin_migrations.go` | modified | +6 tables, 6 indexes, all idempotent |
| `project_admin/backend/internal/domain/canvas.go` | new | TenantPreset/PresetVersion/TenantComponent/ComponentVersion/DesignToken + input payload types |
| `project_admin/backend/internal/ports/canvas_port.go` | new | `CanvasPort` interface |
| `project_admin/backend/internal/adapters/postgres/canvas_adapter.go` | new | ~720 LOC; full CRUD + draft/publish/fork in transactions + interface assertion |
| `project_admin/backend/internal/usecases/canvas.go` | new | validation + design-context version bumping |
| `project_admin/backend/internal/handlers/handler_canvas.go` | new | all routes, error mapping to HTTP codes |
| `project_admin/backend/cmd/server/main.go` | modified | wire `NewCanvasAdapter` → `NewCanvasUseCase` → `NewCanvasHandler`, register 6 routes behind `authMW` |

## Verification

**Compile**: `cd project_admin/backend && go build ./... && go vet ./...` — both clean.

**Manual end-to-end** (to be run by user — DB connection needed):

```bash
# 1. Start admin backend
scripts/start_admin.sh

# 2. Log in and capture JWT for a tenant
TOKEN=$(curl -s -X POST http://localhost:8081/admin/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"owner@heybabescosmetics.com","password":"..."}' | jq -r .token)

# 3. List presets (empty)
curl -s -H "Authorization: Bearer $TOKEN" http://localhost:8081/admin/api/canvas/presets | jq .

# 4. Create a draft preset
curl -s -X POST -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"name":"my_product_card","category":"product","entityType":"product","description":"first test","ops":[]}' \
  http://localhost:8081/admin/api/canvas/presets | jq .

# 5. Save draft ops (PUT)
PRESET_ID=... # from step 4
curl -s -X PUT -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"ops":[{"op":"I","target":"w","node":{"type":"text","fieldName":"name"}}]}' \
  "http://localhost:8081/admin/api/canvas/presets/$PRESET_ID" | jq .

# 6. Publish
curl -s -X POST -H "Authorization: Bearer $TOKEN" \
  "http://localhost:8081/admin/api/canvas/presets/$PRESET_ID/publish" | jq .

# 7. Edit after publish → confirm a new DRAFT version is forked
curl -s -X PUT -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"ops":[{"op":"I","target":"w","node":{"type":"text","fieldName":"price"}}]}' \
  "http://localhost:8081/admin/api/canvas/presets/$PRESET_ID" | jq '.latestVersion.status, .latestVersion.version'
# → "draft", 2

# 8. Tenant isolation: log in as a different tenant, list, confirm empty.
```

**DB-level sanity checks** (via psql):

```sql
-- schemas created
\dt admin.tenant_presets
\dt admin.tenant_preset_versions
\dt admin.tenant_components
\dt admin.tenant_component_versions
\dt admin.tenant_design_tokens
\dt admin.tenant_design_context_version

-- version counter bumps on publish
SELECT tenant_id, version FROM admin.tenant_design_context_version;
```

## Known gaps / caveats

- **Phase 1 only** — nothing from this commit touches the V4 chat backend or the admin frontend. Agent2 still serves only global-registry presets. Phase 2 adds the tenant-aware loader in `project_v4/backend`.
- **No seed data.** No helper script yet to copy global `product_card` ops into a per-tenant draft. Useful to add early in Phase 2 when we want to prove end-to-end.
- **Descriptions are free text, not generated.** Phase 3 will add the auto-gen pass when it's time to feed them to Agent2 as `<tenant_design_context>`.
- **No audit trail** beyond `author_user_id` on each version row. If we want "who published what, when" history it's already there in `published_at` + `created_at`; UI surfacing comes with Phase 6.
- **No op validation against V4 engine.** The usecase only checks that ops parse as a JSON array. Phase 6's "Publish" action will additionally run the ops through `ExpandInlinePresets` + `ApplyConstraints` headless to catch structural errors before flipping to published — deferred until that code path exists on the admin side (or we carve a shared lib).
- **`admin.tenant_design_context_version` has no default row** — the counter starts at 1 on first bump via `INSERT … ON CONFLICT`. Loader in Phase 3 must treat "missing row" as version 0.
- **Admin trace adapter is NOT tenant-filtered** — noted in the plan (Phase 7 blocker). Still leaks cross-tenant if exposed to Save-from-trace without the `chat_sessions` join. Nothing in this commit touches traces so we're still safe; the guardrail is logged for the next person.
- **No commit yet** — files are staged in the working tree, user asked to run this code through review before committing. Once committed, add the sha to this log.
