> ⚠️ **HISTORICAL — this plan has been executed/superseded.** The canonical current V5 architecture is in /FINAL_PHASE_PLAN.md and /SESSION_HANDOFF_2026-05-30.md. V5 is now in production as a modular monolith; the April 6–8 week v9-integration roadmap here is history. Kept for context on the decomposition decision. (flagged 2026-06-01)

# План: интеграция Keepstar_one_v9 в монорепо как фундамент Engine V5

## Context

**Что есть сейчас:**
- В проде V4 движок (`project_v4/backend/`, ветка `feature/engine-v4`) — Go-движок c хардкод-пресетами (12 штук в `presets_*.go`), 6 типов атомов, ограниченный layout (grid/list/single/carousel), без компонент-системы и переменных. Дался тяжело, ещё дорабатывается.
- Параллельно есть отдельный проект `/Users/starknight/Keepstar_project/Keepstar_one_v9` — самостоятельная Figma-class WYSIWYG канва с AI-агентом дизайнером. Стек: Turborepo + Bun, packages/domain (TS), packages/renderer (WebGL2 + SDF/MSDF шрифты), packages/layout (Yoga WASM), apps/web (React Vite, порт 5180), apps/api (Go Chi, порт 8090). Своя схема v2.10 с 13 типами нод (frame, group, rectangle, ellipse, line, polygon, path, text, icon_font, ref, note, prompt, context), своя batch_design DSL (I/C/U/R/D/M/G), система компонентов (RefNode + overrides, MAX_REF_DEPTH=10), переменные/темы ($varname), готовый Postgres адаптер.
- В админке текущая канва — `project_admin/frontend/src/features/canvas/CanvasPage.jsx` (785 строк tldraw editor) — будет заменена.

**Зачем v5:**
v9 операционно и архитектурно мощнее V4 (компоненты, переменные, темы, AI-дизайнер, профессиональный рендер), но не имеет того что есть у V4: data binding (атомы → данные товара), репликация (1 шаблон → 1500 карточек), сериализация пресетов, инжекция действий (like/cart), constraints, multi-widget композиции для сложных запросов, EntityRef. **V5 = тонкий runtime-слой (~1500-2000 LOC) поверх полной vendored-копии v9**, который добавляет именно эти механики и встраивает v9-документы в чат-пайплайн.

**Желаемый результат:**
- Маркетолог в админке открывает v9-канву (внутри iframe), визуально проектирует пресет (карточка товара, секция категорий, страница), сохраняет.
- Агент2 в чате при запросе "покажи кремы" не генерирует с нуля 200 карточек — берёт сохранённый v9-документ, биндит данные, реплицирует, накатывает batch_design ops для модификаций. Дёшево, быстро, предсказуемо.
- Агент2 продолжает уметь собирать сложные multi-widget композиции (несколько пресетов на одном экране) и тонко модифицировать через ops.

**Явно вне scope:** JSON streaming (не делаем).

---

## Recommended approach

Шесть этапов с чекпоинтами и точками отката. Общая оценка: **6-8 недель одним разработчиком, 4-5 недель парой**.

### Этап 0 — Vendor v9 в монорепо (1.5-2 дня)

**Цель:** перенести v9 целиком, оставить рабочим как самостоятельный сервис.

1. `cp -r /Users/starknight/Keepstar_project/Keepstar_one_v9 /Users/starknight/Keepstar_project/Keepstar_one_ultra/project_v9`
2. Зачистить `node_modules`, оставить `bun.lock` для воспроизводимости
3. Переключить storage v9 с `FileStorage` на готовый Postgres-адаптер: `project_v9/apps/api/cmd/server/main.go` строки 42-49 — заменить инстанс
4. Завести в основной БД схему `v9` (через `apps/api/internal/adapters/storage/postgres.go` уже умеет создавать таблицы)
5. Включить порт 8090 для v9-backend в общий `scripts/start_all.sh`
6. Smoke-тест: открыть v9 web (5180), создать документ — проверить что лежит в Postgres

**Точка отката:** просто удалить `project_v9/` если не нравится — ничего другого не тронуто.

**Критические файлы:**
- `project_v9/apps/api/cmd/server/main.go` (storage swap)
- `project_v9/apps/api/internal/adapters/storage/postgres.go` (готов к работе)
- `scripts/start_all.sh` (добавить запуск v9 backend)

