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
const SHAPES = ['', 'pill', 'rounded', 'square', 'circle']
const PLACES = ['', 'default', 'sticky', 'floating']
const FIELDS = [
  'images', 'name', 'price', 'rating', 'brand', 'category',
  'description', 'tags', 'stockQuantity', 'attributes',
  'productForm', 'skinType', 'concern', 'keyIngredients',
]

const API_BASE = 'http://localhost:8080'

export default function TestbenchPage() {
  const [tenantSlug, setTenantSlug] = useState('hey-babes-cosmetics')
  const [preset, setPreset] = useState('')
  const [layout, setLayout] = useState('')
  const [size, setSize] = useState('')
  const [direction, setDirection] = useState('')
  const [place, setPlace] = useState('')
  const [count, setCount] = useState(6)
  const [showFields, setShowFields] = useState([])
  const [hideFields, setHideFields] = useState([])
  const [orderFields, setOrderFields] = useState('')
  const [colorOverrides, setColorOverrides] = useState('')
  const [displayOverrides, setDisplayOverrides] = useState('')
  const [shapeOverrides, setShapeOverrides] = useState('')
  const [anchorOverrides, setAnchorOverrides] = useState('')
  const [layerOverrides, setLayerOverrides] = useState('')
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

  const handleSubmit = useCallback(async () => {
    setLoading(true)
    setError(null)

    const params = {}
    if (preset) params.preset = preset
    if (layout) params.layout = layout
    if (size) params.size = size
    if (direction) params.direction = direction
    if (place && place !== 'default') params.place = place
    if (showFields.length > 0) params.show = showFields
    if (hideFields.length > 0) params.hide = hideFields

    // Order
    if (orderFields.trim()) {
      params.order = orderFields.split(',').map(s => s.trim()).filter(Boolean)
    }

    // JSON overrides
    const jsonFields = [
      ['color', colorOverrides],
      ['display', displayOverrides],
      ['shape', shapeOverrides],
      ['anchor', anchorOverrides],
      ['layer', layerOverrides],
      ['conditional', conditionalRaw],
    ]
    for (const [key, val] of jsonFields) {
      if (val.trim()) {
        try {
          params[key] = JSON.parse(val)
        } catch {
          setError(`Invalid ${key} JSON`)
          setLoading(false)
          return
        }
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
  }, [tenantSlug, preset, layout, size, direction, place, count, showFields, hideFields, orderFields, colorOverrides, displayOverrides, shapeOverrides, anchorOverrides, layerOverrides, conditionalRaw])

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
            <label>Order (comma-separated)</label>
            <input type="text" placeholder="price, name, brand"
              value={orderFields} onChange={e => setOrderFields(e.target.value)} />
          </div>

          <div className="control-group">
            <label>Color (JSON)</label>
            <input type="text" placeholder='{"price":"green","brand":"red"}'
              value={colorOverrides} onChange={e => setColorOverrides(e.target.value)} />
          </div>

          <div className="control-group">
            <label>Display (JSON)</label>
            <input type="text" placeholder='{"brand":"badge","price":"price-lg"}'
              value={displayOverrides} onChange={e => setDisplayOverrides(e.target.value)} />
          </div>

          <div className="control-group">
            <label>Shape (JSON)</label>
            <input type="text" placeholder='{"brand":"pill"}'
              value={shapeOverrides} onChange={e => setShapeOverrides(e.target.value)} />
          </div>

          <div className="control-group">
            <label>Anchor (JSON)</label>
            <input type="text" placeholder='{"brand":"top-right"}'
              value={anchorOverrides} onChange={e => setAnchorOverrides(e.target.value)} />
          </div>

          <div className="control-group">
            <label>Layer (JSON)</label>
            <input type="text" placeholder='{"stockQuantity":"2"}'
              value={layerOverrides} onChange={e => setLayerOverrides(e.target.value)} />
          </div>

          <div className="control-group">
            <label>Conditional (JSON array)</label>
            <input type="text" placeholder='[{"field":"stockQuantity","op":"eq","value":0,"color":"red"}]'
              value={conditionalRaw} onChange={e => setConditionalRaw(e.target.value)} />
          </div>

          <button className="submit-btn" onClick={handleSubmit} disabled={loading}>
            {loading ? 'Loading...' : 'Render'}
          </button>

          {error && <div className="testbench-error">{error}</div>}
          {warnings.length > 0 && (
            <div className="testbench-warnings">
              {warnings.map((w, i) => <div key={i}>{w}</div>)}
            </div>
          )}
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

          {activeTab === 'data' && entities && (
            <div className="preview-content">
              <EntityDataTable entities={entities} />
            </div>
          )}

          {activeTab === 'json' && rawJson && (
            <pre className="json-view">{rawJson}</pre>
          )}

          {!formation && !loading && (
            <div className="preview-empty">Click "Render" to see the formation</div>
          )}
        </div>
      </div>
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
  if (val === true) return '✓'
  if (val === false) return '—'
  if (val === '' || val == null) return '—'
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

  if (['h1', 'h2', 'h3', 'h4'].includes(display)) {
    const Tag = display
    return <Tag className="tb-heading" style={style}>{atom.value}</Tag>
  }
  if (display.startsWith('price')) {
    const currency = atom.meta?.currency || '$'
    const formatted = typeof atom.value === 'number'
      ? atom.value.toLocaleString(undefined, { minimumFractionDigits: 2 })
      : atom.value
    return <span className="tb-price" style={style}>{currency}{formatted}</span>
  }
  if (display.startsWith('rating')) {
    const val = Number(atom.value) || 0
    return <span className="tb-rating">{'★'.repeat(Math.round(val))}{'☆'.repeat(5 - Math.round(val))} {val.toFixed(1)}</span>
  }
  if (display.startsWith('badge')) {
    return <span className="tb-badge" style={style}>{atom.value}</span>
  }
  if (display.startsWith('tag')) {
    return <span className="tb-tag" style={style}>{atom.value}</span>
  }
  return <span className="tb-text" style={style}>{String(atom.value)}</span>
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
