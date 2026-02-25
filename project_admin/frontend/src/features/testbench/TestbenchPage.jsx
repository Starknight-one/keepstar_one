import { useState, useCallback } from 'react'
import './testbench.css'

const PRESETS = [
  { value: '', label: 'Auto (defaults engine)' },
  { value: 'product_card_grid', label: 'Card Grid' },
  { value: 'product_card_detail', label: 'Card Detail' },
  { value: 'product_row', label: 'Product Row' },
  { value: 'product_single_hero', label: 'Single Hero' },
  { value: 'product_comparison', label: 'Comparison' },
  { value: 'category_overview', label: 'Category Overview' },
  { value: 'cart_summary', label: 'Cart Summary' },
  { value: 'info_card', label: 'Info Card' },
]

const LAYOUTS = ['', 'grid', 'list', 'single', 'carousel', 'comparison']
const SIZES = ['', 'tiny', 'small', 'medium', 'large', 'xl']
const DIRECTIONS = ['', 'vertical', 'horizontal']
const FIELDS = [
  'images', 'name', 'price', 'rating', 'brand', 'category',
  'description', 'tags', 'stockQuantity', 'attributes',
  'productForm', 'skinType', 'concern', 'keyIngredients',
]

const NAMED_COLORS = ['green', 'red', 'blue', 'orange', 'purple', 'gray']
const SHAPE_VALUES = ['pill', 'rounded', 'square', 'circle']
const ANCHOR_VALUES = ['top-left', 'top-right', 'bottom-left', 'bottom-right', 'center']
const LAYER_VALUES = ['1', '2', '3', '4', '5']
const DISPLAY_VALUES = [
  'h1', 'h2', 'h3', 'h4',
  'body-lg', 'body', 'body-sm', 'caption',
  'badge', 'badge-success', 'badge-error', 'badge-warning',
  'tag', 'tag-active',
  'price', 'price-lg', 'price-old', 'price-discount',
  'rating', 'rating-text', 'rating-compact',
  'image', 'image-cover', 'thumbnail', 'avatar', 'gallery',
]

const FORMAT_VALUES = [
  'currency', 'stars', 'stars-text', 'stars-compact',
  'percent', 'number', 'date', 'text',
]

const API_BASE = 'http://localhost:8080'

