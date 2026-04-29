# Vertical experts — план перехода и domain mapping

**Дата**: 2026-04-29
**Контекст**: фаза 2 (foundational cleanup). Часть про эксперты.
**Первоисточник**: `/Users/starknight/SelfImproving/agent-experts/README.md` (концепция ACT → LEARN → REUSE, vertical domain experts).
**Аудит до этой работы**: `docs/code_quality_audit_2026-04-27.md` (раздел E про дубликаты, F про доку).

---

## 1. Что не так с текущими 9 горизонтальными экспертами

| Проблема | Подтверждение |
|---|---|
| **Указывают на мёртвые пути** | `backend-adapters/expertise.yaml:7` ссылается на `project/backend/internal/adapters/` — снесено 2026-04-29 |
| **Слишком тонкие** | YAML 98–227 строк vs бенчмарк 600–1000 (первоисточник). Никогда не проходили устойчивый self-improve |
| **Горизонтальные срезы** | Каждый эксперт — слайс одного слоя гексагона через все домены. Чтобы работать над фичей — приходится дёргать 6 экспертов сразу |
| **Self-improve дженерик** | Все 9 файлов `self-improve.md` — 105 строк, идентичны (копия `_templates/self-improve.md`). Нет domain-specific логики обучения |
| **engine-v4 — единственное исключение** | 185 строк на корректных путях `project_v4/backend/internal/engine_v4/`. Скелет есть, нужно расти через self-improve |

---

## 2. Hot-file census (2026-04-29, после сноса легаси)

### Файлы >1k LOC (по правилу Vlad'а — кандидаты на split)

| Файл | LOC | Домен | Split нужен? |
|---|---|---|---|
| `project_v4/.../adapters/postgres/postgres_catalog.go` | 1639 | catalog (V4 read-side) | Maintainability — да; для эксперта — нет (всё внутри catalog) |
| `project_v4/.../engine_v4/ops.go` | 1361 | engine-v4 | Maintainability — спорно; для эксперта — нет (всё engine ops) |
| `project_admin/.../adapters/shopify/client.go` | 1302 | catalog (write-side, источник данных) | Maintainability — да; для эксперта — нет |
| `project_admin/.../adapters/postgres/catalog_adapter.go` | 1293 | catalog (write-side DB) | Maintainability — да; для эксперта — нет |

### Ключевой инсайт

**Ни один из >1k файлов не смешивает домены**. Они все «толстые модули внутри своего домена»:
- `postgres_catalog.go` = весь V4 catalog DB (search + master + digest + embeddings) — всё catalog
- `ops.go` = все engine ops (длинный список) — всё engine-v4
- `shopify/client.go` = весь Shopify API (products, orders, webhooks, OAuth) — всё catalog
- `catalog_adapter.go` = весь admin catalog DB (master writes, listings, categories, proposals) — всё catalog

**→ Splits НЕ блокируют построение экспертов**. Это отдельная maintainability задача, можно отложить в отдельный track.

### Файлы 500–1000 LOC (нормальная толщина)

26 файлов в этом диапазоне (см. census в логах сессии). Никакой действий не требуют.

---

## 3. Финальное domain mapping (6 вертикалей)

### `catalog` (cross-cutting: write в admin + read в V4)
- **admin write-side**: `project_admin/backend/internal/adapters/postgres/catalog_adapter.go` (1293), `master_variants_adapter.go` (603), `categories_v2_adapter.go`, `mapping_artifact_adapter.go`, `candidates_adapter.go`, `merge_apply_tx_adapter.go`, `merge_reports_adapter.go`, `import_adapter.go`, `integrations_adapter.go`, `shopify_staging_adapter.go`, `audit_adapter.go`, `catalog_migrations.go` (726)
- **admin usecases**: `harvester_lite.go` (736), `discovery_tools.go` (726), `discovery_agent.go` (685), `metadata_harvest.go` (566), `enrichment.go` (509), `merge_apply.go` (804), `merge_apply_d3.go`, `match_cascade.go`, `junk_detector.go`, `csv_mapping.go`, `auto_map_tier1.go`, `category_classifier.go`, `validate_artifact.go`, `products.go`, `stock.go`, `import.go`, `integrations.go`, `integrations_wipe.go`
- **admin handlers**: `handler_categories.go`, `handler_products.go`, `handler_curator_merge.go`, `handler_enrichment.go`, `handler_import.go`, `handler_integrations*.go`, `handler_junk.go`, `handler_stock.go`, `handler_api_v1_*.go`
- **admin tools**: `cmd/crawler/main.go` (608), `internal/units/`
- **admin Shopify**: `internal/adapters/shopify/client.go` (1302) — источник данных для каталога
- **V4 read-side**: `project_v4/backend/internal/adapters/postgres/postgres_catalog.go` (1639), `field_definition_adapter.go`, `catalog_migrations.go` (569)
- **V4 usecases**: `catalog_get_product.go`, `catalog_list_products.go`
- **V4 handlers**: `handler_catalog.go`

