# PitchLink — Spec

> Внутреннее рабочее имя — **PitchLink**. Продукт: пользователь загружает любые материалы (PDF, таблицы, сканы, текст, ссылки), говорит «сделай продающую страницу для X» — получает живую веб-ссылку. Делится ссылкой. Видит аналитику. Правит через чат.

Документ — самодостаточная спека: можно открыть и пилить, не держа в голове остальной контекст. Архитектура опирается на существующий V4-движок (`project_v4/backend/`) и фронтенд-рендерер из `project/frontend/` — переиспользуем максимум.

---

## 1. Зачем (продуктовое)

**Проблема пользователя**: продавец / фаундер / агентство хочет быстро отправить клиенту КП, презентацию, one-pager или мини-лендинг. Сейчас:
- PowerPoint/Figma — долго, нужны навыки
- PDF — не отслеживается, мёртвый
- Тильда/готовые лендинги — нужна вёрстка, не персонализируется под каждого клиента

**Наше решение**: загрузил кучу разнородных материалов → за минуту получил живую веб-страницу с трекингом и возможностью править фразой в чате.

**Killer feature**: одна и та же исходная пачка документов → разные персонализированные страницы под разных клиентов («сделай версию для retail» / «версию для enterprise»).

---

## 2. Сценарий пользователя (golden path)

1. Заходит на `pitchlink.app` → логин (Google OAuth, минимально)
2. **New pitch** → drag&drop файлов (PDF спецификации, Excel с ценами, скан брошюры, .txt с тезисами, ссылка на сайт-источник)
3. Промпт: «Сделай продающую страницу для сети салонов красоты, делаем акцент на маржинальность и простоту внедрения. Аудитория — закупщики.»
4. Прогресс-бар (ingest → extract → compose → render). 20–60 секунд.
5. Открывается **редактор**: слева превью лендинга, справа чат + список блоков.
6. Правит фразами: «убери блок с командой», «hero сделай тёмным», «добавь FAQ», «цены покажи в евро».
7. Жмёт **Share** → копирует публичную ссылку (`pitchlink.app/p/abc123`) или генерит персональную (`/p/abc123?to=ivan`).
8. Отправляет клиенту в мессенджере.
9. Возвращается через день → видит **Analytics**: кто открыл, сколько проскроллил, на каком блоке завис, кликнул ли CTA.
10. Может сделать форк («дубликат для другого клиента») — те же исходники, новый промпт.

---

## 3. Высокоуровневая архитектура

```
┌─────────────────────────────────────────────────────────────┐
│  Editor SPA (React)                  Public Page (SSR-lite) │
│  - upload, chat, preview              - rendered formation  │
│  - analytics dashboard                - analytics beacons   │
└────────────┬──────────────────────────────────┬─────────────┘
             │                                  │
             ▼                                  ▼
┌─────────────────────────────────────────────────────────────┐
│  PitchLink Backend (Go, новый сервис, порт 8090)            │
│                                                              │
│  ┌───────────┐  ┌──────────────┐  ┌─────────────────────┐  │
│  │ Documents │  │ Pitch Pipeline│  │ Public + Analytics │  │
│  │ (ingest)  │  │ (Agent1+2)    │  │ (share, events)    │  │
│  └─────┬─────┘  └──────┬───────┘  └──────────┬──────────┘  │
│        │               │                      │             │
└────────┼───────────────┼──────────────────────┼─────────────┘
         │               │                      │
         ▼               ▼                      ▼
   ┌──────────┐    ┌────────────┐         ┌──────────┐
   │ Storage  │    │ V4 Engine  │         │ Postgres │
   │ (S3/R2)  │    │ (reused)   │         │ (Neon)   │
   └──────────┘    └─────┬──────┘         └──────────┘
                         │
                         ▼
                  ┌──────────────┐
                  │  Anthropic   │
                  │  (Claude 4.5)│
                  └──────────────┘
```

**Ключевая идея**: PitchLink — это не отдельный движок. Это новый тонкий сервис, который дёргает **уже существующий V4 engine** как библиотеку (или через HTTP, см. §10) с другим набором пресетов и другими tools. Вся механика ops/binding/tree_map/chat-edit переиспользуется как есть.

