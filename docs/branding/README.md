# Branding assets

Preserved across the 2026-05-13 landing rollback (revert of commit
`cafa5d7 feat(landing): apply brand book v0.6 — logo, tokens, notched UI`).
Files in this folder are the authoritative source; the landing itself
keeps only the logo files needed to render the header + favicon.

## Files

| File | Purpose |
|---|---|
| `logo-mark-e.svg` | Logo mark — E variant (Sky Deep wings + Horizon star, for light backgrounds) |
| `logo-mark-h.svg` | Logo mark — H variant (white wings with grey facets, for dark backgrounds) |
| `logo-icon.jpeg` | Bitmap logo icon (pre-v0.6 asset, kept for completeness) |
| `favicon.svg` | SVG favicon (brand book v0.6 E-mark; replaced the previous purple lightning bolt) |
| `brand-book-v0.6-tokens.css` | Color palette + notch utilities + scanline helpers extracted from the landing's `index.css` before rollback |

## Palette (v0.6)

- Deep Space `#0B1B2E` — primary text / dark backgrounds
- Sky `#5BA3D0` — accent
- Sky Deep `#185FA5` — primary CTA, focus, brand-blue
- Sky Light `#85B7EB` — soft accent
- Horizon `#F5A04A` — secondary accent / brand-orange
- Ignite `#E85D2F` — hot accent / urgent CTA

## When you need to re-apply

Either to the landing or to a new app: copy `brand-book-v0.6-tokens.css`
`:root` block into the target's global CSS, drop the logo files into
`public/`, reference them as `/logo-mark-e.svg` / `/favicon.svg`.
