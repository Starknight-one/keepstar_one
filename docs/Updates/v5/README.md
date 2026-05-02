# V5 Session Logs

Each implementation session on the `v5` branch leaves a log here.

## File naming

`v5_<YYYY-MM-DD>_<HH-MM>.md`

Date and time = the moment of the commit that closes the session (UTC).

## Required sections

```markdown
# <short title>

**Branch**: v5
**Date (UTC)**: 2026-XX-XX HH:MM
**Commit**: <sha>
**Parent**: <sha>

## Context
Why this session happened. What gap or task is being closed. Reference to the chunk plan
in `~/.claude/plans/` if relevant.

## Approach
What we changed and why this approach.

## Files changed
| File | Status | Notes |
|---|---|---|
| ... | added/modified/deleted | ... |

## Verification
How we verified locally (commands run, tests passing). What to watch on prod once deployed.

## Known gaps / caveats
What's NOT closed yet. Deferred details. Follow-ups.
```

## Per-chunk plans

Each implementation chunk gets a detailed plan document in `plans/`, written
in plan mode and approved before execution. Naming:
`plans/chunk-<N>-<short-name>.md` (e.g. `plans/chunk-1-engine-port.md`).

Plan + log are paired: the plan describes what we set out to do; the log
describes what actually happened (commit sha, files changed, gaps).

## See also

- High-level plan: `../../v5-engine-plan.md`
- V4 reference logs: `../feature-engine-v4_*.md`
