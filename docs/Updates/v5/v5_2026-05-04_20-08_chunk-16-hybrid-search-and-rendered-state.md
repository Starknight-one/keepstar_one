# V5 — Chunk 16: Hybrid catalog search + rendered subset in Agent1 context

**Branch**: v5
**Date (UTC)**: 2026-05-04 20:08
**Commit**: (pending — will land in next git commit; this log written before commit per Plan-mode rule)
**Parent**: 88ce3fb

## Context

Vlad открыл prod-виджет 2026-05-04 и попробовал прямые запросы. Запрос
«Привет, покажи крема» (тенант `hey-babes-cosmetics`, сессия
`a3367a7d-eaac-4a94-99e2-195513b4d379`) вернул `empty_not_found` несмотря
на то что в каталоге 23 продукта с `product_form='cream'` и 56 с подстрокой
«крем» в имени.

Расследование по БД показало два слоя:

1. **catalog_search в V5 — keyword-only ILIKE.** В коде `tool_catalog_search.go:14-18`
   зафиксировано как намеренное упрощение от chunk-7: «hybrid path
   (pgvector + RRF) is deferred until V5 grows an EmbeddingPort». Схема
   поля `vector_query` оставлена ради совместимости с V4-prompts, но
   executor роутит строку в `ListProducts.Search` → ILIKE `%substring%`.
   Russian inflection («крема» vs «крем»), translation, опечатки →
   silently 0 строк. Filter `product_form='cream'` AND ILIKE `%крема%`
   = 0 при 23 валидных записях.

2. **Agent1 в V5 не видит что сейчас на экране.** Контекстный `<state>`
   блок несёт только `loaded_products` / `loaded_services` / `available_fields`
   / `liked_count` / `cart_count`. V4 имел `current_display: {preset, mode,
   size}` + `displayed_fields`. V5 уронил это сознательно при scene-graph
   миграции, но эквивалент не вернул. Результат: Agent1 не может
   разрешать референсы вроде «у первого добавь рейтинг» или
   «у COSRX-кремА убери цену» без повторного поиска.

Diagnosis blind spot: span `agent1.tool.catalog_search` имел только
`{tool_name}` — пришлось копать `v5_chat_session_state.conversation_history`
чтобы увидеть какие параметры реально передал LLM. Этот gap покрытия
сам по себе тормозил диагностику.

Этот chunk закрывает оба слоя плюс попутно расширяет атрибуты trace
для будущей дебаг-работы. Prefetch frontend wiring и аудит
deterministic-guard regex отложены на следующий chunk (см. ниже).

## Approach

### 1. Hybrid catalog_search (vector + keyword + RRF)

Портирован V4-flow один-в-один, с поправкой на V5 module path и span API:

- **Новый порт** `internal/ports/embedding_port.go` — `Embed(ctx, []string) ([][]float32, error)`. Batch shape, чтобы будущие N-хитовые сценарии (пере-эмбеддинг, multi-query) не требовали редизайна.
- **OpenAI адаптер** `internal/adapters/openai/embedding_client.go` — raw `net/http` (паттерн как у adapters/anthropic). По умолчанию `text-embedding-3-small` 384 dims — совпадает со схемой `master_products.embedding vector(384)`, миграции не нужны.
- **VectorFilter** добавлен в `ports/catalog_port.go`. Структура зеркалит V4 (`Brand`, `CategoryName`, `ProductForm`, `SkinType`, `Concern`, `KeyIngredient`, `TargetArea`, `RoutineStep`, `Texture`).
- **VectorSearch на CatalogAdapter** — отдельный файл `postgres_catalog_vector.go`. JOIN-ы повторяют V4 (`master_products` INNER, `master_variants` LEFT, `categories`, `stock`); `embedding IS NOT NULL` гард обязателен; `ORDER BY mp.embedding <=> $2 LIMIT $N`. Скан использует существующий `scanCatalogProduct` helper — не дублирует логику колонок.
- **catalog_search Execute** переписан в трёх фазах:
  - Phase 1 — параллельно: `EmbeddingPort.Embed` и `ListProducts` через `errgroup.WithContext` (из существующего `golang.org/x/sync`).
  - Phase 2 — `VectorSearch` с тем же `queryEmbedding`, если эмбеддинг успешен. На любой ошибке (квота, network, nil port) — graceful fallback в keyword-only.
  - RRF merge (k=60), keyword weight 1.5× базово, 2.0× если есть структурные фильтры. Stable tie-break по ID для детерминизма.
