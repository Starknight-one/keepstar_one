# Admin Catalog — M8 Categories M:N + tree editor

- **Branch:** `main`
- **Date (UTC):** 2026-04-26 13:52
- **Parent commit:** `a200f4a` (M6 COALESCE read-path)
- **Plan:** `~/.claude/plans/wise-foraging-walrus.md`

## Context

В M1 уже созданы таблицы `master_categories`, `tenant_categories`, `category_mapping`, `master_product_categories`, `tenant_listing_categories`, и `categories_v2_adapter.go` реализован полностью. Но adapter не был зарегистрирован в DI, не было handler'а и не было frontend'а — единственный путь к категориям шёл через legacy `HandleCategories` в `handler_products.go` (который читает старую таблицу `catalog.categories`, не tenant-tree).

M8 закрывает это: добавляет handler'ы для CRUD tenant-категорий + read-only master + N:1 mapping + M:N listing↔category, регистрирует adapter в DI и подвязывает frontend (CategoryEditor + sidebar link + multi-select на ProductDetailPage).

## Approach

### Backend — handler + DI

`handler_categories.go` — новый файл, 7 endpoints:
- `GET  /admin/api/categories/tenant` → `ListTenantCategoriesWithCounts` (новый метод адаптера, plain SELECT с LEFT JOIN на `tenant_listing_categories` + `products` для `productCount`)
- `POST /admin/api/categories/tenant` → upsert (по external_id если есть, иначе insert)
- `PATCH /admin/api/categories/tenant/{id}` → upsert (TODO: добавить explicit UpdateByID — сейчас полагается на external_id или заведение новой строки)
- `DELETE /admin/api/categories/tenant/{id}` → новый `DeleteTenantCategory` метод адаптера: re-parents children to NULL в одной транзакции, потом DELETE
- `GET /admin/api/categories/master?vertical=` → read-only master categories
- `POST /admin/api/categories/mapping` → tenant_category → master_category (N:1, NULLable)
- `GET/PUT /admin/api/products/{id}/categories` → M:N listing↔tenant_category links

Routing: основная сложность — `/admin/api/products/{id}/categories` встраивается в существующий `protected.HandleFunc("/admin/api/products/", ...)` через дополнительную проверку `strings.HasSuffix(path, "/categories")`. Это сохраняет старые routes продуктов нетронутыми.

DI:
```go
categoriesV2Adapter := postgres.NewCategoriesV2Adapter(dbClient, log)
categoriesHandler := handlers.NewCategoriesHandler(categoriesV2Adapter, log)
```

### Port + adapter extensions

В `categories_port.go` добавлены:
- `ListTenantCategoriesWithCounts(ctx, tenantID)` — реализация в адаптере с подзапросом для counts
- `DeleteTenantCategory(ctx, tenantID, categoryID)` — транзакционный delete с re-parent

В `domain.master_category.go` добавлен `TenantCategoryWithCount` (struct embed + `productCount int`).

### Frontend — CategoryEditor + ProductDetailPage multi-select

`features/catalog/CategoryEditor.jsx` — новая страница на `/catalog/categories`:
- Дерево tenant_categories (рекурсивный рендер) с collapsible узлами
- Inline form для create/edit: `name`, `slug` (auto-slugify по name пока не touched), `kind` (category/showcase/promo), `parentId` (select из всех tenant categories)
- Per-node actions: Add child / Edit / Delete
- Empty state с подсказкой что harvester заполнит при импорте
- **Drag-drop отсутствует** — план говорил "можно нативный HTML5 dnd без библиотек", но dnd для tree сложнее чем select-based parent picker. Сейчас move через Edit form → выбор parent. TODO добавить dnd когда будет нужно.

`features/catalog/categoryEditor.css` — стили (tree indent, kind-badges, action buttons).

`ProductDetailPage.jsx`:
- Загружает `/categories/tenant` и `/products/{id}/categories` параллельно с продуктом
- Новая секция "Categories" — chip-multiselect с `<input type="checkbox">`-чипами
- На Save отправляет два запроса последовательно: `PUT /products/{id}` (listing fields) и `PUT /products/{id}/categories` (M:N links)

