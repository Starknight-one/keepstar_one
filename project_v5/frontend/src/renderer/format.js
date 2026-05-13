// Format pass — turn a raw `content` value into a display string,
// driven by `node.format`. Backend stores `format` as a pass-through
// string on text nodes; the renderer applies it at render time. This
// matches V5's design contract (see internal/prompts/agent2_prompt.go
// FORMAT + WRAPPER section).
//
// Vocabulary:
//   currency        12.99 → "$12.99"  (Intl.NumberFormat, USD, locale-agnostic)
//   stars           4.5   → "★★★★½ 4.5"
//   stars-compact   4.5   → "★ 4.5"
//   stars-text      4.5   → "4.5/5"
//   percent         12.5  → "12.5%"
//   number          1234  → "1234"  (passthrough; locale-agnostic)
//   date            ISO   → "Sep 14, 2025"
//   text (default)        → as-is

export function formatValue(content, format) {
  if (content === null || content === undefined || content === '') return ''
  if (!format || format === 'text') return String(content)

  switch (format) {
    case 'currency':
      return formatCurrency(content)
    case 'stars':
      return formatStars(content, false)
    case 'stars-compact':
      return formatStarsCompact(content)
    case 'stars-text':
      return formatStarsText(content)
    case 'percent':
      return formatPercent(content)
    case 'number':
      return formatNumber(content)
    case 'date':
      return formatDate(content)
    default:
      return String(content)
  }
}

function formatCurrency(v) {
  if (typeof v === 'string') {
    // Already formatted by backend (priceFormatted field) — pass through
    if (/[$€£¥₽]/.test(v)) return v
    const n = Number(v)
    if (Number.isFinite(n)) return formatCurrencyNumber(n)
    return v
  }
  if (typeof v === 'number') return formatCurrencyNumber(v)
  return String(v)
}

function formatCurrencyNumber(n) {
  try {
    return new Intl.NumberFormat('en-US', {
      style: 'currency',
      currency: 'USD',
      minimumFractionDigits: 0,
      maximumFractionDigits: 2,
    }).format(n)
  } catch (_) {
    return `$${n.toFixed(2)}`
  }
}

function formatStars(v, compact) {
  const n = clamp(toFiniteNumber(v), 0, 5)
  const full = Math.floor(n)
  const half = n - full >= 0.5 ? 1 : 0
  const empty = 5 - full - half
  const stars =
    '★'.repeat(full) + (half ? '½' : '') + (compact ? '' : '☆'.repeat(empty))
  return `${stars} ${n.toFixed(1)}`
}

function formatStarsCompact(v) {
  const n = clamp(toFiniteNumber(v), 0, 5)
  return `★ ${n.toFixed(1)}`
}

function formatStarsText(v) {
  const n = clamp(toFiniteNumber(v), 0, 5)
  return `${n.toFixed(1)}/5`
}

function formatPercent(v) {
  const n = toFiniteNumber(v)
  return `${n}%`
}

function formatNumber(v) {
  const n = toFiniteNumber(v)
  return Number.isInteger(n) ? String(n) : n.toString()
}

function formatDate(v) {
  if (!v) return ''
  const d = new Date(v)
  if (Number.isNaN(d.getTime())) return String(v)
  try {
    return new Intl.DateTimeFormat('en-US', {
      year: 'numeric',
      month: 'short',
      day: 'numeric',
    }).format(d)
  } catch (_) {
    return d.toDateString()
  }
}

function toFiniteNumber(v) {
  if (typeof v === 'number') return Number.isFinite(v) ? v : 0
  if (typeof v === 'string') {
    const n = Number(v)
    return Number.isFinite(n) ? n : 0
  }
  return 0
}

function clamp(n, lo, hi) {
  return Math.max(lo, Math.min(hi, n))
}
