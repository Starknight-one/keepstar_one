# Admin Catalog — Curator-Driven Pivot, Этап 3 (dev-store seed code)

- **Branch:** `main`
- **Date (UTC):** 2026-04-27 18:18
- **Parent commit:** `cf7b837` (curator image-URL fix)
- **Active plan:** `docs/New features/admin_catalog_curator_pivot_2026-04-27.md`

## Context

Этап 3 — подготовить dev-store с тестовыми данными чтобы прогнать end-to-end через все этапы пайплайна (dump-to-staging → discover → harvester-lite → merge-agent). Пользователь зарелизил новую версию Shopify-приложения (`Keepstar one`, scopes: read+write для products, inventory, locations, orders, metaobjects, product_feeds, product_listings).

Код этапа полностью написан. Что осталось — действие пользователя: установить новую версию приложения в dev-store и предоставить access token (либо через reinstall + decrypt из БД, либо через Custom App `shpat_` token прямо в shop admin).

## What landed

### `internal/adapters/shopify/client.go` — write methods

Добавлены 4 публичных метода поверх существующего `graphqlRequest`:

| Метод | Что делает |
|---|---|
| `ProductCreate(ctx, shop, token, ProductCreateInput)` | Создаёт product через `productSet(synchronous: true)`. Поддерживает options (Color × Size × Material), variants (с SKU/price/barcode/inventory/option values/grams), metafields, image URLs (Shopify fetch'ает по `originalSource`), collection-attachment через `collectionsToJoin`. Возвращает GID |
| `CollectionCreate(ctx, shop, token, title, handle, descriptionHtml)` | Custom collection через `collectionCreate`. Возвращает GID для последующего assignment'а в ProductCreate |
| `ProductsDeleteAll(ctx, shop, token)` | Wipe всех продуктов (для `--reset`). Лист по 50 → per-product `productDelete`. Refuses non-`*.myshopify.com` domains |
| `CollectionsDeleteAll(ctx, shop, token)` | Wipe custom collections. Same pattern |

Структуры `ProductCreateInput`, `VariantInput`, `MetafieldInput` — Go-side shape для convenience, переводятся в Shopify GraphQL input изнутри метода.

### `cmd/seed-devstore/main.go` — orchestrator (~330 строк)

Наливает 25-30 продуктов покрывающих **8 сценариев** из плана:

| # | Сценарий | Продуктов | Что проверяет |
|---|---|---|---|
| 3.1 | Полный матч с heybabes-мастером (vendor+SKU cascade) | 3 | COSRX/MEDI-PEEL/The Saem с реальными vendor + SKU pattern |
| 3.2 | Матч + обогащение мастера новыми metafield'ами | 3 | Some By Mi / Fraijour / Ma:nyo с scent/spf/packaging metafields |
| 3.3 | Матч но у тенанта меньше данных (no metafields/images/desc) | 3 | Papa Recipe / Derma Factory / IsNtree с минимальными полями |
| 3.4 | Категория переименована (fuzzy category match) | (в 3.1-3.3) | Все cosmetics в коллекции "Skincare Solutions" вместо канонической "Face Care" |
| 3.5 | Нет матча, новая vertical (furniture) | 5 | Walnut chair, oak coffee table, brass lamp, linen armchair, marble side table |
| 3.6 | Multi-axis variants (Color × Size × Material) | 1 (6 variants) | Trail Runner Sneaker — Black/White/Navy × 9/10 × Mesh/Leather |
| 3.7 | Junk variants | 2 | Gift Wrap Service, Sample Sachet |
| 3.8 | Edge cases (no SKU/long title/empty metafields) | 3 | Minimal listing, very-long-SEO-title, generic lip balm w/o vendor |

**Total: ~20 products + 4 collections (Skincare Solutions, Bestsellers, Furniture, Footwear).**

Принимает env `SHOPIFY_SHOP` + `SHOPIFY_TOKEN`. Флаги:
- `--reset` — wipe products + collections перед seed'ом
- `--only` — comma-separated scenario IDs (заглушка под будущую фильтрацию)

### `cmd/decrypt-shopify-token/main.go` — utility

Достаёт offline access token из `admin.tenant_integrations.credentials_encrypted`, расшифровывает через `secretbox` (тот же пакет которым шифрует прод admin) и печатает plaintext в stdout. Для использования с seed-devstore:

```bash
SHOPIFY_TOKEN=$(ADMIN_ENCRYPTION_KEY=<base64> DATABASE_URL=... \
  go run ./cmd/decrypt-shopify-token -shop keepstar-neaqpan1.myshopify.com)
```

## Files changed

| File | Action |
|---|---|
| `project_admin/backend/internal/adapters/shopify/client.go` | EDIT (+~280 строк write-methods) |
| `project_admin/backend/cmd/seed-devstore/main.go` | NEW (~330 строк, 8 scenarios) |
| `project_admin/backend/cmd/decrypt-shopify-token/main.go` | NEW (~60 строк) |

## Verification

```
$ cd project_admin/backend && go build ./...
clean

$ go vet ./...
clean

$ go run ./cmd/seed-devstore --help
Usage of seed-devstore:
  -only string  scenarios filter (placeholder)
  -reset        delete all existing products and collections before seeding
```

Live seed-test пока **не запускался** — нет SHOPIFY_TOKEN от нового приложения. Когда будет — прогоним 1 раз `--reset` чтобы убрать старые 17 snowboard'ов, потом seed → проверим в Shopify Admin что 20 продуктов + 4 collections созданы корректно.

## Что нужно от пользователя

Любой из двух путей даёт `SHOPIFY_TOKEN`:

### Путь A — Custom App (быстрее, не требует Railway env)

1. Открой Shopify Admin: `https://keepstar-neaqpan1.myshopify.com/admin`
2. Apps → "Develop apps" (или "App and sales channel settings → Develop apps")
3. Create an app → "Keepstar Seed (local)"
4. Configuration → Admin API → Configure scopes → выбери:
   - `write_products`, `read_products`
   - `write_inventory`, `read_inventory`
   - `write_publications`, `read_publications`
   - `write_collections`, `read_collections` (если есть отдельный — для удаления)
   - `write_metaobjects`, `read_metaobjects`
5. Save → Install app → подтверди установку
6. API credentials → Admin API access token → Reveal token once → скопируй (формат `shpat_xxxxxxxxxxxx`)
7. Дай мне токен — запущу seed

### Путь B — Reinstall Partners-приложения через прод admin (правильнее long-term)

1. На прод admin (`https://admin-production-4ae4.up.railway.app`) зайди в /integrations
2. Disconnect существующую Shopify-интеграцию (если есть)
3. Connect → подключи новый "Keepstar one" v3 → пройди OAuth
4. Скинь мне `ADMIN_ENCRYPTION_KEY` (из Railway env vars admin-production) и я локально расшифрую токен через `cmd/decrypt-shopify-token`

Путь A проще и не требует Railway secret — рекомендую если просто хочешь побыстрее.

## Known gaps / next steps

- **`weight_value` в variant input** — указан в Go-структуре `VariantInput.WeightGrams`, но не передаётся в `productSet` mutation в текущей реализации (Shopify хочет это через `inventoryItem.measurement.weight` отдельным шагом). Для seed это не критично — variants создаются, weight можно дозаполнить отдельно. Когда будет нужно — добавим
- **`--only` filter** — заглушка под будущую фильтрацию по scenarioID
- **GTIN на heybabes-картах** — DB-проверка показала что у heybabes 979 master_products **нет GTIN** (только SKU + name + brand). Сценарий 3.1 поэтому тестирует не GTIN-exact, а vendor+SKU fuzzy cascade. Когда будет реальный GTIN-каталог — добавим отдельный сценарий
