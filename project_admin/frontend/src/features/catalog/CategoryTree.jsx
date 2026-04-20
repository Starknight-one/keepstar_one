import { useState } from 'react'
import { ChevronRight, ChevronDown } from 'lucide-react'

const STATIC_TREE = [
  {
    name: 'All products',
    value: '',
    children: [],
  },
  {
    name: 'Skincare',
    value: 'skincare',
    children: [
      {
        name: 'Moisturizers',
        value: 'moisturizers',
        children: [
          { name: 'Face creams', value: 'face creams' },
          { name: 'Body lotions', value: 'body lotions' },
          { name: 'Hand creams', value: 'hand creams' },
        ],
      },
      { name: 'Cleansers', value: 'cleansers' },
      { name: 'Serums', value: 'serums' },
      { name: 'Toners', value: 'toners' },
      { name: 'Masks', value: 'masks' },
      { name: 'Sunscreen', value: 'sunscreen' },
    ],
  },
  {
    name: 'Makeup',
    value: 'makeup',
    children: [
      { name: 'Lip', value: 'lip' },
      { name: 'Eye', value: 'eye' },
      { name: 'Face', value: 'face' },
    ],
  },
  { name: 'Hair', value: 'hair' },
  { name: 'Fragrance', value: 'fragrance' },
]

export default function CategoryTree({ value, onChange }) {
  return (
    <aside className="category-tree">
      <div className="card-title">Categories</div>
      <div className="category-tree-list">
        {STATIC_TREE.map(node => (
          <TreeNode key={node.value || 'all'} node={node} value={value} onChange={onChange} depth={0} />
        ))}
      </div>
    </aside>
  )
}

function TreeNode({ node, value, onChange, depth }) {
  const [open, setOpen] = useState(depth < 1)
  const hasChildren = node.children && node.children.length > 0
  const active = value === node.value

  function handleClick() {
    onChange(node.value)
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
        <span>{node.name}</span>
      </button>
      {hasChildren && open && node.children.map(c => (
        <TreeNode key={c.value} node={c} value={value} onChange={onChange} depth={depth + 1} />
      ))}
    </>
  )
}
