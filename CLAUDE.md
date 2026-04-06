# Keepstar One Ultra — Project Context

> **ВАЖНО**: Информация в этом файле может быть устаревшей. Перед реализацией любой задачи ОБЯЗАТЕЛЬНО перечитай соответствующие исходные файлы и проверь актуальное состояние кода. Не полагайся слепо на описания ниже — они дают общую картину, но детали могли измениться.

## Что это

AI-powered SaaS B2B2C чат-виджет для e-commerce. Пользователь пишет в чат — вместо текстовых ответов бот генерирует интерактивные виджеты: карточки товаров, галереи, сравнения, детальные просмотры. Всё собирается динамически на бэкенде через двухагентный LLM-пайплайн.

**Ключевая ценность**: бизнес встраивает `<script>` на сайт — получает AI-ассистента с визуальными ответами без разработки.

## Архитектура (высокий уровень)

```
Пользователь → Chat Widget (React, Shadow DOM)
                    ↓ REST API
              Chat Backend (Go, порт 8080)
                    ↓
        ┌───────────────────────┐
        │  Agent 1 (NLU/Data)   │  ← catalog_search, state_filter, history_lookup
        │  Agent 2 (Render)     │  ← 
        embly tool
        └───────────────────────┘
                    ↓
           Formation JSON → Frontend renders
```

**Backend-first**: фронтенд — "тупой рендерер" JSON. Вся логика, лейаут, ограничения — на бэкенде.

## Трёхуровневая иерархия виджетов

```
Formation (layout: grid, list, single, carousel, comparison, table)
  └── Widget (карточка/строка/блок с атомами)
      └── Atom (6 типов: text, number, image, icon, video, audio)
           ├── subtype (currency, rating, url, date...)
           ├── display/wrapper (h1, badge, tag, price, button...)
           ├── format (currency, stars, percent, date...)
           └── slot (hero, title, price, primary, secondary, badge...)
```

## Структура проекта

```
project/backend/           — Go 1.24, гексагональная архитектура
  cmd/server/              — Entry point, DI, graceful shutdown
  internal/
    domain/                — Сущности (Session, State, Atom, Widget, Formation, Preset, Tool, Trace)
    ports/                 — Интерфейсы (LLM, Catalog, State, Trace, Log)
    adapters/              — Postgres (pgx), Anthropic, OpenAI, Memory
    usecases/              — Pipeline, Agent1, Agent2, Navigation, State management
    handlers/              — HTTP (pipeline, chat, navigation, testbench, debug)
    tools/                 — Tool executors + Visual Assembly Engine
    prompts/               — Системные промпты Agent1/Agent2
    presets/               — Пресеты визуальной сборки
project/frontend/          — React 19, Vite 7, Feature-Sliced Design
  entities/                — atom/, widget/, formation/, message/
  features/                — chat/, catalog/, navigation/, overlay/
  shared/                  — api/, theme/, hooks/, config/
project_admin/backend/     — Go, управление каталогом, импорт, обогащение
project_admin/frontend/    — React, админка (продукты, импорт, виджет, тестбенч, настройки)
docs/                      — Спецификации и логи (ARCHITECTURE, VISUAL_ASSEMBLY_ENGINE, LAYOUT_ENGINE_SPEC...)
ADW/                       — SDLC оркестратор + dev-inspector + спеки
AI_docs/                   — Манифест, архитектурные правила, принципы агентов
scripts/                   — start.sh, stop.sh, start_admin.sh, stop_admin.sh, start_all.sh, stop_all.sh
```

## Dev Servers & Tools

| Сервис | Путь | Порт |
|--------|------|------|
| Chat backend | `project/backend/` | 8080 |
| Chat frontend | `project/frontend/` | 5173 |
| Admin backend | `project_admin/backend/` | 8081 |
| Admin frontend | `project_admin/frontend/` | 5174 |
| Dev Inspector | `ADW/dev-inspector/` | 3457 |

