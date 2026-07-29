import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { render, fireEvent, screen } from '@testing-library/react'
import ChatShell from '../src/shells/ChatShell'
import {
  collectSurfaceLinks,
  mergeSurfaceLinks,
  surfaceKindOf,
  surfaceLinksFromBlocks,
  surfaceLinksFromManifest,
} from '../src/canvas/surfaceLinks'

// Canvas tabs (V2_SPEC §2 step 5). WHY this is picky: a tab is a promise
// that a live surface exists behind it. It may appear only when the
// runtime has actually ISSUED that URL — never from a stray bound field,
// never before `issue_surface_urls` applied. And what it shows must be
// the real page (L1), which is why the pane holds an iframe of the
// issued URL rather than a rendering of it.

const STOREFRONT_URL = 'https://v5.example/s/blue-harbor-realty'
const CRM_URL = 'https://v5.example/crm/blue-harbor-realty?k=tok-1'

// The surface_links preset as it arrives AFTER the engine expands
// `replicate` over the synthetic surfaceLink set and binds the fields
// (seed/surface_links.json + operations/meta_apply_manifest.go).
function surfaceLinksDoc(rows) {
  return {
    version: '2.10',
    children: [
      {
        type: 'frame',
        id: 'surface-links',
        children: [
          {
            type: 'frame',
            id: 'sl-head',
            children: [
              { type: 'text', id: 'sl-title', content: 'Your surfaces are live', format: {} },
            ],
          },
          ...rows.map(([label, url], i) => ({
            type: 'frame',
            id: `sl-row-${i}`,
            children: [
              {
                type: 'frame',
                id: `sl-row-main-${i}`,
                children: [
                  { type: 'text', id: `sl-label-${i}`, fieldBinding: 'label', content: label, format: {} },
                  { type: 'text', id: `sl-url-${i}`, fieldBinding: 'url', content: url, format: {} },
                ],
              },
            ],
          })),
        ],
      },
    ],
  }
}

const LINKS_BLOCK = {
  blockId: 'b-links',
  kind: 'document',
  display: 'inline',
  document: surfaceLinksDoc([
    ['Storefront', STOREFRONT_URL],
    ['CRM', CRM_URL],
  ]),
}

const appliedManifest = (result) => ({
  steps: [
    { id: '1', op: 'create_tenant', status: 'applied', result: { slug: 'blue-harbor-realty' } },
    { id: '2', op: 'issue_surface_urls', status: 'applied', result },
  ],
})

describe('surfaceKindOf', () => {
  it('recognises the URL shapes the runtime issues', () => {
    expect(surfaceKindOf(STOREFRONT_URL)).toBe('storefront')
    expect(surfaceKindOf(CRM_URL)).toBe('crm')
  })

  it('refuses anything that is not a surface address', () => {
    expect(surfaceKindOf('https://v5.example/onboard')).toBeNull()
    expect(surfaceKindOf('https://cdn.example/img/sofa.jpg')).toBeNull()
    expect(surfaceKindOf('/s/blue-harbor-realty')).toBeNull() // not absolute
    expect(surfaceKindOf('')).toBeNull()
    expect(surfaceKindOf(undefined)).toBeNull()
  })
})

describe('surfaceLinksFromManifest (resume payload)', () => {
  it('reads both URLs off the applied issue_surface_urls step', () => {
    const links = surfaceLinksFromManifest(
      appliedManifest({ storefrontUrl: STOREFRONT_URL, crmUrl: CRM_URL }),
    )
    expect(links).toEqual([
      { surface: 'storefront', url: STOREFRONT_URL, label: 'Storefront' },
      { surface: 'crm', url: CRM_URL, label: 'CRM' },
    ])
  })

  it('yields nothing before the step ran, and nothing from the summary shape', () => {
    expect(surfaceLinksFromManifest({ steps: [{ id: '1', op: 'issue_surface_urls', status: 'proposed' }] })).toEqual([])
    // The pipeline turn's manifest field is counts only — no URLs on it.
    expect(surfaceLinksFromManifest({ staged: 5, applied: 5, total: 5 })).toEqual([])
    expect(surfaceLinksFromManifest(null)).toEqual([])
  })
})

