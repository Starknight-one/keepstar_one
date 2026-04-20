import './ui.css'

export default function ChipGroup({ options, value, onChange }) {
  return (
    <div className="chip-group" role="tablist">
      {options.map(opt => (
        <button
          key={opt.value}
          role="tab"
          aria-selected={value === opt.value}
          className={`chip ${value === opt.value ? 'chip-active' : ''}`}
          onClick={() => onChange(opt.value)}
          type="button"
        >
          {opt.label}
          {opt.count != null && <span className="chip-count">{opt.count}</span>}
        </button>
      ))}
    </div>
  )
}
