# v9 Integration Spec

> **Owner**: Vlad (PM, не разработчик)
> **Author**: Claude (Opus 4.7), 2026-04-29 deep-dive session
> **Source read**: `/Users/starknight/Keepstar_project/Keepstar_one_v9/` — README, ARCHITECTURE, packages/{domain,renderer,layout}, apps/web/src/{agent,store,adapters,components,hooks}, apps/api, apps/agent-service. Compared against `project_v4/backend/internal/{engine_v4,tools}`.
> **Цель**: после этого файла любая будущая сессия Claude может зайти в Phase 4 (v9 integration) без повторного чтения v9 с нуля.
>
> **Reading order**: 1 → 2 → 3 → 4 → 5 → 6 → 7. Если торопишься — секции 4 (три задачи) и 7 (оценки) самые важные.

---

## 1. Что такое v9

`Keepstar_one_v9` — **самостоятельный Figma-класс продукт**, не библиотека. Не использует наш V4 ни в каком виде. Происходит от Pencil MCP — наследует .pen schema v2.10 (поэтому 13 типов нод, $variables, themes, ref/descendants).

### 1.1. Стек и монорепо

| Слой | Технология |
|---|---|
| Менеджер | **Bun 1.2** + Turborepo 2.5 |
| Фронт | React 19 + TS, Vite, Zustand |
| Рендерер | **Raw WebGL2** + SDF shaders (instanced rendering) |
| Лейаут | **Yoga WASM** (flexbox) |
| Backend | Go + Chi router (порт **8090**, не 8080 как написано в README — см. `apps/api/cmd/server/main.go:149`) |
| Agent backend | Bun + Hono + SSE (порт **8091**) |
| Хранилище документов | File adapter (dev) + Postgres adapter (prod) |

Хочу подчеркнуть: v9 — это **standalone-приложение**. У него свои порты, свои документы, своя БД-схема. Никаких пересечений с нашей `chat_*` или `catalog.*` нет.

### 1.2. Структура

```
keepstar_one_v9/
├── packages/
│   ├── domain/      # Чистый TS, zero deps. Нодная модель + операции + сервисы (scene-graph, variable-resolver, component-resolver, id-generator).
│   ├── renderer/    # WebGL2 — rect/ellipse/line/polygon/path/text/image/shadow/blur. Один canvas, один контекст.
│   └── layout/      # Yoga WASM — превращает дерево CanvasNode в Map<id, ComputedRect>.
└── apps/
    ├── web/         # React-приложение редактора (Canvas.tsx, store, агент)
    ├── api/         # Go REST: CRUD документов, WS-hub, PDF-экспорт, agent-logs.
    └── agent-service/  # Server-side агент (Bun+Hono) — альтернатива браузерному агенту, для случаев когда ANTHROPIC_API_KEY нельзя класть в браузер.
```

### 1.3. Доменная модель (`packages/domain/src/`)

Корневой объект:
```ts
interface Document {
  version: '2.10'
  themes?:    Record<string, string[]>          // { 'color-mode': ['light','dark'] }
  imports?:   Record<string, string>
  variables?: Record<string, VariableDef>       // $primary, $surface, $borderRadius, ...
  children:   CanvasNode[]                      // дерево
}
```

**13 типов нод** (`entities/nodes.ts`):

| Тип | Что это |
|---|---|
| `frame` | Контейнер с layout (flex/none), детьми, clip, slot, cornerRadius |
| `group` | Лёгкий контейнер с эффектами, без своего фона |
| `rectangle`, `ellipse`, `line`, `polygon`, `path` | Примитивные формы (SDF в шейдерах) |
| `text` | Текст с TextStyle + textGrowth (auto/fixed-width/fixed-width-height) |
| `icon_font` | Иконки lucide/feather/Material Symbols/phosphor |
| `note`, `prompt`, `context` | Аннотации (текстовые, не рендерятся в обычном UI) |
| `ref` | Инстанс переиспользуемого компонента: `{ref: "compId", descendants: {childId: {prop:val}}, ...rootOverrides}` |

**Operations** (Command-паттерн с undo/redo) — `operations/`:
- `InsertCommand(parentId, nodeData)` — вставить ноду; `parentId=null` = корень документа.
- `UpdateCommand(nodeId, props)` — патч свойств (snapshot oldProps для undo).
- `DeleteCommand(nodeId)` — удалить.
- `MoveCommand(nodeId, newParentId, index?)` — переместить.
- `SetOverrideCommand(refId, targetChildId, props)` — переопределить свойство потомка инстанса (не источника).
- `SetRootOverrideCommand(refId, props)` — переопределить корневое свойство инстанса.
- `CommandHistory` — undoStack/redoStack.