describe('surfaceLinksFromBlocks (live turn)', () => {
  it('reads the bound rows of a rendered surface_links block', () => {
    const links = surfaceLinksFromBlocks([{ role: 'bot', blocks: [LINKS_BLOCK] }])
    expect(links).toEqual([
      { surface: 'storefront', url: STOREFRONT_URL, label: 'Storefront' },
      { surface: 'crm', url: CRM_URL, label: 'CRM' },
    ])
  })

  it('ignores url-bound fields that do not address a surface', () => {
    const productCard = {
      blockId: 'b-card',
      kind: 'document',
      document: {
        version: '2.10',
        children: [
          { type: 'text', id: 'p1', fieldBinding: 'label', content: 'Sofa', format: {} },
          { type: 'text', id: 'p2', fieldBinding: 'url', content: 'https://shop.example/p/sofa', format: {} },
        ],
      },
    }
    expect(surfaceLinksFromBlocks([{ role: 'bot', blocks: [productCard] }])).toEqual([])
  })
})

describe('collectSurfaceLinks', () => {
  it('lets the manifest (DB truth) override a stale rendered block', () => {
    const reissued = 'https://v5.example/crm/blue-harbor-realty?k=tok-2'
    const links = collectSurfaceLinks({
      messages: [{ role: 'bot', blocks: [LINKS_BLOCK] }],
      manifest: appliedManifest({ crmUrl: reissued }),
    })
    expect(links.map((l) => l.url)).toEqual([STOREFRONT_URL, reissued])
  })
})

// An issued surface is a fact about the SESSION. Sources come and go —
// the block scrolls out of the projected transcript, the per-turn manifest
// field is counts-only — and a tab must survive all of that.
describe('mergeSurfaceLinks', () => {
  const known = [
    { surface: 'storefront', url: STOREFRONT_URL, label: 'Storefront' },
    { surface: 'crm', url: CRM_URL, label: 'CRM' },
  ]

  it('keeps known surfaces when the current render sees none', () => {
    expect(mergeSurfaceLinks(known, [])).toBe(known) // same identity: no churn
  })

  it('re-points a surface whose URL was re-issued, keeping the other', () => {
    const reissued = 'https://v5.example/crm/blue-harbor-realty?k=tok-2'
    const merged = mergeSurfaceLinks(known, [{ surface: 'crm', url: reissued, label: 'CRM' }])
    expect(merged.map((l) => l.url)).toEqual([STOREFRONT_URL, reissued])
  })

  it('adds a newly issued surface in the flow order', () => {
    const merged = mergeSurfaceLinks([known[1]], [known[0]])
    expect(merged.map((l) => l.surface)).toEqual(['storefront', 'crm'])
  })
})

// ── the shell wiring ──────────────────────────────────────────────────

function sseResponse(wire) {
  const encoder = new TextEncoder()
  let sent = false
  return {
    ok: true,
    status: 200,
    body: {
      getReader: () => ({
        read: async () => {
          if (sent) return { done: true, value: undefined }
          sent = true
          return { done: false, value: encoder.encode(wire) }
        },
        cancel: async () => {},
      }),
    },
  }
}

function turnWire(blocks, result) {
  return (
    blocks.map((b) => `event: block\ndata: ${JSON.stringify(b)}\n\n`).join('') +
    `event: result\ndata: ${JSON.stringify(result)}\n\n`
  )
}

beforeEach(() => {
  globalThis.fetch = vi.fn()
})

afterEach(() => {
  vi.restoreAllMocks()
})

function renderCanvasShell(extraProps = {}) {
  return render(
    <ChatShell
      apiBaseUrl="http://api/v1"
      tenantSlug="keepstar-onboarding"
      sessionId="s-1"
      variant="onboarding"
      layout="canvas"
      placeholder="Describe your business…"
      {...extraProps}
    />,
  )
}