export default function TestbenchPage() {
  const [tenantSlug, setTenantSlug] = useState('hey-babes-cosmetics')
  const [preset, setPreset] = useState('')
  const [layout, setLayout] = useState('')
  const [size, setSize] = useState('')
  const [direction, setDirection] = useState('')
  const [count, setCount] = useState(6)
  const [showFields, setShowFields] = useState([])
  const [hideFields, setHideFields] = useState([])
  const [orderFields, setOrderFields] = useState([])
  const [colorMap, setColorMap] = useState({})
  const [displayMap, setDisplayMap] = useState({})
  const [formatMap, setFormatMap] = useState({})
  const [shapeMap, setShapeMap] = useState({})
  const [anchorMap, setAnchorMap] = useState({})
  const [layerMap, setLayerMap] = useState({})
  const [conditionalRaw, setConditionalRaw] = useState('')

  const [formation, setFormation] = useState(null)
  const [entities, setEntities] = useState(null)
  const [rawJson, setRawJson] = useState('')
  const [warnings, setWarnings] = useState([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState(null)
  const [activeTab, setActiveTab] = useState('preview')

  const toggleField = useCallback((field, list, setList) => {
    setList(prev =>
      prev.includes(field) ? prev.filter(f => f !== field) : [...prev, field]
    )
  }, [])

  const toggleOrderField = useCallback((field) => {
    setOrderFields(prev =>
      prev.includes(field) ? prev.filter(f => f !== field) : [...prev, field]
    )
  }, [])

  const handleSubmit = useCallback(async () => {
    setLoading(true)
    setError(null)

    const params = {}
    if (preset) params.preset = preset
    if (layout) params.layout = layout
    if (size) params.size = size
    if (direction) params.direction = direction
    if (showFields.length > 0) params.show = showFields
    if (hideFields.length > 0) params.hide = hideFields
    if (orderFields.length > 0) params.order = orderFields

    // Map overrides — only include non-empty maps
    if (Object.keys(colorMap).length > 0) params.color = colorMap
    if (Object.keys(displayMap).length > 0) params.display = displayMap
    if (Object.keys(formatMap).length > 0) params.format = formatMap
    if (Object.keys(shapeMap).length > 0) params.shape = shapeMap
    if (Object.keys(anchorMap).length > 0) params.anchor = anchorMap
    if (Object.keys(layerMap).length > 0) params.layer = layerMap

    // Conditional stays as JSON
    if (conditionalRaw.trim()) {
      try {
        params.conditional = JSON.parse(conditionalRaw)
      } catch {
        setError('Invalid conditional JSON')
        setLoading(false)
        return
      }
    }

    try {
      const res = await fetch(`${API_BASE}/api/v1/testbench`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ tenantSlug, count, params }),
      })

      if (!res.ok) {
        throw new Error(`HTTP ${res.status}: ${await res.text()}`)
      }

      const data = await res.json()
      setFormation(data.formation)
      setEntities(data.entities)
      setRawJson(JSON.stringify(data, null, 2))
      setWarnings(data.warnings || [])
    } catch (err) {
      setError(err.message)
    } finally {
      setLoading(false)
    }
  }, [tenantSlug, preset, layout, size, direction, count, showFields, hideFields, orderFields, colorMap, displayMap, formatMap, shapeMap, anchorMap, layerMap, conditionalRaw])

  return (
    <div className="testbench">
      <h1 className="testbench-title">Visual Assembly Testbench</h1>

      <div className="testbench-layout">
        {/* Controls panel */}
        <div className="testbench-controls">
          <div className="control-group">
            <label>Tenant</label>
            <input type="text" value={tenantSlug} onChange={e => setTenantSlug(e.target.value)} />
          </div>

          <div className="control-group">
            <label>Preset</label>
            <select value={preset} onChange={e => setPreset(e.target.value)}>
              {PRESETS.map(p => (
                <option key={p.value} value={p.value}>{p.label}</option>
              ))}
            </select>
          </div>

          <div className="control-row">
            <div className="control-group">
              <label>Layout</label>
              <select value={layout} onChange={e => setLayout(e.target.value)}>
                {LAYOUTS.map(l => (
                  <option key={l} value={l}>{l || 'auto'}</option>
                ))}
              </select>
            </div>
            <div className="control-group">
              <label>Size</label>
              <select value={size} onChange={e => setSize(e.target.value)}>
                {SIZES.map(s => (
                  <option key={s} value={s}>{s || 'auto'}</option>
                ))}
              </select>
            </div>
            <div className="control-group">
              <label>Direction</label>
              <select value={direction} onChange={e => setDirection(e.target.value)}>
                {DIRECTIONS.map(d => (
                  <option key={d} value={d}>{d || 'auto'}</option>
                ))}
              </select>
            </div>
          </div>

          <div className="control-group">
            <label>Count: {count}</label>
            <input type="range" min={1} max={20} value={count}
              onChange={e => setCount(Number(e.target.value))} />
          </div>

          <div className="button-row">
            <button className="submit-btn" onClick={handleSubmit} disabled={loading}>
              {loading ? 'Loading...' : 'Render'}
            </button>
            <button className="clear-btn" onClick={() => {
              setPreset(''); setLayout(''); setSize(''); setDirection('')
              setShowFields([]); setHideFields([]); setOrderFields([])
              setColorMap({}); setDisplayMap({}); setFormatMap({}); setShapeMap({}); setAnchorMap({}); setLayerMap({})
              setConditionalRaw(''); setCount(6)
            }}>Clear</button>
          </div>

          {error && <div className="testbench-error">{error}</div>}
          {warnings.length > 0 && (
            <div className="testbench-warnings">
              {warnings.map((w, i) => <div key={i}>{w}</div>)}
            </div>
          )}

          <div className="control-group">
            <label>Show Fields</label>
            <div className="field-chips">
              {FIELDS.map(f => (
                <button
                  key={f}
                  className={`chip ${showFields.includes(f) ? 'active' : ''}`}
                  onClick={() => toggleField(f, showFields, setShowFields)}
                >{f}</button>
              ))}
            </div>
          </div>

          <div className="control-group">
            <label>Hide Fields</label>
            <div className="field-chips">
              {FIELDS.map(f => (
                <button
                  key={f}
                  className={`chip hide ${hideFields.includes(f) ? 'active' : ''}`}
                  onClick={() => toggleField(f, hideFields, setHideFields)}
                >{f}</button>
              ))}
            </div>
          </div>

          <div className="control-group">
            <label>Order {orderFields.length > 0 && <span className="hint">({orderFields.join(' > ')})</span>}</label>
            <div className="field-chips">
              {FIELDS.map(f => {
                const idx = orderFields.indexOf(f)
                return (
                  <button
                    key={f}
                    className={`chip order ${idx >= 0 ? 'active' : ''}`}
                    onClick={() => toggleOrderField(f)}
                  >{idx >= 0 ? `${idx + 1}. ` : ''}{f}</button>
                )
              })}
            </div>
          </div>

          <FieldOverridePicker
            label="Color"
            fields={FIELDS}
            values={NAMED_COLORS}
            map={colorMap}
            setMap={setColorMap}
            renderValue={(v) => <span className="color-dot" style={{ background: resolveColor(v) }} />}
          />

          <FieldOverridePicker
            label="Display (wrapper)"
            fields={FIELDS}
            values={DISPLAY_VALUES}
            map={displayMap}
            setMap={setDisplayMap}
          />

          <FieldOverridePicker
            label="Format (value transform)"
            fields={FIELDS}
            values={FORMAT_VALUES}
            map={formatMap}
            setMap={setFormatMap}
          />

          <FieldOverridePicker
            label="Shape"
            fields={FIELDS}
            values={SHAPE_VALUES}
            map={shapeMap}
            setMap={setShapeMap}
          />

          <FieldOverridePicker
            label="Anchor"
            fields={FIELDS}
            values={ANCHOR_VALUES}
            map={anchorMap}
            setMap={setAnchorMap}
          />

          <FieldOverridePicker
            label="Layer"
            fields={FIELDS}
            values={LAYER_VALUES}
            map={layerMap}
            setMap={setLayerMap}
          />

          <div className="control-group">
            <label>Conditional (JSON)</label>
            <input type="text" placeholder='[{"field":"stockQuantity","op":"eq","value":0,"color":"red"}]'
              value={conditionalRaw} onChange={e => setConditionalRaw(e.target.value)} />
          </div>

        </div>

        {/* Preview panel */}
        <div className="testbench-preview">
          <div className="preview-tabs">
            <button className={activeTab === 'preview' ? 'tab active' : 'tab'}
              onClick={() => setActiveTab('preview')}>Preview</button>
            <button className={activeTab === 'data' ? 'tab active' : 'tab'}
              onClick={() => setActiveTab('data')}>Entity Data</button>
            <button className={activeTab === 'json' ? 'tab active' : 'tab'}
              onClick={() => setActiveTab('json')}>JSON</button>
          </div>

          {activeTab === 'preview' && formation && (
            <div className="preview-content">
              <TestbenchFormation formation={formation} />
            </div>
          )}

          {activeTab === 'data' && (
            <div className="preview-content">
              {entities?.length ? (
                <EntityDataTable entities={entities} />
              ) : (
                <div className="preview-empty">
                  {formation ? 'No entity data in response — click Render' : 'Click "Render" to load entity data'}
                </div>
              )}
            </div>
          )}

          {activeTab === 'json' && rawJson && (
            <pre className="json-view">{rawJson}</pre>
          )}

          {!formation && !loading && activeTab === 'preview' && (
            <div className="preview-empty">Click "Render" to see the formation</div>
          )}
        </div>
      </div>
    </div>
  )
}

