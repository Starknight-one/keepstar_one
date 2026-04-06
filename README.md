# Keepstar One Ultra

**AI-чат с динамическим интерфейсом** — SaaS B2B2C платформа, которая отвечает не текстом, а интерактивными виджетами.

## Концепция

Традиционные чат-боты отвечают текстом. Keepstar отвечает **интерфейсом**.

Пользователь спрашивает "Покажи кроссовки Nike до 15000₽" — и получает не список ссылок, а готовую галерею карточек товаров с фильтрами, сортировкой и кнопками "В корзину". Все генерируется AI на лету.

```
┌─────────────────────────────────────────────────────────┐
│  "Покажи кроссовки Nike"                                │
│                                                         │
│  ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌─────────┐    │
│  │  [img]  │  │  [img]  │  │  [img]  │  │  [img]  │    │
│  │ Air Max │  │ Pegasus │  │ Dunk    │  │ Jordan  │    │
│  │ ★★★★☆   │  │ ★★★★★   │  │ ★★★★☆   │  │ ★★★★★   │    │
│  │ 12 990₽ │  │ 14 500₽ │  │ 11 990₽ │  │ 18 990₽ │    │
│  │[Корзина]│  │[Корзина]│  │[Корзина]│  │[Корзина]│    │
│  └─────────┘  └─────────┘  └─────────┘  └─────────┘    │
│                                                         │
│  [Показать дешевле] [Сравнить] [Только 5 звёзд]        │
└─────────────────────────────────────────────────────────┘
```

## Ключевые преимущества

### Для бизнеса (B2B)
- **Без разработки** — новые данные загружаются и сразу отображаются
- **Без дизайнеров** — AI композирует виджеты из атомарных элементов
- **Повышение конверсии** — визуальный ответ vs текстовые ссылки
- **Готовая аналитика** — все взаимодействия отслеживаются

### Для пользователей (B2C)
- Естественный язык: "покажи только с быстрой доставкой"
- Динамические форматы: "сделай сравнительную таблицу"
- Бесконечный контент: чем больше данных у продукта, тем умнее ответы

## Архитектура

```
Пользователь → Chat Widget (React 19, Shadow DOM, IIFE)
                    ↓ REST API
              Chat Backend (Go 1.24, порт 8080)
                    ↓
        ┌───────────────────────┐
        │  Agent 1 (NLU/Data)   │  ← catalog_search, state_filter, history_lookup
        │  Agent 2 (Render)     │  ← visual_assembly tool
        └───────────────────────┘
                    ↓
           Formation JSON → Frontend renders
```

### Backend-First принцип

```
Frontend = "тупая рендерилка"
Backend  = вся логика, LLM, layout engine, state
```

Frontend получает готовый JSON и рендерит. Никакой бизнес-логики на клиенте.

### Атомарная композиция

```
Formation (layout: grid, list, single, carousel, comparison, table, composed)
  └── Widget (карточка/строка/блок с атомами)
      └── Atom (6 типов: text, number, image, icon, video, audio)
           ├── subtype (currency, rating, url, date...)
           ├── display/wrapper (h1, badge, tag, price, button...)
           ├── format (currency, stars, percent, date...)
           └── slot (hero, title, price, primary, secondary, badge...)
```

### Движки: V1, V2, V4

| Аспект | V1 | V2 | V4 (актуальный) |
|--------|----|----|-----------------|
| Layout | Плоский `Zone[]` массив | Рекурсивное `LayoutNode` дерево | Ops-driven `LayoutNode` |
| Пресеты | Статический `PresetRegistry` | Динамический `PresetV2Registry` + field_definitions | Реестр пресетов + concat-override через ops |
| Атомы | `Atom` (display) | `AtomV2` (textStyle + wrapper + rigidity) | `AtomV2` + литералы для freestyle |
| Как Agent2 работает | Fixed tool params | Fixed tool params | Ops + optional preset + explicit replicate |

