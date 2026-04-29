# KeepstarCanvas — Implementation Plan

> Status: ✅ **DONE 2026-04-20** — Phases 1-8 all shipped: `4ed581c` (P1 preset CRUD backend), `de81801` (P2 tenant preset loader), `5d561ea` (P3 design context in Agent2 prompt), `281e582` (P4 canvas UI shell), `0b7c314` (P5 tldraw + preset tiles), `4c6ea5c` (P6 inspector editing + publish/delete), `12c7bba` (P7 components library + save-from-trace), `b89bd9c` (P8 design tokens editor + themes).
> Original status: approved 2026-04-12.
> Mockup: scratch `.pen` file, frame `KeepstarCanvas` (1440×900, Soft Bento Clinical dark theme).
> Canonical plan file (for claude sessions): `/Users/starknight/.claude/plans/mutable-launching-fiddle.md` — this project doc is the survivable mirror.

## Context

V4 engine is ops-driven but presets are hardcoded Go builders (`presets_product.go`, etc.) living in a global `init()` registry. This blocks three things the product roadmap needs right now:

1. **Per-tenant presets** — different tenants want different card layouts, but currently all tenants share 12 cosmetics-biased builders.
2. **Instant design** — tenants can't self-serve; every preset change is a Go PR + deploy.
3. **Control** — tenants can't see or override what Agent2 produces. Insights + fixes happen in Vlad's head, not the tenant's.

**Mental model**: KeepstarCanvas is a Pencil-clone that lives inside tenant admin. 3 panels — Agents chat (left), infinite canvas with preset/component tiles (center), Inspector (right). It generates/edits presets that Agent2 uses at render time. Ops are already JSON-serializable; the Agent2 system prompt is already assembled per-request; pipeline traces already persist the raw ops blob. The groundwork is there — this is 80% wiring, 20% new UI.

## Approach at a glance

1. **Presets move to DB**, scoped by tenant, with **draft/published versioning**. Global Go registry stays as seed/fallback.
2. **V4 engine stays unchanged** — `ExpandInlinePresets` already namespaces per-widget, `BindData` already uses `field_definitions`, `Op` is already JSON-clean. We only add a **tenant-aware preset loader** that runs before `ApplyOps`.
3. **Agent2 prompt gains a generative `<tenant_design_context>` block** — appended in the existing `buildSystemPromptWithFields` assembly path. Cache boundary = tenant, matching the existing `fieldsPromptCache` pattern.
4. **Save-from-trace** reads `pipeline_traces.trace_data.agent2.toolInput`, parses into `[]Op`, wraps as a draft preset. Zero reverse engineering.
5. **Canvas UI**: new `features/canvas/` in admin frontend, uses **tldraw v3** as canvas infrastructure, renders preset tiles as custom tldraw shapes backed by the real formation renderer.
6. **Components as second library level** — small reusable atoms (`price_pill`, `rating_stars`) that presets can reference. Matches Pencil's `reusable + ref + descendants` model.

Ship in **8 phases**, each independently mergeable and verifiable on prod. Phases 1–3 unblock the "instant design" story even before the canvas UI exists, via a flat JSON editor.

---

## Critical files and integration points

### V4 chat backend (`project_v4/backend/`)

- `internal/engine_v4/types.go:17-24` — `Op` struct. Already DB-ready. No change.
- `internal/engine_v4/presets.go:22` — global `registry = map[string]*Preset{}`. Becomes seed/fallback; new loader wraps it.
- `internal/engine_v4/presets.go:78-176` — `ExpandInlinePresets` with `p0_`/`p1_` ref namespacing. No change — same mechanism powers tenant presets.
- `internal/tools/tool_visual_assembly.go:173-219` — where `preset` param resolves via `GetPreset`. **Change point**: swap `GetPreset` for `tenantPresetLoader.Get(ctx, tenantID, name)` with fallback to global.
- `internal/usecases/agent2_execute.go:429-454` — `buildSystemPromptWithFields`. **Change point**: append `<tenant_design_context>` block after existing `<fields>` block, same per-tenant memoization.
- `internal/engine_v4/default_ops.go:11-24` — `ProductCardGridOps`/`ProductDetailOps` thin wrappers. No change; continue to route through registry (falls through to global for system presets like `product_card`).
- `internal/engine_v4/engine.go:19-110` — execute pipeline. No change.
- `internal/engine_v4/binding.go` — `BindData` uses atom `FieldName` → data map. Already tenant-agnostic via `field_definitions`. No change.

### Shared Postgres (both backends hit same DB)