// Per-field override picker: click field → pick value from chips
function FieldOverridePicker({ label, fields, values, map, setMap, renderValue }) {
  const [activeField, setActiveField] = useState(null)
  const assigned = Object.keys(map)

  const handleFieldClick = (field) => {
    if (activeField === field) {
      setActiveField(null)
    } else {
      setActiveField(field)
    }
  }

  const handleValueClick = (value) => {
    if (!activeField) return
    setMap(prev => {
      if (prev[activeField] === value) {
        const next = { ...prev }
        delete next[activeField]
        return next
      }
      return { ...prev, [activeField]: value }
    })
  }

  const handleClear = (field, e) => {
    e.stopPropagation()
    setMap(prev => {
      const next = { ...prev }
      delete next[field]
      return next
    })
    if (activeField === field) setActiveField(null)
  }

  return (
    <div className="control-group">
      <label>
        {label}
        {assigned.length > 0 && (
          <button className="clear-all" onClick={() => { setMap({}); setActiveField(null) }}>clear all</button>
        )}
      </label>

      {/* Assigned overrides */}
      {assigned.length > 0 && (
        <div className="override-tags">
          {assigned.map(f => (
            <span key={f} className="override-tag" onClick={() => handleFieldClick(f)}>
              {renderValue && renderValue(map[f])}
              {f}: {map[f]}
              <span className="override-remove" onClick={(e) => handleClear(f, e)}>&times;</span>
            </span>
          ))}
        </div>
      )}

      {/* Field chips */}
      <div className="field-chips">
        {fields.map(f => (
          <button
            key={f}
            className={`chip picker ${activeField === f ? 'active' : ''} ${map[f] ? 'has-value' : ''}`}
            onClick={() => handleFieldClick(f)}
          >{f}</button>
        ))}
      </div>

      {/* Value chips — shown when a field is active */}
      {activeField && (
        <div className="value-picker">
          <div className="value-picker-label">
            {activeField} =
          </div>
          <div className="field-chips">
            {values.map(v => (
              <button
                key={v}
                className={`chip value ${map[activeField] === v ? 'active' : ''}`}
                onClick={() => handleValueClick(v)}
              >
                {renderValue && renderValue(v)}
                {v}
              </button>
            ))}
          </div>
        </div>
      )}
    </div>
  )
}