- **Над-выбор по 2× limit на каждой ветке** перед merge (V4-паттерн): RRF получает богаче пул, тру́нкейтит до `limit` после ранжирования.
- **«Strip brand from search» trick** сохранён.
- **NormalizeProduct** (см. ниже) применяется к merged-списку перед `UpdateData`.
- **Span-meta расширены**: `embed_ms`, `vector_ms`, `sql_ms`, `keyword_count`, `vector_count`, `merged_count`, `search_type` ∈ {`hybrid`,`vector`,`keyword`}, `embed_error`, `vector_error`, `input.{vector_query,brand,category,product_form,skin_type,concern,limit}`. Это закрывает diagnosis blind spot — параметры запроса теперь в трейсе без рейда в `conversation_history`.

### 2. data_normalize

Файл `internal/tools/data_normalize.go` портирует `NormalizeProduct` из V4: trim строк, dedup case-insensitive с сохранением первого порядка для `Tags` / `SkinType` / `Concern` / `KeyIngredients`, фильтр пустых `Images`. V5 не имеет `Service` — `NormalizeService` намеренно не порчен. Вызывается внутри `catalog_search.Execute` после RRF, на финальном merged-списке. Nil-safe.

### 3. Rendered subset в Agent1 контекст

Новая модель `<state>` блока:

```json
{
  "loaded_products": 50,
  "rendered": [
    {"id": "p1", "name": "Snail Cream", "brand": "COSRX", "price": 350000,
     "rating": 4.7, "images": ["https://x/1.jpg"], "marketing_claim": "for dry skin"}
  ]
}
```

Что выкинуто (per Vlad: «туфта»): `liked_count`, `liked_ids`, `cart_count`,
`available_fields`, `loaded_services`. Тестами зафиксировано что они не
ресурфейсятся.

Что добавилось:

- `prompts.RenderedItem` — типизированная shape с `omitempty` на каждом нессентиал-поле, чтобы пустой `marketing_claim` не отъедал токены.
- `prompts.BuildRenderedSubset(products, indices)` — проекция `state.Current.Data.Products[i]` для произвольного списка индексов; out-of-range silently отбрасываются.
- `engine.RenderedDataIndices(*Document)` — извлекает distinct `dataIndex` value на top-level replicate-клонах. Документ без replicate-клонов (text_explainer, error, fresh session) → nil → `rendered` ключ омитится в JSON.
- В `agent1_execute.go:170` (LLM-путь) перед `BuildAgent1ContextPrompt` собираем subset через `buildRenderedSubsetFromState` (round-trip JSON map → engine.Document, та же техника что в `agent2_execute.buildFormationTreeBlock`). Span `agent1.prompt` теперь несёт `rendered_count` + `loaded_products`.

Системный prompt Agent1 расширен одной строчкой в правиле #8: указывает LLM что `rendered` — это товары, которые юзер видит прямо сейчас, и его можно использовать для разрешения референсов («the first one», «the COSRX one», «the one with the snake on the photo»), включая через image URLs.

### 4. Server bootstrap

- `Config.OpenAIAPIKey` (опциональный — пустой не валит boot, только переключает `catalog_search` в keyword-only с warn-логом). Это защита от выкатки на staging без ключа.
- `cmd/server/main.go` инстанциирует `openai.NewEmbeddingClient` если ключ задан, и передаёт в `tools.NewCatalogSearchTool(stateP, catalogP, embeddingP)`. Сигнатура конструктора расширена — третий параметр.

## Files changed