- `project_v4/backend/internal/adapters/postgres/postgres_trace.go` — source of `pipeline_traces` table. Already stores `trace_data JSONB` with `agent2.toolInput` as JSON string.
- `project_admin/backend/internal/adapters/postgres/admin_migrations.go` — where new tables get registered.
- New tables (admin schema): `admin.tenant_presets`, `admin.tenant_components`, `admin.tenant_design_tokens`, `admin.tenant_preset_versions`.

### Admin backend (`project_admin/backend/`)

- `cmd/server/main.go:89-110` — where new `canvasUC`/`canvasHandler` get wired.
- `internal/middleware/middleware_auth.go:22-58` — existing JWT → `tid` context. Canvas endpoints reuse it verbatim.
- New files, copying the `products` module layout:
  - `internal/handlers/handler_canvas.go`
  - `internal/usecases/canvas.go`
  - `internal/adapters/postgres/canvas_adapter.go`
  - `internal/domain/canvas.go`
  - `internal/ports/canvas_port.go`

### Admin frontend (`project_admin/frontend/`)

- `src/App.jsx:24-51` — add `<Route path="canvas" element={<CanvasPage />} />`.
- `src/features/layout/DashboardLayout.jsx:19-37` — add sidebar link.
- `src/shared/api/apiClient.js` — reuse as-is (Bearer token already tenant-scoped).
- New module: `src/features/canvas/` — see Phase 4 below.
- `package.json` — add `tldraw@^3` dep.

---

## Phased delivery

### Phase 1 — DB schema + canvas domain backend (admin)

Create the storage layer and CRUD endpoints. No UI yet.

- Migrations in `admin_migrations.go`:
  - `admin.tenant_presets` — `id, tenant_id, name, category, default_replicate, entity_type, description, latest_version_id, created_at, updated_at`
  - `admin.tenant_preset_versions` — `id, preset_id, version, status ('draft'|'published'), ops_json JSONB, author_user_id, published_at, created_at`. Ops are immutable per version; new edits fork a new draft.
  - `admin.tenant_components` — same shape as presets (reusable fragments referenced by presets).
  - `admin.tenant_design_tokens` — `id, tenant_id, category, name, value, theme_axis, theme_value, updated_at`.
- Adapter + usecase + handler following the `products` module pattern.
- Endpoints: `GET/POST/PUT/DELETE /admin/api/canvas/presets[/:id]`, `POST /presets/:id/publish`, `POST /presets/:id/fork`, mirrors for `components` and `tokens`.
- Middleware unchanged — tenant comes from JWT.

**Verification**: curl CRUD for a minimal preset, confirm tenant isolation with a second tenant JWT.

### Phase 2 — Tenant-aware preset loader in V4 chat backend

Wire the DB presets into Agent2's tool.

- New port `TenantPresetLoader` in `project_v4/backend/internal/ports/`.
- New adapter reading from the same `admin.tenant_presets` + `_versions` tables (latest published version only).
- In `tool_visual_assembly.go:214`, replace the direct `GetPreset(name)` with a two-tier lookup: **(1) tenant-scoped published preset**, fallback **(2) global registry**. Cache (tenantID, name) → parsed `[]Op` with 60s TTL; invalidate on publish via a pg `NOTIFY` or a poll.
- Thread `tenantID` into `VisualAssemblyInput` — it already rides on the session, just needs to reach `tool_visual_assembly.Execute`.

**Verification**: seed one tenant with a copy of `product_card` and a tweaked variant; confirm production chat uses it end-to-end. Global registry still serves `empty_not_found`/`error_generic` system presets.

### Phase 3 — Generative `<tenant_design_context>` block in Agent2 prompt

Tell Agent2 what the tenant has.

- Extend `buildSystemPromptWithFields` in `agent2_execute.go:429-454`:
  - Fetch tenant presets + components + tokens from the new loader.
  - Format as a structured block: preset names + short descriptions + a compact ops summary per preset (not full JSON — think "1 hero image, 1 title bound to `name`, 1 price bound to `price`").
  - Append after the existing `<fields>` block.
- Memoize per (tenantID, design_context_version) — bump version on any publish. Matches existing `fieldsPromptCache` discipline so Anthropic prompt caching stays warm.
- Prompt language stays English — all generated text in English. (Feedback memory: `feedback_prompt_language.md`.)

**Verification**: inspect an Agent2 request/response in `/debug/traces/` — confirm the block is present, tokens look sane, cache boundary holds.

### Phase 4 — Admin canvas UI shell + routing

Static 3-panel shell, no tldraw yet. Matches the Pencil mockup exactly.

