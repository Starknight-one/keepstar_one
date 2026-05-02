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

## See also

- High-level plan: `../../v5-engine-plan.md`
- Per-chunk implementation plans live in `~/.claude/plans/` (not committed)
- V4 reference logs: `../feature-engine-v4_*.md`
