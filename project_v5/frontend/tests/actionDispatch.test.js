import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { applyServerResult, dispatchAction } from '../src/renderer/actionDispatch'

beforeEach(() => {
  globalThis.fetch = vi.fn()
})

afterEach(() => {
  vi.restoreAllMocks()
})

function mkCtx(overrides = {}) {
  return {
    apiBaseUrl: 'http://api/v1',
    tenantSlug: 'demo',
    sessionId: 's-1',
    prefetch: { adjacentTemplate: {}, entities: {} },
    onUpdateDocument: vi.fn(),
    onSearch: vi.fn(),
    ...overrides,
  }
}

describe('dispatchAction — backend-handled kinds', () => {
  it('like calls POST /actions with the right payload', async () => {
    fetch.mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({ success: true, actions: { likedIds: ['p-1'] } }),
    })
    const ctx = mkCtx()
    await dispatchAction(
      { kind: 'like', entity: { type: 'product', id: 'p-1' } },
      ctx,
    )
    expect(fetch).toHaveBeenCalledTimes(1)
    const [url, init] = fetch.mock.calls[0]
    expect(url).toBe('http://api/v1/actions')
    const body = JSON.parse(init.body)
    expect(body.kind).toBe('like')
    expect(body.entity).toEqual({ type: 'product', id: 'p-1' })
  })

  it('cart_add includes params', async () => {
    fetch.mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({ success: true, actions: { cartItems: [] } }),
    })
    const ctx = mkCtx()
    await dispatchAction(
      { kind: 'cart_add', entity: { type: 'product', id: 'p-1' }, params: { quantity: 2 } },
      ctx,
    )
    const body = JSON.parse(fetch.mock.calls[0][1].body)
    expect(body.params).toEqual({ quantity: 2 })
  })
})

describe('dispatchAction — drill_detail', () => {
  it('uses prefetch template for instant drill, then fires background expand', async () => {
    fetch.mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({ success: true, document: { version: '2.10', children: [] } }),
    })
    const tpl = {
      version: '2.10',
      children: [{ type: 'text', id: 't', fieldBinding: 'name' }],
    }
    const ctx = mkCtx({
      prefetch: {
        adjacentTemplate: { product: tpl },
        entities: { product: [{ id: 'p-1', name: 'Glow Serum' }] },
      },
    })
    await dispatchAction(
      { kind: 'drill_detail', entity: { type: 'product', id: 'p-1' } },
      ctx,
    )
    // Optimistic update fires immediately.
    expect(ctx.onUpdateDocument).toHaveBeenCalledTimes(1)
    const filled = ctx.onUpdateDocument.mock.calls[0][0]
    expect(filled.children[0].content).toBe('Glow Serum')
    // Background sync went out.
    expect(fetch).toHaveBeenCalledWith(
      'http://api/v1/navigation/expand',
      expect.objectContaining({ method: 'POST' }),
    )
  })

  it('cold path (no prefetch) waits for backend doc', async () => {
    const remoteDoc = { version: '2.10', children: [{ type: 'text', content: 'remote' }] }
    fetch.mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({ success: true, document: remoteDoc }),
    })
    const ctx = mkCtx() // no prefetch
    await dispatchAction(
      { kind: 'drill_detail', entity: { type: 'product', id: 'p-1' } },
      ctx,
    )
    expect(ctx.onUpdateDocument).toHaveBeenCalledWith(remoteDoc)
  })
})

describe('dispatchAction — back', () => {
  it('POSTs to /navigation/back and updates document', async () => {
    const remoteDoc = { version: '2.10', children: [{ type: 'text', content: 'restored' }] }
    fetch.mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({ success: true, document: remoteDoc, stackSize: 0, canGoBack: false }),
    })
    const ctx = mkCtx()
    await dispatchAction({ kind: 'back' }, ctx)
    expect(fetch).toHaveBeenCalledWith(
      'http://api/v1/navigation/back',
      expect.objectContaining({ method: 'POST' }),
    )
    expect(ctx.onUpdateDocument).toHaveBeenCalledWith(remoteDoc)
  })
})

