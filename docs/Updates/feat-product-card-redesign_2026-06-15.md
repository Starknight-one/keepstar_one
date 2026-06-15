# feat/product-card-redesign — capability layer (2026-06-15)

Branch: `feat/product-card-redesign` (off `main`, in `project_v5`). Not pushed.

## Context

The investor demo needs a pixel-perfect product card in the V5 widget. This
session builds the **capability layer** only: it makes the renderer *able* to
draw a demo-grade card, adds the supporting engine bits, ships a solid
first-pass preset, and lands it green. Exact pixel values are intentionally
left for the main session to tune against the Pencil reference.

Verified going in (recon, re-confirmed against code): nodes are untyped
property bags end-to-end; the engine passes unknown props through verbatim;
the 5-type limit lives only in the widget's NodeRenderer switch; the internal
preview endpoint runs the same Materialise → ApplyOps → ExpandReplicates →
ResolveAndInline → BindData → InjectDefaultActions chain the widget consumes,
so a widget rebuild is enough to see results in the admin preview.

## Capabilities added

### Renderer (frontend) — `src/renderer/decoration.js` (new shared helper)
- **Frame decoration** (`Frame.jsx`): `fill` (solid via `firstSolidColor`),
  `cornerRadius` (number / 4-array / `"full"`), `stroke` ({thickness, fill} →
  border), `effect` outer/inner drop-shadow → `box-shadow`, `clip:true` →
  `overflow:hidden`, `layout.padding` (number / [v,h] / [t,r,b,l]). All flex
  behavior intact; a frame with none of these props carries **no inline style**
  (byte-identical to before).
- **Absolute overlays**: `position:"absolute"` + any of `top/left/right/bottom`
  (numbers→px, strings pass through) + optional `zIndex`, implemented for
  Frame, Rectangle, Text, IconFont. A parent frame with any absolute child
  becomes `position:relative` (overlay anchoring) without overriding an
  explicit position the frame set for itself.
- **Image cornerRadius** (`Image.jsx`): `cornerRadius` number / 4-array → rounds
  the `<img>` (top-corners-only works, e.g. `[16,16,0,0]`).
- **Graceful degradation**: an empty/unbound `Text` renders `null` (no empty
  `<p>` claiming layout space); a frame with `hideWhenEmpty:true` renders
  nothing when it would draw no visible content (the optional badge pill).
- **Button restyle** (`wrapper.js` + `widget.css`): the blue→orange gradient
  default is retired for a neutral dark pill; `node.actionKind` →
  `data-action-kind` drives per-kind styling — `cart_add` = black round 32px
  white "+", `like` = translucent-white round 28px gray heart.
- **14px font token**: `data-fontsize="base"` → `--kw-fs-base:14px`, added
  without disturbing the widely-used `md`=15px.

### Engine (backend) — `internal/engine/inject_actions.go`
- Each injected default button is stamped with `actionKind` ("like" /
  "cart_add") so the widget styles it without re-deriving from `action.kind`.
- **Action-slot routing (backward-compatible)**: a frame carrying
  `acceptsAction:"like"` or `acceptsAction:"cart_add"` receives the matching
  button (heart over the hero, add over the body — different parents). When a
  scope has no typed slots, the legacy single-`actions`-frame fallback fills
  both buttons exactly as before. Routing is per entity scope (replicate clone
  or top-level single-entity fallback); inner frames are never independently
  re-routed; idempotent.

