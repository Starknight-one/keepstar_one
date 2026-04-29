# curator Self-Improve

Update the curator expertise from current code in `curator/`.

## Variables

USE_DIFF: $ARGUMENTS (true | false, default: false)
EXPERTISE: .claude/commands/experts/curator/expertise.yaml
CODE_ROOT: curator/
LINE_LIMIT: 500

## Instructions

- Scan code, update YAML, keep under LINE_LIMIT.
- Preserve overview (architecture pattern, deploy story, merge-proxy rationale) unless code actually changed them.
- Refresh anything tied to specific names/paths (route list, adapter methods, domain types, env vars).

## Workflow

### Step 1 — Read current expertise
Note which sections already exist so you only update, not duplicate.

### Step 2 — Choose scope
- USE_DIFF=true → run `git diff HEAD~10 -- curator/ project_admin/backend/internal/handlers/handler_curator_merge.go --name-only` and focus on changed files.
- USE_DIFF=false → scan all files in CODE_ROOT.

### Step 3 — Refresh facts (high-drift surfaces)

**HTTP routes in `cmd/server/main.go`** — re-read the mux setup. Specifically the protected route table starting at the `protected.HandleFunc(...)` lines and the `/curator/tenants/{id}/...` path-suffix dispatcher. Update `http_routes.protected` arrays.

**Adapter methods in `internal/adapters/postgres.go`** — `grep -n "^func (c \*Client)" curator/backend/internal/adapters/postgres.go`. Update `adapter_methods.groups`. Watch for new methods slipping into the wrong group (e.g. a new junk method appearing in master_catalog block — usually means file growth signal, see hot_files).

**Domain types in `internal/domain/types.go`** — `grep -n "^type" curator/backend/internal/domain/types.go`. Update `domain_types.types`. New type usually means a new endpoint — cross-check against `http_routes`.

**Frontend pages** — `ls curator/frontend/src/pages/`. Update `architecture.frontend_structure.pages`. New page = new route = needs corresponding backend endpoint somewhere in `http_routes`.

**Env vars** — `grep -nE "os\.Getenv|firstNonEmpty.*os\.Getenv" curator/backend/`. Update `env_vars` lists. New env var without entry in this YAML = silent config drift in prod.

**MergeProxy endpoints** — re-read `internal/handlers/handler_merge.go`. Verify the `target := p.adminBase + "/admin/api/internal/curator/..."` path templates still match. If admin-backend renames an internal route, MergeProxy silently 404s.

### Step 4 — Cross-layer drift checks

These are the silent-failure surfaces:

- **adapters.Client.PromoteAttribute → SQL identifier safety**: confirm `validIdent()` is still gated on every code path that builds a column name from candidate input. New promote variant without the gate = SQL injection vector.
- **Curator user role enum** — domain.CuratorUser.Role is a free string in the YAML; if the codebase introduces a `Role` constant set, document the exact values.
- **Cookie name "curator_session"** is hardcoded in `session/middleware.go` AND `handlers/handlers.go` (Logout). Renaming requires both — flag if you find one without the other.
- **CURATOR_INTERNAL_KEY** must match the same env var on admin-backend. If admin's `InternalKeyMiddleware` reads a different name, MergeProxy stops working with no readable error.

### Step 5 — Refresh gotchas

Add a new gotcha when:
- A non-obvious behavior surprised you while answering a question.
- A recent commit fixed a bug (e.g. cookie SameSite, MergeProxy timeout, vertical whitelist).

Inspiration: `git log --since='4 weeks' -- curator/`.

### Step 6 — Refresh hot files

Re-run: `find curator/backend curator/frontend/src -type f \( -name "*.go" -o -name "*.jsx" -o -name "*.js" \) ! -name "*_test.go" -exec wc -l {} \; | sort -nr | head -10`. Update `active_workstreams.hot_files` if any file crosses 500 LOC.

### Step 7 — Refresh deploy info

Verify Railway service still on branch `main`, rootDir `/curator`, port 8082. If anyone migrated to a feature branch, update `deploy.branch`.

### Step 8 — Report

```
curator expertise updated

Changes:
- Added: <items>
- Updated: <items>
- Removed: <items>

Lines: N / 500
```

## Constraints

- DO NOT exceed LINE_LIMIT.
- DO NOT mix in admin-backend canonical catalog domain types — curator's domain is read-shape only. Cite admin types only in `related_experts.admin`.
- DO NOT cite `project/backend/` — legacy, deleted 2026-04-29.
- DO update SQL identifier risks if PromoteAttribute or a new mutating endpoint is added.

## Output

Updated `expertise.yaml` plus a summary report.
