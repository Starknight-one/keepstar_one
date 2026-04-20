import { LayoutGrid } from 'lucide-react'

export default function WidgetThumb({ title, sub, onClick }) {
  return (
    <div className="widget-thumb" onClick={onClick} role={onClick ? 'button' : undefined}>
      <div className="widget-thumb-icon"><LayoutGrid size={16} /></div>
      <div className="widget-thumb-meta">
        <span className="widget-thumb-title">{title}</span>
        {sub && <span className="widget-thumb-sub">{sub}</span>}
      </div>
    </div>
  )
}
