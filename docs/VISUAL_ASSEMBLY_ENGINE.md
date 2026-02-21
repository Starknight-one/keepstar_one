# Visual Assembly Engine — Status & Assessment

**Date:** 2026-02-22
**Branch:** `feature/visual-assembly-engine`
**Goal:** Заменить хардкод preset-систему на динамический движок, где LLM управляет отображением через tool calls.
**Spec:** `ADW/specs/todo/engine-visual-assembly.md`

---

## Coverage vs Spec (~50%)

### Primitives: 7/12 (58%)

| # | Примитив | Статус | Что сделано |
|---|----------|--------|-------------|
| 1 | **show** | Done | `show: ["field1", "field2"]` в visual_assembly |
| 2 | **hide** | Done | `hide: ["rating"]` в visual_assembly |
| 3 | **display** | Done | `display: {"brand":"badge"}` + ValidateDisplay |
| 4 | **color** | Done | `color: {"brand":"red"}`, палитра 6 цветов + hex, badge→bg, text→font |
| 5 | **size** (per-atom) | Not done | Есть widget-level size, нет per-atom |
| 6 | **shape** | Not done | pill/rounded/square/circle |
| 7 | **order** | Done | `order: ["image","price","name"]` |
| 8 | **layer** | Not done | z-index, оверлеи на фото |
| 9 | **anchor** | Not done | привязка к image/parent/viewport |
| 10 | **direction** | Done | `direction: "horizontal"`, CSS horizontal mode |
| 11 | **place** | Not done | позиция виджета на экране |
| 12 | **layout** | Done | grid/list/single/carousel/comparison |

### Implementation Steps: 11/22 (50%)

| Step | Пункт | Статус |
|------|-------|--------|
| **0** | extractFields PIM поля | Done |
| **0** | BuildHistorySummary | Done |
| **0** | CatalogDisplayMeta | Not done |
| **0** | Нормализация данных каталога | Not done |
| **1** | Microcontext Agent1→Agent2 | Done (код, не LLM) |
| **1** | Pipeline передаёт microcontext | Done |
| **1** | state_filter тул | Done |
| **1** | history_lookup тул | Not done |
| **1** | Agent1 промпт обновлён | Done |
| **2** | Delta-интерфейс полей (add/remove) | Not done |
| **2** | Layout отвязан от пресета | Done |
| **2** | visual_assembly тул | Done |
| **2** | History summary в Agent2 промпте | Done |
| **2** | CatalogDisplayMeta в кэше Agent2 | Not done |
| **3** | Resolution engine (defaults→deltas→constraints) | Done (базово) |
| **3** | Constraint solver | Done (MaxAtomsPerSize + ValidateDisplay) |
| **3** | Пресеты как saved configs | Partial |
| **3** | Formation diff/patch | Not done |
| **4** | Composite layouts (compose[]) | Not done |
| **4** | Size calculator | Not done |
| **4** | Comparison/Table shapes | Partial (Comparison done) |
| **4** | Расширенный constraint solver | Not done |

### GAPs from Spec: 6 done + 3 partial / 17 total (~44%)

| # | GAP | Статус |
|---|-----|--------|
| 1 | extractFields не видит PIM поля | Done |
| 2 | Микроконтекст не существует | Done |
| 3 | Agent1 state_filter/history | Partial — state_filter done, history_lookup not |
| 4 | History summary для Agent2 | Done |
| 5 | Tool-train | Partial — visual_assembly есть, не полный |
| 6 | Delta-интерфейс полей | Not done |
| 7 | Layout отвязка от пресета | Done |
| 8 | CatalogDisplayMeta | Not done |
| 9 | Данные каталога — нормализация | Not done |
| 10 | Formation deltas (diff/patch) | Not done |
| 11 | Clarification mechanism | Not done |
| 12 | Compose spatial arrangement | Not done |
| 13 | Compose by filter/category | Not done |
| 14 | Conditional styling | Not done |
| 15 | Empty state | Partial — Agent2 handles 0 results |
| 16 | Graceful degradation | Not done |
| 17 | Pagination | Not done |

---

## What Works (Tested)

| Feature | Status | Notes |
|---------|--------|-------|
| `visual_assembly` tool | Works | preset, show, hide, display, order, layout, size, color, direction |
| Defaults Engine | Works | AutoResolve, BuildFieldConfigs, field ranking |
| GenericCardTemplate | Works | Universal card, renders atoms in backend order |
| Grid / List / Single / Carousel | Works | Formation modes |
| ComparisonTemplate | Works | CSS was missing from Shadow DOM, fixed |
| Horizontal cards | Works | `direction: "horizontal"` |
| Size constraints | Works | MaxAtomsPerSize limits atoms per widget size |
| Display validation | Works | ValidateDisplay prevents invalid type/display combos |
| PIM fields | Works | productForm, skinType, concern, keyIngredients extractable |
| History summary | Works | BuildHistorySummary for multi-turn context |
| Microcontext | Works | Pipeline signals Agent2: "new_search", "filtered", "no_data_change" |

