# Chunk 4 — first product card preset, replicate fan-out, image binding

**Branch**: v5
**Plan author**: Claude (Opus 4.7)
**Reference**: `docs/v5-engine-plan.md`, `docs/v5-known-gaps.md`, chunk 1-3 logs in `docs/Updates/v5/`

---

## Context

After chunks 1-3 we have a v9 scene-graph engine, sectional state with delta-stream, and a binding layer. What's missing is **proof the architecture pays off**: until we ship one preset end-to-end with replication and image binding, we cannot measure whether v9 micropresets actually beat V4 on tokens.

Chunk 4 closes that gap with the first real preset (`product_card`), the engine fan-out for per-record replication, image-fill binding, and the preset storage adapter that the future v9-canvas microservice will eventually write to.

User-confirmed constraints driving this chunk:
1. **No hardcoded preset builders in Go.** Preset is `engine.Document` JSON, lives in DB, seeded for `heybabes` for now.
2. **`v5_presets`/`v5_preset_versions` is the FINAL shape**, not a stopgap. The current `admin.tenant_presets` in `project_admin/` is being torn out; the eventual canvas is a separate v9 microservice that will read/write V5's preset tables. We design schema for that future, not for compat with the doomed admin tables.
3. **Tenants in the future canvas mostly rearrange field bindings**, so the seeded preset must already be group-decomposed (5 named Frames) so movement is meaningful.
4. **Constraints / actions / `<fields>` prompt block / SavePreset workflow are out of scope** — chunks 5/7 and the canvas-microservice chunk.

---

## Approach

### 1. Preset storage — two tables, V5's own Neon schema

Mirrors the *shape* the future v9-canvas microservice needs: header + immutable versions, three-state lifecycle, integer versioning. Single fresh DDL like chunk 2.

```sql
CREATE TABLE v5_presets (
  id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id          UUID NOT NULL REFERENCES catalog.tenants(id) ON DELETE CASCADE,
  name               VARCHAR(200) NOT NULL,
  category           VARCHAR(100) NOT NULL DEFAULT 'product',
  entity_type        VARCHAR(100) NOT NULL DEFAULT 'product',
  description        TEXT NOT NULL DEFAULT '',
  default_replicate  BOOLEAN NOT NULL DEFAULT TRUE,
  latest_version_id  UUID,
  created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(tenant_id, name)
);
CREATE INDEX idx_v5_presets_tenant ON v5_presets(tenant_id);

CREATE TABLE v5_preset_versions (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  preset_id       UUID NOT NULL REFERENCES v5_presets(id) ON DELETE CASCADE,
  version         INTEGER NOT NULL,
  status          VARCHAR(20) NOT NULL DEFAULT 'draft'
                  CHECK(status IN ('draft','published','archived')),
  doc_json        JSONB NOT NULL,                        -- engine.Document JSON
  author_user_id  UUID,
  published_at    TIMESTAMPTZ,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(preset_id, version)
);
CREATE INDEX idx_v5_preset_versions_preset ON v5_preset_versions(preset_id);
CREATE INDEX idx_v5_preset_versions_status ON v5_preset_versions(preset_id, status);
```

"Latest published" via `ORDER BY version DESC WHERE status='published' LIMIT 1`. Migration runner from chunk 2 picks this up automatically.

### 2. Domain + Port

- `internal/domain/preset.go` — `Preset` struct (id, tenant_id, name, category, entity_type, description, default_replicate, version, status, document `*engine.Document`, published_at). `PresetStatus` enum constants.
- `internal/ports/preset_port.go` — minimum read surface for chunk 4:
  ```go
  type PresetPort interface {
      GetPublishedPreset(ctx, tenantSlug, name) (*domain.Preset, error)
      ListPublishedPresets(ctx, tenantSlug) ([]domain.Preset, error)
  }
  ```
  Save/Publish/Fork/Archive added when the canvas-microservice chunk lands. Following the convention from `CatalogPort` (chunk 3) — read subset first, write side later.

### 3. Postgres adapter

