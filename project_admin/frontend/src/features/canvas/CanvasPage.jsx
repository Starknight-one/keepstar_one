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

const CATEGORY_OPTIONS = ['product', 'service', 'nav', 'system']
const ENTITY_TYPE_OPTIONS = ['product', 'service']

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
function CanvasInner({ presets, onSelectPreset, editorRef }) {
  const editor = useEditor()
  const mountedRef = useRef(false)

  // Expose editor to parent via ref
  useEffect(() => {
    if (editor) editorRef.current = editor
  }, [editor, editorRef])

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
        const ops = p.latestVersion?.ops_json || p.latestVersion?.ops
        if (ops) opsCount = (typeof ops === 'string' ? JSON.parse(ops) : ops).length
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

/** Find the tldraw shape ID for a given presetId */
function findShapeByPresetId(editor, presetId) {
  if (!editor || !presetId) return null
  return editor.getCurrentPageShapes().find(
    s => s.type === PRESET_TILE_TYPE && s.props.presetId === presetId
  )
}

/** Update the tldraw tile's visual props to match updated preset data */
function syncTileProps(editor, presetId, updates) {
  const shape = findShapeByPresetId(editor, presetId)
  if (!shape) return
  editor.updateShapes([{
    id: shape.id,
    type: PRESET_TILE_TYPE,
    props: updates,
  }])
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
  const [saving, setSaving] = useState(false)
  const [publishing, setPublishing] = useState(false)
  const editorRef = useRef(null)

  const selected = useMemo(
    () => presets.find(p => p.id === selectedId) || null,
    [presets, selectedId]
  )

  const selectedStatus = selected?.latestVersion?.status || selected?.status || 'draft'

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

  // --- Optimistic field update + PUT ---
  const updatePresetField = useCallback(async (field, value) => {
    if (!selected) return
    const prev = selected[field]
    if (prev === value) return

    // Optimistic local update
    setPresets(list => list.map(p =>
      p.id === selected.id ? { ...p, [field]: value } : p
    ))

    // Optimistic tile update (map field names to tile prop names)
    const tileField = field === 'defaultReplicate' || field === 'default_replicate'
      ? 'defaultReplicate' : field
    if (['name', 'category', 'description', 'defaultReplicate'].includes(tileField)) {
      syncTileProps(editorRef.current, selected.id, { [tileField]: value })
    }

    // Persist
    setSaving(true)
    try {
      await api.put(`/canvas/presets/${selected.id}`, { [field]: value })
    } catch (err) {
      // Rollback on failure
      setPresets(list => list.map(p =>
        p.id === selected.id ? { ...p, [field]: prev } : p
      ))
      if (['name', 'category', 'description', 'defaultReplicate'].includes(tileField)) {
        syncTileProps(editorRef.current, selected.id, { [tileField]: prev })
      }
      alert(`Save failed: ${err.message}`)
    } finally {
      setSaving(false)
    }
  }, [selected])

  // --- Publish ---
  const handlePublish = useCallback(async () => {
    if (!selected) return
    setPublishing(true)
    try {
      const updated = await api.post(`/canvas/presets/${selected.id}/publish`)
      setPresets(list => list.map(p => p.id === selected.id ? updated : p))
      syncTileProps(editorRef.current, selected.id, {
        status: 'published',
      })
    } catch (err) {
      alert(`Publish failed: ${err.message}`)
    } finally {
      setPublishing(false)
    }
  }, [selected])

  // --- Delete ---
  const handleDelete = useCallback(async () => {
    if (!selected) return
    if (!confirm(`Delete preset "${selected.name}"? This cannot be undone.`)) return
    try {
      await api.delete(`/canvas/presets/${selected.id}`)
      // Remove tile from canvas
      const shape = findShapeByPresetId(editorRef.current, selected.id)
      if (shape) editorRef.current.deleteShapes([shape.id])
      // Remove from list
      setPresets(list => list.filter(p => p.id !== selected.id))
      setSelectedId(null)
    } catch (err) {
      alert(`Delete failed: ${err.message}`)
    }
  }, [selected])

  // --- Create draft ---
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

      // Add tile to canvas
      const editor = editorRef.current
      if (editor) {
        const count = editor.getCurrentPageShapes()
          .filter(s => s.type === PRESET_TILE_TYPE).length
        const col = count % TILES_PER_ROW
        const row = Math.floor(count / TILES_PER_ROW)
        editor.createShapes([{
          id: createShapeId(`preset-${created.id}`),
          type: PRESET_TILE_TYPE,
          x: col * (TILE_W + TILE_GAP) + 80,
          y: row * (TILE_H + TILE_GAP) + 80,
          props: {
            w: TILE_W,
            h: TILE_H,
            presetId: created.id,
            name: created.name,
            category: created.category || 'product',
            description: created.description || '',
            status: 'draft',
            defaultReplicate: !!(created.defaultReplicate ?? created.default_replicate),
            opsCount: 0,
          },
        }])
      }
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
            <span className={`canvas-topbar-status ${selectedStatus}`}>
              {selectedStatus}
            </span>
          )}
          {selected && (
            <div className="canvas-topbar-actions">
              {saving && <span className="canvas-topbar-saving">Saving...</span>}
              {selectedStatus === 'draft' && (
                <button
                  className="canvas-btn-publish"
                  onClick={handlePublish}
                  disabled={publishing}
                >
                  {publishing ? 'Publishing...' : 'Publish'}
                </button>
              )}
              <button
                className="canvas-btn-delete"
                onClick={handleDelete}
              >
                Delete
              </button>
            </div>
          )}
        </div>
        <div className="canvas-viewport">
          <Tldraw
            shapeUtils={SHAPE_UTILS}
            hideUi
            onMount={(editor) => {
              editor.updateInstanceState({ isReadonly: false })
            }}
          >
            <CanvasInner
              presets={presets}
              onSelectPreset={handleSelectPreset}
              editorRef={editorRef}
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
                    <input
                      className="canvas-input"
                      value={selected.name}
                      onChange={(e) => {
                        const val = e.target.value.toLowerCase().replace(/\s+/g, '_')
                        setPresets(list => list.map(p =>
                          p.id === selected.id ? { ...p, name: val } : p
                        ))
                        syncTileProps(editorRef.current, selected.id, { name: val })
                      }}
                      onBlur={(e) => {
                        updatePresetField('name', e.target.value)
                      }}
                    />
                  </div>
                  <div className="canvas-field">
                    <label className="canvas-field-label">Category</label>
                    <select
                      className="canvas-select"
                      value={selected.category}
                      onChange={(e) => updatePresetField('category', e.target.value)}
                    >
                      {CATEGORY_OPTIONS.map(c => (
                        <option key={c} value={c}>{c}</option>
                      ))}
                    </select>
                  </div>
                  <div className="canvas-field">
                    <label className="canvas-field-label">Entity Type</label>
                    <select
                      className="canvas-select"
                      value={selected.entityType || selected.entity_type || 'product'}
                      onChange={(e) => updatePresetField('entityType', e.target.value)}
                    >
                      {ENTITY_TYPE_OPTIONS.map(t => (
                        <option key={t} value={t}>{t}</option>
                      ))}
                    </select>
                  </div>
                  <div className="canvas-field">
                    <label className="canvas-field-label">Description</label>
                    <textarea
                      className="canvas-textarea"
                      value={selected.description || ''}
                      rows={3}
                      placeholder="Describe this preset..."
                      onChange={(e) => {
                        const val = e.target.value
                        setPresets(list => list.map(p =>
                          p.id === selected.id ? { ...p, description: val } : p
                        ))
                        syncTileProps(editorRef.current, selected.id, { description: val })
                      }}
                      onBlur={(e) => updatePresetField('description', e.target.value)}
                    />
                  </div>
                  <div className="canvas-field">
                    <label className="canvas-field-label">Default Replicate</label>
                    <label className="canvas-checkbox-label">
                      <input
                        type="checkbox"
                        checked={!!(selected.defaultReplicate ?? selected.default_replicate)}
                        onChange={(e) => updatePresetField('defaultReplicate', e.target.checked)}
                      />
                      <span>Replicate widget for each data item</span>
                    </label>
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
                Design system overview (Phase 8)
              </div>
            </div>
          )}
          {inspectorTab === 'data' && (
            <div className="canvas-inspector-section">
              <div className="canvas-empty-hint">
                Data binding inspector (Phase 7+)
              </div>
            </div>
          )}
        </div>
      </aside>
    </div>
  )
}
