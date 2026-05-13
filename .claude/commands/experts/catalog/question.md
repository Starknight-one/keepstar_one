# catalog Question

Answer questions about the cross-cutting catalog domain (admin write side +
V5 read side + curator browse) without making code changes.

## Variables

USER_PROMPT: $ARGUMENTS
EXPERTISE: .claude/commands/experts/catalog/expertise.yaml
CODE_ROOT: project_admin/backend/ AND project_v5/backend/internal/adapters/postgres/postgres_catalog.go

## Instructions

- IMPORTANT: If USER_PROMPT not provided, STOP and request a question.
- Read EXPERTISE first for the mental model (schema, write pipeline, V5 read methods, Shopify, hot files).
- Verify any specific claim against current code before answering — the YAML can drift.
- DO NOT make any code changes.

## Workflow

### Step 1 — Load expertise
Read `expertise.yaml`. It tells you:
- Two-sided architecture (admin write + V5 read, same Postgres)
- Schema and core tables (catalog.tenants/categories/master_products/master_variants/products + candidates + tenant_catalog_schema)
- Write-side pipeline (ingest → harvest → discovery → validate → merge_apply → curator approval)
- V5 read methods (ListProducts, VectorSearch, GetMasterProduct, GenerateCatalogDigest)
- Shopify integration (OAuth, bulk, webhooks)
- Usecase inventory and HTTP routes

### Step 2 — Decide if a code read is needed

You MUST read code (not rely on YAML alone) when the question is about:
- A specific SQL query / WHERE clause → grep the method in the relevant adapter
- merge_apply matching cascade → read `usecases/merge_apply.go buildProposal`
- discovery agent prompt or budget → read `usecases/discovery_agent.go`
- A Shopify endpoint behavior → read `adapters/shopify/client.go` for the method
- tier2 transform behavior → read `usecases/merge_apply.go applyTransform / extractTier2`
- V5 read tier2 fallback → read `project_v5/.../postgres_catalog.go GetMasterProduct / ListProducts / VectorSearch`
- Schema column / index → read `internal/adapters/postgres/catalog_migrations.go`

### Step 3 — Answer

- Direct answer first.
- File paths with line numbers — be EXPLICIT about which side: `project_admin/...` (write) vs `project_v5/.../postgres_catalog.go` (read).
- For "why is this listing not appearing in chat" / "why does merge skip this product" — walk: ingest → harvest → discovery artifact → match cascade → V5 read filter.
- Mention the related expert (`pipeline-agents` for chat reads, `engine-v5` for atom binding, `admin` for handler/auth, `curator` for promote/junk) when crossing layers.

## Constraints

- DO NOT change code or create files.
- DO NOT cite the legacy `project/backend/internal/adapters/postgres/postgres_catalog.go` — deleted 2026-04-29.
- DO route auth/sessions/billing questions to `experts:admin:question` — those aren't catalog.
- DO route chat-pipeline / Agent prompt questions to `experts:pipeline-agents:question` — catalog feeds them but isn't them.
- DO route curator UI / promotion questions to `experts:curator:question`.
- DO NOT introduce render concerns (Formation, atoms) — that's `engine-v5` / `widget`.

## Output

Direct answer with file references. Note any drift you spot between expertise.yaml and current code (so the next self-improve run can fix it).
