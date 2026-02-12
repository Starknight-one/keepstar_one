# Done: Catalog Evolution — Stock Table + Services + Tags

**Дата:** 2026-02-12
**Ветка:** `feature/catalog-evolution`
**Статус:** Реализовано, компилируется, ожидает проверки миграций на живой БД

## Проблема

Каталог работал, но:
1. Stock жил как `stock_quantity` колонка в `products` — не масштабируется под высокую write-нагрузку
2. Services (услуги) были в domain entities, но не имели таблиц в БД — нельзя импортировать/искать
3. Tags (коллекции) — ни поля, ни индекса — тенанты не могли группировать товары

## Что сделано

### Phase 1: Stock Table

**Миграция:**
- Новая таблица `catalog.stock` с PK `(tenant_id, product_id)`, поля `quantity`, `reserved`, `updated_at`
- Seed-миграция: перенос `stock_quantity` из `products` в `stock`
- Колонка `stock_quantity` в products НЕ удалена (backward compat)

**Chat backend:**
- `ListProducts`, `GetProduct`, `VectorSearch` — `LEFT JOIN catalog.stock` вместо чтения `p.stock_quantity`, `COALESCE(s.quantity, 0)`
- Новый метод `GetStock(ctx, tenantID, productID)` в порте и адаптере

**Admin backend:**
- `POST /admin/api/stock/bulk` — bulk update стоков по SKU
- `StockUseCase` + `StockHandler`
- `BulkUpdateStock` в адаптере: resolve SKU → master_product_id → product_id → upsert в `catalog.stock`

### Phase 2: Services Tables

**Миграция:**
- `catalog.master_services` — аналог `master_products` с `duration`, `provider`, `embedding vector(384)`, HNSW index
- `catalog.services` — аналог `products` с `availability`, unique constraint `(tenant_id, master_service_id)`
- Все нужные индексы: tenant, master, category, sku, embedding

**Chat backend domain:**
- `MasterService` entity (новый файл `master_service_entity.go`)
- `Service` entity обновлён: добавлены `MasterServiceID`, `Tags`

**Chat backend adapter:**
- `ListServices`, `GetService`, `VectorSearchServices` — полные реализации, паттерн из products
- `mergeServiceWithMaster` helper
- `SeedServiceEmbedding`, `GetMasterServicesWithoutEmbedding`

**Chat backend port:**
- 6 новых методов в `CatalogPort`

**Chat backend tools:**
- `tool_catalog_search.go`: поле `entity_type` (product/service/all)
- Keyword + vector поиск по services после products
- `rrfMergeServices` — RRF merge для услуг
- `catalogExtractServiceFields` — extraction helper
- `StateData.Services` заполняется результатами

**Admin backend:**
- `MasterService` + `Service` domain entities
- `UpsertMasterService`, `UpsertServiceListing`, `ListServices`, `GetService`, `UpdateService` в адаптере
- `GetMasterServicesWithoutEmbedding`, `SeedServiceEmbedding`
- Import расширен: `ImportItem.Type` = "product"/"service", `processProductItem` / `processServiceItem`
- `postImport` разделён на `embedProducts` + `embedServices`

### Phase 3: Tags

**Миграция:**
- `ALTER TABLE catalog.products ADD COLUMN tags JSONB DEFAULT '[]'`
- `ALTER TABLE catalog.services ADD COLUMN tags JSONB DEFAULT '[]'`
- GIN индексы на обеих таблицах

**Chat backend:**
- Все product/service queries сканируют `tags`

**Admin backend:**
- `UpsertProductListing` / `UpsertServiceListing` пишут tags
- `ListProducts` / `GetProduct` читают tags
- Import пишет tags из ImportItem

## Файлы изменены

### Chat Backend (project/backend/)

| Файл | Действие |
|------|----------|
| `internal/domain/stock_entity.go` | NEW — Stock struct |
| `internal/domain/master_service_entity.go` | NEW — MasterService struct |
| `internal/domain/service_entity.go` | UPDATE — добавлены MasterServiceID, Tags |
| `internal/ports/catalog_port.go` | UPDATE — 6 новых методов (stock, services) |
| `internal/adapters/postgres/catalog_migrations.go` | UPDATE — 4 новые миграции |
| `internal/adapters/postgres/postgres_catalog.go` | UPDATE — stock JOIN, services CRUD, tags scan |
| `internal/tools/tool_catalog_search.go` | UPDATE — entity_type, services search, RRF merge |

### Admin Backend (project_admin/backend/)

| Файл | Действие |
|------|----------|
| `internal/domain/stock_update.go` | NEW — StockUpdate struct |
| `internal/domain/service.go` | NEW — Service + MasterService structs |
| `internal/domain/product.go` | UPDATE — добавлено Tags field |
| `internal/ports/catalog_port.go` | UPDATE — services + stock bulk методы |
| `internal/adapters/postgres/catalog_migrations.go` | UPDATE — те же 4 миграции |
| `internal/adapters/postgres/catalog_adapter.go` | UPDATE — services CRUD, stock bulk, tags r/w |
| `internal/handlers/handler_stock.go` | NEW — POST /admin/api/stock/bulk |
| `internal/usecases/stock.go` | NEW — StockUseCase |
| `internal/usecases/import.go` | UPDATE — services support, embedServices |
| `cmd/server/main.go` | UPDATE — stock route registration |

## SQL Schema (новые таблицы)

```sql
-- Stock (отдельная таблица)
catalog.stock (tenant_id, product_id, quantity, reserved, updated_at)
  PK: (tenant_id, product_id)

-- Master Services (аналог master_products)
catalog.master_services (id, sku, name, description, brand, category_id, images, attributes, duration, provider, owner_tenant_id, embedding, created_at, updated_at)
  HNSW index on embedding

-- Services (аналог products)
catalog.services (id, tenant_id, master_service_id, name, description, price, currency, rating, images, tags, availability, created_at, updated_at)
  Unique: (tenant_id, master_service_id)
```

## Верификация

- `go build ./...` — оба проекта компилируются
- Миграции выполнятся автоматически при старте серверов
- Тесты не запускались (LLM-зависимости, в бэклоге на доработку)
