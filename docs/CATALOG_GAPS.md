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

### B3 — Junk-маршрут Слой 1 (детерминированные валидаторы) — ~60 мин

Per-field-kind валидаторы на apply. Плохие строки НЕ отбрасываются — флагаются. `inbox_items.data_issues jsonb[]` + `master_products.has_issues bool`.

Валидаторы:
- price: `is_null OR <= 0` → `missing_commercial`
- image_url: `NOT (http|https) OR known_broken_pattern` → `broken_image`
- name: `LIKE '/^(test|demo|tmp|sample|delete me)/i'` → `suspicious_name`
- gtin: не 8/12/13/14 цифр ИЛИ checksum fail → `bad_gtin`
- identity: name И sku оба пустые → reject (уже работает)

Admin frontend получает таб «Товары с проблемами» с фильтром по типу + counts. Клиент видит это в своём кабинете и сам поправит. Куратору — то же + bulk-fix tools.

**Слой 2** (алиасы значений типа «соль» vs «натрий хлор») — отложен в Group C: ждём реальных кейсов в curator pending review, чтобы понимать что важно нормализовать.

**Файлы (план):** новый `internal/usecases/apply_v2_validators.go`, миграция в `catalog_migrations.go`, новый admin таб.

### B4 — Внутрипакетные дубли — кричать видимо — ~40 мин

Если CSV содержит SKU дважды, сейчас обе строки молча мержатся в один мастер.

Решение:
- `tenant_apply_runs` таблица: на каждый apply сохраняется `{total, new_masters, bound, dup_in_batch, dup_skus_sample}`.
- Admin → tenant view → таб «История синхронизаций»: список последних запусков. На строке с `dup_in_batch > 0` — жёлтая плашка, click — список SKU.
- Curator карточка мастера, который собрался из >1 inbox строки в одном run — бейдж «merged from N input rows».

**Файлы (план):** новая таблица, summary capture в `apply_v2.go`, admin таб «История синхронизаций», поле в curator card.

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
| **B — Master lifecycle** | B2 | ~5 | ✦ Закрыто в 0.9.5 (включая Redis broker) |
| **B — Master lifecycle** | B3–B4 | ~2 | Осталось ~2 ч до полной Group B |
| **C — Отложено** | по триггерам | — | Возврат по событиям |

Group A полностью закрыта. Group B на 50% (B1 + B2 готовы). Из Group B остались B3 (junk-маршрут Layer 1) + B4 (внутрипакетные дубли — кричать), суммарно ~2 ч. После них — полный цикл «новый клиент + регулярные обновления без молчаливой потери данных». Group C — по триггерам.