describe('dispatchAction — frontend-only kinds', () => {
  it('external_link opens window.open with noopener', () => {
    const open = vi.fn()
    const orig = window.open
    window.open = open
    dispatchAction(
      { kind: 'external_link', params: { url: 'https://x/y' } },
      mkCtx(),
    )
    expect(open).toHaveBeenCalledWith('https://x/y', '_blank', 'noopener,noreferrer')
    window.open = orig
  })

  it('search calls onSearch with the query', () => {
    const ctx = mkCtx()
    dispatchAction({ kind: 'search', params: { query: 'creams' } }, ctx)
    expect(ctx.onSearch).toHaveBeenCalledWith('creams')
  })

  it('open_category calls onSearch with formatted prompt', () => {
    const ctx = mkCtx()
    dispatchAction({ kind: 'open_category', params: { categoryId: 'face' } }, ctx)
    expect(ctx.onSearch).toHaveBeenCalledWith('show category face')
  })

  it('unknown kind logs a warning and returns null', async () => {
    const spy = vi.spyOn(console, 'warn').mockImplementation(() => {})
    const result = await dispatchAction({ kind: 'wat' }, mkCtx())
    expect(result).toBeNull()
    expect(spy).toHaveBeenCalled()
    spy.mockRestore()
  })
})

// The registry path (RUNTIME_SPEC §4.8, R1): the operation NAME lives in
// the node's action props — the dispatch table has no per-operation
// entries, so a new operation is server/library content only.
describe('dispatchAction — operation_invoke (registry path)', () => {
  function okResponse(payload) {
    return { ok: true, status: 200, text: async () => JSON.stringify(payload) }
  }

  it('POSTs the node-prop operation name + params to /operations/invoke', async () => {
    fetch.mockResolvedValue(okResponse({ status: 'ok', apply: [] }))
    const ctx = mkCtx({ blockId: 'b-3' })
    await dispatchAction(
      {
        kind: 'operation_invoke',
        operation: 'advance_lead',
        params: { toStatus: 'contacted' },
        entity: { type: 'lead', id: 'l-1' },
      },
      ctx,
    )
    expect(fetch).toHaveBeenCalledTimes(1)
    const [url, init] = fetch.mock.calls[0]
    expect(url).toBe('http://api/v1/operations/invoke')
    expect(init.headers['X-Tenant-Slug']).toBe('demo')
    const body = JSON.parse(init.body)
    expect(body.sessionId).toBe('s-1')
    expect(body.operation).toBe('advance_lead')
    expect(body.params).toEqual({ toStatus: 'contacted' })
    expect(body.entity).toEqual({ type: 'lead', id: 'l-1' })
    expect(body.blockId).toBe('b-3')
  })

  it('applies {target:"block"} through onReplaceBlock when the shell provides it', async () => {
    const doc = { version: '2.10', children: [{ type: 'text', content: 'Booked!' }] }
    fetch.mockResolvedValue(
      okResponse({ status: 'ok', apply: [{ target: 'block', blockId: 'b-1', document: doc }] }),
    )
    const ctx = mkCtx({ onReplaceBlock: vi.fn() })
    await dispatchAction({ kind: 'operation_invoke', operation: 'book_showing' }, ctx)
    expect(ctx.onReplaceBlock).toHaveBeenCalledWith('b-1', doc)
    expect(ctx.onUpdateDocument).not.toHaveBeenCalled()
  })

  it('falls back to onUpdateDocument (whole-view swap) without onReplaceBlock', async () => {
    const doc = { version: '2.10', children: [{ type: 'text', content: 'Booked!' }] }
    fetch.mockResolvedValue(
      okResponse({ status: 'ok', apply: [{ target: 'block', blockId: 'b-1', document: doc }] }),
    )
    const ctx = mkCtx() // storefront shell — no block plumbing
    await dispatchAction({ kind: 'operation_invoke', operation: 'book_showing' }, ctx)
    expect(ctx.onUpdateDocument).toHaveBeenCalledWith(doc)
  })

  it('missing operation name warns and never hits the network', async () => {
    const spy = vi.spyOn(console, 'warn').mockImplementation(() => {})
    const result = await dispatchAction({ kind: 'operation_invoke' }, mkCtx())
    expect(result).toBeNull()
    expect(fetch).not.toHaveBeenCalled()
    expect(spy).toHaveBeenCalled()
    spy.mockRestore()
  })

  it('no sessionId skips without a network call', async () => {
    const spy = vi.spyOn(console, 'warn').mockImplementation(() => {})
    const result = await dispatchAction(
      { kind: 'operation_invoke', operation: 'advance_lead' },
      mkCtx({ sessionId: null }),
    )
    expect(result).toBeNull()
    expect(fetch).not.toHaveBeenCalled()
    spy.mockRestore()
  })

  it('structured server error resolves with the payload — no throw, no document swap', async () => {
    fetch.mockResolvedValue({
      ok: false,
      status: 403,
      text: async () => JSON.stringify({ status: 'error', message: 'denied' }),
    })
    const ctx = mkCtx()
    const resp = await dispatchAction({ kind: 'operation_invoke', operation: 'advance_lead' }, ctx)
    expect(resp).toEqual({ status: 'error', message: 'denied' })
    expect(ctx.onUpdateDocument).not.toHaveBeenCalled()
  })
})