---

## 4. Что переиспользуем из Keepstar (точечно)

| Компонент | Откуда | Что меняется |
|---|---|---|
| Ops engine + ApplyOps + tree_map | `project_v4/backend/internal/engine_v4/engine.go` | Ничего, используем как есть |
| BindData (data → atoms by FieldName) | `engine_v4/binding.go` | Ничего |
| Constraints | `engine_v4/constraints.go` | Расширить под landing-атомы (см. §6) |
| Visual assembly tool | `internal/tools/tool_visual_assembly.go` | Импортируем, добавляем новые presets |
| Agent2 compose prompt | `internal/prompts/prompt_compose_widgets.go` | Новый промпт `prompt_compose_landing.go`, та же структура |
| FormationRenderer + AtomRenderer | `project/frontend/src/entities/` | Без изменений, добавить новые atom-display варианты |
| Theming via CSS vars | `project/frontend/src/shared/theme/` | Базис для динамических тем (см. §7) |
| Trace система | `engine_v4` traces | Используем для дебага pitch generation |

**Что НЕ берём**: catalog, hybrid search, product enrichment, admin импортёр — другая доменная область.

---

## 5. Документ ingest (новый слой)

Самая важная новая часть. Без хорошего ingest всё остальное бесполезно.

### 5.1. Source types

| Тип | Обработчик | Извлекаем |
|---|---|---|
| `application/pdf` | `pdfcpu` + Claude vision per-page | text + tables + image regions |
| `image/*` (jpg/png/heic) | Claude vision напрямую | OCR + структура |
| `text/csv`, `xlsx` | `excelize` + headers detection | rows as JSON |
| `text/plain`, `text/markdown` | как есть | raw text |
| URL (https) | headless fetch + readability | clean article text + og:image |
| `application/json` | как есть | structured |

Все source'ы конвертируются в единый промежуточный формат `RawDoc`:

```go
type RawDoc struct {
    ID         string            // doc_<ulid>
    SourceType string            // "pdf" | "image" | "table" | "url" | ...
    Filename   string
    Pages      []DocPage         // для PDF/изображений
    Tables     []DocTable        // нормализованные таблицы
    Text       string            // полный конкатенированный текст для embed/search
    Assets     []DocAsset        // картинки, извлечённые из PDF, с blob-ссылками
    Metadata   map[string]any    // page count, dimensions, etc
}

type DocPage struct {
    Index    int
    Text     string
    Images   []string  // asset IDs
}

type DocAsset struct {
    ID       string  // asset_<ulid>
    URL      string  // S3/R2 public url
    Width    int
    Height   int
    Caption  string  // alt-текст от vision модели
}
```

### 5.2. Extract pipeline

```
Upload → store raw blob in R2 → enqueue ingest job
  → per-source parser → RawDoc
  → Claude vision pass (для pdf/image): "опиши структуру, выдели заголовки, цены, фичи, картинки"
  → store RawDoc as JSON in postgres (table: pitch_documents)
  → mark ready
```

Ingest job — async (см. §11). На фронте — прогресс по каждому файлу.

### 5.3. Asset extraction

Картинки из PDF и загруженные изображения сразу:
1. Заливаются в R2 (`pitchlink-assets` bucket)
2. Получают caption через Claude vision (`{ "alt": "Скриншот дашборда с графиком продаж", "type": "screenshot|photo|logo|chart|illustration" }`)
3. Сохраняются в `pitch_assets` таблицу

Это позволяет Agent2 выбирать подходящие картинки под блоки лендинга по семантике (см. §6.4).

---

## 6. Pitch Generation (Agent1 + Agent2)

### 6.1. Pipeline

```
User submits prompt + selected docs
  ↓
Agent1 (Extractor): получает RawDocs + user prompt
  - tool: query_documents(question) → семантический поиск по text+captions
  - tool: select_assets(intent) → подобрать релевантные изображения
  - tool: extract_structured(schema) → вытащить поля типа "company_name", "price", "features[]"
  - результат: PitchData (см. ниже)
  ↓
Agent2 (Composer): получает PitchData + user prompt + landing presets list
  - использует существующий visual_assembly tool
  - возвращает Formation tree (как для V4 widgets, только пресеты другие)
  ↓
V4 Engine.Execute(): ApplyOps → BindData(PitchData) → Constraints → tree_map
  ↓
Persisted as Pitch (см. §8 модель данных)
```

