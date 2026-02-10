# Done: Admin Panel MVP — самостоятельная загрузка каталогов клиентами

**Дата:** 2026-02-10
**Ветка:** `feat/admin-panel-mvp`
**Статус:** Реализовано, backend + frontend билдятся, готово к интеграционному тестированию

## Контекст

Keepstar — AI-чат для каталогов товаров. Для продаж нужна админка, через которую клиенты самостоятельно загружают каталоги. Без неё продукт нельзя продавать — каталоги зашиты в seed-файлах.

Админка — **отдельный проект** в том же репо (`project_admin/backend/` + `project_admin/frontend/`), своя гексагоналка, та же Postgres БД. Не трогает pipeline/agents.

## Что сделано

### Структура проекта

```
project_admin/
  backend/              ← Go, порт 8081 (34 файла)
    cmd/server/main.go
    internal/
      config/config.go
      logger/logger.go
      domain/           ← 7 entity файлов
      ports/            ← 4 интерфейса
      adapters/
        postgres/       ← 6 файлов (client, migrations×2, auth, catalog, import)
        openai/         ← embedding_client.go
      usecases/         ← 4 файла (auth, products, import, settings)
      handlers/         ← 7 файлов (auth, products, import, settings, cors, jwt, response)
  frontend/             ← React 19 + Vite 7, порт 5174 (25 src файлов)
    src/
      features/auth/    ← LoginPage, SignupPage, AuthProvider
      features/catalog/ ← ProductsPage, ProductDetailPage
      features/import/  ← ImportPage (upload + polling + history)
      features/settings/← SettingsPage (GEO + enrichment)
      features/layout/  ← DashboardLayout + Sidebar
      shared/api/       ← apiClient с JWT interceptor
      shared/ui/        ← 8 компонентов (Button, Input, Table, Pagination, Badge, Spinner, Tabs)
```

### Phase 1: Backend Skeleton
**Файлы:** `go.mod`, `config.go`, `logger.go`, `postgres_client.go`, `catalog_migrations.go`, `admin_migrations.go`, `main.go`
- Go module `keepstar-admin` с зависимостями: pgx/v5, golang-jwt/v5, bcrypt, pgvector-go, godotenv
- Postgres client — копия паттерна из main backend (pool 10/2, 1h lifetime)
- `RunCatalogMigrations()` — идемпотентные CREATE IF NOT EXISTS (гарантия catalog-схемы)
- `RunAdminMigrations()` — admin.admin_users, admin.import_jobs, уникальный индекс products(tenant_id, master_product_id)
- `.env` читается из `../../project/.env` (общий DATABASE_URL)
- Health endpoint: `GET /health → {"status":"ok"}`

### Phase 2: Auth
**Файлы:** `domain/admin_user.go`, `domain/errors.go`, `ports/auth_port.go`, `adapters/postgres/auth_adapter.go`, `usecases/auth.go`, `handlers/handler_auth.go`, `handlers/middleware_auth.go`

- **AdminUser**: id, email, passwordHash (bcrypt, never serialized), tenantID, role (owner|editor)
- **AuthAdapter**: GetUserByEmail, GetUserByID, CreateUser, EmailExists — все с `pgx.ErrNoRows` handling
- **Signup flow**: validate → check email unique → CreateTenant (slug=slugify(companyName), type=retailer, settings={currency:RUB}) → bcrypt hash → CreateUser → JWT
- **Login flow**: GetUserByEmail → bcrypt.Compare → JWT
- **JWT**: claims {uid, tid, role}, expiry 24h, HS256
- **AuthMiddleware**: Bearer token → parse claims → inject uid/tid/role в context
- **Slugify**: unicode-safe, non-alphanumeric → dash

### Phase 3: Catalog CRUD
**Файлы:** `domain/product.go`, `domain/category.go`, `domain/tenant.go`, `ports/catalog_port.go`, `adapters/postgres/catalog_adapter.go`, `usecases/products.go`, `handlers/handler_products.go`

- **AdminCatalogPort**: 14 методов (tenant CRUD, products list/get/update, categories, import upserts, post-import)
- **ListProducts**: JOIN products + master_products + categories, ILIKE search по name/sku/brand, category filter, пагинация, merge master→product, formatPrice
- **GetProduct**: single product с full merge + formatPrice
- **UpdateProduct**: dynamic SET builder (name, description, price, stock, rating), tenant-scoped
- **GetCategories**: all categories sorted by name
- **AdminProductFilter**: Search, CategoryID, Limit, Offset
- **ProductUpdate**: pointer fields для partial update

### Phase 4: Import
**Файлы:** `domain/import_job.go`, `ports/import_port.go`, `adapters/postgres/import_adapter.go`, `usecases/import.go`, `handlers/handler_import.go`

- **ImportJob**: id, tenantID, fileName, status (pending→processing→completed|failed), progress counters, errors JSONB
- **Upload flow**: validate → CreateImportJob(pending) → `go processImport()` → return {jobId, status, totalItems}
- **Background processing**: для каждого item: GetOrCreateCategory → UpsertMasterProduct (ON CONFLICT sku) → UpsertProductListing (ON CONFLICT tenant_id+master_product_id). Progress update каждые 10 items
- **Post-import goroutine**: embedding generation (batch по 100, text = name+desc+brand+category+attrs) → SeedEmbedding (pgvector) → GenerateCatalogDigest (SQL aggregate → JSON → UPDATE tenants)
- **Polling**: GET `/catalog/import/{id}` — клиент поллит до status=completed|failed
- **Error handling**: per-item errors appended to JSONB array, errorCount tracked, all-fail → status=failed

