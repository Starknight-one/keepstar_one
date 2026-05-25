# Катало­г — трекер пробелов

> Живой трекер. Обновлён 2026-05-22 на alpha-0.9.5.
> Все пункты из аудита «автоматизированный PIM-менеджер» (сессия 2026-05-22) —
> здесь. По каждому: статус + файлы + что осталось.

## Где мы сейчас

- Тег: `alpha-0.9.5`
- В этом milestone приехал B2 (drift detection через Redis broker + Sonnet
  classifier + admin таб). Broker заложен как переиспользуемая инфра для
  будущих async-задач (audit fan-out, V5 events, ...).
- Sephora seed (8494 строк) ранее упал на 899 (~10%) из-за Bug 1 + Bug 2.
  Оба бага закрыты в этом milestone, плюс приехало per-field approve +
  discovery patch + lock-in идемпотентности.
- Discovery-агент корректно строит карту (Tier 1 universal / Tier 2 typed /
  Tier 3 source-specific) — это подтвердил seed на 0.9.3. Дальнейшие пробелы
  — downstream от discovery: lifecycle мастера, drift, junk-маршрут, дубли.

## Рамка

Пайплайн автоматизирует матёрого PIM-менеджера, который:
1. Принимает данные от нового клиента (CSV / Shopify / JSONL)
2. Маппит поля клиента в мастер-схему
3. Чистит и трансформирует (единицы, цены, категории)
4. Матчит к существующим мастерам → создаёт новые → обогащает существующие
5. Категоризирует в стабильную таксономию
6. **Обрабатывает регулярные обновления** — daily re-sync с новыми колонками,
   новыми SKU, удалёнными SKU
7. Детектит мусор и проблемы качества; маршрутизирует на ревью
8. Поддерживает lifecycle мастера долго после первого ингеста

Пробелы ниже читаются через эту рамку.

---

## ✦ Сделано в alpha-0.9.4

### A1 — Bug 1 (cosmetics nil arrays) + структурный helper

**Проблема:** `BulkUpsertCosmetics` падал на nil-слайсах (`jsonb_array_elements_text(null)` → SQLSTATE 22023). Из-за этого 0 строк в `master_cosmetics` после Sephora seed.

**Решение:** `coerceStringSlice(s) []string` рядом со `stringSliceArg`. Применён ко всем 6 array-полям (`SkinType`, `Concern`, `KeyIngredients`, `TargetArea`, `FreeFrom`, `Benefits`). nil → `[]string{}` до JSON-marshal.

**Структурный смысл:** будущий `BulkUpsertElectronics` / `BulkUpsertFashion`, если получит typed-таблицу, не сможет повторить этот класс багов — через helper это физически невозможно.

**Файлы:** `project_admin/backend/internal/adapters/postgres/catalog_v2_writer_adapter.go`. Регресс-тест: `internal/integration_test/regression_cosmetics_nil_test.go`.

### A2 — Bug 2 (apply throughput)

**Проблема:** 8494 строк ~10 мин на 10% (≈700ms/row). `MarkApplied` per-row + полный скан в каскадном JOIN.

**Решение:**
- `BulkMarkApplied(itemIDs []string)` в `InboxPort` + adapter (UNNEST + `WHERE id = ANY`). В `apply_v2.go` цикл per-row заменён на один запрос.
- Functional partial index `idx_master_products_lower_sku` на `LOWER(sku)` (3-key cascade JOIN в `BulkMatchOrCreateMaster` теперь не сканирует таблицу).

**Файлы:** `internal/ports/inbox_port.go`, `internal/adapters/postgres/inbox_adapter.go`, `internal/usecases/apply_v2.go`, `internal/adapters/postgres/catalog_migrations.go`.

### A3 — Approve реально пишет колонки

**Проблема:** «Approve» в curator UI до этого был флаг-флиппер — менял `approval_status='approved'`, но НЕ писал `pending_value` в целевые колонки. Куратор «одобрял», данные в проде не появлялись.

**Решение:** `ApplyPendingChanges(changeIDs)` в `curator/backend/internal/adapters/pending.go`. Per-field router по `field_name`:
- `tier3.<key>` → `master_products.tier3 || jsonb_build_object(key, value)`
- cosmetics array column → `master_cosmetics` INSERT … ON CONFLICT с DISTINCT-union
- cosmetics text/int → `master_cosmetics` INSERT … ON CONFLICT с fill-when-empty / fill-when-null
- master_products text col → `master_products` UPDATE с fill-when-empty WHERE
- неизвестное поле → counted as skipped, без ошибки

Allowlist колонок по типам (text/int/array maps) — SQL injection через `field_name` невозможен, несмотря на `fmt.Sprintf` в шаблонах. Handler теперь зовёт `ApplyPendingChanges` ДО `BulkApprove*` — «approved» означает «реально записано».

**Vertical-agnostic:** router не знает «cosmetics» или «electronics» — только «это поле в скаляр X / в массив Y / в tier3 ключ Z». Новые вертикали подключаются через артефакт, не через код.

**Файлы:** `curator/backend/internal/adapters/pending.go`, `curator/backend/internal/handlers/handler_pending.go`. Интеграционный тест: `curator/backend/internal/adapters/regression_pending_apply_test.go`.