### 6.2. PitchData shape

PitchData — нормализованный JSON, который Agent1 готовит для Agent2. Это «тёплая» интерпретация исходников под цель пользователя.

```json
{
  "meta": {
    "audience": "retail buyers",
    "tone": "professional, confident",
    "language": "ru",
    "primary_color_hint": "#8B5CF6"
  },
  "company": {
    "name": "Acme Cosmetics",
    "tagline": "Премиум-уход без переплат",
    "logo_asset_id": "asset_01h..."
  },
  "hero": {
    "headline": "Маржинальность 60% и поставки за 3 дня",
    "subheadline": "Закупочная программа для сетей",
    "image_asset_id": "asset_01h..."
  },
  "features": [
    { "title": "...", "description": "...", "icon": "shield" },
    ...
  ],
  "pricing": [
    { "name": "Starter", "price": "€500/мес", "features": [...] }
  ],
  "testimonials": [...],
  "faq": [...],
  "cta": { "label": "Запросить условия", "href": "mailto:..." }
}
```

PitchData — это **именно та абстракция, которой не хватает в V4 каталоге**: вместо «product fields» (images/name/price) у нас «landing sections». Agent2 биндит PitchData → атомы пресетов через FieldName, точно как сейчас биндит product → product_card.

### 6.3. Новые пресеты (landing kit)

В `project_v4/backend/internal/engine_v4/presets_landing.go`:

| Preset ID | Что строит | Слоты (FieldName) |
|---|---|---|
| `landing_hero_centered` | full-width hero, заголовок по центру + CTA | `hero.headline`, `hero.subheadline`, `hero.image`, `cta.label` |
| `landing_hero_split` | hero с картинкой справа | то же |
| `landing_features_grid` | сетка 3×2 фич с иконкой | `features[].title`, `features[].description`, `features[].icon` |
| `landing_features_alternating` | поочерёдные text/image ряды | то же |
| `landing_pricing_table` | 2–4 колонки тарифов | `pricing[].name`, `pricing[].price`, `pricing[].features` |
| `landing_testimonials` | карусель отзывов | `testimonials[].quote`, `testimonials[].author` |
| `landing_logo_strip` | лента логотипов клиентов | `clients[].logo` |
| `landing_faq_accordion` | список FAQ | `faq[].question`, `faq[].answer` |
| `landing_cta_block` | финальный CTA | `cta.label`, `cta.href` |
| `landing_text_section` | произвольный текст-блок (для произвольных историй) | `text.title`, `text.body` |

Пресеты строятся **точно по образцу `presets_product.go`** — те же ops-builders, тот же `DefaultReplicate=false` (где не нужно) или `=true` (для features/pricing/testimonials, биндящихся из массивов).

**Важно**: реализовать через slot-resolution из `field_definitions` (B7 в трекере). Hardcoded fieldName здесь — путь в тупик с самого начала, потому что у каждого pitch будут свои поля. См. §13 (открытые вопросы — это надо решить до старта кода).

### 6.4. Asset selection

Отдельный tool у Agent1: `select_asset(intent: "hero" | "feature" | "logo" | "background", description)`. Под капотом — embedding-поиск по caption-полям ассетов + фильтр по `type`. Возвращает asset_id, который Agent2 кладёт в нужный FieldName.

---

## 7. Темизация и стиль

Исходники у пользователя разные → нужно унифицировать визуал. Решение: **theme tokens**.

```go
type PitchTheme struct {
    PrimaryColor    string  // hex
    SecondaryColor  string
    BackgroundColor string
    TextColor       string
    FontHeading     string  // google font name
    FontBody        string
    Radius          string  // "sharp" | "soft" | "round"
    Density         string  // "compact" | "comfortable" | "spacious"
}
```

Theme:
- Извлекается Agent1 из исходников (если есть лого / brand book PDF — vision определяет primary color, иначе дефолт)
- Хранится на уровне Pitch
- Рендерится как CSS variables в `<style>` блок публичной страницы
- Меняется через чат: «сделай тёмную тему», «фирменный цвет — #FF5722»

