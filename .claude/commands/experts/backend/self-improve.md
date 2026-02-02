---
allowed-tools: Read, Grep, Glob, Bash, Edit, Write
description: Self-improve Backend expertise by validating against codebase implementation
argument-hint: [check_git_diff (true/false)] [focus_area (optional)]
---

# Purpose

Maintain the Backend expert system's expertise accuracy by comparing the existing expertise file against the actual codebase implementation. The expertise file is your **mental model** — not documentation, the code is the source of truth.

## Variables

CHECK_GIT_DIFF: $1 default to false if not specified
FOCUS_AREA: $2 default to empty string
EXPERTISE_FILE: .claude/commands/experts/backend/expertise.yaml
MAX_LINES: 300

## Instructions

- Validate expertise against real implementation, not assumptions
- Focus on: Go files, handlers, adapters, usecases, domain entities
- If FOCUS_AREA is provided, prioritize validation for that area
- Maintain YAML structure and formatting
- Enforce strict line limit of MAX_LINES
- Prioritize actionable, high-value expertise

## Workflow

### 1. Check Git Diff (Conditional)
If CHECK_GIT_DIFF is "true":
```bash
git diff HEAD~5 --name-only -- project/backend/
```
Note changes for targeted validation.

### 2. Read Current Expertise
Read EXPERTISE_FILE to understand current state:
- overview, project_structure
- layer_rules (hexagonal architecture)
- core_implementation (active + hexagonal)
- api_endpoints, patterns
- migration_status

### 3. Validate Against Codebase
Read key implementation files:

**Entry & Config:**
- `project/backend/cmd/server/main.go`
- `project/backend/internal/config/config.go`

**Hexagonal Layers:**
- `project/backend/internal/domain/*.go`
- `project/backend/internal/ports/*.go`
- `project/backend/internal/adapters/*/*.go`
- `project/backend/internal/usecases/*.go`
- `project/backend/internal/handlers/*.go`
- `project/backend/internal/prompts/*.go`
- `project/backend/internal/tools/*.go`

**Data:**
- `project/backend/data/products.json`

### 3.5. Extract Integration Gotchas (CRITICAL)
This step prevents spec-to-implementation bugs.

**Data Types:**
- Check port interfaces for parameter types (UUID vs string vs slug)
- Look for `uuid.UUID` or string format validation
- Check price fields (int kopecks vs float rubles)

**Foreign Keys:**
- Read migration files: `project/backend/internal/adapters/postgres/*_migrations.go`
- Look for `REFERENCES` in CREATE TABLE statements
- Document which tables depend on which

**SQL Logic:**
- Read adapter implementations for WHERE clause construction
- Check if filters use AND or OR
- Document ILIKE patterns

**External APIs:**
- Read adapter files for API versions, headers
- Check `anthropic-version` header value
- Extract pricing info from domain/tool_entity.go if present

### 4. Identify Discrepancies
List all differences:
- New or removed files
- Changed struct fields or methods
- New endpoints or handlers
- Updated patterns
- Removed features still documented

### 5. Update Expertise File
- Remedy all identified discrepancies
- Add missing information
- Update outdated information
- Remove obsolete information
- Maintain YAML structure
- Keep descriptions concise

### 6. Enforce Line Limit
```bash
wc -l .claude/commands/experts/backend/expertise.yaml
```
If > MAX_LINES:
- Trim verbose descriptions
- Remove redundant examples
- Repeat until <= MAX_LINES

### 7. Validation Check
Verify YAML syntax:
```bash
python3 -c "import yaml; yaml.safe_load(open('.claude/commands/experts/backend/expertise.yaml'))"
```
Fix any syntax errors.

## Report

```
Backend Expert Self-Improvement Complete

Summary:
- Git diff checked: Yes/No
- Focus area: <area or "none">
- Discrepancies found: N
- Discrepancies remedied: N
- Final line count: N/300 lines

Discrepancies Found:
1. <description>
   - Found in: <file path>
   - Remedied: <action taken>

Updates Made:
- Added: <list>
- Updated: <list>
- Removed: <list>

Line Limit Enforcement:
- Initial: N lines
- Final: N lines
- Trimmed: <what was removed or "nothing">

Validation:
- YAML syntax valid: Yes/No
- Active code documented: Yes/No
- Hexagonal stubs documented: Yes/No
```
