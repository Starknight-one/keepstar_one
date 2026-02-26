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
        │  Agent 2 (Render)     │  ← visual_assembly tool
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

## Visual Assembly Engine (текущая работа)

**Ветка**: `feature/visual-assembly-engine`

**12 примитивов**: show, hide, display, color, size, shape, order, layer, anchor, direction, place, layout

**Фазы**:
- Phase 1-3: Базовый движок + 12 примитивов + багфиксы
- Phase 4: Constraints, presets, validation, testbench
- Phase 5: Разделение display на format (трансформация значения) + wrapper (визуальный контейнер)
- **Далее**: Layout Engine (spec в `docs/LAYOUT_ENGINE_SPEC.md`)

**Ключевые файлы движка** (перечитай перед работой):
- `project/backend/internal/tools/tool_visual_assembly.go` — главный tool (~700 LOC)
- `project/backend/internal/tools/defaults_engine.go` — AutoResolve, field ranking
- `project/backend/internal/tools/constraints.go` — 30+ правил в 4 уровнях
- `project/backend/internal/tools/tool_render_preset.go` — сборка атомов
- `project/backend/internal/presets/visual_assembly_presets.go` — пресеты

**Тестбенч**: `POST /api/v1/testbench` (бэкенд) + `/testbench` (админ фронтенд, порт 5174)

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

- `docs/ARCHITECTURE.md` — архитектура и API
- `docs/VISUAL_ASSEMBLY_ENGINE.md` — спецификация движка
- `docs/LAYOUT_ENGINE_SPEC.md` — спека Layout Engine (следующая фаза)
- `docs/ENGINE_VISUAL_RULES.md` — правила визуального рендеринга
- `docs/GLOSSARY.md` — терминология предметной области
- `docs/SPEC_TWO_AGENT_PIPELINE.md` — спецификация двухагентного пайплайна
- `AI_docs/Manifesto.md` — продуктовое видение
- `AI_docs/ARCHITECTURE_RULES.md` — архитектурные принципы