**Pencil MCP** здесь полезен **только если** мы хотим визуальный редактор поверх. Для MVP не нужен — Pencil это про .pen файлы и сложные правки геометрии. Темизация через CSS vars проще и быстрее. Откладываем Pencil-интеграцию до v2 (см. §15).

---

## 8. Модель данных

Новая БД-схема `pitchlink` в существующем Neon Postgres.

```sql
CREATE SCHEMA pitchlink;

-- Пользователи (минимально, OAuth)
CREATE TABLE pitchlink.users (
    id            TEXT PRIMARY KEY,         -- user_<ulid>
    email         TEXT UNIQUE NOT NULL,
    name          TEXT,
    avatar_url    TEXT,
    created_at    TIMESTAMPTZ DEFAULT now()
);

-- Workspace = коллекция документов и питчей одного юзера
-- (на MVP: 1 user = 1 workspace, но поле есть для будущего team-режима)
CREATE TABLE pitchlink.workspaces (
    id         TEXT PRIMARY KEY,
    owner_id   TEXT REFERENCES pitchlink.users(id),
    name       TEXT,
    created_at TIMESTAMPTZ DEFAULT now()
);

-- Загруженные документы (источники)
CREATE TABLE pitchlink.documents (
    id            TEXT PRIMARY KEY,         -- doc_<ulid>
    workspace_id  TEXT REFERENCES pitchlink.workspaces(id),
    filename      TEXT NOT NULL,
    source_type   TEXT NOT NULL,            -- pdf|image|table|url|text
    blob_url      TEXT NOT NULL,            -- R2
    raw_doc       JSONB,                    -- parsed RawDoc (см §5.1)
    status        TEXT NOT NULL,            -- pending|processing|ready|failed
    error         TEXT,
    created_at    TIMESTAMPTZ DEFAULT now()
);

-- Извлечённые ассеты (картинки из PDF и т.п.)
CREATE TABLE pitchlink.assets (
    id            TEXT PRIMARY KEY,         -- asset_<ulid>
    document_id   TEXT REFERENCES pitchlink.documents(id) ON DELETE CASCADE,
    workspace_id  TEXT REFERENCES pitchlink.workspaces(id),
    url           TEXT NOT NULL,
    type          TEXT,                     -- screenshot|photo|logo|chart|illustration
    caption       TEXT,
    width         INT,
    height        INT,
    embedding     VECTOR(384)               -- pgvector для семантического поиска
);

-- Питч = одна сгенерированная страница
CREATE TABLE pitchlink.pitches (
    id            TEXT PRIMARY KEY,         -- pitch_<ulid>
    workspace_id  TEXT REFERENCES pitchlink.workspaces(id),
    title         TEXT,
    public_slug   TEXT UNIQUE,              -- "abc123" → /p/abc123
    public_status TEXT DEFAULT 'private',   -- private|public|password
    password_hash TEXT,                     -- если public_status=password
    pitch_data    JSONB,                    -- PitchData (см §6.2)
    formation     JSONB,                    -- Formation tree от V4 engine
    theme         JSONB,                    -- PitchTheme
    source_doc_ids TEXT[],                  -- какие документы использовались
    created_at    TIMESTAMPTZ DEFAULT now(),
    updated_at    TIMESTAMPTZ DEFAULT now()
);

-- История чат-правок (turn-based)
CREATE TABLE pitchlink.pitch_turns (
    id          TEXT PRIMARY KEY,
    pitch_id    TEXT REFERENCES pitchlink.pitches(id) ON DELETE CASCADE,
    role        TEXT NOT NULL,              -- user|assistant
    content     TEXT,
    ops_applied JSONB,                      -- какие ops применились (для отката)
    created_at  TIMESTAMPTZ DEFAULT now()
);

-- Аналитика просмотров
CREATE TABLE pitchlink.view_events (
    id           BIGSERIAL PRIMARY KEY,
    pitch_id     TEXT REFERENCES pitchlink.pitches(id) ON DELETE CASCADE,
    visitor_id   TEXT,                      -- cookie-based, anonymous
    event_type   TEXT NOT NULL,             -- view|scroll|cta_click|share
    event_data   JSONB,                     -- {block_id, scroll_pct, cta_id, ...}
    referrer     TEXT,
    user_agent   TEXT,
    ip_country   TEXT,                      -- из CF-IPCountry
    created_at   TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX ON pitchlink.view_events (pitch_id, created_at DESC);
CREATE INDEX ON pitchlink.documents (workspace_id);
CREATE INDEX ON pitchlink.pitches (workspace_id);
CREATE INDEX ON pitchlink.assets USING hnsw (embedding vector_cosine_ops);
```

