# Feature: Multi-tenant Product Catalog with Master Product Standardization

## Feature Description

A multi-tenant product catalog system where brands can create canonical (master) products, and retailers/resellers can create their own listings that optionally link to master products. When linked, tenant-specific data (price, stock, custom images) merges with master product data, enabling consistent product representation across different sales channels while allowing per-tenant customization.

## Objective

Enable a multi-tenant e-commerce platform where:
- Brands own and maintain canonical master product definitions
- Retailers/resellers can list products with their own pricing and stock
- Products can be linked to masters for data inheritance
- API returns merged product data transparently
- Frontend displays products via ProductCard widget

## Expertise Context

Expertise used:
- **backend**: Hexagonal architecture patterns, domain entities, ports/adapters structure, PostgreSQL migrations, handler patterns
- **frontend**: Feature-sliced design, widget/atom rendering system, API client patterns, component composition

## Relevant Files

### Existing Files (Backend)
- `project/backend/cmd/server/main.go` - Bootstrap (add new adapters, handlers, routes)
- `project/backend/internal/domain/product_entity.go` - Existing Product entity (to be extended)
- `project/backend/internal/domain/domain_errors.go` - Add new error types
- `project/backend/internal/ports/search_port.go` - SearchPort interface (stub, to implement)
- `project/backend/internal/adapters/postgres/postgres_client.go` - PostgreSQL client
- `project/backend/internal/adapters/postgres/migrations.go` - Add catalog migrations
- `project/backend/internal/handlers/routes.go` - Add new routes

### Existing Files (Frontend)
- `project/frontend/src/shared/api/apiClient.js` - Add product API functions
- `project/frontend/src/entities/widget/WidgetRenderer.jsx` - ProductCard already implemented
- `project/frontend/src/entities/atom/AtomRenderer.jsx` - Atom rendering

### New Files (Backend)
- `project/backend/internal/domain/tenant_entity.go` - Tenant entity
- `project/backend/internal/domain/master_product_entity.go` - MasterProduct entity
- `project/backend/internal/domain/category_entity.go` - Category entity
- `project/backend/internal/ports/catalog_port.go` - CatalogPort interface
- `project/backend/internal/adapters/postgres/postgres_catalog.go` - CatalogPort implementation
- `project/backend/internal/adapters/postgres/catalog_migrations.go` - Catalog schema migrations
- `project/backend/internal/usecases/catalog_list_products.go` - ListProducts use case
- `project/backend/internal/usecases/catalog_get_product.go` - GetProduct use case
- `project/backend/internal/handlers/handler_catalog.go` - Catalog HTTP handler
- `project/backend/internal/handlers/middleware_tenant.go` - Tenant resolution middleware

### New Files (Frontend)
- `project/frontend/src/features/catalog/catalogModel.js` - Catalog state model
- `project/frontend/src/features/catalog/useCatalogProducts.js` - Products hook
- `project/frontend/src/features/catalog/ProductGrid.jsx` - Product grid component
- `project/frontend/src/features/catalog/ProductGrid.css` - Grid styles

## Step by Step Tasks

**IMPORTANT:** Execute strictly in order.

---

### 1. Create Domain Entities

Create new domain entities following existing patterns in `internal/domain/`.

**1.1 Create `tenant_entity.go`:**
```go
package domain

import "time"

type TenantType string

const (
    TenantTypeBrand    TenantType = "brand"
    TenantTypeRetailer TenantType = "retailer"
    TenantTypeReseller TenantType = "reseller"
)

type Tenant struct {
    ID        string            `json:"id"`
    Slug      string            `json:"slug"`
    Name      string            `json:"name"`
    Type      TenantType        `json:"type"`
    Settings  map[string]any    `json:"settings"`
    CreatedAt time.Time         `json:"createdAt"`
    UpdatedAt time.Time         `json:"updatedAt"`
}
```

**1.2 Create `category_entity.go`:**
```go
package domain

type Category struct {
    ID       string `json:"id"`
    Name     string `json:"name"`
    Slug     string `json:"slug"`
    ParentID string `json:"parentId,omitempty"`
}
```

**1.3 Create `master_product_entity.go`:**
```go
package domain

import "time"

type MasterProduct struct {
    ID            string            `json:"id"`
    SKU           string            `json:"sku"`
    Name          string            `json:"name"`
    Description   string            `json:"description"`
    Brand         string            `json:"brand"`
    CategoryID    string            `json:"categoryId"`
    Images        []string          `json:"images"`
    Attributes    map[string]any    `json:"attributes"`
    OwnerTenantID string            `json:"ownerTenantId"`
    CreatedAt     time.Time         `json:"createdAt"`
    UpdatedAt     time.Time         `json:"updatedAt"`
}
```