// Entity data table — shows raw product data so you know what's available
function EntityDataTable({ entities }) {
  if (!entities?.length) return <div className="preview-empty">No entity data</div>

  const fields = [
    { key: 'name', label: 'Name' },
    { key: 'brand', label: 'Brand' },
    { key: 'category', label: 'Category' },
    { key: 'price', label: 'Price' },
    { key: 'rating', label: 'Rating' },
    { key: 'images', label: 'Images' },
    { key: 'stockQuantity', label: 'Stock' },
    { key: 'hasDescription', label: 'Desc?' },
    { key: 'hasTags', label: 'Tags?' },
    { key: 'productForm', label: 'Form' },
    { key: 'skinType', label: 'Skin Type' },
    { key: 'concern', label: 'Concern' },
    { key: 'keyIngredients', label: 'Ingredients' },
  ]

  return (
    <div className="entity-table-wrapper">
      <table className="entity-table">
        <thead>
          <tr>
            <th>#</th>
            {fields.map(f => <th key={f.key}>{f.label}</th>)}
          </tr>
        </thead>
        <tbody>
          {entities.map((e, i) => (
            <tr key={i}>
              <td className="entity-idx">{i + 1}</td>
              {fields.map(f => (
                <td key={f.key} className={cellClass(e[f.key])}>
                  {formatCell(e[f.key])}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

function cellClass(val) {
  if (val === '' || val === 0 || val === false || val == null) return 'cell-empty'
  return ''
}

function formatCell(val) {
  if (val === true) return '\u2713'
  if (val === false) return '\u2014'
  if (val === '' || val == null) return '\u2014'
  if (typeof val === 'number') return val.toLocaleString()
  if (typeof val === 'string' && val.length > 40) return val.slice(0, 37) + '...'
  return String(val)
}

// Minimal formation renderer
function TestbenchFormation({ formation }) {
  if (!formation?.widgets?.length) return <div className="preview-empty">No widgets</div>

  const layoutClass = getLayoutClass(formation.mode, formation.grid?.cols || 2)

  return (
    <div className={layoutClass}>
      {formation.widgets.map((widget, wi) => (
        <TestbenchWidget key={wi} widget={widget} />
      ))}
    </div>
  )
}

function TestbenchWidget({ widget }) {
  const direction = widget.meta?.direction || 'vertical'
  const imageAtoms = (widget.atoms || []).filter(a => a.type === 'image')
  const contentAtoms = (widget.atoms || []).filter(a => a.type !== 'image')
  const images = imageAtoms.length > 0
    ? (Array.isArray(imageAtoms[0].value) ? imageAtoms[0].value : [imageAtoms[0].value]).filter(Boolean)
    : []

  return (
    <div className={`tb-card size-${widget.size || 'medium'} ${direction === 'horizontal' ? 'tb-card-horizontal' : ''}`}>
      {images.length > 0 && (
        <div className="tb-card-media">
          <img src={images[0]} alt="" className="tb-card-img" />
        </div>
      )}
      <div className="tb-card-content">
        {contentAtoms.map((atom, i) => (
          <TestbenchAtom key={i} atom={atom} />
        ))}
      </div>
    </div>
  )
}

function TestbenchAtom({ atom }) {
  if (atom.value == null && atom.type !== 'image') return null

  const display = atom.display || 'body'
  const color = atom.meta?.color
  const style = color ? (
    display.startsWith('badge') || display.startsWith('tag')
      ? { backgroundColor: resolveColor(color), color: '#fff' }
      : { color: resolveColor(color) }
  ) : undefined

  // Format value based on atom.format (or infer from type+subtype)
  const formatted = tbFormatValue(atom)

  if (['h1', 'h2', 'h3', 'h4'].includes(display)) {
    const Tag = display
    return <Tag className="tb-heading" style={style}>{formatted}</Tag>
  }
  if (display.startsWith('price')) {
    return <span className="tb-price" style={style}>{formatted}</span>
  }
  if (display.startsWith('rating')) {
    return <span className="tb-rating">{formatted}</span>
  }
  if (display.startsWith('badge')) {
    return <span className="tb-badge" style={style}>{formatted}</span>
  }
  if (display.startsWith('tag')) {
    return <span className="tb-tag" style={style}>{formatted}</span>
  }
  return <span className="tb-text" style={style}>{String(formatted)}</span>
}

// Format value using atom.format or infer from type+subtype
function tbFormatValue(atom) {
  const format = atom.format || tbInferFormat(atom)
  const value = atom.value

  switch (format) {
    case 'currency': {
      if (value == null) return null
      const currency = atom.meta?.currency || '$'
      const f = typeof value === 'number'
        ? value.toLocaleString(undefined, { minimumFractionDigits: 2 })
        : value
      return `${currency}${f}`
    }
    case 'stars': {
      const v = Number(value) || 0
      const full = Math.min(Math.round(v), 5)
      return '\u2605'.repeat(full) + '\u2606'.repeat(Math.max(0, 5 - full))
    }
    case 'stars-text': {
      const v = Number(value) || 0
      return `${v.toFixed(1)}/5`
    }
    case 'stars-compact': {
      const v = Number(value) || 0
      return `\u2605 ${v.toFixed(1)}`
    }
    case 'percent':
      return `${value}%`
    case 'number':
      return typeof value === 'number' ? value.toLocaleString() : String(value)
    case 'date':
      return value ? new Date(value).toLocaleDateString() : value
    case 'text':
    default:
      return value
  }
}

function tbInferFormat(atom) {
  if (atom.type === 'number') {
    if (atom.subtype === 'currency') return 'currency'
    if (atom.subtype === 'rating') return 'stars-compact'
    if (atom.subtype === 'percent') return 'percent'
    return 'number'
  }
  if (atom.type === 'text') {
    if (atom.subtype === 'date' || atom.subtype === 'datetime') return 'date'
  }
  return 'text'
}

const COLORS = { green: '#22C55E', red: '#EF4444', blue: '#3B82F6', orange: '#F97316', purple: '#8B5CF6', gray: '#6B7280' }
function resolveColor(c) {
  return COLORS[c] || c
}

function getLayoutClass(mode, cols) {
  switch (mode) {
    case 'grid': return `tb-grid cols-${cols}`
    case 'carousel': return 'tb-carousel'
    case 'single': return 'tb-single'
    case 'list':
    default: return 'tb-list'
  }
}
