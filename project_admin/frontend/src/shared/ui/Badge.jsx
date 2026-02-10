import './ui.css'

const COLORS = {
  pending: '#f59e0b',
  processing: '#2563eb',
  completed: '#22c55e',
  failed: '#ef4444',
  default: '#6b7280',
}

export default function Badge({ status, children }) {
  const color = COLORS[status] || COLORS.default
  return (
    <span className="badge" style={{ background: `${color}15`, color, borderColor: `${color}30` }}>
      {children || status}
    </span>
  )
}
