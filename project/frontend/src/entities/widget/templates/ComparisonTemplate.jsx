import { AtomRenderer } from '../../atom/AtomRenderer';
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
};

function getFieldLabel(fieldName) {
  return FIELD_LABELS[fieldName] || fieldName;
}

// Collect unique fieldNames across all widgets, preserving order from the first widget
function collectFieldNames(widgets) {
  const seen = new Set();
  const ordered = [];

  for (const widget of widgets) {
    for (const atom of widget.atoms || []) {
      const key = atom.fieldName || atom.slot || 'unknown';
      if (!seen.has(key)) {
        seen.add(key);
        ordered.push(key);
      }
    }
  }

  return ordered;
}

// Get atom for a given fieldName from a widget's atoms
function getAtomByField(widget, fieldName) {
  return (widget.atoms || []).find(
    (a) => (a.fieldName || a.slot) === fieldName
  );
}

export function ComparisonTemplate({ widgets = [], onWidgetClick }) {
  if (widgets.length === 0) return null;

  const fieldNames = collectFieldNames(widgets);

  return (
    <div className="comparison-wrapper">
      <div
        className="comparison-table"
        style={{ gridTemplateColumns: `120px repeat(${widgets.length}, minmax(150px, 1fr))` }}
      >
        {/* Header row: empty corner + product names */}
        <div className="comparison-cell comparison-corner" />
        {widgets.map((widget) => {
          const nameAtom = getAtomByField(widget, 'name');
          return (
            <div
              key={widget.id}
              className="comparison-cell comparison-header"
              onClick={() =>
                onWidgetClick?.(widget.entityRef?.type, widget.entityRef?.id)
              }
            >
              {nameAtom ? nameAtom.value : `Товар`}
            </div>
          );
        })}

        {/* Data rows */}
        {fieldNames.map((fieldName) => (
          <ComparisonRow
            key={fieldName}
            fieldName={fieldName}
            widgets={widgets}
          />
        ))}
      </div>
    </div>
  );
}

function ComparisonRow({ fieldName, widgets }) {
  return (
    <>
      <div className="comparison-cell comparison-label">
        {getFieldLabel(fieldName)}
      </div>
      {widgets.map((widget) => {
        const atom = getAtomByField(widget, fieldName);
        return (
          <div key={widget.id} className="comparison-cell comparison-value">
            {atom ? <ComparisonCell atom={atom} /> : <span className="comparison-empty">—</span>}
          </div>
        );
      })}
    </>
  );
}

function ComparisonCell({ atom }) {
  // Special handling for images — show thumbnail
  if (atom.type === 'image') {
    const images = normalizeImages(atom.value);
    if (images.length === 0) return <span className="comparison-empty">—</span>;
    return (
      <img
        src={images[0]}
        alt=""
        className="comparison-thumbnail"
      />
    );
  }

  return <AtomRenderer atom={atom} />;
}
