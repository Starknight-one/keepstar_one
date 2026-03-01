# Visual Assembly Engine — Status & Assessment

**Date:** 2026-02-22 (updated after Phase 4)
**Branch:** `feature/visual-assembly-engine`
**Goal:** Заменить хардкод preset-систему на динамический движок, где LLM управляет отображением через tool calls.
**Spec:** `ADW/specs/todo/engine-visual-assembly.md`

---

## Coverage vs Spec

### Primitives: 12/12 (код написан), ~7/12 протестировано

| # | Примитив | Код | Тест | Что сделано | Проблемы |
|---|----------|-----|------|-------------|----------|
| 1 | **show** | Done | Багфикс | `show: ["field"]` в visual_assembly | Было: show ставил поля ПЕРЕД базовыми → MaxAtomsPerSize обрезал цену. Починено: show добавляет в конец, MaxAtomsPerSize пропускается при явном show |
| 2 | **hide** | Done | Багфикс | `hide: ["rating"]` в visual_assembly | Было: детерминистический regex "убери" перехватывал запрос до LLM → вызывал state_filter вместо style. Починено: "убери" убран из regex, добавлена проверка на имена полей |
| 3 | **display** | Done | Works | `display: {"brand":"badge"}` + ValidateDisplay | |
| 4 | **color** | Done | Частично | `color: {"brand":"red"}`, палитра 6 цветов + hex | Backend пишет atom.meta.color, frontend рендерит. Не полностью e2e протестировано |
| 5 | **size** (per-atom) | Done | Не тест. | `size: {"images":"xl","price":"lg"}` через applyAtomMeta | Код есть, frontend читает atom.meta.size, ни разу не тестировалось юзером |
| 6 | **shape** | Done | Не тест. | `shape: {"brand":"pill"}` через applyAtomMeta | Код есть, frontend CSS atom-shape-* добавлены, не тестировалось |
| 7 | **order** | Done | Works | `order: ["image","price","name"]` | |
| 8 | **layer** | Done | Не тест. | `layer: {"stockQuantity":"2"}` через applyAtomMeta | Frontend: atom.meta.layer → inline zIndex. Не тестировалось |
| 9 | **anchor** | Done | Не тест. | `anchor: {"brand":"top-right"}` через applyAtomMeta | Frontend: CSS .atom-anchor-*, GenericCard position:relative. Не тестировалось |
| 10 | **direction** | Done | Works | `direction: "horizontal"`, CSS horizontal mode | |
| 11 | **place** | Done | Не тест. | `place: "sticky"/"floating"` через widget.Meta | Frontend: CSS .widget-place-sticky, .widget-place-floating. Не тестировалось |
| 12 | **layout** | Done | Works | grid/list/single/carousel/comparison/table | table добавлен в Phase 4 |

### Implementation Steps: 19/22 (код), ~12/22 (протестировано)

| Step | Пункт | Код | Тест | Детали |
|------|-------|-----|------|--------|
| **0** | extractFields PIM поля | Done | Works | |
| **0** | BuildHistorySummary | Done | Works | |
| **0** | CatalogDisplayMeta | Done | Не тест. | `GetDisplayMeta()` в defaults_engine.go, отправляется в Agent2 prompt |
| **0** | Нормализация данных каталога | Done | Не тест. | `data_normalize.go`: NormalizeProduct/Service, вызывается после catalog_search |
| **1** | Microcontext Agent1→Agent2 | Done | Works | |
| **1** | Pipeline передаёт microcontext | Done | Works | |
| **1** | state_filter тул | Done | Works | Но: детерминистический regex перехватывает style-запросы. Частично починено |
| **1** | history_lookup тул | Done | Не тест. | `tool_history_lookup.go`, зарегистрирован в registry |
| **1** | Agent1 промпт обновлён | Done | Багфикс | Убраны: "убери" из filter triggers, clarification rule. Добавлены: примеры style vs filter |
| **2** | Delta-интерфейс полей (add/remove) | Not done | — | |
| **2** | Layout отвязан от пресета | Done | Works | |
| **2** | visual_assembly тул | Done | Частично | 15 параметров. Основные (show/hide/layout/size/display) работают. Остальные — код есть, не тестировались |
| **2** | History summary в Agent2 промпте | Done | Works | |
| **2** | CatalogDisplayMeta в кэше Agent2 | Done | Не тест. | Отправляется как display_meta в Agent2 prompt |
| **3** | Resolution engine | Done | Works | AutoResolve, BuildFieldConfigs, field ranking |
| **3** | Constraint solver | Done | Багфикс | MaxAtomsPerSize + ValidateDisplay + ApplySlotConstraints. Было: MaxAtomsPerSize конфликтовал с show. Починено |
| **3** | Пресеты как saved configs | Partial | — | |
| **3** | Formation diff/patch | Done | Частично | visual_assembly читает currentConfig из state, патчит поверх. Работает для базовых случаев |
| **4** | Composite layouts (compose[]) | Done | Не тест. | buildComposedFormation в tool_visual_assembly.go, frontend FormationRenderer.jsx |
| **4** | Size calculator | Done | Works | CalcGridConfig() — авто-колонки по количеству товаров |
| **4** | Comparison/Table shapes | Done | Частично | Comparison работает, Table — код есть, не тестировался |
| **4** | Расширенный constraint solver | Done | Не тест. | ApplySlotConstraints с DefaultSlotConstraints map |

