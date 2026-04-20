# Multi-widget composition (#3) — handoff для следующей сессии

**Дата**: 2026-04-07
**Ветка**: `feature/engine-v4`
**Последний commit**: `58148e6` (feat(v4): explicit mode param rebuild|modify)
**Статус**: ⚠️ **ОБНОВЛЕНО 2026-04-07 (вечер)** — после глубокого code review найдено 6 критических граблей которых не было в первоначальном плане. Все 11 решений приняты и зафиксированы. **См. секцию "Обновление 2026-04-07 (вечер)" ниже — она superседирует "Финальное решение" и "Open questions".** Готово к коду в следующей сессии.

---

## Где мы остановились

Прошли через gap #3 (multi-widget composition aka "фристайл / фронт по запросу") — обсудили, я увёл в overengineering (sections + replicate как свойство виджета), пользователь вернул на землю. **Решение финальное и простое, см. ниже.**

## Предыстория — что уже закрыто

Из gap list в `docs/Updates/feature-engine-v4_2026-04-04_04-07.md`:

| # | Гэп | Статус |
|---|---|---|
| 1 | limit / data slicing | ✅ B3 (`docs/Updates/feature-engine-v4_2026-04-06_14-51.md`) |
| 2 | named ops / presets | ✅ B2 (`docs/Updates/feature-engine-v4_2026-04-06_15-35.md`) |
| 3 | multi-widget composition | **⚠️ ВОТ ОНА** |
| 4 | state carry-over между turns | ✅ explicit mode rebuild|modify (`docs/Updates/feature-engine-v4_2026-04-06_19-30.md`) |
| 5 | freestyle (literal) widgets | ✅ уже работает после B3 — movement/#5 оказался ghost, атомы без fieldName не биндятся, replicate без явного флага не срабатывает |

Также сделано по ходу дела:
- README.md + CLAUDE.md обновлены под V4 реальность (`d7e2a7f`)
- docs/ root почищен, loose files в archive (раньше: `56b553a`)

## Pencil эксперимент — что я делал и что вынес

Пользователь попросил прогнать его пример ("сделай презентацию губной помады, 20 штук, скидка 30%, цена 800₽") через Pencil MCP чтобы понять как оно выглядит на практике.

### Что построил

Открыл новый `.pen` файл, одним вертикальным фреймом 480px × fit_content собрал:

1. **Hero блок** — `{Новая коллекция глянцевых помад}` + `{20 оттенков, цена 800₽, скидка 30%}` на розовой заливке
2. **Heroimage placeholder** — пустой цветной фрейм 280px высоты
3. **Stats row** — 3 карточки (Цена `800₽`, Скидка `−30%`, В наличии `20 шт`) в горизонтальном ряду через `layout: horizontal` + `gap: 12`
4. **Explainer** — заголовок `{Почему сейчас?}` + тело `{Глянцевая формула... скидка до конца недели}` на светлом фоне
5. **Gallery** — заголовок `{Все 20 оттенков}` + grid 3×2 из 6 карточек (color swatch + name + price), каждая карточка это вертикальный фрейм с квадратным color swatch 80px и двумя текстами под ним
6. **"Показать ещё 14 оттенков →"** — строка-заглушка
7. **CTA** — фиолетовая кнопка `{В корзину со скидкой}`

Batches: **3 × ~20 ops**. Pencil имеет лимит 25 ops на batch, для большой композиции надо чанковать. Pencil умеет continuation через ID'шники из предыдущего batch'а (`I("XSYHJ", ...)` ссылается на уже созданный фрейм).

### Ключевые выводы из Pencil

1. **Модель "один длинный вертикальный фрейм"** — это Pencil-специфика. У Pencil нет концепта "виджет", только frames. Всё это был **ОДИН top-level фрейм**, а внутри него стек секций через `layout: vertical` + `gap`. **Нам эта модель не подходит** — мы оперируем виджетами как самостоятельными единицами (см. решение ниже).

2. **Pencil flexbox single-axis, wrap'а нет**. Для 2×3 грида я **руками** создал 2 row-фрейма и в каждую по 3 карточки. Это ровно то же что наш движок делает через `layout: grid` + `columns: 3` — пересчитывает в row'ы под капотом.

