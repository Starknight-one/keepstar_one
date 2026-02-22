import { AtomType, AtomSubtype, LEGACY_TYPE_TO_DISPLAY } from './atomModel';
import { log } from '../../shared/logger';
import './Atom.css';

// Named color palette
const COLOR_PALETTE = {
  green: '#22C55E',
  red: '#EF4444',
  blue: '#3B82F6',
  orange: '#F97316',
  purple: '#8B5CF6',
  gray: '#6B7280',
};

// Resolve named color or pass hex through
function resolveColor(color) {
  if (!color) return null;
  return COLOR_PALETTE[color.toLowerCase()] || color;
}

export function AtomRenderer({ atom, onClick }) {
  // Determine display: explicit display > legacy mapping > inferred from type/subtype
  const display = atom.display || LEGACY_TYPE_TO_DISPLAY[atom.type] || inferDisplay(atom);
  const resolvedColor = resolveColor(atom.meta?.color);

  // Per-atom size and shape classes from meta
  const sizeClass = atom.meta?.size ? `atom-size-${atom.meta.size}` : '';
  const shapeClass = atom.meta?.shape ? `atom-shape-${atom.meta.shape}` : '';

  return (
    <span
      className={`atom display-${display} ${sizeClass} ${shapeClass}`.trim()}
      onClick={onClick}
      data-slot={atom.slot}
    >
      {renderByDisplay(atom, display, resolvedColor)}
    </span>
  );
}

// Infer display from type + subtype when not explicitly set
function inferDisplay(atom) {
  if (atom.type === AtomType.TEXT) {
    return 'body';
  }
  if (atom.type === AtomType.NUMBER) {
    if (atom.subtype === AtomSubtype.CURRENCY) return 'price';
    if (atom.subtype === AtomSubtype.RATING) return 'rating';
    if (atom.subtype === AtomSubtype.PERCENT) return 'percent';
    return 'body';
  }
  if (atom.type === AtomType.IMAGE) {
    return 'image';
  }
  if (atom.type === AtomType.ICON) {
    return 'icon';
  }
  if (atom.type === AtomType.VIDEO) {
    return 'body'; // placeholder
  }
  if (atom.type === AtomType.AUDIO) {
    return 'body'; // placeholder
  }
  return 'body';
}

// Render content based on display type
function renderByDisplay(atom, display, color) {
  // Color style helpers
  const textColorStyle = color ? { color } : undefined;
  const bgColorStyle = color ? { backgroundColor: color, color: 'white' } : undefined;

  // Heading displays
  if (['h1', 'h2', 'h3', 'h4'].includes(display)) {
    const Tag = display;
    return <Tag className="atom-heading" style={textColorStyle}>{formatText(atom)}</Tag>;
  }

  // Body text displays
  if (['body-lg', 'body', 'body-sm', 'caption'].includes(display)) {
    return <span className={`atom-text ${display}`} style={textColorStyle}>{formatText(atom)}</span>;
  }

  // Badge displays
  if (display.startsWith('badge')) {
    return <span className={`atom-badge ${display}`} style={bgColorStyle}>{atom.value}</span>;
  }

  // Tag displays
  if (display.startsWith('tag')) {
    return <span className={`atom-tag ${display}`} style={bgColorStyle}>{atom.value}</span>;
  }

  // Price displays
  if (display.startsWith('price')) {
    return <span className={`atom-price ${display}`} style={textColorStyle}>{formatPrice(atom)}</span>;
  }

  // Rating displays
  if (display.startsWith('rating')) {
    return renderRating(atom, display);
  }

  // Percent display
  if (display === 'percent') {
    return <span className="atom-percent">{atom.value}%</span>;
  }

  // Progress display
  if (display === 'progress') {
    return (
      <div className="atom-progress">
        <div className="atom-progress-bar" style={{ width: `${atom.value}%` }} />
      </div>
    );
  }

  // Image displays
  if (['image', 'image-cover', 'avatar', 'avatar-sm', 'avatar-lg', 'thumbnail', 'gallery'].includes(display)) {
    return renderImage(atom, display);
  }

  // Icon displays
  if (display.startsWith('icon')) {
    return <span className={`atom-icon ${display}`}>{atom.value}</span>;
  }

  // Button displays
  if (display.startsWith('button')) {
    return (
      <button
        className={`atom-button ${display}`}
        data-action={atom.meta?.action}
        onClick={(e) => {
          e.stopPropagation();
          handleAction(atom.meta?.action);
        }}
      >
        {atom.value}
      </button>
    );
  }

  // Divider display
  if (display === 'divider') {
    return <div className="atom-divider" />;
  }

  // Spacer display
  if (display === 'spacer') {
    return <div className="atom-spacer" />;
  }

  // Default: just render value
  return <span>{String(atom.value)}</span>;
}

// Format text based on subtype
function formatText(atom) {
  if (atom.subtype === 'date' && atom.value) {
    return new Date(atom.value).toLocaleDateString();
  }
  if (atom.subtype === 'datetime' && atom.value) {
    return new Date(atom.value).toLocaleString();
  }
  return atom.value;
}

// Format price with currency
function formatPrice(atom) {
  const currency = atom.meta?.currency || '$';
  const value = typeof atom.value === 'number'
    ? atom.value.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })
    : atom.value;
  return `${currency}${value}`;
}

// Render rating based on display variant
function renderRating(atom, display) {
  const value = Number(atom.value) || 0;
  const stars = Math.round(value);

  if (display === 'rating-text') {
    return <span className="atom-rating rating-text">{value.toFixed(1)}/5</span>;
  }

  if (display === 'rating-compact') {
    return <span className="atom-rating rating-compact">★ {value.toFixed(1)}</span>;
  }

  // Default: full star display
  const fullStars = Math.min(stars, 5);
  const emptyStars = Math.max(0, 5 - fullStars);
  return (
    <span className="atom-rating">
      {'★'.repeat(fullStars)}{'☆'.repeat(emptyStars)}
    </span>
  );
}

// Render image based on display variant
function renderImage(atom, display) {
  // Handle array of images (take first) or single value
  const src = Array.isArray(atom.value) ? atom.value[0] : atom.value;

  if (display === 'gallery' && Array.isArray(atom.value)) {
    return (
      <div className="atom-gallery">
        {atom.value.map((imgSrc, i) => (
          <img
            key={i}
            src={imgSrc}
            alt={atom.meta?.label || `Image ${i + 1}`}
            className="atom-image gallery-item"
          />
        ))}
      </div>
    );
  }

  return (
    <img
      src={src}
      alt={atom.meta?.label || ''}
      className={`atom-image ${display}`}
    />
  );
}

function handleAction(action) {
  // TODO: dispatch action to parent via context or callback
  log.debug('Widget action:', action);
}