---

## 9. API (PitchLink Backend, порт 8090)

REST + JSON. Auth — Bearer token (JWT после OAuth).

### 9.1. Documents

```
POST   /api/documents/upload          multipart, returns {document_id, status: "processing"}
GET    /api/documents/:id              {raw_doc, status, assets}
DELETE /api/documents/:id
GET    /api/workspaces/current/documents
```

### 9.2. Pitches

```
POST   /api/pitches                    body: {title, source_doc_ids, prompt}
                                       → kicks off generation, returns {pitch_id, status: "generating"}
GET    /api/pitches/:id                full pitch (data, formation, theme, turns)
POST   /api/pitches/:id/chat           body: {message} → applies ops, returns updated formation
POST   /api/pitches/:id/regenerate     body: {prompt} → full re-generation
POST   /api/pitches/:id/duplicate      body: {prompt?} → новая копия
PATCH  /api/pitches/:id/share          body: {public_status, password?} → returns {share_url}
DELETE /api/pitches/:id
```

### 9.3. Public

```
GET    /api/public/:slug               {formation, theme, title} (no auth, no PII)
POST   /api/public/:slug/event         body: {event_type, event_data, visitor_id}
                                       (анонимный, rate-limited)
```

### 9.4. Analytics

```
GET    /api/pitches/:id/analytics      агрегаты:
                                       - total_views, unique_visitors
                                       - avg_scroll_depth, avg_time_on_page
                                       - top_blocks (по dwell time)
                                       - cta_clicks
                                       - timeline (views по дням)
                                       - geo (top countries)
```

---

## 10. Интеграция с V4 Engine: библиотека или HTTP?

Два варианта:

**Вариант А — V4 как Go-библиотека** (предпочтительный)
- PitchLink импортирует `project_v4/backend/internal/engine_v4` напрямую
- Один монорепо, общий go.mod
- Плюс: ноль overhead, синхронный вызов, общие типы
- Минус: PitchLink и V4 деплоятся вместе → нужно следить за совместимостью

**Вариант Б — V4 как HTTP-сервис**
- PitchLink дёргает `v4-engine-production.up.railway.app` (он уже задеплоен)
- Плюс: независимый деплой
- Минус: latency, сериализация, версионирование контракта

**Решение для MVP**: **Вариант А**. Импортируем engine_v4 как библиотеку. PitchLink backend живёт в новом каталоге `pitchlink/backend/`, но в том же go.work. Когда PitchLink стабилизируется — можно вынести.

---

## 11. Async jobs

Ingest и pitch generation — небыстрые операции (10–60s). Нужен очередь-воркер.

**MVP-подход (без новой инфры)**: PostgreSQL-based queue.
- Таблица `pitchlink.jobs (id, type, payload, status, attempts, created_at, started_at, finished_at, error)`
- В backend крутится N горутин-воркеров (`for { SELECT ... FOR UPDATE SKIP LOCKED LIMIT 1 }`)
- Типы: `ingest_document`, `generate_pitch`, `apply_chat_turn`
- Прогресс — пишется в `jobs.payload->>'progress'`, фронт поллит `GET /api/jobs/:id` каждые 1.5s

Ничего экзотического (Redis/Temporal) на MVP не надо.

---

## 12. Frontend

Два разных приложения:

### 12.1. Editor SPA (`pitchlink/frontend-editor/`)