### Этап 1 — v9 канва внутри админки (3-4 дня)

**Цель:** маркетолог открывает админку → раздел Canvas → редактирует в v9.

1. Убрать tldraw: `project_admin/frontend/src/features/canvas/CanvasPage.jsx` переписывается с 785 LOC до ~80 LOC — оборачивает iframe вокруг v9 web (`http://localhost:5180/?embed=1&token=<jwt>`)
2. В v9 добавить embed-режим: `project_v9/apps/web/src/App.tsx` — параметр `?embed=1` скрывает шапку/панели, оставляет только канву
3. Postmessage-мост: админка → iframe (передать tenant_id, jwt) и iframe → админка (события save/dirty)
4. JWT-проверка в `project_v9/apps/api/` — middleware читает токен админки (общий секрет в `.env`), извлекает tenant_id, изолирует документы по tenant
5. Tab "Pencil presets" в админке: список сохранённых документов из таблицы `pencil_doc_json`
6. Удалить `PresetTileShape.jsx` — больше не нужен

**Точка отката:** вернуть прежний `CanvasPage.jsx` из git.

**Критические файлы:**
- `project_admin/frontend/src/features/canvas/CanvasPage.jsx` (полностью переписать)
- `project_admin/frontend/src/features/canvas/PresetTileShape.jsx` (удалить)
- `project_v9/apps/web/src/App.tsx` (embed-mode)
- `project_v9/apps/api/internal/middleware/` (JWT-проверка, новый файл)
- `project_admin/backend/internal/adapters/postgres/admin_migrations.go` строки 65, 97 — переименовать `ops_json` → `pencil_doc_json` и добавить колонку `format` (TEXT, default 'v9-2.10')

### Этап 2 — Минимальный v5 движок (5-7 дней)

**Цель:** чат-пайплайн умеет загрузить v9-документ, забиндить данные, рендерить через DOM.

Новый пакет `project_v4/backend/internal/engine_v5/` со следующими файлами:
- `types.go` — Go-зеркало v9 schema (можно сгенерировать из `packages/domain/src/entities/nodes.ts`)
- `loader.go` — читать `pencil_doc_json` из БД через эволюционировавший `tenant_preset_loader.go`
- `bind.go` — обходить дерево, для нод с `binding?` подставлять значения из state.data (расширить `nodes.ts` полем `binding?: { source: string; field: string }`)
- `expand.go` — для нод с `repeat?` (новое поле в `nodes.ts`) реплицировать N раз с массивом данных. Использовать паттерн `ReplicateConfig` из `engine_v4/types.go`
- `runtime.go` — публичный `Execute(doc, data) → renderedDoc`

Добавить в `nodes.ts` (v9 domain):
```ts
binding?: { source: string; field: string };
repeat?: { source: string; itemVar: string };
```

Расширить таблицу `chat_state` колонкой `pencil_doc JSONB` (`postgres_state.go` миграция).

Фронтенд чата: новая папка `project/frontend/src/entities/pencil/` с `PencilDocRenderer.jsx` + `PencilNodeRenderer.jsx` — DOM-рендерер v9-документов (Yoga layout → CSS Flexbox через прямой mapping). НЕ тащим WebGL2 рендерер v9 в чат-бандл — слишком тяжело.

В `project_v4/backend/internal/usecases/pipeline_execute.go` (строки 93-412) добавить ветку: если у tenant включён v5-флаг → engine_v5.Execute, иначе старый V4 путь.

**Точка отката:** убрать ветку из pipeline_execute.go, выключить v5-флаг.

**Критические файлы:**
- `project_v4/backend/internal/engine_v5/` (новый пакет)
- `project_v9/packages/domain/src/entities/nodes.ts` (binding? + repeat?)
- `project_v4/backend/internal/engine_v4/tenant_preset_loader.go` (эволюционировать)
- `project_v4/backend/internal/adapters/postgres/canvas_preset_adapter.go` строки 47, 81, 116 (читать `pencil_doc_json`)
- `project_v4/backend/internal/adapters/postgres/postgres_state.go` (миграция: добавить pencil_doc JSONB)
- `project/frontend/src/entities/pencil/PencilDocRenderer.jsx` (новый)
- `project/frontend/src/entities/pencil/PencilNodeRenderer.jsx` (новый)
- `project_v4/backend/internal/usecases/pipeline_execute.go` (ветка v5)