describe('canvas tabs', () => {
  it('shows Builder alone until surface URLs exist', () => {
    renderCanvasShell({ initialMessages: [{ role: 'bot', text: 'Tell me about your business.' }] })
    expect(screen.getByRole('tab', { name: 'Builder' })).toBeTruthy()
    expect(screen.queryByRole('tab', { name: 'Storefront' })).toBeNull()
    expect(screen.queryByRole('tab', { name: 'CRM' })).toBeNull()
  })

  it('grows a Storefront and a CRM tab from the turn that issues the URLs', async () => {
    const blocks = [LINKS_BLOCK]
    fetch.mockResolvedValue(sseResponse(turnWire(blocks, { blocks, latencyMs: 5 })))

    renderCanvasShell()
    fireEvent.change(screen.getByPlaceholderText('Describe your business…'), {
      target: { value: 'give me the links' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Send' }))

    const storefrontTab = await screen.findByRole('tab', { name: 'Storefront' })
    expect(screen.getByRole('tab', { name: 'CRM' })).toBeTruthy()
    // Nothing is embedded until the user opens the tab.
    expect(document.querySelector('.kw-canvas-frame')).toBeNull()

    fireEvent.click(storefrontTab)
    const frame = document.querySelector('.kw-canvas-frame')
    expect(frame.getAttribute('src')).toBe(STOREFRONT_URL)
    expect(storefrontTab.getAttribute('aria-selected')).toBe('true')

    // Both surfaces stay mounted across a switch so the pages keep their
    // state; only the selected one is visible.
    fireEvent.click(screen.getByRole('tab', { name: 'CRM' }))
    const frames = Array.from(document.querySelectorAll('.kw-canvas-frame'))
    expect(frames.map((f) => f.getAttribute('src'))).toEqual([STOREFRONT_URL, CRM_URL])
    const panes = Array.from(document.querySelectorAll('.kw-canvas-pane--surface'))
    expect(panes.map((p) => p.hasAttribute('hidden'))).toEqual([true, false])
  })

  it('shows the tabs straight away on a resumed session (manifest only)', () => {
    renderCanvasShell({
      initialMessages: [{ role: 'user', text: 'I run a realtor agency' }],
      initialManifest: appliedManifest({ storefrontUrl: STOREFRONT_URL, crmUrl: CRM_URL }),
    })
    expect(screen.getByRole('tab', { name: 'Storefront' })).toBeTruthy()
    expect(screen.getByRole('tab', { name: 'CRM' })).toBeTruthy()
  })

  // The shell is refresh-safe by design, so "resume, then take a turn" is
  // an ordinary path, not an edge case. On that turn the wire carries the
  // counts-only ManifestStatusSummary and the resumed transcript carries no
  // document blocks — neither source can re-prove the URLs. The tabs (and
  // the pane the user is looking at) must still be there.
  it('keeps the resumed tabs — and the selected pane — through a turn', async () => {
    const reply = [{ blockId: 'b-reply', kind: 'text', text: 'Done — both surfaces are updated.' }]
    fetch.mockResolvedValue(
      sseResponse(
        turnWire(reply, {
          blocks: reply,
          latencyMs: 5,
          // What an onboarding turn actually answers with: counts, no URLs.
          manifest: { staged: 5, applied: 5, total: 5 },
        }),
      ),
    )

    renderCanvasShell({
      initialMessages: [
        { role: 'user', text: 'I run a realtor agency' },
        { role: 'bot', text: 'Your surfaces are live.' },
      ],
      initialManifest: appliedManifest({ storefrontUrl: STOREFRONT_URL, crmUrl: CRM_URL }),
    })

    fireEvent.click(screen.getByRole('tab', { name: 'CRM' }))
    expect(document.querySelector('.kw-canvas-frame').getAttribute('src')).toBe(CRM_URL)

    fireEvent.change(screen.getByPlaceholderText('Describe your business…'), {
      target: { value: 'rename the storefront' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Send' }))
    await screen.findByText('Done — both surfaces are updated.')

    expect(screen.getByRole('tab', { name: 'Storefront' })).toBeTruthy()
    const crmTab = screen.getByRole('tab', { name: 'CRM' })
    expect(crmTab.getAttribute('aria-selected')).toBe('true')
    expect(document.querySelector('.kw-canvas-frame').getAttribute('src')).toBe(CRM_URL)
  })
})
