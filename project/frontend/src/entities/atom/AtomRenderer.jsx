import { AtomType } from './atomModel';
import './Atom.css';

export function AtomRenderer({ atom }) {
  const style = atom.meta?.style || '';
  const format = atom.meta?.format || '';
  const size = atom.meta?.size || 'medium';
  const variant = atom.meta?.variant || '';

  switch (atom.type) {
    case AtomType.TEXT:
      return (
        <span className={`atom-text ${style ? `style-${style}` : ''}`}>
          {atom.value}
        </span>
      );

    case AtomType.NUMBER:
      return (
        <span className={`atom-number ${format ? `format-${format}` : ''}`}>
          {formatNumber(atom.value, format)}
        </span>
      );

    case AtomType.PRICE:
      return (
        <span className="atom-price">
          {atom.meta?.currency || '$'}{atom.value}
        </span>
      );

    case AtomType.IMAGE:
      return (
        <img
          className={`atom-image size-${size}`}
          src={atom.value}
          alt={atom.meta?.label || ''}
        />
      );

    case AtomType.RATING: {
      const stars = Math.round(atom.value);
      return (
        <span className="atom-rating">
          {'★'.repeat(stars)}{'☆'.repeat(5 - stars)}
        </span>
      );
    }

    case AtomType.BADGE:
      return (
        <span className={`atom-badge ${variant ? `variant-${variant}` : ''}`}>
          {atom.value}
        </span>
      );

    case AtomType.BUTTON:
      return (
        <button
          className="atom-button"
          data-action={atom.meta?.action}
          onClick={() => handleAction(atom.meta?.action)}
        >
          {atom.value}
        </button>
      );

    case AtomType.ICON:
      return <span className="atom-icon">{atom.value}</span>;

    case AtomType.DIVIDER:
      return <div className="atom-divider" />;

    case AtomType.PROGRESS:
      return (
        <div className="atom-progress">
          <div
            className="atom-progress-bar"
            style={{ width: `${atom.value}%` }}
          />
        </div>
      );

    default:
      return <span>{String(atom.value)}</span>;
  }
}

function formatNumber(value, format) {
  if (format === 'currency') return value.toLocaleString();
  if (format === 'percent') return value;
  if (format === 'compact') return compactNumber(value);
  return value;
}

function compactNumber(num) {
  if (num >= 1000000) return (num / 1000000).toFixed(1) + 'M';
  if (num >= 1000) return (num / 1000).toFixed(1) + 'K';
  return num;
}

function handleAction(action) {
  // TODO: dispatch action to parent
  console.log('Widget action:', action);
}
