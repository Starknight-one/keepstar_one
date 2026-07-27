// Form state module (RUNTIME_SPEC §5.2). A frame node carrying `formId`
// is wrapped by NodeRenderer in a FormProvider: one React context
// holding {values, errors, status, message} for every form-primitive
// leaf (input / select / datetime / textarea / submit) beneath it.
//
// Submit flow (§4.8 + R6):
//   operation_invoke → POST /operations/invoke with the operation name
//                      from the submit node's action props + collected
//                      values as params;
//   form_submit      → POST the document-specified same-origin endpoint
//                      (registration → /api/v1/onboard/step/{id}/submit;
//                      credentials NEVER go through the operation
//                      registry).
// Both resolve to the R1 apply[] contract, applied here:
//   {target:"form", formId, status, message} → form status line;
//   {target:"block", blockId, document}      → ctx.onReplaceBlock when
//     the shell provides it (blocks renderer), else the storefront
//     fallback ctx.onUpdateDocument — the returned document (e.g.
//     success_plaque) replaces the current view.
//
// Shadow-DOM safe: pure React state, no document.* queries, no element
// ids. Renders a native <form> (display:contents — layout-neutral) so
// Enter-to-submit works; validation is ours (noValidate), native
// browser popups stay off.

import { createContext, useCallback, useContext, useMemo, useRef, useState } from 'react'
import { useRenderContext } from '../RenderContext'
import { invokeOperation, submitFormEndpoint } from '../../api/operations'

const FormContext = createContext(null)

export function useFormContext() {
  return useContext(FormContext)
}

