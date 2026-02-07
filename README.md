# Keepstar One Ultra

**AI-чат с динамическим интерфейсом** — SaaS B2B2C платформа, которая отвечает не текстом, а интерактивными виджетами.

## Концепция

Традиционные чат-боты отвечают текстом. Keepstar отвечает **интерфейсом**.

Пользователь спрашивает "Покажи кроссовки Nike до 15000₽" — и получает не список ссылок, а готовую галерею карточек товаров с фильтрами, сортировкой и кнопками "В корзину". Всё генерируется AI на лету.

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

Проект использует **гексагональную архитектуру** (Ports & Adapters):

```
                    ┌──────────────────────────────────┐
                    │           Domain Core            │
                    │  (бизнес-логика, use cases)      │
                    └──────────────────────────────────┘
                              ▲           ▲
                    ┌─────────┴───┐ ┌─────┴─────────┐
                    │   Ports     │ │    Ports      │
                    │  (inbound)  │ │  (outbound)   │
                    └─────────────┘ └───────────────┘
                          ▲               ▲
              ┌───────────┴───┐     ┌─────┴───────────┐
              │   Adapters    │     │    Adapters     │
              │  HTTP, gRPC   │     │  DB, LLM, Cache │
              └───────────────┘     └─────────────────┘
```

### Backend-First принцип

```
Frontend = "тупая рендерилка"
Backend  = вся логика, LLM, layout engine, state
```

Frontend получает готовый JSON и рендерит. Никакой бизнес-логики на клиенте.

### Атомарная композиция

```
Atoms → Widgets → Formations
```

- **Atoms**: базовые элементы (Text, Image, Button, Rating...)
- **Widgets**: композиция атомов (карточка = Image + Text + Rating + Button)
- **Formations**: расположение виджетов (grid, circle, stack...)

## Технологии

| Слой | Стек |
|------|------|
| Backend | Go 1.22+, Chi/Fiber, PostgreSQL |
| Frontend | React 18, Tailwind CSS, shadcn/ui |
| LLM | Claude Haiku (Anthropic) |
| Infra | Hetzner, Cloudflare |

## Структура проекта

```
Keepstar_one_ultra/
├── project/
│   ├── backend/           # Go API (гексагональная архитектура)
│   │   ├── cmd/
│   │   │   ├── server/    # HTTP server entry point
│   │   │   └── dbcheck/   # DB connectivity check
│   │   └── internal/
│   │       ├── domain/    # Core бизнес-логика
│   │       ├── ports/     # Interfaces (in/out)
│   │       ├── adapters/  # Implementations (postgres, anthropic, openai, json_store, memory)
│   │       ├── usecases/  # Business logic orchestration
│   │       ├── handlers/  # HTTP handlers, middleware, routes
│   │       ├── config/    # Configuration loading
│   │       ├── tools/     # Agent tools (catalog_search, etc.)
│   │       ├── prompts/   # LLM prompt templates
│   │       ├── presets/   # Widget/formation presets
│   │       └── logger/    # Structured logging
│   │
│   └── frontend/          # React widget (iframe, FSD architecture)
│       └── src/
│           ├── app/           # App initialization
│           ├── entities/      # Domain entities
│           ├── features/      # Feature modules
│           ├── shared/        # Shared utilities, API client
│           └── styles/        # Global styles
│
├── docs/                  # Документация
│   ├── ARCHITECTURE.md    # Детальная архитектура
│   ├── SPEC_TWO_AGENT_PIPELINE.md
│   └── UPDATES.md
│
├── AI_docs/               # Контекст для AI-агентов
│   ├── Manifesto          # Манифест продукта
│   ├── agent-principles.md
│   └── ARCHITECTURE_RULES.md
│
└── ADW/                   # SDLC оркестратор
```

## LLM Pipeline

Двухэтапный пайплайн для генерации ответов:

```
User Query
    │
    ▼
┌─────────────────────────┐
│  Stage 1: Query Analysis│  ← Понимание intent, получение данных
└─────────────────────────┘
    │
    ▼
┌─────────────────────────┐
│  Stage 2: Widget Compose│  ← Генерация структуры виджетов
└─────────────────────────┘
    │
    ▼
┌─────────────────────────┐
│  Layout Engine (no LLM) │  ← Расчёт позиций, lazy loading
└─────────────────────────┘
    │
    ▼
Response JSON
```

## Быстрый старт

```bash
# Backend
cd project/backend
go run cmd/server/main.go

# Frontend
cd project/frontend
npm install
npm run dev
```

## Embed на сайт

```html
<script
  src="https://keepstar.io/chat.js"
  data-tenant="YOUR_TENANT_ID"
  async>
</script>
```

## Документация

- [ARCHITECTURE.md](docs/ARCHITECTURE.md) — детальная архитектура, API, roadmap
- [Manifesto](AI_docs/Manifesto) — продуктовое видение

## Лицензия

Proprietary. All rights reserved.
