# admin Self-Improve

Update the admin expertise from current code in `project_admin/`.

## Variables

USE_DIFF: $ARGUMENTS (true | false, default: false)
EXPERTISE: .claude/commands/experts/admin/expertise.yaml
CODE_ROOT: project_admin/
LINE_LIMIT: 600

## Instructions

- Scan code, update YAML, keep under LINE_LIMIT.
- Preserve overview (scope split with catalog expert, hexagonal pattern, deploy story) unless code actually changed them.
- Refresh anything tied to specific names (auth methods, route paths, env vars, adapter list, hot files).

## Workflow

### Step 1 — Read current expertise
Note which sections already exist so you only update, not duplicate.

### Step 2 — Choose scope
- USE_DIFF=true → run `git diff HEAD~10 -- project_admin/ --name-only` and focus on changed files. Filter out catalog-owned files (handler_categories, handler_products, catalog_adapter, harvester_lite, discovery_*, merge_apply, enrichment, junk_detector, integrations, csv_mapping, shopify/, crawler/, units/) — those belong to catalog expert.
- USE_DIFF=false → scan systematically: usecases/auth*.go → handlers/handler_auth*.go → cmd/server/main.go → frontend/src/features/.

### Step 3 — Refresh facts (high-drift surfaces)

**Auth use cases** — `ls project_admin/backend/internal/usecases/auth*.go`. Update `auth.methods` if new auth method added (e.g. WebAuthn, SAML). Each new method needs handler + adapter cross-reference.

**Auth handlers** — `ls project_admin/backend/internal/handlers/handler_auth*.go`. Update `routes_overview.auth` (public + protected lists). New handler file = new routes; trace through `cmd/server/main.go` to find registration.

**Middleware list** — `ls project_admin/backend/internal/handlers/middleware_*.go`. Update `middleware.files`. New middleware (e.g. rate limiting) needs entry + brief description.

**Adapter directories** — `ls project_admin/backend/internal/adapters/`. Update `architecture.backend_structure.adapters` if new adapter (e.g. stripe/, sendgrid/, pagerduty/).

**Frontend features** — `ls project_admin/frontend/src/features/`. Update `frontend_features` if new feature dir added. Existing dirs may have grown — check page count.

**Env vars** — `grep -nE "os\.Getenv|firstNonEmpty.*os\.Getenv" project_admin/backend/`. Update `env_vars`. New env var without YAML entry = silent config drift.

**Routes in `cmd/server/main.go`** — re-read `mux.Handle` / `mux.HandleFunc` registrations. Update `routes_overview` arrays. Especially watch for new internal-key endpoints (privilege escalation surface).

**Canvas methods** — re-read `internal/usecases/canvas.go`. Update `canvas.methods.{presets,components}` if methods added/removed. Canvas grows as design system matures.

### Step 4 — Cross-layer drift checks

These are silent-failure surfaces:

- **Pre-2FA token route allow-list** — middleware_auth_pre2fa restricts pre-2FA tokens to specific routes. Run `grep -n "pre2fa\|Pre2FA" project_admin/backend/internal/handlers/`. New 2FA-related route must explicitly opt in.
- **Internal-key middleware coverage** — `grep -n "middleware_internal_key\|InternalKey" project_admin/backend/cmd/server/main.go`. Every internal endpoint MUST register through it. New internal route without it = exposed.
- **OAuth state TTL** — `internal/adapters/postgres/oauth_login_states_repo.go`. State rows need TTL or pruning; verify still in place.
- **Magic link code hashing** — `internal/adapters/postgres/challenges_repo.go`. Codes hashed at rest. New challenge type (e.g. SMS) must follow same pattern.
- **Resend vs SMTP adapter** — verify resend is the active path. SMTP adapter remains but is for local dev only after dab0629. New mail flows go via Resend.
- **WIDGET_BASE_URL prefix bug** — gotcha says env value may lack `https://`. Check current Railway env value (read-only) and embed-code generator handles it gracefully.

### Step 5 — Refresh gotchas

Add a new gotcha when:
- A non-obvious auth/billing/canvas behavior was discovered.
- A recent commit fixed a security or UX bug whose root cause was a hidden invariant.

Inspiration: `git log --since='4 weeks' -- project_admin/backend/internal/usecases/auth* project_admin/backend/internal/usecases/canvas.go project_admin/backend/internal/usecases/billing.go`.

### Step 6 — Refresh hot files

Re-run:
```
find project_admin/backend project_admin/frontend/src -name "*.go" -o -name "*.jsx" -o -name "*.js" ! -name "*_test.go" -exec wc -l {} \; | sort -nr | head -10
```
Update `active_workstreams.hot_files`. Note which files belong to catalog expert (don't pretend admin owns them).

### Step 7 — Cross-reference live trackers

Read recent dev logs in `docs/Updates/main-*.md` and `docs/Updates/feature-admin-*.md` for last 4 weeks. Pull recent_milestones from there.

### Step 8 — Report

```
admin expertise updated

Changes:
- Added: <items>
- Updated: <items>
- Removed: <items>

Lines: N / 600
```

## Constraints

- DO NOT exceed LINE_LIMIT.
- DO NOT include catalog domain content here — it's a separate expert. Cite catalog as related_expert when crossing boundaries.
- DO NOT cite legacy `project/backend/` — deleted 2026-04-29.
- DO update auth-method coverage when new auth method added — auth is the largest admin sub-domain and most security-sensitive.
- DO call out internal-key endpoints (cross-domain to curator) — these are auth boundary risks.

## Output

Updated `expertise.yaml` plus a summary report.
