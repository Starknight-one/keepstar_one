# Admin Catalog — Final Design

**Date:** 2026-04-23
**Status:** ✅ Design closed, ready for implementation planning
**Owner:** Vlad
**Source session:** chat 2026-04-23 (Vlad + Claude), полный Q&A — см. §10 changelog

> Этот документ — единый источник правды по дизайну каталога админки. Если читаешь через месяц — начни с §1 (Mental model), оттуда уже разойдёшься. §3 — DDL и схема БД. §4 — flows. §6 — UI. §9 — что НЕ делаем сейчас и почему.
>
> При имплементации можно расходиться с этим доком, но **расхождение должно быть осознанным** — обнови документ, пометь changelog'ом.

---

## 1. Mental model (читать первым)

### 1.1 Зачем это вообще

**Контент-менеджер тенанта** работает с каталогом — это PIM-минималка + CMS в одном лице. Главное что он делает:
1. Импортирует/синхронит каталог (Shopify primary, CSV, public API, ручной CRUD)
2. Превью каждого товара на пресетах своего чата (как он будет выглядеть в виджете)
3. Прикрепляет богатый контент (видео/гифки/истории) — для прокачки чата
4. Bulk-операции: выбрал N товаров → выбрал поля → применил
5. Управляет деревом своих категорий (иерархия Shopify-collections)

**Чат-движок параллельно** ищет товары не по тенанту, а по **общему master-каталогу** (golden record). Это даёт мелкому тенанту с убогими данными сразу прокачанный поиск (если у соседних тенантов данные жирные).

### 1.2 Master vs Listing — ключевая абстракция

```
┌──────────────────────────────────────────────────────────────────┐
│ MASTER (одна универсальная запись на ВСЕ тенанты)                │
│ • read-only для тенантов, write только curator (Vlad)            │
│ • гиперагрегат всего что натаскали тенанты                       │
│ • используется в поиске и эмбеддингах                            │
└──────────────────────────────────────────────────────────────────┘
                            ▲
                            │ (master_variant_id)
                            │
┌───────────────────────────┴──────────────────────────────────────┐
│ LISTING (один на каждого тенанта, продающего этот товар)         │
│ • цена, сток, переименование, тенант-категории, raw_attributes   │
│ • НИКАКОГО дублирования master-данных                            │
│ • рендер = COALESCE(listing.field, master.field)                 │
└──────────────────────────────────────────────────────────────────┘
```

**Один товар = одна master-запись**, на которую ссылаются 1000 тенантов. Если тенант видит у себя "Vitamin C Serum 30ml" за 1290₽ — у него в `catalog.products` строка с `master_variant_id` и `price=1290`. Картинку, описание, ингредиенты он наследует от мастера через COALESCE.

**Запомни две фразы**:
- "Master — это глобальная общая запись. Тенант — подмножество мастера."
- "Listing хранит ТОЛЬКО то, что у тенанта отличается от мастера."

### 1.3 Variants — Option C (master + master_variants)

В Shopify (и Amazon, и Google Merchant) GTIN/SKU/price/stock/weight/image живут на уровне **варианта**, не продукта. Все крупные PIMы (Akeneo, Salsify, Pimcore, inRiver) используют parent-child модель.

```
master_products (МакБук Pro 14")          ← общие поля 1 раз
  │
  ├─ master_variants (M3/512GB/Silver, gtin=...)
  ├─ master_variants (M3/1TB/Silver,  gtin=...)
  └─ master_variants (M3/512GB/Black,  gtin=...)
```

Listing смотрит на `master_variant_id`, а не `master_product_id`. Через variant восходим до parent для рендера.

### 1.4 3-уровневая модель атрибутов

| Tier | Что | Где хранится | Когда сюда попадает |
|---|---|---|---|
| **Tier 1** | Универсальные поля для ЛЮБОГО товара | Колонки `master_products` / `master_variants` | Зафиксировано в схеме (см. §3). Список финальный, расширения через миграции. |
| **Tier 2** | Категорийно-специфичные типизированные поля (кремы → `volume_ml`, ноутбуки → `cpu`, `ram_gb`) | Per-vertical таблицы: `master_cosmetics`, `master_laptops`... (Option B) | Через **promotion**: куратор видит в свалке кандидатов, что поле повторяется → жмёт Promote → ALTER TABLE добавляет колонку → backfill из свалки |
| **Tier 3** | Свободный enrichment-контент (истории клиентов, видео, лайфстайл-фото, всякая экзотика типа `manufacturer_warranty_months`) | JSONB-bag в `master_products.tier3` + `master_attribute_candidates` (свалка) | Тенанты contribute через mapping artifact или ручной toggle "share to global". Есть атрибуция (`owner_tenant_id`) |

### 1.5 Свалка кандидатов и promotion

`master_attribute_candidates` — стейджинг для атрибутов, которые тенанты притаскивают, но которых ещё нет в типизированной схеме.

```
master_attribute_candidates
─────────────────────────────────────────────
key                  "scent"
vertical             "cosmetics"             ← важно для Option B
seen_in_tenants      47                       ← простой counter
sample_values        [floral, woody, citrus, oud, ...]  (158 уникальных)
proposed_type        enum
status               pending | promoted | dismissed | merged_into:<key>
agent_meta           "запах парфюма/косметики"
embedding            vector(384)             ← включается в master embedding сразу
```

**Сигнал шума**: `seen_in_tenants=1` → скрыт. `>=2` → в дашборде куратора.

**Promotion**:
1. Куратор жмёт Promote на `scent` (vertical=cosmetics)
2. Код делает `ALTER TABLE master_cosmetics ADD COLUMN scent TEXT`
3. Background job переносит значения из listing-ов и из свалки в новую колонку
4. Mapping artifacts тенантов помечаются `stale` → следующий импорт пересоберёт с типизацией
5. Кандидат → `status='promoted'`, не удаляется (audit)

### 1.6 Двухключевой принцип — minimal LLM

**LLM зовётся ОДИН раз на тенанта** (при первом импорте) — производит **mapping artifact** (декларативный JSON: какое Shopify-поле куда мапится). Дальше код применяет artifact к каждому товару, без LLM.

Re-import = lookup по artifact + хэш-диф (не изменилось — скип). LLM перевызывается только при появлении нового поля или явной команде "Re-discover schema".

### 1.7 Curator — отдельный сервис