describe('dispatchAction — form_submit (standalone button, R6)', () => {
  it('POSTs params as values to the document-specified endpoint on the API origin', async () => {
    fetch.mockResolvedValue({ ok: true, status: 200, text: async () => JSON.stringify({ status: 'ok' }) })
    await dispatchAction(
      { kind: 'form_submit', endpoint: '/api/v1/onboard/step/s1/submit', params: { name: 'V' } },
      mkCtx(),
    )
    expect(fetch).toHaveBeenCalledTimes(1)
    const [url, init] = fetch.mock.calls[0]
    // Endpoint is a server-root path resolved against the API ORIGIN,
    // not appended to the /v1 base.
    expect(url).toBe('http://api/api/v1/onboard/step/s1/submit')
    expect(init.credentials).toBe('include')
    expect(JSON.parse(init.body)).toEqual({ values: { name: 'V' } })
  })

  it('refuses a non-same-origin endpoint before any network activity', async () => {
    const errSpy = vi.spyOn(console, 'error').mockImplementation(() => {})
    await expect(
      dispatchAction(
        { kind: 'form_submit', endpoint: '//evil.example/steal', params: {} },
        mkCtx(),
      ),
    ).rejects.toThrow(/same-origin/)
    expect(fetch).not.toHaveBeenCalled()
    errSpy.mockRestore()
  })

  it('missing endpoint warns and returns null', async () => {
    const spy = vi.spyOn(console, 'warn').mockImplementation(() => {})
    const result = await dispatchAction({ kind: 'form_submit' }, mkCtx())
    expect(result).toBeNull()
    expect(fetch).not.toHaveBeenCalled()
    spy.mockRestore()
  })
})

// applyServerResult is the generalization of the original like/cart
// state path: ONE applier maps every server-declared result shape onto
// ctx handlers.
describe('applyServerResult — generalized result application', () => {
  it('legacy /actions echo still routes resp.actions to onActionState', async () => {
    const actions = { likedIds: ['p-1'], cartItems: [] }
    fetch.mockResolvedValue({ ok: true, status: 200, json: async () => ({ success: true, actions }) })
    const ctx = mkCtx({ onActionState: vi.fn(), actionState: { likedIds: [], cartItems: [] } })
    await dispatchAction({ kind: 'like', entity: { type: 'product', id: 'p-1' } }, ctx)
    // First call = optimistic flip, last call = canonical server state.
    expect(ctx.onActionState).toHaveBeenLastCalledWith(actions)
  })

  it('form target outside a form warns instead of throwing', () => {
    const spy = vi.spyOn(console, 'warn').mockImplementation(() => {})
    const resp = { status: 'error', apply: [{ target: 'form', formId: 'f', status: 'error', message: 'invalid' }] }
    expect(applyServerResult(resp, mkCtx())).toBe(resp)
    expect(spy).toHaveBeenCalled()
    spy.mockRestore()
  })

  it('unknown apply target warns and skips', () => {
    const spy = vi.spyOn(console, 'warn').mockImplementation(() => {})
    const ctx = mkCtx()
    applyServerResult({ status: 'ok', apply: [{ target: 'toast', message: 'hi' }] }, ctx)
    expect(ctx.onUpdateDocument).not.toHaveBeenCalled()
    expect(spy).toHaveBeenCalled()
    spy.mockRestore()
  })
})
