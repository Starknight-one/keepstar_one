# Multi-widget composition (#3) — handoff для следующей сессии

**Дата**: 2026-04-07
**Ветка**: `feature/engine-v4`
**Последний commit**: `58148e6` (feat(v4): explicit mode param rebuild|modify)
**Статус**: обсуждение закончено, есть решение, осталось закодить

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

## Open questions для следующей сессии

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
