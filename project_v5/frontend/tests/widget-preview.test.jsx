import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act } from '@testing-library/react'
import { renderDocument } from '../src/widget-preview'

import productGrid from './fixtures/product-card-grid.json'

// Wave C contract: window.KeepstarV5Widget.renderDocument renders an
// engine document into a host element with ZERO network calls — the
// admin canvas preview depends on this exact behavior. These tests
// must fail if renderDocument starts fetching, stops reusing the
// shadow root, or the auto-mount guard regresses merchant pages.

function makeHost() {
  const host = document.createElement('div')
  document.body.appendChild(host)
  return host
}

describe('renderDocument — standalone preview render', () => {
  let fetchMock

  beforeEach(() => {
    fetchMock = vi.fn(() => Promise.reject(new Error('network forbidden in preview')))
    vi.stubGlobal('fetch', fetchMock)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    document.body.innerHTML = ''
  })

  it('mounts a fixture document into a host: shadow root, styles, content — no fetch', async () => {
    const host = makeHost()
    let handle
    await act(async () => {
      handle = renderDocument(host, productGrid)
    })

    const shadow = host.shadowRoot
    expect(shadow).not.toBeNull()

    // Same chrome as the main widget: font link + inlined stylesheet.
    const fontLink = shadow.querySelector('link[rel="stylesheet"]')
    expect(fontLink?.href).toContain('fonts.googleapis.com')
    const style = shadow.querySelector('style')
    expect(style).not.toBeNull()
    expect(style.textContent.length).toBeGreaterThan(0)

    // Fixture content actually rendered (bound product names).
    expect(shadow.textContent).toContain('Glow Serum 30ml')
    expect(shadow.textContent).toContain('Hydration Mist')
    expect(shadow.textContent).toContain('Lavender Hand Cream')

    // ZERO network calls — no session init, no actions.
    expect(fetchMock).not.toHaveBeenCalled()
    expect(typeof handle.unmount).toBe('function')
  })

  it('second call on the same host does not throw and replaces content', async () => {
    const host = makeHost()
    await act(async () => {
      renderDocument(host, productGrid)
    })
    expect(host.shadowRoot.textContent).toContain('Glow Serum 30ml')

    const replacement = {
      version: '2.10',
      children: [{ type: 'text', id: 't1', content: 'Replaced content' }],
    }
    await act(async () => {
      // attachShadow twice throws — renderDocument must reuse instead.
      renderDocument(host, replacement)
    })

    const shadow = host.shadowRoot
    expect(shadow.textContent).toContain('Replaced content')
    expect(shadow.textContent).not.toContain('Glow Serum 30ml')
    // Chrome is replaced, not stacked.
    expect(shadow.querySelectorAll('style').length).toBe(1)
    expect(shadow.querySelectorAll('link[rel="stylesheet"]').length).toBe(1)
  })

  it('unmount() empties the mount', async () => {
    const host = makeHost()
    let handle
    await act(async () => {
      handle = renderDocument(host, productGrid)
    })
    expect(host.shadowRoot.textContent).toContain('Glow Serum 30ml')

    await act(async () => {
      handle.unmount()
    })
    expect(host.shadowRoot.childNodes.length).toBe(0)
  })
})

describe('widget.jsx — auto-mount guard + global', () => {
  beforeEach(() => {
    vi.resetModules()
    document.getElementById('keepstar-v5-widget')?.remove()
    delete window.__KEEPSTAR_V5_WIDGET__
    delete window.KeepstarV5Widget
    // Never-resolving fetch: keeps WidgetApp's initSession pending so the
    // auto-mount path produces no stray state updates during the test.
    vi.stubGlobal('fetch', vi.fn(() => new Promise(() => {})))
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    document.getElementById('keepstar-v5-widget')?.remove()
    delete window.__KEEPSTAR_V5_WIDGET__
    delete window.KeepstarV5Widget
  })

  it('noAutoMount=true skips the chat mount but still assigns the global', async () => {
    window.__KEEPSTAR_V5_WIDGET__ = { noAutoMount: true }
    await import('../src/widget.jsx')
    expect(document.getElementById('keepstar-v5-widget')).toBeNull()
    expect(typeof window.KeepstarV5Widget?.renderDocument).toBe('function')
  })

  it('without the flag the chat widget auto-mounts exactly as today', async () => {
    // act-wrap: the IIFE renders WidgetApp on import.
    await act(async () => {
      await import('../src/widget.jsx')
    })
    const chatHost = document.getElementById('keepstar-v5-widget')
    expect(chatHost).not.toBeNull()
    expect(chatHost.parentElement).toBe(document.body)
    expect(chatHost.shadowRoot).not.toBeNull()
    expect(typeof window.KeepstarV5Widget?.renderDocument).toBe('function')
  })
})
