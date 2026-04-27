# Admin Catalog — Curator-Driven Pivot, Этап 1 (curator UI)

- **Branch:** `main`
- **Date (UTC):** 2026-04-27 17:21
- **Parent commit:** `823cd13` (Этап 0 — docs pivot)
- **Active plan:** `docs/New features/admin_catalog_curator_pivot_2026-04-27.md`

## Context

Этап 1 из плана pivot'а: расширение curator до operations dashboard. До этого этапа курратор умел только глобальные curation queues (Candidates / Junk / Audit) и **не видел тенантов как сущностей** — не было списка клиентов, нельзя было провалиться в каталог тенанта или посмотреть его mapping artifact. Master-каталог тоже не был представлен — нельзя было искать по 979 heybabes-мастерам, смотреть их PIM-обогащение или связанные листинги.

Этот этап закрывает оба пробела. Без участия пользователя.

## What landed

### Backend (`curator/backend/`)

**`internal/domain/types.go`** — добавлены 6 новых типов: `IntegrationSummary`, `TenantSummary` (с метриками productsCount / masterLinkedCount / masterLinkCoverage / integrations / schemaStatus), `TenantProductRow` (под listing browse), `MasterProductSummary` + `MasterProductDetail` (с PIM-полями + variants + linked listings), `MasterVariantSummary`, `TenantSchemaSummary`.

**`internal/adapters/postgres.go`** — 6 новых read-only методов поверх `catalog.*` + `admin.tenant_integrations`:

| Метод | Что делает |
|---|---|
| `ListTenants(ctx)` | JOIN `catalog.tenants` + subquery counts из `catalog.products` + LEFT JOIN `tenant_catalog_schema` + hydration `admin.tenant_integrations` отдельным запросом по списку tenant_id'ов. Single round-trip + 1 hydration query |
| `GetTenant(ctx, id)` | то же что ListTenants но per id |
| `ListTenantProducts(ctx, id, search, limit, offset)` | Двухпутевой JOIN (M6 read-path): `LEFT JOIN master_variants mv ON mv.id=p.master_variant_id` потом `LEFT JOIN master_products mp ON mp.id=COALESCE(p.master_product_id, mv.master_product_id)`. COALESCE имени из display_name → original_name → name. Search по name + sku + brand |
| `GetTenantSchema(ctx, id)` | Читает `tenant_catalog_schema` (mapping_artifact JSONB + validation_report JSONB + status + version + timestamps). Возвращает пустой shape если строки нет |
| `ListMasterProducts(ctx, vertical, search, limit, offset)` | Глобальный browse `master_products` с фильтром по vertical и поиском по name + brand. Counts listings через subquery. Owner_tenant_slug через JOIN |
| `GetMasterProduct(ctx, id)` | Карточка master_product + параллельно variants + linked listings. PIM-поля (skin_type/concern/key_ingredients/target_area/free_from/benefits) свёрнуты в `map[string]interface{}` для динамического рендеринга |

**`internal/handlers/handlers.go`** — handlers под все 6 endpoints (single-file pattern, тонкая обёртка над adapter методами).

**`cmd/server/main.go`** — registered routes:
- `GET /curator/tenants` → list
- `GET /curator/tenants/{id}` → detail
- `GET /curator/tenants/{id}/products?search=&limit=&offset=` → catalog
- `GET /curator/tenants/{id}/schema` → mapping artifact + validation report
- `GET /curator/master/products?vertical=&search=&limit=&offset=` → global master browse
- `GET /curator/master/products/{id}` → master detail

Все под `auth(protected)` — те же session middleware что у существующих endpoints.

### Frontend (`curator/frontend/src/`)

Новые страницы:

| Файл | Что |
|---|---|
| `pages/TenantsPage.jsx` | Таблица: slug / name / type / products count / coverage bar (зелёный ≥80%, жёлтый ≥30%, красный <30%) / integration badges (kind + status colored) / schema status |
| `pages/TenantDetailPage.jsx` | Header с метриками-pill'ами + 4 tab'а: **Catalog** (read-only листинги тенанта со search + pagination), **Schema** (mapping_artifact + validation_report как collapsible JSON), **Reports** (placeholder под Этап 5 — merge agent), **Audit** (placeholder под per-tenant filter) |
| `pages/MasterCatalogPage.jsx` | Глобальный browse master_products с vertical-фильтром + search. Coverage dots: V (vector embedding), P (PIM), D (description) — visualize data quality at-a-glance |
| `pages/MasterDetailPage.jsx` | Hero (image + name + pills) + Description + PIM grid (skin_type/concern/etc.) + Variants table + Linked listings table |

**`App.jsx`** — обновлён routing + sidebar категоризован по 3 секциям:

```
Curator
├── Operations
│   ├── Tenants          ← NEW
│   └── Master Catalog   ← NEW
├── Curation queues
│   ├── Candidates
│   └── Junk
└── Activity
    └── Audit
```

Default route: `/tenants` (раньше был `/candidates`).

**`app.css`** — добавлены стили для всех новых компонентов (tenants table, coverage bars, integration badges, schema status colors, pills, tabs, products table, thumbnails, pagination, JSON viewer, master detail layout, PIM grid). Сохранён существующий dark-purple темизм (#7c3aed primary, #c4b5fd accent на тёмном фоне).

## Verification

### Build + smoke test
```
$ cd curator/backend && go build ./... && go vet ./...
clean

$ cd curator/frontend && npm install && npm run build
✓ 50 modules transformed.
dist/index.html                   0.40 kB
dist/assets/index-J1th71yt.css    8.74 kB
dist/assets/index-Cq-Zywiv.js   254.01 kB
✓ built in 589ms
```

### Live endpoint smoke tests (curator running на :8082)

**`GET /curator/tenants`** — возвращает 12 тенантов. heybabes-cosmetics (979/979 master coverage, schema=needs_human_review, shopify integration=connected); test-electronics (8/8); 10 пустых тенантов (test seed).

**`GET /curator/tenants/{heybabes}/products?limit=3`** — возвращает 3 листинга (snowboard'ы с brand=Keepstar, currency=RUB, hasMasterLink=true, masterId указывает на правильные master_products).

**`GET /curator/tenants/{heybabes}/schema`** — возвращает status='needs_human_review', mappingArtifact present (артефакт от M4c discovery теста на dev-store), validationReport not present.

**`GET /curator/master/products?vertical=cosmetics&limit=2`** — возвращает 987 master_products (979 heybabes + 8 test-electronics overlap), фильтр работает, pagination работает.

### Что было найдено в данных (кейсы для Этапа 2)

В процессе smoke-теста обнаружились артефакты от legacy-импортера которые подтверждают необходимость Этапа 2:

1. **17 dev-store snowboard'ов записаны как master_products с vertical='cosmetics'** под owner_tenant_id=hey-babes-cosmetics. Это потому что legacy `runInitialSync` пишет напрямую в master_products с дефолтным `vertical='cosmetics'` независимо от типа продукта. Discovery agent (M4c) корректно их пометил в transcript, но отдельной таблицы для коррекции не было.
2. **Heybabes integration в admin.tenant_integrations указывает на keepstar-neaqpan1.myshopify.com** — это тот dev-store. То есть в integrations связь между tenant'ами и Shopify-stores перепуталась во время M4-тестов.

Эти артефакты — не блокер для Этапа 1 (curator UI работает корректно с тем что есть), но напоминание что Этап 2 (cut legacy) и последующий cleanup нужны.

## Files changed

| Scope | File | Action |
|---|---|---|
| backend | `curator/backend/internal/domain/types.go` | EDIT (+97 строк) |
| backend | `curator/backend/internal/adapters/postgres.go` | EDIT (+285 строк) |
| backend | `curator/backend/internal/handlers/handlers.go` | EDIT (+90 строк) |
| backend | `curator/backend/cmd/server/main.go` | EDIT (+30 строк) |
| frontend | `curator/frontend/src/pages/TenantsPage.jsx` | NEW (~70 строк) |
| frontend | `curator/frontend/src/pages/TenantDetailPage.jsx` | NEW (~170 строк) |
| frontend | `curator/frontend/src/pages/MasterCatalogPage.jsx` | NEW (~95 строк) |
| frontend | `curator/frontend/src/pages/MasterDetailPage.jsx` | NEW (~100 строк) |
| frontend | `curator/frontend/src/App.jsx` | EDIT (+12 строк, sidebar категоризация + 4 routes) |
| frontend | `curator/frontend/src/app.css` | EDIT (+170 строк, все новые стили) |

## Known gaps / next steps

- **Этап 2** (cut legacy + harvester-lite + two-mode search) — следующий, разблокирует наполнение `catalog.products` через новый pipeline и убирает legacy которое сейчас пачкает master_products. После Этапа 2 snowboards в master_products уйдут (DELETE), новый dev-store пойдёт через harvester-lite в `catalog.products` без master_*.
- **Этап 5** Reports tab пока placeholder. После merge agent design (Этап 4) появится "Run merge agent" + список prior reports + drill-down на review.
- **Этап ?** Per-tenant audit фильтр — сейчас Audit показывает всё. Когда будет нужно — добавлю `?tenant_id=` на endpoint и фильтр в UI.
- Master catalog не показывает variants для master_products у которых их нет (heybabes case — все 979 без master_variants). Это ожидаемо, в UI отображается empty-state с пометкой "legacy heybabes shape".