### Этап 3 — Agent2 v5 + per-tenant feature flag (5 дней)

**Цель:** Agent2 умеет писать v9 batch_design DSL для модификаций пресета.

1. Новый промпт: `project_v4/backend/internal/prompts/agent2_v5.md` — основа из `project_v9/apps/web/src/agent/system-prompt.ts`, адаптировать под чат-контекст (state, history, slot resolution)
2. Новый tool: `project_v4/backend/internal/tools/tool_visual_assembly_v5.go` — принимает `preset_id` + `ops` (батч в v9 DSL) + `data_binding`. Парсит DSL переиспользуя логику v9 (port DSL parser из v9 в Go или RPC к v9 backend)
3. Per-tenant feature flag в `tenants` таблице: `engine_version TEXT DEFAULT 'v4'` — значения `v4` / `v5`
4. В `pipeline_execute.go` строки 318-328 — выбор tool на основе `tenant.engine_version`
5. Включить v5 для одного тестового tenant'а, прогнать золотые сценарии

**Точка отката:** UPDATE tenants SET engine_version = 'v4'.

**Критические файлы:**
- `project_v4/backend/internal/prompts/agent2_v5.md` (новый)
- `project_v4/backend/internal/tools/tool_visual_assembly_v5.go` (новый)
- `project_v4/backend/internal/usecases/pipeline_execute.go` (ветка по engine_version)
- `project_v4/backend/internal/engine_v5/dsl_parser.go` + `dsl_apply.go` (порт v9 DSL)

### Этап 4 — Полный порт V4-механик в v5 (7-10 дней)

**Цель:** v5 умеет всё что V4. **Критичная стадия — самые большие риски, тут v5 догоняет V4 по фичам**.

Перечень механик для порта (15 штук, выявленных аудитом session-логов V4):

1. **Multi-widget композиции** — Agent2 выбирает несколько пресетов на один экран → `engine_v5/multi_widget.go`
2. **Auto-sections** — автоматическое разделение на секции по category/tag → `sections.go` (паттерн из `engine_v4/sections.go`)
3. **Inline preset expansion с ref-prefix** — пресет внутри пресета через RefNode (использовать готовое v9 ref-разрешение) → `expand_inline_presets.go`
4. **TreeMap (compact context)** — после рендера строить компактное представление дерева для следующего turn'а Agent2 → `tree_map.go` (паттерн из `engine_v4/tree_ids.go` + `BuildTreeMap`)
5. **Slot resolution** — биндинг по слотам не только по fieldName → `slot_resolve.go` (зависит от `catalog.field_definitions`)
6. **Constraints engine per-group** — нормализация значений по правилам слота → `constraints.go` (паттерн из `engine_v4/constraints.go`, 30+ правил)
7. **EntityRef auto-detect** — определение виджет ↔ товар связки → `entity_ref.go`
8. **Default actions injection** — like/add_to_cart автоматом для entity-bound виджетов → `default_actions.go` (паттерн из `engine_v4/default_ops.go` `DefaultWidgetActions`)
9. **Mode flag** (rebuild vs modify) — для оптимизации турна Agent2
10. **Metadata-driven binding** (slot+fields+samples) — Agent2 видит мета и правильно пишет ops
11. **Pre-built views** — кеш готовых дерев для частых запросов
12. **Components system port** — переиспользовать v9 RefNode + overrides
13. **Variables/Themes per tenant** — переменные tenant'а доступны в v9-документах
14. **Replication groups** — `ReplicateConfig` с GroupID для разделения данных по группам в multi-widget
15. **Limit cap** — защита от рендера 10000+ нод

**Точка отката (на каждой подстадии):** конкретный feature-флаг внутри v5 (`v5.features.multiWidget = false`).

**Критические файлы:** все вышеперечисленные новые файлы в `engine_v5/`, плюс расширение `nodes.ts` в v9-домене для нужных полей.

### Этап 5 — Оптимизации (3-5 дней)

1. **Pre-built views**: кешировать готовые дерева в Redis/in-memory с инвалидацией по обновлению пресета
2. **Prompt caching**: в Agent2 v5 — system + tools + последние сообщения в cache_control breakpoints (Anthropic)
3. **Метрики стоимости**: сравнение V4 vs v5 по токенам/мс/$ на одинаковых сценариях

