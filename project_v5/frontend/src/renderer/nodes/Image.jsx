// Image — leaf node carrying either:
//   node.fills[0].url      — set by binding when fieldBinding resolves
//                            an image. Mode "fill" implies cover.
//   node.content (URL str) — fallback shape used by some atoms.
// Reads node.mediaStyle for {aspectRatio, objectFit}.

export default function Image({ node }) {
  const url = pickURL(node)
  if (!url) return null

  const ms = node.mediaStyle || {}
  return (
    <img
      className="kw-image"
      src={url}
      alt={node.alt || ''}
      data-id={node.id || ''}
      data-aspect={ms.aspectRatio || ''}
      data-fit={ms.objectFit || 'cover'}
      loading="lazy"
    />
  )
}

function pickURL(node) {
  if (Array.isArray(node.fills) && node.fills.length > 0) {
    const f = node.fills[0]
    if (f && typeof f === 'object' && typeof f.url === 'string' && f.url) {
      return f.url
    }
  }
  if (typeof node.content === 'string' && node.content.startsWith('http')) {
    return node.content
  }
  // Backend pre-binding sometimes leaves an array of URLs as content
  // (heroImage from product → list of URLs). Take the first.
  if (Array.isArray(node.content) && typeof node.content[0] === 'string') {
    return node.content[0]
  }
  return null
}