### A4 — Идемпотентность identical re-import

**Открытие из exploration:** уже было реализовано в `inbox_adapter.go:166-170`. `ON CONFLICT DO UPDATE` сохраняет `applied_at`, если `payload_hash` совпадает; сбрасывает на NULL — если отличается. `ListUnapplied` фильтрует по `applied_at IS NULL`. Так что identical re-import УЖЕ корректно пропускается на apply.

**Что добавили:** regression test (`regression_inbox_idempotency_test.go`), который явно лочит этот неявный контракт. Будущий рефактор upsert SQL не сможет молча сломать daily-cron.

### B1 — Discovery патчит существующий артефакт

**Проблема:** на `mapping_miss` / `unknown_vertical` агент строил артефакт с нуля. Существующий тёр на save. Каждый mapping_miss = мини-discovery $0.04-0.13. Воспроизведение всех правил «в уме» агента — ненадёжно.

**Решение:**
- `newDiscoveryDraftFromArtifact(art)` — deep-copy `Branches` + `ClassifyRules` + `ClassifyingField` + `Notes` в свежий draft.
- `runLoop` теперь: на `mapping_miss` / `unknown_vertical` зовёт `d.artifact.Get(...)`; если артефакт есть — draft предзагружается. `first_install` / `manual` остаются с пустым стартом (manual = explicit full rediscovery по семантике промпта).
- Action log пишет `discovery_patch_from_existing` с counts при загрузке.

**Зачем:** разблокирует B2 (schema drift) — кнопка «Применить рекомендацию» там вызывает дешёвый patch вместо полного rebuild.

**Файлы:** `internal/usecases/discovery_v2.go`, `internal/usecases/discovery_v2_draft.go`. Unit-тесты: 3 шт. в `discovery_v2_draft_test.go`.

### Побочное в 0.9.4

- Pre-existing API drift в `integration_test/{listings_test.go, soft_delete_test.go}` — починен (logger arg для `NewCatalogV2WriterAdapter` отсутствовал).
- Mock в `TestScenario_121and123_*` теперь patch-aware — посылает `remove_field_mapping` перед стандартной commit-последовательностью, так как draft теперь предзагружен.

---

## ✦ Осталось — Group B (master lifecycle)

Цель: пайплайн переживает регулярные обновления клиента без молчаливой потери данных. Обратная сторона конвейера — что происходит ПОСЛЕ того как мастер наполнен и тот же клиент шлёт апдейты.

### B2 — Detection дрифта схемы — DONE в 0.9.5

После каждого apply (`apply_v2.go:return` хук) publish'им job в Redis Stream `stream:drift_classify`. Пул из 5 worker-горутин в admin backend (стартует в `cmd/server/main.go` рядом с cron) подписан на stream через consumer group `drift_workers`. На каждое сообщение worker:

1. Загружает существующий артефакт (если нет — записывает action_log "drift_no_artifact" и возвращает).
2. Собирает `unmapped_keys = inbox_fields \ artifact_mapped_keys`. Кап 20 полей на ран — остальные на следующий apply.
3. Для каждого unmapped зовёт `inbox.FieldStats(field, 20)` + детерминированный `GuessFieldType` (numeric/categorical/text/date/unknown).
4. Один batched LLM-вызов через `DriftClassifierClient` (Sonnet 4.6, JSON-ответ). System prompt описывает 3 решения (typo/alias/new) и допустимые таргеты (master.*/cosmetics.*/tier3.*).
5. Upsert в `catalog.schema_drift_findings` по (tenant_id, apply_run_id, field_name) — идемпотентен на redelivery.

Admin таб **/catalog/drift** показывает findings со status='classified', tabs «Pending / Applied / Dismissed». На «Apply» зовёт `discovery_v2.Discover(trigger=manual_drift_apply, payload=action)` — это смыкается с B1's patch-flow и патчит артефакт без полного rebuild'a.

**Гибкая degradation:** если `REDIS_URL` не задан — пул не стартует, `apply_v2.schemaDrift` остаётся nil, apply работает как раньше без drift detection.

**Broker как переиспользуемая инфра:** `internal/ports/broker_port.go` + `internal/adapters/broker/redis_broker_adapter.go`. Используется Redis Streams + consumer group, с `XAUTOCLAIM` каждые 60 сек для подбора stuck-сообщений (idle > 5 мин). Если когда-нибудь захотим NATS / Postgres-based queue — меняется один adapter, остальной код не трогаем.

**Файлы (созданы):** `internal/ports/broker_port.go`, `internal/ports/schema_drift_findings_port.go`, `internal/adapters/broker/redis_broker_adapter.go`, `internal/adapters/postgres/schema_drift_findings_adapter.go`, `internal/adapters/anthropic/drift_classifier_client.go`, `internal/usecases/schema_drift.go`, `internal/usecases/schema_drift_type_guess.go`, `internal/handlers/handler_schema_drift.go`, `internal/domain/schema_drift.go`, `project_admin/frontend/src/features/catalog/SchemaDriftPage.jsx` + CSS.

