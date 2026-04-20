# Catalog: wire category tree to DB, translate names to English

- **Branch:** `main`
- **Date (UTC):** 2026-04-20 23:33
- **Parent commit:** `f8ae15f` (feat(admin): restore Traces link in sidebar nav)
- **Commit sha:** _pending commit_

## Context

The catalog redesign that shipped in `be78e70` wired a brand-new UI but left
the category tree on a hardcoded English skincare hierarchy that never
matched real data. In practice:

- DB categories were seeded in Russian (Очищение, Тонизирование, …) by
  `project_admin/backend/cmd/seed/main.go`, so the frontend's English
  `.toLowerCase().includes("moisturizers")` comparison never matched — every
  click returned 0 products.
- `CategoryTree.jsx` never called `GET /admin/api/categories`, even though
  the route existed and worked.
- Filter was only applied to the current 25-row page, not server-side.
- `GET /admin/api/categories` leaked cross-tenant rows (`Electronics`,
  `Laptops` from `test-electronics`) and a malformed `slug='store'` row.
- `ListProducts` did exact-match on `category_id`, so clicking a parent
  category missed all children.
- Project rule (MEMORY): no Russian in any user-facing text.

## Approach

1. **DB cleanup (one-shot SQL, idempotent).** Translated all 24 existing
   category names by slug (slugs are stable and what LLM prompts reference,
   so renames are safe). Reassigned the single product referencing
   `slug='store'` to `face-care`, then deleted that row. `Electronics` /
   `Laptops` are not touched in the DB — they're re-seeded by the v4 server
   on boot for the `test-electronics` tenant. Instead, hidden via tenant
   scoping in the admin adapter.

2. **Seed source updated.** `cmd/seed/main.go` now uses English names so a
   fresh DB comes up in English. Existing DBs are fixed by the one-shot SQL.

3. **Backend: tenant-scoped tree + recursive filter.**
   - `GetCategories` signature now takes `tenantID`. Query uses a recursive
     CTE to compute product counts for each category as the sum over itself
     and all descendants, tenant-scoped and excluding soft-deleted products.
     Only categories with ≥ 1 product for the tenant are returned. Keeps
     cross-tenant junk out without touching v4 seeds.
   - `ListProducts` `CategoryID` filter replaced with
     `mp.category_id IN (WITH RECURSIVE sub AS (…) SELECT id FROM sub)` so
     clicking a parent captures all descendants.
   - `domain.Category` gained `ProductCount int` (JSON `productCount`).

4. **Frontend: tree from API + server-side filter.**
   - `CategoryTree.jsx` now fetches `/categories` on mount, builds a
     `parentId` → children map, prepends a synthetic "All products" root,
     renders product-count badges, and shows a 3-row skeleton shimmer while
     loading. Emits `{id, name}` instead of a name-match string.
   - `ProductsPage.jsx` now stores `{id, name}`, sends `categoryId` as a
     query param, drops the client-side `rows.filter` block, and shows
     `category.name` in the breadcrumb.

5. **CSS.** Added count badge, label truncation, and skeleton shimmer
   styles in `catalog.css`.

## Files changed

| Scope | File |
|---|---|
| DB cleanup (not committed) | one-shot SQL run via psql |
| Seed translation | `project_admin/backend/cmd/seed/main.go` |
| Backend port | `project_admin/backend/internal/ports/catalog_port.go` |
| Backend domain | `project_admin/backend/internal/domain/category.go` |
| Backend adapter | `project_admin/backend/internal/adapters/postgres/catalog_adapter.go` |
| Backend usecase | `project_admin/backend/internal/usecases/products.go` |
| Backend handler | `project_admin/backend/internal/handlers/handler_products.go` |
| Demo chat seed | `project_admin/backend/cmd/seed-demo-chat/main.go` (new, earlier in session) |
| Frontend tree | `project_admin/frontend/src/features/catalog/CategoryTree.jsx` |
| Frontend list | `project_admin/frontend/src/features/catalog/ProductsPage.jsx` |
| Frontend CSS | `project_admin/frontend/src/features/catalog/catalog.css` |

## Verification

- SQL run: `UPDATE 24 / UPDATE 1 / DELETE 1`. Post-run spot checks:
  `SELECT name FROM catalog.categories WHERE slug='cleansing'` → `Cleansing`;
  `SELECT COUNT(*) FROM catalog.categories WHERE slug='store'` → `0`.
- `cd project_admin/backend && go build ./...` — clean.
- `cd project_admin/backend && go vet ./...` — clean.
- `cd project_admin/frontend && npm run build` — clean (1.89 MB bundle,
  same size order as prior main).
- Manual UI smoke test is the next pass — admin backend/frontend were not
  running during this session. Expected flow:
  - `/catalog` tree loads via API, English-only, counts on the right.
  - "All products" shows full tenant catalog.
  - Clicking "Cleansing" filters to ~399 products paginated server-side.
  - Clicking "Face care" (parent) shows union of descendants (~960 products).
  - No Electronics / Laptops / `store` rows appear.

## Known gaps / caveats

- `ProductDetailPage` form (brand/category/images editability, plus
  Additional information stubs) is explicitly out of scope — separate wave.
- `catalog.products.currency` default `'RUB'` not touched.
- No new categories added — uses the existing 24-item skincare hierarchy.
- Live API smoke test (`curl /admin/api/categories`, `?categoryId=…`) not
  performed in this session; needs to happen when servers are up.
- Demo chat seed (`cmd/seed-demo-chat`) was added earlier in the session
  but is a separate feature; included here for completeness.