- `features/canvas/CanvasPage.jsx` — 3-panel flex, top bar with draft/publish status.
- `features/canvas/LeftPanel/` — brief editor, Insights feed (empty for now), chat composer (wire later).
- `features/canvas/CenterPanel/` — empty canvas placeholder + preset list on top (temporarily a flat grid).
- `features/canvas/RightPanel/Inspector.jsx` — Properties / Design System / Data tabs (stubs).
- Load presets via `useCanvasApi()` hook copied from the `catalog` module's API pattern.
- CSS in `canvas.css`, uses existing `--color-*` vars.

**Verification**: admin navigates to `/canvas`, sees the shell, loads presets from Phase 1, can create a new empty draft.

### Phase 5 — tldraw integration + preset tiles as custom shapes

- Install `tldraw@^3`, mount `<Tldraw>` in CenterPanel.
- Custom shape `PresetTileShape` — wraps an existing `FormationRenderer` (already in admin shared/replay for trace playback) so tiles render with **real data**.
- Each preset is one tile; multiple presets sit on the canvas; components sit in a separate group.
- Drag/select/pan from tldraw. Click → selects preset → Inspector opens its props.
- L3 interactivity: click on an atom inside a tile → select atom → Inspector scopes to atom.
- Canvas state persists to localStorage per tenant (positions, zoom).

**Verification**: open admin canvas on seeded data, see live product cards rendered with bound mock data from `/debug` samples, select atoms inside them.

### Phase 6 — Inspector editing + optimistic ops

The editor becomes real — Inspector mutations generate ops that patch the current draft version.

- Property controls for Layout (mode, columns, gap), Atoms (show/hide, bind field, text style, wrapper, format), Theme (swatch picker bound to design tokens).
- Every edit → produces 1+ `Op`, applied optimistically to the in-memory preset and re-rendered on the tile; persisted via `PUT /canvas/presets/:id` as a new draft revision.
- Draft → Publish button in top bar runs a backend validation (parse ops through `ExpandInlinePresets` headless + `ApplyConstraints`) then flips the version to `published`.
- Undo/redo via tldraw's built-in history + inspector-sourced commands through a command queue.

**Verification**: edit a preset in admin, publish, open the chat widget for that tenant, confirm the new layout shows up on the next turn.

### Phase 7 — Components library + save-from-trace

- Components panel at the bottom of the center canvas. Designer drags a component from the library onto a preset atom slot → Inspector references the component by ID.
- Component editing: click a component tile → enter component-edit mode (tldraw page swap) → edits propagate to all preset references via `ExpandInlinePresets` `$ref` resolution (already implemented).
- **Save-from-trace**: new endpoint `POST /admin/api/canvas/capture` — body `{trace_id}`. Backend loads `pipeline_traces.trace_data.agent2.toolInput`, extracts ops, wraps as a new **draft** preset named "From session `<short_id>`". Admin's session viewer gets a "Save as preset" button on the Agent2 step.
- Tenant-scoped trace read (the existing admin trace adapter is NOT tenant-filtered — add the join: `JOIN chat_sessions cs ON pt.session_id = cs.id WHERE cs.tenant_id = $1`).

**Verification**: tenant impersonates a real session in admin, finds an Agent2 turn they like, clicks "Save as preset", sees the new draft on the canvas.

### Phase 8 — Themes + design tokens switcher

- Design tokens stored per tenant (Phase 1 table) with optional `theme_axis`/`theme_value` pair, matching Pencil's theme axes.
- Inspector → Design System tab shows the 3 example themes from the mockup (Default / Halloween / Pride) as theme axes.
- Switching a theme: bumps the tokens returned by the tenant design context loader, which invalidates the Agent2 prompt cache and the frontend re-renders tiles.
- Live preview in canvas before publish; tenant publishes a specific theme as default.

**Verification**: switch Halloween on → canvas repaints to orange accents → publish → next chat session renders with Halloween tokens.

---

## Guardrails and traps

