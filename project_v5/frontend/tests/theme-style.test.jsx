import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act } from '@testing-library/react'
import { createRoot } from 'react-dom/client'
import { renderDocument, ALL_CSS } from '../src/widget-preview'
import {
  buildThemeCss,
  applyTheme,
  tokensFromDoc,
  THEME_STYLE_ID,
} from '../src/theme-style'

import productGrid from './fixtures/product-card-grid.json'

// Phase 1 — v5 design-system theme runtime (frontend).
//
// Contract: the backend attaches `theme:{tokens,isDefault}` to the
// rendered document; the frontend reads doc.theme.tokens, builds a
// :host{ --kw-…: … } block and injects it into the shadow root AFTER
// the static widget.css so it OVERRIDES the hardcoded defaults. When no
// tokens are present, NOTHING is injected and widget.css stands as the
// byte-identical defaults.
//
// These tests encode WHY the wiring matters:
//   (a) a theme whose colors.blue differs must override --kw-blue, and
//       the override block must come AFTER widget.css in child order
//       (later same-specificity :host rule wins).
//   (b) with NO theme, no override block is added and the only --kw-*
//       source remains widget.css — i.e. defaults are byte-identical.

// The canonical default tokens — MUST be byte-identical to widget.css
// (the backend returns exactly this with isDefault:true when no tenant
// theme row exists). Mirrors widget.css:16-46 (now :23-53 after the
// cross-link comment).
const DEFAULT_TOKENS = {
  colors: {
    blue: '#5BA4D9',
    orange: '#F0924A',
    bg: '#ffffff',
    text: '#1a1a1a',
    text_muted: '#666666',
    border: '#e5e7eb',
    line: '#e5e7eb',
  },
  radius: { base: 8 },
  gap: { xs: 4, sm: 8, md: 16, lg: 24, xl: 32 },
  fontSize: {
    xs: 11,
    sm: 13,
    base: 14,
    md: 15,
    lg: 18,
    xl: 22,
    '2xl': 28,
    '3xl': 36,
  },
  fontWeight: { light: 300, normal: 400, medium: 500, semibold: 600, bold: 700 },
}

// Parse a `--name: value;` declaration out of widget.css's :host block.
// This reads the LITERAL hardcode so the byte-identical assertion can't
// drift silently — if someone edits widget.css without updating the Go
// default tokens (and this mirror), this test fails.
function cssVar(name) {
  // Match the actual single-line declaration (value can't span lines),
  // not the multi-line header comment that also mentions --kw-blue.
  const re = new RegExp(`${name}\\s*:\\s*([^;\\n]+);`)
  const m = ALL_CSS.match(re)
  return m ? m[1].trim() : null
}

function makeHost() {
  const host = document.createElement('div')
  document.body.appendChild(host)
  return host
}

function themeStyleEl(shadow) {
  return shadow.querySelector(`#${THEME_STYLE_ID}`)
}

