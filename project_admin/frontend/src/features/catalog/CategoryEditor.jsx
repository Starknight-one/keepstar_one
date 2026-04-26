import { useState, useEffect, useCallback, useMemo } from 'react'
import { Plus, Edit2, Trash2, Save, X, ChevronRight, ChevronDown } from 'lucide-react'
import { api } from '../../shared/api/apiClient.js'
import Button from '../../shared/ui/Button.jsx'
import Spinner from '../../shared/ui/Spinner.jsx'
import './catalog.css'
import './categoryEditor.css'

const KIND_OPTIONS = [
  { value: 'category', label: 'Category' },
  { value: 'showcase', label: 'Showcase' },
  { value: 'promo', label: 'Promo' },
]

function buildTree(flat) {
  const byId = new Map(flat.map((c) => [c.id, { ...c, children: [] }]))
  const roots = []
  for (const node of byId.values()) {
    if (node.parentId && byId.has(node.parentId)) {
      byId.get(node.parentId).children.push(node)
    } else {
      roots.push(node)
    }
  }
  const sortRec = (xs) => {
    xs.sort((a, b) => a.name.localeCompare(b.name))
    xs.forEach((n) => sortRec(n.children))
  }
  sortRec(roots)
  return roots
}

function slugify(s) {
  return s
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 80)
}