3. **25 ops на batch × несколько batch'ей = итерация**. У нас аналог — Anthropic tool-use loop. Agent2 может в одном turn'е сделать несколько `visual_assembly` tool calls, каждый максимум в размере context budget. В теории мультипликация неограничена. На практике — помещается в один call для типичной композиции (~25 ops на презентацию с пресетами).

4. **Визуально получилось норм** — screenshot выглядел как нормальный landing block для презентации помады. Подтверждает что модель "стек виджетов разной формы" работает.

5. **Все атомы были литералами**. Данные не передавал — всё было `value: "800 ₽"`, `value: "Rouge Éclat"` и т.д. Видно как выглядит "фристайл" (в пользовательском смысле — Agent2 пишет ops руками с литералами), и где в нём могли бы стоять `fieldName` биндинги если бы данные приходили из state.

Скриншот эксперимента остался в pen file сессии (не коммичу в репо).

## Обсуждение sections vs widgets — короткая версия

Я увидел что frontend уже умеет `formation.sections[]` (`FormationRenderer.jsx:18`, CSS `.formation-composed` = vertical flex с gap 24px) и предложил композицию строить через явные секции: hero в single section, explainer в single section, gallery в grid section и т.д.

**Пользователь сказал: не так. Виджеты должны быть первичной единицей.** Аргумент: виджеты будут лежать в state и их можно будет звать по id, сохранять в избранное, копировать между turn'ами. Sections это внутренняя плумбинг, не доменная сущность. Отсюда:

- «покажи тот график что был вчера» → pull by widget_id
- «сохрани эту галерею в избранное» → сохраняем виджет
- «убери блок объяснения» → delete widget by id

Во всех сценариях единица — **виджет**, не секция.

### Моё overengineering — что я ошибочно предложил

Я ушёл в сторону "replicate как свойство виджета" — то есть галерея это **один логический виджет** с `replicate: true, limit: 20` внутри, который на рендере разворачивается в 20 визуальных ячеек. Типа чтобы в state лежала **одна запись** а не 20.

**Пользователь меня вернул**: "Чел, галерея из 20 помад это 20 виджетов через replicate, вот и всё". То есть движок работает **как сегодня** — клонирует widget template в 20 копий, каждая в `formation.widgets[]` как независимый виджет. Никаких изменений в механике replicate.

**Это критический момент.** Я хотел усложнить модель, пользователь вернул на простое решение. Галерея = N виджетов. Точка. Не "один widget с internal replicate".

## Финальное решение для #3

### Доменная модель — НИЧЕГО не меняем
- Widget остаётся как есть (`id`, `atoms`, `layout` tree, `entityRef`)
- `formation.widgets[]` остаётся плоским списком виджетов
- `formation.sections[]` игнорируется в V4 composition — не нужно
- Replication работает **как сегодня** — engine клонирует widget template в N копий

### Что добавляем — только это
1. **Multi-widget ops** — Agent2 в одном tool call может сделать несколько `insert widget` ops, вставляя разные виджеты в `formation`. Движок собирает все в `formation.widgets[]` последовательно.
2. **Per-widget preset в props** — `{op: insert, props: {type: "widget", preset: "product_card"}}`. Пресет раскрывается **локально** в scope этого виджета, а не глобально на всю formation. Корневой `preset` параметр в tool остаётся как shortcut для single-widget (сегодняшний мейнстрим).
3. **Per-widget replicate в props** — `{op: insert, props: {type: "widget", replicate: true, replicateLimit: 20}}`. Движок после `ApplyOps` проходит по widgets, для каждого с `replicate: true` клонирует его по data[0:limit]. Остальные виджеты остаются один экземпляр. Корневые `replicate`/`limit` — shortcut.
4. **EntityRef auto-detect** — движок ставит EntityRef на виджет только если у него есть хотя бы один атом с `fieldName`. Literal виджеты (hero, explainer, cta) EntityRef не получают.
5. **Замена top-level `layout`**: при multi-widget формационный `mode` переключается на что-то типа `"composed"` (либо используем уже существующий режим, проверить как `.formation-single` с >1 виджета рендерится в CSS — вероятно ok, см. `Formation.css:72`). Для single-widget оставляем текущие grid/list/single/carousel.
6. **Промпт** — новая секция **COMPOSING** с примером презентации помады (hero + stats row + gallery preset+replicate + explainer + cta). Объясняем что можно вставлять несколько виджетов разной формы в одной tool call. Preset-first остаётся нормой для простых случаев.

