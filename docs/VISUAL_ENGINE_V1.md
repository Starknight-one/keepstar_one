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

### Phase 7: Split `display` into `format` + `wrapper`

Атом `display` раньше смешивал две задачи: трансформация значения и визуальная обёртка.
Теперь разделено:

- **`format`** — как raw value превращается в текст. Авто-определяется из Type+Subtype, редко переопределяется.
- **`display`** — визуальная обёртка (badge, tag, h1, body). Универсальна — любая обёртка для любого типа данных (кроме image-only/icon-only).

**Пример — "цена в бейдже":**
```
{type: "number", subtype: "currency", format: "currency", display: "badge", value: 329}
→ format currency: "$329.00"
→ display badge: цветная пилюля с "$329.00"
```

#### Domain (Step 1)
- `domain/atom_entity.go` — новый тип `AtomFormat` с 8 константами: `currency`, `stars`, `stars-text`, `stars-compact`, `percent`, `number`, `date`, `text`. Поле `Format` добавлено в `Atom` struct
- `domain/preset_entity.go` — поле `Format` в `FieldConfig`
- `domain/template_entity.go` — поле `Format` в `FieldSpec`

#### Format table
| Type+Subtype | Auto Format | Output |
|---|---|---|
| Number+Currency | `currency` | "$329.00" |
| Number+Rating | `stars-compact` | "★ 4.2" |
| Number+Percent | `percent` | "85%" |
| Number+Int/Float | `number` | "329" |
| Text+Date | `date` | "Feb 25, 2026" |
| Text+String | `text` | as-is |

Overridable: `stars` ("★★★★☆"), `stars-text` ("4.2/5"), `stars-compact` ("★ 4.2")

#### Backend (Steps 2–5)
- `defaults_engine.go`:
  - `InferFormat(explicit, atomType, subtype)` — авто-определение формата
  - `defaultFormatForTypeSubtype` map — Type+Subtype → AtomFormat
  - `ValidateDisplay` переписан: универсальный — любой известный display valid для любого типа, кроме image-only/icon-only ограничений
  - `BuildFieldConfigsWithFormat()` — новая функция с format overrides
- `tool_render_preset.go` — `buildAtoms()` ставит `atom.Format`, `parseFieldSpecs()` парсит format, `BuildTemplateFormation()` несёт format
- `tool_visual_assembly.go`:
  - Новый параметр `format` в Definition schema (object, field→format map)
  - Execute парсит `formatOverrides`, вызывает `BuildFieldConfigsWithFormat`
  - `writeFormation` включает format в FieldSpec
  - `buildComposedFormation` принимает formatOverrides
- `constraints.go` — A5 (rating < 3 → compact) проверяет `atom.Format` + backward compat через `atom.Display`

#### Agent2 prompt (Step 6)
- Документирован параметр `format` (авто-определяется, редко override)
- `display` уточнён как "visual wrapper, universal"
- Добавлена секция FORMAT VALUES
- Новые примеры: "цену в бейдже", "рейтинг текстом", "рейтинг звёздами в заголовке"

#### Testbench handler (Step 7)
- Парсит `params["format"]` map
- Вызывает `BuildFieldConfigsWithFormat` с format overrides

#### Frontend AtomRenderer (Step 8)
- Новая функция `formatValue(atom)` — читает `atom.format` или инферит из type+subtype, возвращает отформатированную строку
- Новая функция `inferFormat(atom)` — type+subtype → format (backward compat)
- `renderByDisplay` переименован в `renderWrapper(formattedContent, display, atom, color)` — чисто визуальная обёртка
- AtomRenderer: `formatValue()` → `renderWrapper()`, badges/tags/headings показывают уже отформатированный контент

#### Testbench UI (Step 9)
- `FORMAT_VALUES` константа с 8 значениями
- `formatMap` state + `setFormatMap`
- FieldOverridePicker "Format (value transform)"
- `TestbenchAtom` — `tbFormatValue()`/`tbInferFormat()` вместо inline formatting
- Clear handler чистит formatMap

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
| `tools/tool_visual_assembly.go` | Main engine: Execute, compose, pagination, validation, format param |
| `tools/constraints.go` | 20 constraint rules in 4 levels, A5 uses Format |
| `tools/tool_render_preset.go` | buildAtoms (with format), field getters, D7 image validation |
| `tools/defaults_engine.go` | AutoResolve, InferFormat, universal ValidateDisplay, BuildFieldConfigsWithFormat |
| `presets/visual_assembly_presets.go` | 9 new GenericCard presets |
| `presets/preset_registry.go` | All presets registered |
| `domain/atom_entity.go` | AtomType, AtomSubtype, AtomFormat, Atom struct |
| `domain/preset_entity.go` | FieldConfig with Format field |
| `domain/template_entity.go` | FieldSpec with Format field |
| `prompts/prompt_compose_widgets.go` | Agent2 system prompt with format docs |
| `handlers/handler_testbench.go` | Testbench API endpoint with format support |
| `frontend/atom/AtomRenderer.jsx` | formatValue + renderWrapper split, inferFormat |
| `frontend/atom/Atom.css` | C5 line-clamp, C3 price width |
| `frontend/formation/Formation.css` | F1 stretch, F5 peek, F6 alternating, F8 mobile |
| `frontend/widget/templates/GenericCardTemplate.css` | carousel-image, empty media |
| `project_admin/frontend/testbench/` | Testbench UI page with Format picker |
