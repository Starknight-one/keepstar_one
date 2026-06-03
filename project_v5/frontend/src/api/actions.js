// Action + navigation API client. Talks to:
//   POST  {base}/actions                       — like / cart actions
//   POST  {base}/navigation/expand             — drill into detail preset
//   POST  {base}/navigation/back               — pop view stack
//
// Fire-and-forget pattern (sync=true) is supported on /actions so the
// frontend can update its local state without waiting for the response
// when no echo is needed.

export async function postAction({ baseUrl, tenantSlug, sessionId, kind, entity, params, sync = false }) {
  const url = `${baseUrl}/actions${sync ? '?sync=true' : ''}`
  const res = await fetch(url, {
    method: 'POST',
    headers: {
      ...tenantHeaders(tenantSlug),
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({ sessionId, kind, entity, params }),
  })
  if (!res.ok) {
    const body = await res.text()
    throw new Error(`actions ${res.status}: ${body}`)
  }
  // 204 (sync mode) → no body to parse.
  if (res.status === 204) return null
  return res.json()
}

export async function expandView({ baseUrl, tenantSlug, sessionId, entityType, entityId }) {
  const res = await fetch(`${baseUrl}/navigation/expand`, {
    method: 'POST',
    headers: {
      ...tenantHeaders(tenantSlug),
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({ sessionId, entityType, entityId }),
  })
  if (!res.ok) {
    const body = await res.text()
    throw new Error(`navigation/expand ${res.status}: ${body}`)
  }
  return res.json()
}

export async function goBack({ baseUrl, tenantSlug, sessionId }) {
  const res = await fetch(`${baseUrl}/navigation/back`, {
    method: 'POST',
    headers: {
      ...tenantHeaders(tenantSlug),
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({ sessionId }),
  })
  if (!res.ok) {
    const body = await res.text()
    throw new Error(`navigation/back ${res.status}: ${body}`)
  }
  return res.json()
}

// filterApply re-renders the current grid preset narrowed by the active
// filter set — deterministic, no LLM. Empty set resets to the full data.
// `filters` is the full active set: [{key, type:'enum'|'range', values?,
// min?, max?}]. Returns the nav response ({document, facets, canGoBack});
// caller swaps the document and updates the (guided) facets.
export async function filterApply({ baseUrl, tenantSlug, sessionId, filters, sort }) {
  const res = await fetch(`${baseUrl}/navigation/filter`, {
    method: 'POST',
    headers: {
      ...tenantHeaders(tenantSlug),
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({ sessionId, filters, sortField: sort?.field, sortOrder: sort?.order }),
  })
  if (!res.ok) {
    const body = await res.text()
    throw new Error(`navigation/filter ${res.status}: ${body}`)
  }
  return res.json()
}

function tenantHeaders(tenantSlug) {
  return tenantSlug ? { 'X-Tenant-Slug': tenantSlug } : {}
}