- **Запуск всего**: `scripts/start_all.sh`
- **psql**: `/opt/homebrew/Cellar/libpq/18.1_1/bin/psql` или `/opt/homebrew/Cellar/postgresql@15/15.15_1/bin/psql`
- **Конфигурация**: `project/.env` (DATABASE_URL, ANTHROPIC_API_KEY, OPENAI_API_KEY, TENANT_SLUG)
- **БД**: Neon PostgreSQL (serverless, AWS). Схемы: catalog, admin, logs + таблицы chat_*
- **Тесты**: `cd project/backend && make test-unit` (быстрые), `make test-all` (полные)

## Двухагентный пайплайн

1. **Agent 1** (NLU) — анализирует запрос, вызывает tools:
   - `catalog_search` — гибридный поиск (SQL keyword + pgvector + RRF merge)
   - `state_filter` — фильтрация уже загруженных данных
   - `history_lookup` — поиск в истории сессии
   - Записывает результат в state.data + state.meta

2. **Agent 2** (Render) — вызывает `visual_assembly` tool:
   - Получает state.meta (count, fields)
   - Выбирает preset, layout, overrides
   - DefaultsEngine автоматически разрешает: layout по count, size по количеству полей, display по subtype
   - Constraints (4 уровня, 30+ правил) валидируют и нормализуют
   - Результат → state.template["formation"]

3. **Frontend** рендерит Formation JSON через FormationRenderer → WidgetRenderer → AtomRenderer

## V4 Engine (актуальный, `feature/engine-v4`)

Ops-driven движок — Agent2 строит и модифицирует UI через операции (insert/update/delete/move) на дереве виджетов. Живёт в `project_v4/backend/`, задеплоен на `v4-engine-production.up.railway.app`.

**Основной tool — `visual_assembly`**, параметры:
- `ops` — массив операций (insert/update/delete/move) на formation tree
- `preset` — именованный пресет (12 штук, см. ниже). Concat'ится с `ops` в один batch → override-ops могут ссылаться на $ref пресета ($w/$root/$info/$meta)
- `replicate` — явный флаг репликации виджет-шаблона (B3). Пресет несёт `DefaultReplicate`, наследуется если не передан
- `limit` — cap на кол-во data items для репликации
- `layout` — grid/list/single/carousel, `columns`, `size`

