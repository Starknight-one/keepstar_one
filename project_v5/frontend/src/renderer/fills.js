// firstSolidColor — extracts the first solid color from a v9 Fills
// value (`Fill | Fill[]`; Fill = hex string | {type:'color', color} |
// image/gradient/mesh objects). Gradient / image / mesh fills are
// skipped — image-filled rectangles arrive as type `image` already.
// Returns undefined when no solid color is present.

export function firstSolidColor(fills) {
  if (fills === undefined || fills === null) return undefined
  const list = Array.isArray(fills) ? fills : [fills]
  for (const f of list) {
    if (typeof f === 'string' && f !== '') return f
    if (
      f &&
      typeof f === 'object' &&
      f.type === 'color' &&
      typeof f.color === 'string' &&
      f.color !== ''
    ) {
      return f.color
    }
  }
  return undefined
}
