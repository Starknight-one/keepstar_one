// Submit — form-primitive leaf (§5.2):
//   {type:"submit", label, loadingLabel?,
//    action:{kind:"operation_invoke", operation, params?, entity?}
//          |{kind:"form_submit", endpoint}}                      // R6
//
// Clicking (or Enter in any field — the action is registered with the
// provider for native form submission) runs the FormProvider submit
// flow: validate → POST → apply[]. The form-level status line renders
// here, next to the button, so success/error lands inside the form
// block. NOT routed through actionDispatch — form submission is form
// state, not a document action. Standalone (formless) buttons carrying
// operation_invoke/form_submit kinds dispatch via actionDispatch instead.

import { useEffect } from 'react'
import { useFormContext } from '../form/FormContext'

export default function Submit({ node }) {
  const form = useFormContext()
  const registerSubmitAction = form?.registerSubmitAction

  useEffect(() => {
    if (registerSubmitAction && node?.action) registerSubmitAction(node.action)
  }, [registerSubmitAction, node])

  const submitting = form?.status === 'submitting'
  const label = node.label || 'Submit'
  const showMessage = form && form.message && (form.status === 'error' || form.status === 'success')

  return (
    <div className="kw-submit-row" data-id={node.id || ''}>
      <button
        type="button"
        className="kw-submit"
        disabled={submitting || undefined}
        aria-busy={submitting || undefined}
        onClick={(e) => {
          e.stopPropagation()
          e.preventDefault()
          if (!form) {
            // eslint-disable-next-line no-console
            console.warn('[v5-form] submit node outside a formId frame', node?.id)
            return
          }
          form.submit(node.action)
        }}
      >
        {submitting ? node.loadingLabel || 'Submitting…' : label}
      </button>
      {showMessage ? (
        <p className="kw-form-message" data-status={form.status} role={form.status === 'error' ? 'alert' : 'status'}>
          {form.message}
        </p>
      ) : null}
    </div>
  )
}