describe('buildThemeCss — token -> --kw-* mapping', () => {
  it('maps every token group per the contract (colors raw hex; px; unitless weights)', () => {
    const css = buildThemeCss(DEFAULT_TOKENS)
    // colors raw hex
    expect(css).toContain('--kw-blue: #5BA4D9;')
    expect(css).toContain('--kw-orange: #F0924A;')
    expect(css).toContain('--kw-bg: #ffffff;')
    expect(css).toContain('--kw-text: #1a1a1a;')
    // text_muted -> text-muted remap
    expect(css).toContain('--kw-text-muted: #666666;')
    expect(css).toContain('--kw-border: #e5e7eb;')
    expect(css).toContain('--kw-line: #e5e7eb;')
    // radius number + px
    expect(css).toContain('--kw-radius: 8px;')
    // gaps number + px
    expect(css).toContain('--kw-gap-xs: 4px;')
    expect(css).toContain('--kw-gap-xl: 32px;')
    // fontSize number + px (incl. numeric-prefixed keys)
    expect(css).toContain('--kw-fs-base: 14px;')
    expect(css).toContain('--kw-fs-2xl: 28px;')
    expect(css).toContain('--kw-fs-3xl: 36px;')
    // fontWeight unitless
    expect(css).toContain('--kw-fw-light: 300;')
    expect(css).toContain('--kw-fw-bold: 700;')
    // wrapped in a :host{} block so it overrides widget.css's :host,:root
    expect(css.startsWith(':host{')).toBe(true)
  })

  it('byte-identical invariant: default tokens equal widget.css literals', () => {
    // Every default declaration emitted by buildThemeCss MUST equal the
    // literal value hardcoded in widget.css. If widget.css drifts from
    // the canonical default tokens, this fails loud.
    const checks = [
      ['--kw-blue', '#5BA4D9'],
      ['--kw-orange', '#F0924A'],
      ['--kw-bg', '#ffffff'],
      ['--kw-text', '#1a1a1a'],
      ['--kw-text-muted', '#666666'],
      ['--kw-border', '#e5e7eb'],
      ['--kw-line', '#e5e7eb'],
      ['--kw-radius', '8px'],
      ['--kw-gap-xs', '4px'],
      ['--kw-gap-sm', '8px'],
      ['--kw-gap-md', '16px'],
      ['--kw-gap-lg', '24px'],
      ['--kw-gap-xl', '32px'],
      ['--kw-fs-xs', '11px'],
      ['--kw-fs-sm', '13px'],
      ['--kw-fs-base', '14px'],
      ['--kw-fs-md', '15px'],
      ['--kw-fs-lg', '18px'],
      ['--kw-fs-xl', '22px'],
      ['--kw-fs-2xl', '28px'],
      ['--kw-fs-3xl', '36px'],
      ['--kw-fw-light', '300'],
      ['--kw-fw-normal', '400'],
      ['--kw-fw-medium', '500'],
      ['--kw-fw-semibold', '600'],
      ['--kw-fw-bold', '700'],
    ]
    const css = buildThemeCss(DEFAULT_TOKENS)
    for (const [name, value] of checks) {
      // widget.css literal == default token emission
      expect(cssVar(name)).toBe(value)
      expect(css).toContain(`${name}: ${value};`)
    }
  })

  it('returns empty string for absent / empty / non-object tokens', () => {
    expect(buildThemeCss(undefined)).toBe('')
    expect(buildThemeCss(null)).toBe('')
    expect(buildThemeCss({})).toBe('')
    expect(buildThemeCss({ colors: {} })).toBe('')
    expect(buildThemeCss('nope')).toBe('')
  })

  it('emits only the named keys for a partial theme (rest fall through to widget.css)', () => {
    const css = buildThemeCss({ colors: { blue: '#000000' } })
    expect(css).toBe(':host{ --kw-blue: #000000; }')
  })

  it('ignores invalid token values (non-hex color, non-number size)', () => {
    const css = buildThemeCss({
      colors: { blue: 'red', orange: '#F0924A' },
      gap: { md: '16px' },
      radius: { base: 12 },
    })
    expect(css).not.toContain('--kw-blue')
    expect(css).not.toContain('--kw-gap-md')
    expect(css).toContain('--kw-orange: #F0924A;')
    expect(css).toContain('--kw-radius: 12px;')
  })
})

describe('tokensFromDoc — precedence doc.theme || opts.theme || none', () => {
  it('prefers doc.theme.tokens', () => {
    const doc = { theme: { tokens: { colors: { blue: '#111111' } } } }
    const opts = { theme: { tokens: { colors: { blue: '#222222' } } } }
    expect(tokensFromDoc(doc, opts).colors.blue).toBe('#111111')
  })
  it('falls back to opts.theme.tokens when doc carries none', () => {
    const doc = { children: [] }
    const opts = { theme: { tokens: { colors: { blue: '#222222' } } } }
    expect(tokensFromDoc(doc, opts).colors.blue).toBe('#222222')
  })
  it('returns undefined when neither is present', () => {
    expect(tokensFromDoc({ children: [] }, {})).toBeUndefined()
    expect(tokensFromDoc(null, null)).toBeUndefined()
  })
})

