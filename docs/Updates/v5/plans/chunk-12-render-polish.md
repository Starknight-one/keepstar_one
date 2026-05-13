# V5 — Chunk 12: Render-quality polish

## Context

Chunks 9–11 closed the entire P0 surface (tool shapes, frontend renderer,
interaction loop). End-to-end works, but **визуально карточки сломаны**.
Vlad открыл V5 widget локально и зафиксировал четыре пересекающихся
гэпа (`docs/v5-known-gaps.md` → секция «Render-quality gaps surfaced by
first manual test (2026-05-03)»). Каждый по отдельности всё равно даёт
сломанный визуал — починить надо все четыре одним когерентным чанком.

После предпланового исследования (см. диагностику в чате):

- **Q1 (стек≠сетка)**: `Frame.jsx` не читает `layout.wrap`, `widget.css`
  не имеет селектора `[data-wrap]`, корневой `.kw-display-inner` —
  жёсткий `flex-direction: column`. Реплицируемые клоны падают
  параллельно в `Document.Children[]` без grid-обёртки. V9 этот режим
  знает (frame с `direction:row + wrap:true`), мы недотащили рендер.
- **Q2 (priceFormatted ломается)**: `binding_to_map.go:43-45` пишет
  `priceFormatted` только если `p.PriceFormatted != ""`. Hey-babes
  каталог хранит `Price` как число и не заполняет `PriceFormatted` —
  биндинг падает молча в пустую строку. `heroImage` работает (там
  `m["heroImage"] = p.Images[0]` без условия). Tier2/Extra fields
  вообще-то работают «бесплатно» через spread в ProductToMap; реальный
  гэп — только price.
- **Q3 (нет size на карточках)**: `Frame.jsx:14-27` не читает
  `node.width / maxWidth / minWidth`. V9/Pencil умеет
  (`engine/value_position.go` парсит `Sizing`), но Node-уровневая
  поддержка не была дотащена в рендер. Атомный enum `size` (V4-style)
  не нужен — width/maxWidth + grid-columns на родителе закроют usecase.
- **Q4 (modify-bias)**: V4 имел REQUIRED `mode: rebuild|modify` enum на
  схеме `visual_assembly`; LLM пикала каждый ход. V5 этот параметр
  потерял в чанке 9 — теперь decision implicit «есть Template → modify».
  Когда юзер на 2-м ходе говорит «сделай лендинг», Agent2 эмитит ops
  без preset, V5 видит non-empty Template → автоматически modify, и
  лендинг получается салатом из старого grid + новых ops.

Все четыре фикса — точечные правки в одном слое (seeds + tool schema +
prompt + renderer).

## Locked-in decisions (после диагностики и обсуждения)

- **`mode` параметр возвращается на schema как REQUIRED** (Q4). Mirrors
  V4 contract. На отсутствие — `IsError` ToolResult с понятным текстом
  «mode required: rebuild|modify». LLM пикает каждый ход.
- **`mode=rebuild`** → engine стартует с пустого `engine.NewDocument()`
  (или с preset.Materialise) **независимо** от `state.Current.Template`.
  Это фикс modify-bias: LLM может явно сказать «забудь что было».
- **`mode=modify`** → текущее поведение (load Template + applyOps на
  существующий tree).
- **Серверный price formatter в `ProductToMap`** (Q2): порт V4
  `formatPrice(kopecks int, currency string) string` из
  `project_v4/.../postgres_catalog.go:547-572`. Если
  `p.PriceFormatted == "" && p.Price > 0` → форматнём
  `formatPrice(p.Price, p.Currency)`. Никакого frontend-formatter'a.
- **`Frame.jsx` дотягивается до полного flex-vocabulary**: `wrap`,
  `width`, `maxWidth`, `minWidth` (Q1 + Q3). Width-значения парсим как
  числа → `<n>px` или строки с unit pass-through. Без полного V9
  `Sizing` (Fixed/Fill/Fit) — это когда понадобится canvas microservice.
