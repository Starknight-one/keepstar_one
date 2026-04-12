import { useState, useEffect, useCallback, useRef, useMemo } from 'react'
import { Tldraw, useEditor, createShapeId } from 'tldraw'
import 'tldraw/tldraw.css'
import { api } from '../../shared/api/apiClient.js'
import Spinner from '../../shared/ui/Spinner.jsx'
import Tabs from '../../shared/ui/Tabs.jsx'
import { PresetTileShapeUtil, PRESET_TILE_TYPE } from './PresetTileShape.jsx'
import './canvas.css'

const INSPECTOR_TABS = [
  { key: 'properties', label: 'Properties' },
  { key: 'design', label: 'Design System' },
  { key: 'data', label: 'Data' },
]

const SHAPE_UTILS = [PresetTileShapeUtil]

const TILE_GAP = 32
const TILES_PER_ROW = 4
const TILE_W = 260
const TILE_H = 160

// Persist camera position per tenant to localStorage
const CAMERA_KEY = 'keepstar_canvas_camera'
function loadCamera() {
  try { return JSON.parse(localStorage.getItem(CAMERA_KEY)) } catch { return null }
}
function saveCamera(camera) {
  try { localStorage.setItem(CAMERA_KEY, JSON.stringify(camera)) } catch { /* noop */ }
}

/** Inner component rendered inside <Tldraw> — has access to useEditor() */
function CanvasInner({ presets, onSelectPreset }) {
  const editor = useEditor()
  const mountedRef = useRef(false)

  // On first mount: create preset tile shapes from loaded presets
  useEffect(() => {
    if (mountedRef.current || !editor || presets.length === 0) return
    mountedRef.current = true

    // Avoid duplicating shapes if they already exist (e.g. hot reload)
    const existingIds = new Set(
      editor.getCurrentPageShapes()
        .filter(s => s.type === PRESET_TILE_TYPE)
        .map(s => s.props.presetId)
    )

    const newShapes = []
    let idx = existingIds.size
    for (const p of presets) {
      if (existingIds.has(p.id)) continue
      const col = idx % TILES_PER_ROW
      const row = Math.floor(idx / TILES_PER_ROW)
      let opsCount = 0
      try {
        const ops = p.latestVersion?.ops_json
        if (ops) opsCount = JSON.parse(ops).length
      } catch { /* noop */ }

      newShapes.push({
        id: createShapeId(`preset-${p.id}`),
        type: PRESET_TILE_TYPE,
        x: col * (TILE_W + TILE_GAP) + 80,
        y: row * (TILE_H + TILE_GAP) + 80,
        props: {
          w: TILE_W,
          h: TILE_H,
          presetId: p.id,
          name: p.name,
          category: p.category || 'product',
          description: p.description || '',
          status: p.latestVersion?.status || p.status || 'draft',
          defaultReplicate: !!(p.defaultReplicate ?? p.default_replicate),
          opsCount,
        },
      })
      idx++
    }

    if (newShapes.length > 0) {
      editor.createShapes(newShapes)
    }

    // Restore camera
    const cam = loadCamera()
    if (cam) {
      editor.setCamera({ x: cam.x, y: cam.y, z: cam.z })
    } else if (newShapes.length > 0) {
      // Zoom to fit all shapes
      editor.zoomToFit({ animation: { duration: 0 } })
    }
  }, [editor, presets])

  // Listen for selection changes → notify parent
  useEffect(() => {
    if (!editor) return
    const handleChange = () => {
      const shapes = editor.getSelectedShapes()
      const tile = shapes.find(s => s.type === PRESET_TILE_TYPE)
      if (tile) {
        onSelectPreset(tile.props.presetId)
      } else if (shapes.length === 0) {
        onSelectPreset(null)
      }
    }
    // Subscribe to the store for selection changes
    const unsub = editor.store.listen(handleChange, {
      source: 'user',
      scope: 'session',
    })
    return unsub
  }, [editor, onSelectPreset])

  // Persist camera on move
  useEffect(() => {
    if (!editor) return
    const unsub = editor.store.listen(
      () => {
        const cam = editor.getCamera()
        saveCamera({ x: cam.x, y: cam.y, z: cam.z })
      },
      { source: 'user', scope: 'session' }
    )
    return unsub
  }, [editor])

  return null
}