V4 живёт в `project_v4/backend/` на ветке `feature/engine-v4` и задеплоен на `v4-engine-production.up.railway.app`. V1/V2 остались в `project/backend/` как legacy. Основной прод использует V4.

## Технологии

| Слой | Стек |
|------|------|
| Backend | Go 1.24, Chi, PostgreSQL (Neon serverless), pgvector |
| Frontend | React 19, Vite 7, CSS Variables (custom, без Tailwind) |
| LLM | Claude Haiku 4.5 (Anthropic), prompt caching |
| Embeddings | OpenAI text-embedding-3-small (384 dim) |
| Admin | Go 1.24 + React 19, JWT auth |

## Структура проекта

```
Keepstar_one_ultra/
├── project_v4/                 # АКТУАЛЬНЫЙ движок (feature/engine-v4)
│   └── backend/
│       └── internal/
│           ├── engine_v4/      # Ops-driven engine: presets.go, ops.go, binding.go, constraints.go
│           ├── tools/          # visual_assembly tool (preset/ops/replicate/limit params)
│           ├── prompts/        # Agent2 prompt with PRESETS section
│           ├── usecases/       # Pipeline, Agent1, Agent2, navigation
│           ├── handlers/       # HTTP + debug/traces
│           ├── adapters/       # Postgres, Anthropic, OpenAI
│           └── domain/         # Widget, AtomV2, LayoutNode, Formation
│
├── project/                    # LEGACY V1/V2 (main ветка)
│   ├── backend/                # Go API (гексагональная архитектура)
│   │   ├── cmd/server/         # HTTP server entry point
│   │   └── internal/
│   │       ├── domain/         # Session, State, Atom, AtomV2, Widget, Formation, LayoutNode
│   │       ├── ports/          # LLM, Catalog, State, Trace, Embedding, FieldDefinition
│   │       ├── adapters/       # Postgres (pgx), Anthropic, OpenAI, Memory
│   │       ├── usecases/       # Pipeline, Agent1, Agent2 (v1+v2), Navigation, State, Actions
│   │       ├── handlers/       # HTTP handlers + middleware (CORS, tenant, logging)
│   │       ├── engine/         # Visual Assembly Engine V2 (layout tree, rules, tokens, auto_layout)
│   │       ├── tools/          # Tool executors (catalog_search, visual_assembly, state_filter...)
│   │       ├── prompts/        # LLM prompt templates (Agent1 + Agent2 v1/v2)
│   │       ├── presets/        # Widget presets (v1: 16 пресетов, v2: PresetV2Registry)
│   │       ├── config/         # Env-based configuration
│   │       └── logger/         # Structured logging
│   │
│   └── frontend/               # React widget (Shadow DOM, FSD architecture) — общий на V2 и V4
│       └── src/
│           ├── widget.jsx      # IIFE entry + Shadow DOM setup
│           ├── entities/       # atom/ (v1+v2), widget/ (templates, LayoutTreeRenderer), formation/, message/
│           ├── features/       # chat/, actions/, catalog/, navigation/, overlay/
│           └── shared/         # api/, config/, theme/, ui/, logger/
│
├── project_admin/
│   ├── backend/                # Go API (порт 8081): auth, CRUD, import, enrichment
│   │   └── cmd/                # server, crawler, seed, rebuild-embeddings
│   └── frontend/               # React (порт 5174): каталог, импорт, тестбенч, виджет
│
├── Keepstar_one_landing/       # Marketing landing page (React 19, Vite 7)
│
├── tests/                      # E2E тесты (Python)
│   ├── e2e_run.py              # Main test runner
│   ├── e2e_quick_test.py       # Quick validation
│   └── e2e_recon.py            # Reconnaissance tests
│
├── docs/                       # Updates/ (live session logs), archive/ (legacy specs), bugs/, ответы/, Engine_hustle/, New features/
├── AI_docs/                    # Манифест, архитектурные правила, принципы агентов
├── ADW/                        # SDLC оркестратор + dev-inspector (порт 3457)
├── scripts/                    # start/stop скрипты для всех сервисов
└── .claude/                    # Claude Code: skills, commands, experts, worktrees
```

