import { AtomV2Renderer } from './AtomV2Renderer';
import { normalizeImages } from './templateUtils';
import './ComparisonTemplate.css';

// Human-readable labels for field names
const FIELD_LABELS = {
  images: 'Фото',
  name: 'Название',
  brand: 'Бренд',
  category: 'Категория',
  price: 'Цена',
  rating: 'Рейтинг',
  description: 'Описание',
  tags: 'Теги',
  stockQuantity: 'В наличии',
  attributes: 'Характеристики',
  provider: 'Провайдер',
  duration: 'Длительность',
  availability: 'Доступность',
};

function getFieldLabel(fieldName) {
  return FIELD_LABELS[fieldName] || fieldName;
}

// Get atoms array
function getAtoms(widget) {
  return widget.atomsV2 || widget.atoms || [];
}

// Collect unique fieldNames across all widgets, preserving order from first widget
// Skip 'images' and 'name' — they go into column headers
const HEADER_FIELDS = new Set(['images', 'name', 'brand']);

function collectFieldNames(widgets) {
  const seen = new Set();
  const ordered = [];

  for (const widget of widgets) {
    for (const atom of getAtoms(widget)) {
      const key = atom.fieldName || atom.slot || 'unknown';
      if (!seen.has(key) && !HEADER_FIELDS.has(key)) {
        seen.add(key);
        ordered.push(key);
      }
    }
  }

  return ordered;
}

// Get atom for a given fieldName from a widget's atoms
function getAtomByField(widget, fieldName) {
  return getAtoms(widget).find(
    (a) => (a.fieldName || a.slot) === fieldName
  );
}

// Detect boolean-like values for checkmark rendering
function isBooleanLike(value) {
  if (typeof value === 'boolean') return true;
  if (typeof value === 'string') {
    const v = value.toLowerCase().trim();
    return v === 'yes' || v === 'no' || v === 'true' || v === 'false' || v === 'да' || v === 'нет';
  }
  return false;
}

function isTruthy(value) {
  if (typeof value === 'boolean') return value;
  if (typeof value === 'number') return value > 0;
  if (typeof value === 'string') {
    const v = value.toLowerCase().trim();
    return v === 'yes' || v === 'true' || v === 'да';
  }
  return !!value;
}

// Determine featured widget (highest rating, or middle one)
function getFeaturedIndex(widgets) {
  if (widgets.length <= 1) return -1;

  // Try to find the one with highest rating
  let bestIdx = -1;
  let bestRating = -1;
  widgets.forEach((w, i) => {
    const ratingAtom = getAtomByField(w, 'rating');
    if (ratingAtom && typeof ratingAtom.value === 'number' && ratingAtom.value > bestRating) {
      bestRating = ratingAtom.value;
      bestIdx = i;
    }
  });

  // Fallback: middle widget
  if (bestIdx === -1) {
    bestIdx = Math.floor(widgets.length / 2);
  }

  return bestIdx;
}

export function ComparisonTemplate({ widgets = [] }) {
  if (widgets.length === 0) return null;

  const fieldNames = collectFieldNames(widgets);
  const featuredIdx = getFeaturedIndex(widgets);

  return (
    <div className="comparison-wrapper">
      <div className="comparison-table">
        {/* Label column */}
        <div className="comparison-column comparison-label-column">
          {/* Empty header area to align with product headers */}
          <div className="comparison-column-header comparison-label-header" />

          {/* Field labels */}
          {fieldNames.map((fieldName) => (
            <div key={fieldName} className="comparison-row-label">
              {getFieldLabel(fieldName)}
            </div>
          ))}

          {/* Empty CTA area */}
          <div className="comparison-cta-row" />
        </div>

        {/* Product columns */}
        {widgets.map((widget, idx) => {
          const isFeatured = idx === featuredIdx;
          const nameAtom = getAtomByField(widget, 'name');
          const brandAtom = getAtomByField(widget, 'brand');
          const imageAtom = getAtomByField(widget, 'images');
          const images = imageAtom ? normalizeImages(imageAtom.value) : [];
          return (
            <div
              key={widget.id}
              className={`comparison-column comparison-product-column${isFeatured ? ' comparison-featured' : ''}`}
            >
              {/* Column header: image + name + brand */}
              <div className="comparison-column-header">
                {isFeatured && (
                  <span className="comparison-popular-badge">Popular</span>
                )}
                {images.length > 0 && (
                  <img
                    src={images[0]}
                    alt={nameAtom?.value || ''}
                    className="comparison-header-image"
                  />
                )}
                <span className="comparison-header-name">
                  {nameAtom ? nameAtom.value : 'Товар'}
                </span>
                {brandAtom && (
                  <span className="comparison-header-brand">
                    {brandAtom.value}
                  </span>
                )}
              </div>

              {/* Field values */}
              {fieldNames.map((fieldName) => {
                const atom = getAtomByField(widget, fieldName);
                return (
                  <div key={fieldName} className="comparison-cell">
                    {atom ? (
                      <ComparisonCell atom={atom} isFeatured={isFeatured} />
                    ) : (
                      <span className="comparison-empty">—</span>
                    )}
                  </div>
                );
              })}

              {/* CTA row — read-only, no click handler */}
              <div className="comparison-cta-row">
                <button
                  className={`comparison-cta-btn${isFeatured ? ' comparison-cta-featured' : ''}`}
                  disabled
                >
                  Подробнее
                </button>
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}

function ComparisonCell({ atom, isFeatured }) {
  // Boolean-like values → checkmark/cross
  if (isBooleanLike(atom.value)) {
    return isTruthy(atom.value) ? (
      <span className="comparison-check">✓</span>
    ) : (
      <span className="comparison-cross">✗</span>
    );
  }

  // Stock quantity → checkmark if > 0
  if (atom.fieldName === 'stockQuantity' && typeof atom.value === 'number') {
    return atom.value > 0 ? (
      <span className="comparison-check">✓ {atom.value}</span>
    ) : (
      <span className="comparison-cross">✗</span>
    );
  }

  // Images — show thumbnail
  if (atom.type === 'image') {
    const images = normalizeImages(atom.value);
    if (images.length === 0) return <span className="comparison-empty">—</span>;
    return (
      <img src={images[0]} alt="" className="comparison-thumbnail" />
    );
  }

  return <AtomV2Renderer atom={atom} />;
}