- `internal/adapters/postgres/postgres_preset.go` — `PresetAdapter` with the two read methods.
- `GetPublishedPreset` accepts tenant slug or UUID like `CatalogAdapter.GetTenantBySlug` does. Joins `v5_presets` × `v5_preset_versions` × `catalog.tenants`, status='published', highest version.
- `doc_json` unmarshalled into `engine.Document` (round-trip already validated in chunk 1's `TestDocumentJSONRoundTrip`).
- Integration test against live Neon — same pattern as chunk 3 (`postgres_catalog_integration_test.go`): seed `v5_presets/_versions` rows, load, assert Document round-trips.

### 4. Seed product_card preset

JSON file at `internal/engine/presets/seed/product_card.json` — `engine.Document` with:

```
root Frame "card" {replicate:true}
  ├── Frame "hero"     → Image atom {fieldBinding:"heroImage"}
  ├── Frame "info" (column, gap=xs)
  │     ├── Text   {fieldBinding:"name", fontSize:xl, bold}
  │     ├── Frame "meta" (row)
  │     │     ├── Number {fieldBinding:"price",  format:"currency"}
  │     │     └── Number {fieldBinding:"rating", format:"stars-compact"}
  │     └── Text   {fieldBinding:"brand", wrapper:"badge"}
  ├── Frame "actions"  (empty placeholder; chunk 7 wires defaults)
  └── Frame "specs"    (empty placeholder; deferred)
```

5 named groups so canvas-microservice users can move/hide groups. `replicate:true` lives on the `card` Frame, not the document root, so the rest of the doc (theme/variables) survives fan-out untouched. Field names match `ProductToMap` output (`binding_to_map.go:59-62` already produces `heroImage` + `images`).

Seeding happens in the integration test — no production data is touched. Stage:
1. Resolve heybabes tenant_id.
2. Insert v5_presets row + v5_preset_versions row with `doc_json` = file contents, status='published'.
3. Test loads it back, runs ExpandReplicates → BindData → asserts.

### 5. Replicate fan-out — pre-bind step (NOT a Command)

New `internal/engine/replicate.go`:

```go
func ExpandReplicates(doc *Document, count int)
```

Plan-agent feedback: take **count**, not the data slice — cleaner seam, fan-out doesn't need data. Reasoning for *not* a Command: replication is a render-time projection of preset+data, parallel to `ComponentResolver.Expand`. Both are projections, neither needs undo, neither belongs in `CommandHistory` (`engine/command.go:6-9` exists for reversible user-driven mutations).

Algorithm:
- Walk `doc.Children` recursively, find every node with `replicate:true`.
- For each match:
  1. Locate parent + index (we have `FindParent` in `scene_graph.go`).
  2. Deep-clone via `cloneNode` (`component_resolver.go:204`) — count times.
  3. **Mint fresh IDs on every cloned node, root + descendants** (re-walk with `WalkNodes` and re-id). Closes plan-agent finding #1: without this, the post-replicate `ComponentResolver.expandRef` (`component_resolver.go:98-100` forces `clone["id"] = NodeID(refNode)`) would collide all N expanded clones onto the same id.
  4. Stamp `dataIndex: i` on each clone root only (not descendants — see fix below).
  5. Replace the source node in parent's children with the N clones.
- Order in pipeline: `ApplyOps → ExpandReplicates → ComponentResolver.ExpandAll → BindData`.

### 6. Fix `dataIndexOf` to walk up the tree

Plan-agent finding #2: `binding.go:111-126`'s `dataIndexOf` reads only from the node itself, defaulting to 0. Means nested atoms inside a replicated clone would all bind to `data[0]`. We don't want to stamp `dataIndex` on every descendant during clone (bloats JSON, bug-prone) — instead the binding walker carries the nearest-ancestor `dataIndex` down.

Smallest viable fix: make `BindData` pass an inherited `dataIndex` through recursion (parameter to its internal walker), checking the current node first then falling back to inherited. Nodes carrying their own `dataIndex` override; nodes without inherit. Existing tests for chunk-3 binding stay green (no replicate → no parent has `dataIndex` → behaviour identical to today).

### 7. Image binding

- `internal/engine/node_types.go` — add `NodeTypeImage = "image"` constant.
- `internal/engine/binding.go` `bindTargetForNode` (line 101-106) — branch: if `NodeType(n) == NodeTypeImage`, write to `fills` instead of `value`. Uses helper `setImageFill(n, url)` which:
  - If `n["fills"]` exists and is non-empty array, mutates `fills[0].image`.
  - Else writes `n["fills"] = []any{ map[string]any{"type":"image","image":url,"mode":"fill"} }`.
  - Image-fill payload shape (`{type,image,mode}`) is **flagged as known-gap**: `value_fill.go:1-62` only declares discriminators, no body schema. Plan-agent's recommended shape matches v9 convention (discriminator's same-named payload key) and is consistent with `FillTypeColor → color:{...}`. Confirm against v9 TS source before chunk 4 lands; if v9 uses `url` or `src` instead of `image`, change one constant.
- Centralise the image-key as `attrFillImageURL` constant in `binding.go` so it's not stringly scattered.

### 8. Tests

Unit:
- `engine/replicate_test.go`:
  - no replicate marker → no-op
  - one marker on a Frame + count=3 → 3 clones with `dataIndex` 0/1/2 + unique IDs throughout
  - nested replicate (replicate inside replicate) → outer wins (skip inner)
  - count=0 → marker removed, parent is empty