describe('applyTheme — idempotent shadow-root injector', () => {
  let shadow
  beforeEach(() => {
    const host = makeHost()
    shadow = host.attachShadow({ mode: 'open' })
    const base = document.createElement('style')
    base.textContent = ALL_CSS
    shadow.appendChild(base)
  })
  afterEach(() => {
    document.body.innerHTML = ''
  })

  it('appends the theme <style> AFTER widget.css (override DOM order)', () => {
    applyTheme(shadow, { colors: { blue: '#000000' } })
    const styles = shadow.querySelectorAll('style')
    expect(styles.length).toBe(2)
    // The widget.css block is first; the theme override is LAST.
    expect(styles[0].textContent).toBe(ALL_CSS)
    expect(styles[1].id).toBe(THEME_STYLE_ID)
    expect(styles[1].textContent).toContain('--kw-blue: #000000;')
  })

  it('replaces in place on re-apply (no stacking)', () => {
    applyTheme(shadow, { colors: { blue: '#000000' } })
    applyTheme(shadow, { colors: { blue: '#abcabc' } })
    const themed = shadow.querySelectorAll(`#${THEME_STYLE_ID}`)
    expect(themed.length).toBe(1)
    expect(themed[0].textContent).toContain('--kw-blue: #abcabc;')
    // Still exactly 2 styles total — widget.css + the single override.
    expect(shadow.querySelectorAll('style').length).toBe(2)
    // And the override is still last in child order.
    const all = shadow.querySelectorAll('style')
    expect(all[all.length - 1].id).toBe(THEME_STYLE_ID)
  })

  it('removes a prior block and injects nothing when tokens go absent', () => {
    applyTheme(shadow, { colors: { blue: '#000000' } })
    expect(themeStyleEl(shadow)).not.toBeNull()
    applyTheme(shadow, undefined)
    expect(themeStyleEl(shadow)).toBeNull()
    // widget.css survives untouched => defaults stand.
    expect(shadow.querySelectorAll('style').length).toBe(1)
  })

  it('injects nothing for empty tokens (byte-identical default behavior)', () => {
    applyTheme(shadow, {})
    expect(themeStyleEl(shadow)).toBeNull()
    expect(shadow.querySelectorAll('style').length).toBe(1)
  })
})

