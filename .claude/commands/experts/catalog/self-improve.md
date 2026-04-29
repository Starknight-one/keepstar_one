# catalog Self-Improve

Update the catalog expertise from current code in `project_admin/backend/`,
`project_v4/backend/internal/adapters/postgres/postgres_catalog.go`, and
related files.

## Variables

USE_DIFF: $ARGUMENTS (true | false, default: false)
EXPERTISE: .claude/commands/experts/catalog/expertise.yaml
CODE_ROOT: project_admin/backend/ AND project_v4/backend/internal/adapters/postgres/postgres_catalog.go
LINE_LIMIT: 600

## Instructions

- Scan code, update YAML, keep under LINE_LIMIT.
- Preserve overview (cross-cutting nature, write+read split, pipeline stages) unless code actually changed them.
- Refresh anything tied to specific names (table columns, method names, route paths, artifact shape).
- This is the BIGGEST domain — drift surface is largest. Be patient.

## Workflow

### Step 1 — Read current expertise
Note which sections already exist so you only update, not duplicate.

### Step 2 — Choose scope
- USE_DIFF=true → run `git diff HEAD~10 -- project_admin/backend/internal/{adapters/postgres,adapters/shopify,usecases,handlers} project_v4/backend/internal/adapters/postgres/postgres_catalog.go --name-only` and focus on changed files.
- USE_DIFF=false → scan systematically: schema migrations → adapter methods → usecase signatures → handler routes.

### Step 3 — Refresh facts (high-drift surfaces)

**Schema in `internal/adapters/postgres/catalog_migrations.go`** — re-read the migration list. Update `schema.core_tables` if columns added/removed. New table = new section. New index on a hot path = note in `schema.indexes`.

**V4 read methods in `project_v4/.../postgres_catalog.go`** — `grep -nE "^func \(a \*CatalogAdapter\)\s+[A-Z]"`. Update `read_side_v4.read_methods`. New method without an entry = silent. Cross-check with V4 catalog handler routes.

**Admin adapter methods** — `grep -nE "^func \(c \*Client\)\s+[A-Z]" project_admin/backend/internal/adapters/postgres/catalog_adapter.go`. Update relevant sections. Most catalog-related methods live here.

**Usecase signatures** — for the 7 listed in `usecases_inventory`, re-grep their entry funcs:
```
grep -nE "^func \(uc \*[A-Z][a-z]+UseCase\)" project_admin/backend/internal/usecases/{harvester_lite,discovery_agent,merge_apply,validate_artifact,enrichment,match_cascade}.go
```

**Shopify client methods** — `grep -nE "^func \(c \*Client\)\s+[A-Z]" project_admin/backend/internal/adapters/shopify/client.go`. Update `shopify_integration` sections. New API endpoints (e.g. ProductUpdate) likely missing.

**Mapping Artifact shape** — re-read `domain.MappingArtifact` struct in `project_admin/backend/internal/domain/`. Update `write_side_pipeline.3_discovery.artifact_shape`. Adding a new field there changes what discovery agent must produce — a known LLM-prompt drift surface.

**HTTP routes** — `grep -nE "HandleFunc|Handle\(" project_admin/backend/cmd/server/main.go`. Update `http_routes` arrays. Especially watch internal-curator routes — name changes break MergeProxy silently.

### Step 4 — Cross-layer drift checks

These are silent-failure surfaces:

- **catalog.master_products columns** → V4 read methods → Agent2 prompt FIELDS block. New column without V4 read path = invisible to chat.
- **tier2 transforms in `extractTier2`** — `match` switch in merge_apply.go. New transform value (e.g. `"split_kebab"`) without a switch case silently does nothing.
- **MappingArtifact.MatchStrategyConfig** — thresholds drive `thresholdsFromArtifact()`. Field add → buildProposal must consume it.
- **field_definitions table** — feeds B7 role-based binding (engine-v4 future). Schema changes here cascade to engine-v4 expert.
- **catalog.master_products.tier2 JSONB** — feeding read-side requires updated GetMasterProduct/ListProducts/VectorSearch. Recent (2026-04-29 commit 8a3357d). Verify still in place.
- **Shopify HMAC verification** — webhook handler MUST call VerifyWebhookHMAC. Run `grep -n "VerifyWebhookHMAC" project_admin/backend/` to confirm every webhook entry uses it.
- **Curator's PromoteAttribute** — only path that runs ALTER TABLE on catalog.master_products. Confirm validIdent() gate is still in place.

### Step 5 — Refresh gotchas

Add a new gotcha when:
- A non-obvious ingest/match/merge edge case was discovered.
- A recent commit fixed a bug in pipeline (junk detection, dedup, schema drift).
- Inspiration: `git log --since='4 weeks' -- project_admin/backend/internal/usecases project_admin/backend/internal/adapters/postgres/catalog_adapter.go project_v4/backend/internal/adapters/postgres/postgres_catalog.go`.

### Step 6 — Refresh hot files

Re-run:
```
find project_admin/backend project_v4/backend/internal/adapters/postgres -name "*.go" ! -name "*_test.go" -exec wc -l {} \; | sort -nr | head -10
```
Update `active_workstreams.hot_files` if any file crosses 1000 LOC (signal of split candidate per experts_plan).

### Step 7 — Cross-reference live trackers

Read `docs/CATALOG_GAPS.md` (phase 1 live trackers). Update `active_workstreams.recent_milestones` from any newly-closed gaps. Read `docs/PRE_LAUNCH_TASKS.md` for any catalog-tagged tasks.

### Step 8 — Report

```
catalog expertise updated

Changes:
- Added: <items>
- Updated: <items>
- Removed: <items>

Lines: N / 600
```

## Constraints

- DO NOT exceed LINE_LIMIT (this is set higher — 600 — because catalog is cross-cutting).
- DO NOT cite the legacy `project/backend/internal/adapters/postgres/postgres_catalog.go` — deleted 2026-04-29.
- DO NOT mix in admin auth/billing — those belong to the `admin` expert.
- DO call out V4 read-side vs admin write-side explicitly when the question or answer references a method that exists on both sides with similar names but different responsibilities.

## Output

Updated `expertise.yaml` plus a summary report.
