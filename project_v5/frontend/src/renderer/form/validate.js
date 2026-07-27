// Client-side field validation for form primitive nodes. Rules derive
// ONLY from node props (required / inputType / validation / datetime
// guards) — the server-side operation validator is always the
// authority (R11: schedule_slot guards, §4.1 registry validate.go);
// this layer exists so obvious mistakes never cost a round-trip.
//
// Every validator returns '' when the value passes, else a
// human-readable English message rendered under the field.

const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/

export function validateInputValue(node, raw) {
  const value = typeof raw === 'string' ? raw.trim() : raw == null ? '' : String(raw)
  if (value === '') {
    return node?.required ? requiredMessage(node) : ''
  }
  const v = node?.validation || {}
  switch (node?.inputType) {
    case 'email':
      if (!EMAIL_RE.test(value)) return 'Enter a valid email address.'
      break
    case 'tel': {
      // Advisory E.164 shape (R18 phone_e164): digits with optional +,
      // separators tolerated. The server normalizes.
      const digits = value.replace(/[\s\-().]/g, '')
      if (!/^\+?\d{7,15}$/.test(digits)) return 'Enter a valid phone number.'
      break
    }
    case 'number': {
      const n = Number(value)
      if (!Number.isFinite(n)) return 'Enter a number.'
      if (typeof v.min === 'number' && n < v.min) return `Must be at least ${v.min}.`
      if (typeof v.max === 'number' && n > v.max) return `Must be at most ${v.max}.`
      break
    }
    default:
      break
  }
  if (typeof v.minLength === 'number' && value.length < v.minLength) {
    return `Must be at least ${v.minLength} characters.`
  }
  if (typeof v.maxLength === 'number' && value.length > v.maxLength) {
    return `Must be at most ${v.maxLength} characters.`
  }
  if (typeof v.pattern === 'string' && v.pattern !== '') {
    try {
      if (!new RegExp(v.pattern).test(value)) return 'Invalid format.'
    } catch (_) {
      // Malformed pattern in a document must not brick the form —
      // skip the check, the server validator still guards.
      // eslint-disable-next-line no-console
      console.warn('[v5-form] invalid validation.pattern on node', node?.id, v.pattern)
    }
  }
  return ''
}

export function validateTextareaValue(node, raw) {
  return validateInputValue({ ...node, inputType: 'text' }, raw)
}

export function validateSelectValue(node, raw) {
  const value = raw == null ? '' : String(raw)
  if (value === '') {
    return node?.required ? requiredMessage(node, 'Select an option.') : ''
  }
  return ''
}

// validateDatetimeValue — datetime node (§5.2): mode date|time|datetime,
// guards minOffsetHours (value at least N hours in the future) and
// hours:{from,to} (business-hours window). Both are client-advisory
// copies of the schedule_slot server guards (R11) and only apply when
// the node carries them. Values are the native input formats:
//   date → YYYY-MM-DD, time → HH:MM, datetime → YYYY-MM-DDTHH:MM.
export function validateDatetimeValue(node, raw) {
  const value = typeof raw === 'string' ? raw.trim() : raw == null ? '' : String(raw)
  if (value === '') {
    return node?.required ? requiredMessage(node, 'Choose a date and time.') : ''
  }
  const mode = node?.mode === 'date' || node?.mode === 'time' ? node.mode : 'datetime'

  if (mode !== 'date' && node?.hours && typeof node.hours.from === 'number' && typeof node.hours.to === 'number') {
    const hour = hourOf(value, mode)
    if (hour !== null && (hour < node.hours.from || hour >= node.hours.to)) {
      return `Choose a time between ${pad(node.hours.from)}:00 and ${pad(node.hours.to)}:00.`
    }
  }

  if (mode !== 'time' && typeof node?.minOffsetHours === 'number') {
    const t = Date.parse(value)
    if (!Number.isNaN(t) && t - Date.now() < node.minOffsetHours * 3600 * 1000) {
      return node.minOffsetHours <= 0
        ? 'Choose a time in the future.'
        : `Choose a time at least ${node.minOffsetHours} hours from now.`
    }
  }
  return ''
}

function hourOf(value, mode) {
  const timePart = mode === 'time' ? value : value.split('T')[1]
  if (!timePart) return null
  const hour = parseInt(timePart.slice(0, 2), 10)
  return Number.isFinite(hour) ? hour : null
}

function requiredMessage(node, fallback = 'This field is required.') {
  return node?.label ? `${node.label} is required.` : fallback
}

function pad(n) {
  return String(n).padStart(2, '0')
}
