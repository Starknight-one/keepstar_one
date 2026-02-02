# frontend-features Self-Improve

Update frontend-features expertise from codebase.

## Variables

USE_DIFF: $ARGUMENTS (true|false, default: false)
EXPERTISE: .claude/commands/experts/frontend-features/expertise.yaml
CONFIG: ADW/adw.yaml

## Instructions

- Scan codebase for frontend-features knowledge
- Update expertise.yaml with findings
- Keep within line limit from CONFIG

## Workflow

### Step 1: Load Current Expertise
Read EXPERTISE to understand current state.

### Step 2: Load Configuration
Read CONFIG to get:
- Paths for this domain
- Line limit

### Step 3: Scan Codebase
If USE_DIFF = true:
```bash
git diff HEAD~5 --name-only
```
Focus on changed files.

Otherwise, scan domain paths from CONFIG.

### Step 4: Extract Knowledge
For each relevant file:
- File purpose
- Key patterns
- Important structures
- Dependencies

### Step 4.5: Extract Integration Gotchas (CRITICAL)
This prevents spec-to-implementation bugs:

**Data Types:**
- Check interface parameter types (UUID vs string vs slug)
- Check numeric types (int cents vs float dollars)

**Database Constraints:**
- Read migration files for REFERENCES (foreign keys)
- Document: "creating X requires Y to exist first"

**External APIs:**
- Extract API versions from adapter code
- Document auth headers, base URLs

**SQL/Filter Logic:**
- Check how WHERE conditions combine (AND vs OR)
- Document edge cases

Add findings to `gotchas`, `external_apis`, `integration_patterns` sections.

### Step 5: Update Expertise
Update expertise.yaml:

**PRESERVE (never modify):**
- `overview.architecture` — architectural pattern name
- `overview.description` — high-level description
- `layer_rules` — dependency rules between layers
- `patterns` — naming conventions and patterns

**UPDATE (from codebase scan):**
- `project_structure` — actual files and directories
- `core_implementation` — current implementation details
- `api_endpoints` — actual endpoints
- `run_commands` — verified commands
- `migration_status` — current state

Rules:
- Keep format consistent
- Stay within line limit
- Focus on actionable knowledge

### Step 6: Report Changes

## Constraints

- DO NOT: exceed line limit
- DO: focus on patterns, not docs
- DO: include file paths
- DO: keep it actionable

## Output

```
frontend-features Expertise Updated

Changes:
- Added: <what>
- Updated: <what>
- Removed: <what>

Lines: N / {limit}
```