| Action | Path |
|---|---|
| add | `project_v5/backend/internal/ports/embedding_port.go` |
| add | `project_v5/backend/internal/adapters/openai/embedding_client.go` |
| add | `project_v5/backend/internal/adapters/postgres/postgres_catalog_vector.go` |
| add | `project_v5/backend/internal/engine/rendered.go` |
| add | `project_v5/backend/internal/engine/rendered_test.go` |
| add | `project_v5/backend/internal/tools/data_normalize.go` |
| add | `project_v5/backend/internal/tools/data_normalize_test.go` |
| add | `project_v5/backend/internal/prompts/agent1_prompt_test.go` |
| edit | `project_v5/backend/internal/ports/catalog_port.go` (add `VectorFilter`, `VectorSearch` to interface) |
| edit | `project_v5/backend/internal/tools/tool_catalog_search.go` (3-phase hybrid + RRF + trace meta) |
| edit | `project_v5/backend/internal/tools/tool_agent1_test.go` (mock `VectorSearch` + `nil` 3rd arg) |
| edit | `project_v5/backend/internal/usecases/agent1_execute.go` (rendered subset, span attrs, helper) |
| edit | `project_v5/backend/internal/usecases/agent1_execute_test.go` (mock `VectorSearch`, `nil` 3rd arg) |
| edit | `project_v5/backend/internal/prompts/agent1_prompt.go` (new <state> shape + helpers + system-prompt rule #8 line) |
| edit | `project_v5/backend/internal/config/config.go` (add `OpenAIAPIKey` field) |
| edit | `project_v5/backend/cmd/server/main.go` (wire embedding client) |
| edit | `project_v5/backend/go.mod`, `go.sum` (add `pgvector-go v0.3.0`) |

## Verification

### Local

```sh
cd project_v5/backend
go build ./...              # OK
go vet ./...                # OK
go test ./...               # all green; 4 new test files
```

Tests added:

- `prompts/agent1_prompt_test.go` — cold session passthrough, loaded-no-render, full rendered-subset shape, omitempty regression for empty fields, banned-keys regression (liked_count / cart_count / available_fields don't resurface), out-of-range index handling.
- `tools/data_normalize_test.go` — trim+dedup correctness, nil-safe, RRF kw-only, RRF vec-only, kw-outweighs-vec at equal rank, hasFilters topology, limit truncation.
- `engine/rendered_test.go` — nil/empty doc → nil, walker skips literals + reusables + pre-replicate templates, dedup + JSON-float coercion (post-unmarshal), text-explainer-style doc → nil.

### Live HTTP smoke (caller's responsibility — costs ~$0.05/run)

```sh
ANTHROPIC_API_KEY=… OPENAI_API_KEY=… TEST_DATABASE_URL=$DATABASE_URL \
  go test -tags="integration live" -v -count=1 \
  ./internal/handlers/... -run TestHTTPLiveSmoke
```

### Prod smoke after Railway deploy

```sh
scripts/v5-smoke.sh
```

Compare new `docs/v5-smoke/<UTC>/summary.md` против бейзлайна 2026-05-03_20-16:

- p06 «show me toners» → было `empty_not_found` (keyword не cross-language); ожидаем ≥3 продукта через vector path.
- p01 «hi» / p02 «hello there» → НЕ закрывается этим chunk-ом (greeting fallback — отдельная задача A1 в `v5-engine-plan.md`).
- Manual: «Привет, покажи крема» на `hey-babes-cosmetics` должно вернуть ≥3 продукта (23 cream-records в каталоге).
- Manual: после поиска ask «у первого добавь рейтинг» — Agent1 не должен звать `catalog_search`; в `<state>` блоке Agent1 видит `rendered[0].id`.

### Trace attrs sanity

`v5_chat_session_traces.spans` для свежего поиска должен нести
`agent1.tool.catalog_search.attrs` с `input.vector_query`, `keyword_count`,
`vector_count`, `search_type` ∈ {hybrid,vector,keyword}.

## Known gaps / caveats

Не закрыто в этом chunk:

- **Greeting handling (A1)**: «Привет» по-прежнему попадает в `catalog_search` или `empty_not_found`. Hybrid не лечит, нужен отдельный preset-fallback в Agent1 prompt.
- **Modify-vs-rebuild misclassification (A2)**: «другая категория» при непустом state не fires catalog_search. Связано с правилами в Agent1 prompt.
- **Replicate count + pagination (A3)**: V5 hardcodes 3, V4 показывает все + страницы.
- **Back button (A4)**: фронт V5 не имеет UI кнопки back, /navigation/back эндпоинт работает.
- **Skip Agent2 on Agent1 no-op (A5)**: ~1s + $0.001 экономии на турн.
- **Layout density (A6)**: карточки V5 уже V4.
- **Prefetch frontend wiring (chunk 17)**: backend `pipelineResponse.Prefetch` уже шлётся, фронт не подключён — drill-клик идёт через `/navigation/expand` round-trip.
- **Deterministic guard regex audit**: byte-identical с V4 и корректно бейлится на mixed style+filter prompts (тесты в `agent1_execute_test.go` подтверждают). Vlad жаловался на проблемы — скорее всего корень не в regex, а в `_internal_state_filter`-стороне; отдельный фоллоу-ап.
- **Agent1 prompt cache below 2048 floor**: prod-trace всё ещё показывает `cache_creation=0` для Agent1; новая `<state>` секция per-turn dynamic, не помогает кешу. Структурный фикс — слепить tools+system в один блок — отложен.

## Operational notes

- Если развёртывается без `OPENAI_API_KEY` — V5 boot пройдёт, в логе будет
  `warn: OPENAI_API_KEY not set — catalog_search will run keyword-only`.
  В этом режиме «крема» по-прежнему дадут 0; ключ обязателен для прод-эффекта.
- `master_products.embedding` в shared Neon уже заполнен (V4 прод его
  использует), миграции не нужны. Для свежих тенантов потребуется
  seed-скрипт — V4 имеет `cmd/seed_embeddings/`, у V5 пока нет, но это
  отдельный проблем-кадр.
- Cost импакт: одна embedding-вызовка на каждый `catalog_search` вызов.
  text-embedding-3-small @ 384 dims: $0.02 / 1M токенов; типичный
  vector_query ~10 токенов → ~$0.0000002 на запрос — пренебрежимо.