React 19 + Vite. Layout:
```
┌─────────────────────────────────────────────────────────┐
│  Top bar: pitch title, Share button, Analytics button   │
├──────────────┬──────────────────────────┬───────────────┤
│ Documents    │  Live preview (iframe    │  Chat         │
│ panel        │   с FormationRenderer)   │  + ops log    │
│ (uploads,    │                          │               │
│  asset       │                          │               │
│  gallery)    │                          │               │
└──────────────┴──────────────────────────┴───────────────┘
```

Preview — в iframe, чтобы CSS лендоса не конфликтовал с CSS редактора. После каждого turn — message через `postMessage` с новым formation, iframe ререндерит без полного reload.

Переиспользуем `FormationRenderer` из `project/frontend/src/entities/formation/`.

### 12.2. Public Page (`pitchlink/frontend-public/`)

Минимальный React-бандл. Один route — `/p/:slug`. Делает:
1. `GET /api/public/:slug` → получает formation + theme
2. Применяет theme как CSS vars в `<html>`
3. Рендерит `<FormationRenderer>`
4. Шлёт beacon: `view` на mount, `scroll` на каждые 25%, `cta_click` на клики по CTA-атомам, `unload` с total time

`visitor_id` — first-party cookie на 1 год, ставится на public-домене.

**SEO**: для MVP — клиентский рендер ок. v2 — server-side prerender для og-картинки и описания.

### 12.3. Analytics view (внутри Editor SPA)

Открывается из top-bar. Карточки + графики (recharts):
- Big numbers: views, unique, avg time, CTA conversion
- Timeline (views по дням, line chart)
- Heatmap блоков (какие блоки больше всего смотрели)
- Гео-таблица
- Список последних visitor-сессий (с возможностью drill-down: кто, когда, что смотрел, докуда доскроллил)

---

## 13. Открытые вопросы (решить до старта кода)

1. **Slot resolution через field_definitions**. Текущие V4 пресеты hardcoded под `images/name/price`. Для landing-пресетов это не сработает (поля произвольные). Решение: либо ускорить B7 (роль-based slots), либо для PitchLink сделать **отдельный набор field_definitions** (`landing.field_definitions` schema) с записями типа `hero.headline → text/h1`, `features[].icon → icon`. **Нужно решить какой путь.**

2. **PitchData как formal schema или freeform**. Если formal — Agent1 проще, но негибко. Если freeform — Agent2 решает что куда биндить, гибче но дороже по токенам. **Предлагаю**: formal с extension-полем `extras` для произвольных секций.

3. **Регенерация vs incremental edit**. После 30 чат-правок formation становится мутным. Нужна стратегия: либо периодический «consolidate» (Agent2 пересобирает с чистого листа из текущего PitchData), либо честный undo-стек по turns. Предлагаю: явная кнопка «Regenerate with edits applied» + undo последних 10 turns.

4. **Лимиты**. Сколько документов на pitch (10?), макс размер файла (50MB?), макс pitches на user в день (5?). Нужно чтобы не разорить нас на vision API. На MVP: hard cap, потом — billing.

5. **Какие модели**. Ingest vision — Sonnet 4.6 (точность важнее скорости). Agent2 compose — Haiku 4.5 (как в V4). Agent1 extract — Sonnet 4.6 на первом проходе, потом Haiku при чат-правках.

6. **Multi-tenant изоляция**. Сейчас V4 в принципе не имеет user-уровня (всё под одним TENANT_SLUG). PitchLink с самого начала должен быть multi-tenant. Workspace_id протаскивается через все запросы, проверка ownership на каждом endpoint.

---

## 14. План работ (волны)

**Wave 0 — Skeleton (день 1)**
- Создать `pitchlink/backend/` (Go), `pitchlink/frontend-editor/`, `pitchlink/frontend-public/`
- go.work с импортом engine_v4
- Postgres миграции (см §8)
- Минимальный health endpoint, OAuth заглушка

**Wave 1 — Document ingest (день 2–3)**
- Upload endpoint + R2 (или локальный fs для dev)
- PDF parser (pdfcpu) → text + assets
- Image OCR через Claude vision
- Excel/CSV парсер
- URL fetcher
- Job queue + worker (см §11)
- Frontend: drop-zone, list, статусы