### GAPs from Spec

| # | GAP | Код | Тест | Детали |
|---|-----|-----|------|--------|
| 1 | extractFields не видит PIM поля | Done | Works | |
| 2 | Микроконтекст не существует | Done | Works | |
| 3 | Agent1 state_filter/history | Done | Частично | state_filter работает, history_lookup — код есть, не тестировался |
| 4 | History summary для Agent2 | Done | Works | |
| 5 | Tool-train | Done | Частично | visual_assembly с 15 параметрами, основные работают |
| 6 | Delta-интерфейс полей | Not done | — | |
| 7 | Layout отвязка от пресета | Done | Works | |
| 8 | CatalogDisplayMeta | Done | Не тест. | GetDisplayMeta() отправляется в prompt |
| 9 | Данные каталога — нормализация | Done | Не тест. | NormalizeProduct/Service в data_normalize.go |
| 10 | Formation deltas (diff/patch) | Done | Частично | Через currentConfig в visual_assembly |
| 11 | Clarification mechanism | Removed | — | Было добавлено в Phase 4, сломало весь pipeline (ловило ВСЕ текстовые ответы Agent1 как clarification). Удалено |
| 12 | Compose spatial arrangement | Done | Не тест. | compose[] в visual_assembly |
| 13 | Compose by filter/category | Done | Не тест. | compose[] поддерживает show/hide/count per section |
| 14 | Conditional styling | Done | Не тест. | conditional[] в visual_assembly, evalCondition, applyConditionalStyling |
| 15 | Empty state | Done | Works | Agent2 возвращает "Ничего не найдено" + подсказка + tag action |
| 16 | Graceful degradation | Done | Не тест. | Agent2: fallback visual_assembly({}) при ошибке tool. visual_assembly: unknown layout → fallback + degraded flag |
| 17 | Pagination | Done | Не тест. | Backend: limit/offset, PaginationMeta. Frontend: FormationRenderer читает pagination, onLoadMore. Handler: Sections+Pagination в FormationResponse |

---

## What Works (Tested by user)

| Feature | Status | Notes |
|---------|--------|-------|
| Grid рендеринг (3 колонки, 13+ товаров) | **Works** | Defaults engine: grid/small/3 fields для 13+ товаров |
| Детальная карточка (instant expand) | **Works** | adjacentTemplates + fillFormation на клиенте, мгновенно |
| Back навигация | **Works** | Stepper trail model |
| catalog_search | **Works** | Поиск товаров с фильтрами |
| Defaults Engine | **Works** | AutoResolve по типу и количеству, field ranking, CalcGridConfig |
| GenericCardTemplate | **Works** | Универсальная карточка, рендерит атомы в порядке backend |
| Grid / List / Single / Carousel / Comparison | **Works** | Formation modes |
| Direction (horizontal) | **Works** | CSS horizontal mode в GenericCard |
| Microcontext | **Works** | Pipeline сигналы Agent2 |

## Known Bugs (Found during testing)

### 1. Детерминистический regex перехватывает style-запросы

**Файл:** `agent1_execute.go:19, 126`

**Symptom:** "убери описание" вызывает state_filter вместо стиля. Agent1 LLM = 0ms, 0 tokens — LLM даже не вызывался.

**Root cause:** `filterTriggers` regex содержал "убери" как trigger. "Убери описание" матчилось → bypassed LLM → state_filter({text_match: "Убери описание"}) → 0 results → microcontext = "filtered" → Agent2 рендерит grid.

**Fix applied:** Убрано "убери" из regex. Добавлена `styleFieldNames` regex — если запрос содержит имя поля (описание, рейтинг, бренд...), детерминистический шорткат не срабатывает.

**Status:** Код написан, не протестировано после фикса.

### 2. show/hide теряет существующие поля

**Файл:** `tool_visual_assembly.go`

**Symptom:** "покажи с описанием" — пропала цена, остались только фото+название.

**Root cause:** show ставил новые поля ПЕРЕД базовыми, затем MaxAtomsPerSize["small"]=3 обрезал. Результат: [description, images, name] — цена потеряна.

**Fix applied:** show добавляет поля в КОНЕЦ списка. MaxAtomsPerSize пропускается при явном show/hide.

