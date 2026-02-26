import { useState, useCallback, useEffect, useRef } from 'react'
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
  const [overlayOpen, setOverlayOpen] = useState(false)

  const previewRef = useRef(null)
  const previewReady = useRef(false)

  // Load preview.js on mount
  useEffect(() => {
    if (window.__keepstar_preview) {
      previewReady.current = true
      return
    }
    const script = document.createElement('script')
    script.src = '/lib/preview.js'
    script.onload = () => { previewReady.current = true }
    document.head.appendChild(script)
  }, [])

  // Render formation when it changes or overlay opens
  useEffect(() => {
    if (!overlayOpen || !formation || !previewRef.current || !previewReady.current) return
    window.__keepstar_preview?.render(previewRef.current, formation, {
      onClose: () => setOverlayOpen(false),
    })
  }, [overlayOpen, formation])

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
      setOverlayOpen(true)
    } catch (err) {
      setError(err.message)
    } finally {
      setLoading(false)
    }
  }, [tenantSlug, preset, layout, size, direction, count, showFields, hideFields, orderFields, colorMap, displayMap, formatMap, shapeMap, anchorMap, layerMap, conditionalRaw])

  // Close overlay on Escape key
  useEffect(() => {
    const handleKey = (e) => {
      if (e.key === 'Escape' && overlayOpen) setOverlayOpen(false)
    }
    window.addEventListener('keydown', handleKey)
    return () => window.removeEventListener('keydown', handleKey)
  }, [overlayOpen])

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

        {/* Preview panel — tabs for data/json, overlay for visual */}
        <div className="testbench-preview">
          <div className="preview-tabs">
            <button className={activeTab === 'preview' ? 'tab active' : 'tab'}
              onClick={() => { setActiveTab('preview'); if (formation) setOverlayOpen(true) }}>Preview</button>
            <button className={activeTab === 'data' ? 'tab active' : 'tab'}
              onClick={() => setActiveTab('data')}>Entity Data</button>
            <button className={activeTab === 'json' ? 'tab active' : 'tab'}
              onClick={() => setActiveTab('json')}>JSON</button>
          </div>

          {activeTab === 'preview' && (
            <div className="preview-content">
              {formation ? (
                <div className="preview-hint">
                  Formation loaded ({formation.widgets?.length || 0} widgets).
                  <button className="submit-btn" style={{ marginLeft: 12 }} onClick={() => setOverlayOpen(true)}>
                    Open Preview
                  </button>
                </div>
              ) : (
                <div className="preview-empty">Click "Render" to see the formation</div>
              )}
            </div>
          )}

          {activeTab === 'data' && (
            <div className="preview-content">
              {entities?.length ? (
                <EntityDataTable entities={entities} />
              ) : (
                <div className="preview-empty">
                  {formation ? 'No entity data in response' : 'Click "Render" to load entity data'}
                </div>
              )}
            </div>
          )}

          {activeTab === 'json' && rawJson && (
            <pre className="json-view">{rawJson}</pre>
          )}
        </div>
      </div>

      {/* Full-screen widget preview — renders inside Shadow DOM with real overlay layout */}
      {overlayOpen && (
        <div ref={previewRef} className="tb-fullscreen-preview" />
      )}
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

const COLORS = { green: '#22C55E', red: '#EF4444', blue: '#3B82F6', orange: '#F97316', purple: '#8B5CF6', gray: '#6B7280' }
function resolveColor(c) {
  return COLORS[c] || c
}
