> ⚠️ **HISTORICAL — documents the V4 Pencil hybrid engine, which is NOT on the production path.** V4 was dropped; V5 is the only chat engine (live at v5-engine-production.up.railway.app). The ops/build modes here were not carried forward to V5. See one_ultra/CLAUDE.md and /SESSION_HANDOFF_2026-05-30.md. Kept for architectural reference on ops/build concepts. (flagged 2026-06-01)

# Pencil Hybrid Engine — Status Report

## What Was Built

Three new modes added to `visual_assembly` tool alongside existing auto pipeline.

### Mode: `auto` (existing, enhanced)
- V2 engine pipeline unchanged
- **New**: `StampTreeIDs` stamps readable IDs on every atom (`a-s0-w0-price`) and layout node (`n-s0-w0-root`) after each auto build
- **New**: `BuildTreeMap` creates compact tree summary passed to Agent2 as `formation_tree` in prompt context
- **New**: `SnapshotCustomizations` / `ApplySnapshot` — before auto rebuild, extracts locked atom overrides (textStyle, wrapper, etc.) and freestyle atoms; re-applies them after rebuild

### Mode: `ops` — modify existing formation tree
- **Operations**: update, delete, insert, move
- **Update**: merge props (textStyle, wrapper, mediaStyle, format, value) into target atom/node
- **Delete**: remove atom + fix layout indices, or remove layout node
- **Insert**: add freestyle atoms (buttons, badges, text) or layout nodes into tree
- **Move**: relocate atom/node to different parent/position
- **Wildcard targets** (Fix D): `target:"price"` auto-expands to all widgets containing that field
- **Ref chaining**: `{"ref":"cta"}` → `{"parent":"$cta"}` for multi-op sequences
- **Data binding** (Fix B): `ResolveInsertedFieldValues` fills in entity values for atoms with `fieldName`
- Auto-locks modified atoms (`rigidity: locked`)
- Runs constraint pipeline after all ops

### Mode: `build` — multi-section page composition
- `SectionSpec` with `source: "auto"` or `source: "freestyle"`
- Auto sections run full V2 engine pipeline
- Freestyle sections create widgets from literal atom definitions
- `ParseSectionSpecs` parses raw JSON into typed specs

### Files Created
- `project/backend/internal/engine/ops.go` (~820 LOC) — operations executor, wildcard expansion, snapshot/restore, field resolution
- `project/backend/internal/engine/build.go` (~220 LOC) — section builder
- `project/backend/internal/engine/tree_ids.go` (~155 LOC) — ID stamping + tree map

### Files Modified
- `project/backend/internal/domain/atom_entity.go` — added `ID` field to `AtomV2`
- `project/backend/internal/domain/layout_entity.go` — added `ID` field to `LayoutNode`
- `project/backend/internal/engine/engine_v2.go` — one line: call `StampTreeIDs` at end
- `project/backend/internal/tools/tool_visual_assembly.go` — mode dispatch, ops/build executors, snapshot carry-over, wildcard expansion, field resolution
- `project/backend/internal/usecases/agent2_execute.go` — build formation_tree for Agent2 context, DB deserialization fix
- `project/backend/internal/prompts/prompt_compose_widgets.go` — ops/build mode docs in Agent2 system prompt
- `project/frontend/src/entities/formation/FormationRenderer.jsx` — sections rendering guard fix

### Commits (branch: `refactor/v1-engine-removal`)
1. `1cca1ab` — feat: Pencil Hybrid Engine (all 3 phases)
2. `bd304dd` — fix: 3 critical bugs (DB deserialization, frontend sections guard, formation_tree)
3. `38af35a` — fix: 4 critical fixes (snapshot carry-over, field resolution, wildcard ops, build limit)

---

## Production Test Results

### Session 1 (pre-fixes, 13 traces)

| Test | Result | Issue |
|------|--------|-------|
| "покажи крема" | OK | Auto mode works |
| "покажи крупными" | OK | size:large works |
| "show with descriptions" | PARTIAL | Auto added description but reset size to small (lost large) |
| "make price bigger" | OK | Ops mode used correctly, 23 ops (one per widget) |
| "add brand as tag" | BROKEN | Brand value = `<UNKNOWN>` — ops insert can't resolve entity data |
| "красная цена жирным" | OK | Ops update works |
| "добавь рейтинг" | BROKEN | Used auto mode → wiped all ops customizations |
| "убери рейтинг" | OK via auto | But auto, not ops delete |
| "добавь кнопку купить" | BROKEN | `error: invalid formation in state` (DB deserialization) |
| "добавь бейдж новинка" | BROKEN | Updated existing button instead of inserting new badge |
| "покажи шампуни" | OK | Auto with new data, expected reset |
| "лендинг" (x2) | BROKEN | Backend OK ("built 3 sections") but frontend showed nothing |

### Session 2 (post-fixes, 4 traces)

| Test | Result | Issue |
|------|--------|-------|
| "покажи в 5 колонок + кнопку купить" | BROKEN | Agent2 tried to do both in one call, neither worked |
| "добавь кнопку купить" | PARTIAL | Button added but wrong position, doubled on first widget |
| "лендинг из первых трёх" | PARTIAL | Rendered but: freestyle text plain, comparison table from preset (not creative), layout basic |