**Status:** Код написан, не протестировано после фикса.

### 3. Agent2 не знает что на экране (screen context)

**Файл:** множество файлов (frontend→handler→pipeline→agent2→prompt)

**Symptom:** "убери рейтинг" на детальной карточке → возврат на grid. Agent2 не знал что юзер на detail card.

**Root cause:** Frontend не отправлял текущее состояние экрана. Backend state мог быть устаревшим после клиентской навигации (expand/back).

**Fix applied:** Frontend отправляет `screenContext: {mode, widgetCount, fields}` с каждым запросом. Agent2 видит `screen_state` в промпте. Правило в промпте: "если mode=single, widget_count=1 → юзер на детальной карточке".

**Status:** Код написан, visible_fields заполняется после фикса fillFormation. Не протестировано.

### 4. Clarification mechanism ломала pipeline

**Файл:** `agent1_execute.go`, `agent2_execute.go`, `pipeline_execute.go`

**Symptom:** Любой style-запрос ("покажи с описанием", "убери рейтинг") показывал текстовый виджет вместо рендеринга.

**Root cause:** В agent1_execute.go любой текстовый ответ без tool call превращался в `_clarify`. Это ловило ВСЕ style-запросы (где Agent1 правильно не вызывал tool). Каскад: toolName="_clarify" → microcontext="clarify:..." → Agent2 рендерил текстовый виджет.

**Fix applied:** Удалена вся clarification логика: auto-detect в agent1, clarification widget в agent2, _clarify case в buildMicrocontext. Удалено clarification rule из Agent1 промпта.

**Status:** Починено и протестировано — пользователь подтвердил что grid рендерится.

### 5. Defaults engine для 13+ товаров — list/tiny

**Symptom:** 23 товара → 1 колонка, только фото+название (без цены).

**Root cause:** AutoResolve для 13+ давал layout=list, size=tiny. MaxAtomsPerSize["tiny"]=2 → только 2 поля.

**Fix applied:** 13+ товаров → layout=grid, size=small (MaxAtomsPerSize["small"]=3 → фото+имя+цена).

**Status:** Починено и протестировано — пользователь подтвердил grid 3 колонки с ценой.

### 6. FormationResponse не содержал Sections и Pagination

**Файл:** `handler_pipeline.go`

**Root cause:** Struct FormationResponse не имел полей Sections и Pagination → compose[] и пагинация не доходили до фронтенда.

**Fix applied:** Добавлены поля + заполнение в handler.

**Status:** Код написан, не протестировано.

### 7. Чат не прокручивается

**Файл:** `ChatPanel.css`, `ChatHistory.jsx`

**Root cause:** `justify-content: flex-end` + `overflow-y: auto` = CSS баг, верхние сообщения недоступны.

**Fix applied:** Убран justify-content, добавлен spacer + auto-scroll через useRef.

**Status:** Код написан, не протестировано.

---

## Architecture Assessment

### Solid

- **Atom model** — 6 types, subtypes, display styles, slots
- **Defaults Engine** — field ranking, auto-resolve by entity count, CalcGridConfig
- **Zone-write state** — atomic updates with deltas
- **GenericCardTemplate** — truly generic, no template proliferation
- **Tool-based rendering** — Agent2 calls `visual_assembly`, tool writes formation to state
- **Instant expand** — adjacentTemplates + fillFormation, zero latency detail view

### Fragile / Problematic

- **Детерминистический regex в Agent1** — перехватывает запросы до LLM, не различает style vs filter. Частично починен, но хрупкий
- **LLM decision quality** — Both agents depend on prompt compliance. #1 risk
- **No validation layer** — Agent2's tool call goes straight to execution. show/hide/MaxAtomsPerSize конфликтовали
- **Screen context gap** — Frontend и backend не были синхронизированы по текущему экрану. Добавлен screenContext, но не протестирован
- **Prompt coupling** — Changes to one agent's prompt can break the other
- **Phase 4 code untested** — ~650 строк кода добавлены без e2e тестирования. Многие фичи (compose, conditional, pagination, layer, anchor, place) существуют только как код

### Architectural Debt

| Проблема | Влияние | Сложность фикса |
|----------|---------|-----------------|
| Детерминистический regex bypass в Agent1 | Высокое — ломает style-запросы | Средняя — нужно переосмыслить когда bypass нужен |
| MaxAtomsPerSize vs show/hide | Высокое — теряет поля | Починено, но хрупко |
| Screen state синхронизация | Высокое — Agent2 рендерит не то что на экране | Добавлен screenContext, нужно тестить |
| Нет юнит-тестов на движок | Высокое — каждый фикс может сломать другое | Высокая — нужно покрыть ключевые flows |
| 15 параметров visual_assembly | Среднее — LLM может ошибиться в любом | Концептуальная — нужен ли intent-layer |

---

---