## LLM Pipeline

Двухагентный пайплайн для генерации визуальных ответов:

```
User Query
    │
    ▼
┌─────────────────────────┐
│  Agent 1: NLU & Data    │  ← catalog_search (keyword + vector + RRF)
│                         │  ← state_filter, history_lookup
└─────────────────────────┘
    │
    ▼
┌─────────────────────────┐
│  Agent 2: Visual Render │  ← visual_assembly tool: preset + ops (overrides) + replicate/limit
│                         │  ← V4: 12 named presets (6 product card variants, 3 system, 3 nav)
└─────────────────────────┘
    │
    ▼
┌─────────────────────────┐
│  Formation JSON         │  ← widgets[], atoms[], sections[], pagination
└─────────────────────────┘
    │
    ▼
Frontend: FormationRenderer → WidgetRenderer → AtomRenderer (v1/v2)
```

### Span Waterfall Tracing

Каждый pipeline execution собирает `Span`-ы через `SpanCollector` (thread-safe, context-propagated).
Debug UI: `GET /debug/traces/` — список трейсов с визуальным waterfall.

## Dev Servers

| Сервис | Путь | Порт |
|--------|------|------|
| Chat backend | `project/backend/` | 8080 |
| Chat frontend | `project/frontend/` | 5173 |
| Admin backend | `project_admin/backend/` | 8081 |
| Admin frontend | `project_admin/frontend/` | 5174 |
| Dev Inspector | `ADW/dev-inspector/` | 3457 |

## Быстрый старт

```bash
# Все сервисы
./scripts/start_all.sh

# Или по отдельности:
cd project/backend && go run cmd/server/main.go    # Backend :8080
cd project/frontend && npm install && npm run dev   # Frontend :5173

# Тесты
cd project/backend && make test-unit    # быстрые (<1 сек)
cd project/backend && make test-all     # полные (~180 сек)

# E2E
cd tests && python e2e_run.py
```

## Embed на сайт

```html
<script
  src="https://your-domain/widget.js"
  data-tenant="YOUR_TENANT_SLUG"
  data-api="https://your-api/api/v1"
  async>
</script>
```

## API Endpoints

| Endpoint | Method | Описание |
|----------|--------|----------|
| `/api/v1/pipeline` | POST | Основной: query → Agent1 → Agent2 → Formation |
| `/api/v1/session/init` | POST | Создать сессию |
| `/api/v1/session/{id}` | GET | Получить сессию + сообщения |
| `/api/v1/chat` | POST | Отправить сообщение |
| `/api/v1/navigation/expand` | POST | Drill-down в деталь |
| `/api/v1/navigation/back` | POST | Назад |
| `/api/v1/actions` | POST | Действие (лайк, корзина) |
| `/api/v1/testbench` | POST | Тест visual assembly без LLM |
| `/debug/traces/` | GET | Waterfall трейсы |
| `/health`, `/ready` | GET | Health checks |

## Документация

- [docs/Updates/](docs/Updates/) — дев-логи сессий (актуальное состояние V4, по дате)
- [docs/PRE_LAUNCH_TASKS.md](docs/PRE_LAUNCH_TASKS.md) — трекер задач до релиза (волны B2/B3/B4/E1/E2...)
- [docs/archive/](docs/archive/) — старые спеки (V1/V2 движок, ENGINE_V2_SPEC, PROJECT_STATUS и др.)
- [AI_docs/Manifesto.md](AI_docs/Manifesto.md) — продуктовое видение
- [AI_docs/ARCHITECTURE_RULES.md](AI_docs/ARCHITECTURE_RULES.md) — архитектурные принципы

## Лицензия

Proprietary. All rights reserved.
