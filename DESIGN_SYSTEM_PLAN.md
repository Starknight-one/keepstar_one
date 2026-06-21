# Design System — видимая, редактируемая, единый источник

Статус: в работе (старт 2026-06-16). Этот файл — живой план, владелец правит его сам.

## Цель (подтверждено владельцем)

1. **Видеть** всю дизайн-систему в одном месте (цвета, кнопки, формы, шрифты, компоненты).
2. **Править элемент дизайн-системы** по клику → попадаем в канвас → правим (ЛЛМ или руками) → изменение расходится везде, где элемент используется.
3. **Править пресеты** так же — открыть в канвасе, поменять как надо.
4. Сделать **по-настоящему**: один источник правды, чтобы правка реально доезжала до продукта. Не заплатка.

## Сошлись на архитектуре (3 решения)

1. **Источник токенов двухслойный.**
   - *Тема тенанта в v5* — то, что реально читает рантайм и инжектит как `--kw-*`. У каждого тенанта своя.
   - *Публичная полка* тем/дизайн-систем (и пресетов) — глобальный слой с copy-логикой. «Скачать» (бесплатно) / «купить» = копия строк падает в тенант, дальше она его, правит как хочет. Та же модель, что отложенный preset-маркетплейс — теперь общая для пресетов **и** дизайн-систем. Провенанс закладываем сразу, покупку включаем позже.
2. **Канвас — единственная поверхность редактирования** (токены и пресеты). «Увидеть всю систему» = canvas-документ-витрина (style guide board): смотришь и правишь там же. Чего в канвасе не хватит — дорабатываем.
3. **14 встроенных системных сидов — путь А:** рисуем им canvas-исходники один раз, они же становятся первым наполнением публичной полки. Без лоссового обратного конвертера.

## Что уже есть (по факту кода, рекогносцировка 2026-06-15/16)

- Round-trip «дизайн в канвасе → пресет → правка → перерегистрация» работает; оригинальный v9-документ хранится в `admin.canvas_designs.doc_json` и связан с пресетом через `admin.canvas_registrations`. → «открыть канвас-пресет в канвасе» почти бесплатно.
- Движок наполовину готов под токены: есть `Document.Themes`, `Document.Variables`, `VariableResolver` (резолвит `$variable` на рендере) — но это design-time, ни пер-тенант хранилища, ни рантайм-инъекции.
- `--kw-*` захардкожены в `project_v5/frontend/src/widget.css:16-46`, override-пути нет.
- `keepstar-admin/backend/internal/canvas/translate.go:124-127` выбрасывает `variables/themes` (эмитит только `{version, children}`).
- В админке UI дизайн-системы нет.

## План постройки (по зависимостям, каждый шаг проверяем отдельно)

- [x] **Фаза 1 — рантайм темы (KEYSTONE).** ✅ СОБРАНА 2026-06-16, **ЗАДЕПЛОЕНА НА ПРОД 2026-06-22** (commit `e736765`). v5: таблица `v5_themes` (tenant_id PK, tokens JSONB), `domain.DefaultThemeTokens` byte-identical к `widget.css` (тест-guard 27/27), `attachThemeToDocument` цепляет `doc.theme` в чат/стрим/превью **+ nav expand/back/filter** (последнее добавлено после adversarial-проверки — иначе кастомная тема слетала при первом drill). GET/PUT `/api/v1/internal/theme`. Фронт: `theme-style.js` строит `:host{--kw-*}` и инжектит ПОСЛЕ `widget.css` (override), чат + превью, idempotent, strip при отсутствии. Тесты: бэк `go test ./...` весь зелёный (+ интеграционные на живом Neon), фронт 102/102, бандл 237 КБ.
  - *Проверка:* byte-identical дефолт доказан в обе стороны; override доказан (порядок+единицы); нет темы → 0 визуальных отличий. Бонус: мгновенный рескин под бренд для демо.
  - *Долг:* выделенного nav-theme регресс-теста нет (фикс = 3 вызова уже-протестированного helper, проверен компиляцией + полным прогоном); добавить при first follow-up.