- `engine/binding_test.go` (extend):
  - image atom + `heroImage` data → `fills[0].image` set, `fills[0].type == "image"`
  - image atom with existing fills → `fills[0].image` updated, no append
  - image atom + missing data field → no fills mutation, `__bound` not set (Грабля #1 still honoured)
  - dataIndex inheritance: replicated Frame with text descendants binds each clone's atoms to its own `data[i]`
  - non-replicated tree: `dataIndex` semantics unchanged from chunk 3

Integration (live Neon, behind `//go:build integration` like chunk 3):
- `postgres_preset_integration_test.go` — seed product_card preset row → `GetPublishedPreset(heybabes, "product_card")` → assert Document round-trips, has `replicate:true` on card Frame, atoms have `fieldBinding`s.
- `engine_pipeline_integration_test.go` (new) — load product_card → fetch 3 heybabes products via `CatalogPort.ListProducts` → `ProductToMap` × 3 → `ExpandReplicates(doc, 3)` → `BindData` → assert each clone has unique heroImage URL, name, price; `__bound` present on hits, missing on misses.

### 9. Files

| File | Status |
|---|---|
| `project_v5/backend/internal/domain/preset.go` | added |
| `project_v5/backend/internal/domain/preset_test.go` | added |
| `project_v5/backend/internal/ports/preset_port.go` | added |
| `project_v5/backend/internal/adapters/postgres/preset_migrations.go` | added |
| `project_v5/backend/internal/adapters/postgres/postgres_preset.go` | added |
| `project_v5/backend/internal/adapters/postgres/postgres_preset_integration_test.go` | added |
| `project_v5/backend/internal/adapters/postgres/state_migrations.go` | modified — register new DDL |
| `project_v5/backend/internal/engine/node_types.go` | modified — add `NodeTypeImage` |
| `project_v5/backend/internal/engine/binding.go` | modified — image-fill branch + dataIndex inheritance + `attrFillImageURL` const |
| `project_v5/backend/internal/engine/binding_test.go` | modified — image cases + dataIndex inheritance cases |
| `project_v5/backend/internal/engine/replicate.go` | added |
| `project_v5/backend/internal/engine/replicate_test.go` | added |
| `project_v5/backend/internal/engine/presets/seed/product_card.json` | added |
| `project_v5/backend/internal/engine/engine_pipeline_integration_test.go` | added |
| `docs/Updates/v5/plans/chunk-4-first-preset.md` | added — frozen plan copy |
| `docs/v5-known-gaps.md` | modified — see "Known gaps to register" below |

---

## Critical files & helpers to reuse

- `engine/component_resolver.go:204 cloneNode` / `:248 cloneAny` — deep-clone for replicate fan-out
- `engine/scene_graph.go FindParent`, `WalkNodes` — find where to splice clones in
- `engine/id_generator.go GenerateID` — fresh IDs per clone
- `engine/binding_to_map.go:59-62` — `heroImage` already produced
- `adapters/postgres/postgres_catalog.go GetTenantBySlug` — slug-or-UUID resolution pattern
- `adapters/postgres/state_migrations.go` — single-DDL migration pattern

## Known gaps to register in `docs/v5-known-gaps.md`

| Gap | Reason | Closes |
|---|---|---|
| Image-fill body shape `{type:"image", image:"<url>", mode:"fill"}` is V5's guess; not yet confirmed against v9 TS source | `value_fill.go:1-62` only declares discriminators, leaves bodies as untyped `any` | Verify when first frontend renderer port lands (or sooner via grep on v9 source) |
| `GroupID` for cross-widget constraint scoping not ported (V4 had `rg-{counter}` shared by replicated clones) | Chunk 4 defers constraints entirely | Constraints chunk (TBD, post chunk 5) |
| `replicate:true` only honoured at one level deep — nested replicate-in-replicate skipped | Avoids combinatorial blow-up; not a real preset shape today | Re-evaluate if/when a real preset needs nested replication |
| Current `admin.tenant_presets` / `tenant_preset_versions` / `tenant_components` / `tenant_design_tokens` schemas in `project_admin/` are flagged for deletion (not migration) | User confirmed: existing canvas-in-admin is being torn out, replaced by a separate v9 microservice that will read/write V5's `v5_presets` tables | Canvas-microservice chunk (Stream B, post chunk 5) |

## Verification

```sh
cd project_v5/backend
go build ./...                     # OK
go test ./...                      # unit tests pass (engine + domain + usecases + postgres unit)
go vet ./...                       # OK

# Live Neon:
TEST_DATABASE_URL=$DATABASE_URL go test -tags=integration -v ./internal/adapters/postgres/...
# 9/9 expected: + 2 new preset adapter tests on top of chunk 3's 7

TEST_DATABASE_URL=$DATABASE_URL go test -tags=integration -v ./internal/engine/...
# end-to-end: load product_card from DB, replicate × 3 heybabes products, bind, assert
```

What to watch in next chunks:
- Chunk 5 will write the `<fields>` prompt block consumer; if heroImage / dataIndex inheritance has a corner case, it will surface there first
- The image-fill shape known-gap should be closed before any frontend renderer port; until then, V5 is internally consistent but unverified against v9 wire format
