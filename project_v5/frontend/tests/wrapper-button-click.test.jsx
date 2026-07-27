import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { render, fireEvent, act, waitFor } from '@testing-library/react'
import SceneGraphRenderer from '../src/renderer/SceneGraphRenderer'
import RenderContext from '../src/renderer/RenderContext'

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

describe('wrapper button + action prop', () => {
  it('button with action.kind=like POSTs to /actions on click', async () => {
    fetch.mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({ success: true, actions: { likedIds: ['p-1'] } }),
    })
    const doc = {
      version: '2.10',
      children: [
        {
          type: 'text',
          id: 'btn',
          content: '♥',
          wrapper: 'button',
          action: { kind: 'like', entity: { type: 'product', id: 'p-1' } },
        },
      ],
    }
    const { container } = render(
      <RenderContext.Provider value={mkCtx()}>
        <SceneGraphRenderer document={doc} />
      </RenderContext.Provider>,
    )
    const btn = container.querySelector('button.kw-button')
    fireEvent.click(btn)
    // The dispatch is async; flush a microtask.
    await Promise.resolve()
    expect(fetch).toHaveBeenCalledWith(
      'http://api/v1/actions',
      expect.objectContaining({ method: 'POST' }),
    )
  })

  it('button click stops bubbling so a parent card click does not also fire', async () => {
    fetch.mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({ success: true, actions: { likedIds: ['p-1'] } }),
    })
    const ctx = mkCtx({
      prefetch: {
        adjacentTemplate: { product: { version: '2.10', children: [] } },
        entities: { product: [{ id: 'p-1', name: 'X' }] },
      },
    })
    const doc = {
      version: '2.10',
      children: [
        {
          type: 'frame',
          id: 'card',
          __templateOrigin: 'tpl-card',
          dataIndex: 0,
          children: [
            {
              type: 'text',
              id: 'btn',
              content: '♥',
              wrapper: 'button',
              action: { kind: 'like', entity: { type: 'product', id: 'p-1' } },
            },
          ],
        },
      ],
    }
    const { container } = render(
      <RenderContext.Provider value={ctx}>
        <SceneGraphRenderer document={doc} />
      </RenderContext.Provider>,
    )
    const btn = container.querySelector('button.kw-button')
    fireEvent.click(btn)
    await Promise.resolve()
    // Backend POST went out (button click).
    expect(fetch).toHaveBeenCalledWith(
      'http://api/v1/actions',
      expect.objectContaining({ method: 'POST' }),
    )
    // Card click did NOT update the document (drill suppressed by
    // stopPropagation on the inner button).
    expect(ctx.onUpdateDocument).not.toHaveBeenCalled()
  })
})

describe('wrapper button — operation_invoke (registry path, §4.8)', () => {
  function operationDoc() {
    return {
      version: '2.10',
      children: [
        {
          type: 'text',
          id: 'btn',
          content: 'Mark contacted',
          wrapper: 'button',
          action: {
            kind: 'operation_invoke',
            operation: 'advance_lead',
            params: { toStatus: 'contacted' },
            entity: { type: 'lead', id: 'l-1' },
          },
        },
      ],
    }
  }

  it('POSTs to /operations/invoke, disables while in flight, and a double click fires ONCE', async () => {
    let resolveFetch
    fetch.mockImplementation(() => new Promise((r) => { resolveFetch = r }))
    const ctx = mkCtx({ blockId: 'b-7', onReplaceBlock: vi.fn() })
    const { container } = render(
      <RenderContext.Provider value={ctx}>
        <SceneGraphRenderer document={operationDoc()} />
      </RenderContext.Provider>,
    )
    const btn = container.querySelector('button.kw-button')
    fireEvent.click(btn)
    // In flight: disabled + aria-busy, so the second click cannot
    // double-fire the server mutation.
    expect(btn.disabled).toBe(true)
    expect(btn.getAttribute('aria-busy')).toBe('true')
    fireEvent.click(btn)
    expect(fetch).toHaveBeenCalledTimes(1)
    const [url, init] = fetch.mock.calls[0]
    expect(url).toBe('http://api/v1/operations/invoke')
    const body = JSON.parse(init.body)
    expect(body.operation).toBe('advance_lead')
    expect(body.blockId).toBe('b-7')

    const doc = { version: '2.10', children: [{ type: 'text', content: 'Lead updated' }] }
    await act(async () => {
      resolveFetch({
        ok: true,
        status: 200,
        text: async () => JSON.stringify({ status: 'ok', apply: [{ target: 'block', blockId: 'b-7', document: doc }] }),
      })
    })
    await waitFor(() => expect(btn.disabled).toBe(false))
    // The server-declared apply landed through the block applier.
    expect(ctx.onReplaceBlock).toHaveBeenCalledWith('b-7', doc)
  })

  it('a failed dispatch re-enables the button and stays contained (no unhandled rejection)', async () => {
    const errSpy = vi.spyOn(console, 'error').mockImplementation(() => {})
    let rejectFetch
    fetch.mockImplementation(() => new Promise((_r, rej) => { rejectFetch = rej }))
    const { container } = render(
      <RenderContext.Provider value={mkCtx()}>
        <SceneGraphRenderer document={operationDoc()} />
      </RenderContext.Provider>,
    )
    const btn = container.querySelector('button.kw-button')
    fireEvent.click(btn)
    expect(btn.disabled).toBe(true)
    await act(async () => {
      rejectFetch(new Error('network down'))
    })
    await waitFor(() => expect(btn.disabled).toBe(false))
    errSpy.mockRestore()
  })
})

describe('Frame card click — drill_detail', () => {
  it('replicate clone with prefetch fires drill on body click', async () => {
    fetch.mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({ success: true }),
    })
    const ctx = mkCtx({
      prefetch: {
        adjacentTemplate: {
          product: {
            version: '2.10',
            children: [{ type: 'text', id: 't', fieldBinding: 'name' }],
          },
        },
        entities: { product: [{ id: 'p-1', name: 'Glow Serum' }] },
      },
    })
    const doc = {
      version: '2.10',
      children: [
        {
          type: 'frame',
          id: 'card-clone',
          __templateOrigin: 'tpl-card',
          dataIndex: 0,
          children: [{ type: 'text', id: 'title', content: 'Glow Serum' }],
        },
      ],
    }
    const { container } = render(
      <RenderContext.Provider value={ctx}>
        <SceneGraphRenderer document={doc} />
      </RenderContext.Provider>,
    )
    const card = container.querySelector('.kw-frame--clickable')
    expect(card).not.toBeNull()
    fireEvent.click(card)
    await Promise.resolve()
    // Optimistic drill — onUpdateDocument fired with the filled template.
    expect(ctx.onUpdateDocument).toHaveBeenCalledTimes(1)
    const filled = ctx.onUpdateDocument.mock.calls[0][0]
    expect(filled.children[0].content).toBe('Glow Serum')
  })

  it('frame without __templateOrigin does NOT get a click handler', () => {
    const ctx = mkCtx({
      prefetch: {
        adjacentTemplate: { product: { version: '2.10', children: [] } },
        entities: { product: [{ id: 'p-1' }] },
      },
    })
    const doc = {
      version: '2.10',
      children: [
        {
          type: 'frame',
          id: 'plain-frame',
          children: [{ type: 'text', content: 'Plain' }],
        },
      ],
    }
    const { container } = render(
      <RenderContext.Provider value={ctx}>
        <SceneGraphRenderer document={doc} />
      </RenderContext.Provider>,
    )
    expect(container.querySelector('.kw-frame--clickable')).toBeNull()
  })
})
