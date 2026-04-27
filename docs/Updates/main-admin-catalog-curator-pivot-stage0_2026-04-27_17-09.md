# Admin Catalog — Curator-Driven Pivot, Этап 0 (docs pivot)

- **Branch:** `main`
- **Date (UTC):** 2026-04-27 17:09
- **Parent commit:** `6909f0d` (alpha-0.5 — M6/M8/M9/M10/M11/M12 shipped, M4d+M7 remain)
- **Plan file:** `~/.claude/plans/flickering-napping-gem.md`
- **Active plan doc:** `docs/New features/admin_catalog_curator_pivot_2026-04-27.md`

## Context

После сессии M1-M12 в системе оставались два больших открытых пункта:
- **M4d** — auto-harvester orchestrator (применяет mapping_artifact ко всему staging автоматом, cut-over legacy, frontend progress UI и т.д.)
- **M7** — backfill 967 heybabes-продуктов с предполагаемыми русскими названиями

На сессии 2026-04-27 пользователь принял **другие архитектурные решения**, отменяющие оба пункта в их изначальной форме. Этот этап (Этап 0) — документирование pivot'а в трёх файлах: новый active plan + banner на двух старых документах.

## Архитектурные решения, требующие pivot

1. **Legacy ShopifyUseCase убирается полностью.** Никаких параллельных пайплайнов в `master_products`
2. **После Connect клиент попадает только в `catalog.products`.** Свои листинги, без `master_*`. Виджет работает сразу через **listing-only** режим поиска. Плашка "Search runs on basic schema. Full PIM enrichment in progress"
3. **Мерджинг в master — исключительно ручной**, через curator. Никакого auto-apply
4. **Merge-агент пишет отчёт, не БД.** Курратор approve/reject per-row или batch'ем → детерминированный applier выполняет
5. **Поиск V4 в двух режимах:** listing-only (нет master coverage) / master+listing (≥30% master_link_coverage)
6. **Heybabes становится seed cosmetics master-каталога** — DB-проверка перевернула M7

## Критическое открытие про heybabes (DB-проверка 2026-04-27)

Прямой `SELECT` на `catalog.master_products WHERE owner_tenant_id=hey-babes-cosmetics`:

| Метрика | Значение |
|---|---|
| Всего master_products | **979** (не 967) |
| Чистый английский в name | 978 / 979 (один сломанный — мусор в SKU) |
| Кириллица в description / display_name / original_name | **0 везде** |
| С embedding | 979 / 979 |
| С brand | 978 / 979 |
| С images | 979 / 979 |
| С PIM (skin_type/concern/key_ingredients/benefits) | 961 / 979 (98%) |
| С description | 0 (все пустые) |
| Уникальных брендов | 10+ (MEDI-PEEL 57, COSRX 52, The Saem 45, Some By Mi 37, Fraijour 36...) |

Вывод: heybabes — не "проблема M7", а **готовый seed cosmetics master-каталога**. M7 в новой формулировке: фикс одной сломанной записи + LLM batch для description backfill (~$5).

## What landed (Этап 0)

| Файл | Действие | Описание |
|---|---|---|
| `docs/New features/admin_catalog_curator_pivot_2026-04-27.md` | **CREATE** | Новый active plan (380 строк): архитектурные решения, 8 этапов с детальными task'ами, тестовые сценарии для dev-store, reuse map, что НЕ входит. Единый источник правды для оставшейся работы по каталогу/PIM |
| `docs/New features/admin_catalog_design_2026-04-23.md` | **EDIT** | Banner вверху: "Partially superseded by Curator-Driven Pivot 2026-04-27". Чётко указано что схема/UI/mental model остаются актуальными, отменены только M4d и M7 в старой формулировке |
| `docs/New features/admin_catalog_implementation_plan_2026-04-26.md` | **EDIT** | Banner вверху: "🔴 SUPERSEDED for remaining work". Список что отменено / что остаётся актуальным / ссылка на pivot |

## 8 этапов нового плана (для будущих логов)

```
Этап 0  ✅ docs pivot                              ← этот лог
Этап 1     curator UI (tenants + master browse)    самостоятельный, 4-6 ч
Этап 2     cut legacy + harvester-lite + 2-mode    самостоятельный, 3-4 ч
Этап 3     dev-store seed                          BLOCKED on user (Shopify app release)
Этап 4     merge agent design (с пользователем)    focused session
Этап 5     merge agent + curator review UI         после 4
Этап 6     e2e tests                               после 2+3+5
Этап 7     heybabes master cleanup                 30 мин когда удобно
Этап 8     забытое — пользователь вспомнит позже
```

## Files changed

| Scope | File | Action |
|---|---|---|
| docs | `docs/New features/admin_catalog_curator_pivot_2026-04-27.md` | NEW |
| docs | `docs/New features/admin_catalog_design_2026-04-23.md` | EDIT (banner) |
| docs | `docs/New features/admin_catalog_implementation_plan_2026-04-26.md` | EDIT (banner) |

## Verification

- Visual check: оба старых документа открываются → видно красный banner со ссылкой на pivot
- Pivot doc открывается → читается линейно, есть таблица heybabes-метрик, есть таблица 8 dev-store сценариев
- Никаких изменений кода → `go build` / `go vet` / тесты не запускались (не требуется)

## Known gaps / next steps

- **Этап 1 (curator UI)** — следующий, начинается сразу после коммита Этапа 0
- Этап 3 (dev-store seed) ждёт пользователя: нужен релиз Shopify-app в Partners с write-scopes
- Этап 4 (agent design) — focused-сессия, не сегодня