### Phase 5: Settings
**Файлы:** `domain/tenant_settings.go`, `usecases/settings.go`, `handlers/handler_settings.go`

- **TenantSettings**: theme, currency, geoCountry (ISO 3166-1), geoRegion, enrichCrossData
- Хранится в существующем `catalog.tenants.settings` JSONB — без новых миграций
- GET: read tenant → unmarshal settings. PUT: marshal → UPDATE settings

### Phase 6: Frontend
**25 src файлов**, React 19 + Vite 7 + react-router-dom

- **AuthProvider**: JWT в localStorage, login/signup/logout, auto-check `/auth/me` on mount
- **apiClient**: base `/admin/api`, JWT interceptor, 401 → clearToken → redirect /login
- **Login/Signup**: email + password (+ companyName for signup), error handling, redirect /catalog
- **DashboardLayout**: sidebar (Catalog, Import, Settings icons via lucide-react), user email, logout
- **ProductsPage**: таблица (image thumb, name, brand, category, price, stock), search bar, category filter dropdown, пагинация 25/page, click → detail
- **ProductDetailPage**: image preview, meta (brand, category, SKU), edit form (name, description, price, stock, rating) → PUT
- **ImportPage**: file input (.json), preview первых 5 items, upload → progress bar с polling каждые 2 сек, history table с Badge статусов
- **SettingsPage**: country dropdown (8 стран), region input, enrichment toggle, save
- **UI kit**: Button (primary/secondary/ghost/danger), Input (label+error), Table (sortable columns, empty state, clickable rows), Pagination, Badge (status colors), Spinner, Tabs

### API Routes (10 endpoints)

| Method | Path | Auth | Описание |
|--------|------|------|----------|
| GET | `/health` | — | Health check |
| POST | `/admin/api/auth/signup` | — | Регистрация |
| POST | `/admin/api/auth/login` | — | Логин |
| GET | `/admin/api/auth/me` | JWT | Текущий юзер |
| GET | `/admin/api/products` | JWT | Список товаров |
| GET | `/admin/api/products/{id}` | JWT | Детали товара |
| PUT | `/admin/api/products/{id}` | JWT | Обновить товар |
| GET | `/admin/api/categories` | JWT | Категории |
| POST | `/admin/api/catalog/import` | JWT | Загрузить каталог |
| GET | `/admin/api/catalog/import/{id}` | JWT | Статус импорта |
| GET | `/admin/api/catalog/imports` | JWT | История импортов |
| GET | `/admin/api/settings` | JWT | Настройки тенанта |
| PUT | `/admin/api/settings` | JWT | Обновить настройки |

### DB Migrations

```sql
-- admin schema (new)
CREATE TABLE admin.admin_users (id UUID PK, email UNIQUE, password_hash, tenant_id FK, role, timestamps)
CREATE TABLE admin.import_jobs (id UUID PK, tenant_id FK, file_name, status, counters, errors JSONB, timestamps)
CREATE UNIQUE INDEX idx_catalog_products_tenant_master ON catalog.products(tenant_id, master_product_id)

-- catalog schema (idempotent, same as main backend)
catalog.tenants, catalog.categories, catalog.master_products, catalog.products, pgvector extension
```

### DevOps: Commands + Scripts

**Claude commands** (`.claude/commands/`):
- `/start_admin` — запуск админки (backend :8081 + frontend :5174)
- `/stop_admin` — остановка админки
- `/start_all` — запуск чата + админки
- `/stop_all` — остановка всего

**Shell scripts** (`scripts/`):
- `start.sh`, `stop.sh` — чат
- `start_admin.sh`, `stop_admin.sh` — админка
- `start_all.sh`, `stop_all.sh` — всё

### Port Allocation

| Сервис | Порт | Конфликт |
|--------|------|----------|
| Chat backend | :8080 | — |
| Chat frontend | :5173 | — |
| Dev inspector | :3457 | — |
| Admin backend | :8081 | — |
| Admin frontend | :5174 | — |

## Паттерны из main backend

| Компонент | Стратегия |
|-----------|-----------|
| postgres.Client | Копия (pool 10/2) |
| Config | Копия + JWTSecret, AdminPort |
| Migration pattern | Тот же RunXxxMigrations() |
| Embedding client | Копия (OpenAI raw HTTP) |
| CORS middleware | Копия + PUT/DELETE + Authorization header |
| writeJSON | Копия + writeError helper |
| mergeProductWithMaster | Адаптация (inline в ListProducts/GetProduct) |
| formatPrice | Копия |
| Embedding text builder | Копия в import postImport() |

## Что НЕ сделано (backlog)

- Тесты (unit, integration)
- Drag-and-drop upload (только file input)
- Pagination в истории импортов
- Role-based access (editor vs owner restrictions)
- Password reset / email verification
- CSV/Excel import (только JSON)
- Product image upload (только URL)
- Audit log
