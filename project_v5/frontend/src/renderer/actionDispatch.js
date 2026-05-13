// dispatchAction — single entry point for every click on a button /
// frame carrying a domain.UserAction. Switches on action.kind and
// routes to either the backend (/actions, /navigation/*) or a
// frontend-only handler (external_link, search).
//
// ctx must be the value of RenderContext.Provider — see
// src/renderer/RenderContext.js for the shape.
//
// Returns a Promise that resolves once the action is applied (or
// rejects on backend failure). Caller usually doesn't await — fire and
// forget is fine for clicks.

import { postAction, expandView, goBack } from '../api/actions'
import { fillTemplate } from './fillTemplate'

const KIND_LIKE = 'like'
const KIND_UNLIKE = 'unlike'
const KIND_CART_ADD = 'cart_add'
const KIND_CART_REMOVE = 'cart_remove'
const KIND_DRILL = 'drill_detail'
const KIND_BACK = 'back'
const KIND_OPEN_CATEGORY = 'open_category'
const KIND_EXTERNAL_LINK = 'external_link'
const KIND_SEARCH = 'search'

export async function dispatchAction(action, ctx) {
  if (!action || typeof action !== 'object') return
  const { kind } = action
  switch (kind) {
    case KIND_LIKE:
    case KIND_UNLIKE:
    case KIND_CART_ADD:
    case KIND_CART_REMOVE:
      return dispatchBackendAction(action, ctx)
    case KIND_DRILL:
      return dispatchDrill(action, ctx)
    case KIND_BACK:
      return dispatchBack(ctx)
    case KIND_EXTERNAL_LINK:
      return dispatchExternalLink(action)
    case KIND_SEARCH:
      return dispatchSearch(action, ctx)
    case KIND_OPEN_CATEGORY:
      return dispatchOpenCategory(action, ctx)
    default:
      // eslint-disable-next-line no-console
      console.warn('[v5-action] unknown kind', kind, action)
      return null
  }
}

async function dispatchBackendAction(action, ctx) {
  const { apiBaseUrl, tenantSlug, sessionId } = ctx || {}
  if (!sessionId) {
    // eslint-disable-next-line no-console
    console.warn('[v5-action] no sessionId; skipping', action)
    return null
  }
  try {
    const resp = await postAction({
      baseUrl: apiBaseUrl,
      tenantSlug,
      sessionId,
      kind: action.kind,
      entity: action.entity,
      params: action.params,
    })
    // eslint-disable-next-line no-console
    console.debug('[v5-action] backend ok', { kind: action.kind, entity: action.entity, actions: resp?.actions })
    return resp
  } catch (err) {
    // eslint-disable-next-line no-console
    console.error('[v5-action] backend failed', { kind: action.kind, err: err.message })
    throw err
  }
}

// dispatchDrill handles drill_detail. Optimistic path: if prefetch
// has both a template for entity.type AND the entity itself, fill the
// template locally and update the document for instant feedback. Fire
// /navigation/expand in the background to keep server state in sync.
//
// Cold path: when prefetch is empty (e.g., LLM rendered a freestyle
// view with no preset_in_use, so no adjacency), wait for the server
// to render the detail and use its document.
async function dispatchDrill(action, ctx) {
  const { apiBaseUrl, tenantSlug, sessionId, prefetch, onUpdateDocument } = ctx || {}
  if (!action.entity || !action.entity.id) {
    // eslint-disable-next-line no-console
    console.warn('[v5-action] drill_detail missing entity', action)
    return null
  }
  const entityType = action.entity.type
  const entityId = action.entity.id
  const template = prefetch?.adjacentTemplate?.[entityType]
  const entities = prefetch?.entities?.[entityType] || []
  const entity = entities.find((e) => e?.id === entityId)

  if (template && entity) {
    const filled = fillTemplate(template, entity)
    onUpdateDocument(filled)
    // Background sync — best effort; ignore failures (UI already
    // updated).
    expandView({ baseUrl: apiBaseUrl, tenantSlug, sessionId, entityType, entityId })
      .catch((err) => {
        // eslint-disable-next-line no-console
        console.warn('[v5-action] background expand failed', err.message)
      })
    return { document: filled }
  }

  // Cold path.
  try {
    const resp = await expandView({ baseUrl: apiBaseUrl, tenantSlug, sessionId, entityType, entityId })
    if (resp?.document) onUpdateDocument(resp.document)
    return resp
  } catch (err) {
    // eslint-disable-next-line no-console
    console.error('[v5-action] expand failed', err.message)
    throw err
  }
}

async function dispatchBack(ctx) {
  const { apiBaseUrl, tenantSlug, sessionId, onUpdateDocument } = ctx || {}
  try {
    const resp = await goBack({ baseUrl: apiBaseUrl, tenantSlug, sessionId })
    if (resp?.document) onUpdateDocument(resp.document)
    return resp
  } catch (err) {
    // eslint-disable-next-line no-console
    console.error('[v5-action] back failed', err.message)
    throw err
  }
}

function dispatchExternalLink(action) {
  const url = action.params?.url
  if (typeof url !== 'string' || url === '') {
    // eslint-disable-next-line no-console
    console.warn('[v5-action] external_link missing url', action)
    return null
  }
  // noopener+noreferrer to keep the embedding page safe from window.opener.
  if (typeof window !== 'undefined') {
    window.open(url, '_blank', 'noopener,noreferrer')
  }
  return null
}

function dispatchSearch(action, ctx) {
  const query = action.params?.query
  if (typeof query !== 'string' || query === '') {
    // eslint-disable-next-line no-console
    console.warn('[v5-action] search missing query', action)
    return null
  }
  if (typeof ctx?.onSearch === 'function') {
    ctx.onSearch(query)
  }
  return null
}

function dispatchOpenCategory(action, ctx) {
  // open_category is conceptually a new pipeline turn ("show category
  // X"). Delegate to onSearch so the chat shell decides whether to
  // pre-fill the input or fire a turn directly.
  const cid = action.params?.categoryId
  if (typeof ctx?.onSearch === 'function' && cid) {
    ctx.onSearch(`show category ${cid}`)
  }
  return null
}