**Стоимость:** Sonnet 4.6 — ~$0.0165 на тенант на apply (3000 input + 500 output tokens). 10 тенантов × daily = ~$5/мес. Воркер-пул capped на 5 параллельных вызовов через размер пула; rate-limit Anthropic'а ловить нечем при текущей шкале.

**Followups (не критично сейчас):**
- Storm-фильтр в admin табе («много findings — показать ТОП-N с highest confidence»). Сейчас просто список из 200 max.
- Action log дополнить агрегатной сводкой apply_run_summary («drift: 5 findings, 3 typo, 1 alias, 1 new»).
- Опциональный inspect-draft tool для discovery агента (B1 followup).

**Test coverage — осознанно неполное (отложено для вдумчивого захода):**

Покрыто юнитами:
- `schema_drift_test.go` — PublishJob happy path, nil-broker no-op, error propagation
- `schema_drift_type_guess_test.go` — эвристики numeric/categorical/text/date/unknown

НЕ покрыто (требует отдельной тестовой сессии):
- **E2E на реальном Redis** — `XADD` → consumer group → классификатор → запись
  в `schema_drift_findings`. Сейчас брокер замокан.
- **XAUTOCLAIM recovery** — «воркер упал в середине, через 5 мин другой
  подобрал». Логика есть в адаптере, тестов нет.
- **LLM-классификатор на живых данных** — Sonnet на полях Sephora / sample
  инбокса. Промпт и парсер JSON-ответа не валидированы на реальных кейсах.
- **Patch-flow B1 ↔ B2** — кнопка «Apply» в админке → `discovery_v2.Discover(trigger=manual_drift_apply)` →
  артефакт обновлён без rebuild. Связка end-to-end не прогонялась.
- **Frontend `SchemaDriftPage.jsx`** — не открывался в браузере. Pending /
  Applied / Dismissed tabs, фильтры, кнопки — визуально не проверены.

**Почему отложили:** код 0.9.5 написан и закоммичен, но тестирование делается
отдельной сессией с реальным Redis + sample-инбоксом + чек-листом, чтобы
покрыть e2e осмысленно, а не «зелёный CI ради зелёного CI». До прогона
этого чек-листа B2 в проде не включаем.

### B2.1 — Invisibility window между drift detect и approve — дизайн не выбран

После того как B2 ловит незамапленную колонку, **значения этих ячеек в инбоксе физически живут в `inbox_items.raw`**, но в `master_products` / `master_cosmetics` они не попадают, пока владелец не апрувит таргет в drift-табе. Окно «detect → approve» = минуты-дни. В это окно данные **невидимы** в мастере, V5 чате, куратор-UI. Не потеряны навсегда (raw жив), но не доступны для query.

Варианты (решение не принято):

- **A. Auto-tier3 fallback.** На apply, если поле в инбоксе но не в карте → пишем сразу в `master_products.tier3.pending:<field>`. После approve в drift-табе мигрируем из `pending:<field>` в финальный таргет. Цена: + ~10 строк в apply, +1 jsonb write per row.
- **B. Backfill on drift apply.** Кнопка «Apply» в drift-табе помимо обновления карты гоняет повторный apply по затронутым `apply_run_id`. Дороже (re-apply), без temp-storage.
- **C. Принять окно.** Сказать «между detect и approve данные не теряются, но временно невидимы — следующий cron-sync их подхватит». Дёшево, но для one-shot upload не работает.

**Триггер выбора:** когда дойдём до сидинга боевого тенанта на B2 — на этом этапе invisibility window становится наблюдаемой проблемой, а пока — теоретическая.

### B3 — Junk-маршрут Слой 1 (детерминированные валидаторы) — ~75 мин

Per-field-kind валидаторы на apply. Плохие строки НЕ отбрасываются — флагаются. Валидация разделена по уровню данных (master vs listing), потому что мастер shared между тенантами, а листинг — per-tenant. Тенант B может иметь чистый листинг даже если у мастера флагнут `bad_gtin` от тенанта A.

**Master-level валидаторы** (флаг → `master_products.has_issues bool`):
- name: `LIKE '/^(test|demo|tmp|sample|delete me)/i'` → `suspicious_name`
- gtin: не 8/12/13/14 цифр ИЛИ checksum fail → `bad_gtin`
- image (на `master_products.images`): `NOT (http|https) OR known_broken_pattern` → `broken_image`
- identity: name И sku оба пустые → reject (уже работает)

**Listing-level валидаторы** (флаг → `catalog.products.has_issues bool`):
- price: `is_null OR <= 0` → `missing_commercial`
- currency: invalid ISO code → `bad_currency`
- stock_quantity: < 0 → `negative_stock`
- image (на `catalog.products.images` override): `NOT (http|https)` → `broken_listing_image`

**Сторадж:** `inbox_items.data_issues jsonb[]` (полный список issues на исходной строке для трассировки) + два булевых флага на разных таблицах.

**Admin frontend:**
- Куратор: таб «Мастера с проблемами» — фильтр по типу, видит все тенанты + bulk-fix
- Клиент в своём кабинете: таб «Мои товары с проблемами» — только свой `tenant_id`, чинит свой листинг