Куратор (Vlad) — это **отдельный сервис в репо**, отдельная папка, отдельный логин (email+password). Он управляет master'ом: промоутит атрибуты/категории, ревьюит match-конфликты, аппрувит low-confidence master-записи.

Тенанты в master не лезут вообще. `owner_tenant_id` на master — это provenance ("кто первый принёс товар"), не access control.

---

## 2. Бизнес-роли и flows

### 2.1 Акторы

| Роль | Что делает | Где работает |
|---|---|---|
| **Контент-менеджер тенанта** | CRUD listing-полей, импорт, превью, добавление media/stories, bulk edit, public API push | `/admin/*` (admin-frontend, port 5174) |
| **Curator (Vlad)** | Promotion attributes/categories, match review, junk classification, master cleanup | Отдельный сервис: `curator-frontend` + `curator-backend` (новые папки в репо) |
| **Chat-движок** | Поиск по master-каталогу, рендер с COALESCE(listing, master) | `project_v4/backend/` (engine v4) |

### 2.2 Сценарии тенанта

| Сценарий | Поток |
|---|---|
| Первичный onboarding | Нажал "Connect Shopify" → OAuth → metadata pull → bulk job (15-40 min) → mapping artifact → harvester → видит каталог |
| Inventory update | Shopify webhook → hash-diff → если изменилось stock/price — обновляем listing — без LLM |
| Добавил новый продукт в Shopify | Webhook product.create → harvester применяет mapping artifact → match cascade → либо линкуется к существующему master, либо создаёт новый (тенант=owner) |
| Добавил кастомное поле в Shopify | Webhook payload содержит unknown field → попадает в listing.raw_attributes + кандидат с `seen_in_tenants++` |
| Тенант в админке заводит свой атрибут | Visibility toggle: Private (только в его listing) / Contribute (уходит в candidates) |
| Тенант переименовывает продукт у себя | Пишется в `listing.display_name` (или `original_name` если import) — master не трогается |
| Тенант жмёт Export | Получает CSV/JSON только своих данных. Master не отдаём (это будущая платная фича "Boost") |
| Тенант пушит обновления через Public API | API ключ → REST endpoint → тот же harvester pipeline |

### 2.3 Сценарии куратора