**Пайплайн Execute** (`engine.go`):
1. Init formation (или загрузка existing)
2. Формационные настройки (layout, columns, size)
3. `ApplyOps` — применить ops на дерево (preset + user в одном batch)
4. Limit slice → Replicate (explicit flag) / single-bind
5. Inject `DefaultWidgetActions` (like, add_to_cart) для entity-bound виджетов
6. `BindData` — атомы с FieldName получают значения из data[i]
7. `ApplyConstraints` — нормализация
8. `StampTreeIDs` → `BuildTreeMap` (compact context для Agent2 следующего turn'а)

**12 пресетов** (`engine_v4/presets_*.go`):
- product: `product_card`, `product_card_compact`, `product_card_horizontal`, `product_card_list_row`, `product_detail`, `product_detail_horizontal`
- system: `text_explainer`, `empty_not_found`, `error_generic`
- nav: `catalog_category_card`, `liked_grid`, `cart_grid`

Пресеты пока hardcoded под косметику (fieldName: images/name/price/rating/brand/...). B7 заменит это на роль-based slot resolution через `catalog.field_definitions`.

**Ключевые файлы V4** (перечитай перед работой):
- `project_v4/backend/internal/engine_v4/engine.go` — главный Execute pipeline
- `project_v4/backend/internal/engine_v4/presets.go` + `presets_{product,system,nav}.go` — реестр + builders
- `project_v4/backend/internal/engine_v4/ops.go` — `ApplyOps` + $ref binding
- `project_v4/backend/internal/engine_v4/binding.go` — data → atom binding
- `project_v4/backend/internal/engine_v4/constraints.go` — нормализация
- `project_v4/backend/internal/tools/tool_visual_assembly.go` — tool definition + Execute
- `project_v4/backend/internal/prompts/prompt_compose_widgets.go` — Agent2 system prompt
- `project_v4/backend/internal/engine_v4/default_ops.go` — thin wrappers для usecases (`ProductCardGridOps`, `ProductDetailOps`, `DefaultWidgetActions`, `GridColumnsForCount`)

**Трекер задач**: `docs/PRE_LAUNCH_TASKS.md` (волны B2/B3/B4/E1/E2/A2/B7/AD1/UX1/...)

**Дев-логи сессий**: `docs/Updates/feature-engine-v4_<YYYY-MM-DD>_<HH-MM>.md` — каждая сессия оставляет лог с context, changes, tests, commit hash, known gaps.

## Legacy Engine V1/V2 (`project/backend/`, ветка `main`)

Старый движок остался в `project/backend/` как legacy. Основной прод переключен на V4, но V1/V2 код собирается и тесты проходят. Не трогать без явной необходимости.

## Модель данных (ключевые сущности)

- **SessionState**: current (data + meta + template), view, viewStack, conversationHistory, step
- **Delta**: source/actor/trigger/type + action/result (append-only история)
- **Atom**: type + subtype + display + format + value + slot + meta
- **Preset**: fieldConfigs (field→atom mapping) + slotConfigs (constraints per slot)
- **Formation**: mode + grid + widgets[] + sections[] + pagination
- **Widget**: template + size + atoms[] + entityRef + meta

## API Endpoints (Chat Backend, порт 8080)

- `POST /api/v1/pipeline` — основной: query → Agent1 → Agent2 → Formation
- `POST /api/v1/navigation/expand` — drill-down в деталь
- `POST /api/v1/navigation/back` — назад
- `POST /api/v1/session/init` — создать сессию
- `GET /api/v1/session/{id}` — получить сессию
- `POST /api/v1/testbench` — тест visual assembly без LLM
- `GET /debug/traces/` — waterfall трейсы пайплайна
- `GET /debug/session/` — отладка сессий

## LLM & Стоимость

- **Модель**: Claude Haiku 4.5 (по умолчанию, настраивается через LLM_MODEL)
- **Embeddings**: OpenAI text-embedding-3-small (384 dim)
- **Prompt caching**: System + tools + conversation кешируются (TTL 5 мин)
- **Стоимость Haiku**: $1 input / $5 output per 1M tokens; cache write 1.25×, cache read 0.1×

## Фронтенд (Chat Widget)

- **Деплой**: единый IIFE бандл `widget.js`, встраивается через `<script data-tenant="slug" data-api="url">`
- **Shadow DOM**: изоляция стилей от хост-страницы
- **Instant expand**: `adjacentTemplates` + `fillFormation()` — детальный просмотр без round-trip
- **Session cache**: localStorage, TTL 30 мин, восстановление при F5
- **Тема**: CSS Variables (marketplace theme, purple primary #8B5CF6)

## Админка

- **Каталог**: просмотр, поиск, редактирование продуктов
- **Импорт**: JSON upload → async processing → master_products + product listings
- **Обогащение**: Claude Haiku извлекает PIM-атрибуты (skin_type, concern, ingredients...)
- **Виджет**: embed-код для интеграции на сайт
- **Краулер**: `cmd/crawler/` — scrape JSON-LD с e-commerce сайтов
- **Данные**: 967 продуктов heybabescosmetics в `project_admin/Crawler_results/crawl_enriched_967.json`

## Документация

- `docs/Updates/` — дев-логи сессий V4 (актуальное состояние, по дате)
- `docs/PRE_LAUNCH_TASKS.md` — трекер задач до релиза (волны B2/B3/B4/E1/E2/B7/AD1/...)
- `docs/archive/` — старые спеки (ARCHITECTURE, VISUAL_ASSEMBLY_ENGINE, LAYOUT_ENGINE_SPEC, GLOSSARY, SPEC_TWO_AGENT_PIPELINE и др.)
- `AI_docs/Manifesto.md` — продуктовое видение
- `AI_docs/ARCHITECTURE_RULES.md` — архитектурные принципы
