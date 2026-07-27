// dispatchAction — single entry point for every click on a button /
// frame carrying a domain.UserAction. Routes through the HANDLERS
// table (RUNTIME_SPEC §4.8): the legacy like/cart kinds keep the
// /actions path unchanged (back-compat), client built-ins run locally,
// and everything else reaches the operation registry through
// POST /operations/invoke — the operation NAME + params live in the
// node's action props, so adding an operation is library content +
// a server-declared apply[], zero client code.
//
// ctx must be the value of RenderContext.Provider — see
// src/renderer/RenderContext.js for the shape.
//
// Returns a Promise that resolves once the action is applied (or
// rejects on backend failure). Caller usually doesn't await — fire and
// forget is fine for clicks.

import { postAction, expandView, goBack } from '../api/actions'
import { invokeOperation, submitFormEndpoint } from '../api/operations'
import { fillTemplate } from './fillTemplate'

const KIND_LIKE = 'like'
const KIND_UNLIKE = 'unlike'
const KIND_CART_ADD = 'cart_add'
const KIND_CART_REMOVE = 'cart_remove'

// The dispatch table (RUNTIME_SPEC §4.8). Every handler takes
// (action, ctx). Kinds outside the table are unknown — warn + null,
// stale documents degrade visibly, never into a network call.
const HANDLERS = {
  // Legacy backend kinds — /actions path, unchanged.
  [KIND_LIKE]: dispatchBackendAction,
  [KIND_UNLIKE]: dispatchBackendAction,
  [KIND_CART_ADD]: dispatchBackendAction,
  [KIND_CART_REMOVE]: dispatchBackendAction,
  // Client built-ins.
  drill_detail: dispatchDrill,
  back: dispatchBack,
  open_category: dispatchOpenCategory,
  external_link: dispatchExternalLink,
  search: dispatchSearch,
  // Registry-driven operations (R1) + document-specified form posts (R6).
  operation_invoke: dispatchOperation,
  form_submit: dispatchFormSubmit,
}

export async function dispatchAction(action, ctx) {
  if (!action || typeof action !== 'object') return null
  const handler = HANDLERS[action.kind]
  if (!handler) {
    // eslint-disable-next-line no-console
    console.warn('[v5-action] unknown kind', action.kind, action)
    return null
  }
  return handler(action, ctx)
}

// applyServerResult — ONE applier for every server-declared result
// shape, mapped onto ctx handlers. Generalizes the original like/cart
// state path (resp.actions → onActionState) to the R1 apply[] contract:
//   resp.actions                          → ctx.onActionState (legacy /actions echo)
//   {target:"block", blockId, document}   → ctx.onReplaceBlock (block-hosting
//       shells), else ctx.onUpdateDocument — the storefront whole-view
//       swap (e.g. success_plaque).
//   {target:"form", ...}                  → owned by FormContext
//       (FormProvider.submit applies it); reaching a formless dispatch
//       means the server answered a standalone button with a form
//       target — surface the message in the console, nothing to bind.
export function applyServerResult(resp, ctx) {
  if (!resp || typeof resp !== 'object') return resp
  if (resp.actions && typeof ctx?.onActionState === 'function') {
    ctx.onActionState(resp.actions)
  }
  const applies = Array.isArray(resp.apply) ? resp.apply : []
  for (const a of applies) {
    if (!a || typeof a !== 'object') continue
    if (a.target === 'block' && a.document) {
      if (typeof ctx?.onReplaceBlock === 'function') {
        ctx.onReplaceBlock(a.blockId, a.document)
      } else if (typeof ctx?.onUpdateDocument === 'function') {
        ctx.onUpdateDocument(a.document)
      }
    } else if (a.target === 'form') {
      // eslint-disable-next-line no-console
      console.warn('[v5-action] form apply target outside a form', a.status, a.message)
    } else {
      // eslint-disable-next-line no-console
      console.warn('[v5-action] unknown apply target', a.target)
    }
  }
  return resp
}