| Сценарий | Поток |
|---|---|
| Periodic review | Открывает `curator-frontend` → видит counter "47 attribute candidates", "12 category candidates", "5 match reviews" |
| Promote attribute | Видит `scent` у 47 тенантов → выбирает vertical → жмёт Promote → ALTER TABLE + backfill |
| Match review | Тенант принёс "Vitamin C Serum 30ml", автомат не уверен → куратор видит 3 кандидата с фото → линкует или создаёт новый master |
| Junk classification | Появилась страница `/curator/junk-detected` (показывается только когда есть варианты-аддоны) → куратор просматривает → массово отправляет агенту классифицировать |
| Master cleanup | Видит дубликаты master-записей (один и тот же товар у разных owner'ов) → merge через UI → один остаётся, другие переадресуются |

---

## 3. Схема БД (DDL-sketch)

### 3.1 Master tables

```sql
-- Универсальные поля любого товара
CREATE TABLE catalog.master_products (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name            TEXT NOT NULL,                   -- короткое каноничное (40 симв)
    brand           TEXT,
    description     TEXT,                            -- master-уровень описание
    image_url       TEXT,                            -- главная картинка (parent fallback)
    additional_images TEXT[] DEFAULT '{}',
    vertical        TEXT NOT NULL,                   -- cosmetics | laptops | apparel | ...
    embedding       VECTOR(384),                     -- семантический embedding (включает Tier 3)
    tier3           JSONB DEFAULT '{}'::jsonb,       -- свалка enrichment-полей
    confidence      TEXT DEFAULT 'unverified'        -- unverified | reviewed | approved
                    CHECK (confidence IN ('unverified', 'reviewed', 'approved')),
    owner_tenant_id UUID REFERENCES catalog.tenants(id), -- provenance, НЕ access
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW()
    -- master append-only, deleted_at нет: только curator вручную мержит/дропает
);
CREATE INDEX ON catalog.master_products USING ivfflat (embedding vector_cosine_ops);
CREATE INDEX ON catalog.master_products (vertical);
CREATE INDEX ON catalog.master_products (brand);

-- Варианты: всё что различается между конфигурациями одного товара
CREATE TABLE catalog.master_variants (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    master_product_id  UUID NOT NULL REFERENCES catalog.master_products(id) ON DELETE CASCADE,
    sku                TEXT,                         -- может быть NULL для default variant
    gtins              TEXT[] DEFAULT '{}',          -- массив штрих-кодов (один может иметь несколько)
    image_url          TEXT,                         -- вариант-специфик, fallback на master
    weight_g           INTEGER,
    volume_ml          INTEGER,
    length_mm          INTEGER,
    width_mm           INTEGER,
    height_mm          INTEGER,
    color              TEXT,
    size               TEXT,                         -- "236ml" / "M" / "14 inch" — представление
    material           TEXT,
    axes               JSONB DEFAULT '{}'::jsonb,    -- {size: "236ml", color: "Silver"} — original axes от тенанта
    variant_kind       TEXT NOT NULL DEFAULT 'real'  -- real | addon | bundle
                       CHECK (variant_kind IN ('real', 'addon', 'bundle')),
    -- dual-store для unit fields:
    weight_raw         TEXT,
    volume_raw         TEXT,
    parse_status       JSONB DEFAULT '{}'::jsonb,    -- {volume_ml: "ok", weight_g: "failed"}
    created_at         TIMESTAMPTZ DEFAULT NOW(),
    updated_at         TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX ON catalog.master_variants (master_product_id);
CREATE INDEX ON catalog.master_variants USING gin (gtins);
CREATE INDEX ON catalog.master_variants (variant_kind);
CREATE INDEX ON catalog.master_variants (sku);

-- Per-vertical Tier 2 таблицы (Option B)
CREATE TABLE catalog.master_cosmetics (
    master_variant_id  UUID PRIMARY KEY REFERENCES catalog.master_variants(id) ON DELETE CASCADE,
    skin_type          TEXT[],
    concern            TEXT[],
    ingredients        TEXT[],
    scent              TEXT,
    spf                INTEGER
    -- появляются через promotion
);

CREATE TABLE catalog.master_laptops (
    master_variant_id  UUID PRIMARY KEY REFERENCES catalog.master_variants(id) ON DELETE CASCADE,
    cpu                TEXT,
    ram_gb             INTEGER,
    storage_gb         INTEGER,
    screen_inch        NUMERIC(3,1),
    battery_mah        INTEGER
);
-- ... per новый вертикал, по мере появления
```

### 3.2 Categories (M:N)

```sql
-- Master-таксономия (общая, чистая, ведёт куратор)
CREATE TABLE catalog.master_categories (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    parent_id       UUID REFERENCES catalog.master_categories(id),
    slug            TEXT UNIQUE NOT NULL,
    name            TEXT NOT NULL,
    vertical        TEXT NOT NULL,
    created_at      TIMESTAMPTZ DEFAULT NOW()
);

-- M:N: один master-product может быть в нескольких master-категориях
CREATE TABLE catalog.master_product_categories (
    master_product_id   UUID NOT NULL REFERENCES catalog.master_products(id) ON DELETE CASCADE,
    master_category_id  UUID NOT NULL REFERENCES catalog.master_categories(id) ON DELETE CASCADE,
    PRIMARY KEY (master_product_id, master_category_id)
);

-- Тенант-таксономия (копия их Shopify-collections, их названия)
CREATE TABLE catalog.tenant_categories (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL REFERENCES catalog.tenants(id) ON DELETE CASCADE,
    parent_id       UUID REFERENCES catalog.tenant_categories(id),
    external_id     TEXT,                            -- gid://shopify/Collection/123
    slug            TEXT NOT NULL,
    name            TEXT NOT NULL,
    kind            TEXT DEFAULT 'category'          -- category | showcase (Best Sellers) | promo (On Sale)
                    CHECK (kind IN ('category', 'showcase', 'promo')),
    UNIQUE (tenant_id, external_id)
);

-- Mapping tenant_category -> master_category (N:1, может быть NULL для showcase/promo)
CREATE TABLE catalog.category_mapping (
    tenant_category_id  UUID PRIMARY KEY REFERENCES catalog.tenant_categories(id) ON DELETE CASCADE,
    master_category_id  UUID REFERENCES catalog.master_categories(id) ON DELETE SET NULL,
    confidence          TEXT DEFAULT 'unverified',
    mapped_by           TEXT                          -- 'auto' | 'curator' | 'tenant'
);

-- M:N listing -> tenant_categories
CREATE TABLE catalog.tenant_listing_categories (
    listing_id          UUID NOT NULL REFERENCES catalog.products(id) ON DELETE CASCADE,
    tenant_category_id  UUID NOT NULL REFERENCES catalog.tenant_categories(id) ON DELETE CASCADE,
    PRIMARY KEY (listing_id, tenant_category_id)
);
```

### 3.3 Tenant Listing (катаются под `catalog.products`)

```sql
-- Существующая таблица catalog.products эволюционирует:
ALTER TABLE catalog.products
    ADD COLUMN master_variant_id  UUID REFERENCES catalog.master_variants(id),
    ADD COLUMN original_name      TEXT,              -- как в Shopify (170 симв с маркетингом)
    ADD COLUMN display_name       TEXT,              -- 40 симв, генерится агентом или ручной override
    ADD COLUMN raw_attributes     JSONB DEFAULT '{}'::jsonb,
    ADD COLUMN media              JSONB DEFAULT '[]'::jsonb,  -- private или contributed media
    ADD COLUMN source_system      TEXT,              -- shopify | csv | api | manual
    ADD COLUMN source_id          TEXT,              -- shopify product.id для идемпотентности
    ADD COLUMN payload_hash       TEXT,              -- sha256 от raw payload, для diff-skip
    ADD COLUMN deleted_at         TIMESTAMPTZ;       -- soft delete только на listing уровне
-- Существующее: price, currency, stock_quantity, name, ...
-- name остаётся для legacy и быстрого fallback'а; рендер использует COALESCE(display_name, original_name, master.name)

CREATE INDEX ON catalog.products (master_variant_id);
CREATE INDEX ON catalog.products (tenant_id, source_id);
```

### 3.4 Mapping artifact + schema cache

```sql
CREATE TABLE catalog.tenant_catalog_schema (
    tenant_id           UUID PRIMARY KEY REFERENCES catalog.tenants(id) ON DELETE CASCADE,
    mapping_artifact    JSONB NOT NULL,              -- см. §4.3 формат
    artifact_version    INTEGER NOT NULL DEFAULT 1,
    status              TEXT NOT NULL DEFAULT 'active'
                        CHECK (status IN ('active', 'stale', 'needs_human_review')),
    discovered_at       TIMESTAMPTZ NOT NULL,
    validated_at        TIMESTAMPTZ,
    re_discover_after   TIMESTAMPTZ                  -- max 90 дней страховка
);
```

### 3.5 Candidates (свалки)

```sql
CREATE TABLE catalog.master_attribute_candidates (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    key                 TEXT NOT NULL,
    vertical            TEXT NOT NULL,
    seen_in_tenants     INTEGER NOT NULL DEFAULT 1,
    sample_values       JSONB DEFAULT '[]'::jsonb,
    proposed_type       TEXT,                        -- text | number | enum | bool | array
    agent_meta          TEXT,
    embedding           VECTOR(384),
    status              TEXT NOT NULL DEFAULT 'pending'
                        CHECK (status IN ('pending', 'promoted', 'dismissed', 'merged')),
    merged_into_key     TEXT,
    promoted_to_column  TEXT,                        -- 'master_cosmetics.scent' после promote
    created_at          TIMESTAMPTZ DEFAULT NOW(),
    updated_at          TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (key, vertical)
);

CREATE TABLE catalog.master_category_candidates (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name                TEXT NOT NULL,
    proposed_parent     TEXT,                        -- путь в master-таксономии
    seen_in_tenants     INTEGER NOT NULL DEFAULT 1,
    vertical            TEXT NOT NULL,
    status              TEXT NOT NULL DEFAULT 'pending',
    promoted_to_id      UUID REFERENCES catalog.master_categories(id),
    created_at          TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (name, vertical)
);
```

### 3.6 Junk variants

```sql
CREATE TABLE catalog.tenant_variant_candidates_junk (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id           UUID NOT NULL REFERENCES catalog.tenants(id) ON DELETE CASCADE,
    listing_id          UUID NOT NULL REFERENCES catalog.products(id) ON DELETE CASCADE,
    master_variant_id   UUID REFERENCES catalog.master_variants(id) ON DELETE SET NULL,
    detected_reason     JSONB,                       -- {axis_name: "gift wrap", price_delta: 5, no_gtin: true}
    classification      TEXT DEFAULT 'pending'       -- pending | confirmed_addon | false_positive
                        CHECK (classification IN ('pending', 'confirmed_addon', 'false_positive')),
    classified_at       TIMESTAMPTZ,
    classified_by       TEXT,
    created_at          TIMESTAMPTZ DEFAULT NOW()
);
```

### 3.7 Audit log

```sql
CREATE TABLE catalog.audit_log (
    id              BIGSERIAL PRIMARY KEY,
    tenant_id       UUID,                            -- NULL для системных
    actor_kind      TEXT NOT NULL,                   -- user | system | agent | curator | api
    actor_id        TEXT,                            -- user.id / 'system' / 'agent:harvester' / etc
    entity_kind     TEXT NOT NULL,                   -- listing | master_product | master_variant | category | candidate
    entity_id       UUID NOT NULL,
    action          TEXT NOT NULL,                   -- create | update | delete | rename | promote | merge
    field_changes   JSONB,                           -- {field: [old, new]} для update
    aggregate_meta  JSONB,                           -- {batch_id, count, source} для system bulk
    created_at      TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX ON catalog.audit_log (entity_kind, entity_id, created_at DESC);
CREATE INDEX ON catalog.audit_log (tenant_id, created_at DESC);
```

**Правило записи**:
- **System bulk** (импорт обновил 7234 цены) → ОДНА запись с `aggregate_meta={batch_id, count: 7234, source: 'shopify_webhook'}`
- **Human action** (тенант переименовал товар) → детальная запись per-field с `field_changes={display_name: ["X", "Y"]}`

### 3.8 Public API

```sql
CREATE TABLE catalog.api_keys (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL REFERENCES catalog.tenants(id) ON DELETE CASCADE,
    key_hash        TEXT NOT NULL,                   -- bcrypt hash, не plain
    label           TEXT,                            -- "production webhooks", "staging"
    last_used_at    TIMESTAMPTZ,
    revoked_at      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX ON catalog.api_keys (tenant_id);
```

---

## 4. Ключевые flows

### 4.1 First-time Shopify import (с metadata-first)

```
[1] Тенант жмёт "Connect Shopify"
    OAuth → tenant_shop_id, access_token сохраняем
    │
    ▼
[2] METADATA PULL (sync, ~5 GraphQL calls, < 30 сек)
    • metafieldDefinitions(ownerType: PRODUCT)
    • metafieldDefinitions(ownerType: PRODUCTVARIANT)
    • metafieldDefinitions(ownerType: COLLECTION)
    • menu(handle: "main-menu")  ← дерево навигации
    • shop.{productVendors, productTypes, productTags}
    │
    ▼
[3] BULK JOB (async, 15-40 min для 100k товаров)
    bulkOperationRunQuery → весь каталог JSONL
    Тенант видит progress bar + параллельные onboarding-таски
    (заполнить профиль, настроить виджет, инвайтить команду)
    │
    ▼
[4] METADATA HARVEST (чистый код, без LLM)
    Анализ JSONL по полям:
      • frequency (сколько раз поле встречается)
      • inferred type
      • value distribution (top-10 значений или min/max для num)
      • empty_rate
      • language (en/other)
    + Анализ collections:
      • строим дерево из nav menu (primary)
      • fallback: handle prefix
      • fallback: parent metafield
    + metafield discovery:
      • declared (из metafieldDefinitions)
      • undeclared (sampled из JSONL)
    │
    ▼
[5] AUTO-MAPPING TIER 1 (чистый код)
    Очевидные совпадения:
      "Vendor"|"Brand"|"Manufacturer" → master.brand
      "Weight" + numeric + unit → master_variants.weight_g (через unit parser)
      GTIN/UPC/EAN/Barcode → master_variants.gtins
      collection.title → tenant_categories.name
    Что не совпало → пакуем в meta-report для агента.
    │
    ▼
[6] AGENT NORMALIZER (ОДИН LLM-вызов на тенанта)
    Input: meta-report (~1-2KB JSON) + Tier-1 preview mapping
    Бюджет: max 5 tool-calls для сэмплов реальных товаров
    Если бюджет исчерпан → status='needs_human_review' → curator-дашборд
    Output: mapping_artifact (формат в §4.3)
    │
    ▼
[7] VALIDATION (чистый код)
    Прогон mapping на 10-20 батчевых товарах:
      • coverage % (сколько полей замапилось)
      • parser failures
    Если ≥80% coverage и нет fatal-ошибок → status='active'
    Иначе → 'needs_human_review'
    │
    ▼
[8] HARVESTER (чистый код, batch over JSONL)
    Per товар:
      • match cascade: GTIN exact → vendor+SKU → vendor+title+axes → embedding → новый master
      • если матч → создаём listing с master_variant_id
      • если новый → создаём master_product + variants, тенант=owner, confidence='unverified'
      • не замапленные поля → listing.raw_attributes
      • кастомные поля с `seen_in_tenants>=2` per vertical → candidates
      • категории → tenant_categories + category_mapping
      • junk-эвристика для variants → tenant_variant_candidates_junk
    │
    ▼
[9] EMBEDDING JOB (async)
    Per новый master_product:
      embedding = embed(name + brand + description + ingredients + tier3_text + candidates_text)
    │
    ▼
[10] DONE
     Тенант видит свой каталог 1:1 как в Shopify
     + toggle "Master view" → видит нашу таксономию
```

### 4.2 Re-import (webhook-driven, LLM почти не вызывается)

```
[1] Shopify webhook (product.update / inventory.update / product.create)
    │
    ▼
[2] HASH-DIFF
    sha256(payload) сравниваем с listing.payload_hash
    Совпал → skip полностью.
    │
    ▼
[3] MAPPING ARTIFACT LOOKUP
    tenant_catalog_schema.mapping_artifact уже есть.
    Применяем lookup → master + listing fields. Без LLM.
    │
    ▼
[4] ВЕТКИ:
    a) listing.master_variant_id != NULL → update listing fields, master трогает только curator
    b) listing.master_variant_id == NULL → match cascade → новый master или линк
    c) Прилетело новое поле, которого нет в artifact:
        • кладём в listing.raw_attributes
        • создаём/инкрементим candidate
        • НЕ дёргаем агента сразу (накопится — куратор увидит)
    │
    ▼
[5] AUDIT LOG
    System action → одна запись с aggregate_meta для batch
```

**Полный re-discover schema** триггерится только:
- Раз в 90 дней (страховка)
- Тенант явно жмёт "Re-discover schema" (например после большой реструктуризации каталога)

### 4.3 Mapping artifact — формат

```json
{
  "version": 1,
  "validated_at": "2026-04-23T14:00:00Z",
  "status": "active",
  "field_mapping": {
    "Vendor": { "target": "master.brand", "transform": null },
    "Volume": {
      "target": "master_variants.volume_ml",
      "transform": "ml_from_string",
      "default_unit": "ml"
    },
    "Hair Type": {
      "target": "candidate:hair_type",
      "vertical": "cosmetics",
      "type": "enum",
      "samples": ["straight", "curly", "wavy", "coily"]
    },
    "Internal SKU": { "target": "listing.raw_attributes.internal_sku" },
    "Marketing Title": {
      "target": "listing.original_name",
      "shorten_to": "listing.display_name",
      "shorten_max": 40
    }
  },
  "category_mapping": {
    "gid://shopify/Collection/123": {
      "target": "master_category:cleansing",
      "tenant_label": "Cleansers & Toners",
      "kind": "category"
    },
    "gid://shopify/Collection/456": {
      "target": null,
      "tenant_label": "Best Sellers",
      "kind": "showcase"
    }
  },
  "match_strategy": ["gtin", "vendor+sku", "vendor+title+axes", "embedding"],
  "variant_strategy": "master_with_variants",
  "agent_notes": "Vendor=brand 1:1 чисто. Hair Type — новый кандидат, 4 значения, рекомендую как enum."
}
```

### 4.4 Render (любой каталожный API)

```sql
-- ?view=tenant (default)
SELECT
  l.id AS listing_id,
  COALESCE(l.display_name, l.original_name, mp.name) AS name,
  COALESCE(l.original_name, mp.name) AS full_name,
  COALESCE(NULLIF(l.media->>'images','[]'), mv.image_url, mp.image_url) AS image,
  l.price, l.currency, l.stock_quantity,
  mp.brand,
  mv.gtins, mv.sku, mv.size,
  l.raw_attributes
FROM catalog.products l
JOIN catalog.master_variants mv ON mv.id = l.master_variant_id
JOIN catalog.master_products mp ON mp.id = mv.master_product_id
WHERE l.tenant_id = $1
  AND l.deleted_at IS NULL;

-- ?view=master  (curator или для дебага)
SELECT mp.*, mv.* FROM catalog.master_products mp ...
```

### 4.5 Promotion (curator action)

```
[1] Curator открывает /curator/candidates
    Список candidate'ов с seen_in_tenants >= 2, sorted by count DESC
    │
    ▼
[2] Жмёт "Promote scent → master_cosmetics"
    Подтверждение типа (TEXT / TEXT[] / INTEGER / NUMERIC)
    │
    ▼
[3] МИГРАЦИЯ (transactional)
    BEGIN;
    ALTER TABLE catalog.master_cosmetics ADD COLUMN scent TEXT;
    UPDATE catalog.master_attribute_candidates
       SET status='promoted',
           promoted_to_column='master_cosmetics.scent',
           updated_at=NOW()
       WHERE key='scent' AND vertical='cosmetics';
    COMMIT;
    │
    ▼
[4] BACKFILL JOB (background)
    Для каждого master_product вертикали cosmetics:
      • Проверить если у listing-ов с этим master есть raw_attributes->'scent' или 'fragrance' (из дедупа)
      • Записать в master_cosmetics.scent
      • Обновить embedding (новый Tier 2 факт стал типизированным)
    Перейти по всем active mapping_artifact, найти где target='candidate:scent':
      • Заменить target='master_variants.scent_via_master_cosmetics'
      • status='stale' → следующий import пересчитает с типизированной target
    │
    ▼
[5] AUDIT
    INSERT в audit_log: actor_kind='curator', action='promote',
    entity_kind='candidate', aggregate_meta={migration: '...', backfilled_count: N}
```

### 4.6 Match cascade при новом variant

```
function matchVariant(payload, tenantId):
  // 1. GTIN exact (highest confidence)
  if payload.barcode != null:
    found = SELECT * FROM master_variants WHERE $1 = ANY(gtins)
    if found.count == 1: return link(found, confidence='gtin_exact')
    if found.count > 1:  return reviewQueue('gtin_collision', found)

  // 2. Vendor + SKU exact (if both present)
  if payload.vendor != null AND payload.sku != null:
    found = SELECT mv.* FROM master_variants mv
              JOIN master_products mp ON mp.id = mv.master_product_id
              WHERE mp.brand ilike $1 AND mv.sku = $2
    if found.count == 1: return link(found, confidence='sku_exact')

  // 3. Vendor + normalized_title + axes match (fuzzy)
  candidates = SELECT mv.*, similarity(mp.name, $title) AS sim
                FROM master_variants mv ...
                WHERE mp.brand ilike $vendor
                ORDER BY sim DESC LIMIT 5
  if candidates[0].sim > 0.85 AND axesMatch(candidates[0].axes, payload.options):
    return link(candidates[0], confidence='fuzzy_high')

  // 4. Embedding similarity
  pseudo_embedding = embed(payload.title + payload.vendor + payload.description)
  nearest = vectorSearch(pseudo_embedding, threshold=0.92)
  if nearest: return reviewQueue('embedding_match', nearest)

  // 5. Новый master, тенант=owner
  return createNewMaster(payload, ownerTenantId=tenantId, confidence='unverified')
```

### 4.7 Junk variant detection

```
function isJunkCandidate(variant):
  signals = []
  if variant.axis_name matches /(gift wrap|engraving|warranty|insurance|add.?on|service|protection plan)/i:
    signals.append('axis_name_pattern')
  if variant.gtins.length == 0 AND variant.sku == null:
    signals.append('no_identifiers')
  if variant.weight_g == null AND variant.volume_ml == null:
    signals.append('no_dimensions')
  if abs(variant.price - parent_min_variant.price) < 10 AND variant.price < 50:
    signals.append('small_price_delta')
  return signals.length >= 2
```

Обнаруженные складываются в `tenant_variant_candidates_junk`. Страница `/admin/catalog/detected-addons` появляется только когда есть `pending` записи.

---

## 5. Юниты (детально)

### 5.1 Где что лежит

**В коде** (`internal/units/`):
- Канонические единицы: `mL`, `g`, `mm`, `pcs` (UCUM-совместимые)
- Конверсии: `liter → 1000 ml`, `kg → 1000 g`, `inch → 25.4 mm` (с округлением до integer для длин/масс/объёмов)
- Static, версионируется с бинарём, unit-tested
- **Конверсии в БД не пишем** — ошибка в строке БД молча портит весь каталог

**В БД** (`catalog.unit_aliases`):
```sql
CREATE TABLE catalog.unit_aliases (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID REFERENCES catalog.tenants(id) ON DELETE CASCADE,  -- NULL = global
    raw_token       TEXT NOT NULL,                  -- "мл", "MILLILITERS", "ml.", "fl oz"
    canonical_unit  TEXT NOT NULL,                  -- "mL"
    confidence      TEXT DEFAULT 'auto',
    source          TEXT,                            -- 'seed' | 'agent' | 'curator'
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (tenant_id, raw_token)
);
-- seed: мл, ml, milliliter, milliliters, MILLILITERS, ml. → mL
-- seed: г, g, gram, grams, GRAMS, гр → g
```

### 5.2 Парсер (state machine)

```
INPUT: "2x30ml"
  │
  ▼
TOKENIZE
  ['2', 'x', '30', 'ml']
  │
  ▼
ALIAS LOOKUP
  'ml' → canonical 'mL' (global alias)
  │
  ▼
PATTERN CLASSIFY
  <num>x<num><unit> → MULTI_PACK
  │
  ▼
PARSE
  unit_count=2, value=30, unit='mL'
  │
  ▼
STORE
  volume_ml=30, unit_count=2 (НЕ volume_ml=60 — semantics для LLM-рендера "2 × 30 ml")
  volume_raw="2x30ml"
  parse_status="ok"
```

### 5.3 Edge cases

| Input | Поведение | parse_status |
|---|---|---|
| `30 ml` | volume_ml=30 | ok |
| `2x30ml` | volume_ml=30, unit_count=2 | ok |
| `30ml/100ml` (dual label) | volume_ml=30 (primary first), validate с tolerance 2% | dual_mismatch если рассходится |
| `60` (bare number) | volume_ml=NULL если в artifact нет `default_unit` | ambiguous |
| `60` + artifact.default_unit=ml | volume_ml=60 | ok |
| `large` | volume_ml=NULL | failed (только raw сохраняем) |

**Никогда не угадываем единицу из категории.** Тихие неверные данные хуже отсутствующих.

**Multi-pack семантика**: `2x30ml` ≠ `60ml`. Чат покажет "пачка 2×30 мл", а не "60 мл".

---

## 6. UI — что делаем в админке

### 6.1 Sidebar (admin-frontend)

```
- Chats
- Catalog                       ← главная фокус-зона
    - All products              (таблица с фильтрами)
    - Categories                (дерево + редактор)
    - Imports                   (jobs, status, re-discover)
    - Detected add-ons          (показывается только если есть pending)
- Import                        (Shopify connect, CSV upload, public API keys)
- Widget
- Canvas
- Settings
- Billing
```

### 6.2 Catalog → All products

| Колонка | Источник |
|---|---|
| Image | COALESCE(listing.media[0], variant.image, master.image) |
| Name | COALESCE(display_name, original_name, master.name) |
| Brand | master.brand |
| SKU | master_variant.sku |
| Price | listing.price |
| Stock | listing.stock_quantity |
| Status | derived: Active / Low stock / Out / Archived |
| Categories | tenant_categories.name array |

Filters: category tree (sidebar), brand, price range, stock status, search.
Sort: name, price, stock, updated_at.
Bulk actions: bulk edit (price, stock, display_name, archive/restore — listing-only).

### 6.3 Catalog → Product detail

```
┌────────────────────────────────────────────────────────────────┐
│ Breadcrumb: All products / Cleansing / Hydrating Cleanser      │
│                                       [ Preview in chat ▾ ]    │
├────────────────────────────────────────────────────────────────┤
│ LEFT (2/3)                       │ RIGHT (1/3)                 │
│                                  │                             │
│ Hero                             │ Variants                    │
│ ─ image                          │  • 236ml (sku: ...)         │
│ ─ display_name (editable)        │  • 473ml (sku: ...)         │
│ ─ original_name (small, view)    │ + Add variant               │
│                                  │                             │
│ ───────────────────────          │ Performance                 │
│ Master fields (read-only)        │  views, ctr, conversion     │
│  brand, ingredients, ...         │  (chat-engine analytics)    │
│  [view in master view]           │                             │
│                                  │ Quick actions               │
│ Listing overrides                │  [Preview]                  │
│  display_name: ...               │  [Duplicate listing]        │
│  price: ...                      │  [Archive]                  │
│  stock: ...                      │  [Delete]                   │
│  raw_attributes (kv editor)      │                             │
│                                  │                             │
│ Tier 3 enrichment                │                             │
│  ┌───┐ ┌───┐ ┌───┐               │                             │
│  │vid│ │gif│ │img│ + Add         │                             │
│  └───┘ └───┘ └───┘               │                             │
│  Stories: ...                    │                             │
│  Visibility: ◉ Private           │                             │
│              ○ Contribute        │                             │
└──────────────────────────────────┴─────────────────────────────┘
```

**Preview in chat**: модалка, дёргает V4 engine с `master_variant_id` + выбранным пресетом. Возвращает Formation JSON → рендерит виджет в модалке.

### 6.4 Catalog → Categories

Tree editor (как в обычном PIM). Тенант редактирует ИХ дерево. Mapping в master-категории — read-only для тенанта (показывает кто куда смаплен).

### 6.5 Catalog → Imports

Список jobs: type (initial / webhook / public_api), status (running / done / failed), counts, started_at. Кнопка "Re-discover schema" (тяжёлая, с подтверждением).

### 6.6 Catalog → Detected add-ons

Список junk candidates с master_variant ref. Кнопки "Mark as add-on" / "Mark as real" / "Send batch to agent for classification".

### 6.7 Import → Public API

В разделе Settings подраздел "API access":
- Список API ключей (label, last_used, revoke)
- "Generate new key" (показывает plain-text один раз)
- Документация ссылка → `/docs/api`

### 6.8 Curator service (отдельный сервис)

Папка в репо: `curator/backend/`, `curator/frontend/`. Email+password логин (отдельная таблица `curator.users`). Доступ к master tables через service-account, тенантский middleware не применяется.

Главные страницы:
- `/curator/candidates` — attribute promotion
- `/curator/categories` — category promotion
- `/curator/match-reviews` — manual линковка спорных вариантов
- `/curator/junk-classification` — массовая классификация junk
- `/curator/master-cleanup` — поиск дубликатов master, merge UI
- `/curator/audit` — read-only лог системных и кураторских действий

---

## 7. Public API (REST, MVP)

```
Auth: Header `X-API-Key: <plain key>`. Rate limit per tenant per tariff.

GET    /api/v1/products?cursor=&limit=50&updated_since=
POST   /api/v1/products                      # bulk push, до 100 за вызов
GET    /api/v1/products/{id}
PATCH  /api/v1/products/{id}
DELETE /api/v1/products/{id}                  # soft delete listing

GET    /api/v1/categories
POST   /api/v1/categories
PATCH  /api/v1/categories/{id}
DELETE /api/v1/categories/{id}

POST   /api/v1/imports                        # start bulk import job
GET    /api/v1/imports/{id}                   # job status

POST   /api/v1/webhooks                       # subscribe to listing/master events
```

Тело — наш upstream JSON (не Shopify dialect). Из Shopify — через harvester, из API — напрямую в тот же pipeline.

---

## 8. Embedding strategy (для variants)

**Решение по итогам ресёрча**: parent embedding для семантики + axis-keyword index для запросов типа «473ml ceramide cleanser».

Перед коммитом — A/B тест:
- Базлайн: только parent embedding
- Кандидат: parent + variant embeddings (×N стоимость)
- 20 запросов микс из реальных чатов
- Метрика: recall@5 для variant-specific запросов

Если разрыв небольшой (<10%) — берём parent-only. Если велик — per-variant embedding для critical variants (с GTIN, с уникальным image).

---

## 9. Что НЕ делаем сейчас и почему

| Что | Почему отложено |
|---|---|
| **Multi-language / Markets** | MVP English-only. translatableResources Shopify не подсасываем, локализованные metafields игнорируем. Когда тенанты потребуют — отдельный модуль. |
| **Inventory per location** | Архитектурно правильно `(tenant, variant, location, qty)`, но это отдельный кусок (цены/стоки). Сейчас плоский `stock_quantity`. |
| **Свой CDN для медиа** | Используем Shopify CDN URLs. Загрузку через нашу админку (R2 bucket) — когда дойдём до видео/гифок. |
| **Tier 2 для ноутбуков/одежды** | Вертикалей пока 1 (cosmetics). При выходе во 2-й — создаём `master_laptops` table, стартует промоушен per-vertical. |
| **Markup edit для master** | Только curator. Тенант никогда не редактирует master напрямую (только через contribute candidates). |
| **Master export для тенантов** | Будущая платная фича "Boost your catalog with master data". Не в MVP. |
| **GraphQL для public API** | REST для MVP. GraphQL когда у нас будут тенанты с серьёзной интеграцией. |
| **Auto-detection of schema drift** | Прилетает новое — перезаписываем + audit log. Не пытаемся magic-detect rename'ы. |
| **Webhook-OUT на изменения мастера** | Curator update master → тенант не уведомляется push'ем. Видит при следующем чате через COALESCE. |
| **Per-field master ACL для тенантов-owner'ов** | Owner == provenance, не write-rights. Все правки master — через curator. |

---

## 10. Чек-лист перед стартом имплементации

### Что должно быть готово до первой строки кода

- [x] Mental model согласована (§1)
- [x] Master / Variants schema (§3.1)
- [x] Listing schema (§3.3)
- [x] Categories M:N (§3.2)
- [x] Mapping artifact format (§4.3)
- [x] Candidates / promotion (§3.5, §4.5)
- [x] Junk variants (§3.6, §4.7)
- [x] Audit log policy (§3.7)
- [x] Units strategy (§5)
- [x] Match cascade (§4.6)
- [x] UI скелет (§6)
- [x] Public API endpoints (§7)
- [x] **Roadmap по milestone'ам** → `docs/New features/admin_catalog_implementation_plan_2026-04-26.md` (12 milestones M1-M12)
- [x] **Migration order** — закрыто M1, см. `catalog_migrations.go` + `docs/Updates/main-admin-catalog-m1-m3-plus-shopify-oauth_2026-04-26_10-28.md`

### Что должно быть проверено в первой неделе имплементации

- [ ] Match cascade на реальных данных heybabes (967 продуктов): какой % уходит в auto, какой в review queue — **закрывается в M5+M7**
- [ ] Embedding A/B test (parent-only vs parent+variant) — **deferred until после M7**
- [x] **Unit parser** — 40 subtests pass, English-only (M3, commit `71d5d7c`)
- [ ] Bulk job перформанс на полном каталоге одного тенанта — **закрывается в M4**
- [ ] COALESCE рендер на 1000 listings — нет ли N+1, нужны ли materialized views — **закрывается в M6**

### Прогресс по milestone-плану (обновлено 2026-04-26 13:13)

| M | Описание | Status | Commit | Лог |
|---|---|---|---|---|
| M1 | Schema migrations (additive) | ✅ done | `ad2386a` | `docs/Updates/main-admin-catalog-m1-m3-plus-shopify-oauth_2026-04-26_10-28.md` |
| M2 | Domain types + ports + adapters | ✅ done | `8065fcc` | (same) |
| M3 | Units parser | ✅ done | `71d5d7c` | (same) |
| M4a | Foundation — staging + GraphQL bulk client | ✅ done | `6067fb7` | `docs/Updates/main-admin-catalog-m4abc-discovery-tested_2026-04-26_13-13.md` |
| M4b | Deterministic harvest + match cascade + junk detector | ✅ done | `d537518` | (same) |
| M4c | Discovery agent (Sonnet 4.6, 8 tools) + validation | ✅ done | `8cd0b2f` (+ fixes `10e1145`, `5a795ac`) | (same) |
| **M4d** | **Harvester orchestrator + cut-over (legacy delete)** | **🔴 deferred to end** | — | (planned during final M4 polish session) |
| M6 | COALESCE-render admin + V4 engine | ⏳ next | — | — |
| M7 | Heybabes 967 backfill script | pending | — | — |
| M8 | Categories M:N + tree editor | pending | — | — |
| M9 | Detected add-ons page (junk triage UI) | pending | — | — |
| M10 | Public API + api_keys management | pending | — | — |
| M11 | Curator service (standalone) + audit + promotion | pending | — | — |

**Plan correction (2026-04-26 13:10 UTC).** Discovery agent работает end-to-end на dev-store (51s / 13 turns / commit_artifact, см. лог). M4d (harvester + cut-over legacy) **отложен в самый конец** — чтобы при финальном слиянии пользователь сел и сосредоточенно прокинул как pipeline должен работать end-to-end. Сейчас идём по M6→M7→M8→M9→M10→M11; M4d полируется последним. Detail rationale + что осталось от M4 — в логе сессии.

Бонусом этой сессии (не в M-плане): проdovskoy Shopify OAuth полностью заработал на dev-store `keepstar-neaqpan1.myshopify.com` (коммиты `c99589f` + `d87a676` + Railway env config). 17 продуктов синкнулись в **старую схему** через legacy importer — будут backfill'нуты в M4 или отдельным шагом.

---

## 11. Связь с известными пробелами текущего admin

Из логов прошлых сессий (`docs/Updates/`):

| Пробел | Закроется секцией |
|---|---|
| `ProductDetailPage` форма не редактируется по-настоящему | §6.3 |
| `Variants` placeholder | §3.1 + §6.3 |
| `Performance metrics` placeholder | (отдельная аналитика, не сюда) |
| `Additional information` (соцссылки/галерея/stories/reviews) disabled | §6.3 Tier 3 enrichment |
| `currency` дефолт `RUB` | Тривиально, фикс при миграции под мульти-валюту |
| `Export` визуал-only | §9 (только tenant data, формат CSV/JSON), реализация после core |
| Управление деревом категорий через UI | §6.4 |
| `Detected add-ons` страница | §6.6 + §3.6 |

---

## 12. Что НЕ входит в этот документ (явно)

- Канвас (отдельная фича, отдельный план)
- Engine V4/V5 — это про рендеринг чата, не про каталог
- Аналитика тенанта (sales, conversion) — отдельный модуль
- Биллинг/тарифы (уже сделано отдельно, см. `docs/Updates/main-billing-page_*.md`)
- Auth/multi-tenant механика (уже сделано, см. `docs/Updates/feature-admin-auth-screens_*.md`)
- Curator service детальный API/UI (этот документ покрывает только что куратор делает на верхнем уровне)

---

## Changelog

- **2026-04-26 13:13 UTC — M4 a/b/c shipped, discovery agent verified end-to-end, M4d deferred**
  - 4a foundation (`6067fb7`), 4b deterministic (`d537518`), 4c discovery agent (`8cd0b2f`) merged to main
  - Two production fixes: `10e1145` (Shopify 2026-04 schema for variant weight) + `5a795ac` (prompt caching, partial salvage, drop OpenAI dep)
  - **End-to-end test on dev-store passed**: `dump-to-staging` (4.3s, 17 products) + `discover` (51s, 13 turns, status=committed, 13 field mappings + 3 categories + 1 master_template `winter_sports`, ~$0.15 cost). Validation gave 73.7% coverage → `needs_human_review` (false negative caused by system fields like `id`/`createdAt` counted as unmapped — fix is in the deferred polish list).
  - **Plan correction**: M4d (harvester orchestrator + cut-over legacy) deferred to the very end. Reason: discovery works, but the user needs to sit and walk through the agent's transcript + validation report at the cut-over point. Going M6→M7→M8→M9→M10→M11 first.
  - Session log: `docs/Updates/main-admin-catalog-m4abc-discovery-tested_2026-04-26_13-13.md`

- **2026-04-26 — implementation kickoff (M1-M3 done)**
  - §10 чек-лист обновлён: добавлен прогресс по milestone-плану, отмечены закрытые задачи
  - Сам план — `docs/New features/admin_catalog_implementation_plan_2026-04-26.md`
  - Лог сессии — `docs/Updates/main-admin-catalog-m1-m3-plus-shopify-oauth_2026-04-26_10-28.md`
  - **Решения по 3 развилкам** (через AskUserQuestion при старте плана):
    - Cosmetics PIM-колонки → **извлекаем в master_cosmetics** (Tier 2 extract)
    - Heybabes 967 + dev-store 17 → **wipe dev-store + resync, backfill heybabes**
    - Curator service → **standalone** (отдельная папка curator/, отдельный логин)
  - В сессии также проdovskoy Shopify OAuth заработал end-to-end (отдельная задача, не из M-плана)

- **2026-04-23 — v1 (final design closed)**
  - Свёрнут весь Q&A раунд (~5 часов работы), все решения зафиксированы
  - Полный DDL-sketch, все ключевые flows прописаны
  - Принято: Master = одна универсальная запись на ВСЕ тенанты (не per-tenant)
  - Принято: Option C для variants (master + master_variants), GTIN как primary match key
  - Принято: Option B для Tier 2 (per-vertical таблицы), promotion через ALTER TABLE + backfill
  - Принято: 3-tier модель атрибутов с candidates staging
  - Принято: metadata-first import, 1 LLM-вызов на тенанта, mapping artifact
  - Принято: 4-layer cache при re-import (webhooks + hash + schema cache + skip-link)
  - Принято: COALESCE-рендер listing-as-overrides, никакого дублирования каталога
  - Принято: curator — отдельный сервис в репо, email+password логин
  - Принято: junk variants → отдельная страница в админке (только когда есть pending)
  - Принято: English-only MVP, defer multi-language/markets/locations/CDN/master-export-for-tenants
  - Принято: Tier 1 финальный список 12 полей, категории — M:N join (не Tier 1)

  Q&A исходники — в чате 2026-04-23. Полный transcript: `/Users/starknight/.claude/projects/-Users-starknight-Keepstar-project-Keepstar-one-ultra/39ae0694-3b02-4929-bdb5-286e59ce1665.jsonl`
