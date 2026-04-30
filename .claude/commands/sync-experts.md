# Sync Experts

Refresh vertical-expert YAMLs by re-reading current code. Selective by default.

## Variables

MODE: $ARGUMENTS (--auto | --all | --domain <name> | --diff <ref>)
META_FILE: .claude/commands/experts/_meta.yaml
EXPERTS: catalog, engine-v4, pipeline-agents, widget, admin, curator

## Instructions

- The 6 vertical experts above are the ONLY ones to sync. Do not touch the
  9 legacy horizontal experts (backend-*, frontend-*) — they reference deleted
  `project/backend/` paths and are scheduled for removal.
- Each vertical expert has its own `self-improve.md` with domain-specific
  scanning logic. This command's job is to decide WHICH experts to refresh,
  then invoke their per-expert self-improve.

## Workflow

### Step 1 — Parse arguments

- `--auto` (default; used by SessionEnd hook): scan changed files vs
  `origin/main` (or `main~5` if currently on `main` itself), then map to
  affected experts via META_FILE globs.
- `--all`: refresh all 6 verticals unconditionally.
- `--domain <name>`: refresh only the named domain. Validate name is in EXPERTS.
- `--diff <ref>`: same as --auto but use the supplied ref as the diff base.

### Step 2 — Load META_FILE

Read `META_FILE` (YAML). Each top-level key under `experts:` is a domain;
each has `globs: [...]` of fnmatch-style patterns. Patterns may contain
`{a,b,c}` brace expansion — expand these into individual globs before matching.

### Step 3 — Determine affected domains

For `--auto` / `--diff`:
1. Run `git diff <base> --name-only` UNION `git diff --name-only` (working tree)
   UNION `git ls-files --others --exclude-standard` (untracked).
2. For each domain in META_FILE: if ANY glob matches ANY changed file
   (after brace expansion), mark domain as affected.
3. Report skipped domains (no matching files) and proceed only with affected.

For `--all`: mark all 6 domains as affected.
For `--domain X`: mark only X.

### Step 4 — Invoke per-domain self-improve

For each affected domain D in priority order
(catalog → engine-v4 → pipeline-agents → widget → admin → curator):

1. Run the domain's self-improve via `/experts:<D>:self-improve true`.
   The `true` arg enables USE_DIFF mode where supported (faster scan).
2. Capture its output.
3. If self-improve reports an error, log and continue (do not abort the batch).

### Step 5 — Report

Single concise report. No expert YAML preview — the per-expert self-improve
already reports its own changes.

## Constraints

- DO NOT invoke any of the 9 legacy horizontal experts.
- DO NOT modify `_meta.yaml` from this command — that's a separate manual edit.
- DO NOT commit changes from this command — the SessionEnd hook (or user)
  decides whether to commit refreshed YAMLs.

## Output

```
sync-experts mode=<MODE>

Affected: <comma-separated domains>
Skipped: <count> (no matching changes)

Per-domain results:
- catalog: <updated|ok|error>
- engine-v4: <updated|ok|error>
- ...
```
