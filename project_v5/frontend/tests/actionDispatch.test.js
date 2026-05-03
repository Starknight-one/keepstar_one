import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { dispatchAction } from '../src/renderer/actionDispatch'

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