**Слой 2** (алиасы значений типа «соль» vs «натрий хлор») — отложен в Group C: ждём реальных кейсов в curator pending review.

**Файлы (план):** новый `internal/usecases/apply_v2_validators.go`, миграция в `catalog_migrations.go` (две `has_issues` колонки), curator таб + клиентский view.

### B4 — Внутрипакетные дубли — кричать видимо — ~40 мин

Если CSV содержит SKU дважды, сейчас обе строки молча мержатся в один мастер.

Решение:
- `tenant_apply_runs` таблица: на каждый apply сохраняется `{total, new_masters, bound, dup_in_batch, dup_skus_sample}`.
- Admin → tenant view → таб «История синхронизаций»: список последних запусков. На строке с `dup_in_batch > 0` — жёлтая плашка, click — список SKU.
- Curator карточка мастера, который собрался из >1 inbox строки в одном run — бейдж «merged from N input rows».

**Файлы (план):** новая таблица, summary capture в `apply_v2.go`, admin таб «История синхронизаций», поле в curator card.

### B5 — Master conflict resolution — дизайн не выбран

Master shared между тенантами. Если тенант A и тенант B оба прислали один и тот же SKU, но `brand` / `name` / `description` у них **разные** — сейчас silent last-write-wins. Кто apply'нулся позже, того значение и в мастере.

Не теоретическая проблема: в Sephora seed уже видны кейсы где один товар приходит из разных источников с чуть разными формулировками названия. Без conflict resolution мастер «дрейфует» туда-сюда между значениями тенантов.

Варианты (решение не принято):

- **A. Lock-after-first-non-empty.** Поле мастера, заполненное в первый раз, больше не перезаписывается из inbox. Новое значение → запись в curator queue «conflict candidate». Куратор решает. Самый дешёвый stub.
- **B. Per-field source tracking.** Каждое поле мастера хранит `{value, source_tenant_id, set_at}`. При конфликте приоритет «доверенному» источнику (e.g. owner_tenant > guest_tenant), фолбэк в queue.
- **C. Confidence-based.** Каждое поле — массив значений с counts. Доминирует то, что прислали >50% тенантов. Конфликт = распределение близко к 50/50.

**Триггер:** второй cosmetics-тенант с пересекающимися SKU. До тех пор A достаточно как stub.

**Файлы (план):** миграция (per-field source columns или conflict_log таблица), apply_v2.go (router выбора значения), curator queue UI, новый таб «Conflicts».

---

## ✦ Запланировано — Group D (Catalog Cleanup, identity + search layer)

> **Authoritative implementation spec: `docs/CATALOG_GROUP_D_SPEC.md`** (English).
> Реализуем по спеке; здесь — трекер-уровень. DoD и пошаговый план — в спеке.

Цель: убрать накопленный долг в схеме, переложить identity-слой в один консистентный паттерн, заложить чистый PIM-фундамент под гипер-PIM (10M+ мастеров на горизонте).

**Две фазы (решено 2026-05-25, владелец подтвердил split):**

| Фаза | Что | Часы | Когда |
|---|---|---|---|
| **Phase 1 — PIM core + чистка** | D1a, D1b, D3, D4, D6-data. Всё про данные/identity/качество мастеров. **Поиск и V5 НЕ трогаем.** | ~21-28 | сейчас («ваншот») |
| **Phase 2 — search wiring** | D2, D6-search, D5. Единственное что реально лезет в V5 read-path. | ~8-11 | отложено до захода владельца на V5 |

**Почему split:** поиск — отдельная нетривиальная задача, держим её отдельно (решение владельца). Используем expand-contract: Phase 1 строит чистые структуры/колонки **рядом** со старыми, V5 продолжает читать старое и не ломается. Phase 2 переключает V5 на новое и сносит старое.

**Порядок относительно B-серии:** Group D Phase 1 идёт ПЕРЕД оставшимися B3-B5. B3 (junk-маршрут) спроектирован против листинговой структуры; B5 (conflict resolution) зависит от уровня identity. Делать их до чистки = переделывать дважды.

**Регрессионный периметр (точки 3-4 от владельца):**
- Существующий каталог-код (`apply_v2.go`, `catalog_v2_writer_adapter.go`, `catalog_adapter.go`) — это и есть то что правим. После каждого шага существующий ingest→apply работает как раньше; проверяем регрессией.
- Group D **НЕ добавляет новых агентных фич**. Discovery-агент, B2 drift, apply — логику не расширяем, только сохраняем рабочей через рефактор. Никаких новых «агент наполняет/правит PIM» возможностей.

**English-only (точка 5 от владельца):** русского в данных и поиске нет вообще и никак. Vocab-нормализация (D6) — про английские варианты / опечатки / casing / аббревиатуры, **НЕ** про RU↔EN. Поиск на русском не поддерживаем и не трогаем.

### D1 — Identity consolidation (Position B: family + variants) — Phase 1 — ~10-13 ч (D1a + D1b)