export function FormProvider({ node, children }) {
  const renderCtx = useRenderContext()
  const formId = node?.formId || node?.id || 'form'

  // Field registry lives in a ref: registration happens on mount effects
  // and must not re-render the form. Each entry: {validate, coerce,
  // hidden}. Values/errors are state — they drive the controls.
  const fieldsRef = useRef(new Map())
  const submitActionRef = useRef(null)
  const inFlightRef = useRef(false)

  const [values, setValues] = useState({})
  const [errors, setErrors] = useState({})
  const [status, setStatus] = useState('idle') // idle | submitting | success | error
  const [message, setMessage] = useState('')

  const registerField = useCallback((name, { initialValue = '', validate, coerce, hidden = false }) => {
    fieldsRef.current.set(name, { validate, coerce, hidden })
    // Seed the value once so untouched fields (hidden bound ids,
    // defaults) still reach the submit payload.
    setValues((prev) => (name in prev ? prev : { ...prev, [name]: initialValue }))
  }, [])

  const unregisterField = useCallback((name) => {
    fieldsRef.current.delete(name)
  }, [])

  const setValue = useCallback((name, value) => {
    setValues((prev) => ({ ...prev, [name]: value }))
    // Typing clears the field's stale error.
    setErrors((prev) => (prev[name] ? { ...prev, [name]: '' } : prev))
  }, [])

  const setFieldError = useCallback((name, msg) => {
    setErrors((prev) => ({ ...prev, [name]: msg }))
  }, [])

  // The submit node registers its action so native form submission
  // (Enter in a text input) fires the same path as a button click.
  const registerSubmitAction = useCallback((action) => {
    submitActionRef.current = action
  }, [])

  const applyResponse = useCallback(
    (resp) => {
      const applies = Array.isArray(resp?.apply) ? resp.apply : []
      let formTouched = false
      for (const a of applies) {
        if (!a || typeof a !== 'object') continue
        if (a.target === 'form') {
          if (a.formId && a.formId !== formId) continue
          setStatus(a.status === 'error' ? 'error' : 'success')
          setMessage(typeof a.message === 'string' ? a.message : '')
          formTouched = true
        } else if (a.target === 'block' && a.document) {
          if (typeof renderCtx?.onReplaceBlock === 'function') {
            renderCtx.onReplaceBlock(a.blockId, a.document)
          } else if (typeof renderCtx?.onUpdateDocument === 'function') {
            renderCtx.onUpdateDocument(a.document)
          }
          formTouched = true
        } else {
          // eslint-disable-next-line no-console
          console.warn('[v5-form] unknown apply target', a.target)
        }
      }
      if (!formTouched) {
        if (resp?.status === 'error') {
          setStatus('error')
          setMessage(typeof resp.message === 'string' && resp.message !== '' ? resp.message : 'Something went wrong. Please try again.')
        } else {
          setStatus('success')
          setMessage(typeof resp?.message === 'string' ? resp.message : '')
        }
      }
    },
    [formId, renderCtx],
  )

  const submit = useCallback(
    async (actionArg) => {
      const action = actionArg || submitActionRef.current
      if (!action || typeof action !== 'object' || !action.kind) {
        // eslint-disable-next-line no-console
        console.warn('[v5-form] submit without action', formId)
        return null
      }
      if (inFlightRef.current) return null

      // Validate every registered field against current values.
      const errs = {}
      const hiddenErrs = []
      for (const [name, field] of fieldsRef.current) {
        const msg = typeof field.validate === 'function' ? field.validate(values[name] ?? '') : ''
        if (msg) {
          errs[name] = msg
          if (field.hidden) hiddenErrs.push(msg)
        }
      }
      if (Object.keys(errs).length > 0) {
        setErrors(errs)
        setStatus('error')
        // Hidden fields (bound ids) render no inline error — surface
        // theirs in the form-level line so the failure isn't invisible.
        setMessage(hiddenErrs.length > 0 ? hiddenErrs.join(' ') : 'Please fix the highlighted fields.')
        return null
      }

      // Coerce per-field (e.g. number inputs → numbers) into the payload;
      // state keeps the raw strings the controls are bound to.
      const payload = {}
      for (const [name, value] of Object.entries(values)) {
        const field = fieldsRef.current.get(name)
        payload[name] = field && typeof field.coerce === 'function' ? field.coerce(value) : value
      }

      inFlightRef.current = true
      setStatus('submitting')
      setMessage('')
      setErrors({})
      try {
        let resp
        if (action.kind === 'form_submit') {
          resp = await submitFormEndpoint({
            baseUrl: renderCtx?.apiBaseUrl,
            endpoint: action.endpoint,
            values: payload,
            formId,
          })
        } else if (action.kind === 'operation_invoke') {
          resp = await invokeOperation({
            baseUrl: renderCtx?.apiBaseUrl,
            tenantSlug: renderCtx?.tenantSlug,
            sessionId: renderCtx?.sessionId,
            operation: action.operation,
            params: { ...(action.params || {}), ...payload },
            entity: action.entity,
            formId,
            blockId: renderCtx?.blockId || undefined,
          })
        } else {
          // eslint-disable-next-line no-console
          console.warn('[v5-form] unknown submit action kind', action.kind)
          setStatus('idle')
          return null
        }
        applyResponse(resp)
        return resp
      } catch (err) {
        // eslint-disable-next-line no-console
        console.error('[v5-form] submit failed', formId, err.message)
        setStatus('error')
        setMessage('Something went wrong. Please try again.')
        return null
      } finally {
        inFlightRef.current = false
      }
    },
    [values, formId, renderCtx, applyResponse],
  )

  const ctxValue = useMemo(
    () => ({
      formId,
      values,
      errors,
      status,
      message,
      registerField,
      unregisterField,
      setValue,
      setFieldError,
      registerSubmitAction,
      submit,
    }),
    [formId, values, errors, status, message, registerField, unregisterField, setValue, setFieldError, registerSubmitAction, submit],
  )

  return (
    <FormContext.Provider value={ctxValue}>
      <form
        className="kw-form"
        data-form-id={formId}
        noValidate
        onSubmit={(e) => {
          e.preventDefault()
          submit(null)
        }}
      >
        {children}
      </form>
    </FormContext.Provider>
  )
}

export default FormContext
