# Catalog spec audit — что построено vs что в спеке

**Дата**: 2026-05-07
**Спека**: `docs/archive/New features/admin_catalog_design_2026-04-23.md`
**Сверка**: live DB (Neon, prod) + Go-код + миграции

> Цель: разложить по полочкам что реально построено по твоей апрельской спеке, что недостроено, что построено криво (наследие февральского PIM redesign'а).

---

## Главный вывод за 30 секунд

**Архитектура спеки реализована частично; критическая разводка между слоями НЕ доделана.**

Спека определяет 3 уровня атрибутов:
- **Tier 1** — универсальные поля на `master_products` (12 колонок)
- **Tier 2** — vertical-specific на `master_cosmetics` / `master_laptops` / ... (FK на `master_variants`, растёт через ALTER TABLE при promotion)
- **Tier 3** — JSONB-bag на `master_products.tier3`

**Что в БД сейчас:**
- `master_products` имеет **36 колонок** вместо 12 (24 лишних — это Tier 1 + Tier 2 косметика свалены в одну таблицу, наследие февральского PIM redesign'а до спеки)
- `master_cosmetics` существует с правильной схемой, **0 строк** (никогда не наполнялась)
- `master_variants` правильная схема, **4 строки** на 991 master_products (пайплайн, который должен наполнять варианты, в основной поток не подключён)
- `master_field_definitions` (реестр promotion'ов) **0 строк** — кнопка Promote в кураторе работает, но никто не нажимал
- 990 листингов из 1024 линкованы через **legacy** `master_product_id`, только 3 — через спецовый `master_variant_id`
- V4-чат читает Tier 2 косметику с **`master_products.skin_type`, `concern`, `key_ingredients`** — то есть с НЕ-спецового места, на спецовое место (`master_cosmetics`) даже не смотрит

**Чтобы привести каталог к спеке нужно сделать пять последовательных вещей** — раздел «Что доделать» в конце.

---

## §3.1 Master tables — schema conformance

### `master_products` (Tier 1)

**Спека требует 12 колонок**: `id, name, brand, description, image_url, additional_images, vertical, embedding, tier3, confidence, owner_tenant_id, created_at, updated_at`.

| Поле спеки | В БД? | Статус |
|---|---|---|
| `id` | ✅ | ok |
| `name` | ✅ | ok |
| `brand` | ✅ | ok |
| `description` | ✅ | ok (но 99.6% пусто на hey-babes) |
| `image_url` | ❌ — в БД `images` (jsonb) | ⚠️ переименовано без апдейта спеки |
| `additional_images` | ❌ | ⚠️ слилось в `images` |
| `vertical` | ✅ | ok |
| `embedding` | ✅ | ok |
| `tier3` | ✅ | ok (0 строк наполнено) |
| `confidence` | ✅ | ok (все DEFAULT 'unverified') |
| `owner_tenant_id` | ✅ | ok |

**Лишние колонки в `master_products` (всего 24, по спеке должны быть в других таблицах)**:

| Колонка | Спека отправляет в | Текущее значение |
|---|---|---|
| `sku` | `master_variants.sku` | дублируется |
| `original_name` | `catalog.products.original_name` | дублируется |
| `product_line` | (нет в спеке) | мусор |
| `product_form` | `master_cosmetics` (Tier 2) | в неправильном месте |
| `texture` | `master_cosmetics` (Tier 2) | в неправильном месте |
| `routine_step` | `master_cosmetics` (Tier 2) | в неправильном месте |
| `routine_time` | `master_cosmetics` (Tier 2) | в неправильном месте |
| `application_method` | `master_cosmetics` (Tier 2) | в неправильном месте |
| `skin_type` | `master_cosmetics.skin_type` | в неправильном месте |
| `concern` | `master_cosmetics.concern` | в неправильном месте |
| `key_ingredients` | `master_cosmetics.ingredients` | в неправильном месте |
| `target_area` | `master_cosmetics` (Tier 2) | в неправильном месте |
| `free_from` | `master_cosmetics` (Tier 2) | в неправильном месте |
| `marketing_claim` | `master_cosmetics` (Tier 2) | в неправильном месте |
| `benefits` | `master_cosmetics` (Tier 2) | в неправильном месте |
| `how_to_use` | `master_cosmetics` (Tier 2) | в неправильном месте |
| `enrichment_version` | (нет в спеке) | технический |
| `volume_ml` | `master_variants.volume_ml` | дублируется |
| `weight_g` | `master_variants.weight_g` | дублируется |
| `unit_count` | `master_variants` (нет явно) | в неправильном месте |
| `source_system` | `catalog.products.source_system` | дублируется |
| `source_id` | `catalog.products.source_id` | дублируется |
| `category_id` | (legacy 1:N — спека требует M:N через `master_product_categories`) | устарело |
| `tier2` | (спека требует Tier 2 в per-vertical таблицах, не jsonb) | конкурирующая модель |

**Итого**: master_products в БД содержит 36 колонок, по спеке должна 12.

### `master_variants` (variant-level)

Схема **точно соответствует спеке** (21 колонка): `id, master_product_id, sku, gtins[], image_url, weight_g, volume_ml, длины, color, size, material, axes, variant_kind, weight_raw, volume_raw, parse_status, embedding, timestamps`.

**Проблема — наполнение**: `master_variants` имеет **4 строки** на 991 master_products. То есть 99.6% мастер-товаров **вообще не имеют вариантов**. Вариант-модель построена в коде и схеме, но дата-пайплайн её не наполняет.

Соответственно: 990 из 1024 листингов hey-babes линкуются через **legacy `master_product_id`** (старая 1:1 модель), только **3** через `master_variant_id` (спецовая модель). 34 листинга unlinked (чистые тенант-only).

### `master_cosmetics` (Tier 2 cosmetics)

Схема соответствует спеке: `master_variant_id (PK FK), skin_type[], concern[], ingredients[], scent, spf`.

**Наполнение: 0 строк**. Никогда не использовалась. Все cosmetic-specific атрибуты приходят на `master_products` напрямую (см. выше).

### `master_laptops`, `master_apparel`, `master_furniture`, etc.

**Не существуют**. Спека упоминает как примеры будущих vertical-таблиц. Их создание зависит от curator promotion (которого пока 0).

### `master_services`

Существует, **0 строк**. Спека сервисов не требует — это легаси.

---

## §3.2 Categories — M:N modelling

| Таблица | Спека | Live | Статус |
|---|---|---|---|
| `master_categories` | M:N parent — общая чистая таксономия | 0 строк | ❌ не наполнена |
| `master_product_categories` (M:N) | M:N junction | 0 строк | ❌ не наполнена |
| `tenant_categories` | копия Shopify-collections тенанта | 7 строк (5 hey-babes, 2 test2) | 🟡 частично |
| `category_mapping` | tenant_category → master_category (N:1) | 0 строк | ❌ не наполнена |
| `tenant_listing_categories` | M:N listing → tenant_categories | 0 строк | ❌ не наполнена |
| `categories` (легаси) | (не в спеке) | 30 строк | ⚠️ legacy 1:N модель всё ещё используется |

**По факту**: категории всё ещё работают на старой модели `master_products.category_id → categories.id` (1:N). Спецовая M:N модель (`master_categories` + два junction'а) построена в схеме, не подключена к ингесту.

---

## §3.3 Listing schema (`catalog.products`)

**Все спецовые колонки добавлены**: `master_variant_id, original_name, display_name, raw_attributes, media, source_system, source_id, payload_hash, deleted_at`. ✅

**Legacy колонки сохранены для обратной совместимости**: `master_product_id, name, description, price, currency, stock_quantity, rating, images, tags, extra`. Это норм по миграционному плану — двойной путь.

**Проблема использования**: 96% листингов используют legacy `master_product_id`, не `master_variant_id`. Render через COALESCE работает (V4 SELECT в `postgres_catalog.go:142-156` правильно делает `LEFT JOIN master_variants → COALESCE(p.master_product_id, mv.master_product_id)`), но **по факту master_variants ничего не даёт** — 99.7% листингов резолвятся через старую ветку.

---

## §3.4 Mapping artifact

`catalog.tenant_catalog_schema` — таблица существует. **1 строка** для hey-babes, status=`needs_human_review`. Discovery-агент бежал, артефакт сохранён, но валидация не прошла → автомат не активен.

---

## §3.5 Candidates + promotion

| Таблица | Спека | Live |
|---|---|---|
| `master_attribute_candidates` | свалка кандидатов (key, vertical, seen_in_tenants, sample_values, ...) | 22 строк (8 furniture + 7 cosmetics + 5 unknown + 2 footwear) — **все pending** |
| `master_category_candidates` | свалка категорий | 8 строк — все pending |
| `master_field_definitions` | реестр promoted-полей | **0 строк** — никто никого не промоутил |

**Promotion код** (curator/backend/internal/adapters/postgres.go:294-319) **существует и работает** (`PromoteAttribute` делает ALTER TABLE в транзакции). Эндпоинт `POST /curator/candidates/attributes/{id}/promote` зарегистрирован.

**Никто никогда не нажимал кнопку Promote**.

---

## §3.6 Junk variants

`tenant_variant_candidates_junk` — таблица существует, **0 строк**. Junk detector в коде есть (`junk_detector.go`), но харвестер пока ничего не клал, потому что master_variants флоу не активен на проде.

---

## §3.7 Audit log

`audit_log` — **4 строки**, все `actor_kind='curator', action='merge'`. Все остальные LogHuman-вызовы (product update, junk classify, api_keys CRUD, categories CRUD) **не пишут**. Возможно ошибка в DI или middleware-обёртке — отдельный баг.

---

## §3.8 Public API + api_keys

`api_keys` — **0 строк**. Public API эндпоинты зарегистрированы в коде (M10), но никто не сгенерил ни одного ключа. Это норм — пока тенант его не использует.

---

## §4 Flows — реализация

### §4.1 First-time Shopify import (10 шагов)

| Шаг | Спека | Реализация |
|---|---|---|
| [1] OAuth | ✅ | работает (test2 prod-verified 2026-04-29) |
| [2] Metadata pull | ✅ | `metadata_harvest.go` |
| [3] Bulk job | ✅ | `harvester_lite.go` |
| [4] Metadata harvest | ✅ | `harvester_lite.go` + signal recording (Phase A4) |
| [5] Auto-mapping Tier 1 | ✅ | `auto_map_tier1.go` |
| [6] Agent normalizer (1 LLM-call) | ✅ | `discovery_agent.go` (Sonnet 4.6, 8 tools) |
| [7] Validation | 🟡 | бежит, но даёт false-negatives (id/createdAt считаются unmapped → 73.7% coverage → status=needs_human_review). Спека требует ≥80% → активный |
| [8] Harvester apply | 🟡 | `merge_apply_d3.ApplyProposals` существует — пишет в master_products + master_variants + products. **НО**: он применяет proposal'ы вручную через куратора, не из mapping_artifact автоматом. Авто-флоу spec.§4.1 [8] не достроен. |
| [9] Embedding job | 🟡 | `cmd/seed_embeddings/` в V4, в новом флоу не интегрирован (свежие master'а не получают embedding до ручного запуска) |
| [10] DONE | ❌ | hey-babes status=needs_human_review с 2026-04-29 |

### §4.2 Re-import (webhook-driven)

| Подшаг | Реализация |
|---|---|
| Hash-diff (skip on payload_hash match) | ✅ — `payload_hash` колонка есть |
| Mapping artifact lookup | 🟡 — артефакт сохраняется, lookup на webhook не подключён |
| Match cascade | ✅ — `match_cascade.go` |
| Webhooks → продолжение пайплайна | 🟡 — webhook handler есть, но через старый legacy importer, не через метадата-флоу |

### §4.3 Mapping artifact format

JSON-формат соответствует спеке (см. `domain/discovery.go::MappingArtifact`). Все ключи из спеки присутствуют (`field_mapping, category_mapping, match_strategy, variant_strategy, agent_notes`).

### §4.4 Render (COALESCE)

V4 SELECT (`postgres_catalog.go:120-185`):
```sql
LEFT JOIN catalog.master_variants mv ON mv.id = p.master_variant_id
LEFT JOIN catalog.master_products mp ON mp.id = COALESCE(p.master_product_id, mv.master_product_id)
```

✅ Двойной путь резолва работает. Спека выполнена.

**НО**: V4 SELECT тащит cosmetic-specific поля из `mp.skin_type, mp.concern, mp.key_ingredients, mp.target_area, mp.marketing_claim, mp.benefits, mp.product_form, mp.texture, mp.routine_step, mp.tier2`. Это **не из master_cosmetics**, как спека требует, а с master_products (где они и не должны были оказаться).

**Когда master_cosmetics наполнят** — V4 SELECT надо будет переписать на JOIN `master_cosmetics ON master_variant_id`, и брать поля оттуда.

### §4.5 Promotion

Код есть и работает. Не использован ни разу. Никаких UI-индикаторов «У вас 22 pending candidates», нет triage-флоу.

### §4.6 Match cascade

`match_cascade.go` реализует все 5 уровней спеки: GTIN → vendor+SKU → vendor+title+axes → embedding → новый master. ✅

### §4.7 Junk variant detection

`junk_detector.go::IsJunkCandidate` реализует сигналы спеки. ✅ (но 0 кандидатов в БД пока)

---

## §6 UI — admin frontend conformance

| Раздел | Спека | Реализация |
|---|---|---|
| Sidebar (Catalog/Import/Widget/Settings/Billing) | ✅ | работает |
| Catalog → All products | ✅ | таблица с фильтрами |
| Catalog → Product detail | 🟡 | master/listing split + sku/size/color chips сделаны (M6); **Tier 3 enrichment block (видео/гифки) — placeholder**, **Performance metrics — placeholder** |
| Catalog → Categories tree | ✅ | edit/move/delete, kind toggle |
| Catalog → Imports | ✅ | jobs list |
| Catalog → Detected add-ons | ✅ | страница есть, badge на sidebar (M9) |
| Settings → API keys | ✅ | generate/copy-once/revoke (M10) |

**Curator service** (отдельный):
- Login + sessions ✅
- ListAttributeCandidates / Promote / Dismiss ✅
- ListCategoryCandidates ✅ (но **нет** PromoteCategory endpoint!)
- Junk classify ✅
- Audit ✅
- Tenants list + merge run/discover (через MergeProxy) ✅
- Master products browse ✅
- Chats (V5 trace inspection) ✅ (chunk 13)

**Чего нет в curator**:
- PromoteCategory (только список, не promotion) — endpoint отсутствует
- Match-reviews (заглушка, требует таблиц которых нет)
- Master-cleanup (заглушка)

---

## §11 Milestones — что реально доделано

Спека помечает 12 milestones. Сверка с git/code:

| M | Описание | Статус по спеке | Реальное состояние |
|---|---|---|---|
| M1 | Schema migrations | ✅ done | подтверждено |
| M2 | Domain types + ports + adapters | ✅ done | подтверждено |
| M3 | Units parser | ✅ done | подтверждено |
| M4a | Foundation — staging + GraphQL bulk | ✅ done | подтверждено |
| M4b | Deterministic harvest + match + junk | ✅ done | подтверждено |
| M4c | Discovery agent | ✅ done | подтверждено |
| **M4d** | **Harvester orchestrator + cut-over legacy** | **🔴 deferred** | **до сих пор не сделан** — это и есть корень всех бед: спецовый pipeline не соединён с продом |
| M6 | COALESCE-render | ✅ done | подтверждено |
| **M7** | **Heybabes 967 backfill** | **🔴 deferred** | до сих пор не сделан — поэтому heybabes на старой схеме |
| M8 | Categories M:N tree | ✅ done | UI работает, **но реальных строк в master_categories — 0** |
| M9 | Detected add-ons UI | ✅ done | UI работает, очередь пуста (нет харвестера) |
| M10 | Public API + api_keys | ✅ done | подтверждено |
| M11 | Curator service | ✅ done | подтверждено |
| M12 | Audit log | ✅ done | code wired, но фактически пишет только curator merge'ы |

**Корень проблемы**: M4d (harvester cut-over) и M7 (hey-babes backfill) **отложены до конца** — и до сих пор не сделаны. Без них спецовый pipeline существует параллельно с легаси, ничего не наполняется по-новому.

---

## Что доделать (в порядке приоритета)

1. **M7 — миграция hey-babes на спец-модель** *(основная боль; всё остальное упирается)*
   - Перенести Tier 2 косметика-поля с `master_products` → `master_cosmetics`. Один INSERT INTO ... SELECT, нужен `master_variant_id` для FK
   - Создать `master_variants` для каждого `master_product` (1 default variant если у hey-babes нет реальных вариантов)
   - Перепривязать `catalog.products.master_variant_id` (вместо `master_product_id`)
   - Снести лишние 24 колонки с `master_products` (ALTER TABLE DROP COLUMN)
   - Перевести V4 SELECT на JOIN `master_cosmetics` для cosmetic полей
   - Аналогично адаптировать V5

2. **M4d — финализация harvester'а** *(чтобы новые тенанты сразу попадали в спецовую схему)*
   - Соединить mapping_artifact lookup с webhook'ом (§4.2 шаг 2)
   - Триггерить `merge_apply_d3.ApplyProposals` автоматически при coverage ≥80%
   - Embedding job на каждый новый master
   - Тест end-to-end на свежем dev-store

3. **Curator promotion flow polish**
   - Добавить эндпоинт `PromoteCategory` (есть только Promote attribute)
   - UI для triage 22 pending attributes — сейчас они в БД, но в curator UI неясно «эти 22 надо смотреть прямо сейчас»
   - Промоутить очевидные косметик-атрибуты (`scent`, `spf` если найдутся, etc.) → `master_cosmetics` через ALTER TABLE
   - Tier 3 enrichment UI в админке (видео/истории/transcript) — спека определяет, реализации нет

4. **Categories M:N — наполнить master**
   - Из 30 категорий в `categories` (legacy) — выбрать чистые → перенести в `master_categories`
   - Подключить `category_mapping` для hey-babes
   - V4/V5 SELECT — переключить с `mp.category_id → categories` на M:N через `master_product_categories → master_categories`

5. **Audit log — починить wiring**
   - 4 строки от curator merge'ов вместо ожидаемых десятков (product update / junk / api keys / categories) — где-то лажа в `LogHuman` вызовах. Найти и починить

---

## Вне аудита, но связанное

- **Tier 3 enrichment** — спека описывает (видео, истории, transcripts, лайфстайл-фото) для богатого поиска. Колонка `master_products.tier3 jsonb` существует, **0 строк**. Discovery-агент на это не нацелен. UI в админке тоже placeholder. Это отдельная фича, не «доделка спеки».
- **Множественные тенанты в master** — спека предусматривает, что один master шарится между тенантами через `master_variant_id`. На hey-babes сейчас все master'а имеют `owner_tenant_id=hey-babes` — то есть нет ни одного шареного master'а с другим тенантом. Это станет видно когда подключится второй cosmetics-тенант.
- **Curator UI / flow** — Vlad сказал «нерабочий» в чате 2026-05-07. Аудит подтверждает: curator МОЖЕТ promote attribute (код работает), но **никаких очевидных триггеров действовать нет**: 22 кандидата сидят с 2026-04-26 без ремайндера. Спецовый flow «куратор раз в неделю заходит, видит counter, делает 5 promote'ов» не сложился, потому что counter ни на что не указывает (badge на sidebar нет в curator-UI).

---

## Что НЕ ломается

- Tier 1 поля (name, brand, description, image) на 95%+ заполнены и работают везде
- Embedding на master_products работает (используется hybrid search в V5 chunk 16)
- Discovery-агент бежит и сохраняет artifact (сам факт)
- Curator standalone сервис работает (login, sessions, list endpoints)
- Public API ключевая инфраструктура есть (просто не использована)
- Audit log хотя бы в части merge-флоу пишет

То есть **продакшн НЕ упадёт**, если ничего не трогать. Просто продукт остаётся в текущем неполном состоянии: косметика держится на легаси-колонках мастера, master_cosmetics стоит пустой, multi-vertical не поддерживается, второй тенант на сноубордах сразу сломает дайджест.
