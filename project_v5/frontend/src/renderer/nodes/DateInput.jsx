// DateInput — the `datetime` form-primitive leaf (§5.2):
//   {type:"datetime", name, mode:"date|time|datetime", label?, required?,
//    minOffsetHours?, hours?:{from,to}}
//
// Renders the matching native control (date / time / datetime-local).
// minOffsetHours doubles as the native `min` attribute so the picker
// itself grays out the past; the same guard plus the hours window run
// as blur/submit validation. Both are client-advisory — the
// schedule_slot server guards are the authority (R11).
//
// Payload format: the native input string with seconds appended for
// time-bearing modes ("YYYY-MM-DDTHH:MM:SS" local, no timezone) —
// ISO-8601, wall-clock semantics; server-side coercion parses it.

import { useFormField, boundInitialValue } from '../form/useFormField'
import { validateDatetimeValue } from '../form/validate'
import { FieldLabel, FieldError } from './Input'

const MODE_TO_HTML = { date: 'date', time: 'time', datetime: 'datetime-local' }

export default function DateInput({ node }) {
  const mode = MODE_TO_HTML[node.mode] ? node.mode : 'datetime'
  const { value, error, disabled, onChange, onBlur } = useFormField(node.name, {
    initialValue: boundInitialValue(node),
    validate: (v) => validateDatetimeValue({ ...node, mode }, v),
    coerce: (v) => coerceDatetime(v, mode),
  })

  return (
    <label className="kw-field" data-id={node.id || ''}>
      <FieldLabel node={node} />
      <input
        className="kw-input"
        type={MODE_TO_HTML[mode]}
        name={node.name || ''}
        value={value}
        min={minAttr(node, mode)}
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

function minAttr(node, mode) {
  if (mode === 'time' || typeof node.minOffsetHours !== 'number') return undefined
  const d = new Date(Date.now() + node.minOffsetHours * 3600 * 1000)
  return toLocalInputValue(d, mode)
}

function toLocalInputValue(d, mode) {
  const pad = (n) => String(n).padStart(2, '0')
  const date = `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
  if (mode === 'date') return date
  return `${date}T${pad(d.getHours())}:${pad(d.getMinutes())}`
}

function coerceDatetime(v, mode) {
  if (typeof v !== 'string' || v === '' || mode === 'date') return v
  // "HH:MM" / "YYYY-MM-DDTHH:MM" → append seconds once.
  return /:\d{2}$/.test(v) && !/:\d{2}:\d{2}$/.test(v) ? `${v}:00` : v
}