---

## Remaining Issues (Priority Order)

### P0: Insert positioning / duplication
**Symptom**: "добавь кнопку купить" — button appears in wrong layout position, doubles on first widget.
**Root cause**: `insertAtom` appends to parent node's children, but when wildcard expansion creates one insert per widget, the first widget may get hit twice (once from wildcard, once from original). Also `insertChildIntoNode` positioning logic may fail when `after` ID doesn't match children in parent.
**Where**: `ops.go` → `applyInsert` / `insertAtom` / `ExpandWildcardOps`
**Impact**: Any ops insert is unreliable.

### P1: Wildcard expansion for insert ops
**Symptom**: Insert with `parent:"root"` should expand to all widgets' root nodes, but the expansion logic in `ExpandWildcardOps` only handles `target` (for update/delete) and `parent` via node Name. If multiple nodes share a name across widgets, only some get matched.
**Where**: `ops.go` → `ExpandWildcardOps` — the node name matching uses the flat `idx.nodes` map which has ID-based keys, not name-based
**Impact**: Insert ops don't reliably apply to all widgets.

### P2: Auto mode still resets some state
**Symptom**: "покажи в 5 колонках" used auto mode which may reset customizations despite snapshot. The snapshot preserves locked atom overrides, but NOT: layout parameters (columns), widget size, formation mode.
**Where**: `ops.go` → `SnapshotCustomizations` only captures atom-level overrides, not formation-level params (columns, gap, grid config)
**Impact**: Layout-level customizations lost on auto rebuild.

### P3: Build mode — freestyle widget rendering
**Symptom**: Freestyle text blocks render as plain unstyled text (Image 4 — "Сравни топ 3 продукта" appears as raw text).
**Root cause**: 
1. `WidgetTypeTextBlock` may not have proper rendering in `WidgetRenderer.jsx` for V2 atoms
2. `WidgetV2ToLegacy` conversion for freestyle atoms may produce wrong legacy format
3. Freestyle widgets don't go through auto layout pipeline (`AutoLayout`, etc.) — they have a manual column layout
**Where**: Frontend `WidgetRenderer.jsx` (template routing), engine `compat.go` (v1 conversion), `build.go` (layout)
**Impact**: Build mode produces visually broken freestyle sections.

### P4: Build mode — Agent2 uses preset comparison instead of creative layout
**Symptom**: "сделай лендинг из первых трёх, расскажи кто круче" — Agent2 used comparison table preset instead of a creative freestyle section explaining differences.
**Root cause**: Agent2 doesn't have enough guidance on when to use freestyle sections creatively vs falling back to presets. The prompt examples are basic.
**Where**: Prompt `prompt_compose_widgets.go` — build mode section
**Impact**: Build mode produces generic preset output instead of creative compositions.

### P5: Cost — ops generate too many tokens
**Symptom**: 5 cents for 4 queries (2.5x normal). Agent2 generates verbose ops arrays even with wildcard support.
**Root cause**: 
1. formation_tree in context adds tokens (all widget/atom/node IDs for 50 widgets)
2. Agent2 may still generate per-widget ops despite wildcard prompt guidance
3. Build mode sections with detailed props are verbose
**Where**: Prompt size (formation_tree), Agent2 output verbosity
**Potential fix**: Compact formation_tree (only show first widget's atoms + total count), enforce 1-op-per-field in prompt more aggressively

### P6: `columns` parameter ignored in ops mode
**Symptom**: "покажи в 5 колонок" — Agent2 may have sent columns:5 but in ops mode, columns is a formation-level property not handled by ops (ops targets atoms/nodes).
**Root cause**: `mergeWidgetProps` handles size but not grid columns. Columns is on `FormationWithData.Grid.Cols`, not on widget or atom.
**Where**: `ops.go` — no formation-level property handling
**Potential fix**: Add formation-level ops targets or handle columns in auto mode params.

---

## Architecture Assessment

### What works well
- Tree IDs: stable, readable, deterministic
- Ops update/delete: correct for single-target modifications
- DB deserialization: fixed, formation survives state roundtrip
- Frontend sections: guard fixed, CSS exists
- Snapshot carry-over: atom-level overrides preserved across auto rebuilds

### What needs rework
1. **Wildcard expansion**: Current approach (expand field name → N per-widget ops) is fragile. Better: ops executor should internally iterate widgets when target is a field name, not expand into N ops
2. **Insert reliability**: Position determination (`insertChildIntoNode`) needs better parent/after resolution. Duplication guard needed
3. **Freestyle rendering**: Need a proper rendering path for text-block widgets with V2 atoms — either a new template or fallback in AtomV2Renderer
4. **Formation-level ops**: Columns, gap, grid config should be modifiable via ops (e.g. `target:"formation"` or `target:"grid"`)
5. **Prompt cost optimization**: formation_tree should be compact (one widget template, not all 50)

### Risk assessment
- Ops mode for simple modifications (update style, delete field) — **works, shippable**
- Ops insert — **unreliable, needs fixes before production**
- Build mode — **backend works, frontend rendering broken for freestyle**
- Cost — **2.5x increase acceptable short-term, needs optimization**