**1.4 Update `product_entity.go`** to support tenant context:
```go
// Add fields to existing Product struct:
TenantID        string   `json:"tenantId"`
MasterProductID string   `json:"masterProductId,omitempty"`
StockQuantity   int      `json:"stockQuantity"`
```

**1.5 Add errors to `domain_errors.go`:**
```go
var ErrTenantNotFound = &Error{Code: "TENANT_NOT_FOUND", Message: "tenant not found"}
var ErrCategoryNotFound = &Error{Code: "CATEGORY_NOT_FOUND", Message: "category not found"}
```

---

### 2. Create Catalog Port Interface

Create `internal/ports/catalog_port.go`:

```go
package ports

import (
    "context"
    "backend/internal/domain"
)

type ProductFilter struct {
    CategoryID string
    Brand      string
    MinPrice   int
    MaxPrice   int
    Search     string
    Limit      int
    Offset     int
}

type CatalogPort interface {
    // Tenant operations
    GetTenantBySlug(ctx context.Context, slug string) (*domain.Tenant, error)

    // Category operations
    GetCategories(ctx context.Context) ([]domain.Category, error)

    // Master product operations
    GetMasterProduct(ctx context.Context, id string) (*domain.MasterProduct, error)

    // Tenant product operations
    ListProducts(ctx context.Context, tenantID string, filter ProductFilter) ([]domain.Product, error)
    GetProduct(ctx context.Context, tenantID string, productID string) (*domain.Product, error)
}
```

---

### 3. Create Database Migrations

Create `internal/adapters/postgres/catalog_migrations.go`:

**Schema: `catalog`** (separate from chat schema)

Tables:
1. `catalog.tenants` - id, slug (unique), name, type, settings (jsonb), created_at, updated_at
2. `catalog.categories` - id, name, slug, parent_id (self-reference), created_at
3. `catalog.master_products` - id, sku (unique), name, description, brand, category_id (FK), images (jsonb), attributes (jsonb), owner_tenant_id (FK), created_at, updated_at
4. `catalog.products` - id, tenant_id (FK), master_product_id (nullable FK), name, description, price (integer, kopecks), currency, stock_quantity, images (jsonb), created_at, updated_at

Indexes:
- `catalog.tenants` - unique on slug
- `catalog.products` - on tenant_id, master_product_id
- `catalog.master_products` - on category_id, owner_tenant_id, sku

---

### 4. Implement Catalog Adapter

Create `internal/adapters/postgres/postgres_catalog.go`:

**4.1 CatalogAdapter struct:**
```go
type CatalogAdapter struct {
    client *Client
}

func NewCatalogAdapter(client *Client) *CatalogAdapter
```

**4.2 GetTenantBySlug:**
- Query: `SELECT * FROM catalog.tenants WHERE slug = $1`
- Return: `*domain.Tenant` or `ErrTenantNotFound`

**4.3 ListProducts with merge logic:**
- Query products for tenant
- For products with `master_product_id`, fetch master and merge:
  - Use master's: name, description, images, attributes (if tenant's are empty)
  - Use tenant's: price, currency, stock_quantity
  - Merge images: tenant's images extend master's if both exist
- Apply filters (category, brand, price range, search)
- Return merged `[]domain.Product`

**4.4 GetProduct with merge:**
- Similar merge logic for single product
- Return merged `*domain.Product` or `ErrProductNotFound`

---

### 5. Create Use Cases

**5.1 Create `internal/usecases/catalog_list_products.go`:**

```go
type ListProductsUseCase struct {
    catalog ports.CatalogPort
}

type ListProductsRequest struct {
    TenantSlug string
    Filter     ports.ProductFilter
}

type ListProductsResponse struct {
    Products []domain.Product
    Total    int
}

func (uc *ListProductsUseCase) Execute(ctx, req) (*ListProductsResponse, error)
```

Flow:
1. Resolve tenant by slug
2. Call catalog.ListProducts with tenant ID
3. Return products

**5.2 Create `internal/usecases/catalog_get_product.go`:**

```go
type GetProductUseCase struct {
    catalog ports.CatalogPort
}

type GetProductRequest struct {
    TenantSlug string
    ProductID  string
}

func (uc *GetProductUseCase) Execute(ctx, req) (*domain.Product, error)
```

---

### 6. Create Tenant Middleware

