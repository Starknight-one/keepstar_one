# PRE_LAUNCH_TASKS

Единый каталог задач до прод-реди. Источники:
1. Наброски из google sheet (владелец-only, экспорт 2026-04-08)
2. Known gaps из `docs/Updates/feature-engine-v4_*.md` (2026-04-04 … 2026-04-13)
3. ~~`docs/New features/LAUNCH_CHECKLIST.md`~~ — удалён 2026-04-26 как устаревший snapshot, всё актуальное здесь

Формат: **Задача** → Контекст (откуда) → Что сделать → Как проверить → Якоря.

Без приоритетов, без оценок — просто каталог. Фильтрацию/сортировку делаем потом вручную.

Статусы в скобках после заголовка: `(open)` / `(partial)` / `(done)` / `(blocked)` / `(reverted)`.

**Последняя сверка с git: 2026-04-26.** Закрытые с прошлой сверки задачи отмечены DONE-баннером с датой и коммитами.

---

# 1. Agent1 (NLU / data intent)

## 1.1 Routing: "search vs handoff vs chat" (open)

**Контекст**: sheet row 1. На входе в чат Agent1 должен принять одно из трёх решений — (a) запустить поиск данных, (b) отдать управление Agent2 сразу (когда запрос про UI/формат и данные не нужны), (c) ответить текстом без пайплайна.

**Что сделать**:
- Формализовать три ветки в prompt Agent1 + в `pipeline_execute.go`: `search | handoff_to_agent2 | chat_reply`.
- Ветки должны различать:
  - **Cold start**: стейт пустой, запрос требует данных → search.
  - **Cold start без данных**: запрос генеративный (объяснить, посоветовать, сгенерировать текстовый блок) → handoff_to_agent2 (freestyle) или chat_reply.
  - **Cold start UI-only**: «покажи мне большой баннер» при пустом стейте → handoff_to_agent2 без данных.
  - **Hot start**: стейт уже с данными — решить, достаточно ли их или нужен доп.поиск (delta append к state.data).
- Связано с 1.3 (стейт context) и 2.8 (freestyle ветка Agent2).

**Как проверить**: testbench-набор из ~8 фраз, по 2 на каждую ветку, смотреть в трейсах какой tool вызвался.

**Якоря**: `project_v4/backend/internal/usecases/pipeline_execute.go`, `project_v4/backend/internal/prompts/` (Agent1 prompt), `project_v4/backend/internal/usecases/agent1_execute.go`.

---

## 1.2 Tenant metadata digest в system prompt (done)

**Контекст**: sheet row 2. Чтобы Agent1 лучше использовал `catalog_search` (vector + SQL + merge), ему нужна сжатая метаинформация о каталоге тенанта в закэшированной части system prompt.

**Закрыто** 2026-04-09 (`403d1fe` + `64dc6ae`) — дайджест инлайнится в Agent1 system prompt через `buildSystemPromptWithDigest()` с per-tenant мемоизацией (sync.Map). Prompt cache активирован: single breakpoint на конце system покрывает [tools+system] одним блоком >2048 токенов. Logs: `2026-04-09_01-21.md`, `2026-04-09_02-05.md`.

**Открытый под-вопрос**: что делать при каталоге 10k+ items когда сам дайджест превышает N токенов — пока не актуально (heybabes 967 продуктов укладывается), задача отложена до появления большого тенанта.

---

## 1.3 State context injection перед каждым запросом (partial)

**Контекст**: sheet row 3. Agent1 должен понимать что прямо сейчас на экране, историю переписки, какие данные уже загружены (и какие могут быть показаны по клику), атрибуты видимых items.

**Что сделать**:
- Перед запросом Agent1 подмешивать в user message или system prompt compact state snapshot:
  - что на экране сейчас (visible widgets + их entity refs)
  - что в стейте суммарно (len(data), какие типы items)
  - последние N user turns
  - активные фильтры / лайкнутые / корзина (см. 3.1)
- **Оптимизация**: если данные не менялись между turn'ами (был только UI-запрос), не перетаскивать полный data snapshot — только diff.

**Как проверить**: «а покажи только те что в корзине» — Agent1 должен использовать `state_filter`, не `catalog_search`. Трейс должен показать что в контексте был список cart items.

**Якоря**: `project_v4/backend/internal/engine_v4/tree_ids.go` (уже строит compact tree map — часть работы сделана), `project_v4/backend/internal/usecases/pipeline_execute.go`, `project_v4/backend/internal/domain/session_state.go`.

---

# 2. Agent2 (rendering)

## 2.1 Multi-widget composition (done)

**Контекст**: sheet rows 11, 12 («композиции из пресетов и сгенерированных виджетов», «всё в рамках одного tool call»). Закрыто 2026-04-07 в #3 — 6 фаз, `01fcc15..5989cf3`.

**Что осталось**: follow-ups из `2026-04-07_04-11.md` — см. задачи 2.2, 2.3, 2.4.

---

## 2.2 Cost optimization 1.15¢ → <1¢/turn (open, merge-gate)

**Контекст**: `2026-04-07_04-11.md` gap #1. Multi-widget composition сейчас стоит ~1.15¢/turn, поставлено merge-gate на <1¢.

