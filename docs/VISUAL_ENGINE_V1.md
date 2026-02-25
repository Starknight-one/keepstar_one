# Visual Assembly Engine v1

## Summary

Visual Assembly Engine — движок рендеринга сущностей (товары/услуги) в chat widget.
Pipeline: данные → атомы → виджеты → формации.
Ветка: `feature/visual-assembly-engine`

---

## What was done (this session)

### Phase 1: Bug fixes
- **1.1 Compose duplicates** — `buildComposedFormation()` теперь трекает `productOffset`/`serviceOffset` между секциями, каждая секция получает уникальные сущности
- **1.2 Negative pagination offset** — clamp offset к 0, bounds guard перед slice
- **1.4 resolveColor crash** — `typeof color !== 'string'` guard
- **1.5 formatPrice "$null"** — early return when `atom.value == null`
- **1.6 carousel-image CSS** — добавлен `.carousel-image { width:100%; height:100%; object-fit:cover }`
- ~~1.3 MaxAtomsPerSize "xl"~~ — пропущено намеренно, xl и так без лимита

### Phase 2: Tier 1 constraints
- **2.1 A6: Null value guard** — `AtomRenderer.jsx` пропускает атомы с null value (кроме image и 0)
- **2.2 D3: Price 0 → nil** — `productFieldGetter`/`serviceFieldGetter` возвращают nil для price=0
- **2.3 F5: Carousel peek** — `.formation-carousel > * { width: 85% }`
- **2.4 W3: Empty media** — `.generic-card-media:empty { display: none }`

### Phase 3: New preset system (9 presets)
- `preset_entity.go` — 9 новых PresetName констант
- `presets/visual_assembly_presets.go` — конфигурации:
  - **product_card_grid** — grid/medium, image+name+price
  - **product_card_detail** — single/large, все поля
  - **product_row** — list/small, horizontal, 5 полей
  - **product_single_hero** — single/large, hero с description
  - **search_empty** — single/medium, title+description
  - **category_overview** — grid/medium, image+category+name
  - **attribute_picker** — grid/small, name as tag
  - **cart_summary** — list/small, thumbnail+name+price+qty
  - **info_card** — single/medium, title+body
- `preset_registry.go` — все зарегистрированы
- `tool_visual_assembly.go` Definition() — enum с 17 пресетами
- `prompt_compose_widgets.go` — таблица пресетов в Agent2 промпте

### Phase 4: Constraints pipeline
- **NEW `tools/constraints.go`** — 4 уровня:
  - Level 0 (sanitization): D5 truncate by slot, D6 downgrade long name display, D7 validate image URLs
  - Level 1 (per-atom): A1 badge>20→tag, A2 tag>40→body-sm, A4 badge capitalize, A5 rating<3→compact
  - Level 2 (per-widget): W1 max 2 badges, W2 max 5 tags, W4 one h1/h2, W7 horizontal+>4→vertical, W8 tiny→no images
  - Level 4 (cross-widget): C1 uniform field set in grid (70% threshold)
- Integration в `tool_visual_assembly.go` Execute() — после buildVisualWidgets, до writeFormation
- D7 image URL validation в `buildAtoms()`
- **CSS safety nets:**
  - F1: grid align-items stretch + content flex:1
  - F6: alternating list row backgrounds
  - F8: mobile grid fallback @media 300px
  - C5: line-clamp h2/h3 в grid (2 lines medium, 1 line small)
  - C3: price min-width 4em
  - A7: default fallback → body class
  - A3: color contrast (luminance → white/dark text)

### Phase 5: Input validation
- `validateInput()` в начале Execute():
  - size: только valid values, иначе → medium
  - color: named (6) или hex #xxx/#xxxxxx, иначе strip
  - shape: pill/rounded/square/circle, иначе strip
  - anchor: 5 values, иначе strip
  - layer: parseable int, иначе strip

### Phase 6: Visual testbench
- **Backend:** `handlers/handler_testbench.go` — POST /api/v1/testbench
  - Loads products by tenant slug from catalog
  - Runs visual assembly pipeline directly (no LLM)
  - Returns formation JSON + entity raw data + warnings
- **Frontend:** `project_admin/frontend/src/features/testbench/`
  - Controls: tenant, preset, layout, size, direction, place, count, show/hide fields, order, color/display/shape/anchor/layer/conditional (JSON)
  - Preview tab: mini formation renderer
  - Entity Data tab: raw product data table (shows which fields exist)
  - JSON tab: raw response
- Route `/testbench` в admin App.jsx, nav link с FlaskConical icon

### Agent2 prompt
- Переписан на английский (инструкции, правила, описания)
- Примеры user_request остались на русском
- Добавлена таблица пресетов с описаниями

---

## Known issues / TODO

- **Testbench Entity Data** — не показывает данные если нажать Entity Data после рендера (нужно нажать Render снова, т.к. entities приходят с каждым запросом)
- **Compose UI** — в тестбенче нет UI для compose (мульти-секции), только через JSON tab
- **Testbench lives in chat backend** — endpoint в chat backend (port 8080), UI в admin frontend (port 5174). Engine code в chat backend, поэтому endpoint там же
- **MaxAtomsPerSize** — нет жёсткого лимита для xl, будет покрыто нормальными constraints позже
- **product_comparison preset** — не создан отдельно в visual_assembly_presets.go (используется существующий ProductComparisonPreset из product_presets.go)

## Key files

| File | What |
|------|------|
| `tools/tool_visual_assembly.go` | Main engine: Execute, compose, pagination, validation |
| `tools/constraints.go` | 20 constraint rules in 4 levels |
| `tools/tool_render_preset.go` | buildAtoms, field getters, D7 image validation |
| `tools/defaults_engine.go` | AutoResolve, field rankings, slot constraints |
| `presets/visual_assembly_presets.go` | 9 new GenericCard presets |
| `presets/preset_registry.go` | All presets registered |
| `domain/preset_entity.go` | PresetName constants |
| `prompts/prompt_compose_widgets.go` | Agent2 system prompt (EN) + examples (RU) |
| `handlers/handler_testbench.go` | Testbench API endpoint |
| `frontend/atom/AtomRenderer.jsx` | Null guard, resolveColor fix, formatPrice fix, A3 contrast |
| `frontend/atom/Atom.css` | C5 line-clamp, C3 price width |
| `frontend/formation/Formation.css` | F1 stretch, F5 peek, F6 alternating, F8 mobile |
| `frontend/widget/templates/GenericCardTemplate.css` | carousel-image, empty media |
| `project_admin/frontend/testbench/` | Testbench UI page |
