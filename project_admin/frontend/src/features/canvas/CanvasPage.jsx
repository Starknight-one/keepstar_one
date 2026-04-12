import { useState, useEffect } from 'react'
import { api } from '../../shared/api/apiClient.js'
import Spinner from '../../shared/ui/Spinner.jsx'
import Tabs from '../../shared/ui/Tabs.jsx'
import './canvas.css'

const INSPECTOR_TABS = [
  { key: 'properties', label: 'Properties' },
  { key: 'design', label: 'Design System' },
  { key: 'data', label: 'Data' },
]

export default function CanvasPage() {
  const [presets, setPresets] = useState([])
  const [tokens, setTokens] = useState([])
  const [loading, setLoading] = useState(true)
  const [selected, setSelected] = useState(null)
  const [inspectorTab, setInspectorTab] = useState('properties')
  const [showNewDraft, setShowNewDraft] = useState(false)
  const [draftName, setDraftName] = useState('')
  const [creating, setCreating] = useState(false)

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
      setSelected(created)
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
              className={`canvas-preset-item ${selected?.id === p.id ? 'selected' : ''}`}
              onClick={() => setSelected(p)}
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

      {/* ---- Center Panel: Canvas Placeholder ---- */}
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
          {selected ? (
            <div className="canvas-placeholder-card">
              <div className="canvas-placeholder-icon">
                <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5">
                  <rect x="3" y="3" width="18" height="18" rx="2" />
                  <path d="M3 9h18M9 21V9" />
                </svg>
              </div>
              <p className="canvas-placeholder-text">
                Canvas editor will render here (Phase 5: tldraw integration)
              </p>
              <p className="canvas-placeholder-sub">
                Selected: <strong>{selected.name}</strong>
                {selected.latestVersion?.ops_json && (
                  <> &middot; {JSON.parse(selected.latestVersion.ops_json || '[]').length} ops</>
                )}
              </p>
            </div>
          ) : (
            <div className="canvas-placeholder-card">
              <div className="canvas-placeholder-icon">
                <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5">
                  <path d="M12 5v14M5 12h14" />
                </svg>
              </div>
              <p className="canvas-placeholder-text">
                Select a preset from the left panel or create a new draft
              </p>
            </div>
          )}
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
