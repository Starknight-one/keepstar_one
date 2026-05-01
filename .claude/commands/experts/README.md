# Expert System

Per-domain mental models that Claude reads/refreshes to give grounded,
current answers about this codebase. Replaces "ask Claude to grep around"
with "ask the catalog expert" — same model, but with a structured
cheat-sheet already loaded.

> **Source of truth is always the code.** Expertise YAMLs are working
> memory that gets validated against the code via `self-improve`.

## The 6 vertical experts

Each covers ONE business domain across ALL hexagonal layers (NOT one layer
across all domains — that pattern was the previous design and didn't work).

| Expert | Owns |
|---|---|
| `catalog` | Cross-cutting domain. Write side: `project_admin/` (harvester, discovery agent, mapping artifact, merge_apply, Shopify integration). Read side: `project_v4/.../postgres_catalog.go`. |
| `engine-v4` | Ops-driven UI assembly engine: `project_v4/backend/internal/engine_v4/`. Op types, presets, binding, constraints, TreeMap. |
| `pipeline-agents` | V4 chat orchestration: pipeline_execute, Agent1/Agent2, prompts, tools, anthropic adapter, span tracing, prompt caching. |
| `widget` | Embeddable chat widget: `project/frontend/`. Shadow DOM, FormationRenderer, fillFormation (instant expand), sessionCache. |
| `admin` | Auth + billing + KeepstarCanvas + admin SPA in `project_admin/` (catalog usecases excluded — those are owned by `catalog`). |
| `curator` | Standalone service: `curator/`. Operator dashboard for catalog curation. Proxies merge to admin via MergeProxy. |

Each expert lives in `.claude/commands/experts/<name>/` with three files:

- `expertise.yaml` — the mental model (overview, key files, invariants, gotchas, related_experts)
- `question.md` — slash command `/experts:<name>:question <prompt>` answers without code changes
- `self-improve.md` — slash command `/experts:<name>:self-improve true` re-reads code and updates the YAML

## How to use

**Ask a question** (no code changes):
```
/experts:catalog:question Why does merge_apply skip listings without SKU?
```
The expert answers using its YAML + reads the relevant code, returns specific
`file:line` refs, mentions related experts when the answer crosses layers.

**Refresh manually** after a big refactor in one domain:
```
/experts:catalog:self-improve true
```

**Refresh ALL changed domains at once**:
```
/sync-experts                # selective by default (auto)
/sync-experts --all          # full sync, all 6 experts
/sync-experts --domain catalog
/sync-experts --diff HEAD~5  # diff from a non-default base
```

## Auto-sync at session close

When you close Claude Code (`/exit` or terminal close), a `SessionEnd` hook
spawns a background `claude --print "/sync-experts --auto"` in a fresh
headless session. It:

1. Diffs your work since `origin/main` + working tree changes
2. Maps changed files to affected domains via `_meta.yaml` globs
3. For each affected domain, runs that expert's `self-improve`
4. Logs to `.claude/.last_sync.log`
5. Exits silently

The original Claude Code closes immediately — the background agent does its
work without blocking you. Cost is bounded (~$0.10 max for full sync; usually
much less since it's selective). Headless invocation = fresh context, no
need to `/compact` your closing session.

**Auto-sync skips when:**
- Repo is fully clean (no diff vs `origin/main`, no working-tree edits, no untracked files)
- `reason == "clear"` (the `/clear` command isn't a real session close)
- Another sync is already running (atomic `mkdir` lock)

**To disable auto-sync temporarily:** rename the hook script
(`mv .claude/hooks/sync_experts_on_session_end.sh{,.disabled}`) or delete
the `SessionEnd` block from `.claude/settings.json`.

**Caveat (macOS):** the lock uses `mkdir` (atomic on POSIX). The original
implementation used `flock` which doesn't ship on macOS — silently failed.
If you see hooks not running, check `.claude/.last_sync.log`; missing log
file usually means the script aborted before that line.

## Files

```
.claude/
├── commands/
│   ├── experts/
│   │   ├── README.md             ← you are here
│   │   ├── _meta.yaml            ← single source of truth: domain → file globs
│   │   ├── _templates/           ← scaffolding for new experts
│   │   ├── catalog/{expertise.yaml, question.md, self-improve.md}
│   │   ├── engine-v4/...
│   │   ├── pipeline-agents/...
│   │   ├── widget/...
│   │   ├── admin/...
│   │   └── curator/...
│   └── sync-experts.md           ← /sync-experts slash command
├── hooks/
│   └── sync_experts_on_session_end.sh   ← SessionEnd hook
├── settings.json                 ← registers the hook
├── .last_sync.log                ← (gitignored) last auto-sync run output
└── .sync.lock.d/                 ← (gitignored) atomic lock dir during sync
```

## Adding a new expert

1. Add a top-level domain block to `_meta.yaml` with its `globs:` list.
2. Create `.claude/commands/experts/<name>/` with the 3 files. Use
   `_templates/` as a starting point. Claude Code auto-discovers the trio
   as `/experts:<name>:question` and `/experts:<name>:self-improve`.
3. Write a **domain-specific** `self-improve.md` (NOT a copy of the
   generic template — list specific files, structs, and drift surfaces to
   check for THIS domain).
4. Run `/experts:<name>:self-improve true` to populate the YAML.
5. Update this README's table.

## Origin

The pattern is from
`/Users/starknight/SelfImproving/agent-experts/README.md` — ACT → LEARN
→ REUSE with vertical domain experts. Each expertise YAML lives somewhere
in the 600-1000 line range when mature. Smaller = thin/incomplete. Larger =
either a real big domain or signal that the underlying module needs splitting.