## What Has Issues

### 1. state_filter not reliably chosen by Agent1

**Symptom:** "только COSRX" (with data loaded) triggers catalog_search or comparison instead of `_internal_state_filter`.

**Root cause:** Agent1 (LLM) doesn't reliably distinguish "filter subset" from "search new data". Prompt was strengthened (moved filter rules to top, added trigger words), but LLM compliance is probabilistic.

**Fixes to try:**
- Deterministic pre-filter before LLM (regex on "только/лишь/оставь" + loaded_products > 0)
- Tool choice hints from pipeline (force tool when pattern matches)
- Few-shot examples in conversation history

### 2. Agent2 changes layout when not asked

**Symptom:** "покажи бренд красным бейджем" switches from grid to comparison table.

**Root cause:** Agent2 doesn't respect `current_formation` context. Prompt rules added but LLM compliance varies.

**Fixes to try:**
- Strip `layout` parameter from tool definition when microcontext = "no_data_change"
- Post-validate: if user query has no layout keywords, force layout from current config
- Pass `current_formation.mode` as immutable default

### 3. Color — not fully e2e tested

Backend writes `atom.meta.color`, frontend resolves and applies. Works in isolation but test was interrupted by issue #2.

---

## Architecture Assessment

### Solid

- **Atom model** — 6 types, subtypes, display styles, slots
- **Defaults Engine** — field ranking, auto-resolve by entity count
- **Zone-write state** — atomic updates with deltas
- **GenericCardTemplate** — truly generic, no template proliferation
- **Tool-based rendering** — Agent2 calls `visual_assembly`, tool writes formation to state

### Fragile

- **LLM decision quality** — Both agents depend on prompt compliance. #1 risk
- **No validation layer** — Agent2's tool call goes straight to execution
- **Prompt coupling** — Changes to one agent's prompt can break the other

### Not done (out of scope, needed eventually)

| Feature | Complexity | Why it matters |
|---------|-----------|----------------|
| **compose[]** | High | Multi-section layouts |
| **layers / z-index** | High | Overlays, badges on photos |
| **anchor / place** | High | Spatial positioning |
| **CatalogDisplayMeta** | Medium | Per-category display rules |
| **Formation deltas** | Medium | Incremental updates vs full rebuild |
| **history_lookup** | Low | "что я смотрел раньше" |
| **Clarification** | Medium | Agent asks to clarify instead of guessing |

---

## Decision Point (утро 2026-02-23)

Тестируем на широком спектре кейсов. Два исхода:

1. **Приемлемо** — фиксим LLM issues с deterministic guards, продолжаем
2. **Не приемлемо** — убиваем ветку, делаем супер-простую версию без LLM-решений на уровне отображения

---

## Files

### Created (this branch)
- `backend/internal/tools/tool_state_filter.go`
- `backend/internal/tools/tool_visual_assembly.go` (MVP)
- `backend/internal/tools/defaults_engine.go` (MVP)
- `frontend/src/entities/widget/templates/GenericCardTemplate.jsx` (MVP)
- `frontend/src/entities/widget/templates/GenericCardTemplate.css` (MVP)

### Modified (Phase 2)
- `frontend/src/widget.jsx` — ComparisonTemplate CSS in Shadow DOM
- `backend/internal/tools/tool_render_preset.go` — PIM fields in fieldTypeMap + productFieldGetter
- `backend/internal/tools/tool_registry.go` — state_filter registration
- `backend/internal/tools/tool_catalog_search.go` — PIM fields in extractProductFields
- `backend/internal/prompts/prompt_compose_widgets.go` — color, direction, history, microcontext, layout rules
- `backend/internal/prompts/prompt_analyze_query.go` — state_filter rules (priority)
- `backend/internal/usecases/agent2_execute.go` — microcontext, allDeltas
- `backend/internal/usecases/pipeline_execute.go` — microcontext generation
- `frontend/src/entities/atom/AtomRenderer.jsx` — color palette support
- `frontend/src/entities/widget/WidgetRenderer.jsx` — direction prop passthrough
- `frontend/src/entities/widget/templates/GenericCardTemplate.css` — horizontal mode CSS