## Phase 6: Layout Engine (2026-02-26)

**Branch:** `feature/layout-engine`

### Что сделано

Backend после constraints вычисляет `zones[]` для каждого виджета. 8 правил классификации (по display/type/slot) → 7 zone types (hero, row, stack, flow, grid, collapsed). Фронт маппит на 6 CSS-классов, zero логики.

### Ключевые изменения

| Файл | Что |
|------|-----|
| `domain/widget_entity.go` | ZoneType, Zone struct, Widget.Zones |
| `tools/layout_engine.go` | CalculateZones + DesignTokens (NEW) |
| `tools/tool_visual_assembly.go` | Вызов CalculateZones (стандартный + composed) |
| `handlers/handler_testbench.go` | Вызов CalculateZones |
| `tools/formation_fuzz_test.go` | +6 инвариантов (I12-I17) |
| `GenericCardTemplate.jsx` | ZoneLayout + LegacyLayout (backward compat) |
| `GenericCardTemplate.css` | 6 zone CSS-классов + fold toggle |
| `WidgetRenderer.jsx` | zones prop |

### Тестирование

327K+ комбинаций, 17 инвариантов (11 existing + 6 zone), 0 failures.

### Ограничения

- Нет zone overrides для Agent 2 (нужен примитив `group`)
- `order` работает внутри зоны, не между зонами
- Один design token (FoldMaxVisible=9)
- Нет responsive hints

**Полная спека:** `docs/LAYOUT_ENGINE_SPEC.md`

---

## Files

### Created (Phase 1-3)
- `backend/internal/tools/tool_state_filter.go`
- `backend/internal/tools/tool_visual_assembly.go`
- `backend/internal/tools/defaults_engine.go`
- `frontend/src/entities/widget/templates/GenericCardTemplate.jsx`
- `frontend/src/entities/widget/templates/GenericCardTemplate.css`

### Created (Phase 4)
- `backend/internal/tools/tool_history_lookup.go` — history_lookup тул (не тестировался)
- `backend/internal/tools/data_normalize.go` — NormalizeProduct/Service (не тестировался)

### Modified (Phase 2-3)
- `frontend/src/widget.jsx` — ComparisonTemplate CSS in Shadow DOM
- `backend/internal/tools/tool_render_preset.go` — PIM fields in fieldTypeMap + productFieldGetter
- `backend/internal/tools/tool_registry.go` — state_filter + history_lookup registration
- `backend/internal/tools/tool_catalog_search.go` — PIM fields + NormalizeProduct call
- `backend/internal/prompts/prompt_compose_widgets.go` — color, direction, history, microcontext, layout rules, display_meta, screen_state, все новые параметры
- `backend/internal/prompts/prompt_analyze_query.go` — state_filter rules, style vs filter examples
- `backend/internal/usecases/agent2_execute.go` — microcontext, allDeltas, empty state, graceful degradation, screenContext passthrough
- `backend/internal/usecases/pipeline_execute.go` — microcontext generation, ScreenContext struct, passthrough to Agent2
- `backend/internal/usecases/agent1_execute.go` — filterTriggers regex fix, styleFieldNames guard, removed _clarify auto-detection
- `frontend/src/entities/atom/AtomRenderer.jsx` — color palette, layer style (zIndex), anchor class
- `frontend/src/entities/atom/Atom.css` — anchor position classes
- `frontend/src/entities/widget/WidgetRenderer.jsx` — direction prop, place class
- `frontend/src/entities/widget/Widget.css` — place-sticky, place-floating styles
- `frontend/src/entities/formation/FormationRenderer.jsx` — composed sections, table mode, pagination, onLoadMore
- `frontend/src/entities/formation/formationModel.js` — TABLE mode
- `frontend/src/entities/formation/Formation.css` — composed, table, section styles
- `frontend/src/features/chat/ChatPanel.jsx` — lastFormationRef passthrough to useChatSubmit
- `frontend/src/features/chat/ChatHistory.jsx` — auto-scroll, spacer instead of justify-content
- `frontend/src/features/chat/ChatPanel.css` — chat-history-spacer, removed justify-content: flex-end
- `frontend/src/features/chat/useChatSubmit.js` — screenContext extraction from lastFormationRef
- `frontend/src/features/chat/model/fillFormation.js` — fieldName passthrough in filled atoms
- `frontend/src/shared/api/apiClient.js` — screenContext in sendPipelineQuery
- `backend/internal/handlers/handler_pipeline.go` — ScreenContext in request, Sections+Pagination in FormationResponse
- `backend/internal/domain/template_entity.go` — FormationSection, PaginationMeta structs
- `backend/internal/domain/formation_entity.go` — FormationTypeTable
- `backend/internal/domain/state_entity.go` — ActionClarify (unused)
- `backend/internal/domain/catalog_digest_entity.go` — FieldDisplayHint struct