**Services**:
- `scene-graph.ts` — `findNodeById`, `findParent`, `walkNodes`, `findReusableNodes`, `findRefsToNode`, `findSlots`.
- `component-resolver.ts` — `expand(refNode, doc)`: deep-clone источника, наложение root-overrides, descendants-overrides, обработка slot-children, рекурсивный вход в nested refs (depth limit 10).
- `variable-resolver.ts` — `resolveColor/Number/Boolean/String` против `Document.variables` + `activeThemes`. Поддерживает chained vars (с защитой от циклов) и themed-arrays.
- `id-generator.ts` — 5-значные id.

**Ports**: inbound (EditorPort, DocumentPort, QueryPort), outbound (StoragePort, RendererPort, LayoutPort, ImagePort).

### 1.4. AI-агент (`apps/web/src/agent/`)

Это самостоятельный агент, **не наш Agent2**. Работает прямо в браузере (через `anthropic-dangerous-direct-browser-access: true`) или серверно через `apps/agent-service`.

**Модель**: `claude-haiku-4-5-20251001` по умолчанию, max_iterations=40.

**Кеширование**: STATIC_CORE prompt на 1h TTL + tools на тот же breakpoint. DOCUMENT_CONTEXT (current doc state) на 5m TTL. Использует `extended-cache-ttl-2025-04-11` beta-header.

**11 тулов** (`tools/index.ts`):

| Tool | Назначение |
|---|---|
| `get_editor_state` | Текущая selection + documentId + topLevelCount. Все компоненты/переменные уже в system prompt — этот tool НЕ возвращает их. |
| `batch_get` | Поиск/чтение нод по `nodeIds`, `patterns` ({name regex, type, reusable}), с `readDepth`. |
| `batch_design` | **Главный тул.** Выполняет batch операций (≤25/вызов) на DSL-строках. См. ниже. |
| `get_guidelines` | Загрузка структурных гайдов (`guides/`: web-app, mobile-app, dashboard, form, table, design-system, slides, code, tailwind, landing-page) и стилей (`styles/`: 30+ MD-файлов с визуальными направлениями). Стили могут требовать `params` (цветовая палитра, шрифт). |
| `snapshot_layout` | **Дешёвая структурная проверка.** Текстовый JSON: размеры, перекрытия, флаги проблем. По умолчанию использовать ЭТО, не `get_screenshot`. |
| `find_empty_space` | Найти координаты для вставки нового top-level фрейма. |
| `get_variables` / `set_variables` | Чтение и запись `Document.variables`. |
| `get_screenshot` | **Дорогая визуальная проверка.** Imagetokens. Hard cap 20/run. Только когда нужно судить контраст/иерархию/композицию. Никогда — для проверки структуры. |
| `search_replace_properties` | Bulk replace свойств по совпадению. |
| `export_nodes` | Экспорт PNG/SVG/PDF выделенных нод. |

**`batch_design` DSL** — самое интересное (`tools/batch-design.ts`). Каждая операция — строка вида:
```
binding=I(parentRef, { nodeProps })       // Insert
binding=C(srcRef, parentRef, { overrides? })  // Copy
U(pathRef, { propChanges })                // Update
binding=R(pathRef, { newProps })           // Replace
D(ref)                                     // Delete
M(ref, newParentRef, index?)               // Move
G(nodeRef, "ai"|"stock", "≥30-word prompt")  // Generate image as fill
```

Особенности:
- **Run-scoped bindings**: `runBindings` персистят между вызовами `batch_design` в одном агентском run'е. `resetBindings()` дёргается в `AgentRunner.run()`. Это даёт агенту возможность сослаться на ноду из предыдущего batch'а.
- **Path syntax `binding+"/childId"`**: разрешает в id потомка по `id`/`name`/endsWith.
- **Slot insertion**: `I("instance/slotKey", {...})` мутирует `refNode.descendants[slotFrameId].children`, не источник. Не "протекает" в другие инстансы.
- **Inline children sugar**: `I(parent, {type:"ref", ref:"X", children:[...]})` транслируется в `descendants[slotFrameId].children` (требует ровно 1 slot frame).
- **Post-batch validator** (`validateDirtyNodes`): после успешного batch'а проверяет `dirtyIds` на типичные ошибки (`fill_container` без flex parent, x/y в flex parent, текст без fill, frame с `fit_content` и 0 детей, переполнение по ширине, cornerRadius > halfMin, ref на несуществующий target). Возвращает `Potential issues detected` в Pencil-style markdown — агент видит и фиксит на следующем turn'е.
- **Rollback**: failed batch откатывает И document, И bindings.