// dispatchOperation — standalone operation_invoke button (e.g. the
// advance_lead atoms on lead_detail). Runs the registry operation named
// in the node props via POST /operations/invoke (R1) and maps the
// response's apply[] onto ctx. Form-hosted submissions do NOT come
// through here — FormProvider.submit collects field values and calls
// the same API client with a formId.
async function dispatchOperation(action, ctx) {
  const { apiBaseUrl, tenantSlug, sessionId, blockId } = ctx || {}
  if (typeof action.operation !== 'string' || action.operation === '') {
    // eslint-disable-next-line no-console
    console.warn('[v5-action] operation_invoke missing operation', action)
    return null
  }
  if (!sessionId) {
    // eslint-disable-next-line no-console
    console.warn('[v5-action] no sessionId; skipping', action)
    return null
  }
  try {
    const resp = await invokeOperation({
      baseUrl: apiBaseUrl,
      tenantSlug,
      sessionId,
      operation: action.operation,
      params: action.params,
      entity: action.entity,
      blockId: blockId || undefined,
    })
    return applyServerResult(resp, ctx)
  } catch (err) {
    // eslint-disable-next-line no-console
    console.error('[v5-action] operation failed', { operation: action.operation, err: err.message })
    throw err
  }
}

// dispatchFormSubmit — standalone form_submit button (no formId frame
// above it). POSTs action.params to the document-specified same-origin
// endpoint (R6 — the endpoint validation lives in the API client).
// Inside a form, Submit.jsx routes through FormProvider.submit instead,
// carrying the collected field values.
async function dispatchFormSubmit(action, ctx) {
  if (typeof action.endpoint !== 'string' || action.endpoint === '') {
    // eslint-disable-next-line no-console
    console.warn('[v5-action] form_submit missing endpoint', action)
    return null
  }
  try {
    const resp = await submitFormEndpoint({
      baseUrl: ctx?.apiBaseUrl,
      endpoint: action.endpoint,
      values: action.params || {},
      formId: action.formId,
    })
    return applyServerResult(resp, ctx)
  } catch (err) {
    // eslint-disable-next-line no-console
    console.error('[v5-action] form submit failed', { endpoint: action.endpoint, err: err.message })
    throw err
  }
}

async function dispatchBackendAction(action, ctx) {
  const { apiBaseUrl, tenantSlug, sessionId, actionState, onActionState } = ctx || {}
  if (!sessionId) {
    // eslint-disable-next-line no-console
    console.warn('[v5-action] no sessionId; skipping', action)
    return null
  }
  // The engine injects fixed-kind buttons (always like / cart_add), so
  // the toggle lives here: a like on an already-liked entity becomes
  // unlike, an add on an in-cart entity becomes remove.
  let kind = action.kind
  const entityId = action.entity?.id
  if (entityId && actionState) {
    if (kind === KIND_LIKE && isLiked(actionState, entityId)) kind = KIND_UNLIKE
    else if (kind === KIND_CART_ADD && isInCart(actionState, entityId)) kind = KIND_CART_REMOVE
  }
  // Optimistic flip so the control answers the click instantly; the
  // canonical state from the response (or a rollback on error) follows.
  const prevState = actionState
  if (typeof onActionState === 'function' && entityId) {
    onActionState(optimisticActionState(actionState, kind, action.entity))
  }
  try {
    const resp = await postAction({
      baseUrl: apiBaseUrl,
      tenantSlug,
      sessionId,
      kind,
      entity: action.entity,
      params: action.params,
    })
    return applyServerResult(resp, ctx)
  } catch (err) {
    if (typeof onActionState === 'function' && prevState) {
      onActionState(prevState)
    }
    // eslint-disable-next-line no-console
    console.error('[v5-action] backend failed', { kind, err: err.message })
    throw err
  }
}

export function isLiked(actionState, entityId) {
  return Boolean(actionState?.likedIds?.includes(entityId))
}

export function isInCart(actionState, entityId) {
  return Boolean(actionState?.cartItems?.some((it) => it?.entityId === entityId))
}

// optimisticActionState projects the expected post-action state locally.
function optimisticActionState(state, kind, entity) {
  const likedIds = [...(state?.likedIds || [])]
  const cartItems = [...(state?.cartItems || [])]
  const id = entity?.id
  switch (kind) {
    case KIND_LIKE:
      if (!likedIds.includes(id)) likedIds.push(id)
      break
    case KIND_UNLIKE:
      return { likedIds: likedIds.filter((e) => e !== id), cartItems }
    case KIND_CART_ADD:
      if (!cartItems.some((it) => it?.entityId === id)) {
        cartItems.push({ entityType: entity?.type || 'product', entityId: id, quantity: 1 })
      }
      break
    case KIND_CART_REMOVE:
      return { likedIds, cartItems: cartItems.filter((it) => it?.entityId !== id) }
    default:
      break
  }
  return { likedIds, cartItems }
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

async function dispatchBack(action, ctx) {
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
