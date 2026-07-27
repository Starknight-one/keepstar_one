// Textarea — form-primitive leaf:
//   {type:"textarea", name, label?, placeholder?, required?,
//    defaultValue?, rows?, validation?:{minLength?, maxLength?}}
//
// Native <textarea>, controlled, same field wiring as Input.

import { useFormField, boundInitialValue } from '../form/useFormField'
import { validateTextareaValue } from '../form/validate'
import { FieldLabel, FieldError } from './Input'

export default function Textarea({ node }) {
  const { value, error, disabled, onChange, onBlur } = useFormField(node.name, {
    initialValue: boundInitialValue(node),
    validate: (v) => validateTextareaValue(node, v),
  })

  return (
    <label className="kw-field" data-id={node.id || ''}>
      <FieldLabel node={node} />
      <textarea
        className="kw-textarea"
        name={node.name || ''}
        value={value}
        placeholder={node.placeholder || ''}
        rows={typeof node.rows === 'number' && node.rows > 0 ? node.rows : 4}
        disabled={disabled || undefined}
        aria-required={node.required || undefined}
        aria-invalid={error ? true : undefined}
        data-invalid={error ? 'true' : ''}
        onChange={(e) => onChange(e.target.value)}
        onBlur={onBlur}
        onClick={(e) => e.stopPropagation()}
      />
      <FieldError error={error} />
    </label>
  )
}