**Что сделать**:
- Сессия замера: снять 10 прод-трейсов composition через `/debug/traces/`, разбить стоимость по секциям (system prompt, tool description, conversation history, tool results).
- Ranking кандидатов на срезание: dedup в prompt, компрессия COMPOSING примеров, вынос examples в отдельный `COMPOSING_EXAMPLES.md` загружаемый только при первом turn (gap #3 из 03-51), компрессия tree_map в tool result.
- Прицельное срезание до <1¢.

**Как проверить**: те же 10 сценариев после изменений, diff стоимости.

**Якоря**: `project_v4/backend/internal/prompts/prompt_compose_widgets.go`, `project_v4/backend/internal/engine_v4/tree_ids.go` (BuildTreeMap output → prompt), `/debug/traces/` dashboard.

---

## 2.3 Prompt richness: replicateLimit guidance (open)

**Контекст**: `2026-04-07_04-11.md` gap #2. Agent2 берёт `replicateLimit: 3` для лендингов вместо разумных 6-8. В COMPOSING секции нет guidance про typical item count.

**Что сделать**: добавить 1-2 строки в COMPOSING секцию промпта: «For landings/presentations/catalogs, typical replicateLimit is 6-12. For comparisons 2-3. For single detail views, don't replicate.»

**Как проверить**: «сделай презентацию новой линии кремов» → trace показывает replicateLimit ≥6.

**Якоря**: `project_v4/backend/internal/prompts/prompt_compose_widgets.go` (COMPOSING section).

---

## 2.4 Грабля 9: modify + multi-widget + data refresh (open, low-freq)

**Контекст**: `2026-04-07_04-11.md` gap #3. Не встречается в текущих E2E, но всплывёт на сценарии «сделай такой же лендинг но с сыворотками вместо кремов» поверх existing composition. Fail-safe оставлен.

**Что сделать**: отложено до первого воспроизводимого примера. Когда найдётся — обработать в `expandReplicatedWidgets` + BuildTreeMap re-binding на новые data.

**Якоря**: `project_v4/backend/internal/engine_v4/expand.go` (или где expandReplicatedWidgets), `project_v4/backend/internal/usecases/pipeline_execute.go`.

---

## 2.5 COMPOSING examples: dataIndex, text_explainer, icons (open)

**Контекст**: `2026-04-07_03-51.md` gaps #4, #5; `2026-04-07_04-11.md` gap #4.
- `props.dataIndex` введён в Phase 1 для non-replicated entity widgets (compose detail для конкретного товара из multi-product data), но в примерах отсутствует → Agent2 может не использовать.
- Hero всегда показан как literal text, без примера с `text_explainer` preset.
- Нет примера с icon atom'ами (Turn 3 справился, но без guidance промахи возможны).

**Что сделать**: добавить 3 коротких примера в COMPOSING секцию — dataIndex, hero=text_explainer, feature block с icon atom.

**Как проверить**: E2E на «покажи детально первый товар + карусель остальных», «сделай лендинг с фичами и иконками».

**Якоря**: `project_v4/backend/internal/prompts/prompt_compose_widgets.go`.

---

## 2.6 Pencil parity / vision data-to-any-UI (deferred)

**Контекст**: `2026-04-07_04-11.md` gap #5, memory `vision_data_to_any_ui.md`. После multi-widget composition structural parity с Pencil component tree достигнута. Следующий шаг: либо (a) import .pen как шаблон → ops, либо (b) Agent2 читает Pencil variables.

**Что сделать**: отложено, это отдельная крупная фича за рамками pre-launch.

---

## 2.7 Session-scoped preset overrides — E1 (open)

**Контекст**: sheet row 8 («пресеты должны уметь перезаписываться в рамках сессии»), и волна E1 из `14-51.md`. Если пользователь попросил переопределить пресет («всегда показывай brand крупнее»), это должно примениться к последующим рендерам пресета в той же сессии.

**Что сделать**:
- Добавить в `SessionState` поле `presetOverrides map[string][]Op`.
- При `ExpandInlinePresets` / preset expansion в engine — применять overrides поверх базового пресета.
- Agent2 tool сигнатура: способ сказать «апдейт session-wide», например `persist: true` на override ops.
- Связано с 2.12 (session-wide design tokens).

**Как проверить**: «всегда показывай бренд жирным» → следующий grid уже с жирным брендом, без повторного запроса.

**Якоря**: `project_v4/backend/internal/engine_v4/ops.go` (ExpandInlinePresets), `project_v4/backend/internal/domain/session_state.go`, `project_v4/backend/internal/tools/tool_visual_assembly.go`.

---

## 2.8 Freestyle mode: UI без данных — A2 (open)

**Контекст**: sheet row 6 (буквально «режим Pencil»), волна A2 из `14-51.md`, gap #5 из `2026-04-04_04-07.md`. Agent2 должен уметь построить виджет с литеральным контентом, который не привязан к `state.data` и не реплицируется.

**Что сделать**:
- Флаг `freestyle: true` на widget insert op (или автодетект: если все атомы литеральные, без `fieldName`, не реплицировать).
- Route в Agent1: запросы типа «покажи большой баннер со скидкой 20%», «напиши мне список преимуществ» → handoff_to_agent2 без `catalog_search`.
- Убрать existing behaviour где движок пытается bind/replicate freestyle виджет.

**Как проверить**: «нарисуй hero-блок с текстом Распродажа 50%» — должен отрендериться 1 widget с литералами, без обращения в каталог.

**Якоря**: `project_v4/backend/internal/engine_v4/binding.go` (BindData — там добавить skip для freestyle), `project_v4/backend/internal/engine_v4/expand.go`, `project_v4/backend/internal/prompts/prompt_compose_widgets.go`.

---

## 2.9 State carry-over cleanup (open)

**Контекст**: sheet row 4/5 (стейт секции обновляются дельтами), gap #4 из `2026-04-04_04-07.md`, упомянуто в `14-51.md` как одна из остающихся. Existing formation не очищается между turn'ами если Agent2 не делает `delete formation` — виджеты от прошлых запросов «протекают».

**Что сделать**:
- Решить политику: (a) auto-clear formation при каждом data_change, (b) явный контракт в промпте «при build from scratch first delete existing», (c) гибрид — auto-clear при смене data, сохранение при modify-only.
- Реализовать выбранный вариант в `Execute()` pipeline.

**Как проверить**: «покажи крема» → «покажи шампуни» — во втором рендере не должно остаться ни одной карточки крема.

**Якоря**: `project_v4/backend/internal/engine_v4/engine.go` (Execute pipeline, start), `project_v4/backend/internal/usecases/pipeline_execute.go`.

---

## 2.10 Named ops bundles / preset economy (partial)

**Контекст**: sheet rows 7, 9, 14, 22 («использовать пресеты для экономии токенов», «стандартные пресеты для быстрой генерации»). Волна B2 уже сделана (12 пресетов, 2026-04-06), но sheet подчёркивает что нужны ещё и для **будущих кастомных пресетов тенанта** (см. 2.13) и для экономии в промпте.

**Что сделать**: оценить, нужны ли ещё базовые пресеты кроме 12 существующих. Кандидаты из sheet: comparison_2col, comparison_3col, feature_grid, hero_with_cta. Если нужны — добавить. Иначе закрыть как done.

**Якоря**: `project_v4/backend/internal/engine_v4/presets.go`, `presets_product.go`, `presets_system.go`, `presets_nav.go`.

---

## 2.11 Minimal state data для Agent2 (partial)

**Контекст**: sheet row 15. Agent2 должен получать минимум из стейта — достаточно для задачи, не больше. Сейчас через compact tree_map + state snippets. Проверить что не передаётся лишнее после Phase 4 (multi-widget tree map).

**Что сделать**: аудит — что реально уходит в Agent2 tool prompt. Если передаётся full data snapshot — скомпрессировать до meta + первые 3 items.

**Как проверить**: замер tokens через трейс, до/после.

**Якоря**: `project_v4/backend/internal/usecases/agent2_execute.go`, `project_v4/backend/internal/engine_v4/tree_ids.go` (BuildTreeMap).

---

## 2.12 Tenant design system + session overrides (partial)

**Прогресс**: design tokens editor + themes — DONE через KeepstarCanvas Phase 8 (`b89bd9c`). Тенант может задать свои tokens в админке, runtime их применяет.

**Что осталось open**: session-scoped overrides (когда user в чате говорит «поменяй кнопки на красные» — только эта сессия). Связь с 2.7.

---

## 2.12-orig Tenant design system + session overrides — original spec (для контекста)

**Контекст**: sheet row 16. Движок должен использовать дизайн-систему тенанта (tokens: colors, typography, spacing, radii). В админке тенант задаёт свою, в runtime применяется. User session может запросить переопределение — только в рамках сессии.

**Что сделать**:
- Схема `tenant.design_tokens` в БД (JSONB).
- Загрузка в `SessionState.DesignTokens` при session init.
- Frontend: CSS variables из токенов в Shadow DOM.
- Session overrides: Agent2 tool parameter `designOverrides: { color_primary: "..." }`.
- Админка: форма редактирования токенов.

**Как проверить**: сменить primary color в админке → новый чат покажет новую кнопку. «поменяй кнопки на красные» в чате → только эта сессия.

**Якоря**: `project_v4/frontend/src/shared/theme/`, `project_admin/frontend/src/pages/Settings/`, новая таблица или расширение `tenant` table, `project_v4/backend/internal/domain/session_state.go`.

---

## 2.13 Текстовые ответы агента как виджет (open)

**Контекст**: sheet row 17. Кейсы когда нужно явно ответить пользователю текстом и показать наглядно, а не в чат-пузыре. Должна быть возможность: агент возвращает текст → превращается в atom/widget → показывается на экране (как отдельный виджет или часть compose).

**Что сделать**: уже частично покрыто через freestyle (2.8) + `text_explainer` preset. Проверить что работает сценарий «объясни мне разницу между AHA и BHA» → text_explainer widget на экране, не только в чате.

**Как проверить**: прогнать 3 таких запроса.

**Якоря**: `text_explainer` preset, связь с 2.8 freestyle.

---

## 2.14 Маркетинговые триггеры (open)

**Контекст**: sheet row 18. Когда пользователь спрашивает про товары конкретной марки и не покупает — триггер подкидывает скидочный виджет или overridит бейдж цены.

**Что сделать**:
- Доменная модель `Trigger`: условие (events + duration + state predicate) + action (inject widget / override atom).
- Триггер-движок в pipeline: проверяется после Agent1, до Agent2 — если сработал, даёт Agent2 инструкцию «добавь discount badge к текущей карточке» или «добавь hero-виджет с промо».
- Конфигурация тенанта — в админке (отложено до отдельной итерации).

**Как проверить**: тестовый триггер «3 turns про brand X без add_to_cart → discount 10%», пройти через три запроса, на 4-м должен появиться discount.

**Якоря**: новый `project_v4/backend/internal/domain/trigger.go`, новая таблица `tenant.triggers`, `project_v4/backend/internal/usecases/pipeline_execute.go`.

---

## 2.15 Точный data binding (partial)

**Контекст**: sheet row 19. Сейчас binding через `FieldName` → `data[i][fieldName]`. Проблемы: неймспейс полей (`brand.name` vs `brand`), nested structures, formatting (price как number vs price как "12.99 USD").

**Что сделать**: audit на реальном каталоге ноутбуков + косметики — где ломается. Ожидаемые fix'ы: поддержка dot-path в FieldName, auto-format по subtype.

**Якоря**: `project_v4/backend/internal/engine_v4/binding.go`, `project_v4/backend/internal/engine_v4/constraints.go` (C3 — same field → same format).

---

# 3. State

## 3.1 Стейт: многосекционная структура с независимыми дельтами (partial)

**Контекст**: sheet rows 4, 5. Состоит из секций, каждая обновляется независимо:
1. Данные из БД (инкрементально, с пометкой «на экране/не на экране»)
2. Что на экране прямо сейчас (overwrite каждый turn)
3. Что на экране суммарно (инкрементально или overwrite)
4. История запросов user (append)
5. История действий user (append)
6. Misc: активные фильтры, лайкнутые, корзина (инкрементально)

**Что сделать**:
- Audit текущей структуры `SessionState` в `project_v4/backend/internal/domain/session_state.go` — насколько совпадает.
- Добавить недостающие секции (likes, cart, active_filters если нет).
- Реализовать delta append/remove/overwrite operations для каждой секции.
- Пометка «used_on_screen» на data items (для 3.2).

**Как проверить**: unit-тесты на delta операции по каждой секции.

**Якоря**: `project_v4/backend/internal/domain/session_state.go`, `project_v4/backend/internal/usecases/state_*.go`.

---

## 3.2 Лайкнутые и корзина — рабочие страницы (open)

**Контекст**: LAUNCH_CHECKLIST, sheet row 10 (CLAUDE.md — `cart_grid`, `liked_grid` пресеты существуют).

**Что сделать**:
- Like/unlike actions уже инжектятся в entity widgets (`DefaultWidgetActions`). Проверить что state_filter + `liked_grid` preset отрабатывают.
- Cart: add_to_cart action → state.cart → `cart_grid` preset.
- Inline счётчик на header (badge).
- **Instant expand**: переход на страницу likes/cart без round-trip — через `adjacentTemplates` mechanism (уже есть во фронте, проверить что работает с V4).

**Как проверить**: лайкнуть 3 товара → «покажи мои лайки» или клик на heart-icon в header → инстант переход на liked_grid.

**Якоря**: `project_v4/backend/internal/engine_v4/presets_nav.go`, `project_v4/frontend/src/features/navigation/`, `project_v4/backend/internal/usecases/action_view.go`.

---

# 4. Engine

## 4.1 Layout nesting bug: insertLayoutNode не регистрирует pending ID (open)

**Контекст**: `2026-04-04.md` "Known issues". `insertLayoutNode` в `ops.go` возвращает `__pending_node_{ptr}`, но НЕ регистрирует этот ID в `idx.nodes`. Последующие ops с `parent:"$root"` (resolve → pending ID) не находят parent → атомы падают на root вместо вложения в layout node.

**Что сделать**:
- В `insertLayoutNode` после создания node добавить `idx.nodes[pendingID] = node`.
- Unit test: op chain `insertLayoutNode ref=row → insertAtom parent=$row` — атом должен быть внутри row.

**Как проверить**: unit + E2E «сделай карточку с двумя колонками» — атомы во второй колонке, не на root.

**Якоря**: `project_v4/backend/internal/engine_v4/ops.go` (`insertLayoutNode`).

---

## 4.2 Deploy issue 04-04: старый промпт после деплоя (verify)

**Контекст**: `2026-04-04.md` "Known issues". Railway показывал старый Agent2 промпт несмотря на правильный коммит. С тех пор было ~10 коммитов и деплоев, и multi-widget работает — значит проблема либо самоустранилась, либо так и не была реальной (мог быть кешированный трейс).

**Что сделать**: verify — зайти в трейсы, убедиться что на проде живой новый COMPOSING prompt из Phase 5. Если да — закрыть задачу. Если нет — разобраться.

**Якоря**: `/debug/traces/` на `v4-engine-production.up.railway.app`.

---

## 4.3 B7: role-based field resolution (любой каталог) (done)

**Закрыто 2026-04-10** через 3 сессии metadata-driven binding (вместо изначального плана с `role` enum в field_definitions выбран более элегантный путь — Agent2 видит per-tenant `<fields>` dictionary в системном промпте и в runtime эмитит fieldName-override ops):
- `7e15cd9` — Session A: conditional tree_map + Agent2 `<fields>` block
- `9261327` — Session B: fieldName override + FIELD BINDING rule в промпте
- `5566439` — Session C: `catalog.products.extra` JSONB + test-electronics PoC seed
- `2fa746e` — VectorSearch reads p.extra
- `7caeb8f` — verified в проде

**Дизайн**: `docs/New features/METADATA_DRIVEN_BINDING_2026-04-09.md`. Доки сессий: `2026-04-09_*` + `2026-04-10_01-43.md` (PoC verification).

**Якоря**: `project_v4/backend/internal/engine_v4/ops.go` (mergeAtomProps fieldName override), `project_v4/backend/internal/usecases/agent2_execute.go` (loadFieldLabels + buildSystemPromptWithFields), `project_v4/backend/internal/adapters/postgres/field_definition_adapter.go`, `project_v4/backend/internal/prompts/prompt_compose_widgets.go` (FIELD BINDING section).

---

## 4.4 Instant nav tree pre-building (open)

**Контекст**: sheet row 10. Движок должен строить «дерево готовых переходов» — превью страниц, которые пользователь может открыть кликом, без LLM round-trip. Пример: клик по карточке → мгновенный открыт detail view (уже частично есть через `adjacentTemplates` + `fillFormation`).

**Что сделать**:
- Аудит текущего механизма `adjacentTemplates` в V4 — работает ли для всех entity widgets, а не только для product_card.
- Расширить на: product detail, liked_grid, cart_grid, back-navigation.
- Кешировать pre-built formations в `SessionState.PreBuiltViews`.

**Как проверить**: в Dev Inspector — клик по карточке должен давать 0ms на backend (чистый фронт).

**Якоря**: `project_v4/frontend/src/shared/hooks/useFillFormation.ts` (или где-то там), `project_v4/backend/internal/engine_v4/default_ops.go`, `project_v4/backend/internal/usecases/navigation_*.go`.

---

## 4.5 buildWidgetMap + entityRef (open, минорный)

**Контекст**: `2026-04-07_03-49.md` gap #4. `buildWidgetMap` не возвращает `entityRef` для литералов → Agent2 не видит какой entity привязан к single product detail widget. Сейчас узнаёт из top-level state context.

**Что сделать**: добавить `entity_ref` в literal entry шедулы tree_map.

**Якоря**: `project_v4/backend/internal/engine_v4/tree_ids.go` (buildWidgetMap).

---

## 4.6 Validation: per-widget preset существует (open, минорный)

**Контекст**: `2026-04-07_03-51.md` gap #2. Если Agent2 напишет `props.preset: "nonexistent"`, ошибка всплывает в `ExpandInlinePresets` с невнятным сообщением. Можно добавить early validation.

**Что сделать**: в tool handler валидировать `props.preset ∈ enum` до вызова движка.

**Якоря**: `project_v4/backend/internal/tools/tool_visual_assembly.go` (validation section).

---

## 4.7 deepCopyOpProps — расширение на nested slices (deferred)

**Контекст**: `2026-04-07_03-00.md` gap #7. `deepCopyOpProps` копирует только `map[string]interface{}` вглубь; slice'ы и сложные значения — по reference. На текущих preset definitions OK. Если в будущем добавятся nested slices — расширить.

**Что сделать**: отложено, триггер — добавление первого nested-slice пресета.

**Якоря**: `project_v4/backend/internal/engine_v4/ops.go` (`deepCopyOpProps`).

---

## 4.9 Parallel multi tool_use (split visual_assembly на узкие tools) (open)

**Контекст**: обсуждение 2026-04-09. Сейчас Agent2 имеет один tool `visual_assembly` с широкой schema на все режимы (build, modify, compose, replicate). Альтернатива — разбить на несколько узких tools, которые Agent2 эмитит **параллельно** в одном assistant message: `assemble_hero`, `assemble_grid`, `assemble_comparison`, `assemble_detail`, `modify_widget`. Anthropic поддерживает несколько tool_use блоков в одном response.

**Что приобретаешь**:
- Узкие schemas → Agent2 меньше галлюцинирует, output короче, cost ниже
- Параллельная обработка на бэке (goroutine per tool_use)
- Чище error reporting (один блок упал — остальные ок)
- Хорошая совместимость со streaming (явные границы блоков в потоке)

**Что не приобретаешь**: self-correction (Agent2 не видит результаты до конца своей генерации — все блоки в одном проходе).

**Стоимость**: ~+5-10% output tokens на boilerplate (tool_use IDs, обёртки), input не меняется. ≈ +0.05-0.1¢/turn.

**Что сделать**:
- Спроектировать tool registry: 5-7 узких tools вместо одного `visual_assembly`.
- Каждый tool — своя схема с минимально необходимыми полями.
- Backend dispatch: один handler на каждый tool, общий движок под капотом.
- Промпт переписать: больше нет COMPOSING секции с множественными widget inserts в одном вызове — теперь Agent2 эмитит несколько tool_use блоков.
- Aggregation на бэке: собрать results всех tool_use в одну формацию перед отдачей фронту (или стримить по мере готовности — связь с задачей streaming).

**Как проверить**: те же E2E что Phase 6 (#3) — composition, regression, modify. Замер cost + quality на 10 трейсах.

**Якоря**: `project_v4/backend/internal/tools/tool_visual_assembly.go` (split), `project_v4/backend/internal/tools/registry.go` (или где регистрируются tools), `project_v4/backend/internal/prompts/prompt_compose_widgets.go` (rewrite COMPOSING), `project_v4/backend/internal/usecases/agent2_execute.go` (handle multiple tool_use results).

---

## 4.10 Sequential multi tool_use для freestyle / iterative building (deferred)

**Контекст**: обсуждение 2026-04-09. Другая парадигма: Agent2 эмитит один tool_use → backend исполняет → result возвращается → Agent2 видит результат → решает следующий шаг → следующий tool_use. Это N round-trips в одном пользовательском turn.

**Когда это нужно**:
- Freestyle / from-scratch генерация UI без пресета (связь с задачей 2.8 / волной A2): Agent2 строит каркас, видит, наполняет.
- Self-correction: tool_use упал на validation → Agent2 видит error → пробует иначе в том же turn'е.
- Адаптивная композиция: hero оказался жирнее ожидаемого → grid делает 2 колонки вместо 3.

**Стоимость**: **жёсткая**. Latency N× (каждый round-trip ~300-500ms TTFT + генерация). Cost +30-60% на 2 step'а, до 2× на 3 step'ах (input растёт с conversation, prompt cache частично спасает).

**Когда не делать**: regression queries (preset reuse), composition с известными пресетами — sequential тут чистое зло, single call побеждает.

**Что сделать**: отложено. Триггер — старт работы над A2 (freestyle / from-scratch UI generation, задача 2.8). Тогда sequential станет естественным механизмом её реализации.

**Якоря**: `project_v4/backend/internal/usecases/agent2_execute.go` (loop с stop_reason check), Anthropic API streaming docs.

---

## 4.11 Streaming Agent2 + прогрессивный рендер фронта (dropped)

**Закрыто как dropped 2026-04-26.** Пробовали 2026-04-13 (`54cce02 streaming Agent2 + progressive widget rendering`, `d3a66b1 expose Flusher`, `8b63ab8 preset-aware provisional rendering`), затем откатили: `e4f9a0d`, `0962165`, `23a080d` (всё в один день). Решение Vlad'а: **выкинуть из роадмапа полностью**. Не возвращаемся.

---

## 4.8 Button wrapper visual check во фронте (open, мелкое)

**Контекст**: `2026-04-07_03-51.md` gap #6. Пример COMPOSING промпта показывает `{type:"text","wrapper":{"type":"button","variant":"primary"}}` для CTA. Supported V4 engine'ом, но визуальный рендер надо проверить.

**Что сделать**: E2E «сделай hero с кнопкой Купить сейчас», проверить что кнопка рендерится как кнопка, не как plain text.

**Якоря**: `project_v4/frontend/src/entities/atom/AtomRenderer.jsx` (wrapper logic), `project_v4/frontend/src/entities/widget/`.

---

# 5. Admin

## 5.1 Tenant-defined presets (done)

**Закрыто 2026-04-12 … 2026-04-20** через KeepstarCanvas Phases 1-8:
- `4ed581c` — P1: preset CRUD backend (`tenant.custom_presets` JSONB schema)
- `de81801` — P2: tenant preset loader в engine (merge default + tenant в runtime)
- `5d561ea` — P3: tenant design context в Agent2 prompt
- `4c6ea5c` — P6: inspector editing + publish/delete
- `12c7bba` — P7: components library + save-from-trace

Дизайн: `docs/New features/KEEPSTAR_CANVAS_PLAN.md`.

**Якоря**: `project_admin/backend/internal/usecases/preset_*.go`, `project_v4/backend/internal/engine_v4/tenant_preset_loader.go`, `project_admin/frontend/src/features/canvas/`.

---

## 5.2 Admin import: любой формат + agent builds catalog (partial)

**Прогресс 2026-04-19** (`b8adcd1 feat(admin): onboarding MVP — CSV + Shopify integrations`):
- ✅ CSV upload pipeline
- ✅ Shopify connector (OAuth + product fetch)
- ✅ Базовая нормализация в `master_products` + `catalog.products`
- ✅ Per-tenant `field_definitions` сидится при первом импорте

**Что осталось** — это содержание `docs/New features/admin_catalog_design_2026-04-23.md` (final design closed 2026-04-23):
- Master/Listing split с COALESCE-рендером
- Match cascade (GTIN → vendor+SKU → fuzzy → embedding → новый master)
- Mapping artifact (1 LLM-вызов на тенанта, кешируется)
- Candidates staging + curator promotion
- Junk variants detection + classification
- Per-vertical Tier 2 таблицы (Option B)
- Public API (REST endpoints)
- Curator service (отдельная папка)
- Метаdata-first import flow с 4-layer cache при re-import
- Юнит-парсер (`internal/units/`)

**Следующий шаг**: имплементация по дизайну (см. `admin_catalog_design_2026-04-23.md` §10 чек-лист).

**Якоря**: `project_admin/backend/internal/usecases/import_*.go`, `project_admin/backend/internal/usecases/enrichment_*.go`, `project_admin/frontend/src/features/import/`, `project_admin/frontend/src/features/catalog/`.

---

# 6. Infrastructure & Ops (LAUNCH_CHECKLIST)

## 6.1 Dev stand (open)

**Что сделать**: отдельный Railway environment `staging` с отдельной БД (Neon branch). Деплой из `main`, прод — из `release` tag.

## 6.2 Clean prod git (open)

**Что сделать**: вычистить prod-ветку — убрать experiments, draft docs, test fixtures. Определить что есть prod (только `project_v4/` + `project_admin/` + `scripts/`?).

## 6.3 Security + GDPR (open)

**Что сделать**: аудит — что хранится в сессиях (PII?), cookies, retention, export/delete-me endpoints. Privacy policy черновик.

## 6.4 Monitoring & alerts (open)

**Что сделать**: Railway metrics не хватает — добавить что-то для tenant-facing (Grafana? Umami?). Алерты: 5xx rate, p99 latency, LLM error rate, Anthropic quota.

## 6.5 Rate limiting (open)

**Что сделать**: per-IP + per-session rate limit на `/api/v1/pipeline`. Защита от burn-down LLM кредитов. Библиотека: `github.com/ulule/limiter` или inline.

**Якоря**: `project_v4/backend/internal/handlers/middleware_*.go` (новое).

## 6.6 Fallback pages (open)

**Что сделать**: 404, «Anthropic API недоступно», «Слишком много запросов», «Тенант не найден». Фронт + backend.

## 6.7 Migrations + DB versioning (open)

**Что сделать**: сейчас миграции хаотичные. Подключить `goose` или `golang-migrate`, перенести текущую схему в initial migration, процесс для новых.

**Якоря**: `project_v4/backend/db/migrations/` (новое), `cmd/server/main.go`.

## 6.8 Sentry / error tracking (open)

**Что сделать**: подключить Sentry в backend (Go SDK) + frontend (React SDK).

## 6.9 Load testing (open)

**Что сделать**: k6 или Vegeta скрипт — 50 concurrent users, 100 turns, измерить p50/p95/p99.

---

# 7. Product Quality (LAUNCH_CHECKLIST)

## 7.1 E2E user tests (open)

**Что сделать**: Playwright скрипт, 10-15 пользовательских сценариев — поиск, просмотр, лайк, корзина, back-навигация, composition, modify. Запускается в CI.

**Якоря**: `project_v4/frontend/e2e/` (новое).

## 7.2 Любой каталог (open)

**Блокер**: 4.3 (B7 role-based). После — тест на ноутбуках / книгах / мебели.

## 7.3 Локальные фильтры и сортировки (open)

**Контекст**: LAUNCH_CHECKLIST. «Не всегда проще писать в чат, порой проще ткнуть». Кнопки фильтров под grid'ом, sort dropdown.

**Что сделать**: фронт компонент `FormationFilters` на базе доступных атрибутов текущих data. Клик → local re-render без backend.

**Якоря**: `project_v4/frontend/src/features/formation/FormationFilters.tsx` (новое).

## 7.4 Поиск: качество (open)

**Что сделать**: аудит — hybrid search (SQL + vector + RRF) на текущем каталоге. Ошибки ranking, миссы. Тюнинг RRF weights, re-indexing embeddings если надо.

**Якоря**: `project_v4/backend/internal/adapters/postgres/catalog_search.go`, `project_v4/backend/internal/tools/tool_catalog_search.go`.

## 7.5 Параллельные чаты, один тенант (open)

**Что сделать**: проверить что два чата в разных вкладках не шарят стейт, оба работают. Возможно issue с session cookies.

## 7.6 Параллельные чаты, разные тенанты (open)

**Что сделать**: то же, но разные tenant slugs. Проверить isolation.

## 7.7 Code quality / dead code (partial)

**Что сделать**: `go vet`, `staticcheck`, `deadcode`; на фронте — `ts-prune`, `knip`. Удалить unused.

**Закрыто частично** (phase2-cleanup, ветка): legacy `project/backend/` (~22K строк) снесено через `git rm -r`. Скрипты `scripts/start.sh`/`stop.sh`/`stop_all.sh` переключены на V4 (порт 8082), `start_v4.sh`/`stop_v4.sh` удалены как дубликаты, `ADW/adw.yaml` обновлён. Осталось: dead-code в активных модулях (V4/admin/curator).

## 7.8 Docs cleanup (done)

**Закрыто 2026-04-26**:
- DONE-баннеры в SPEC-доках: METADATA_DRIVEN_BINDING, KEEPSTAR_CANVAS_PLAN, multi_widget_handoff, admin_onboarding_gaps, V1_ENGINE_REMOVAL.
- Удалены устаревшие: BACKEND_REFACTORING_SPEC, LAUNCH_CHECKLIST, TEST_RESET_SPEC, PENCIL_VS_V4_COMPARISON.html, Engine_hustle.
- Удалён мусор: `docs/.DS_Store` (+ .gitignore), `docs/ответы`, `docs/plan10.md`.
- Этот трекер сверен с git, статусы обновлены.

**Что оставили живым** в `docs/New features/`: admin_catalog_design (текущий), PITCHLINK_SPEC (KEEP), engine_v5_v9_integration_plan (partial — design phase), marketing_triggers_and_narration_spec (partial), sales_readiness_assessment (snapshot, актуален частично), COMPOSE_SPEC (open), OWNER_DASHBOARD_SPEC (reference), PENCIL_CONVERGENCE_SPEC + PENCIL_ENGINE_V4_HYPOTHESIS + PENCIL_HYBRID_ENGINE (reference), TRACE_VISIBILITY_SPEC (reference), V1_ENGINE_REMOVAL_SPEC (open), METADATA_DRIVEN_BINDING (DONE-banner), KEEPSTAR_CANVAS_PLAN (DONE-banner), multi_widget_handoff (DONE-banner), admin_onboarding_gaps (MOSTLY DONE-banner).

---

# 8. Marketing & Sales (LAUNCH_CHECKLIST)

## 8.1 Лендинг (open)

**Что сделать**: single-page, ценность + боль + демо + CTA. Базовый есть в `Keepstar_one_landing/` — аудит, редизайн, транслировать ценность.

## 8.2 Демо ролик (open)

**Что сделать**: 60-90 сек, screen capture + voice. Сценарий: ввод в чат → рендер → лайк → модификация → покупка.

## 8.3 План продаж + портрет покупателя (open)

**Что сделать**: список 10-20 магазинов, по 2-3 per vertical. Письмо-пинг. ICP: размер каталога, regions, текущий стек.

## 8.4 Ценообразование (open)

**Что сделать**: tiers. Модель: fixed monthly + per-1k-turns overage? Или чисто usage-based? Учитывать 1¢/turn cost floor.

## 8.5 Marketing triggers в продукте (open)

**Связь**: 2.14. Marketing часть = контент триггеров (тексты скидок, промо), техчасть = 2.14.

---

# 9. Testing & Reliability (LAUNCH_CHECKLIST, дубли 6.8, 6.9)

Покрыто в 6.8 (Sentry) и 6.9 (load testing).

---

## Закрытые задачи (done — для памяти и чтобы не возникали повторно)

### Engine V4 core
- **B3** replicate explicit flag + limit → `2026-04-06_14-51.md`
- **B2** 12 named presets → `2026-04-06_15-35.md`
- **#3** multi-widget composition, 6 phases → `2026-04-07_04-11.md`
- **DefaultWidgetActions** (like, add_to_cart) auto-inject → часть B3
- **Auto-sections** post-process → Phase 3 of #3
- **Group-aware constraints** → Phase 1 of #3
- **Multi-widget tree map schema** → Phase 4 of #3
- **Tool validation + COMPOSING prompt** → Phase 5 of #3
- **Phase 6 E2E verification** → `2026-04-07_04-11.md`
- **1.2** Agent1 tenant digest в system prompt + prompt cache → `2026-04-09_01-21.md` + `2026-04-09_02-05.md`
- **ReplicateConfig persistence** (fix: `json:"-"` → `json:"replicate"`) → `2026-04-09_02-05.md`
- **B7 (4.3)** metadata-driven binding (любой каталог) → `7e15cd9, 9261327, 5566439, 2fa746e, 7caeb8f` (2026-04-10)

### Admin (auth, onboarding, billing, redesign)
- **Admin auth stack** — Google OAuth, Telegram Login, SMTP reset, refresh rotation, TOTP/Email 2FA, multi-tenant, invitations → `ef280aa` (2026-04-19/20)
- **Admin onboarding MVP** — CSV + Shopify integrations → `b8adcd1` (2026-04-19)
- **Admin redesign Pencil** — black sidebar, redesigned Chats/Catalog/Import → `be78e70` (2026-04-20)
- **Admin Billing & Usage page** — full UI + real token usage aggregation → `c9893d1` (2026-04-21)
- **Admin Catalog tree wiring + English names** → `bce6c8c` (2026-04-20)

### KeepstarCanvas (Phases 1-8) → закрывает 5.1, partial 2.12
- P1 preset CRUD backend → `4ed581c`
- P2 tenant preset loader → `de81801`
- P3 design context в Agent2 prompt → `5d561ea`
- P4 canvas UI shell + routing → `281e582`
- P5 tldraw integration + preset tiles → `0b7c314`
- P6 inspector editing + publish/delete → `4c6ea5c`
- P7 components library + save-from-trace → `12c7bba`
- P8 design tokens editor + themes → `b89bd9c`

### Прокачка пайплайна (April)
- **Streaming Agent2** — попробовали и откатили → `54cce02 + reverts e4f9a0d/0962165/23a080d` (2026-04-13). Решение: dropped, см. 4.11.
- **Prompt cache audit + Agent2 conversation cache** → `403d1fe` (2026-04-09)

### Дизайн закрыт, ждёт имплементации
- **Admin Catalog** (master/listing, candidates, mapping artifact, curator) → `docs/New features/admin_catalog_design_2026-04-23.md` + commit `c0e776c` (2026-04-23)