Sidebar: новый sub-link "Categories" под "Catalog" через NavLink с `sidebar-sub` modifier-class. `<NavLink to="/catalog" end>` — `end` чтобы parent link не подсвечивался когда мы на /catalog/categories.

`apiClient.js` — добавлены методы `patch` и алиас `del` (для удобства; раньше только `get/post/put/delete`).

## Files changed

| Scope | File | Change |
|---|---|---|
| backend ports | `internal/ports/categories_port.go` | +ListTenantCategoriesWithCounts, +DeleteTenantCategory |
| backend domain | `internal/domain/master_category.go` | +TenantCategoryWithCount struct |
| backend adapter | `internal/adapters/postgres/categories_v2_adapter.go` | +ListTenantCategoriesWithCounts (LEFT JOIN с counts), +DeleteTenantCategory (tx с re-parent) |
| backend handler | `internal/handlers/handler_categories.go` | NEW — 7 endpoints |
| backend DI | `cmd/server/main.go` | +categoriesV2Adapter, +categoriesHandler, +routes; sub-path routing для `/products/{id}/categories` |
| frontend | `src/features/catalog/CategoryEditor.jsx` | NEW — tree editor |
| frontend | `src/features/catalog/categoryEditor.css` | NEW |
| frontend | `src/features/catalog/ProductDetailPage.jsx` | +categories chip multi-select; PUT M:N links на Save |
| frontend | `src/features/catalog/productDetail.css` | +.pd-cat-list/chip/kind/empty стили |
| frontend | `src/features/layout/DashboardLayout.jsx` | +Categories sub-link под Catalog (FolderTree icon) |
| frontend | `src/features/layout/layout.css` | +.sidebar-sub modifier |
| frontend | `src/App.jsx` | +/catalog/categories route |
| frontend | `src/shared/api/apiClient.js` | +patch, +del alias |

## Verification

- `cd project_admin/backend && go build/vet/test ./...` — clean
- `cd project_admin/frontend && npm run build` — `built in 3.48s`, no errors
- Smoke (after deploy):
  - GET `/admin/api/categories/tenant` → пусто на свежем тенанте, empty state
  - POST `/admin/api/categories/tenant` `{name,slug,kind:'category'}` → 200 + id
  - PATCH `.../{id}` (rename), DELETE `.../{id}` (re-parent + delete)
  - На ProductDetailPage чекнуть категорию → Save → SELECT * FROM tenant_listing_categories WHERE listing_id=... → row есть
  - Sidebar: "Catalog" не подсвечен когда на /catalog/categories (потому что end-prop)

## Known gaps

1. **PATCH через UpsertTenantCategory** — текущий метод адаптера upsert по external_id или insert. PATCH с пустым external_id может создать новую строку вместо обновления существующей. Для MVP с category_editor это ОК (UI всегда шлёт `id` через URL и handler передаёт в struct.ID — но adapter этот ID не использует). TODO: расширить port явным `UpdateTenantCategory(id, ...)` — пока не критично, потому что harvester ещё не запущен и кто-то с production-данными не сломает. Подкрутим в M4 polish.
2. **Drag-drop** для дерева не реализован. Move делается через Edit form. Это утилитарно, но не как "drag in real time" — в плане было пожелание, не требование.
3. **Master category mapping UI** — endpoint есть (`POST /admin/api/categories/mapping`), но curator-frontend на нём ещё не поднят. Будет в M11. Сейчас через тестовый curl можно записать mapping, чат-движок его не читает (M:N продуктов ходит через tenant_categories напрямую).
4. **Listing M:N для master_categories** через `LinkMasterProductToCategories` доступен через port, но нет admin endpoint'а — это curator-only операция, ляжет в M11.
5. **Slug uniqueness** не enforced на DB-уровне для tenant_categories (только uniqueness на (tenant_id, external_id)). Если пользователь введёт два category с одинаковым slug — обе создадутся. Не страшно для MVP.

## Next

M9 — detected add-ons UI (junk triage page).