**Решение:** master_products = семья (одна сущность «L'Oreal Mascara X»), master_variants = оси вариативности (объём / цвет / аромат), листинг всегда ссылается на конкретный variant_id. Для невариативных товаров создаётся **default-variant** (1:1 с мастером, все оси null).

**Шаги:**
- DROP 17 легаси cosmetics-колонок на `master_products` (`skin_type`, `concern`, `key_ingredients`, `target_area`, `free_from`, `benefits`, `marketing_claim`, `how_to_use`, `volume`, `product_form`, `texture`, `routine_step`, `routine_time`, `application_method`, `inci_text`, `short_name`, `original_name`, `product_line`, `enrichment_version`). Все эти атрибуты уже живут в `master_cosmetics` — текущее место.
- Backfill default-variant для всех мастеров без вариантов.
- `catalog.products.master_variant_id` сделать NOT NULL (после backfill).
- apply_v2.go: router всегда создаёт variant даже для «плоских» товаров.
- master_variants → status «первый класс», убрать комментарий «being phased out» из catalog_migrations.go.

**Риск:** существующие writer'ы могут читать легаси-колонки master_products. Перед DROP — grep на каждый из 17 имён, переключить читателей на master_cosmetics, потом DROP.

### D2 — Tenant search projection layer — **Phase 2 (отложено до V5)** — ~5-7 ч

> Phase 2: лезет в V5 read-path. НЕ делаем в текущий «ваншот». Когда владелец зайдёт на V5 — тогда строим таблицу, populate, и переключаем чтение.

**Решение:** новая таблица `catalog.tenant_search_projection` — денормализованная per-tenant копия search-relevant данных мастера + варианта + tenant_overrides. Чат V5 ищет ТОЛЬКО по ней. master_products.embedding остаётся, но используется только в ingest/curator/merger.

**Схема:**
```sql
CREATE TABLE catalog.tenant_search_projection (
  tenant_id          UUID NOT NULL REFERENCES catalog.tenants(id) ON DELETE CASCADE,
  master_variant_id  UUID NOT NULL REFERENCES catalog.master_variants(id) ON DELETE CASCADE,
  master_product_id  UUID NOT NULL,
  vertical           TEXT NOT NULL,
  tier1_text         TEXT,                   -- name + brand
  tier2_text         TEXT,                   -- per-vertical typed flatten
  tier3_text         TEXT,                   -- сборная солянка для богатого поиска
  variant_text       TEXT,                   -- 7мл, тон 03, etc.
  override_text      TEXT,                   -- tenant_overrides + display_name (приватное)
  embedding          vector(384),
  search_tsv         tsvector,
  updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (tenant_id, master_variant_id)
);
CREATE INDEX idx_tsp_hnsw ON catalog.tenant_search_projection USING hnsw (embedding vector_cosine_ops);
CREATE INDEX idx_tsp_fts ON catalog.tenant_search_projection USING gin (search_tsv);
CREATE INDEX idx_tsp_tenant_vertical ON catalog.tenant_search_projection (tenant_id, vertical);
```

**Заполнение:** триггер ИЛИ explicit `projection.Rebuild(tenant_id, master_variant_id)` вызов из apply_v2 (после write мастера) и из catalog.products write-path (когда тенант редактирует override). Я бы выбрал explicit (триггер на jsonb-апдейтах — болезненно дебажить).

**V5 read-path:** переписать `project_v5/backend/internal/adapters/postgres/postgres_catalog_vector.go` — вместо JOIN master_products+products с фильтром по tenant, прямой запрос к projection с `WHERE tenant_id = $1`.

**Что это даёт:** при 10M мастеров и 1000 тенантов чат не деградирует. HNSW смотрит только в slice конкретного тенанта, ~10ms latency. Без проекции тот же запрос на 10M = полный degradation HNSW.

### D3 — Categories cleanup — Phase 1 — ~2-3 ч

**Сейчас:** 6 категорийных таблиц (`catalog.categories`, `master_categories`, `tenant_categories`, `master_product_categories`, `category_mapping`, `tenant_listing_categories`). Часть V1, часть V2, часть мёртвая.

**Шаги:**
- grep на читателей `catalog.categories` (V1). Если только legacy admin-код — DROP. Если живые — переключить на `master_categories`.
- Аудит: какие из 6 таблиц реально нужны для дизайна **master_categories ↔ master_products** и **tenant_categories ↔ catalog.products** + **category_mapping** между ними? Остальное DROP.
- Целевой набор: 4 таблицы вместо 6.

### D4 — DROP мёртвого — Phase 1 — ~3-4 ч (после impact assessment)

- `catalog.master_services` + `catalog.services` — структурные клоны master_products + products для услуг. **Действительно мёртвые**: `CreateOrUpsertService`, `GetServices`, `UpdateServiceEmbedding` + 3 неиспользуемых `LEFT JOIN catalog.master_services` в `catalog_adapter.go`. DROP методы, потом DROP таблицы.
- ~~`catalog.merge_reports` — мёртвый pre-inbox артефакт.~~ **Корректировка после inspect: таблица ALIVE.** Используется в admin handlers + `integrations_wipe.go`. **Не трогаем.**
- `catalog.stock` vs `catalog.products.stock_quantity` — оба активны. Решение: `catalog.stock` становится hot-path канонической (под будущий KeyDB/Tarantool вынос), `catalog.products.stock_quantity` остаётся как denorm для backward compat. `BulkUpdateStock` пишет в обе. Eventually deprecate denorm.

### D6 — Controlled vocabularies + aliases (dim-tables) — ~8-12 ч (split по фазам)

**English-only:** все canonical-значения и алиасы — английские. Алиасы покрывают варианты написания / опечатки / casing / аббревиатуры (НЕ перевод). Примеры ниже все английские.

**Проблема:** масса полей мастер-слоя — свободные `TEXT` / `TEXT[]` там, где должны быть словари с алиасами. Из-за этого `"oily"` ≠ `"oil-prone"` ≠ `"oily skin"` ≠ `"combination-oily"` в фильтрах, бренды задваиваются (`"L'Oreal"` vs `"L'OREAL"` vs `"Loreal"` vs `"L'Oréal"`), ингредиенты живут двумя параллельными мирами (есть `catalog.ingredients` + есть `master_cosmetics.key_ingredients TEXT[]` без связи).

**Split по фазам (expand-contract):**
- **D6-data (Phase 1):** строим dim-таблицы + aliases + нормализатор, пишем canonical-ID в **новые параллельные колонки** (`*_ids UUID[]`), старые `TEXT[]` оставляем нетронутыми → V5 search не ломается. Brand-дедуп + ingredient-линк (они не фигурируют в V5-фильтрах).
- **D6-search (Phase 2):** переключаем V5-фильтры (skin_type/concern/target_area/free_from в `postgres_catalog_vector.go`) на `*_ids UUID[]`, потом DROP старых `TEXT[]`.

**Поля-кандидаты** (controlled vocab, не свободный текст):

| Поле | Сейчас | Должно стать |
|---|---|---|
| `master_products.brand` | `VARCHAR(255)` | FK → `dim_brands` (+ aliases) |
| `master_cosmetics.skin_type[], concern[], target_area[], free_from[], benefits[]` | `TEXT[]` | `UUID[]` → `dim_skin_type` / `dim_concern` / etc. |
| `master_cosmetics.key_ingredients[]` | `TEXT[]` | подключить к существующему `catalog.ingredients` через `product_ingredients` junction (обе таблицы уже есть, не связаны) |
| `master_cosmetics.product_form, texture, routine_step, routine_time, application_method, scent` | `TEXT` | enum или FK на `dim_*` (мало значений на вертикаль) |
| `master_variants.color, material` | `TEXT` | FK → `dim_colors` / `dim_materials` |
| `master_variants.size` | `TEXT` | FK → `dim_sizes` (с размерной системой: EU/US/RU/буквы) |
| `master_products.vertical` | `TEXT` без констрейнта | минимум `CHECK` enum |

**Паттерн (уже работает в `catalog.unit_aliases` — это прецедент):**

```sql
catalog.dim_<attr> (
  id          UUID PK,
  canonical   TEXT UNIQUE,        -- "oily"
  vertical    TEXT,               -- "cosmetics"
  display     TEXT,               -- "Oily skin" (English display label)
  source      TEXT CHECK IN ('seed', 'curator', 'promoted')
);
catalog.dim_<attr>_aliases (
  raw_token   TEXT PK,            -- "oily-prone", "oily skin", "combination-oily", "OILY"
  <attr>_id   UUID FK → dim_<attr>.id,
  source      TEXT
);
```

**Ingest-нормализатор:** на apply каждое value (lowercased) прогоняется через aliases. Hit → canonical-ID в `*_ids` колонку. Miss → строка в `master_attribute_candidates` (таблица уже есть, готова к этому workflow).

**Что это даёт:**
- Поиск (когда D6-search в Phase 2 подключит): `"oily-prone"` и `"oily skin"` → один `skin_type_id` → фильтр находит оба. Поиск становится **по значению, а не по строке**.
- Куратор: новые значения попадают в очередь, не плодят дубли молча.
- Дедупликация мастеров: `"L'Oreal"` / `"L'OREAL"` / `"Loreal"` → один `brand_id`, мастера не задваиваются. **Это работает уже в Phase 1** (brand-дедуп не зависит от V5).
- Facets-фильтры в чате становятся возможны (Phase 2): конечное индексированное множество значений.

**Файлы (план):**
- **D6-data (Phase 1):** новые `dim_*` + `*_aliases` миграции; seed из текущих values; ingest-нормализатор в `apply_v2.go` (или `vocab_normalizer.go`); `master_cosmetics` writer пишет `*_ids` колонки **в дополнение** к `TEXT[]`; brand→`brand_id` на `master_products`; ingredient-линк `master_cosmetics`↔`catalog.ingredients` через `product_ingredients`; curator-таб «Vocab promotion queue» (из `master_attribute_candidates`).
- **D6-search (Phase 2):** `postgres_catalog_vector.go` фильтры → `*_ids`; DROP старых `TEXT[]`.

**Зависимость:** D6-data идёт ПОСЛЕ D1 (легаси-колонки на master_products снесены, источник правды один — master_cosmetics, переключать на одной таблице).

### D5 — vertical → tier2 unification — отложено

После D1+D2+D6 решение становится тривиальным: либо vertical остаётся отдельной колонкой как hot fast-path-фильтр, либо переезжает в `master_field_definitions` как обычный promoted attribute с денормализованным кэшем. Сейчас не принимаем — приоритет D1-D4-D6.

---

## ✦ Отложено — Group C

Каждый пункт со своим триггером возврата. Не «забыто» — отложено осознанно.

| Пункт | Почему отложено | Триггер вернуться |
|---|---|---|
| **Нормализация брендов + дедуп (pg_trgm + curator queue)** | Тенантов мало, пересечения брендов редкие. Curator вручную мержит. Standalone ~90 мин. | Приходит 2-й cosmetics-тенант с пересекающимися брендами и ручной merge начинает раздражать |
| **Удалённые SKU (Shopify Bulk async + listing soft-delete)** | Standalone 3-4 ч. Per-tenant — мастер не трогаем, только `catalog.products` этого тенанта получает `deleted_at`. Мастер общий для всех, у других тенантов товар может остаться. | Включаем production daily-sync для боевых тенантов |
| **Junk-маршрут Слой 2 (алиасы значений атрибутов)** | Ждём реальные кейсы | Появляются пробелы алиасов в curator pending review |
| **Группировка вариантов (1 товар → N SKU)** | Структурное изменение. Меняет форму `master_products` с 1:1 на 1:N. Каскадно бьёт по curator UI, V5 widget, Shopify ingest. 8-12 ч piece. | Отдельный milestone после Group A+B |
| **Обогащение описаний (LLM-rewrite копирайта)** | Мастер чистый, владелец курирует руками. Не планируется. | — |
| **Confidence score per row / needs_review приоритизация** | Владелец делает QC сам, авто-приоритет не нужен. Не планируется. | — |
| **Периодическое re-discovery против накопленного мастер-состояния** | B1 + B2 покрывают reactive путь (mapping_miss / drift detection). Расписное обновление — позже. | Тенанты накопили >3 месяцев master history |
| **Master change history / undo last apply** | Истории изменений мастера нет — нельзя ответить «какая была цена 3 месяца назад» или откатить плохой apply. inbox_items хранит входные строки, но агрегата по мастеру нет. | Появится curator-фича «откатить последний sync», биллинг-аудит на изменения, или конфликт-resolution из B5 потребует истории |

**Минимальный дешёвый шаг для variant grouping сейчас** (если хочется не терять данные): при Shopify ingest сохранять `source_parent_id` + `source_variant_id` в `inbox_items.raw`. Ничего не стоит сейчас, потом структурная переделка получит данные без re-ingest.

---

## Структурные заметки

### Новые вертикали (electronics, fashion, ...) работают без правки кода

Текущая схема:
- `master_products` — универсальные scalar-колонки (name, brand, sku, image, vertical, ...)
- `master_cosmetics` — типизированные cosmetics scalar + array
- `master_products.tier3 jsonb` — хвост source-specific или vertical-specific полей

**Haircare уже целиком живёт в tier3** — типизированной таблицы под него нет — и нормально работает. Так же будет с electronics: универсальные поля в `master_products`, хвост в `tier3`. Таблица `master_electronics` НЕ нужна, пока вертикаль не докажет популярность И V5 движку не понадобится быстрый структурированный фильтр (типа «RAM ≥ 16 GB»).

### Cosmetics nil-баг был КЛАССОМ багов

Структурный фикс A1 (`coerceStringSlice` helper) делает повторение этого класса физически невозможным в любых будущих `BulkUpsert*` writer'ах. Стоило сделать, даже если сейчас типизированная таблица только одна.

### Discovery не имел «inspect-draft» tool

`discovery_v2.go` до B1 не имел инструмента, чтобы агент мог прочитать существующий артефакт. На mapping_miss агент получал одну строку контекста и строил с нуля. B1 загружает существующий артефакт в draft на mapping_miss / unknown_vertical. Промпт ещё не обновлён под patch-flow — это followup внутри B2.

### Физическое разделение per-tenant данных — отложено, но критично

Сейчас `catalog.products` совмещает в одной таблице два разных слоя:
- **Offering** (commerce): `price`, `currency`, `stock_quantity`, `rating`
- **Presentation** (CMS-override): `name` override, `description` override, `images` override

Это два разных паттерна доступа:
- **Offering** — high read (каждый просмотр карточки в чате), high write (каждое изменение цены/стока через webhook), latency-sensitive, должно быть rock-solid атомарно
- **Presentation** — низкий read относительно offering (только при рендере карточки, кэшируется), редкие writes (тенант правит руками раз в недели), допускает stale

**Внешний сигнал (опыт владельца, Магнит):** в проде PIM в одной БД хранит только товары (мастера), а цены/стоки вынесены в отдельную БД на Tarantool — потому что их запрашивают per-second с гигантской нагрузкой. Когда мы дорастём до боевых тенантов с реальным трафиком — этот split тоже понадобится.

**Что делать сейчас:** ничего. Оставить `catalog.products` как есть. Зафиксировать что split понадобится. Триггеры расщепления:
- (a) первый тенант с реальным трафиком, где offering-чтения упрутся в shared `catalog.products`, ИЛИ
- (b) появится необходимость в разных правах доступа (бухгалтерия правит цены, контент — описания), ИЛИ
- (c) появится Shopify webhook stock-update flow с частотой выше «раз в день».

**Цена расщепления когда дойдём:** новые `tenant_offerings` + `tenant_presentations`, миграция данных, JOIN'ы в read-path (или денормализованный кэш для V5 чата). Оценка ~6-10 ч работы. После расщепления offering можно вынести в Redis/KeyDB/Tarantool как горячий слой, оставив Postgres только для presentation + master.

### PIM как отдельный сервис — отложено

Оценка по коду: ~17-25 ч работы. Связки лёгкие (4 точки coupling), но видимого пользователю эффекта 0. Триггер вернуться: либо отдельный billing-план для PIM, либо discovery начнёт тормозить admin под нагрузкой, либо появится отдельный разработчик/команда. До этого — модуль внутри admin, граница укрепляется через ports.

---

## Что вне зоны этого трекера

- V5 chat engine — см. `docs/v5-known-gaps.md`
- Auth / billing / admin SPA — см. `docs/PRE_LAUNCH_TASKS.md`
- Pre-existing test failures (`Scenario_052-055` Shopify consent flow) — out of scope
- PIM как отдельный сервис — обсуждается отдельно (см. структурные заметки)

---

## Источники

- `docs/Updates/main_2026-05-21_03-08.md` — Alpha 0.9.1 (pending approval + bidirectional discovery + cron), первый Sephora seed
- `docs/Updates/main_2026-05-22_00-14.md` — Alpha 0.9.2 + 0.9.3 (builder pattern + bulk apply), Bug 1 + Bug 2 обнаружены, агент построил карту корректно
- `docs/Updates/main_2026-05-22_02-27.md` — Alpha 0.9.4 (Group A + B1, этот milestone)
- Разговор 2026-05-22 — PIM-manager-frame аудит, владелец дал verdict по каждому пункту, разбили на A/B/C

---

## Сводка по оценке

| Группа | Пункты | Часы | Итог |
|---|---|---|---|
| **A — Sephora blockers** | A1–A4 | ~3.5 | ✦ Закрыто в 0.9.4 |
| **B — Master lifecycle** | B1 | ~1 | ✦ Закрыто в 0.9.4 |
| **B — Master lifecycle** | B2 | ~5 | ✦ Код в 0.9.5 (включая Redis broker); e2e/LLM/frontend не оттестированы — см. блок «Test coverage» в B2 |
| **B — Master lifecycle** | B2.1 | — | Дизайн не выбран (A/B/C); решение при сидинге боевого тенанта |
| **B — Master lifecycle** | B3 | ~1.25 | Junk Layer 1 — master + listing split валидаторов |
| **B — Master lifecycle** | B4 | ~0.7 | Внутрипакетные дубли — кричать видимо |
| **B — Master lifecycle** | B5 | ~3–5 | Master conflict resolution — дизайн не выбран; триггер 2-й cosmetics-тенант |
| **D — Catalog Cleanup** | D1a | ~4-5 | DROP 17 legacy cosmetics-колонок на master_products. 6 файлов, 4×M severity. |
| **D — Catalog Cleanup** | D1b | ~6-8 | master_variant_id NOT NULL + synthetic default-variants. **2 L-severity refactors в `catalog_v2_writer_adapter.go`**. |
| **D — Catalog Cleanup** | D2 | ~5-7 | tenant_search_projection + переписать V5 read-path. **1 L-severity в `postgres_catalog_vector.go`**. |
| **D — Catalog Cleanup** | D3 | ~2-3 | Cleanup `catalog.categories` (V1). Все 6 категорийных таблиц V2 alive — трогать не надо. |
| **D — Catalog Cleanup** | D4 | ~3-4 | DROP master_services + services. **merge_reports — alive, не трогаем.** Stock dedup через двойной write. |
| **D — Catalog Cleanup** | D6 | ~8-10 | Controlled vocabularies. **2 L-severity в `UpsertCosmetics`/`BulkUpsertCosmetics`** + переключение 4 фильтров в V5 search. |
| **D — Catalog Cleanup TOTAL** | D1–D6 | **~28-37** | Реалистичная оценка после impact assessment. Прошлый estimate 14-16ч был недосчитан в 2 раза. |
| **C — Отложено** | по триггерам | — | Возврат по событиям |

Group A полностью закрыта. Group B по коду на 40% (B1 + B2 из 5 пунктов), B2 ждёт e2e-тестовой сессии. **Следующий ход — Group D (Catalog Cleanup, ~28-37 ч по impact assessment)**: identity-консолидация + search projection layer + category cleanup + DROP мёртвого + controlled vocabularies. 5 L-severity refactors (synthetic variants ×2, projection JOIN rewrite, UpsertCosmetics FK conversion ×2). Группа D идёт ПЕРЕД оставшимися B3-B5 потому что они спроектированы против текущей грязной структуры. Группа C — отложено по триггерам. После D + B3/B4 — полный цикл «новый клиент + регулярные обновления без молчаливой потери данных», на фундаменте, готовом к 10M+ мастеров и мульти-язычному поиску.
