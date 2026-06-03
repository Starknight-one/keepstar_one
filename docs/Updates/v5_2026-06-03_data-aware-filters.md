# V5 — Data-aware filter panel (2026-06-03)

Deterministic, no-LLM filter layer that sits BESIDE the generative layout (not inside Agent2).
Spec + roadmap: `docs/V5_INTERACTIVITY_SPEC.md`.

## What shipped
- **Facets derived from the displayed products** (`usecases/facets.go`): introspects each product's
  flattened attribute map (typed + `tier2` + `extra`, via `engine.ProductToMap`) and infers a filter
  dimension per attribute — vertical-agnostic (furniture surfaces material/style, cosmetics surfaces
  skin_type/concern, same code). Numeric → `range`; low-cardinality string/array → `enum` with counts.
  Quality gate: drop identifiers/free-text (`maxLen>40`) and columns that don't group (no value ≥2),
  require ≥2 distinct + ≥50% coverage.
- **Refine** (in-memory, narrows): enum multi-select (OR within a facet) + numeric range, multi-facet
  compose (AND across facets) + reset. `current_data` is never mutated — the base set is the reset.
- **Guided faceting**: each facet's value set is recomputed over the subset matching every OTHER active
  filter (exclude-own), so a filtered facet still shows its options while the rest narrow.
- **Sort**: Relevance / Price ↑ / Price ↓ / Top rated (in-memory sorted copy, then re-render).
- **Panel**: collapsible drawer in the chat column, open by default. Facets ship in the same pipeline
  response as the grid (immediate). Filter/sort clicks hit the deterministic
  `POST /api/v1/navigation/filter` (re-filters + re-renders via the nav handler) — **zero LLM cost**.

## Files
- Backend: `usecases/facets.go` (+ `facets_test.go`), `usecases/pipeline_execute.go` (facets in
  response), `handlers/handler_pipeline.go` (facets field), `handlers/handler_navigation.go`
  (`/navigation/filter` generalised to a typed filter set + sort; `materialiseGrid`, `sortProducts`),
  `handlers/routes.go` (route).
- Frontend: `chat/FilterPanel.jsx` (new — typed controls + sort + debounced range), `chat/ChatPanel.jsx`,
  `WidgetApp.jsx`, `api/actions.js` (`filterApply`), `widget.css`.

## Verification (live, not just unit)
Local stack against `pim-furniture-demo`:
- Browser: "sofas under $300" → 12 cards + clean open filter panel, no console errors.
- curl: price ≤ $200 → **12 → 6 cards**; guided facets correct (price stays full-range [exclude-own],
  brand 11→6, etc.). Sort: relevance `[299,189,…]` → desc `[299,294,…,126]` / asc `[126,…,299]` (both
  ordered). Browser confirmed grid re-orders on "Price ↓".
- `go build`/`vet` clean, `facets_test.go` green, `npm run build` clean.

## Design decision — Expand dropped
Explored Expand (apply a filter that pulls MORE from the catalog, overwriting current_data). **Dropped:**
filters must NARROW — "apply a filter → expect fewer, not more". Expand only makes sense as a separate
explicit "search wider" action, never as filter behaviour. The catalog code was read (search supports
brand/category/price + a hardcoded tier2 set; generic tier2 filtering would be the B1 unblocker) but
nothing in the catalog/handler was changed; the one started field (`ProductFilter.Tier2`) was reverted.

## Parked (see spec)
v1c freestyle surgical re-render (filter without a grid preset); input/forms, likes/cart state stamps,
nav graph, resource buttons.
