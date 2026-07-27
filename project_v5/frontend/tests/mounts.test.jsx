import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act } from '@testing-library/react'
import {
  mountCRM,
  mountStorefront,
  parseCRMSlug,
  parseStorefrontSlug,
  parseSurfaceToken,
} from '../src/mounts'

// Page mounts (§5.1): /s/{slug} and /crm/{slug}?k= derive their config
// from the page URL; each mount attaches the same shadow-root chrome as
// the embedded widget (font + stylesheet) and renders its shell.

describe('URL parsing', () => {
  it('parses the storefront slug from /s/{slug}', () => {
    expect(parseStorefrontSlug('/s/realtor-demo')).toBe('realtor-demo')
    expect(parseStorefrontSlug('/s/realtor-demo/')).toBe('realtor-demo')
    expect(parseStorefrontSlug('/s/')).toBeNull()
    expect(parseStorefrontSlug('/crm/x')).toBeNull()
    expect(parseStorefrontSlug('/s/a%20b')).toBe('a b')
  })

  it('parses the CRM slug from /crm/{slug}', () => {
    expect(parseCRMSlug('/crm/realtor-demo')).toBe('realtor-demo')
    expect(parseCRMSlug('/s/realtor-demo')).toBeNull()
  })

  it('parses the surface token from ?k=', () => {
    expect(parseSurfaceToken('?k=tok-123')).toBe('tok-123')
    expect(parseSurfaceToken('?x=1')).toBeNull()
    expect(parseSurfaceToken('')).toBeNull()
  })
})

describe('page mounts', () => {
  beforeEach(() => {
    globalThis.fetch = vi.fn()
  })

  afterEach(() => {
    vi.restoreAllMocks()
    document.body.innerHTML = ''
  })

  it('mountCRM without a surface token renders the access-denied page — zero network', async () => {
    const host = document.createElement('div')
    document.body.appendChild(host)
    let handle
    await act(async () => {
      handle = mountCRM(host, { tenant: 'realtor-demo', api: 'http://api/v1' })
    })
    const shadow = host.shadowRoot
    expect(shadow).not.toBeNull()
    expect(shadow.textContent).toContain('This CRM link is invalid or has been revoked')
    expect(fetch).not.toHaveBeenCalled()
    await act(async () => {
      handle.unmount()
    })
  })

  it('mountStorefront inits a storefront-mode session and renders the page shell', async () => {
    fetch.mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({ sessionId: 's-1', mode: 'storefront', tenant: { slug: 'realtor-demo', name: 'Realtor' } }),
    })
    const host = document.createElement('div')
    document.body.appendChild(host)
    await act(async () => {
      mountStorefront(host, { tenant: 'realtor-demo', api: 'http://api/v1' })
    })
    // session/init carried {mode:"storefront"} (§5.1)
    const [url, init] = fetch.mock.calls[0]
    expect(url).toBe('http://api/v1/session/init')
    expect(JSON.parse(init.body)).toEqual({ mode: 'storefront' })
    // kw-page variant of the overlay + the same shadow chrome
    const shadow = host.shadowRoot
    expect(shadow.querySelector('.kw-overlay.kw-overlay--page')).not.toBeNull()
    expect(shadow.querySelector('style')).not.toBeNull()
  })
})
