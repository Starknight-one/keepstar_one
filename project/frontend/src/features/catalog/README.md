# Catalog

Каталог товаров с мультитенантностью.

## Файлы

- `catalogModel.js` — Модель состояния каталога
- `useCatalogProducts.js` — Хук для загрузки товаров
- `ProductGrid.jsx` — Грид товаров через WidgetRenderer
- `ProductGrid.css` — Стили грида

## Использование

```jsx
import { useCatalogProducts } from './useCatalogProducts';
import { ProductGrid } from './ProductGrid';

function CatalogPage() {
  const { products, isLoading, error, loadProducts } = useCatalogProducts('nike');

  useEffect(() => {
    loadProducts({ limit: 20 });
  }, []);

  return (
    <ProductGrid
      products={products}
      isLoading={isLoading}
      error={error}
    />
  );
}
```

## useCatalogProducts

```js
const {
  products,      // Product[]
  total,         // int
  isLoading,     // boolean
  error,         // string | null
  filters,       // current filters
  loadProducts,  // (filters?) => Promise
  setFilters,    // (filters) => void
  resetFilters,  // () => void
} = useCatalogProducts(tenantSlug);
```

## ProductGrid

Рендерит товары через `WidgetRenderer` с типом `PRODUCT_CARD`.
Конвертирует Product в Widget с атомами: IMAGE, TEXT, PRICE, RATING, BUTTON.