### Что НЕ трогаем
- `replicateWidgets` в `engine.go` — работает как сегодня, только теперь применяется per-widget-with-flag вместо global
- Frontend `FormationRenderer` — надо проверить что `.formation-single` с >1 виджета рендерит вертикально, возможно мелкая правка CSS или использование `formation-composed` класса
- Domain model Widget/Formation — без изменений
- Actions, navigation, entity handling — те же механизмы
- V1/V2 legacy в `project/backend/`

## Обновление 2026-04-07 (вечер) — после глубокого code review

После того как handoff был написан, проведена session чтения: `engine.go`, `ops.go`, `binding.go`, `constraints.go`, `tree_ids.go`, `presets_*.go`, `tool_visual_assembly.go`, `prompt_compose_widgets.go`, `FormationRenderer.jsx`, `Formation.css`, `GenericCardV2Template.jsx`, тесты + domain entities. **Найдено 6 критических граблей + 5 нюансов которые в "Финальное решение" выше не отражены**. Старая секция слишком оптимистична — реальный пайплайн в 5-6 местах молча предполагает single-widget. По итогам обсуждения с пользователем зафиксированы следующие решения. **Эта секция = source of truth для имплементации.**

### Стратегический контекст (важно для решений)

Продукт = B2B2C: бизнес загружает каталог → Keepstar нормализует → бизнес встраивает чат-виджет на сайт → юзер получает discovery+sales чат, который генерирует **всё что нужно для принятия решения** ("голограммы из фантастических фильмов"). Multi-widget composition = ядро этой метафоры (без неё движок умеет только "карточка карточка карточка").

Бизнес-ограничения:
- **Скорость**: 2-4с на пайплайн = конкурентное преимущество, терять нельзя
- **Стоимость**: ~$0.01 за turn input (Haiku + embeddings + БД), продаётся за ~$0.10 (потолок). Markup ~10×, узкая маржа
- **Следствие**: переложить логику на LLM нельзя — каждый дополнительный call ломает unit-economics. Multi-widget composition должен быть **детерминированным движком**, LLM только даёт intent

### 11 принятых решений

| # | Грабля | Файл/место | Решение |
|---|---|---|---|
| 1 | `BindData` биндит позиционно `widget[i] → data[i]` — сдвиг данных в gallery если в композиции есть literals перед replicate | `engine_v4/binding.go:14-19` | Replicate проход биндит данные **inline** в момент клонирования. Top-level `BindData` пропускает виджеты с уже заполненными values (условие в `bindWidgetAtoms:46` усилить — добавить явный флаг "bound" или гарантировать что после inline bind все values заполнены). |
| 2 | `replicateWidgets` затирает `formation.Widgets` целиком | `engine_v4/engine.go:104-120` | **Expand-in-place**: проход backwards по widgets, для каждого с replicate флагом → splice клонами на месте оригинала. Остальные виджеты не трогать. |
| 3 | Cross-widget constraint C1 удалит fields из gallery когда literals > 30% | `engine_v4/constraints.go:84-101` | **Group-aware**: replicated клоны = одна группа (метка через `ReplicateConfig` или `widget.Meta["replicateGroup"]`), literal widgets пропускаются в C1 целиком. Constraints применяются внутри группы. |
| 4 | `BuildTreeMap` показывает только первый виджет → Agent2 ослепнет к остальным в modify mode | `engine_v4/tree_ids.go:92-97` | Возвращать **массив widget templates** с группировкой: literal widgets индивидуально, replicated bunch = один template + count + ids array. Schema: `{widgets: [{id, kind: "literal", atoms: [...]}, {kind: "replicated", count, ids: [...], template: {...}}, ...]}`. |
| 5 | Refs namespace глобальный — все 12 пресетов используют одинаковые `w/root/info/meta/tags` → коллизии при per-widget preset | `engine_v4/ops.go:126` | При per-widget preset expansion engine **префиксует** все refs пресета (`<userRef>_w`, `<userRef>_root` итд). Если userRef не задан → auto `p<idx>_*`. Override-ops пользователя ссылаются на префиксованные refs. |
| 6 | Frontend `formation-single` без `flex-direction: column` → виджеты сваливаются в строку. `formation-list` тоже не подходит (zebra + horizontal cards) | `Formation.css:72-76` + `FormationRenderer.jsx` | Новый mode `"composed"` в `domain/formation_entity.go` enum. Новый CSS класс `.formation-multi` (flex column gap 24px, full width). Новая ветка в `getLayoutClass()` в `FormationRenderer.jsx`. |
| 7 | EntityRef auto-detect для одиночных entity-bound widgets в композиции | `engine_v4/engine.go` | **Гибрид**: после ApplyOps engine проходит по виджетам, для каждого с ≥1 fieldName атомом и без replicate флага — auto-присваивает EntityRef из `data[0]`. Agent2 может **override** через `{op: insert, props: {type: widget, dataIndex: N}}`. Edge case `dataIndex >= len(data)` → fallback на data[0] + warning. |
| 8 | top-level `preset` + multi-widget ops — взаимоисключение | `tools/tool_visual_assembly.go` | **Запретить** валидацией: если в `ops` есть >1 `insert widget` (или хоть один widget insert с собственным preset в props) → top-level `preset` параметр = error. Top-level preset остаётся single-widget shortcut. |
| 9 | Modify mode + multi-widget formation + новый data — конфликт повторного binding | — | **Ничего не делать в коде**. Поведение fail-safe: BindData пропускает уже заполненные атомы, новые data игнорируются. Если Agent2 ошибётся → юзер увидит stale results, переспросит, Agent2 retry'нется с rebuild. Self-validation hook отвергнут как слишком дорогой ($0.005+ за turn ломает unit-economics). |
| 10 | Где хранить replicate/preset на виджете | `domain/widget_entity.go` | Выделенный тип `*ReplicateConfig` с `json:"-"` — engine-internal, **не утекает на фронтенд** (после expand виджеты уже клонированы, флаг не нужен). Тип-безопасно. |
| 11 | Промпт на русском — переводить? | `prompts/prompt_compose_widgets.go` | **НЕ переводим**. Промпт остаётся английским (cost + Claude training bias). **Оптимизация** (компрессия, dedup) приветствуется в той же сессии что и добавление COMPOSING секции. |