- **Card seeds оборачиваются в grid-frame**: каждая product_card*
  получает корневой `{type:"frame", id:"grid", layout:{direction:"row",
  wrap:true, gap:"md"}, children:[<существующая card-frame>]}`. Сама
  card-frame получает `width: 280` чтобы flex-wrap её корректно
  переносила. detail-варианты не трогаем — они одиночные.
- **CSS-side**: добавляем `.kw-frame[data-wrap='true'] { flex-wrap:
  wrap; }` + поддержку `data-width`/`data-maxwidth` через inline style
  (не через CSS, чтобы не плодить enum-классы для каждого пиксельного
  значения).

## Approach

### Backend — Part A: `mode` parameter

`internal/tools/tool_visual_assembly.go`:

1. Add to `Definition().InputSchema.properties`:
   ```go
   "mode": map[string]interface{}{
       "type":        "string",
       "enum":        []string{"rebuild", "modify"},
       "description": "REQUIRED. \"rebuild\" — discard the previous Document and build fresh from preset/ops (use when the user asks for new content, a different layout, drill-down, or after a search). \"modify\" — load the current Document and apply ops as deltas targeting ids from tree_map (cosmetic / structural tweaks on what's on screen). When in doubt: «keep what's on screen but ...» = modify; «show me ...» / «render ...» = rebuild.",
   },
   ```
2. Add `mode` to `required` array (alongside the implicit preset/ops
   either-or check we already have).
3. In `Execute`:
   - Read `mode, _ := input["mode"].(string)`. If
     `mode != "rebuild" && mode != "modify"` → `errorResult("mode required: rebuild|modify")`.
   - Replace the current implicit branch:
     - `mode == "rebuild" && presetName != ""` → load preset + components → Materialise (как сейчас).
     - `mode == "rebuild" && presetName == ""` → `merged = engine.NewDocument()` (freestyle build).
     - `mode == "modify"` → load `state.Current.Template`. Если пустой
       → `errorResult("modify requires a current Document; pick rebuild for the first turn")`.
4. Update slog summary `mode=` value to use the explicit param (we
   already log mode; just remove the auto-detection).

### Backend — Part B: server-side price formatter

`internal/engine/binding_to_map.go`:

1. Port `formatPrice` from V4. Place it as private helper at the
   bottom of the file:
   ```go
   func formatPrice(price int, currency string) string {
       rubles := price / 100   // V4 stores kopecks; verify V5 does too
       // → thousand-separator + currency symbol switch
   }
   ```
2. **Verify the price unit before deciding the divisor**. Read
   `internal/adapters/postgres/postgres_catalog.go` ListProducts +
   schema migrations to confirm whether `products.price` is kopecks
   (V4 contract) or whole units. If whole units, drop the `/100`.
3. Replace the `priceFormatted` block (lines 43-45):
   ```go
   if p.PriceFormatted != "" {
       m["priceFormatted"] = p.PriceFormatted
   } else if p.Price > 0 {
       m["priceFormatted"] = formatPrice(p.Price, p.Currency)
   }
   ```
4. Same for Service price block at lines 143-144.
5. Test in `binding_to_map_test.go`: cover (a) explicit
   PriceFormatted wins, (b) empty PriceFormatted + Price + USD →
   `"$24"` (or whatever the unit math yields), (c) empty
   PriceFormatted + Price=0 → no key at all (consistent with current
   skip-empty behaviour).

### Backend — Part C: card seeds get grid-wrapper + width

For each of the 4 product_card variants
(`product_card.json`, `product_card_compact.json`,
`product_card_horizontal.json`, `product_card_list_row.json`):

```jsonc
{
  "version": "2.10",
  "children": [
    {
      "type": "frame",
      "id": "grid",
      "layout": {"direction": "row", "wrap": true, "gap": "md", "justify": "start"},
      "children": [
        {
          "type": "frame",
          "id": "card",
          "replicate": true,
          "width": 280,
          "layout": { /* existing card layout */ },
          "children": [ /* existing card body */ ]
        }
      ]
    }
  ]
}
```