**Критические файлы:**
- `project_v4/backend/internal/engine_v5/cache.go` (новый)
- `project_v4/backend/internal/prompts/agent2_v5.md` (cache_control разметка)

### Этап 6 — Production rollout (5-7 дней)

1. **Migration tool**: скрипт `cmd/migrate_v4_to_v5/` — конвертирует существующие V4 пресеты в v9-документы (best-effort, ручная доработка в канве)
2. **A/B testing**: 50/50 для одного tenant'а, сравнение conversion + latency + cost
3. **Observability**: трейсы v5 пайплайна в `/debug/traces/` (использовать существующий `port.Trace`)
4. **Runbook**: краткая инструкция как откатить tenant с v5 на v4
5. **Постепенный rollout**: tenant за tenant'ом

---

## Существующие функции/утилиты для переиспользования

**Из V4 (`project_v4/backend/internal/engine_v4/`):**
- `tenant_preset_loader.go` — загрузка пресетов tenant'а (эволюционировать под v9-доки)
- `binding.go` `BindData()` — паттерн биндинга атомов из state.data
- `default_ops.go` `DefaultWidgetActions()`, `GridColumnsForCount()` — инжекция действий, layout-эвристики
- `constraints.go` — 30+ правил нормализации (паттерн на копирование)
- `sections.go` — auto-sections
- `tree_ids.go` `StampTreeIDs` + `BuildTreeMap` — компактный контекст для Agent2
- `replicate_behavior_test.go` — паттерны тестов репликации
- `engine.go` `Execute()` — общая последовательность пайплайна (init → ops → limit → replicate → bind → constraints → tree_map)

**Из V4 adapters (`project_v4/backend/internal/adapters/postgres/`):**
- `canvas_preset_adapter.go` строки 47, 81, 116 — SELECT-ы для пресетов
- `postgres_state.go` — паттерн миграций chat_state

**Из v9 (после vendoring → `project_v9/`):**
- `apps/api/internal/adapters/storage/postgres.go` — готовый Postgres-storage для v9-доков
- `packages/domain/` — все типы нод, RefNode-разрешение, variable-resolver
- `apps/web/src/agent/system-prompt.ts` — основа промпта Agent2 v5
- `apps/api/internal/usecases/batch_design.go` — DSL парсер (портировать в Go-код v5 или вызывать через RPC)
- Component-resolver и variable-resolver из `packages/domain/`

**Из админки (`project_admin/`):**
- `internal/adapters/postgres/admin_migrations.go` строки 65, 97 — место для миграции `ops_json → pencil_doc_json + format`

---

## Verification (как проверять end-to-end)

**После Этапа 0:** `bash scripts/start_all.sh` поднимает все сервисы включая v9 (8090) + v9 web (5180). Открыть v9 web, создать пустой документ, добавить frame с текстом — проверить через `psql` что запись в Postgres есть.

**После Этапа 1:** Открыть админку (5174) → Canvas → видим v9 редактор внутри iframe. Создать пресет "test-card", сохранить, перезагрузить страницу — пресет на месте. Через `psql` проверить запись в `pencil_doc_json`. JWT-токен другого tenant'а не видит чужие документы (curl-проверка).

**После Этапа 2:** Включить v5-флаг для тестового tenant'а. В чате запрос "покажи продукты" → бэкенд берёт сохранённый v9-документ, биндит данные первого товара, отдаёт фронту. Открыть DevTools → проверить структуру отрендеренного DOM соответствует v9-документу. `make test-unit` в `project_v4/backend/` зелёный.

**После Этапа 3:** Тот же запрос → Agent2 v5 возвращает batch_design ops, движок применяет → товар отображается с модификациями (например, изменён цвет цены). Сравнить latency vs V4 на тех же запросах.

**После Этапа 4:** Прогнать золотые сценарии V4 (multi-widget "косметика для лица + сравнение", auto-sections "по категориям", drill-down "детали продукта") в v5 — поведение совпадает или лучше. Все existing-тесты `engine_v4` имеют зеркала в `engine_v5_test`.

**После Этапа 5:** Метрики: средний токен/запрос v5 < 70% от V4. Latency p95 v5 ≤ V4. Cache hit rate > 60% на повторных запросах.

**После Этапа 6:** A/B тест 50/50 на одном tenant'е, неделя — conversion rate v5 ≥ V4 (без регрессии), p95 latency v5 ≤ V4, cost per session v5 < V4.