### План имплементации — 5 фаз, каждая независимо тестируема

**Phase 1 — Engine pipeline foundations** (грабли 1, 2, 3, 7, 10)
1. Добавить тип `ReplicateConfig` + поле `Widget.ReplicateConfig *ReplicateConfig` (`json:"-"`) в `domain/widget_entity.go`
2. Переписать `replicateWidgets` → `expandReplicatedWidgets(formation, data, entityType)` — backwards итерация + splice in-place, inline bind + EntityRef + Actions для каждого клона
3. EntityRef auto-detect: после ApplyOps проход по entity-bound widgets без EntityRef → присвоить из `data[0]`
4. dataIndex override support: `applyInsert` для widget читает `props.dataIndex`, сохраняет в `ReplicateConfig`, engine использует при auto-bind
5. `applyCrossWidgetConstraints` group-aware: пометить replicate группы → итерация по группам, literals skip
6. Тесты: новый `composition_behavior_test.go` — composition с replicate + literals + entity widget с dataIndex; убедиться что existing single-widget tests все ещё проходят

**Phase 2 — Ops layer для per-widget preset** (грабли 5, 6 backend)
1. `applyInsert` для widget с `props.preset` — берёт `preset.Build()`, префиксует refs, инжектит в текущий ApplyOps batch на месте этого insert (engine "разворачивает" preset как inline ops)
2. `applyInsert` для widget с `props.replicate` / `props.replicateLimit` — сохраняет в `ReplicateConfig`
3. Engine emit `mode: "composed"` когда после expand widgets > 1 (либо в `Engine.Execute`, либо в `tool_visual_assembly` post-processing)
4. Тесты: insert widget с preset раскрывается в scope; два разных preset в одном tool call не пересекаются; per-widget replicate работает

**Phase 3 — Context для Agent2** (грабля 4)
1. `BuildTreeMap` переписать под multi-widget schema (массив widget templates с группировкой replicated/literal)
2. Тесты: tree map содержит все виджеты композиции; Agent2 в modify mode видит IDs всех

