# curator Question

Answer questions about the standalone curator service without making code changes.

## Variables

USER_PROMPT: $ARGUMENTS
EXPERTISE: .claude/commands/experts/curator/expertise.yaml
CODE_ROOT: curator/

## Instructions

- IMPORTANT: If USER_PROMPT not provided, STOP and request a question.
- Read EXPERTISE first for the mental model (architecture, routes, session, merge proxy, adapter methods, deploy).
- Verify any specific claim against current code in CODE_ROOT before answering — the YAML can drift.
- DO NOT make any code changes.

## Workflow

### Step 1 — Load expertise
Read `expertise.yaml`. It tells you:
- Hexagonal-lite architecture (single adapter, single handlers package)
- Public vs protected route table
- Session middleware (Bearer + curator_session cookie)
- MergeProxy reverse-proxy to admin-backend (when, why, auth)
- Adapter method groups (auth, candidates, junk, audit, tenants, master_catalog)
- Frontend pages and routing
- Env vars and deploy story

### Step 2 — Decide if a code read is needed
You MUST read code (not rely on YAML alone) when the question is about:
- A specific SQL query → read `internal/adapters/postgres.go` (use grep for the method name)
- A merge-proxy path mismatch → read `internal/handlers/handler_merge.go` AND cross-check admin-backend's `handler_curator_merge.go`
- A route 404 or auth bypass → read `cmd/server/main.go` mux registration
- Session token TTL or cookie attrs → read `internal/session/middleware.go`
- A frontend page issue → read `frontend/src/pages/<Name>.jsx` and `frontend/src/api.js`

### Step 3 — Answer
- Direct answer first.
- File paths with `curator/<file>:<line>` where applicable.
- Mention the related expert (admin, catalog) if the answer crosses to admin-backend ownership.
- For "why does this proxy fail" / "merge run hangs" questions, walk: curator-frontend → curator-backend session → MergeProxy → admin-backend internal endpoint → response back.

## Constraints

- DO NOT change code or create files.
- DO NOT cite the legacy `project/backend/` code — deleted 2026-04-29.
- DO route merge logic questions to `admin` expert when they're really about `MergeApplyUseCase` semantics — curator only owns the proxy, not the use case.
- DO NOT introduce canonical catalog domain types in curator answers — curator's domain is read-shape only.

## Output

Direct answer with file references. Note any drift you spot between expertise.yaml and current code (so the next self-improve run can fix it).