Create `internal/handlers/middleware_tenant.go`:

```go
type TenantMiddleware struct {
    catalog ports.CatalogPort
}

func (m *TenantMiddleware) ResolveTenant(next http.Handler) http.Handler
```

- Extract tenant slug from URL path (`/api/v1/tenants/{slug}/...`)
- Validate tenant exists via catalog port
- Store tenant in request context
- Return 404 if tenant not found

---

### 7. Create Catalog Handler

Create `internal/handlers/handler_catalog.go`:

**7.1 CatalogHandler struct:**
```go
type CatalogHandler struct {
    listProducts *usecases.ListProductsUseCase
    getProduct   *usecases.GetProductUseCase
}
```

**7.2 HandleListProducts:**
- Endpoint: `GET /api/v1/tenants/{slug}/products`
- Query params: `category`, `brand`, `minPrice`, `maxPrice`, `search`, `limit`, `offset`
- Response: `{ products: [], total: int }`

**7.3 HandleGetProduct:**
- Endpoint: `GET /api/v1/tenants/{slug}/products/{id}`
- Response: Full product object (merged with master)

**7.4 Response DTOs:**
```go
type ProductResponse struct {
    ID            string         `json:"id"`
    Name          string         `json:"name"`
    Description   string         `json:"description"`
    Price         int            `json:"price"`
    PriceFormatted string        `json:"priceFormatted"` // e.g., "12 990 ₽"
    Currency      string         `json:"currency"`
    Images        []string       `json:"images"`
    Rating        float64        `json:"rating"`
    StockQuantity int            `json:"stockQuantity"`
    Brand         string         `json:"brand"`
    Category      string         `json:"category"`
    Attributes    map[string]any `json:"attributes"`
}
```

---

### 8. Register Routes and Bootstrap

**8.1 Update `routes.go`:**
```go
// Add catalog routes with tenant middleware
mux.Handle("GET /api/v1/tenants/{slug}/products", tenantMw(catalog.HandleListProducts))
mux.Handle("GET /api/v1/tenants/{slug}/products/{id}", tenantMw(catalog.HandleGetProduct))
```

**8.2 Update `main.go`:**
- Initialize CatalogAdapter
- Run catalog migrations
- Initialize use cases (ListProducts, GetProduct)
- Initialize CatalogHandler
- Initialize TenantMiddleware
- Register routes with middleware

---

### 9. Create Seed Data

Create `internal/adapters/postgres/catalog_seed.go`:

**Tenants:**
```go
{Slug: "nike", Name: "Nike Official", Type: TenantTypeBrand}
{Slug: "sportmaster", Name: "Sportmaster", Type: TenantTypeRetailer}
```

**Categories:**
```go
{Name: "Sneakers", Slug: "sneakers"}
{Name: "Running", Slug: "running", ParentID: sneakersID}
{Name: "Basketball", Slug: "basketball", ParentID: sneakersID}
```

**Master Products (5-10 sneakers):**
```go
{
    SKU: "NIKE-AIR-MAX-90",
    Name: "Nike Air Max 90",
    Brand: "Nike",
    Images: ["https://images.unsplash.com/..."],
    OwnerTenantID: nikeID,
}
// ... more products with Unsplash images
```

**Products (tenant listings):**
```go
// Nike's own listing
{TenantID: nikeID, MasterProductID: airMax90ID, Price: 1299000, Currency: "RUB"}
// Sportmaster's listing (different price)
{TenantID: sportmasterID, MasterProductID: airMax90ID, Price: 1199000, Currency: "RUB"}
```

**Run seed on first migration** (check if tenants table empty).

---

### 10. Frontend API Integration

**10.1 Update `apiClient.js`:**

```javascript
export async function getProducts(tenantSlug, filters = {}) {
  const params = new URLSearchParams();
  if (filters.category) params.set('category', filters.category);
  if (filters.search) params.set('search', filters.search);
  if (filters.limit) params.set('limit', filters.limit);
  if (filters.offset) params.set('offset', filters.offset);

  const url = `${API_BASE_URL}/tenants/${tenantSlug}/products?${params}`;
  const response = await fetch(url);

  if (!response.ok) {
    throw new Error(`API error: ${response.status}`);
  }

  return response.json();
}

export async function getProduct(tenantSlug, productId) {
  const response = await fetch(`${API_BASE_URL}/tenants/${tenantSlug}/products/${productId}`);

  if (response.status === 404) {
    return null;
  }

  if (!response.ok) {
    throw new Error(`API error: ${response.status}`);
  }

  return response.json();
}
```