**Phase 4 — Tool validation + Frontend rendering** (грабли 6 frontend, 8)
1. `tool_visual_assembly.go` — валидация: top-level `preset` + multi-widget ops = error с понятным сообщением
2. `domain/formation_entity.go` — добавить `FormationTypeComposed FormationType = "composed"` в enum
3. `Formation.css` — новый класс `.formation-multi` (`display: flex; flex-direction: column; gap: 24px; width: 100%`)
4. `FormationRenderer.jsx` — добавить case `'composed'` в `getLayoutClass()` → возвращает `'formation-multi'`. Без zebra, без lazy load (или сохранить lazy load если widgets > 12 — TBD)
5. `formationModel.js` — `FormationMode.COMPOSED = 'composed'`
6. E2E: composition через testbench → визуально проверить рендер

**Phase 5 — Prompt** (грабля 11)
1. Добавить COMPOSING секцию в `prompt_compose_widgets.go` (английский, компактно — 1 пример презентации помады)
2. Прочитать весь промпт целиком, найти повторы/раздутости, ужать без потерь смысла
3. **Не переводить** на русский
4. Прогнать пайплайн на тестовом промпте "сделай презентацию помады" → проверить что Agent2 понимает COMPOSING и не ломает обычные грид-кейсы

### Что готовить заранее к следующей сессии

- Перечитать **актуальные** версии файлов на свежем коммите перед Phase 1 (между сессиями может что-то измениться) — особенно `engine.go`, `binding.go`, `constraints.go`, `ops.go`, `tree_ids.go`, `widget_entity.go`
- Подготовить behavior test fixture: composition с hero + stats row + gallery (replicate) + cta — пригодится в Phase 1 и Phase 4
- Иметь под рукой Pencil screenshot из предыдущей сессии (помада презентация) как референс что должно получиться визуально
- Помнить про cost ceiling $0.10 — если по ходу имплементации возникнет искушение "пусть LLM сделает X" → вспомнить unit-economics и закопать в детерминированный движок

## Критические файлы для правки

| Файл | Что делать |
|---|---|
| `project_v4/backend/internal/engine_v4/engine.go` | В Execute pipeline после ApplyOps: проход по widgets, per-widget replicate. Заменить глобальный `if Replicate == true && len(Widgets) == 1` на `for each widget where widget.meta.replicate == true: replicateWidget(w, data[0:w.limit])`. Остальные виджеты оставить как есть |
| `project_v4/backend/internal/engine_v4/ops.go` | `ApplyOps` — научить insert op типа widget читать `props.preset`, `props.replicate`, `props.replicateLimit` и сохранять в widget.Meta или во временные поля для engine Execute pipeline |
| `project_v4/backend/internal/engine_v4/presets.go` | per-widget preset expansion — если insert widget с `props.preset`, движок должен локально раскрыть preset ops в scope этого виджета (тот же механизм concat что в B2, но внутри виджета) |
| `project_v4/backend/internal/engine_v4/binding.go` / `constraints.go` | EntityRef autodetect: проход по виджетам, для каждого проверить наличие атомов с fieldName, ставить EntityRef только если есть |
| `project_v4/backend/internal/tools/tool_visual_assembly.go` | Убрать/упростить `preset` top-level parameter? Или оставить как shortcut. Описать в schema что preset/replicate можно класть в widget insert props |
| `project_v4/backend/internal/prompts/prompt_compose_widgets.go` | Новая секция COMPOSING с примером многовиджетной композиции. Промпт сейчас 280+ строк, **пользователь просил перечитать его на русском** при реализации — это отдельная задача внутри #3. Промпт надо переструктурировать и перевести, т.к. он сильно разрастётся |
| `project/frontend/src/entities/formation/FormationRenderer.jsx` | Проверить как `mode: single` с `widgets.length > 1` рендерится. Если плохо — добавить ветку для multi-widget composition режима или использовать класс `.formation-composed` для плоского списка виджетов (без вложенных sections) |
| `project/frontend/src/entities/formation/Formation.css:72-84` | Посмотреть `.formation-single` — у него `display: flex; justify-content: center;` + `> * { width: 100%; max-width: 400px }`. Для композиции нужно: `flex-direction: column; gap: 24px`. Возможно просто завести класс `formation-multi` аналогичный `formation-composed` но без рекурсии по секциям |

## Open questions для следующей сессии — ✅ ВСЕ RESOLVED (см. секцию "Обновление 2026-04-07 (вечер)" выше)