Width values per variant:
- `product_card`: 280
- `product_card_compact`: 200
- `product_card_horizontal`: 540 (horizontal cards take ~half-row)
- `product_card_list_row`: full-row, drop `width` and use
  `layout: {direction: "column", gap: "md"}` on the wrapper instead
  of `direction: "row" + wrap`.

Detail seeds (`product_detail.json`, `product_detail_horizontal.json`)
not touched — single-instance views don't need grid wrappers; we add
`maxWidth: 720` on the root `detail` frame instead so they don't
sprawl.

### Backend — Part D: prompt rules update

`internal/prompts/agent2_prompt.go`:

1. **Add a new MODE section** before the existing
   «MODIFYING EXISTING» section (line ~309), patterned on V4
   `prompt_compose_widgets.go:13-99`:
   ```
   ## MODE — REQUIRED, pick one per turn

   The tool requires a "mode" parameter. No default. Choose based on
   user intent:

     mode: "rebuild" — discard the previous view, build fresh from
       preset / ops. Use when:
         • new data arrived (search, filter, drill-down)
         • user asks for a different view («show details», «empty
           state», «compare these», «landing», «grid»)
         • current view is irrelevant to what the user wants next

     mode: "modify" — load the existing view and apply ops as deltas
       on top. Use when:
         • cosmetic tweak («make price red», «remove rating»)
         • structural tweak on the current view («add a Buy button»,
           «2 columns»)
         • formation_tree present + the request is about changing
           what is already on screen

   When in doubt: if the user could have said «keep what's on screen
   but ...» — it's "modify". If they said «show me ...» or «render
   ...» and it implies different content — it's "rebuild".
   ```
2. **Update DECISION RULES** (lines 358-382) — replace existing rule 3
   with explicit mode guidance:
   ```
   3. ALWAYS pass mode. data_change present → almost always
      mode="rebuild" + preset (or freestyle ops). cosmetic/structural
      tweak with no new data → mode="modify" with ops only.
   ```
3. **Update example calls** in BUILDING / COMPOSING / MODIFYING
   sections to include `mode` in every `visual_assembly({...})`
   example. Existing examples must stop being valid templates without
   it.

### Frontend — Part E: Frame.jsx + CSS for wrap + width

`project_v5/frontend/src/renderer/nodes/Frame.jsx`:

1. Read additional layout / size attributes from `node`:
   ```js
   const layout = node.layout || {}
   const width = node.width
   const maxWidth = node.maxWidth
   const minWidth = node.minWidth
   const styleProps = {}
   if (width != null) styleProps.width = sizeValue(width)
   if (maxWidth != null) styleProps.maxWidth = sizeValue(maxWidth)
   if (minWidth != null) styleProps.minWidth = sizeValue(minWidth)
   ```
2. `sizeValue(v)` helper: number → `${v}px`, string → pass-through.
3. Add `data-wrap={layout.wrap ? 'true' : ''}` and pass `style={styleProps}`
   alongside existing `data-*` attributes.

`project_v5/frontend/src/widget.css`:

1. Add `.kw-frame[data-wrap='true'] { flex-wrap: wrap; }` near the
   existing `.kw-frame[data-direction]` block.
2. Add `.kw-frame[data-justify='start'] { justify-content: flex-start; }`
   if not already present.
3. Optional: tweak `.kw-display-inner` to `align-items: flex-start` so
   wrapped grid frame doesn't stretch to full container height when
   it has fewer than `columns` items.

### Frontend — Part F: tests

`project_v5/frontend/tests/`:

1. New `frame-layout.test.jsx` (~60 lines):
   - Frame with `layout.wrap=true` → renders with `data-wrap="true"`.
   - Frame with `node.width=280` → renders with `style="width: 280px"`.
   - Frame with `node.maxWidth="50%"` → string pass-through.
   - Frame without width/wrap → no `data-wrap`, no inline style.
2. Extend `renderer.test.jsx` if any existing fixture starts wrapping
   product-card grid (probably not — fixtures are pre-engine-rendered
   docs).

### Backend tests — Part G

1. `tool_visual_assembly_test.go`:
   - `mode: "rebuild"` + non-empty Template → ignores Template, builds
     from preset (assert `Children` count matches preset replicate
     output).
   - `mode: "modify"` + empty Template → `IsError` with the «modify
     requires a current Document» message.
   - `mode missing` → `IsError` «mode required».
2. `binding_to_map_test.go`:
   - Empty `PriceFormatted` + `Price=2400` + `Currency="USD"` →
     `priceFormatted` key set, `m["price"] == 2400`.
   - Explicit `PriceFormatted="$24.00"` wins over computed.
3. **Live HTTP smoke** (`handler_pipeline_live_test.go`): minimal touch
   — every existing call needs `mode` in the request body.
   - Turn 1 «show 3 product cards» → expect `mode=rebuild` in tool
     call input.
   - Existing chunk-11 turn 5 (product_card preset rebuild) → also
     `mode=rebuild`.
   - One «modify» turn («make titles bold») after turn 1 → expect
     `mode=modify`.
   - Verify pipeline response Document has 1 child of type=frame
     with `layout.wrap=true` (the grid wrapper) on rebuild turns.

### Manual visual verification

Cleared dev session → backend on 8084 + frontend on 5173.
1. «покажи 3 крема» → grid wrap (3 cards in a row, properly
   spaced).
2. «сделай 6» → 6 cards wrapping to 2 rows.
3. «сделай лендинг с hero и 4 карточками» → mode=rebuild,
   composed view; old grid disappears.
4. «сделай заголовок жирным красным» → mode=modify, only the
   targeted text changes.
5. Card price renders as `$N` not blank.

## Files changed (planned)

Backend:
| File | Status | Notes |
|---|---|---|
| `internal/tools/tool_visual_assembly.go` | modified | + mode REQUIRED, branch on mode in Execute |
| `internal/tools/tool_visual_assembly_test.go` | modified | + 3 mode-related tests |
| `internal/engine/binding_to_map.go` | modified | + formatPrice helper, fallback in priceFormatted block |
| `internal/engine/binding_to_map_test.go` | modified | + price-format fallback tests |
| `internal/prompts/agent2_prompt.go` | modified | new MODE section + DECISION RULES update + every example carries mode |
| `internal/engine/presets/seed/product_card.json` | modified | grid wrapper, card width=280 |
| `internal/engine/presets/seed/product_card_compact.json` | modified | grid wrapper, card width=200 |
| `internal/engine/presets/seed/product_card_horizontal.json` | modified | grid wrapper, card width=540 |
| `internal/engine/presets/seed/product_card_list_row.json` | modified | column-stack wrapper, no card width |
| `internal/engine/presets/seed/product_detail.json` | modified | maxWidth on root |
| `internal/engine/presets/seed/product_detail_horizontal.json` | modified | maxWidth on root |
| `internal/handlers/handler_pipeline_live_test.go` | modified | + mode in every turn body, assert wrap on rebuild docs |

Frontend:
| File | Status | Notes |
|---|---|---|
| `src/renderer/nodes/Frame.jsx` | modified | wrap + width/maxWidth/minWidth + sizeValue helper |
| `src/widget.css` | modified | `.kw-frame[data-wrap]`, `.kw-frame[data-justify='start']`, align-items tweak |
| `tests/frame-layout.test.jsx` | added | wrap + width unit tests |

