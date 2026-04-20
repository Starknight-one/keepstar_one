import './ui.css'

export default function Button({ children, variant = 'primary', size = 'md', pill, className = '', disabled, ...props }) {
  const cls = ['btn', `btn-${variant}`, `btn-${size}`, pill && 'btn-pill', className].filter(Boolean).join(' ')
  return (
    <button className={cls} disabled={disabled} {...props}>
      {children}
    </button>
  )
}
