// surfaceLinks — the issued Storefront / CRM addresses, read from what
// the wire ACTUALLY carries (V2_SPEC §2 step 5). Two honest sources,
// because neither one covers both moments:
//
//  1. The onboarding resume payload (GET /onboard/session) carries the
//     FULL manifest: the applied `issue_surface_urls` step's result holds
//     {storefrontUrl, crmUrl} (usecases/manifest_apply.go). This is DB
//     truth and survives a refresh.
//  2. A live turn does NOT: the pipeline response's `manifest` field is
//     the ManifestStatusSummary — counts only (staged/applied/failed), no
//     URLs. In the turn where the links are issued the only carrier is
//     the rendered `surface_links` preset block: one replicated row per
//     surface over the synthetic `surfaceLink` EntitySet
//     ({label, url, surface} — operations/meta_apply_manifest.go).
//
// What the preset actually binds is label + url as TEXT CONTENT
// (seed/surface_links.json: fieldBinding "label" / "url"); `surface` is
// in the data but is not bound to any node. So the kind is derived from
// the URL the server generated — `/s/{slug}` and `/crm/{slug}?k=` (same
// shapes mounts.jsx parses to route those pages).

export const SURFACE_STOREFRONT = 'storefront'
export const SURFACE_CRM = 'crm'

// Stable tab order — Storefront first, then CRM (the flow's own order).
const SURFACE_ORDER = [SURFACE_STOREFRONT, SURFACE_CRM]

const CANONICAL_LABEL = {
  [SURFACE_STOREFRONT]: 'Storefront',
  [SURFACE_CRM]: 'CRM',
}

// surfaceKindOf — which surface a URL addresses, or null when it is not
// a surface URL at all (so an unrelated bound `url` field can never
// conjure a tab).
export function surfaceKindOf(url) {
  if (typeof url !== 'string' || url === '') return null
  let path
  try {
    path = new URL(url).pathname
  } catch (_) {
    return null
  }
  if (/^\/crm\/[^/]+\/?$/.test(path)) return SURFACE_CRM
  if (/^\/s\/[^/]+\/?$/.test(path)) return SURFACE_STOREFRONT
  return null
}

// surfaceLinksFromManifest — source 1 (resume). Only the FULL manifest
// shape has steps; the summary shape yields nothing, by design.
export function surfaceLinksFromManifest(manifest) {
  if (!manifest || !Array.isArray(manifest.steps)) return []
  const out = []
  for (const step of manifest.steps) {
    if (!step || step.op !== 'issue_surface_urls' || !step.result) continue
    push(out, step.result.storefrontUrl)
    push(out, step.result.crmUrl)
  }
  return out
}

// surfaceLinksFromBlocks — source 2 (live turn): walks the rendered
// document blocks for bound surface URLs, keeping the bound label when
// the preset carried one.
export function surfaceLinksFromBlocks(messages) {
  const out = []
  for (const m of messages || []) {
    if (!m || m.role !== 'bot' || !Array.isArray(m.blocks)) continue
    for (const b of m.blocks) {
      if (!b || b.kind === 'text' || !b.document) continue
      collectFromDocument(b.document, out)
    }
  }
  return out
}

// collectSurfaceLinks — one record per surface, newest wins (a re-issue
// replaces the address the tab points at). Manifest last: DB truth
// overrides whatever a stale rendered block still shows.
export function collectSurfaceLinks({ manifest, messages } = {}) {
  const bySurface = new Map()
  for (const link of surfaceLinksFromBlocks(messages)) bySurface.set(link.surface, link)
  for (const link of surfaceLinksFromManifest(manifest)) bySurface.set(link.surface, link)
  return ordered(bySurface)
}

// mergeSurfaceLinks — an ISSUED surface is a fact about the session, not
// about the current render, so a tab that has appeared never disappears:
// `known` carries forward and a fresh sighting only ever re-points its own
// surface. WHY this matters concretely: after a refresh the resume manifest
// is the sole carrier of the URLs (the per-turn `manifest` field is the
// counts-only summary and the resumed transcript carries no document
// blocks), so any source going quiet mid-session must not take the tab —
// and the user's selected pane — down with it.
//
// Returns `known` unchanged when nothing moved, so the surface list keeps a
// stable identity across renders (mounted iframes must not remount).
export function mergeSurfaceLinks(known, found) {
  const bySurface = new Map()
  for (const link of known || []) bySurface.set(link.surface, link)
  let changed = false
  for (const link of found || []) {
    const prev = bySurface.get(link.surface)
    if (prev && prev.url === link.url && prev.label === link.label) continue
    bySurface.set(link.surface, link)
    changed = true
  }
  if (!changed) return known || []
  return ordered(bySurface)
}

function ordered(bySurface) {
  return SURFACE_ORDER.filter((s) => bySurface.has(s)).map((s) => bySurface.get(s))
}

function push(out, url, label) {
  const surface = surfaceKindOf(url)
  if (!surface) return
  out.push({ surface, url, label: label || CANONICAL_LABEL[surface] })
}

// collectFromDocument — one walk over a rendered scene graph, tracking
// the most recent label-bound text so a row's {label, url} pair stays
// together (the preset renders the label immediately before the url).
function collectFromDocument(document, out) {
  let pendingLabel = null
  walk(document, (node) => {
    const binding = node.fieldBinding
    if (binding === 'label' && typeof node.content === 'string') {
      pendingLabel = node.content
      return
    }
    if (binding !== 'url' || typeof node.content !== 'string') return
    const before = out.length
    push(out, node.content, pendingLabel)
    if (out.length > before) pendingLabel = null
  })
}

function walk(node, visit) {
  if (!node || typeof node !== 'object') return
  if (node.fieldBinding) visit(node)
  if (Array.isArray(node.children)) {
    for (const child of node.children) walk(child, visit)
  }
}