Docs:
| File | Status | Notes |
|---|---|---|
| `docs/v5-engine-plan.md` | modified | mark P0-B render-polish closed (the «card visual quality» row in Snapshot) |
| `docs/v5-known-gaps.md` | modified | strike through 4 render-quality gap rows + reference closing commit |
| `docs/Updates/v5/plans/chunk-12-render-polish.md` | added | frozen plan |
| `docs/Updates/v5/v5_2026-05-03_<HHMM>_chunk-12.md` | added | session log |
| `docs/Updates/v5/README.md` | modified | + chunk 12 entry in index |
| `CLAUDE.md` | modified | mention chunk 12 in V5 status block |

## Verification

```sh
cd project_v5/backend
go build ./... && go build -tags=integration ./... && \
  go build -tags="integration live" ./... && go build -tags=tokens ./...
go vet ./... && go vet -tags=integration ./... && \
  go vet -tags="integration live" ./...
go test -count=1 ./...

# Live HTTP smoke (~$0.025 — already covered with chunk-11 budget)
TEST_DATABASE_URL=$DB ANTHROPIC_API_KEY=$KEY \
  go test -tags="integration live" -v -count=1 -timeout 12m \
  ./internal/handlers/... -run TestHTTPLiveSmoke

cd ../frontend
npm test                     # vitest jsdom

# Manual browser sweep — see «Manual visual verification» above.
```

Acceptance:
- All Go tests + live smoke green at all build tags.
- Manual: a 3-product «покажи 3 крема» renders as a 3-column grid (or
  wraps to 2 rows on narrow screens) with visible prices.
- Manual: «сделай лендинг» triggers mode=rebuild and replaces the
  view (no salad of old + new).
- Manual: «сделай заголовок жирным» triggers mode=modify and only
  the title changes.

## Known gaps after chunk 12

- **Cross-tenant chat inspection in Curator** — chunk 13.
- **Liked / in-cart visual state** — buttons fire actions but cards
  don't change visually after click (chunk-11 known gap, still
  open).
- **Tier2-promoted fields visibility in `<fields>` block** — works
  today through ProductToMap spread, but `<fields>` block only shows
  promoted-by-curator fields. Stays as-is until canvas microservice
  ships and tenants self-curate field_definitions.
- **Atom-level enum size (V4 `tiny/small/medium/large`)** — not
  ported. Width/maxWidth covers all current uses; constraints engine
  (W8 / C1 / C3) deferred entirely.
- **Real V9 `Sizing` (Fixed/Fill/Fit)** — out of scope. Width/maxWidth
  as numbers/strings is enough for chunk 12; full sizing lands when
  canvas microservice ships.
- **Modify-mode error path on empty Template** — chunk 12 errors with
  «modify requires a current Document»; the LLM should learn to flip
  to rebuild. If observed mid-conversation, prompt may need a
  reinforcement.

## Quick reference for execution

- **Branch**: `v5`. **Last commit** before chunk-12 work: `57d5279`.
- **Local dev**: V5 backend on `:8084`, V5 frontend on `:5173`.
- **First reads** during execution:
  1. This file.
  2. `internal/tools/tool_visual_assembly.go` (current Execute body) +
     `project_v4/.../tool_visual_assembly.go` (V4 mode handling
     reference, lines 35-163 + 167-276).
  3. `internal/engine/binding_to_map.go` lines 24-127 +
     `project_v4/.../postgres_catalog.go:547-572` (formatPrice port).
  4. `internal/prompts/agent2_prompt.go` lines 309-394 (MODIFYING +
     DECISION RULES) + V4 `prompt_compose_widgets.go:13-99` (MODE
     section reference).
  5. `internal/engine/presets/seed/product_card.json` (target shape
     after wrap + width).
  6. `frontend/src/renderer/nodes/Frame.jsx` (current props read) +
     `frontend/src/widget.css` lines 171-205 (current frame CSS).
- **Verify the price unit** (kopecks vs whole) by reading
  `postgres_catalog.go` ListProducts before writing the formatPrice
  helper — V5 may differ from V4 here.
