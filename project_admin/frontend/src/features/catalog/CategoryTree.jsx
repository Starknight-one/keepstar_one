import { useState, useEffect, useMemo } from 'react'
import { ChevronRight, ChevronDown } from 'lucide-react'
import { api } from '../../shared/api/apiClient.js'

const ALL_NODE = { id: '', name: 'All products', parentId: '', productCount: null }

export default function CategoryTree({ value, onChange }) {
  const [categories, setCategories] = useState(null)
  const [error, setError] = useState(false)

  useEffect(() => {
    let cancelled = false
    api.get('/categories')
      .then(data => {
        if (cancelled) return
        setCategories(data.categories || [])
      })
      .catch(() => {
        if (cancelled) return
        setError(true)
        setCategories([])
      })
    return () => { cancelled = true }
  }, [])

  const tree = useMemo(() => {
    if (!categories) return null
    const byParent = new Map()
    for (const c of categories) {
      const p = c.parentId || ''
      if (!byParent.has(p)) byParent.set(p, [])
      byParent.get(p).push(c)
    }
    const build = (parentId) => {
      const kids = byParent.get(parentId) || []
      return kids.map(c => ({ ...c, children: build(c.id) }))
    }
    return [{ ...ALL_NODE, children: [] }, ...build('')]
  }, [categories])

  return (
    <aside className="category-tree">
      <div className="card-title">Categories</div>
      <div className="category-tree-list">
        {categories === null && !error && (
          <>
            <div className="category-tree-skeleton" />
            <div className="category-tree-skeleton" />
            <div className="category-tree-skeleton" />
          </>
        )}
        {error && <div className="category-tree-empty">Failed to load categories</div>}
        {tree && tree.map(node => (
          <TreeNode
            key={node.id || 'all'}
            node={node}
            selectedId={value?.id ?? ''}
            onChange={onChange}
            depth={0}
          />
        ))}
      </div>
    </aside>
  )
}

function TreeNode({ node, selectedId, onChange, depth }) {
  const [open, setOpen] = useState(depth < 1)
  const hasChildren = node.children && node.children.length > 0
  const active = selectedId === node.id

  function handleClick() {
    onChange({ id: node.id, name: node.name })
    if (hasChildren) setOpen(o => !o)
  }

  return (
    <>
      <button
        className={`category-tree-item ${active ? 'active' : ''}`}
        style={{ paddingLeft: 12 + depth * 14 }}
        onClick={handleClick}
        type="button"
      >
        {hasChildren
          ? (open ? <ChevronDown size={14} /> : <ChevronRight size={14} />)
          : <span className="category-tree-spacer" />}
        <span className="category-tree-label">{node.name}</span>
        {node.productCount != null && node.productCount > 0 && (
          <span className="category-tree-count">{node.productCount}</span>
        )}
      </button>
      {hasChildren && open && node.children.map(c => (
        <TreeNode
          key={c.id}
          node={c}
          selectedId={selectedId}
          onChange={onChange}
          depth={depth + 1}
        />
      ))}
    </>
  )
}
