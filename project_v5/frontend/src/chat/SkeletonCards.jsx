// SkeletonCards — shimmering placeholder grid shown between the
// "data found" stage frame and the rendered scene graph. Pure
// presentational: N card shells (image block + two text bars) matching
// product_card geometry.

export default function SkeletonCards({ count }) {
  return (
    <div className="kw-skeleton-row">
      {Array.from({ length: count }, (_, i) => (
        <div key={i} className="kw-skeleton-card">
          <div className="kw-skeleton-image" />
          <div className="kw-skeleton-bar" />
          <div className="kw-skeleton-bar" />
        </div>
      ))}
    </div>
  )
}
