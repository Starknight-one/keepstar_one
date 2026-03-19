import { AtomType } from './atomModel';
import { log } from '../../shared/logger';
import './AtomV2.css';

// Semantic token maps — match backend engine/tokens.go
const FONT_SIZE_TOKENS = { xs: 10, sm: 12, md: 14, lg: 18, xl: 24, '2xl': 30, '3xl': 36 };
const FONT_WEIGHT_TOKENS = { light: 300, normal: 400, medium: 500, semibold: 600, bold: 700 };

const COLOR_PALETTE = {
  green: '#22C55E', red: '#EF4444', blue: '#3B82F6',
  orange: '#F97316', purple: '#8B5CF6', gray: '#6B7280',
  muted: '#9CA3AF', error: '#EF4444', success: '#22C55E',
  warning: '#F59E0B', info: '#3B82F6',
};

function resolveColor(color) {
  if (!color) return undefined;
  return COLOR_PALETTE[color] || color;
}

function contrastText(hex) {
  if (!hex || !hex.startsWith('#')) return '#FFFFFF';
  const h = hex.replace('#', '');
  const full = h.length === 3 ? h.split('').map(c => c + c).join('') : h;
  const r = parseInt(full.substring(0, 2), 16) / 255;
  const g = parseInt(full.substring(2, 4), 16) / 255;
  const b = parseInt(full.substring(4, 6), 16) / 255;
  return (0.2126 * r + 0.7152 * g + 0.0722 * b) > 0.5 ? '#18181B' : '#FFFFFF';
}

/**
 * AtomV2Renderer — renders a v2 atom with separate textStyle + wrapper.
 * Used by LayoutTreeRenderer for v2 widget rendering.
 */
export function AtomV2Renderer({ atom }) {
  if (atom.value == null && atom.value !== 0 && atom.type !== 'image') return null;

  const formatted = formatV2Value(atom);
  const textStyles = resolveTextStyle(atom.textStyle);
  const content = renderV2Content(formatted, atom, textStyles);

  // Wrap with wrapper if present
  if (atom.wrapper && atom.wrapper.type && atom.wrapper.type !== 'none') {
    return renderWrapper(content, atom.wrapper, atom);
  }

  return content;
}

const LINE_HEIGHT_TOKENS = { tight: 1.25, normal: 1.5, relaxed: 1.625, loose: 2 };
const LETTER_SPACING_TOKENS = { tight: '-0.025em', normal: '0', wide: '0.05em' };

function resolveTextStyle(ts) {
  if (!ts) return {};
  const style = {};
  if (ts.fontSize) {
    style.fontSize = (FONT_SIZE_TOKENS[ts.fontSize] || 14) + 'px';
  }
  if (ts.fontWeight) {
    style.fontWeight = FONT_WEIGHT_TOKENS[ts.fontWeight] || ts.fontWeight;
  }
  if (ts.color) {
    style.color = resolveColor(ts.color);
  }
  if (ts.textDecoration) {
    style.textDecoration = ts.textDecoration;
  }
  if (ts.textTransform) {
    style.textTransform = ts.textTransform;
  }
  if (ts.lineHeight) {
    style.lineHeight = LINE_HEIGHT_TOKENS[ts.lineHeight] || ts.lineHeight;
  }
  if (ts.letterSpacing) {
    style.letterSpacing = LETTER_SPACING_TOKENS[ts.letterSpacing] || ts.letterSpacing;
  }
  if (ts.lineClamp && ts.lineClamp > 0) {
    style.display = '-webkit-box';
    style.WebkitLineClamp = ts.lineClamp;
    style.WebkitBoxOrient = 'vertical';
    style.overflow = 'hidden';
  }
  return style;
}

function formatV2Value(atom) {
  if (atom.type === AtomType.IMAGE || atom.type === 'icon' || atom.type === 'video' || atom.type === 'audio') {
    return atom.value;
  }

  const format = atom.format || inferV2Format(atom);
  const value = atom.value;

  switch (format) {
    case 'currency': {
      if (value == null) return null;
      const currency = atom.meta?.currency || '$';
      const num = typeof value === 'number'
        ? value.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })
        : value;
      return `${currency}${num}`;
    }
    case 'stars': {
      const v = Math.min(Math.round(Number(value) || 0), 5);
      return '★'.repeat(v) + '☆'.repeat(Math.max(0, 5 - v));
    }
    case 'stars-text':
      return `${(Number(value) || 0).toFixed(1)}/5`;
    case 'stars-compact':
      return `★ ${(Number(value) || 0).toFixed(1)}`;
    case 'percent':
      return `${value}%`;
    case 'number':
      return typeof value === 'number' ? value.toLocaleString() : String(value);
    case 'date':
      return value ? new Date(value).toLocaleDateString() : value;
    default:
      return value;
  }
}