### Preset — `internal/engine/presets/seed/product_card.json` (rewritten)
- `grid` (flex-wrap row) → `card` (replicate, white fill, radius 16, 1px
  `#ECECEC` stroke, soft outer shadow `0,2 blur 12 #00000010`, clip) →
  - `hero` (column) holding the 1:1 image + 3 absolute overlays: a
    `hideWhenEmpty` `#FF9800` badge pill (top-left, bound to optional `badge`),
    a `like-slot` (`acceptsAction:"like"`, top-right), an `add-slot`
    (`acceptsAction:"cart_add"`, bottom-right, bottom:-16 to straddle the edge).
  - `info` (column, padding [10,14,14,14]): `title` (name, 14px/`base`
    semibold, clamp 2) → inline `price-row` (price currency bold + rating
    stars-compact muted) → plain muted `brand` (#999).
- Drops the `brand-badge-root` ref and inlines price/rating (the
  `price-rating-root`/`brand-badge-root` components stay in the registry — the
  list-row preset still uses them).

### No-DB fixture for frontend iteration
- Guarded test `internal/engine/presets/fixture_dump_test.go` runs the REAL
  pipeline on the new seed with 3 realistic fake cosmetics products (Rice
  Cleansing Cream / The Saem / $119 / 4.0 / badge "Bestseller"; Avocado
  Cleansing Cream / The Saem / $99 / 4.3 / **no badge**; AC Ultimate Spot Cream
  / COSRX / $319 / 4.0 / badge "Sale") and writes the processed document — the
  exact shape `window.KeepstarV5Widget.renderDocument(el, doc)` consumes — to
  `frontend/tests/fixtures/product-card-rendered.json`.
- **Regen command** (writes only when the env var is set; normal `go test`
  never touches the tree):
  ```
  cd project_v5/backend && DUMP_PRODUCT_CARD_FIXTURE=1 \
    go test ./internal/engine/presets/ -run TestDumpProductCardFixture -count=1
  ```

## New node props / capability vocabulary (for the main session's pixel-tuning)

Usable on the listed node types in preset JSON (engine passes them through):

| Prop | Node types | Values |
|---|---|---|
| `fill` | frame | solid color (hex string or `{type:"color",color}`) |
| `cornerRadius` | frame, image, rectangle | number, `[tl,tr,br,bl]`, `"full"` |
| `stroke` | frame, rectangle, line | `{thickness, fill}` |
| `effect` | frame | `[{type:"shadow", offset:{x,y}|[x,y], blur, spread?, color, shadowType?:"inner"}]` |
| `clip` | frame | `true` → overflow hidden |
| `layout.padding` | frame | number, `[v,h]`, `[t,r,b,l]` |
| `position:"absolute"` + `top/left/right/bottom` + `zIndex` | frame, rectangle, text, icon_font | numbers→px, strings pass through |
| `hideWhenEmpty` | frame | `true` → render nothing when no visible content |
| `acceptsAction` | frame | `"like"` / `"cart_add"` (action drop-slot) |
| `textStyle.fontSize:"base"` | text | 14px token |

Notes: a parent frame auto-becomes `position:relative` when it has any absolute
child. `actionKind` is engine-stamped on injected buttons, not authored.
`ExpandReplicates` re-mints descendant ids — identify slots by `acceptsAction`,
not by id.

## Action-slot routing design

`InjectDefaultActions` resolves the entity once per scope (replicate clone, or
the top-level single-entity fallback). For that scope it `collectSlots` —
frames with `acceptsAction:"like"|"cart_add"` (closed vocabulary, typos
ignored). If any slot exists → SLOT mode: each default button goes into its
matching empty slot, the legacy `actions` frame is left untouched. If no slot
exists → LEGACY mode: both buttons fill the empty `actions` frame (unchanged).
`walkScope` stops at nested clones so each clone routes against its own
`dataIndex`. Already-populated slots/frames are skipped (idempotent, respects
LLM-authored actions).

## Files

Backend:
- `internal/engine/inject_actions.go` (rewritten — actionKind + slot routing)
- `internal/engine/inject_actions_test.go` (+3 tests: slot routing, slot
  idempotency, actionKind stamp)
- `internal/engine/presets/seed/product_card.json` (rewritten)
- `internal/engine/presets/presets.go` (doc comments)
- `internal/engine/presets/presets_test.go` (round-trip rewritten to new
  intent + new full-pipeline render test)
- `internal/engine/presets/fixture_dump_test.go` (new, guarded)

Frontend:
- `src/renderer/decoration.js` (new shared helper)
- `src/renderer/nodes/Frame.jsx` (decoration + position + hideWhenEmpty)
- `src/renderer/nodes/Image.jsx` (cornerRadius + position)
- `src/renderer/nodes/Rectangle.jsx` (position)
- `src/renderer/nodes/IconFont.jsx` (position)
- `src/renderer/nodes/Text.jsx` (position + empty→null)
- `src/renderer/wrapper.js` (actionKind → data-action-kind)
- `src/widget.css` (button restyle, base font token, action-kind variants)
- `tests/frame-decoration.test.jsx` (new — 15 tests)
- `tests/fixtures/product-card-rendered.json` (generated fixture)

## Verification (asserted output)

Backend — `go vet ./...` clean; `go test ./... -count=1` all green:
```
ok  keepstar_v5/internal/engine          (incl. InjectDefaultActions slot routing + actionKind)
ok  keepstar_v5/internal/engine/presets  (product_card round-trip + full-pipeline render + fixture dump)
ok  keepstar_v5/internal/domain
ok  keepstar_v5/internal/handlers
ok  keepstar_v5/internal/prompts          (Agent2 prompt-cache budget test intact)
ok  keepstar_v5/internal/tools
ok  keepstar_v5/internal/usecases
ok  keepstar_v5/internal/adapters/anthropic, .../postgres
```
Asserted: the redesigned product_card fans out to N bound cards, like→like-slot
and cart_add→add-slot are populated with correctly-stamped buttons, no legacy
`actions` frame is created, and the optional missing `badge` is not a fatal
bind error. All pre-existing canvas/vocab contract tests stay green (other
presets unaffected).

Frontend — `npm test -- --run`: **83 passed (11 files)**; `npm run build` green.
```
dist/widget.js  235.86 kB │ gzip: 73.51 kB
```
Asserted (one-shot render of the generated fixture through the real
SceneGraphRenderer): 3 hero images, exactly 2 orange badge pills (clone 1's
empty badge hidden), 3 `data-action-kind="like"` + 3 `data-action-kind="cart_add"`
buttons, and bound/formatted title + `$119.00` + `★ 4.0`.

## Left for visual pixel-tuning by the main session

- Exact paddings, gaps, radii, shadow blur/opacity, badge offsets, slot
  offsets (esp. the add-slot bottom:-16 straddle), font sizes/weights/colors
  against the Pencil reference.
- Hover/press states for the like / add buttons (only base styling shipped).
- Title line-height / clamp tuning; brand color/size; price/rating type scale.
- Whether the hero image needs its own top-corner rounding vs relying on the
  card `clip:true` + radius (both paths are supported).
- Image aspect ratio / object-fit for the demo imagery; real demo image URLs
  (the fixture uses picsum placeholders).
- The `like` heart glyph is the default "♥"; swap for an outline/lucide icon
  if the reference wants one.

## Known gaps / notes

- `ExpandReplicates` re-mints all descendant ids except the reserved `actions`
  id, so the `like-slot`/`add-slot` ids are not stable post-fan-out; routing
  and tests key off the `acceptsAction` attribute, which is stable.
- Empty-`Text`→null is a small global behavior change (no test relied on an
  empty `<p>`); it only affects genuinely-empty bound text since literals
  always carry non-empty content.
- No backend changes were needed for the new visual props beyond the action
  layer — the engine treats them as opaque pass-through.
