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

### Двухверсионный движок (V1 + V2)

| Аспект | V1 | V2 |
|--------|----|----|
| Layout | Плоский `Zone[]` массив | Рекурсивное `LayoutNode` дерево |
| Пресеты | Статический `PresetRegistry` | Динамический `PresetV2Registry` + field_definitions из БД |
| Атомы | `Atom` (display) | `AtomV2` (textStyle + wrapper + rigidity) |
| Engine | Zone-based assembly | Two-pass algorithm (BudgetDown/NeedsUp) |

Активация: `ENGINE_VERSION=v2` + `AGENT2_PROMPT_VERSION=v2` в `.env`. По умолчанию V1, полная обратная совместимость.

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
├── project/
│   ├── backend/                # Go API (гексагональная архитектура)
│   │   ├── cmd/server/         # HTTP server entry point
│   │   └── internal/
│   │       ├── domain/         # 30+ сущностей (Session, State, Atom, AtomV2, Widget, Formation, LayoutNode...)
│   │       ├── ports/          # 8 интерфейсов (LLM, Catalog, State, Trace, Embedding, FieldDefinition...)
│   │       ├── adapters/       # Postgres (pgx), Anthropic, OpenAI, Memory
│   │       ├── usecases/       # Pipeline, Agent1, Agent2 (v1+v2), Navigation, State, Actions
│   │       ├── handlers/       # 10+ HTTP handlers + middleware (CORS, tenant, logging)
│   │       ├── engine/         # Visual Assembly Engine V2 (layout tree, rules, tokens, auto_layout)
│   │       ├── tools/          # Tool executors (catalog_search, visual_assembly, state_filter...)
│   │       ├── prompts/        # LLM prompt templates (Agent1 + Agent2 v1/v2)
│   │       ├── presets/        # Widget presets (v1: 16 пресетов, v2: PresetV2Registry)
│   │       ├── config/         # Env-based configuration
│   │       └── logger/         # Structured logging
│   │
│   └── frontend/               # React widget (Shadow DOM, FSD architecture)
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
├── docs/                       # Спецификации (27 файлов, вкл. archive/)
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
│  Agent 2: Visual Render │  ← visual_assembly tool (17 presets, 30+ constraints)
│                         │  ← DefaultsEngine: layout, size, display auto-resolve
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

- [docs/PROJECT_STATUS.md](docs/PROJECT_STATUS.md) — текущий статус проекта
- [docs/ENGINE_V2_SPEC.md](docs/ENGINE_V2_SPEC.md) — спецификация Engine V2
- [AI_docs/Manifesto.md](AI_docs/Manifesto.md) — продуктовое видение
- [AI_docs/ARCHITECTURE_RULES.md](AI_docs/ARCHITECTURE_RULES.md) — архитектурные принципы

## Лицензия

Proprietary. All rights reserved.