export default function CategoryEditor() {
  const [categories, setCategories] = useState([])
  const [loading, setLoading] = useState(true)
  const [collapsed, setCollapsed] = useState(new Set())
  const [editing, setEditing] = useState(null) // { id, name, kind, parentId }
  const [creating, setCreating] = useState(null) // { parentId } | null
  const [error, setError] = useState('')

  const refresh = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      const data = await api.get('/categories/tenant')
      setCategories(data.categories || [])
    } catch (e) {
      setError(e.message || 'Failed to load categories')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { refresh() }, [refresh])

  const tree = useMemo(() => buildTree(categories), [categories])
  const parentOptions = useMemo(
    () => [{ id: '', name: '— top level —' }, ...categories.map((c) => ({ id: c.id, name: c.name }))],
    [categories],
  )

  function toggle(id) {
    setCollapsed((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  async function handleCreate(payload) {
    setError('')
    try {
      await api.post('/categories/tenant', payload)
      setCreating(null)
      refresh()
    } catch (e) {
      setError(e.message || 'Failed to create')
    }
  }

  async function handleUpdate(id, payload) {
    setError('')
    try {
      await api.patch(`/categories/tenant/${id}`, payload)
      setEditing(null)
      refresh()
    } catch (e) {
      setError(e.message || 'Failed to update')
    }
  }

  async function handleDelete(id) {
    if (!confirm('Delete this category? Children will become top-level.')) return
    setError('')
    try {
      await api.del(`/categories/tenant/${id}`)
      refresh()
    } catch (e) {
      setError(e.message || 'Failed to delete')
    }
  }

  if (loading) return <div className="center-spinner"><Spinner /></div>

  return (
    <div className="category-editor">
      <div className="page-header">
        <div>
          <div className="page-title">Categories</div>
          <div className="catalog-breadcrumb">Catalog / <strong>Categories</strong></div>
        </div>
        <Button variant="primary" pill onClick={() => setCreating({ parentId: '' })}>
          <Plus size={14} /> Add category
        </Button>
      </div>

      {error && <div className="pd-message error">{error}</div>}

      {creating && (
        <CategoryForm
          mode="create"
          initial={{ name: '', slug: '', kind: 'category', parentId: creating.parentId }}
          parents={parentOptions}
          onSubmit={(payload) => handleCreate(payload)}
          onCancel={() => setCreating(null)}
        />
      )}

      {tree.length === 0 && !creating && (
        <div className="ce-empty">
          <p>No categories yet.</p>
          <p className="ce-empty-hint">
            Categories will be auto-populated from Shopify collections after import. You can also
            create them manually here.
          </p>
        </div>
      )}

      <ul className="ce-tree">
        {tree.map((node) => (
          <CategoryNode
            key={node.id}
            node={node}
            depth={0}
            collapsed={collapsed}
            editing={editing}
            parents={parentOptions}
            onToggle={toggle}
            onAddChild={(parentId) => setCreating({ parentId })}
            onStartEdit={(n) => setEditing({ id: n.id, name: n.name, slug: n.slug, kind: n.kind, parentId: n.parentId || '' })}
            onCancelEdit={() => setEditing(null)}
            onSaveEdit={(id, payload) => handleUpdate(id, payload)}
            onDelete={handleDelete}
          />
        ))}
      </ul>
    </div>
  )
}

function CategoryNode({ node, depth, collapsed, editing, parents, onToggle, onAddChild, onStartEdit, onCancelEdit, onSaveEdit, onDelete }) {
  const isCollapsed = collapsed.has(node.id)
  const hasChildren = node.children.length > 0
  const isEditing = editing?.id === node.id

  return (
    <li className="ce-node" style={{ '--depth': depth }}>
      <div className="ce-row">
        <button
          className="ce-toggle"
          onClick={() => onToggle(node.id)}
          disabled={!hasChildren}
          aria-label={isCollapsed ? 'Expand' : 'Collapse'}
        >
          {hasChildren ? (isCollapsed ? <ChevronRight size={14} /> : <ChevronDown size={14} />) : <span style={{ width: 14 }} />}
        </button>
        {isEditing ? (
          <CategoryForm
            mode="edit"
            initial={editing}
            parents={parents.filter((p) => p.id !== node.id)}
            inline
            onSubmit={(payload) => onSaveEdit(node.id, payload)}
            onCancel={onCancelEdit}
          />
        ) : (
          <>
            <div className="ce-name">{node.name}</div>
            <span className={`ce-kind ce-kind-${node.kind}`}>{node.kind}</span>
            <span className="ce-count">{node.productCount} items</span>
            <div className="ce-actions">
              <button className="ce-action" title="Add child" onClick={() => onAddChild(node.id)}>
                <Plus size={13} />
              </button>
              <button className="ce-action" title="Edit" onClick={() => onStartEdit(node)}>
                <Edit2 size={13} />
              </button>
              <button className="ce-action ce-action-danger" title="Delete" onClick={() => onDelete(node.id)}>
                <Trash2 size={13} />
              </button>
            </div>
          </>
        )}
      </div>
      {hasChildren && !isCollapsed && (
        <ul className="ce-tree">
          {node.children.map((child) => (
            <CategoryNode
              key={child.id}
              node={child}
              depth={depth + 1}
              collapsed={collapsed}
              editing={editing}
              parents={parents}
              onToggle={onToggle}
              onAddChild={onAddChild}
              onStartEdit={onStartEdit}
              onCancelEdit={onCancelEdit}
              onSaveEdit={onSaveEdit}
              onDelete={onDelete}
            />
          ))}
        </ul>
      )}
    </li>
  )
}

function CategoryForm({ mode, initial, parents, inline, onSubmit, onCancel }) {
  const [name, setName] = useState(initial.name || '')
  const [slug, setSlug] = useState(initial.slug || '')
  const [kind, setKind] = useState(initial.kind || 'category')
  const [parentId, setParentId] = useState(initial.parentId || '')
  const [touched, setTouched] = useState(Boolean(initial.slug))

  function submit(e) {
    e?.preventDefault?.()
    const finalSlug = slug || slugify(name)
    if (!name || !finalSlug) return
    onSubmit({ name, slug: finalSlug, kind, parentId: parentId || '' })
  }

  return (
    <form className={`ce-form ${inline ? 'ce-form-inline' : ''}`} onSubmit={submit}>
      <input
        className="ce-input"
        placeholder="Name"
        value={name}
        onChange={(e) => { setName(e.target.value); if (!touched) setSlug(slugify(e.target.value)) }}
        autoFocus
      />
      <input
        className="ce-input"
        placeholder="slug"
        value={slug}
        onChange={(e) => { setSlug(e.target.value); setTouched(true) }}
      />
      <select className="ce-input" value={kind} onChange={(e) => setKind(e.target.value)}>
        {KIND_OPTIONS.map((o) => <option key={o.value} value={o.value}>{o.label}</option>)}
      </select>
      <select className="ce-input" value={parentId} onChange={(e) => setParentId(e.target.value)}>
        {parents.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}
      </select>
      <button type="submit" className="ce-action ce-action-primary" title="Save"><Save size={13} /></button>
      <button type="button" className="ce-action" title="Cancel" onClick={onCancel}><X size={13} /></button>
    </form>
  )
}