### `engine-v4` (V4 only, уже хороший базовый скелет)
- **engine_v4 пакет**: весь `project_v4/backend/internal/engine_v4/` — `engine.go`, `ops.go` (1361), `binding.go`, `constraints.go`, `presets*.go`, `expand.go`, `default_ops.go`, `types.go`, `sections.go`, `tree_ids.go`, `tenant_preset_loader.go`
- **domain shapes**: `project_v4/backend/internal/domain/atom_*`, `widget_*`, `formation_*`, `preset_*`, `layout_*`, `display_entity.go`, `category_entity.go`, `product_entity.go` (используемые engine'ом)
- **tools**: `internal/tools/tool_visual_assembly.go` (901)
- **canvas read adapters**: `internal/adapters/postgres/canvas_*_adapter.go` (читают tenant-specific overrides из admin Canvas)

### `pipeline-agents` (V4 chat orchestration: Agent1/Agent2)
- **usecases**: `pipeline_execute.go` (532), `agent1_execute.go`, `agent2_execute.go` (768), `action_execute.go`, `action_view.go`, `chat_send_message.go`, `navigation_back.go`, `navigation_expand.go`, `state_reconstruct.go`, `state_rollback.go`, `template_apply.go`
- **prompts**: `prompt_analyze_query.go`, `prompt_compose_widgets.go`
- **tools (LLM-side)**: `tool_catalog_search.go` (660), `tool_history_lookup.go`, `tool_state_filter.go`, `tool_search_products.go`, `tool_registry.go`, `data_normalize.go`
- **LLM adapters**: `internal/adapters/anthropic/anthropic_client.go` (648), `cache_types.go`, `internal/adapters/openai/embedding_client.go`
- **handlers**: `handler_chat.go`, `handler_pipeline.go`, `handler_navigation.go`, `handler_action.go`, `handler_session.go`, `handler_testbench.go`, `handler_trace.go` (644), `handler_debug.go` (728)
- **state/trace**: `internal/adapters/postgres/postgres_state.go` (620), `postgres_trace.go`, `postgres_events.go`, `postgres_logs.go`, `postgres_cache.go`, `retention.go`
- **domain**: `span.go`, `trace_entity.go`, `message_entity.go`, `session_entity.go`, `event_entity.go`, `tool_entity.go`

### `widget` (chat frontend, project/frontend/)
- **корень**: `WidgetApp.jsx`, `widget.jsx`, `widget.css`, `preview.jsx`, `vite.config.js`, `package.json`
- **features**: `actions/`, `canvas/`, `catalog/`, `chat/`, `navigation/`, `overlay/`
- **entities**: `atom/`, `widget/`, `formation/`, `message/`
- **shared**: `api/`, `theme/`, `hooks/`, `config/`
- **build target**: IIFE bundle `widget.js` (встраивается через `<script>`), Shadow DOM изоляция

### `admin` (project_admin/ кроме каталога, + admin frontend)
- **auth backend**: `internal/usecases/auth*.go` (магик-линк, OAuth, 2FA, sessions, password reset, invitations, telegram, tenants), `internal/adapters/postgres/auth_adapter.go`, `sessions_repo.go`, `challenges_repo.go`, `oauth_login_states_repo.go`, `invitations_repo.go`, `user_tenants_repo.go`, `api_keys_adapter.go`
- **billing**: `internal/usecases/billing.go`, `internal/adapters/postgres/billing_adapter.go`, `handler_billing.go`
- **canvas (KeepstarCanvas write-side)**: `internal/usecases/canvas.go`, `internal/adapters/postgres/canvas_adapter.go` (870), `handler_canvas.go`
- **settings/audit**: `handler_settings.go`, `handler_audit.go`, `audit_adapter.go`
- **auth handlers**: `handler_auth*.go` (10 файлов)
- **integrations infra**: `internal/adapters/google/`, `smtp/`, `resend/`, `telegram/`, `totp/`
- **crypto**: `internal/crypto/`, `internal/crypto/secretbox/`
- **bootstrap**: `cmd/server/main.go` (798)
- **admin frontend**: весь `project_admin/frontend/` (features: canvas, catalog UI, auth screens, settings, billing, members)

### `curator` (curator/ standalone)
- **бэк**: `curator/backend/cmd/server/`, `cmd/seed-curator/`, `internal/adapters/postgres.go` (876), `internal/session/`, `internal/domain/`, `internal/handlers/`
- **фронт**: `curator/frontend/` (SPA + pages + styles)

---

## 4. Spit candidates analysis (deferred)

Splits НЕ блокируют экспертов. Зафиксирую кандидатов на отдельный maintainability track:

| Файл | LOC | Предлагаемая декомпозиция (черновик) |
|---|---|---|
| `project_v4/.../postgres_catalog.go` | 1639 | `postgres_catalog_search.go` (keyword + vector), `postgres_catalog_master.go` (master_products CRUD read), `postgres_catalog_digest.go` (digest gen/cache), `postgres_catalog_embedding.go` (embedding seed/lookup) |
| `project_admin/.../catalog_adapter.go` | 1293 | `catalog_master.go` (master writes), `catalog_listings.go` (product listings), `catalog_categories.go` (categories), `catalog_proposals.go` (curator proposals) |
| `project_admin/.../shopify/client.go` | 1302 | `shopify_products.go`, `shopify_orders.go`, `shopify_webhooks.go`, `shopify_oauth.go`, `shopify_install.go` |
| `project_v4/.../engine_v4/ops.go` | 1361 | оставить как есть — это не разрозненные концерны, а linear ops walker. Split поможет читаемости, но семантически неясен |

**Решение по splitам**: делаются отдельной волной кодовой чистки, не блокируют эксперт-работу.

---

## 5. Порядок построения экспертов

| # | Эксперт | Зачем эта последовательность |
|---|---|---|
| 1 | **engine-v4 refresh** | Существующий 185-стр скелет на корректных путях. Прогон self-improve до стабилизации = тест самого LEARN-цикла на чистом стенде |
| 2 | **widget** | Frontend-only, изолирован, никто не трогает `project/frontend/`. Тест create-from-scratch |
| 3 | **curator** | Маленький полный гексагональный вертикал (домен/порты/адаптеры/юзкейсы/хэндлеры/фронт). Тест полной вертикали |
| 4 | **pipeline-agents** | Средний V4 бэкенд, известная зона. Тест на средней сложности |
| 5 | **catalog** | Самый активный и ценный, но нужны устаканенные паттерны. После фазы 1 |
| 6 | **admin** | Самый большой бэк, сильно связан с catalog. Последним |

Шаблон каждого эксперта (по первоисточнику):
1. `expertise.yaml` — overview / key_files / pipeline / invariants / gotchas / related_experts
2. **Domain-specific** `self-improve.md` — что именно проверять в коде этого домена, какие failure modes отслеживать (НЕ копия `_templates/`)
3. Прогнать self-improve итеративно (3-5 раз) до стабилизации
4. Добавить `question.md`, опционально `plan.md`
5. Валидация реальной задачей в этом домене

---

## 6. Что значит «domain-specific self-improve» в нашем контексте

Главный gap текущих 9 экспертов — все `self-improve.md` идентичны и дженерик. Правильный self-improve содержит:

- **Какие файлы — primary anchors** (читать всегда при синке)
- **Какие сигналы означают «mental model отстал»** (например для `engine-v4`: новый `presets_*.go` файл; новый op type в `types.go`; изменение `ApplyOps` сигнатуры)
- **Какие domain failure modes проверять** (например для `catalog`: дрейф схемы master_products, новые типы tier2 поверх Transform, расхождения migrations)
- **Какие инварианты должны сохраняться после изменений** (например `engine-v4`: formation tree всегда валиден после ApplyOps; replicate всегда explicit)
- **Какие related-experts проверять на drift** (например `catalog` обновление → проверить engine-v4 binding на новых fields)

Эта структура пишется руками для каждого эксперта на основе глубокого знания домена.

---

## 7. Что НЕ делается в этой работе

- Не запускаем self-improve на 9 старых экспертах (мёртвые пути, бессмысленно)
- Не переименовываем старые в `_legacy_` — стираются когда соответствующая вертикаль готова
- Не делаем 6-й вертикал до того как первые 2 прошли валидацию
- Не делаем splits (отдельный track)

---

## 8. Размеры (минуты)

- Stage 0 (этот документ): ~30 мин — DONE
- Stage 2 на эксперта: 30-60 мин (skeleton + 3-5 self-improve итераций + валидация)
- Engine-v4 + widget + curator + pipeline-agents = ~3 часа total
- Catalog и admin отложены до завершения фазы 1

---

## 9. Что прямо сейчас делаю

Stage 2.1 — `engine-v4` refresh:
1. Расширить existing 185-стр скелет (полнее покрыть presets, binding rules, op types)
2. Написать domain-specific `self-improve.md` под engine-v4 specifics
3. Запустить self-improve, посмотреть что добавит
4. Итерировать промт пока выход не стабилизируется

После — отчёт что сработало / что нет, и идём дальше по списку.
