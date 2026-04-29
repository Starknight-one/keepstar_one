# backend-domain Self-Improve

Update V4 backend-domain expertise from current code in `project_v4/backend/internal/domain/`.

## Variables

USE_DIFF: $ARGUMENTS (true | false, default: false)
EXPERTISE: .claude/commands/experts/backend-domain/expertise.yaml
CODE_ROOT: project_v4/backend/internal/domain/
LINE_LIMIT: 250

## Instructions

- Scan code, update YAML, keep under LINE_LIMIT.
- Preserve mental model: ui_primitives / chat / catalog / pipeline / tracing / errors / gotchas.
- Refresh anything tied to specific names, fields, or constants.

## Workflow

### Step 1 — Read current expertise
Note which sections already exist so you only update, not duplicate.

### Step 2 — Choose scope
- USE_DIFF=true →
  ```bash
  git diff HEAD~10 --name-only -- project_v4/backend/internal/domain/
  ```
  Focus on changed files.
- USE_DIFF=false → scan all `*_entity.go` files in CODE_ROOT.

### Step 3 — Refresh facts

For each `<entity>_entity.go`:
- Re-read top-of-file `type <X> struct { ... }` declarations and `const ( ... )` enums.
- Update the matching subsection (e.g. `ui_primitives.atom_entity.types`, `state_entity.action_types`) to reflect actual types/enum values.
- If a NEW type appeared → add a line under the relevant entity's `types` array.
- If a type was REMOVED → drop the line.

Quick scan command for enums:
```bash
grep -nE "^type [A-Z][A-Za-z]+ string|^const \(|^\t[A-Z][A-Za-z_]+ +[A-Z][A-Za-z]+ += +" project_v4/backend/internal/domain/<file>.go
```

### Step 3.5 — Integration gotchas

Preserve the existing gotchas. Add a new one when:
- A unit/value boundary surprised you (kopecks vs rubles, ms vs seconds, slug vs UUID).
- Two types have similar names and the wrong one was used (e.g. domain.Preset vs engine_v4.Preset vs preset_v2 Preset).
- A migration changed a field shape and old data still exists in DB.

Remove a gotcha if its underlying behavior changed.

### Step 4 — Cross-layer drift

Verify these still hold:
- All entity files still compile under `keepstar_v4` module (no accidental imports of internal/ packages — domain layer must stay leaf).
- `LegacyTypeMapping` still exists in atom_entity (used for pre-V4 stored data).
- `domain_errors.go` still exports the listed errors.

### Step 5 — Tests

```bash
ls project_v4/backend/internal/domain/*_test.go
```
Update `state_entity.test_file` / `catalog_digest_entity.test_file` references and the Tests array if a new test file appeared.

### Step 6 — Active workstreams

Skim `docs/PRE_LAUNCH_TASKS.md` for waves that introduce new domain types (B7 → field_definitions). Update `active_workstreams.notes` if scope shifted.

### Step 7 — Report

```
backend-domain expertise updated

Changes:
- Added: <items>
- Updated: <items>
- Removed: <items>

Lines: N / 250
```

## Constraints

- DO NOT exceed LINE_LIMIT.
- DO NOT mix in V1/V2 entities from `project/backend/internal/domain/` — that's the legacy expert (deprecated).
- DO record enum values verbatim from `const ( ... )` — these are user-facing strings used in JSON.
- DO NOT scan `ADW/adw.yaml` for line limits — it's stale.
- DO preserve the gotchas section — it's load-bearing for question-answering.

## Output

Updated `expertise.yaml` plus a summary report.
