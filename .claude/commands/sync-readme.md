# Sync README

Update all project README files to match current code state.

## Instructions

Check and update README files using expertise as reference. Skip node_modules.

## Workflow

### Root Level
- `README.md` (if exists) - project overview, stack, run commands
- `AI_docs/README.md` - documentation index

### Claude Commands
- `.claude/README.md` - slash commands overview
- `.claude/commands/experts/README.md` - expert system docs

### ADW
- `ADW/dev-inspector/README.md` - dev-inspector usage

### Backend (Hexagonal Architecture)
Use expertise files as reference:
- `.claude/commands/experts/backend-domain/expertise.yaml`
- `.claude/commands/experts/backend-handlers/expertise.yaml`
- `.claude/commands/experts/backend-usecases/expertise.yaml`

READMEs to check:
- `project/backend/README.md` - backend overview
- `project/backend/internal/domain/README.md` - entities
- `project/backend/internal/ports/README.md` - interfaces
- `project/backend/internal/adapters/README.md` - adapters index
- `project/backend/internal/adapters/anthropic/README.md` - LLM adapter
- `project/backend/internal/adapters/postgres/README.md` - PostgreSQL
- `project/backend/internal/usecases/README.md` - business logic
- `project/backend/internal/handlers/README.md` - HTTP handlers
- `project/backend/internal/prompts/README.md` - LLM prompts
- `project/backend/internal/tools/README.md` - Tool executors
- `project/backend/internal/config/README.md` - configuration
- `project/backend/internal/logger/README.md` - logging

### Frontend (Feature-Sliced Design)
Use expertise files as reference:
- `.claude/commands/experts/frontend-shared/expertise.yaml`
- `.claude/commands/experts/frontend-entities/expertise.yaml`
- `.claude/commands/experts/frontend-features/expertise.yaml`

READMEs to check:
- `project/frontend/README.md` - frontend overview
- `project/frontend/src/shared/README.md` - shared layer
- `project/frontend/src/shared/api/README.md` - API client
- `project/frontend/src/entities/README.md` - entities layer
- `project/frontend/src/entities/atom/README.md` - atom entity
- `project/frontend/src/entities/widget/README.md` - widget entity
- `project/frontend/src/entities/formation/README.md` - formation entity
- `project/frontend/src/entities/message/README.md` - message entity
- `project/frontend/src/features/README.md` - features layer
- `project/frontend/src/features/chat/README.md` - chat feature
- `project/frontend/src/features/catalog/README.md` - catalog feature
- `project/frontend/src/features/overlay/README.md` - overlay feature
- `project/frontend/src/features/canvas/README.md` - canvas feature

## For Each README

1. Read current content
2. Compare with actual code in that directory
3. Check if structure/exports/usage is accurate
4. Update only if discrepancies found

## Output

```
README Sync Complete

Updated:
- [list of updated files]

No changes:
- [list of unchanged files]

Missing:
- [list of missing READMEs that might need creation]
```
