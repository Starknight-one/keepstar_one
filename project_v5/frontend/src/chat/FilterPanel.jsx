import { useEffect, useRef, useState } from 'react'

// Sort options (fixed UI control, not a data-derived facet). Empty key =
// relevance (the order the engine returned). Others map to a deterministic
// in-memory sort applied server-side before re-render.
const SORTS = [
  { key: '', label: 'Relevance' },
  { key: 'price_asc', label: 'Price ↑', field: 'price', order: 'asc' },
  { key: 'price_desc', label: 'Price ↓', field: 'price', order: 'desc' },
  { key: 'rating_desc', label: 'Top rated', field: 'rating', order: 'desc' },
]

// FilterPanel — data-derived filter controls + sort, in a drawer (open by
// default so it's noticed). Enum facets → multi-select chips; range facets →
// min/max inputs. Holds the active selection + sort locally; on any change it
// builds the full payload {filters, sort} and calls onChange(payload).
// Selections clear when `searchEpoch` changes (a new search). Facets are
// GUIDED — the backend recomputes available values per active filter.
//
// Filters/sort never touch the LLM: onChange hits the deterministic
// /navigation/filter endpoint which re-filters + re-sorts in memory.
export default function FilterPanel({ facets, searchEpoch, onChange }) {
  const [open, setOpen] = useState(true)
  const [sel, setSel] = useState({}) // key -> {type, values:[], min, max}
  const [sortKey, setSortKey] = useState('')
  const timer = useRef(null)

  // New search → drop the active selection + sort.
  useEffect(() => {
    setSel({})
    setSortKey('')
  }, [searchEpoch])

  // Flush any pending debounced push on unmount.
  useEffect(() => () => clearTimeout(timer.current), [])

  if (!Array.isArray(facets) || facets.length === 0) return null

  // currency facets store values in minor units (kopecks); show/accept rubles.
  const fmt = (f, n) => (f.unit === 'currency' ? Math.round(n / 100) : n)

  const buildPayload = (nextSel, nextSort) => {
    const filters = []
    for (const f of facets) {
      const s = nextSel[f.key]
      if (!s) continue
      if (f.type === 'range') {
        if (s.min != null || s.max != null) {
          filters.push({ key: f.key, type: 'range', min: s.min ?? undefined, max: s.max ?? undefined })
        }
      } else if (Array.isArray(s.values) && s.values.length) {
        filters.push({ key: f.key, type: 'enum', values: s.values })
      }
    }
    const so = SORTS.find((x) => x.key === nextSort)
    const sort = so && so.field ? { field: so.field, order: so.order } : null
    return { filters, sort }
  }

  // Enum/sort clicks apply immediately; range inputs debounce so we don't fire
  // a request (and briefly empty the grid) on every keystroke of a number.
  const pushNow = (nextSel, nextSort) => {
    clearTimeout(timer.current)
    onChange(buildPayload(nextSel, nextSort))
  }
  const pushDebounced = (nextSel, nextSort) => {
    clearTimeout(timer.current)
    timer.current = setTimeout(() => onChange(buildPayload(nextSel, nextSort)), 400)
  }

  const toggleValue = (f, value) => {
    const cur = sel[f.key]?.values || []
    const values = cur.includes(value) ? cur.filter((v) => v !== value) : [...cur, value]
    const next = { ...sel, [f.key]: { type: 'enum', values } }
    setSel(next)
    pushNow(next, sortKey)
  }

  const setRange = (f, bound, raw) => {
    const scale = f.unit === 'currency' ? 100 : 1
    const num = raw === '' ? null : Number(raw) * scale
    const cur = sel[f.key] || { type: 'range' }
    const next = { ...sel, [f.key]: { ...cur, type: 'range', [bound]: Number.isFinite(num) ? num : null } }
    setSel(next)
    pushDebounced(next, sortKey)
  }

  const pickSort = (key) => {
    setSortKey(key)
    pushNow(sel, key)
  }

  const activeCount = Object.values(sel).reduce((n, s) => {
    if (!s) return n
    if (s.type === 'range') return n + (s.min != null || s.max != null ? 1 : 0)
    return n + (s.values && s.values.length ? 1 : 0)
  }, 0)

  return (
    <div className="kw-filter-panel" data-open={open ? 'true' : 'false'}>
      <button type="button" className="kw-filter-toggle" onClick={() => setOpen((o) => !o)}>
        <span>Filters{activeCount ? ` · ${activeCount}` : ''}</span>
        <span className="kw-filter-caret">{open ? '▾' : '▸'}</span>
      </button>
      <div className="kw-filter-body">
        <div className="kw-facet">
          <div className="kw-filter-group-label">Sort</div>
          <div className="kw-chip-row">
            {SORTS.map((s) => (
              <button
                key={s.key || 'relevance'}
                type="button"
                className="kw-chip"
                data-active={sortKey === s.key ? 'true' : 'false'}
                onClick={() => pickSort(s.key)}
              >
                {s.label}
              </button>
            ))}
          </div>
        </div>
        {facets.map((f) => {
          const isRange = f.type === 'range'
          const values = f.values || []
          // Guided recompute can leave an enum facet with no available values
          // (e.g. an over-narrow filter) — skip it rather than render an empty
          // group (and never .map over undefined).
          if (!isRange && values.length === 0) return null
          return (
            <div className="kw-facet" key={f.key}>
              <div className="kw-filter-group-label">{f.label}</div>
              {isRange ? (
                <div className="kw-range-row">
                  <input
                    type="number"
                    className="kw-range-input"
                    placeholder={f.min != null ? String(fmt(f, f.min)) : 'min'}
                    value={sel[f.key]?.min != null ? fmt(f, sel[f.key].min) : ''}
                    onChange={(e) => setRange(f, 'min', e.target.value)}
                  />
                  <span className="kw-range-sep">–</span>
                  <input
                    type="number"
                    className="kw-range-input"
                    placeholder={f.max != null ? String(fmt(f, f.max)) : 'max'}
                    value={sel[f.key]?.max != null ? fmt(f, sel[f.key].max) : ''}
                    onChange={(e) => setRange(f, 'max', e.target.value)}
                  />
                </div>
              ) : (
                <div className="kw-chip-row">
                  {values.map((v) => {
                    const active = (sel[f.key]?.values || []).includes(v.value)
                    return (
                      <button
                        key={v.value}
                        type="button"
                        className="kw-chip"
                        data-active={active ? 'true' : 'false'}
                        onClick={() => toggleValue(f, v.value)}
                      >
                        {v.value}
                        {typeof v.count === 'number' ? ` (${v.count})` : ''}
                      </button>
                    )
                  })}
                </div>
              )}
            </div>
          )
        })}
      </div>
    </div>
  )
}
