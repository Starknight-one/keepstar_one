// useFormField — the one hook every form-primitive leaf uses to plug
// into the enclosing FormProvider. Handles registration (so submit can
// validate + collect untouched fields), controlled value/error wiring,
// and blur-time validation.
//
// Degrades gracefully outside a provider (freestyle LLM-emitted form
// nodes with no formId frame): the field keeps purely local state so it
// still renders and validates — it just has nowhere to submit to.

import { useEffect, useRef, useState } from 'react'
import { useFormContext } from './FormContext'

export function useFormField(name, { initialValue = '', validate, coerce, hidden = false } = {}) {
  const form = useFormContext()
  const [local, setLocal] = useState({ value: initialValue, error: '' })

  // Validators close over node props that may change between renders —
  // keep the registered callbacks stable and route through refs.
  const validateRef = useRef(validate)
  validateRef.current = validate
  const coerceRef = useRef(coerce)
  coerceRef.current = coerce

  const registerField = form?.registerField
  const unregisterField = form?.unregisterField
  useEffect(() => {
    if (!registerField || !name) return undefined
    registerField(name, {
      initialValue,
      validate: (v) => (validateRef.current ? validateRef.current(v) : ''),
      coerce: (v) => (coerceRef.current ? coerceRef.current(v) : v),
      hidden,
    })
    return () => unregisterField?.(name)
    // initialValue is a mount-time seed by design — re-registering on
    // every keystroke-driven prop echo would thrash the registry.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [registerField, unregisterField, name, hidden])

  if (form && name) {
    const value = form.values[name] ?? initialValue
    return {
      value,
      error: form.errors[name] || '',
      disabled: form.status === 'submitting',
      onChange: (v) => form.setValue(name, v),
      onBlur: () => {
        const msg = validateRef.current ? validateRef.current(form.values[name] ?? initialValue) : ''
        if (msg) form.setFieldError(name, msg)
      },
    }
  }

  // Standalone fallback (no provider or nameless node).
  return {
    value: local.value,
    error: local.error,
    disabled: false,
    onChange: (v) => setLocal((prev) => ({ ...prev, value: v })),
    onBlur: () => {
      const msg = validateRef.current ? validateRef.current(local.value) : ''
      setLocal((prev) => ({ ...prev, error: msg }))
    },
  }
}

// boundInitialValue — the node's pre-filled value. The engine's
// BindData resolves fieldBinding into the node before the document
// reaches the widget (hidden inputs bind ids, §5.2); depending on the
// binding target that lands as `value` or — leaf convention, like
// text nodes — as `content`. `defaultValue` is the authored fallback.
export function boundInitialValue(node) {
  const v = firstDefined(node?.value, node?.defaultValue, node?.content)
  if (v === undefined || v === null) return ''
  return typeof v === 'string' ? v : String(v)
}

function firstDefined(...xs) {
  for (const x of xs) {
    if (x !== undefined && x !== null && x !== '') return x
  }
  return undefined
}
