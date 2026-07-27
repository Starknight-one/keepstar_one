// Select — form-primitive leaf:
//   {type:"select", name, label?, placeholder?, required?, defaultValue?,
//    options:[{value, label} | "plain string"]}
//
// Options follow the value-set entry shape (R27: {value, label}, ordered
// by array position); bare strings are accepted as value===label so
// LLM-authored freestyle selects still render. Native <select> — no
// custom dropdown machinery, shadow-DOM safe.

import { useFormField, boundInitialValue } from '../form/useFormField'
import { validateSelectValue } from '../form/validate'
import { FieldLabel, FieldError } from './Input'

export default function Select({ node }) {
  const { value, error, disabled, onChange, onBlur } = useFormField(node.name, {
    initialValue: boundInitialValue(node),
    validate: (v) => validateSelectValue(node, v),
  })
  const options = normalizeOptions(node.options)

  return (
    <label className="kw-field" data-id={node.id || ''}>
      <FieldLabel node={node} />
      <select
        className="kw-select"
        name={node.name || ''}
        value={value}
        disabled={disabled || undefined}
        aria-required={node.required || undefined}
        aria-invalid={error ? true : undefined}
        data-invalid={error ? 'true' : ''}
        onChange={(e) => onChange(e.target.value)}
        onBlur={onBlur}
        onClick={(e) => e.stopPropagation()}
      >
        <option value="" disabled={node.required === true}>
          {node.placeholder || 'Select…'}
        </option>
        {options.map((opt) => (
          <option key={opt.value} value={opt.value}>
            {opt.label}
          </option>
        ))}
      </select>
      <FieldError error={error} />
    </label>
  )
}

function normalizeOptions(raw) {
  if (!Array.isArray(raw)) return []
  const out = []
  for (const o of raw) {
    if (typeof o === 'string') {
      out.push({ value: o, label: o })
    } else if (o && typeof o === 'object' && o.value !== undefined) {
      out.push({ value: String(o.value), label: o.label != null ? String(o.label) : String(o.value) })
    }
  }
  return out
}