describe('renderDocument — theme injection (preview path)', () => {
  let fetchMock
  beforeEach(() => {
    fetchMock = vi.fn(() => Promise.reject(new Error('network forbidden in preview')))
    vi.stubGlobal('fetch', fetchMock)
  })
  afterEach(() => {
    vi.unstubAllGlobals()
    document.body.innerHTML = ''
  })

  it('(a) doc.theme with a differing colors.blue overrides --kw-blue, after widget.css', async () => {
    const host = makeHost()
    const themedDoc = {
      ...productGrid,
      theme: {
        tokens: { colors: { blue: '#0A0A0A' } },
        isDefault: false,
      },
    }
    await act(async () => {
      renderDocument(host, themedDoc)
    })
    const shadow = host.shadowRoot
    const override = themeStyleEl(shadow)
    expect(override).not.toBeNull()
    expect(override.textContent).toContain('--kw-blue: #0A0A0A;')

    // PRECEDENCE: the override <style> must come AFTER the widget.css
    // <style> in shadow-root child order, else the later-rule-wins rule
    // doesn't apply and the override silently fails.
    const styles = Array.from(shadow.querySelectorAll('style'))
    const baseIdx = styles.findIndex((s) => s.textContent === ALL_CSS)
    const overrideIdx = styles.findIndex((s) => s.id === THEME_STYLE_ID)
    expect(baseIdx).toBeGreaterThanOrEqual(0)
    expect(overrideIdx).toBeGreaterThan(baseIdx)

    // widget.css still declares the DEFAULT blue (override is additive,
    // not a rewrite) — proves the win is by cascade order, not edit.
    expect(cssVar('--kw-blue')).toBe('#5BA4D9')
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('(b) NO theme: no override block; the only --kw-* source is widget.css (byte-identical)', async () => {
    const host = makeHost()
    await act(async () => {
      renderDocument(host, productGrid) // fixture carries no theme
    })
    const shadow = host.shadowRoot
    // No override block at all.
    expect(themeStyleEl(shadow)).toBeNull()
    // Exactly the widget.css <style> remains as the --kw-* source.
    const styles = shadow.querySelectorAll('style')
    expect(styles.length).toBe(1)
    expect(styles[0].textContent).toBe(ALL_CSS)
    // And that source carries the byte-identical defaults.
    expect(cssVar('--kw-blue')).toBe('#5BA4D9')
    expect(cssVar('--kw-radius')).toBe('8px')
    expect(cssVar('--kw-fs-3xl')).toBe('36px')
  })

  it('honors opts.theme when doc.theme is absent (admin preview convenience)', async () => {
    const host = makeHost()
    await act(async () => {
      renderDocument(host, productGrid, {
        theme: { tokens: { colors: { blue: '#FE00FE' } } },
      })
    })
    const override = themeStyleEl(host.shadowRoot)
    expect(override).not.toBeNull()
    expect(override.textContent).toContain('--kw-blue: #FE00FE;')
  })

  it('doc.theme wins over opts.theme', async () => {
    const host = makeHost()
    await act(async () => {
      renderDocument(
        host,
        { ...productGrid, theme: { tokens: { colors: { blue: '#111111' } } } },
        { theme: { tokens: { colors: { blue: '#222222' } } } },
      )
    })
    expect(themeStyleEl(host.shadowRoot).textContent).toContain('--kw-blue: #111111;')
  })

  it('re-render with a different theme replaces (does not stack) the override', async () => {
    const host = makeHost()
    await act(async () => {
      renderDocument(host, { ...productGrid, theme: { tokens: { colors: { blue: '#000000' } } } })
    })
    await act(async () => {
      renderDocument(host, { ...productGrid, theme: { tokens: { colors: { blue: '#999999' } } } })
    })
    const shadow = host.shadowRoot
    expect(shadow.querySelectorAll(`#${THEME_STYLE_ID}`).length).toBe(1)
    expect(themeStyleEl(shadow).textContent).toContain('--kw-blue: #999999;')
  })

  it('re-render from themed to un-themed drops the override (back to defaults)', async () => {
    const host = makeHost()
    await act(async () => {
      renderDocument(host, { ...productGrid, theme: { tokens: { colors: { blue: '#000000' } } } })
    })
    expect(themeStyleEl(host.shadowRoot)).not.toBeNull()
    await act(async () => {
      renderDocument(host, productGrid) // no theme
    })
    expect(themeStyleEl(host.shadowRoot)).toBeNull()
  })
})

// Chat path — WidgetApp owns the doc state and the shadow root is
// threaded in from widget.jsx mount(). When a pipeline turn lands a doc
// carrying theme.tokens, WidgetApp must inject the override into THAT
// shadow root (after widget.css). This drives WidgetApp with a mocked
// api client so the turn resolves deterministically.
vi.mock('../src/api/client', () => ({
  initSession: vi.fn(() => Promise.resolve({ sessionId: 'sess-1' })),
  pipelineSmartRequest: vi.fn(() =>
    Promise.resolve({
      document: {
        version: '2.10',
        children: [{ type: 'text', id: 't1', content: 'Hello' }],
        theme: { tokens: { colors: { blue: '#C0FFEE' } }, isDefault: false },
      },
      prefetch: { adjacentTemplate: {}, entities: {} },
      facets: [],
      toolCalls: [],
      spans: [],
    }),
  ),
}))

describe('WidgetApp — chat-path theme injection into the threaded shadow root', () => {
  let WidgetApp
  beforeEach(async () => {
    vi.stubGlobal('fetch', vi.fn(() => new Promise(() => {})))
    WidgetApp = (await import('../src/WidgetApp')).default
  })
  afterEach(() => {
    vi.unstubAllGlobals()
    document.body.innerHTML = ''
  })

  it('a pipeline turn whose doc carries theme.tokens injects the override after widget.css', async () => {
    // Build the same shadow-root chrome widget.jsx mount() builds.
    const host = document.createElement('div')
    document.body.appendChild(host)
    const shadow = host.attachShadow({ mode: 'open' })
    const base = document.createElement('style')
    base.textContent = ALL_CSS
    shadow.appendChild(base)
    const mountPoint = document.createElement('div')
    shadow.appendChild(mountPoint)

    const root = createRoot(mountPoint)
    await act(async () => {
      root.render(
        <WidgetApp tenantSlug="acme" apiBaseUrl="http://x/api/v1" shadowRoot={shadow} />,
      )
    })

    // No doc yet -> no override.
    expect(shadow.querySelector(`#${THEME_STYLE_ID}`)).toBeNull()

    // Drive a turn: reach into ChatPanel's input + send. Simpler: call
    // the exposed send by simulating the form. The fixture renders a
    // chat input; submit a query through it.
    const input = shadow.querySelector('.kw-chat-input input')
    const form = shadow.querySelector('form.kw-chat-input')
    expect(input).not.toBeNull()
    expect(form).not.toBeNull()

    // Set the controlled input through React's value tracker so onChange
    // fires and `draft` updates, then submit the form.
    const setter = Object.getOwnPropertyDescriptor(
      window.HTMLInputElement.prototype,
      'value',
    ).set
    await act(async () => {
      setter.call(input, 'show me products')
      input.dispatchEvent(new Event('input', { bubbles: true }))
    })
    await act(async () => {
      form.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    })

    // The mocked turn resolved a doc with theme.colors.blue=#C0FFEE.
    const override = shadow.querySelector(`#${THEME_STYLE_ID}`)
    expect(override).not.toBeNull()
    expect(override.textContent).toContain('--kw-blue: #C0FFEE;')

    // Override is LAST in child order (after widget.css), so it wins.
    const styles = Array.from(shadow.querySelectorAll('style'))
    expect(styles[styles.length - 1].id).toBe(THEME_STYLE_ID)

    await act(async () => {
      root.unmount()
    })
  })
})
