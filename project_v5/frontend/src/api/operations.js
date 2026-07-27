// Operations API client (RUNTIME_SPEC R1 + R6). Talks to:
//   POST  {base}/operations/invoke   — closed-set operation execution
//   POST  <document-specified path>  — form_submit endpoints (R6: the
//                                      registration form posts to
//                                      /api/v1/onboard/step/{id}/submit,
//                                      NEVER through /operations/invoke)
//
// Both calls resolve to the R1 response contract:
//   {status:"ok"|"error", message?, result?,
//    apply:[ {target:"block", blockId, document}
//          | {target:"form",  formId, status, message} ]}
//
// Non-2xx responses that still carry a JSON body (validation errors,
// denied operations) are returned as that payload — the form renders
// the server's message instead of a generic network error. Only
// bodyless / non-JSON failures throw.

export async function invokeOperation({
  baseUrl,
  tenantSlug,
  sessionId,
  operation,
  params,
  entity,
  formId,
  blockId,
}) {
  const res = await fetch(`${baseUrl}/operations/invoke`, {
    method: 'POST',
    headers: {
      ...tenantHeaders(tenantSlug),
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({ sessionId, operation, params, entity, formId, blockId }),
  })
  return parseApplyResponse(res, 'operations/invoke')
}

// submitFormEndpoint — POST collected form values to a document-specified
// endpoint (submit node action {kind:"form_submit", endpoint}). The
// endpoint MUST be a same-origin relative path: form values can carry
// credentials (registration password, R6), so an absolute or
// protocol-relative endpoint smuggled into a document is refused before
// any network activity.
//
// Body shape: {formId, values:{<fieldName>: <value>}}. credentials are
// included so the ks_onboard cookie (R5) rides along in dev setups where
// the API origin differs from the page origin.
export async function submitFormEndpoint({ baseUrl, endpoint, values, formId }) {
  const url = resolveEndpoint(baseUrl, endpoint)
  const res = await fetch(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    credentials: 'include',
    body: JSON.stringify({ formId, values }),
  })
  return parseApplyResponse(res, 'form submit')
}

// resolveEndpoint validates the document-specified path and anchors it
// to the API origin. Rules:
//   - must be a non-empty string starting with a single "/" ("//host"
//     protocol-relative URLs are rejected);
//   - when baseUrl is absolute (cross-origin dev embed), the endpoint is
//     resolved against the baseUrl ORIGIN (endpoints are server-root
//     paths like /api/v1/onboard/step/x/submit, not baseUrl-relative);
//   - otherwise the path is used as-is (same-origin deploy, §5.1).
export function resolveEndpoint(baseUrl, endpoint) {
  if (typeof endpoint !== 'string' || !endpoint.startsWith('/') || endpoint.startsWith('//')) {
    throw new Error(`form_submit endpoint must be a same-origin path, got: ${String(endpoint)}`)
  }
  if (typeof baseUrl === 'string' && /^https?:\/\//i.test(baseUrl)) {
    try {
      return new URL(baseUrl).origin + endpoint
    } catch (_) {
      /* malformed baseUrl — fall through to the relative path */
    }
  }
  return endpoint
}

// parseApplyResponse normalizes every outcome onto the R1 payload shape.
async function parseApplyResponse(res, label) {
  const text = await res.text()
  let payload = null
  if (text) {
    try {
      payload = JSON.parse(text)
    } catch (_) {
      payload = null
    }
  }
  if (payload && typeof payload === 'object' && !Array.isArray(payload)) {
    // Server said something structured; trust it, but never let a non-2xx
    // response pass as implicit success.
    if (!res.ok && payload.status === undefined) payload.status = 'error'
    return payload
  }
  if (!res.ok) {
    throw new Error(`${label} ${res.status}: ${text}`)
  }
  return { status: 'ok' }
}

function tenantHeaders(tenantSlug) {
  return tenantSlug ? { 'X-Tenant-Slug': tenantSlug } : {}
}