- **Do not mutate the global preset registry at runtime.** Treat it as read-only seed. Race conditions are real.
- **Forbid editing published preset versions.** Any edit on a published version forks a new draft. Keeps old sessions reproducible.
- **Field binding requires `FieldName`.** Inspector must force field selection when inserting an atom that should bind to data; freestyle atoms are opt-in only.
- **Cache invalidation is critical.** Anthropic prompt caching is keyed on the tenant design context block — any token/preset change must bump a version that cache-busts *exactly that tenant* and no others. Use a per-tenant version counter in the loader.
- **Admin trace reads must be tenant-filtered.** Current admin trace adapter sees ALL traces — add the `chat_sessions` join before exposing Save-from-trace, or we leak cross-tenant data.
- **Runtime IDs are not stable across sessions.** Ops captured from a trace reference instance IDs that don't persist. The capture endpoint must rewrite IDs into pattern-form (replace stamped `w0/w1/...` with the preset's canonical `w`/`root` refs) before saving. This is the same transform `ExpandInlinePresets` already does in reverse — we can mirror it.
- **Prompt language stays English.** All generated descriptions, insights, and UI strings passed to the LLM are English (feedback memory rule).
- **Legacy V1/V2 chat backend (`project/backend/`) is frozen** — only V4 gets changes. Do not touch it.

---

## Verification (end-to-end)

After Phase 6 (minimum shippable slice):

1. `scripts/start_all.sh` — chat backend (V4), admin backend, admin frontend.
2. Seed tenant `heybabescosmetics` with one custom `product_card_v2` in admin.
3. Publish it.
4. Open chat widget for that tenant → "покажи крема" → confirm V4 agent picks up the tenant preset (check `/debug/traces/` — Agent2 system prompt contains `<tenant_design_context>`, tool call uses the new preset name).
5. Open admin canvas → edit `product_card_v2` → publish → reload chat → new layout visible.
6. Confirm a second tenant does not see `product_card_v2` (isolation check).
7. Confirm `product_card` still works for tenants that have no custom preset (fallback check).

After Phase 7:

8. In admin traces, pick a real Agent2 turn, click "Save as preset" → confirm new draft appears with expected atoms.

After Phase 8:

9. Switch theme to Halloween in admin → publish → confirm chat renders with orange accents; switch back → confirm revert.

---

## Out of scope (explicit non-goals for this plan)

- Real-time multi-user editing of the same preset (single-author for now, draft lock).
- Importing .pen files directly.
- Exporting presets as code for self-hosted tenants.
- AI-generated previews of random presets (Agents chat is scoped to optimize/edit, not "generate from scratch" in Phase 1 — that's a follow-up).
- Cross-tenant component sharing.

---

## Answered design questions (context for future sessions)

Collected during planning so a fresh session can pick up without re-deriving:

| Question | Answer |
|----------|--------|
| Canvas unit | One widget = one unit on canvas |
| Interactivity level | L3 (full drag-n-drop) from day 1 |
| Workflow | Draft → Published, manual publish button |
| Pencil guidelines | Reuse them (dark Soft Bento Clinical + Deep Space Neon palette for mockup) |
| Preset source for save-from-trace | Real production sessions, admin-mode on the real widget |
| Descriptions | Auto-gen with manual editing |
| Components | Separate entity (Variant A) — reusable, referenced by presets |
| Canvas infrastructure | tldraw v3 |
| Tenant scope | Everything per-tenant: presets, components, tokens, Agent2 system prompt segment |

## Exploration findings reference (from Phase 1 of planning)

**V4 engine feasibility**: all infrastructure is ready. `Op` struct at `engine_v4/types.go:17-24` is JSON-native. Preset merge happens in `tool_visual_assembly.go:214` via `presetOps := p.Build(); engineInput.Ops = append(presetOps, engineInput.Ops...)`. Engine pipeline is untouched. Agent2 system prompt is already assembled per-request in `agent2_execute.go:429-454`, so appending a tenant block is additive. `default_ops.go` wrappers already route through the registry — zero callers break.

**Admin backend**: JWT middleware at `middleware_auth.go:22-58` extracts `tid` claim → `TenantID(ctx)`. Copy the `products` module layout for new canvas module. Migrations are in-code at `admin_migrations.go`. Same Postgres as chat backend — admin reads `pipeline_traces.trace_data` directly. **Trap**: current admin trace adapter is NOT tenant-filtered — add `JOIN chat_sessions cs ON pt.session_id = cs.id WHERE cs.tenant_id = $1` before exposing Save-from-trace.

**Admin frontend**: React 19 + Vite + React Router v7 + FSD-lite. No TypeScript. Shared fetch wrapper at `shared/api/apiClient.js`. CSS vars at `index.css:9-22`. No existing canvas or drag libs. tldraw v3 is compatible. Add `/canvas` route in `App.jsx:24-51` and nav link in `DashboardLayout.jsx:19-37`.

**Save-from-trace readiness**: 80% done. `trace.Agent2.ToolInput` is already a JSON string containing the ops array. Parse → rewrite stamped IDs to pattern-form → store as draft preset version. No reverse engineering.
