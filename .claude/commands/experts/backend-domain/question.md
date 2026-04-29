# backend-domain Question

Answer questions about V4 domain entities (project_v4/backend/internal/domain/) without making code changes.

## Variables

USER_PROMPT: $ARGUMENTS
EXPERTISE: .claude/commands/experts/backend-domain/expertise.yaml
CODE_ROOT: project_v4/backend/internal/domain/

## Instructions

- IMPORTANT: If USER_PROMPT not provided, STOP and request a question.
- Read EXPERTISE first — it has the entity map (UI primitives, chat, catalog, pipeline, tracing) and the gotchas.
- For exact field shapes/JSON tags, read the actual `<entity>_entity.go` file — domain types drift via small commits.
- DO NOT make code changes.

## Workflow

### Step 1 — Load expertise
The YAML covers: atom/widget/layout/formation primitives, chat (message/session/user/event), catalog (product/service/master_*/category/tenant/digest), pipeline (tool/state), tracing (trace/span), errors. It also lists gotchas for kopecks vs rubles, two coexisting Preset shapes, FormationWithData location, etc.

### Step 2 — Decide whether to read code
Read the actual file when the question is about:
- Exact field names / JSON tags / pointer-vs-value → `<entity>_entity.go`
- Constants for an enum (TriggerType, AtomType, ViewMode, etc.) → corresponding entity file
- Helper methods (LLMUsage.CalculateCost, DeltaInfo.ToDelta, SpanCollector.Start) → corresponding file
- Test expectations → `<entity>_test.go` (state_entity_test.go, catalog_digest_test.go)

### Step 3 — Answer
- Direct answer first.
- File paths as `project_v4/backend/internal/domain/<entity>_entity.go:<line>`.
- Cross-link to engine-v4 when answering about FormationWithData/Atom/Widget/LayoutNode/ActionDef.
- Cross-link to backend-pipeline when answering about ToolDefinition/LLMUsage/CatalogDigest.

## Constraints

- DO NOT change code or create files.
- DO NOT cite `project/backend/internal/domain/` — that's V1/V2 legacy and the entity set there differs (no LayoutNode, no master_service, no preset_v2, etc.).
- DO flag drift between YAML and code so the next self-improve run can fix it.

## Output

Direct answer with file references. Note any drift you spotted.
