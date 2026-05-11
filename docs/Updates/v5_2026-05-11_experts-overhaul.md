# Session Update Log

**Branch**: v5  
**Date**: 2026-05-11  
**Commits**: 7d11f8f, ba05dc5  
**Parent**: 73eebc7

---

## Context

Two tasks in this session:

1. **OpenUI research → 3 improvements** — Studied thesysdev/openui repo. Verdict: their full DSL is not portable (flat declarations vs our ops-on-persistent-graph), but 3 patterns were worth lifting.

2. **Expert system overhaul** — Existing experts (engine-v4, pipeline-agents, widget) all pointed at V4 code. With V5 in production and V4 engine no longer being touched, these were replaced with V5-targeting versions.

---

## Approach

### Commit 7d11f8f — OpenUI-inspired improvements

Three independent changes:

**Change 1 — React Error Boundary** (`project_v5/frontend/src/renderer/`):  
V5 widget had no error boundary — any thrown error in any node component crashed the entire widget. Added `RendererErrorBoundary.jsx` (class component, ~35 LOC) and wrapped `SceneGraphRenderer.jsx`'s render output in it. The boundary auto-resets when the document reference changes (new turn → new object reference → `getDerivedStateFromProps` resets `hasError`).

**Change 2 — Preset catalog auto-generation** (`project_v5/backend/internal/engine/presets/prompt.go`, `prompts/agent2_prompt.go`):  
Agent2's system prompt had a hardcoded list of 12 preset names — 3 of which (`catalog_category_card`, `liked_grid`, `cart_grid`) don't exist in V5's `SystemPresetSeeds`. New `presets/prompt.go` generates `SystemPresetsBlock` at startup from the live registry keys + a `SystemPresetDescriptions` map. `agent2_prompt.go` split into two static parts with the generated block injected between them. Adding a new preset seed + description is now sufficient for it to appear in Agent2's context.

**Change 3 — Modify patch-size hint** (`prompts/agent2_prompt.go`):  
Added one sentence in the `## MODE` section: *"A typical modify patch is 1–5 ops. If you need more, use rebuild — it is cleaner and avoids partial-state bugs."* Addresses known gap A2 (Agent2 sometimes emits full-rebuild worth of ops in modify mode).

### Commit ba05dc5 — Expert system overhaul

- **Deleted**: `engine-v4/` expert (3 files)
- **Created**: `engine-v5/` expert (3 files) — covers V5 scene-graph engine: 14 node types, 5 op types, 8-step pipeline, 9 system presets, binding, TreeMap
- **Rewrote**: `pipeline-agents/` — now covers `project_v5/backend/internal/`. Documents Agent2SystemPrompt as `var` (not const), `PromptCache.GetOrBuild`, `v5_chat_session_traces`, prefetch/span_helper/state_reconstruct/rollback files, readyz handler, known gaps A1–A5
- **Rewrote**: `widget/` — now covers `project_v5/frontend/`. Flat structure (no FSD), SceneGraphRenderer → NodeRenderer → Frame/Group/Text/Image/Ref, RendererErrorBoundary, RenderContext shape, 9 action kinds, fillTemplate binding, gotchas (no session cache, no back button A4)
- **Updated**: `_meta.yaml` — replaced engine-v4 globs with engine-v5 globs; updated pipeline-agents and widget globs to `project_v5/` paths
- **Updated**: `README.md` — expert table and Files tree updated

---

## Files Changed

| File | Change |
|---|---|
| `project_v5/frontend/src/renderer/RendererErrorBoundary.jsx` | Created |
| `project_v5/frontend/src/renderer/SceneGraphRenderer.jsx` | Modified — wrap in RendererErrorBoundary |
| `project_v5/backend/internal/engine/presets/prompt.go` | Created |
| `project_v5/backend/internal/prompts/agent2_prompt.go` | Rewrote — var + two-part split + modify hint |
| `.claude/commands/experts/engine-v4/` (3 files) | Deleted |
| `.claude/commands/experts/engine-v5/` (3 files) | Created |
| `.claude/commands/experts/pipeline-agents/` (3 files) | Rewrote for V5 |
| `.claude/commands/experts/widget/` (3 files) | Rewrote for V5 |
| `.claude/commands/experts/_meta.yaml` | Updated globs |
| `.claude/commands/experts/README.md` | Updated expert table + Files tree |

---

## Verification

**Backend** (commit 7d11f8f):
- `cd project_v5/backend && go build ./...` — passed
- `go test ./...` — passed

**Frontend** (commit 7d11f8f):
- `cd project_v5/frontend && npm test` — all 38 tests passed

---

## Known Gaps / Not Closed

- A1 greeting handling (V5 emits empty_not_found on «Привет»)
- A2 fully addressed only with prompt hint — Agent2 behavior change requires prod observation
- A3 replicate count + pagination (V5 hardcoded 3)
- A4 back button absent in widget
- A5 skip Agent2 on Agent1 no-op
- A6 layout density (cards narrower than V4)
- chunk 16 frontend route swap behind flag (pending A1–A4)