> Старые open questions ниже оставлены как исторический контекст. Все они закрыты в таблице 11 решений выше. Если есть противоречия — верить новой секции.

1. **Где хранить `replicate`/`preset` per-widget?** В `widget.Meta` map[string]interface{}? В выделенных полях `Widget.Replicate bool, Widget.ReplicateLimit int`? Выбор: добавить поля в struct vs использовать meta map. **Рекомендация**: выделенные поля, чище и тип-безопаснее. Добавить в `project_v4/backend/internal/domain/widget.go` или где там сидит Widget V4.

2. **`formation.mode` при multi-widget** — новый enum `"composed"` или переиспользовать `"single"`? Нужно прочитать `FormationRenderer.jsx` + CSS и принять решение. **Рекомендация**: проверить что делает `single` с несколькими виджетами; если не стекает — ввести `"composed"` и добавить CSS класс (тривиально).

3. **Top-level preset/replicate оставлять или убрать?** Сегодня это shortcut. Multi-widget требует per-widget флаги. Оставить top-level как backwards compat — пользователи / старые турны продолжат работать. **Рекомендация**: оставить, пометить в промпте как legacy shortcut для single-widget, новые примеры делать через per-widget.

4. **Промпт на русском** — user специально попросил перечитать его на русском при реализации. Сейчас промпт большой и постоянно растёт. Предложение: перед реализацией #3 **предложить полный reorg промпта + русский перевод как отдельный шаг** в начале сессии, чтобы когда мы добавим COMPOSING секцию он не утонул в 400 строках английского.

5. **Pencil batch limit аналогия**: Pencil заставляет чанковать на 25 ops. У нас такого жёсткого лимита нет, но context budget Claude Haiku на output ~4k токенов — для композиции из 6 разнородных виджетов с пресетами это вполне помещается в один tool call (~25-40 ops). Если не помещается — Agent2 делает второй tool call `mode: "modify"` в том же turn'е и добавляет остаток виджетов. **Это уже работает** после fix #4 (mode explicit). Проверить в реальности на тесте презентации.

## Что делать в начале следующей сессии

1. **Прочитать этот файл целиком**
2. **Прочитать свежо ключевые файлы** перед любыми правками (CLAUDE.md просит этого явно — "Информация в этом файле может быть устаревшей"):
   - `project_v4/backend/internal/engine_v4/engine.go`
   - `project_v4/backend/internal/engine_v4/ops.go`
   - `project_v4/backend/internal/domain/widget.go` или `formation.go`
   - `project/frontend/src/entities/formation/FormationRenderer.jsx` + `Formation.css`
   - `project_v4/backend/internal/prompts/prompt_compose_widgets.go`
3. **Предложить пользователю план** (5 фаз типа):
   - (а) Domain/struct changes (Widget.Replicate, Widget.ReplicateLimit, Widget.Preset?)
   - (б) Engine.go — per-widget replicate loop вместо глобального
   - (в) Ops.go — чтение flags из widget insert props
   - (г) Presets — local expansion в widget scope
   - (д) Frontend — `.formation-composed` для плоского multi-widget (один класс + CSS)
   - (е) Promtp — новая секция COMPOSING + русский перевод
4. **Обсудить open question про промпт** (перевод до или после #3)
5. **Код**

## Напоминание контекста

- Session в которой это обсуждалось была долгой, контекст сжимался — handoff нужен именно поэтому
- Пользователь не владеет деталями кода свободно, полагается на моё понимание архитектуры, но интуиция по продукту и архитектуре у него **очень хорошая** — когда я ухожу в overengineering он это ловит
- Простые решения побеждают. "Галерея это 20 виджетов через replicate, вот и всё" > "галерея это один логический widget с internal replication expansion"
- `CLAUDE.md` в корне проекта — там описан V4 и предупреждение перечитывать исходники
- `docs/Updates/feature-engine-v4_*.md` — там живут логи сессий по дате, формат уже устоялся, при закрытии #3 надо создать новый update с актуальным commit hash и описанием

## Контекст веток и деплоя

- Активная ветка: `feature/engine-v4`
- Push'нута: `origin/feature/engine-v4` до `58148e6`
- Прод: `v4-engine-production.up.railway.app` (из `README.md`, V4 задеплоен)
- Legacy V1/V2: `project/backend/` на `main`, не трогаем