function inferV2Format(atom) {
  if (atom.type === 'number') {
    if (atom.subtype === 'currency') return 'currency';
    if (atom.subtype === 'rating') return 'stars-compact';
    if (atom.subtype === 'percent') return 'percent';
    return 'number';
  }
  if (atom.subtype === 'date' || atom.subtype === 'datetime') return 'date';
  return 'text';
}

function renderV2Content(formatted, atom, textStyles) {
  // Image types
  if (atom.type === AtomType.IMAGE || atom.type === 'image') {
    const src = Array.isArray(atom.value) ? atom.value[0] : atom.value;
    const imgStyle = {};
    if (atom.mediaStyle) {
      if (atom.mediaStyle.aspectRatio) {
        imgStyle.aspectRatio = atom.mediaStyle.aspectRatio.replace(':', '/');
      }
      if (atom.mediaStyle.objectFit) {
        imgStyle.objectFit = atom.mediaStyle.objectFit;
      }
    }
    return <img src={src} alt={atom.label || ''} className="atom-v2-image" style={Object.keys(imgStyle).length > 0 ? imgStyle : undefined} />;
  }

  // Icon
  if (atom.type === 'icon') {
    return <span className="atom-v2-icon" style={textStyles}>{atom.value}</span>;
  }

  // Progress (special: needs raw value for bar width)
  if (atom.format === 'progress' || (atom.wrapper && atom.wrapper.type === 'progress')) {
    return (
      <div className="atom-v2-progress">
        <div className="atom-v2-progress-bar" style={{ width: `${atom.value}%` }} />
      </div>
    );
  }

  // Text content with textStyle
  return <span className="atom-v2-text" style={textStyles}>{formatted}</span>;
}

function renderWrapper(content, wrapper, atom) {
  const variant = wrapper.variant || '';
  const color = resolveColor(atom.textStyle?.color);

  switch (wrapper.type) {
    case 'badge': {
      const bgColor = color || variantColor(variant);
      const style = bgColor ? { backgroundColor: bgColor, color: contrastText(bgColor) } : undefined;
      return <span className={`atom-v2-badge ${variant}`} style={style}>{content}</span>;
    }
    case 'tag': {
      const borderColor = color || variantColor(variant);
      const style = borderColor ? { borderColor, color: borderColor } : undefined;
      return <span className={`atom-v2-tag ${variant}`} style={style}>{content}</span>;
    }
    case 'pill': {
      const bgColor = color || variantColor(variant);
      const style = bgColor ? { backgroundColor: bgColor, color: contrastText(bgColor) } : undefined;
      return <span className={`atom-v2-pill ${variant}`} style={style}>{content}</span>;
    }
    case 'avatar':
      return <span className="atom-v2-avatar">{content}</span>;
    case 'tooltip':
      return <span className="atom-v2-tooltip" title={atom.label || ''}>{content}</span>;
    case 'alert': {
      const alertColor = variantColor(variant);
      return <div className={`atom-v2-alert ${variant}`} style={alertColor ? { borderLeftColor: alertColor } : undefined}>{content}</div>;
    }
    case 'link':
      return <a className="atom-v2-link" href={typeof atom.value === 'string' ? atom.value : '#'}>{content}</a>;
    case 'progress':
      return (
        <div className="atom-v2-progress">
          <div className="atom-v2-progress-bar" style={{ width: `${atom.value}%` }} />
        </div>
      );
    case 'button': {
      return (
        <button
          className={`atom-v2-button ${variant}`}
          onClick={(e) => { e.stopPropagation(); log.debug('v2 button action:', atom.meta?.action); }}
        >
          {content}
        </button>
      );
    }
    default:
      return content;
  }
}

function variantColor(variant) {
  switch (variant) {
    case 'success': return '#22C55E';
    case 'error': return '#EF4444';
    case 'warning': return '#F59E0B';
    case 'info': return '#3B82F6';
    case 'primary': return '#8B5CF6';
    case 'secondary': return '#6B7280';
    default: return null;
  }
}
