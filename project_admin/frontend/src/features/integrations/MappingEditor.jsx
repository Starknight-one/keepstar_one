const FIELD_OPTIONS = [
  { value: 'ignore', label: '— ignore —' },
  { value: 'sku', label: 'SKU' },
  { value: 'name', label: 'Name' },
  { value: 'brand', label: 'Brand' },
  { value: 'category', label: 'Category' },
  { value: 'category_slug', label: 'Category slug' },
  { value: 'price', label: 'Price' },
  { value: 'currency', label: 'Currency' },
  { value: 'stock', label: 'Stock' },
  { value: 'rating', label: 'Rating' },
  { value: 'description', label: 'Description' },
  { value: 'images', label: 'Images' },
  { value: 'tags', label: 'Tags' },
  { value: 'duration', label: 'Duration (service)' },
  { value: 'provider', label: 'Provider (service)' },
  { value: 'availability', label: 'Availability (service)' },
]

// MappingEditor renders a row per CSV column with a dropdown to pick the
// internal field. Kept as a named component so the Shopify post-OAuth
// "confirm column mapping" flow can share it later.
export default function MappingEditor({ headers, mapping, confidence, onChange }) {
  return (
    <div className="mapping-editor">
      <div className="mapping-editor-row" style={{ fontWeight: 600, fontSize: 12, color: '#6b7280' }}>
        <span>CSV column</span>
        <span></span>
        <span>Maps to</span>
        <span></span>
      </div>
      {headers.map((header) => {
        const current = mapping[header] ?? 'ignore'
        const conf = confidence?.[header] ?? 0
        const suggested = conf >= 0.7
        return (
          <div key={header} className="mapping-editor-row">
            <span className="mapping-header">{header}</span>
            <span className="mapping-arrow">→</span>
            <select
              className="mapping-select"
              value={current}
              onChange={(e) => onChange(header, e.target.value)}
            >
              {FIELD_OPTIONS.map((opt) => (
                <option key={opt.value} value={opt.value}>{opt.label}</option>
              ))}
            </select>
            {suggested ? (
              <span className="mapping-confidence">AI · {Math.round(conf * 100)}%</span>
            ) : conf > 0 ? (
              <span className="mapping-confidence low">low · {Math.round(conf * 100)}%</span>
            ) : (
              <span></span>
            )}
          </div>
        )
      })}
    </div>
  )
}