export default function CanvasPage() {
  const [presets, setPresets] = useState([])
  const [tokens, setTokens] = useState([])
  const [loading, setLoading] = useState(true)
  const [selectedId, setSelectedId] = useState(null)
  const [inspectorTab, setInspectorTab] = useState('properties')
  const [showNewDraft, setShowNewDraft] = useState(false)
  const [draftName, setDraftName] = useState('')
  const [creating, setCreating] = useState(false)

  const selected = useMemo(
    () => presets.find(p => p.id === selectedId) || null,
    [presets, selectedId]
  )

  useEffect(() => {
    Promise.all([
      api.get('/canvas/presets').catch(() => []),
      api.get('/canvas/tokens').catch(() => []),
    ])
      .then(([p, t]) => {
        setPresets(Array.isArray(p) ? p : p.presets || [])
        setTokens(Array.isArray(t) ? t : t.tokens || [])
      })
      .finally(() => setLoading(false))
  }, [])

  const handleSelectPreset = useCallback((presetId) => {
    setSelectedId(presetId)
  }, [])

  async function handleCreateDraft(e) {
    e.preventDefault()
    if (!draftName.trim()) return
    setCreating(true)
    try {
      const created = await api.post('/canvas/presets', {
        name: draftName.trim().toLowerCase().replace(/\s+/g, '_'),
        category: 'product',
        entityType: 'product',
        description: '',
        defaultReplicate: true,
        ops: [],
      })
      setPresets((prev) => [...prev, created])
      setDraftName('')
      setShowNewDraft(false)
      setSelectedId(created.id)
    } catch (err) {
      alert(err.message)
    } finally {
      setCreating(false)
    }
  }

  if (loading) {
    return (
      <div className="canvas-loading">
        <Spinner />
      </div>
    )
  }

  return (
    <div className="canvas-shell">
      {/* ---- Left Panel: Preset Library ---- */}
      <aside className="canvas-left">
        <div className="canvas-left-header">
          <h2 className="canvas-panel-title">Presets</h2>
          <button
            className="canvas-btn-sm"
            onClick={() => setShowNewDraft(!showNewDraft)}
          >
            +
          </button>
        </div>

        {showNewDraft && (
          <form className="canvas-new-draft" onSubmit={handleCreateDraft}>
            <input
              className="canvas-input"
              placeholder="preset_name"
              value={draftName}
              onChange={(e) => setDraftName(e.target.value)}
              autoFocus
            />
            <button
              className="canvas-btn-sm"
              type="submit"
              disabled={creating}
            >
              {creating ? '...' : 'Create'}
            </button>
          </form>
        )}

        <div className="canvas-preset-list">
          {presets.map((p) => (
            <button
              key={p.id}
              className={`canvas-preset-item ${selectedId === p.id ? 'selected' : ''}`}
              onClick={() => setSelectedId(p.id)}
            >
              <span className="canvas-preset-name">{p.name}</span>
              <span className={`canvas-preset-status ${p.latestVersion?.status || p.status || 'draft'}`}>
                {p.latestVersion?.status || p.status || 'draft'}
              </span>
            </button>
          ))}
          {presets.length === 0 && (
            <div className="canvas-empty-hint">
              No presets yet. Create your first draft above.
            </div>
          )}
        </div>

        {/* Design Tokens summary */}
        {tokens.length > 0 && (
          <div className="canvas-tokens-summary">
            <h3 className="canvas-panel-subtitle">Design Tokens</h3>
            <div className="canvas-token-count">{tokens.length} tokens</div>
          </div>
        )}
      </aside>

      {/* ---- Center Panel: tldraw Canvas ---- */}
      <div className="canvas-center">
        <div className="canvas-topbar">
          <span className="canvas-topbar-title">
            {selected ? selected.name : 'Canvas'}
          </span>
          {selected && (
            <span className={`canvas-topbar-status ${selected.latestVersion?.status || 'draft'}`}>
              {selected.latestVersion?.status || 'draft'}
            </span>
          )}
        </div>
        <div className="canvas-viewport">
          <Tldraw
            shapeUtils={SHAPE_UTILS}
            hideUi
            onMount={(editor) => {
              // Disable all default tools except select/hand
              editor.updateInstanceState({ isReadonly: false })
            }}
          >
            <CanvasInner
              presets={presets}
              onSelectPreset={handleSelectPreset}
            />
          </Tldraw>
        </div>
      </div>

      {/* ---- Right Panel: Inspector ---- */}
      <aside className="canvas-right">
        <Tabs tabs={INSPECTOR_TABS} active={inspectorTab} onChange={setInspectorTab} />
        <div className="canvas-inspector-body">
          {inspectorTab === 'properties' && (
            <div className="canvas-inspector-section">
              {selected ? (
                <>
                  <div className="canvas-field">
                    <label className="canvas-field-label">Name</label>
                    <div className="canvas-field-value">{selected.name}</div>
                  </div>
                  <div className="canvas-field">
                    <label className="canvas-field-label">Category</label>
                    <div className="canvas-field-value">{selected.category}</div>
                  </div>
                  <div className="canvas-field">
                    <label className="canvas-field-label">Entity Type</label>
                    <div className="canvas-field-value">{selected.entityType || selected.entity_type || 'product'}</div>
                  </div>
                  <div className="canvas-field">
                    <label className="canvas-field-label">Description</label>
                    <div className="canvas-field-value">{selected.description || '(empty)'}</div>
                  </div>
                  <div className="canvas-field">
                    <label className="canvas-field-label">Default Replicate</label>
                    <div className="canvas-field-value">{selected.defaultReplicate || selected.default_replicate ? 'Yes' : 'No'}</div>
                  </div>
                </>
              ) : (
                <div className="canvas-empty-hint">Select a preset to inspect</div>
              )}
            </div>
          )}
          {inspectorTab === 'design' && (
            <div className="canvas-inspector-section">
              <div className="canvas-empty-hint">
                Design system overview (Phase 6+)
              </div>
            </div>
          )}
          {inspectorTab === 'data' && (
            <div className="canvas-inspector-section">
              <div className="canvas-empty-hint">
                Data binding inspector (Phase 6+)
              </div>
            </div>
          )}
        </div>
      </aside>
    </div>
  )
}
