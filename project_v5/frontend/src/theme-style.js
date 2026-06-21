// theme-style — turns a document's `theme.tokens` (the canonical
// --kw-* design-system token set) into a `:host{ --kw-…: … }` <style>
// block and injects it into a shadow root AFTER the static widget.css
// so it OVERRIDES the hardcoded defaults (later same-specificity :host
// rule wins).
//
// DELIVERY PATH (fixed): the backend attaches `theme:{tokens,isDefault}`
// to the rendered document (doc.theme). When no tenant theme row exists
// the backend returns the byte-identical defaults with isDefault:true,
// so the injected vars equal the widget.css hardcode => zero visual
// change. When the backend attaches NOTHING (no theme key / no tokens),
// we inject nothing and the static widget.css stands as the defaults —
// also byte-identical.
//
// The token model + the --kw-* mapping is the contract. It MUST stay in
// lockstep with the hardcoded defaults in widget.css:16-46 — see
// applyTheme()'s mapping below, which mirrors that block exactly.

// Stable id for the injected style element so re-renders REPLACE in
// place instead of stacking a new block every turn.
const THEME_STYLE_ID = 'kw-theme'

// Per-token-group emitters. Each takes the group object from
// theme.tokens and pushes `--kw-…: …;` declarations onto `out`.
//   colors.{k}     -> --kw-{remap}   raw hex
//   radius.base    -> --kw-radius     number + "px"
//   gap.{k}        -> --kw-gap-{k}    number + "px"
//   fontSize.{k}   -> --kw-fs-{k}     number + "px"
//   fontWeight.{k} -> --kw-fw-{k}     unitless number
// Unknown keys are ignored; absent groups contribute nothing (so a
// partial theme overrides only what it names, leaving widget.css for the
// rest).

// colors uses an explicit remap because two keys differ from their
// css-var suffix (text_muted -> text-muted) and the var names are
// fixed by widget.css:16-23.
const COLOR_VARS = {
  blue: '--kw-blue',
  orange: '--kw-orange',
  bg: '--kw-bg',
  text: '--kw-text',
  text_muted: '--kw-text-muted',
  border: '--kw-border',
  line: '--kw-line',
}

function isHex(v) {
  return typeof v === 'string' && /^#[0-9a-fA-F]{3,8}$/.test(v.trim())
}

function isNum(v) {
  return typeof v === 'number' && Number.isFinite(v)
}

// buildThemeCss — pure: tokens object -> CSS string, or '' when there
// is nothing valid to emit. Exported for tests.
export function buildThemeCss(tokens) {
  if (!tokens || typeof tokens !== 'object') return ''
  const out = []

  const colors = tokens.colors
  if (colors && typeof colors === 'object') {
    for (const [k, varName] of Object.entries(COLOR_VARS)) {
      const v = colors[k]
      if (isHex(v)) out.push(`${varName}: ${v.trim()};`)
    }
  }

  const radius = tokens.radius
  if (radius && typeof radius === 'object' && isNum(radius.base)) {
    out.push(`--kw-radius: ${radius.base}px;`)
  }

  const gap = tokens.gap
  if (gap && typeof gap === 'object') {
    for (const k of ['xs', 'sm', 'md', 'lg', 'xl']) {
      if (isNum(gap[k])) out.push(`--kw-gap-${k}: ${gap[k]}px;`)
    }
  }

  const fontSize = tokens.fontSize
  if (fontSize && typeof fontSize === 'object') {
    for (const k of ['xs', 'sm', 'base', 'md', 'lg', 'xl', '2xl', '3xl']) {
      if (isNum(fontSize[k])) out.push(`--kw-fs-${k}: ${fontSize[k]}px;`)
    }
  }

  const fontWeight = tokens.fontWeight
  if (fontWeight && typeof fontWeight === 'object') {
    for (const k of ['light', 'normal', 'medium', 'semibold', 'bold']) {
      if (isNum(fontWeight[k])) out.push(`--kw-fw-${k}: ${fontWeight[k]};`)
    }
  }

  if (out.length === 0) return ''
  return `:host{ ${out.join(' ')} }`
}

// applyTheme — idempotent injector. Reads `doc?.theme?.tokens` and
// upserts a single <style id="kw-theme"> APPENDED to the shadow root
// (so it follows the static widget.css <style> and overrides it).
//
//   - Replaces in place on re-render (same id) — no stacking.
//   - No tokens / empty css -> REMOVES any prior block and injects
//     nothing, leaving the static widget.css defaults intact
//     (byte-identical default behavior).
//   - Appends (never prepends): override correctness depends on this
//     element coming AFTER widget.css in shadow-root child order.
export function applyTheme(shadowRoot, tokens) {
  if (!shadowRoot) return
  const css = buildThemeCss(tokens)
  const existing = shadowRoot.getElementById
    ? shadowRoot.getElementById(THEME_STYLE_ID)
    : shadowRoot.querySelector(`#${THEME_STYLE_ID}`)

  if (!css) {
    if (existing) existing.remove()
    return
  }

  if (existing) {
    // Replace-in-place AND re-append so a later doc keeps the override
    // last in child order even if other nodes were appended meanwhile.
    existing.textContent = css
    shadowRoot.appendChild(existing)
    return
  }

  const style = document.createElement('style')
  style.id = THEME_STYLE_ID
  style.textContent = css
  shadowRoot.appendChild(style)
}

// tokensFromDoc — precedence helper: doc.theme.tokens || opts.theme.tokens
// (admin preview passes a theme via opts when the doc carries none).
// Returns undefined when neither is present (=> inject nothing).
export function tokensFromDoc(doc, opts) {
  const fromDoc = doc?.theme?.tokens
  if (fromDoc) return fromDoc
  const fromOpts = opts?.theme?.tokens
  if (fromOpts) return fromOpts
  return undefined
}

export { THEME_STYLE_ID }