**Wave 2 — Pitch generation (день 4–5)** ← сердце
- Новые пресеты `landing_*` в engine_v4 (hero, features, pricing, cta, text, faq)
- Решить вопрос §13.1 (field_definitions для landing)
- Agent1 extractor: tools `query_documents`, `select_asset`, `extract_structured`
- Agent2 composer: новый prompt + visual_assembly с landing-presets
- POST /api/pitches → job → готовый pitch
- Editor preview iframe
- Smoke-test на 3 разных pitch-сценариях

**Wave 3 — Chat editing (день 6)**
- POST /api/pitches/:id/chat → ops через Agent2 (тот же механизм что в V4)
- Frontend: chat panel, live update preview через postMessage
- Undo stack (хранить ops_applied в pitch_turns)

**Wave 4 — Sharing + Public page (день 7)**
- Slug generation, public_status, password
- Public route + frontend-public bundle
- Theme injection как CSS vars
- Beacon endpoint + visitor cookie

**Wave 5 — Analytics (день 8)**
- Aggregation queries
- Analytics view в Editor SPA (карточки + графики)
- Heatmap по блокам (нужны block_id в Formation — V4 уже их генерит через StampTreeIDs ✓)

**Wave 6 — Polish (день 9–10)**
- Auth (Google OAuth real)
- Лимиты и rate limiting
- Error states, empty states
- Deploy на Railway (отдельный сервис)
- Custom domain + SSL для public pages

---

## 15. Что НЕ делаем в MVP (явно)

- Нет teams/collaboration. 1 user = 1 workspace.
- Нет версий/branches. Pitch = одна линейная история turns.
- Нет встроенного редактора (drag&drop блоков). Только чат.
- Нет интеграции с Pencil MCP. Темизация через CSS vars достаточна.
- Нет email-уведомлений («ваш pitch посмотрели»). Шлём только в UI.
- Нет A/B тестов разных версий pitch.
- Нет lead capture форм внутри лендоса (пока CTA — это только ссылка mailto/href).
- Нет SSR/SEO. Public page — клиентский рендер.
- Нет custom CSS в чате («примени этот CSS»). Только high-level правки.
- Нет экспорта в PDF/PNG. (Просто, но не обязательно для MVP — можно добавить через `export_nodes` Pencil или headless chrome за день.)

Эти пункты — для phase 2 после валидации.

---

## 16. Метрики успеха MVP

Через 2 недели после запуска:
- 10+ внешних юзеров создали хотя бы 1 pitch
- 50%+ из них дошли до Share (то есть результат был достаточно хорош, чтобы отправить)
- Среднее время от upload до share ссылки < 5 минут
- Стоимость генерации одного pitch < $0.30
- Hit rate чат-правок «получил то что просил с первого раза» > 70%

Если хоть одна метрика проваливается — стоп, разбираемся, не накидываем фичи.

---

## 17. Риски и контр-меры

| Риск | Митигация |
|---|---|
| Vision модель плохо парсит сложные PDF (многоколонки, формулы) | Fallback на text-only режим, явно говорим юзеру «слой картинок не распознан» |
| Agent1 галлюцинирует данных, которых нет в источниках | В системном промпте: «only use facts present in <docs>». Доп. валидатор-проход по PitchData |
| Лендинги получаются однотипными | 2 варианта hero + 2 варианта features → Agent2 выбирает по `tone`. Phase 2 — больше пресетов |
| Стоимость vision $$$$ | Кешируем RawDoc в БД (никогда не парсим один файл дважды). Лимит файлов на pitch |
| Юзер хочет правку, которую чат не может («сделай этот блок зигзагом») | Честно отвечаем «not supported», кладём в фичреквесты |
| Утечка приватных pitches через public slug guess | Slug ≥ 16 chars random base62, + опциональный пароль |

---

## 18. Резюме одной фразой

Берём готовый V4 ops-движок, прикручиваем к нему документ-ingest и landing-пресеты, оборачиваем в сервис с публичными ссылками и аналитикой — получаем продукт «загрузил → отправил клиенту → видишь как смотрят».

Главная работа — **не движок** (он есть), а **ingest** (новый код) и **landing presets через field_definitions** (требует решения по §13.1). Всё остальное — обвязка.