- [x] **Фаза 2 — токены текут из канваса.** ✅ СОБРАНА + ЗАДЕПЛОЕНА 2026-06-22 (keepstar-admin `c0ad2d6`). `translate.go` passthrough закоммичен; `canvas.TokensFromDocument` — чистый маппер `kw/<token>`→`--kw-*` по закрытому списку (unknown/mismatch/themed → warns); `v5.WriteTenantTheme` (PUT theme); колонка `canvas_designs.kind`; хук в `handleRegister`: доска (`kind='design_system'`) → запись темы. Юнит-тесты зелёные.
  - *Проверка:* поменял цвет в канвасе → сохранил → виджет перекрасился. **(behavioral e2e ещё не прогнан — нужна сессия; см. ниже)**
- [x] **Фаза 3 — витрина дизайн-системы.** ✅ СОБРАНА + ЗАДЕПЛОЕНА 2026-06-22 (keepstar-admin `841cf91`). `GET /admin/api/canvas/design-system/board` get-or-create доски-синглтона (сид `canvas.DesignSystemBoardDoc()` — v9 style guide со всем kw/*); пункт меню «Design System» (SwatchBook) + resolver-страница → `/canvas/edit/:id`; `VariableEditor` выведён кнопкой в Toolbar вендоренного канваса (дивергенция в `canvas/VENDORED.md`). board_test доказывает round-trip через маппер Фазы 2.
  - *Проверка:* видно всю систему на доске, клик по элементу → правка → разъехалось. **(behavioral e2e ещё не прогнан — нужна сессия)**
- [x] **Фаза 4 — правка пресетов в канвасе + 14 сидов.** ✅ СОБРАНА + ЗАДЕПЛОЕНА 2026-06-22 (v5 `caacc96` + admin `f7f9bf8`/`27321d2`). Вместо ручного ре-авторинга 14 сидов — **декомпилятор v5→v9** (`canvas.Decompile`, right-inverse `translate.go`; доказан round-trip `forward(decompile(seed))==seed` на 5 реальных сидах, 4 точных + 1 честный XFAIL). v5 отдаёт doc_json сида по имени; admin `POST /canvas/presets/open` → Decompile → `canvas_design`; кнопка «Open in canvas» на System-карточках; правишь → Register под тем же именем = пер-тенант override (path A). Декомпайл даёт корню белый фон для видимости на тёмном канвасе.
  - *Проверка:* открыл пресет в канвасе → видна структура (owner: «стало лучше»); refs/статичный текст лосси → warning'ами.
- [x] **Фаза 5 — публичная полка** ✅ СОБРАНА + ЗАДЕПЛОЕНА 2026-06-22 (admin `f9e14cb`, admin-side). `admin.shelf_entries`/`shelf_adoptions`; `POST /shelf/publish` (snapshot пресета/темы + провенанс) / `GET /shelf` / `POST /shelf/{id}/adopt` (free-only; тема→`v5_themes`, пресет→`v5_presets`+редактируемый `canvas_design`). UI: «Publish to shelf» (Library + Appearance) + страница `/shelf` с Adopt. Оплата отложена (`price_cents NULL`=free). +22 теста. *(Отступление от плана: реализовано admin-side, таблицы в схеме `admin`, а не в v5/public.)*
  - *Проверка:* опубликовал тему/пресет → другой тенант adopt'нул, правит независимо.

## Контракт Фазы 1 (интерфейс бэк↔фронт — обе стороны строят к нему)

**Модель токенов** (множество `--kw-*`):

| Группа | Ключи | Дефолт |
|---|---|---|
| colors | blue, orange, bg, text, text_muted, border, line | `#5BA4D9`, `#F0924A`, `#ffffff`, `#1a1a1a`, `#666666`, `#e5e7eb`, `#e5e7eb` |
| radius | base | `8px` |
| gap | xs, sm, md, lg, xl | `4, 8, 16, 24, 32px` |
| fontSize | xs, sm, base, md, lg, xl, 2xl, 3xl | `11, 13, 14, 15, 18, 22, 28, 36px` |
| fontWeight | light, normal, medium, semibold, bold | `300, 400, 500, 600, 700` |

Маппинг ключ→CSS-переменная: `colors.blue`→`--kw-blue`, `gap.sm`→`--kw-gap-sm`, `fontSize.base`→`--kw-fs-base`, `fontWeight.semibold`→`--kw-fw-semibold`, и т.д.

**Хранилище:** таблица `v5_themes` (`tenant_id` PK, `tokens` JSONB, `version`, `updated_at`). Засидить дефолт = текущие значения. Нет строки → built-in дефолт (поведение byte-identical).

**Доставка в виджет:** виджет на init забирает тему тенанта (точный путь — fetch-on-init vs метаданные документа — фиксирует рекогносцировка) → строит блок `:host{ --kw-*: … }` и инжектит в shadow-root **после** статичного `widget.css` (чтобы переопределял). Admin-превью `renderDocument(host, doc, opts)` принимает `opts.theme` и инжектит так же.

**Инвариант:** тема == дефолт (или отсутствует) ⇒ инжектированные значения равны хардкоду ⇒ ноль визуальных отличий. Проверяется adversarial-агентом.

**Границы:** новая таблица (НЕ трогаем `master_products` / 1600-колоночный шрам); следуем существующему раннеру миграций и `schema_version`-гейту; `widget.css` дефолты НЕ удаляем (остаются ultimate fallback).

---

# Прогресс и резюме (пауза 2026-06-16)

## Статус одной строкой
Фаза 1 **закоммичена (`e736765`) и ЗАДЕПЛОЕНА на прод 2026-06-22.** Деплой подтверждён обходным путём (Railway MCP-токен на тот момент протух): новый роут `/api/v1/internal/theme` отвечает 403 вместо 404, а bogus-путь под тем же префиксом → 404, и сервис поднялся без 502 ⇒ boot-миграция `v5_themes` (fail-loud `os.Exit(1)`) применилась. **Behavioral live-smoke (PUT override → виджет перекрасился) ещё НЕ прогнан** — нужен `X-Internal-Key`. **Фазы 2 и 3 тоже собраны и задеплоены 2026-06-22** (keepstar-admin `c0ad2d6` + `841cf91`): полный цикл «доска → тема → виджет» собран и юнит-проверен, но behavioral e2e ещё не прогнан (нужна залогиненная сессия admin — путь проверки в Updates-логе). **Фазы 4 и 5 тоже собраны и задеплоены 2026-06-22** (декомпилятор v5→v9 + «Open in canvas»; публичная полка publish/adopt). **Весь дизайн-системный план (Фазы 1–5) закрыт** — остаются только behavioral e2e под сессией + мелкие шероховатости (см. долги ниже).

## Точка возобновления (резюме)
1. ✅ **Деплой Фазы 1 СДЕЛАН (2026-06-22):** ветка `feat/design-system-phase1-theme` → коммит `e736765` → FF-merge в main → push → выкат на прод. Миграция `v5_themes` применилась на boot. Admin-правка `translate.go` оставлена для Фазы 2.
2. **← СЛЕДУЮЩИЙ ШАГ — Живой proof keystone** (нужен `X-Internal-Key`): `PUT /api/v1/internal/theme?tenant=<slug>` с телом `{"tokens":{"colors":{"blue":"#FF0000"}}}` → открыть чат-виджет → синие акценты станут красными. Откат — `PUT` с пустым/дефолтным набором или удалить строку.
3. **Дальше — воркфлоу Фаз 2+3** (полный цикл «вижу токены на доске → правлю → виджет перекрашивается»). Саб-спеки и решения — ниже.

## Фаза 1 — что собрано (для будущего коммита)
**v5 backend** (`project_v5/backend`): `internal/domain/theme.go` (+`_test`), `internal/adapters/postgres/theme_migrations.go`, `postgres_theme.go` (+integration `_test`), `internal/ports/theme_port.go`, `internal/handlers/handler_theme.go` (+`_test`), правки `handler_pipeline.go`, `handler_pipeline_stream.go`, `handler_presets.go` (+`_test`), `handler_navigation.go` (nav-attach — мой фикс), `routes.go`, `cmd/server/main.go`, `handler_pipeline_live_test.go`.
**v5 frontend** (`project_v5/frontend`): `src/theme-style.js` (новый helper buildThemeCss/applyTheme/tokensFromDoc), правки `widget.jsx`, `WidgetApp.jsx`, `widget-preview.jsx`, `widget.css` (cross-link коммент), `tests/theme-style.test.jsx`.
**admin backend** (`keepstar-admin/backend`): `internal/canvas/translate.go` (passthrough variables/themes) + `translate_test.go` + `internal/handlers/handler_canvas_test.go`.

**Проверка:** бэк `go vet`/`go build` чисто, `go test ./...` весь зелёный (+ интеграционные на живом Neon); фронт 102/102, бандл 237 КБ; admin canvas+handlers зелёные (падают только пред-существующие Shopify-consent тесты, подтверждено `git stash`).
**Adversarial поймала дыру → починена:** тема цеплялась к чату/стриму/превью, но не к nav-документам (drill/back/filter), а фронт снимает `--kw-*` при доке без темы → кастомная тема слетала при первом клике. Исправлено: `attachThemeToDocument` на всех трёх nav-ответах + проброс `themePort` в `NavigationHandler`. Долг: выделенного nav-регресс-теста нет.

## Решения, которые я уже закрыл сам (НЕ перерешать)
- Доска дизайн-системы — **пер-тенант** (тема пер-тенант; глобальное — публичная полка Фазы 5).
- Сохранение доски **всегда пишет тему** (доска — выделенный singleton-дизайн, опознаётся по `kind`; чекбокс per-preset не нужен).
- Маппинг канвас-переменных: **`kw/<token>` → `--kw-<token>`** по закрытому списку из `widget.css`, неизвестные `kw/*` — drop+warn. Доску авторю я, так что конвенцию просто применяю.
- Адопт темы (Фаза 5) = **перезапись** строки `v5_themes` целевого тенанта (PK=tenant_id, singleton). Версионных/именованных тем не делаем.

## Открытые развилки для owner (не блокируют старт 2+3, но спросить)
- **Сидинг доски:** name-convention (`__design_system__`, виден в библиотеке Canvas, удаляем) **vs** новая колонка `kind` + getOrCreate (singleton, скрыт). Я склоняюсь к `kind`-колонке — чище.
- **Регистрация 14 сидов под теми же именами** (`product_card`) даёт пер-тенант override системного сида для этого тенанта — это и есть желаемое (path A), но подтвердить, что не хотим суффикс (`product_card_canvas`).
- **Компоненты доски** (Фаза 3): живые зарегистрированные пресеты через рендерер **vs** статичные фреймы. Статика проще и самодостаточна.

## Заземлённые саб-спеки Фаз 2–5 (из рекогносцировки воркфлоу — file:line)

### Фаза 2 — токены из канваса → тема тенанта
Порядок: (1) `translate.go` passthrough — **УЖЕ СДЕЛАНО в Фазе 1**; (2) v5 миграция `v5_tenant_theme`+endpoint+клиент — **частично есть: `v5_themes`+PUT уже в Фазе 1**, нужен admin→v5 клиент-метод; (3) admin var→token маппер (чистая функция рядом с `knownFields`) + запись темы в `handleRegister` после коммита (рядом с `invalidateV5`, best-effort); (4) триггер «это доска» (по `kind`, см. решения).
Якоря: `keepstar-admin/backend/internal/canvas/translate.go:540-582` (knownFields/bindingFromName — паттерн by-name для маппинга); `internal/adapters/v5/v5_client.go:128-145` (InvalidatePresetCache — клонировать в WriteTenantTheme); `internal/handlers/handler_canvas.go:74-91` (registerRequest) + `244-250` (call-site после коммита); v5 `internal/handlers/handler_presets.go:54-72`+`routes.go:72-74` (паттерн X-Internal-Key endpoint). Формы переменной v9: `keepstar-admin/canvas/packages/domain/src/entities/document.ts:7-24`; v5-зеркало `project_v5/backend/internal/engine/document.go:19-39`; флаттен темизированных значений `variable_resolver.go:259-281`.
Риск: тематические (light/dark) переменные схлопываются в один токен (тема — плоский набор) → документировать warning, не молча.

### Фаза 3 — витрина дизайн-системы (SEE+EDIT в канвасе)
Дизайн грузится как обычный design по роуту `/canvas/edit/:id` (EditorPage: `api.get('/canvas/designs/:id')` → `setDocument`; автосейв PUT с дебаунсом 2с) — доске не нужно ничего нового тут, это просто ещё один id.
Шаги: (1) пункт меню `frontend/src/features/layout/DashboardLayout.jsx` NAV ~22-31 (группа Build), иконка не Palette (занята Canvas) — SwatchBook/Brush; (2) тонкая страница-резолвер + роут `frontend/src/App.jsx:95-96`, она резолвит id доски и `navigate('/canvas/edit/:id', {replace:true})`; (3) **surface VariableEditor** — он уже полностью готов в `canvas/apps/web/src/components/VariableEditor.tsx` (цвет/число/строка/булево + темы; CSS `kc-var-*` уже в app.css), но **нигде не импортируется** — добавить кнопку «Variables» в `canvas/apps/web/src/components/Toolbar.tsx` (это дивергенция от vendored-пина → залогировать в `canvas/VENDORED.md`); (4) сид доски один раз в `admin.canvas_designs` (миграция `backend/internal/adapters/postgres/admin_migrations.go:300`); (5) контент доски — v9 Document `{version:"2.10", variables, themes, children}`: свотчи (rectangle на токен), type ramp (text на размер/вес), радиусы/гэпы, кнопки, компоненты.
Развилки: сидинг доски (выше); shared-vs-per-tenant (решено: per-tenant); где монтировать тоггл VariableEditor; entitlement в preview (создание дизайна гейтится `useEntitled().blocked` — решить, сидится ли доска в preview).
Риск: vendored canvas — read-only пин; правки Toolbar/embed логировать в `VENDORED.md` и переприменять после re-sync.

### Фаза 4 — правка пресетов в канвасе + 14 сидов
«Открыть канвас-пресет в канвасе» **уже работает**: `LibraryPage.jsx:593,608-612` (sourceDesignId → кнопка «Open source design» → `/canvas/edit/:id`); backend `handler_canvas_presets.go:110-161` отдаёт designId через JOIN на `canvas_registrations`. Phase-4 = (а) переименовать/перенести кнопку и/или добавить affordance на System-library карточки (у них нет source design — net-new UX); (б) **path A**: отрисовать 14 сидов в канвасе и зарегать.
14 сидов (`project_v5/backend/internal/engine/presets/seed/`, карта `presets.go:95,135,160`): product_card, product_card_compact, product_card_horizontal, product_card_list_row, product_carousel, product_comparison (cards); product_detail, product_detail_accordion, product_detail_horizontal (details); text_explainer (narrative); empty_not_found, error_generic (states); component_price_rating, component_brand_badge (components).
Ключевой факт: сиды в КОМПИЛИРОВАННОМ v5-словаре, канвас — v9; транслятор ОДНОСТОРОННИЙ (`translate.go:42/58`, binding by layer name `:558`), v5→v9 декомпилятора нет → path A = **ре-авторинг** (нарисовать заново по binding-guide `EditorPage.jsx:20-27`), НЕ импорт. Регистрация под именем системного сида НЕ блокируется (`v5_presets` UNIQUE per `(tenant_id,name)`; DB-first резолв `system_registry.go:9-19` → пер-тенант override). Фиделити accordion/comparison/carousel/absolute-overlay проверять рендером каждого.

### Фаза 5 — публичная полка (каркас, покупка отложена)
Дом таблиц — **v5 (public schema)**, рядом с `v5_presets`/`v5_preset_versions`/`v5_themes` (адопт = same-pool TX; виджет полку не читает структурно). Новый `internal/adapters/postgres/shelf_migrations.go` (паттерн `preset_migrations.go:18-52`) + регистрация в `cmd/server/main.go:49-63` после "trace".
Схема: `v5_shelf_entries(id, kind theme|preset|family, source_tenant_id, name, description, version, price_cents NULLABLE, currency, snapshot_json JSONB, provenance_json JSONB, published_at)` + `v5_shelf_adoptions(id, shelf_entry_id, target_tenant_id, new_preset_id, new_theme_tenant, adopted_at)`. **snapshot_json, не FK на живые строки** (admin.canvas_designs в другой схеме/сервисе; источник может меняться/удаляться). Адопт=COPY: preset → INSERT v5_presets+v5_preset_versions(+ admin.canvas_designs/registrations в admin-сервисе, т.к. кросс-схема); theme → UPSERT v5_themes целевого. Скаффолдим сейчас: обе таблицы + publish + list + free-adopt + провенанс; отложено: оплата/entitlement (гейт по `price_cents IS NULL`=free).
Блокер: theme-adopt ждёт `v5_themes` (Фаза 1 — уже есть). Первый писатель `v5_presets` — адопт (postgres_preset.go сейчас read-only); если Фазы 1–4 добавят CreatePreset — переиспользовать, не плодить второй путь записи.

## Долги/харденинг (не блокеры)
- nav-theme регресс-тест (Фаза 1).
- `translate.go` passthrough shape-blind: кривой v9-дизайн (`themes` как строка вместо массива) даст doc_json, который v5 не разворачивает → 500 на рендере того пресета. Для корректных Pencil-дизайнов невозможно; добавить валидацию формы при passthrough.
- byte-identical drift guard: если правят `widget.css:16-46`, надо синхронно править `domain.DefaultThemeTokens()` (тест `TestDefaultThemeTokens` упадёт, если разойдутся).
