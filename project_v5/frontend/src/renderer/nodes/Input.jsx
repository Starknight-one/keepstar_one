// Input — form-primitive leaf (§5.2):
//   {type:"input", name, inputType:"text|email|password|tel|number|hidden",
//    label?, placeholder?, required?, defaultValue?, fieldBinding?,
//    validation?:{pattern?, minLength?, maxLength?, min?, max?}}
//
// Hidden inputs render nothing but still register their bound value
// (fieldBinding resolved server-side, e.g. booking_form's listingId)
// so it reaches the submit payload. Number inputs coerce to a real
// number in the payload; the control itself stays a string.
//
// Label association is by nesting (<label> wraps the control) — no
// generated ids, safe inside the shadow root.

import { useFormField, boundInitialValue } from '../form/useFormField'
import { validateInputValue } from '../form/validate'

const HTML_TYPES = ['text', 'email', 'password', 'tel', 'number']

export default function Input({ node }) {
  const inputType = node.inputType || 'text'
  const hidden = inputType === 'hidden'
  const { value, error, disabled, onChange, onBlur } = useFormField(node.name, {
    initialValue: boundInitialValue(node),
    validate: (v) => validateInputValue(node, v),
    coerce: inputType === 'number' ? coerceNumber : undefined,
    hidden,
  })

  if (hidden) return null

  return (
    <label className="kw-field" data-id={node.id || ''}>
      <FieldLabel node={node} />
      <input
        className="kw-input"
        type={HTML_TYPES.includes(inputType) ? inputType : 'text'}
        name={node.name || ''}
        value={value}
        placeholder={node.placeholder || ''}
        disabled={disabled || undefined}
        aria-required={node.required || undefined}
        aria-invalid={error ? true : undefined}
        data-invalid={error ? 'true' : ''}
        autoComplete={inputType === 'password' ? 'new-password' : undefined}
        onChange={(e) => onChange(e.target.value)}
        onBlur={onBlur}
        onClick={(e) => e.stopPropagation()}
      />
      <FieldError error={error} />
    </label>
  )
}

// FieldLabel / FieldError — shared presentation bits for every form
// leaf (Select / DateInput / Textarea import them from here).
export function FieldLabel({ node }) {
  if (!node.label) return null
  return (
    <span className="kw-field-label">
      {node.label}
      {node.required ? (
        <span className="kw-field-required" aria-hidden="true">
          {' *'}
        </span>
      ) : null}
    </span>
  )
}

export function FieldError({ error }) {
  if (!error) return null
  return (
    <span className="kw-field-error" role="alert">
      {error}
    </span>
  )
}

function coerceNumber(v) {
  if (v === '' || v === null || v === undefined) return v
  const n = Number(v)
  return Number.isFinite(n) ? n : v
}
