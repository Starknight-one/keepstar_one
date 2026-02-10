import './ui.css'

export default function Button({ children, variant = 'primary', size = 'md', disabled, ...props }) {
  return (
    <button
      className={`btn btn-${variant} btn-${size}`}
      disabled={disabled}
      {...props}
    >
      {children}
    </button>
  )
}
