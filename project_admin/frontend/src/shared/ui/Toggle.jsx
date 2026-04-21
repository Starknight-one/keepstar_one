import './ui.css'

export default function Toggle({ checked, onChange, disabled }) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      disabled={disabled}
      onClick={() => !disabled && onChange?.(!checked)}
      className={`toggle ${checked ? 'toggle-on' : ''}`}
    >
      <span className="toggle-dot" />
    </button>
  )
}
