import { WidgetRenderer } from '../../entities/widget/WidgetRenderer';
import { WidgetType } from '../../entities/widget/widgetModel';
import { AtomType } from '../../entities/atom/atomModel';
import './ProductGrid.css';

function productToWidget(product) {
  const atoms = [
    {
      type: AtomType.IMAGE,
      value: product.images?.[0] || '',
      meta: { label: product.name },
    },
    {
      type: AtomType.TEXT,
      value: product.name,
      meta: { style: 'title' },
    },
  ];

  if (product.brand) {
    atoms.push({
      type: AtomType.TEXT,
      value: product.brand,
      meta: { style: 'subtitle' },
    });
  }

  atoms.push({
    type: AtomType.PRICE,
    value: product.priceFormatted,
    meta: { currency: product.currency },
  });

  if (product.rating) {
    atoms.push({
      type: AtomType.RATING,
      value: product.rating,
    });
  }

  atoms.push({
    type: AtomType.BUTTON,
    value: 'Add to cart',
    meta: { action: 'add_to_cart' },
  });

  return {
    id: product.id,
    type: WidgetType.PRODUCT_CARD,
    atoms,
    meta: { productId: product.id },
  };
}

export function ProductGrid({ products, isLoading, error }) {
  if (isLoading) {
    return (
      <div className="product-grid-loading">
        Loading products...
      </div>
    );
  }

  if (error) {
    return (
      <div className="product-grid-error">
        Error: {error}
      </div>
    );
  }

  if (!products || products.length === 0) {
    return (
      <div className="product-grid-empty">
        No products found
      </div>
    );
  }

  return (
    <div className="product-grid">
      {products.map(product => (
        <WidgetRenderer key={product.id} widget={productToWidget(product)} />
      ))}
    </div>
  );
}
