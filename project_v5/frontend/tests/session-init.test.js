import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { initSession } from '../src/api/client'

// R17 session init: no mode → the legacy BODYLESS request (deployed
// widgets keep working byte-identical); mode=storefront sends {mode};
// mode=crm sends {mode, k} (R13 surface token) and a 403 surfaces with
// .status so the CRM shell can render the denied page.

beforeEach(() => {
  globalThis.fetch = vi.fn()
})

afterEach(() => {
  vi.restoreAllMocks()
})

const OK = {
  ok: true,
  status: 200,
  json: async () => ({ sessionId: 's-1', mode: 'storefront', tenant: { slug: 'demo', name: 'Demo' } }),
}

describe('initSession', () => {
  it('legacy call sends no body and no content-type', async () => {
    fetch.mockResolvedValue(OK)
    await initSession({ baseUrl: 'http://api/v1', tenantSlug: 'demo' })
    const [url, init] = fetch.mock.calls[0]
    expect(url).toBe('http://api/v1/session/init')
    expect(init.body).toBeUndefined()
    expect(init.headers['Content-Type']).toBeUndefined()
    expect(init.headers['X-Tenant-Slug']).toBe('demo')
  })

  it('mode=storefront sends {mode} only', async () => {
    fetch.mockResolvedValue(OK)
    await initSession({ baseUrl: 'http://api/v1', tenantSlug: 'demo', mode: 'storefront' })
    const [, init] = fetch.mock.calls[0]
    expect(JSON.parse(init.body)).toEqual({ mode: 'storefront' })
  })

  it('mode=crm sends {mode, k}', async () => {
    fetch.mockResolvedValue(OK)
    await initSession({ baseUrl: 'http://api/v1', tenantSlug: 'demo', mode: 'crm', k: 'tok-1' })
    const [, init] = fetch.mock.calls[0]
    expect(JSON.parse(init.body)).toEqual({ mode: 'crm', k: 'tok-1' })
  })

  it('a 403 throws with .status so shells can route to the denied page', async () => {
    fetch.mockResolvedValue({ ok: false, status: 403, text: async () => 'invalid surface token' })
    await expect(
      initSession({ baseUrl: 'http://api/v1', tenantSlug: 'demo', mode: 'crm', k: 'bad' }),
    ).rejects.toMatchObject({ status: 403 })
  })
})
