# admin Question

Answer questions about the admin backend + frontend (auth, billing, canvas,
settings, conversations, traces, widget UI, embed code) without making code
changes.

## Variables

USER_PROMPT: $ARGUMENTS
EXPERTISE: .claude/commands/experts/admin/expertise.yaml
CODE_ROOT: project_admin/

## Instructions

- IMPORTANT: If USER_PROMPT not provided, STOP and request a question.
- Read EXPERTISE first for the mental model (auth methods, middleware, routes, billing, canvas, env vars).
- Verify any specific claim against current code before answering — the YAML can drift.
- DO NOT make any code changes.

## Workflow

### Step 1 — Load expertise
Read `expertise.yaml`. It tells you:
- Scope split with catalog expert (catalog/import/integrations/junk/enrichment owned by `catalog`)
- Auth methods: email/password, 2FA, Google OAuth, magic link, password reset, telegram, invitations, multi-tenant
- Middleware (auth, pre-2FA, api-key, internal-key, cors, logging)
- Routes overview (auth public/protected, billing, canvas, settings, traces, widget-config)
- Billing (read-only display; payment processor not wired)
- Canvas (KeepstarCanvas presets/components/design-tokens; DRAFT vs PUBLISHED)
- Env vars and known gotchas (Resend over SMTP, WIDGET_BASE_URL prefix)

### Step 2 — Decide if a code read is needed

You MUST read code (not rely on YAML alone) when the question is about:
- A specific auth flow → read `internal/usecases/auth_*.go`
- A handler routing or middleware order → read `cmd/server/main.go`
- 2FA verification details → read `internal/usecases/auth_2fa.go` and `internal/adapters/totp/`
- Magic link timing/race conditions → read `internal/usecases/auth_magic_link.go`
- Canvas DRAFT/PUBLISHED mechanics → read `internal/usecases/canvas.go`
- Why an internal endpoint isn't accepting → read `internal/handlers/middleware_internal_key.go`
- Embed code generator output → read `frontend/src/features/widget/WidgetPage.jsx`

### Step 3 — Answer

- Direct answer first.
- File paths with `project_admin/<file>:<line>` where applicable.
- For "why does login fail with X" / "why is magic link not arriving" — walk: usecase → adapter (resend/smtp/google/telegram) → DB row → email port.
- Mention the related expert (`catalog` for any catalog/import/integrations question, `engine-v4` for canvas → engine consumption, `pipeline-agents` for traces UI consuming pipeline_traces, `curator` for internal-curator endpoints).

## Constraints

- DO NOT change code or create files.
- DO NOT cite legacy `project/backend/` — deleted 2026-04-29.
- DO route catalog/import/integrations/junk/enrichment questions to `experts:catalog:question` — those live in project_admin but belong to the catalog domain.
- DO route Formation/render questions to `experts:engine-v4:question` or `experts:widget:question` — admin canvas produces data engine consumes; engine renders.
- DO route MergeApply use case semantics to `experts:catalog:question` — admin only proxies internal-curator endpoints; the merge logic is catalog domain.

## Output

Direct answer with file references. Note any drift you spot between expertise.yaml and current code (so the next self-improve run can fix it).
