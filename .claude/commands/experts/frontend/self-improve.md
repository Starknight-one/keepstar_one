---
allowed-tools: Read, Grep, Glob, Bash, Edit, Write
description: Self-improve Frontend expertise by validating against codebase implementation
argument-hint: [check_git_diff (true/false)] [focus_area (optional)]
---

# Purpose

Maintain the Frontend expert system's expertise accuracy by comparing the existing expertise file against the actual codebase implementation. The expertise file is your **mental model** — not documentation, the code is the source of truth.

## Variables

CHECK_GIT_DIFF: $1 default to false if not specified
FOCUS_AREA: $2 default to empty string
EXPERTISE_FILE: .claude/commands/experts/frontend/expertise.yaml
MAX_LINES: 300

## Instructions

- Validate expertise against real implementation, not assumptions
- Focus on: React components, hooks, API integration, styling
- If FOCUS_AREA is provided, prioritize validation for that area
- Maintain YAML structure and formatting
- Enforce strict line limit of MAX_LINES
- Prioritize actionable, high-value expertise

## Workflow

### 1. Check Git Diff (Conditional)
If CHECK_GIT_DIFF is "true":
```bash
git diff HEAD~5 --name-only -- project/frontend/
```
Note changes for targeted validation.

### 2. Read Current Expertise
Read EXPERTISE_FILE to understand current state:
- overview, project_structure
- layer_rules (feature-sliced)
- core_implementation (active + feature_sliced)
- patterns
- migration_status

### 3. Validate Against Codebase
Read key implementation files:

**Active Code (working):**
- `project/frontend/src/App.jsx`
- `project/frontend/src/App.css`
- `project/frontend/src/components/Chat.jsx`
- `project/frontend/src/components/Chat.css`
- `project/frontend/src/main.jsx`

**Feature-Sliced Structure (stubs):**
- `project/frontend/src/shared/api/apiClient.js`
- `project/frontend/src/entities/atom/*.js`
- `project/frontend/src/entities/widget/*.js`
- `project/frontend/src/entities/message/*.js`
- `project/frontend/src/features/chat/*.js`
- `project/frontend/src/features/overlay/*.js`
- `project/frontend/src/app/App.jsx`

**Config:**
- `project/frontend/package.json`
- `project/frontend/vite.config.js`

### 4. Identify Discrepancies
List all differences:
- New or removed components
- Changed hooks or state management
- New API endpoints
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
wc -l .claude/commands/experts/frontend/expertise.yaml
```
If > MAX_LINES:
- Trim verbose descriptions
- Remove redundant examples
- Repeat until <= MAX_LINES

### 7. Validation Check
Verify YAML syntax:
```bash
python3 -c "import yaml; yaml.safe_load(open('.claude/commands/experts/frontend/expertise.yaml'))"
```
Fix any syntax errors.

## Report

```
Frontend Expert Self-Improvement Complete

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
- Feature-sliced stubs documented: Yes/No
```
