# Epic: Embeddable Widget + Multi-Tenant Deploy

**Дата:** 2026-02-10 20:00
**Статус:** Draft
**Приоритет:** Critical — это путь к первым клиентам

## Суть

Один JS-бандл (`widget.js`) который клиент вставляет на свой сайт одной строкой:
```html
<script src="https://keepstar.one/widget.js" data-tenant="nike"></script>
```

Появляется кнопка чата → по клику открывается чат + область для formations (карточки товаров, детали, карусели). Всё в shadow DOM — изолировано от стилей сайта клиента.

## Домены и сервисы

```
keepstar.one              → Основной продукт (Amvera + Vercel)
                            ├── Go backend (API + widget.js статика)
                            ├── Админка (React SPA, project_admin/)
                            └── Всё в одном месте: API, виджет, админка

swarm-engineer.ru         → Демо-клиент #1 (имитация сайта клиента)
                            HTML/React страница + <script src="keepstar.one/widget.js" data-tenant="...">

swarmengineer.ru          → Демо-клиент #2 (имитация сайта клиента)
                            Простая HTML-страница + <script src="keepstar.one/widget.js" data-tenant="...">
```

## Архитектура

### Что есть сейчас
```
[React App :5173]  →  [Go Backend :8080]  →  [PostgreSQL]
     ↑                      ↑
  localhost            localhost
  полноэкранное        API + formation
  приложение           сборка
```

### Что нужно
```
keepstar.one                          ← Основной сервер (Amvera)
  ├── /api/v1/*                       ← Go backend API
  ├── /widget.js                      ← статика, Go отдаёт файл
  └── /admin/*                        ← Админка SPA (или отдельный Vercel)

[Сайт клиента: swarm-engineer.ru / swarmengineer.ru / любой]
     ↓
  <script src="https://keepstar.one/widget.js" data-tenant="nike">
     ↓
  [Shadow DOM Container]
  ├── Chat Panel (постоянный, справа)
  ├── Formation Overlay (карточки/детали, по центру/слева)
  └── Кнопка-триггер (плавающая, угол экрана)
     ↓
  fetch → keepstar.one/api/v1/*  →  [PostgreSQL]
```

### Ключевые принципы

1. **Один бандл** — виджет = фронт. Не два отдельных проекта. Чат + рендерер formations + оверлей = один `widget.js`
2. **Бэк собирает, фронт рисует** — formation приходит готовым JSON, фронт рендерит без логики
3. **Shadow DOM** — полная изоляция стилей. CSS сайта клиента не ломает виджет, виджет не ломает сайт. Справляется с любым объёмом рендеринга — это просто изоляция, не ограничение
4. **Tenant из атрибута** — `data-tenant="nike"` → header `X-Tenant-Slug: nike` → бэк резолвит

## Компоненты

### 1. Widget Shell (новый entry point)

Файл: `project/frontend/src/widget.jsx`

Что делает:
- Создаёт shadow DOM контейнер на странице клиента
- Рендерит React-приложение внутри shadow root
- Внутри: ChatPanel + FormationOverlay + триггер-кнопка
- Читает `data-tenant` из `<script>` тега → передаёт в API клиент как header

По сути это текущий `App.jsx`, но:
- Обёрнут в shadow DOM вместо `#root`
- Стили инлайнятся в бандл (не внешний CSS)
- Минимальный размер: чат справа, formations слева/по центру как overlay

### 2. API Client — tenant header

Файл: `project/frontend/src/shared/api/apiClient.js`

Все запросы шлют `X-Tenant-Slug` header (уже поддержан CORS и middleware).

### 3. Formation Overlay

То что сейчас рендерится в отдельной зоне `App.jsx` — станет overlay поверх сайта клиента. Открывается когда приходит formation, закрывается по клику вне или по кнопке.

### 4. Сборка

Vite конфиг для library mode:
- Entry: `widget.jsx`
- Output: один файл `widget.js` (IIFE, не ESM)
- CSS инлайнится в JS
- React включён в бандл (клиент не должен ставить зависимости)
- Целевой размер: ~50-80KB gzipped (React + рендереры)

## Деплой

### Схема
```
keepstar.one                         ← Amvera (всё в одном)
  ├── /api/v1/*                      ← Go backend API
  ├── /widget.js                     ← статика (Go отдаёт собранный бандл)
  └── /admin/                        ← Админка SPA (Vercel или встроена)

swarm-engineer.ru                    ← Демо-клиент #1
  └── React-приложение + <script src="https://keepstar.one/widget.js" data-tenant="...">

swarmengineer.ru                     ← Демо-клиент #2 (позже)
  └── HTML-страница + <script src="https://keepstar.one/widget.js" data-tenant="...">
```

Widget.js раздаётся прямо с Go backend на keepstar.one. Для MVP — один сервер, просто. CDN добавим когда будет нагрузка.

## Нагрузка и масштабирование

**MVP уровень: 10 клиентов × 50 юзеров = 500 одновременных**

- **Go backend** — тысячи goroutines из коробки. 500 юзеров — не проблема.
- **PostgreSQL** — pgxpool уже используется, 20-50 коннектов хватит.
- **LLM — bottleneck.** Реально одномоментно шлют запрос 5-10% юзеров (остальные читают/скроллят) = 25-50 запросов. Anthropic rate limits выдержат.
- **Брокер (Kafka и т.д.)** — для MVP не нужен. Go нативно обрабатывает concurrent requests.

**Когда понадобится масштабирование (не сейчас):**
- Rate limiter на LLM запросы (очередь с приоритетами)
- Несколько инстансов бэкенда за load balancer
- Кэширование частых запросов (одинаковые товарные запросы → cached formation)
- Переключение LLM провайдеров через админку (уже в планах)

## Этапы реализации

### Этап 1: Widget Build (1 сессия)
- Vite library config → `widget.js` IIFE бандл
- Shadow DOM shell (`widget.jsx`)
- Инлайн стили
- `data-tenant` → API header
- Тест: `<script>` на пустой HTML → чат работает

### Этап 2: Formation Overlay (1 сессия)
- Overlay компонент внутри shadow DOM
- Позиционирование: чат справа, formations по центру
- Анимации входа/выхода
- Клик по карточке → expand (уже работает)
- Тест: запрос → карточки появляются в overlay

### Этап 3: Deploy на keepstar.one (1 сессия)
- Go backend раздаёт widget.js как static file
- Деплой на Amvera (keepstar.one)
- CORS настроен для любого origin (уже `*`)
- Тест: keepstar.one/widget.js доступен

### Этап 4: Демо-клиенты (1 сессия)
- swarm-engineer.ru → React-приложение + `<script>` виджета
- swarmengineer.ru → HTML страничка + `<script>` виджета с другим data-tenant
- Проверка мультитенантности end-to-end

### Этап 5: Tenant Branding (позже)
- Цвета/лого из tenant settings
- Кастомный greeting из админки
- Позиция виджета (настройка)

## Что НЕ меняется
- Backend API — всё уже готово (session init, pipeline, navigation)
- Formation сборка — бэк собирает, фронт рендерит
- Компоненты рендеринга (FormationRenderer, WidgetRenderer, AtomRenderer) — переиспользуются as-is