**System prompt** (`system-prompt.ts`): 220 строк. Включает workflow, batch DSL ref, design principles (purpose & hierarchy, content, system state, composition, typography, CTA, mobile-specific), placeholder protocol, color format, distilled node schema. Это полноценная "конституция" дизайнера. Для нашего chat-кейса 70% этих принципов — over-engineering (в чате нет screen hierarchy, hero, CTA conversion, narrative arc).

### 1.5. Сетевая поверхность

| Endpoint | Сервис |
|---|---|
| `GET/POST/PUT/DELETE /api/documents[/:id]` | apps/api (CRUD) |
| `GET /api/documents/:id/export/pdf` | apps/api (gofpdf) |
| `GET /ws/documents/:id` | apps/api (WS hub, заготовка под коллаборацию) |
| `PUT/GET /api/agent-logs/:id` | apps/api (стор для дебага runs) |
| `POST /api/agent/run` | apps/agent-service (SSE-стрим run'а) |

---

## 2. Что v9 умеет лучше нашего V4

### 2.1. Ops-модель — на голову выше

| | V4 ops | v9 ops |
|---|---|---|
| Уровни дерева | 3 жёстких: Formation → Widget → LayoutNode → Atom | 1 рекурсивный: CanvasNode → CanvasNode |
| Семантика операций | `insert/update/delete/move/replace` с `target/parent/after/ref/props`, JSON-форматом | Та же семантика, но через DSL-строки + run-scoped bindings + slot-paths |
| Reuse | 12 хардкод-пресетов в Go (`presets_product.go` и т.д.), фильтрованы под косметику | Любая нода с `reusable:true` — компонент. Инстанс через `ref` + `descendants{}` overrides + slots |
| Undo/redo | Нет в движке (это движок-без-состояния, undo живёт где-то на фронте) | First-class CommandHistory в `domain/operations/command.ts` |
| Validation | `ApplyConstraints` — нормализация по правилам | Post-batch validator — даёт **советы** агенту в естественном формате, не правит молча |
| Variables/themes | Marketplace theme через CSS variables на фронте | First-class `$varName` + multi-theme массивы в нодах + Resolver service |

Самое ценное: **v9-овский RefNode + descendants** даёт реальную compositional model. Один раз создаёшь `product_card` как reusable, дальше ставишь N инстансов с разными overrides. Наши пресеты — статика в Go.

### 2.2. Renderer

WebGL2 SDF-рендерер умеет рендерить тысячи примитивов в один draw call, с pixel-perfect геометрией на любом zoom'е. Наш FormationRenderer — это React DOM, ограничения те же что у любого DOM (perf падает после ~500 элементов, нет true zoom без CSS-трюков).

Для канваса (фаза 4 задача "a") это решающее преимущество — Tldraw в админке еле тянет, v9 тянет.

Для chat-виджета это **не преимущество** (см. 3.2).

### 2.3. AI-agent: prompt + DSL + verification loop

- DSL-форма ops (`b=I(p, {...})`) короче и читаемее JSON'а — экономит токены.
- Run-scoped bindings снимают необходимость прокидывать ID между вызовами.
- Pencil-style markdown в ответе тула (а не raw JSON) лучше ложится на тренировочный распределение Haiku — модель меньше галлюцинирует на следующем turn'е.
- `snapshot_layout` vs `get_screenshot` — раздельные тулы для структурной vs визуальной верификации. Это **тонкая, но важная штука**: наш Agent2 такого разделения не имеет, проверки нет вообще, ошибки видны только когда фронт отрисует.
- Post-batch validator с warnings — агент учится на своих ошибках в рамках одного run'а.

### 2.4. Style guides и principles

`apps/web/src/agent/guidelines/` содержит 10 структурных гайдов и 30+ визуальных стилей. Для landing-кейсов это полноценная база. Для chat-виджета — лишнее (см. 3.4).

### 2.5. Cost-conscious архитектура

- 1h cache breakpoint на STATIC_CORE+tools, 5m на DOCUMENT_CONTEXT.
- Hard cap 20 screenshots/run.
- README заявляет "~7 cents per design on Haiku 4.5".
- Cache_creation/cache_read метрики прокидываются в `onApiResponse` callback — можно строить dashboard.

Наш V4 уже использует prompt caching, но без 1h-варианта и без отделения static/dynamic.

---

## 3. Что v9 НЕ умеет — то, что V4 делает критически

### 3.1. **Раскладка реальных данных каталога — нет вообще**

Это самый главный gap. v9 — про **визуальный канвас**, не про **связь UI с данными**.

В V4: Атом имеет `FieldName` (e.g. `"price"`). На стадии `BindData` движок проходит атомы, и где `FieldName != ""`, подставляет `entityData[FieldName]`. Replicate-флаг + `expandReplicatedWidgets` клонит widget-template ×N для N продуктов. EntityRef проставляется автоматически — фронт знает, что виджет = продукт `id=X`, и роутит клики на back/expand/cart правильно.

В v9: Нода содержит **литеральный `content: "Hello"`**. Никакого `FieldName`. Никакой replicate-семантики. Никакого `entityRef`. Если агент хочет 8 product-карточек — он эмитит 8 `I(...)` с уже подставленными названиями, ценами, картинками. Это работает для дизайна одной screen'ы, но в chat-flow это поломка:

- 8 LLM-операций на каждый чат-запрос вместо 1 replicate.
- Нет связи node→entity, нет роутинга кликов.
- Нет diff'а с previous state, чтобы Agent1 знал что показано.

### 3.2. **Embedded chat rendering — не его юзкейс**

v9-renderer = canvas-приложение в полный экран. Нашему чат-виджету нужно:
- Shadow DOM (не пропустить стили хост-страницы)
- Адаптивная высота bubble (не fixed canvas)
- Accessibility (a11y attrs на каждом элементе — на canvas'е их нет вообще)
- Lighthouse-совместимый рендеринг (canvas не индексируется)
- Partial render (одно сообщение = маленькое дерево, не вся документ-страница)
- Theme inherits from host page

Канвас на этих требованиях буквально работает в обратную сторону. Поэтому: v9-renderer **не подходит для chat widget**. Только для админки/курaтора.

### 3.3. **Backend pipeline / Agent1 / catalog_search / state**

Тривиально: у v9 нет понятий sessionState, catalog_search tool, state_filter, history_lookup, navigation/expand, navigation/back. Всё это живёт у нас и должно остаться.

### 3.4. **Data-driven layout decisions**

Наш V4 умеет: `layout=grid, columns=4` если data.count=8 (DefaultsEngine). v9 этого не делает — агент сам решает абсолютно всё.

### 3.5. **Server-side render output**

v9 рендерит в браузере. У нас layout определяется на сервере (в Formation JSON), фронт — тупой переводчик в DOM. Если перейти на v9, либо ставим Yoga WASM в чат-фронт (~200 KB), либо реализуем Yoga в Go (биндингов нет).

---

## 4. Три интеграционные задачи

> Каждая задача описана как: **что подменяется** → **где границы** → **какие пакеты** → **открытые вопросы**.

### 4.a) Заменить канвас в админке на v9-ный

**Что сейчас**: `project_admin/frontend/src/features/canvas/CanvasPage.jsx` (785 строк) использует [tldraw](https://tldraw.com) с custom shape'ом `PresetTileShape` для отображения пресетов как тайлов. Tldraw тяжёл, плохо подходит под "редактирование UI", по факту он у нас используется как board с превью пресетов, а не как настоящий редактор.

**Что подменяется**: всё содержимое `features/canvas/CanvasPage.jsx`. Вместо Tldraw — v9-овский Canvas + Inspector + LayerTree + Toolbar + AgentChat.

**Где границы**:
- Внутри: всё что v9 рисует на канвасе — нодная модель, редактирование, undo, экспорт.
- Снаружи: admin auth, admin shell (sidebar, header), tenant scoping, integration с нашими публикуемыми пресетами для V4.

**Какие пакеты v9 нужны**:
- `@keepstar/domain` (npm/workspace)
- `@keepstar/renderer`
- `@keepstar/layout`
- React-компоненты из `apps/web/src/components/{Canvas, Inspector, LayerTree, Toolbar, AgentChat, ResizablePanel, VariableEditor, LeftPanelTabs, ...}`
- Zustand store + hooks

**Два пути**:
1. **Iframe + postMessage**. Поднимаем `apps/web` отдельным сервисом, в админке `<iframe src="canvas-app">`. Между ними — postMessage-протокол: load(docId), export, save. Плюсы: минимальная связь, v9-обновления не ломают админку. Минусы: разные origin, два build'а, авторизация через токен в URL.
2. **Прямое внедрение**. Скопировать (или yarn-link) пакеты в admin, монтировать `<Canvas>` внутри admin страницы. Плюсы: один build, общая auth. Минусы: глобальный Zustand store конфликтует с другими страницами админки; v9 ApiClient полагается на `VITE_API_URL=http://localhost:8090` — придётся перепиливать на admin backend; компонент `useDocumentLoader` читает `window.location.search?doc=X` — это надо менять.

**Открытые вопросы**:
- Документы v9 (его JSON Document) хранятся отдельно от наших presets? Или мы хотим, чтобы admin canvas редактировал именно те JSON'ы, что движок V4 потом использует? Если ДА — это значит задача (a) не самостоятельна, она зависит от (b). Если НЕТ — (a) это просто "красивый канвас для дизайна", без интеграции в чат. Vlad'у решать что выбрать.
- Нужна ли коллаборация (WebSocket)? У v9 заготовка есть, у нас в админке — нет.
- Фонты/иконки — у v9 свои MSDF атласы в `fonts/` (lucide, feather, Material Symbols). Объём ~10MB. Для админки норм, для виджета — нет.

### 4.b) Перенести ops-модель в движок V4

**Что подменяется**: вся "формационная" модель в `project_v4/backend/internal/domain/` (Formation, Widget, LayoutNode, Atom, Section, EntityRef, ReplicateConfig, FormationWithData) → CanvasNode-tree. Соответственно — `engine_v4/{ops, binding, constraints, expand, presets, presets_*, sections}` под переписку.

**Что выживает (потому что v9 этого не имеет)**:
- `BindData` как идея (FieldName → data binding) — обязательно остаётся, либо как отдельный pre-process pass перед persist'ом ноды, либо как расширение схемы CanvasNode полем `binding: {field: "price"}`.
- Replicate как идея — остаётся, реализованная через "для каждого data[i] инстанцируем ref `product_card` с descendants из data[i]".
- EntityRef — остаётся как metadata на инстансе ref, чтобы front знал какой product закреплён за нодой.

**Что умирает**:
- Жёсткая иерархия Formation/Widget/LayoutNode (заменяется единым CanvasNode-деревом).
- 12 хардкод-пресетов в Go (заменяются reusable нодами в JSON-документе пресетов).
- LayoutNode типы (`row/column/flow/span`) — заменяются `frame.layout: vertical/horizontal/none` + Yoga-семантикой.
- Слойшеры в `engine_v4/sections.go` (group widgets in sections) — у v9 это просто frame с детьми.
- `ApplyConstraints` нормализация — частично заменяется post-batch валидатором (но валидатор только warnings, не правит, надо договориться).
- `BuildTreeMap` (compact context для Agent2) — заменяется v9-овским `get_editor_state` + `batch_get`.

**Какие пакеты v9 нужны на бэке (Go)**:
- Go-структуры из `apps/api/internal/domain/document.go` — это уже копия v9-схемы для Go (CanvasNode + Document). Можно взять as is.
- `component-resolver` — переписать на Go (короткая функция, ~150 строк).
- `variable-resolver` — переписать на Go (короткая, ~120 строк).
- `scene-graph` (find/walk/findParent) — тривиально на Go.
- `id-generator` — тривиально (5 base36 chars).

**Что НЕ берём**: операционный command-pattern (это для undo/redo в редакторе, в чат-движке undo не нужен — есть session.history).

**Mapping V4 → v9 примеры**:

| V4 | v9 эквивалент |
|---|---|
| `Formation{mode:grid, grid:{cols:4}, widgets:[...]}` | `frame{layout:'horizontal', children:[ref(product_card)×4]}` (или `vertical` + manual cols, либо CSS Grid в кастомном frame) |
| `Widget{template:'product_card', size:'medium', atoms:[{fieldName:'price',...}]}` | `ref{ref:'product_card', descendants:{priceText:{content:'$$field:price'}}}` + post-process binding |
| `Atom{type:'text', subtype:'currency', display:'price', format:'currency', slot:'price', value:299}` | `text{content:'$299', fontSize:18, fontWeight:'600', fill:[{type:'color', color:'$accent'}]}` (всё уже разрешено) |
| `ReplicateConfig{enabled:true, limit:8}` + 1 widget template | 8 × `ref{ref:'product_card', descendants:{...data[i]...}}` (распакованные на бэке) |

**Открытые вопросы (важные)**:
- **Куда идёт data binding?** Три варианта:
  - (i) Расширить v9 schema: `CanvasNode.binding?: {field: string}`. Перед persist'ом сервер заменяет на literal value. Но тогда инспектор/редактор v9 должен это понимать.
  - (ii) Магическая convention: `content: "$$field:price"` парсится бэком и заменяется. Простое, но грязное.
  - (iii) Server-side post-process: агент эмитит template-ноду с placeholder'ами, отдельный pipeline-шаг применяет data. Чище но больше кода.
  - **Я бы выбрал (i)** — но это инвазивно к v9-source-of-truth.
- **Где живут пресеты после миграции?** Это reusable-ноды в неком "глобальном" документе? Или отдельный store? Нужна tenant-scope (как сейчас в `tenant_preset_loader.go`).
- **Constraints оставляем?** Post-batch валидатор v9 даёт warnings, не нормализует. Часть нашего ApplyConstraints критичны (приведение текста к лимитам, нормализация цвета). Их надо поднять выше валидатора.

### 4.c) Решение по AI-agent

Это **не "что подменить"**, а **"что выбрать"**. Три варианта.

**Вариант 1: Оставить наш Agent2**

Что: Agent1 (NLU) и Agent2 (Render) остаются, но Agent2 учится эмитить v9 CanvasNode-ops вместо V4-ops.

Плюсы:
- Нулевой риск регресса по NLU/data flow — Agent1 не трогаем.
- Контроль над промптом, knows about catalog.
- Нет style-guides → дешевле в input tokens.

Минусы:
- Теряем готовый verification loop (snapshot_layout, get_screenshot, post-batch validator).
- Теряем style guides (но мы их и не используем).
- Надо переписать system prompt Agent2 (~600 строк) под v9-схему.
- Самим строить ops-DSL и парсер (или использовать v9-овский на сервере).

**Вариант 2: Взять v9-агента полностью**

Что: Agent1 остаётся (ему ничего не надо). На месте Agent2 — v9 system prompt + 11 тулов + DSL.

Плюсы:
- Готовая, обкатанная, cost-optimized архитектура.
- Verification loop работает из коробки.
- Style guides бонусом для curator-кейса.

Минусы:
- v9-агент **ничего не знает про каталог, FieldName, EntityRef, navigation**. Надо учить — а это переписывание system prompt'а.
- Style guides и design principles в STATIC_CORE — для chat-кейса 70% мусора, грузим cache.
- v9 рассчитан на одиночные дизайн-сессии (40 итераций!), наш chat — 1-2 итерации/turn. Iteration cap уберём, но prompt всё равно "дизайнерский".
- Надо адаптировать тулы (batch_design в browser → batch_design на сервере, потому что у нас агент серверный).
- Cost оптимизация v9 (1h cache на STATIC_CORE) хороша только если STATIC_CORE стабилен. После наших изменений — пересборка кэша.

**Вариант 3: Гибрид**

Что: Берём от v9 — DSL parser, ops-applier, post-batch валидатор, batch_get/get_editor_state. Не берём — system prompt, style guides, screenshot tool. Пишем свой узкий system prompt под chat-кейс, но используем v9-овскую infrastructure для исполнения ops.

Плюсы:
- Лучшее от обоих.
- Тонкий system prompt, дешёвый input.
- Verification loop сохраняется.

Минусы:
- Самый дорогой по разработке.
- Сложно поддерживать — пишем свою прокладку между нашим prompt'ом и v9-implementation.
- Риск рассинхрона: v9 апдейтится, наш prompt не учитывает новые тулы или изменения DSL.

**Моя рекомендация (явно subjective)**: **Вариант 3 (гибрид)**, но с фазированием:
1. Сначала Вариант 1 (минимальная перепись Agent2 под новые ops). Это разблокирует фазу 4(b) — без него движок не получит ввода.
2. После стабилизации — добавить v9-овский post-batch validator как отдельный pipeline-step (warnings в ответ Agent2, проходим ещё одну итерацию если нужно).
3. По мере необходимости — добавлять отдельные v9 тулы (`batch_get` для inspect existing formation в "modify" режиме).

Не брать `style guides`/`get_screenshot` совсем — они для landing-агента, не для chat-агента.

---

## 5. Зависимости между задачами

```
(a) Admin canvas      ──independent──>  можно делать ANY TIME
                          │
                          └─ если решим что admin canvas редактирует
                             ИМЕННО pre-defined пресеты для V4-engine,
                             то (a) blocked-by (b)

(b) Ops migration    ──blocks──>  (c) AI-agent
                                  (агент эмитит то, что движок ест)

(c) AI-agent         ──blocks──>  готовность фазы 4 в целом
```

**Практический порядок** (если решено что admin canvas = standalone дизайн-инструмент):
1. (a) сначала — **самое маленькое**, даёт быстрый win'a, разогревает команду на v9-stack.
2. (b) основной кусок — **самый большой**, переписка движка.
3. (c) после (b) — без движка агенту нечего эмитить.

**Если admin canvas = редактор для presets V4**:
1. (b) сначала.
2. (a) и (c) параллельно после (b).

---

## 6. Риски

В порядке убывания неопределённости.

### 6.1. **(b) Data binding — нет очевидного места для FieldName** 🔴

Это **главный риск всей фазы 4**. v9-схема идеологически противоречит data binding'у. Все три варианта (расширение схемы, convention, post-process) имеют недостатки. Ошибка здесь ломает всё: Agent2 не сможет генерировать data-driven UI, replicate перестанет работать.

Mitigation: на старте (b) делаем proof-of-concept на одном пресете (`product_card`), доводим binding до прода, только потом мигрируем остальные.

### 6.2. **(b) Server-side layout — Yoga в Go нет** 🔴

V4 решает layout server-side (отдаёт фронту готовое дерево с layout-параметрами, фронт переводит в CSS). v9 — client-side через Yoga WASM.

Если оставляем server-side: пишем sub-set Yoga на Go (boring, но реально, формула flexbox известна), либо CGo binding к C-Yoga (есть, но добавляет CGO в build).

Если переходим на client-side: чат-фронт получает 200KB Yoga WASM. Холодный старт виджета сейчас ~30KB всего. Это **6-кратный рост**. Может сломать "single script tag" обещание.

Mitigation: перед началом (b) — спайк, замерить bundle size с Yoga и без, замерить latency на 8 product cards.

### 6.3. **(c) v9-агент не знает про каталог** 🟠

Если идти Вариант 2 (полный перенос v9-агента), system prompt надо учить о Agent1, FieldName, replicate, EntityRef, navigation/expand/back. Это **переписать STATIC_CORE целиком**. Кэш греется заново, любое изменение — invalidation.

Mitigation: идти Вариант 1 или 3, не 2.

### 6.4. **(a) WebGL2 в Shadow DOM / iframe** 🟠

Если решим встроить v9 canvas в admin страницу как сабкомпонент — WebGL2 контекст должен корректно создаваться/уничтожаться при размонтировании. У v9 это работает в standalone, но в admin shell с навигацией между страницами — не уверен. Memory leaks реальны.

Mitigation: путь iframe сильно безопаснее.

### 6.5. **v9 "90% готов" — неизвестные баги** 🟠

Vlad сказал "допилен на 90%". README заявляет все 9 фаз done. Реальность где-то посередине. По коду видно: agent-service — placeholder (`session logs in-memory map`), WebSocket hub — заготовка, PDF export использует gofpdf без custom fonts.

Mitigation: при старте каждой задачи — отдельная сессия "smoke test v9 на нашем юзкейсе", прежде чем интегрировать.

### 6.6. **Порт 8080 / 8090 / 8091 конфликт** 🟢

V9-api заявлен на 8080 в README, по факту на 8090 (`main.go:149`). Наш chat backend на 8080. Если в одной dev-машине поднимаем оба — окей. На проде — раздельные сервисы.

Mitigation: тривиальное.

### 6.7. **v9 — отдельный репозиторий** 🟢

`/Users/starknight/Keepstar_project/Keepstar_one_v9/.git` — это отдельный repo. Vendor'ить в наш monorepo? Включать subtree? npm-link во время dev?

Mitigation: решить организационно. Самое чистое — npm-link на dev, фиксированная версия артефактов в prod.

---

## 7. Оценки объёма

> **Единица**: 6-часовая сессия. Без округления вверх. Vlad сказал "не приукрашивай" — оценки честные, основаны на: размер кода (LOC), известные unknowns, опыт что любой "5-минутный фикс" в v9 быстро становится 30 минутами из-за незнакомого стэка.

### (a) Admin canvas swap

| Путь | Оценка | Что входит |
|---|---|---|
| **Iframe + postMessage** | **3–5 сессий** | Поднять v9-app standalone, обернуть в iframe в админке, реализовать postMessage-протокол (load/save/export), интегрировать с admin auth (token в URL), стилизация под admin theme. |
| **Прямое внедрение** | **5–8 сессий** | Всё то же + борьба с глобальным Zustand store, переписка `useDocumentLoader`/`api-client.ts`/auth, лечение WebGL context lifecycle, фигурное мерж-в-build. |

Рекомендую iframe-путь.

### (b) Ops migration

**15–25 сессий.** Раскладка:

| Подзадача | Сессии |
|---|---|
| Доменная модель: переписать Formation/Widget/Atom → Document/CanvasNode в Go | 1–2 |
| Поты переписать: `engine_v4/ops.go` (сейчас 1361 строк) под новую схему | 4–6 |
| `binding.go`/`expand.go` — выбрать механизм binding'а, реализовать | 2–3 |
| `constraints.go` — что от него остаётся, что мигрирует в post-batch validator | 1 |
| Migrate 12 пресетов в reusable-ноды (JSON документы или Go-builders) | 2–3 |
| `tool_visual_assembly.go` — обновить tool definition под новую ops-схему | 1 |
| Frontend FormationRenderer → новый CanvasNodeRenderer (DOM, не WebGL!) | 3–5 |
| Layout: или Yoga server-side в Go, или Yoga WASM в чат-фронт | 2–3 |
| Тесты: composition_behavior_test.go (1014 строк) полностью переписать | 1–2 |

Это диапазон при отсутствии **сюрпризов в data binding**. Если упрёмся в (6.1) — добавь ещё 5–8 сессий.

### (c) AI-agent

| Вариант | Сессии |
|---|---|
| Вариант 1 (минимум, наш Agent2 эмитит v9 ops) | **3–5** |
| Вариант 3 (гибрид, добавляем v9 validator + batch_get) | **6–10** |
| Вариант 2 (полный перенос v9-агента + adapt под каталог) | **8–14** |

Рекомендую V1 → итеративно V3 (см. секцию 4c).

### Итого по фазе 4

При плане iframe + V1: **3 + 15 + 3 = 21 сессия минимум**, **5 + 25 + 5 = 35 сессий максимум**. ~3–6 недель чистого кодинга.

При наихудшем сценарии (deep embed + sticky data binding + V3-агент): **8 + 33 + 10 = 51 сессия**. ~9 недель.

**Это совпадает с тем, что Vlad сказал в LAUNCH_ROADMAP** ("каждый проект ≈ размер всего рефакторинга каталога", + 1.5 месяца на пост-стабилизацию). Roadmap честно отражает реальность.

---

## 8. Что прочитать перед стартом фазы 4

Если зашёл в сессию и начинаешь интеграцию — прочитай в порядке:

1. **Этот файл целиком** (ты сейчас здесь).
2. **`/Users/starknight/Keepstar_project/Keepstar_one_v9/docs/ARCHITECTURE.md`** — короче 200 строк, точная карта.
3. **`packages/domain/src/entities/{document,nodes}.ts`** — нодная схема, 250 строк суммарно.
4. **`packages/domain/src/operations/insert.ts` + `update.ts`** — паттерн команд.
5. **`packages/domain/src/services/component-resolver.ts`** — как ref/descendants/slots на самом деле работают (наследование данных в инстанс).
6. **`apps/web/src/agent/system-prompt.ts`** — для понимания DSL, design principles, верификационного цикла.
7. **`apps/web/src/agent/tools/batch-design.ts`** — как DSL парсится и применяется. Пример run-scoped bindings.
8. **`project_v4/backend/internal/engine_v4/{engine,ops,binding}.go`** — для сравнения с v9.

Не надо читать: `packages/renderer/src/**` (WebGL детали — не нужны если рендерим в DOM), `apps/web/src/components/Canvas.tsx` (только если делаешь задачу `a`), `fonts/`, `docs/insights/`, `docs/specs/` (старые планы v9, не наш контекст).

---

## 9. История

| Дата | Кто | Что |
|---|---|---|
| 2026-04-29 | Claude (Opus 4.7) | Первая версия. Phase 3 deep dive. Чтение v9 на ~2 часа, синтез в эту спеку. |
