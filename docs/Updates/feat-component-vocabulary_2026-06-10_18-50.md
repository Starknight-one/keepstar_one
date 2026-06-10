# Component vocabulary PR1 — new node types + composite presets

**Branch:** `feat/component-vocabulary` (ultra) + `feat/canvas-new-node-types` (admin, translate passthrough only)
**Context:** C1-parity track, item 5.1 of `../V5_VS_C1_PARITY.md` — close the widest visible gap vs Thesys C1 (their ~30-component library vs our 5 renderable node types). Direction pre-blessed by `docs/v5-tool-format-decision.md`: growing preset coverage is the 12.7× token lever vs freestyle ops.

## What changed

### Widget renderer (frontend) — 3 new node types + 2 frame substrates
- `Rectangle.jsx` — decorative color block: `fills` (solid, array convention) → background, `cornerRadius` (number or 4-array), `stroke` → border, sizing, opacity.
- `Line.jsx` — divider; thickness/color from `stroke`; vertical when `height > width` (deliberate divergence from v9's bbox-diagonal semantics, noted in-code).
- `IconFont.jsx` + `icons.jsx` — inline-SVG lucide registry, 24 curated commerce icons, path data fetched verbatim from lucide-static v1.17.0 (none hand-written). Unknown name → warn + render nothing. Zero new npm deps.
- `fills.js` — shared `firstSolidColor()` accepting hex string | `{type:'color',color}` | arrays.
- `Frame.jsx` — `layout.scroll:"x"` → `data-scroll` scroll-strip (carousel substrate; scroll-snap CSS); `collapsible:true` → native `<details>/<summary>` accordion substrate, zero JS state, first child = header, `defaultOpen` supported.
- `format.js` — array bindings (e.g. `tags`) now render as `"a, b"` instead of `"a,b"`.

### Engine/backend (ultra) — 3 composite system presets, zero engine-code changes
Engine node pipeline needed **no behavior changes** — nodes are untyped property bags; the 5-type limit lived only in the widget switch.
- `seed/product_carousel.json` — horizontal scroll strip of replicated cards (adjacency → `product_detail`).
- `seed/product_comparison.json` — side-by-side replicated columns with line dividers (adjacency → `product_detail`).
- `seed/product_detail_accordion.json` — detail view with collapsible Description/Details sections (literal summaries survive: seeds are hand-authored, BindData leaves non-bound atoms alone).
- `presets.go` / `prompt.go` / `adjacency.go` — registrations, prompt descriptions (auto-injected into the Agent2 catalog), drill edges.
- `agent2_prompt.go` — `props.type` vocabulary extended with `rectangle, line, icon_font`; one ~80-token paragraph documenting the new leaves + frame substrates + a guard sentence ("never set collapsible on a replicated card — kills tap-to-detail"). **Busts the Anthropic prompt cache once per tenant**; prompt only grows, so the ≥4500-token cache-stability floor is untouched (`TestAgent2SystemPromptCacheBudget` green).

### Admin — translate passthrough
- `translate.go` — `rectangle` (plain) / `line` / `icon_font` now survive translation (previously dropped with warnings): solid fill (canonical **array** form `fills:["#hex"]`, same convention as image atoms), cornerRadius, stroke, sizing, iconFontName. Image-filled-rectangle → image atom branch unchanged. Rectangle drop-warning removed from `finish()`.
- Frames still drop decoration (fill/padding/cornerRadius) — that is Wave 2.5 branding-slice scope, not PR1.

## Out of scope (deliberate, documented)
- **Input/Select/Checkbox** — do NOT exist in the v9 schema either (recon corrected the parity doc's claim); they are net-new schema + the form_submit action loop → parity track item 5.3, separate step.
- **Tabs** — needs client-side state; deferred with forms.
- **ellipse/polygon/path** — low commerce value, still dropped by translate (with warnings).

## Verification (all fresh, -count=1 / no cache)
- ultra backend: `go vet ./...` clean; `go test ./...` all packages ok — including new `vocabulary_contract_test.go` which runs each new preset through the full zero-LLM chain (Materialise → ExpandReplicates → ResolveAndInline → BindData → InjectDefaultActions) and asserts fan-out counts, per-clone dataIndex, bound text equality vs fake data, scroll/collapsible attr survival, injected action entity ids; plus `testdata/canvas_translate_card_newtypes.json` cross-repo fixture.
- admin backend: `go vet` clean; `go test ./internal/canvas/...` ok (12 translate tests incl. 5 new: exact key sets, fill variants, gradient→no-fill, unique ids, no stale drop-warnings).
- frontend: vitest 53/53 (10 new node-type tests asserting rendered inline styles, svg path presence, details/summary structure); `npm run build` ok; `dist/widget.js` 228,431 bytes (~71.3 KB gzip), +8.5 KB vs 219,924 baseline; bundle greps confirm `kw-rect`/`iconFontName` present.
- Adversarial review (2 lenses) found 1 major — admin emitted `fills` as bare string while the ultra fixture/prompt teach the array form (cross-repo drift the fixture exists to prevent) — **fixed**: admin now emits `[]any{c}`, tests updated both sides, re-verified green. 3 minors fixed: collapsible body forwards `data-scroll`, empty collapsible body not rendered, collapsible×replicate clone now warns + prompt guard sentence. Known accepted nits: `tags` binding absent on tag-less products (atom stays empty inside a collapsed section); `<p>` inside `<summary>` violates the strict content model (browsers tolerate); height-only lines render horizontal (vertical needs both width+height).

## Known follow-ups
- Live smoke on deployed v5: ask for "compare X and Y" / "show me more like this" and confirm Agent2 actually picks `product_comparison`/`product_carousel`; deterministic check via `POST /api/v1/internal/presets/preview?tenant=…` for all 3 new presets once deployed.
- Prompt-cache bust is one-time per tenant on deploy (prompt edit) — expected cost blip.
- ARCHITECTURE.md renderer section updated; `docs/v5-known-gaps.md` untouched (no A-gap closed/opened by PR1).