---

### 11. Frontend Catalog Feature

**11.1 Create `features/catalog/catalogModel.js`:**
```javascript
export function createInitialCatalogState() {
  return {
    products: [],
    isLoading: false,
    error: null,
    filters: {},
  };
}
```

**11.2 Create `features/catalog/useCatalogProducts.js`:**
```javascript
export function useCatalogProducts(tenantSlug) {
  const [state, setState] = useState(createInitialCatalogState);

  const loadProducts = useCallback(async (filters = {}) => {
    setState(s => ({ ...s, isLoading: true, error: null }));
    try {
      const data = await getProducts(tenantSlug, filters);
      setState(s => ({ ...s, products: data.products, isLoading: false }));
    } catch (err) {
      setState(s => ({ ...s, error: err.message, isLoading: false }));
    }
  }, [tenantSlug]);

  return { ...state, loadProducts };
}
```

**11.3 Create `features/catalog/ProductGrid.jsx`:**
```jsx
import { WidgetRenderer } from '../../entities/widget/WidgetRenderer';
import { WidgetType } from '../../entities/widget/widgetModel';
import { AtomType } from '../../entities/atom/atomModel';

function productToWidget(product) {
  return {
    id: product.id,
    type: WidgetType.PRODUCT_CARD,
    atoms: [
      { type: AtomType.IMAGE, value: product.images[0], meta: { label: product.name } },
      { type: AtomType.TEXT, value: product.name, meta: { style: 'title' } },
      { type: AtomType.TEXT, value: product.brand, meta: { style: 'subtitle' } },
      { type: AtomType.PRICE, value: product.priceFormatted, meta: { currency: product.currency } },
      { type: AtomType.RATING, value: product.rating },
      { type: AtomType.BUTTON, value: 'Add to cart', meta: { action: 'add_to_cart' } },
    ],
    meta: { productId: product.id },
  };
}

export function ProductGrid({ products }) {
  return (
    <div className="product-grid">
      {products.map(product => (
        <WidgetRenderer key={product.id} widget={productToWidget(product)} />
      ))}
    </div>
  );
}
```

**11.4 Create `features/catalog/ProductGrid.css`:**
```css
.product-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(250px, 1fr));
  gap: 20px;
  padding: 20px;
}
```

---

### 12. Validation

Run all validation commands:

```bash
# Backend build
cd project/backend && go build ./...

# Backend tests
cd project/backend && go test ./...

# Frontend build
cd project/frontend && npm run build

# Frontend lint
cd project/frontend && npm run lint
```

---

## Validation Commands

From `ADW/adw.yaml`:

| Name | Path | Command | Required |
|------|------|---------|----------|
| Backend build | project/backend | `go build ./...` | Yes |
| Backend tests | project/backend | `go test ./...` | No |
| Frontend build | project/frontend | `npm run build` | Yes |
| Frontend lint | project/frontend | `npm run lint` | No |

---

## Acceptance Criteria

- [ ] Tenant entity created with slug, name, type, settings
- [ ] MasterProduct entity created with canonical product fields
- [ ] Product entity extended with tenant context and master link
- [ ] Category entity created with tree structure support
- [ ] CatalogPort interface defined with all operations
- [ ] PostgreSQL migrations create catalog schema and tables
- [ ] CatalogAdapter implements product merging logic
- [ ] GET /api/v1/tenants/{slug}/products returns filtered products
- [ ] GET /api/v1/tenants/{slug}/products/{id} returns merged product
- [ ] Tenant middleware validates tenant slug
- [ ] Seed data includes Nike and Sportmaster tenants
- [ ] Seed data includes 5-10 master sneaker products
- [ ] Seed data includes tenant-specific listings with different prices
- [ ] Frontend can fetch products via API
- [ ] ProductGrid displays products using existing widget system
- [ ] Price displayed in formatted RUB (e.g., "12 990 ₽")
- [ ] Backend builds successfully
- [ ] Frontend builds successfully

---

## Notes

- **Price Storage**: Store in kopecks (integer) to avoid floating-point issues. 1299000 kopecks = 12 990 ₽
- **Price Formatting**: Backend formats price string for display (`12 990 ₽`)
- **Image URLs**: Use Unsplash for seed data (no file upload needed)
- **Merge Strategy**: Tenant data overrides master data; images can extend
- **Schema Isolation**: Use `catalog` schema to separate from `public` chat tables
- **Graceful Degradation**: If no master link, product uses its own fields only
- **Widget Reuse**: Leverage existing ProductCard widget and atom system
