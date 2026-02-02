# Sync Experts

Update all expert knowledge from codebase.

## Variables

EXPERTS_DIR: .claude/commands/experts/

## Instructions

- Sync each expert with its codebase layer
- Report changes

## Experts List

**Backend (hexagonal layers):**
- `backend-domain` → `project/backend/internal/domain/`
- `backend-ports` → `project/backend/internal/ports/`
- `backend-adapters` → `project/backend/internal/adapters/`
- `backend-usecases` → `project/backend/internal/usecases/`
- `backend-handlers` → `project/backend/internal/handlers/`
- `backend-pipeline` → `project/backend/internal/tools/`, `project/backend/internal/prompts/`

**Frontend (FSD layers):**
- `frontend-shared` → `project/frontend/src/shared/`
- `frontend-entities` → `project/frontend/src/entities/`
- `frontend-features` → `project/frontend/src/features/`

## Workflow

### Step 1: Sync Each Expert
For each expert:
1. Read current `expertise.yaml`
2. Glob files in corresponding path
3. Compare structure with expertise
4. Update if needed (preserve format, update facts)

### Step 2: Quick Validation
Check that key files exist:
```bash
ls project/backend/internal/domain/*.go | head -5
ls project/frontend/src/entities/*/
```

### Step 3: Report

## Output

```
Experts Synced

Backend:
- backend-domain: [ok|updated]
- backend-ports: [ok|updated]
- backend-adapters: [ok|updated]
- backend-usecases: [ok|updated]
- backend-handlers: [ok|updated]
- backend-pipeline: [ok|updated]

Frontend:
- frontend-shared: [ok|updated]
- frontend-entities: [ok|updated]
- frontend-features: [ok|updated]

Total: 9 experts
```
