import { useState, useEffect } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { ArrowLeft, Eye, Copy, Archive, Trash2 } from 'lucide-react'
import { api } from '../../shared/api/apiClient.js'
import Button from '../../shared/ui/Button.jsx'
import Spinner from '../../shared/ui/Spinner.jsx'
import './catalog.css'
import './productDetail.css'

export default function ProductDetailPage() {
  const { id } = useParams()
  const navigate = useNavigate()
  const [product, setProduct] = useState(null)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [form, setForm] = useState({})
  const [message, setMessage] = useState('')

  const [allCategories, setAllCategories] = useState([])
  const [linkedCategoryIds, setLinkedCategoryIds] = useState([])
  const [history, setHistory] = useState([])

  useEffect(() => {
    api.get(`/products/${id}`)
      .then((p) => {
        setProduct(p)
        setForm({
          displayName: p.displayName || '',
          description: p.description || '',
          price: p.price || 0,
          stock: p.stockQuantity || 0,
          rating: p.rating || 0,
        })
      })
      .catch(() => navigate('/catalog'))
      .finally(() => setLoading(false))
    api.get('/categories/tenant').then((d) => setAllCategories(d.categories || [])).catch(() => {})
    api.get(`/products/${id}/categories`).then((d) => setLinkedCategoryIds((d.categories || []).map((c) => c.id))).catch(() => {})
    api.get(`/audit?entity_kind=listing&entity_id=${id}&limit=10`).then((d) => setHistory(d.entries || [])).catch(() => {})
  }, [id, navigate])

  async function handleSave() {
    setSaving(true)
    setMessage('')
    try {
      const payload = {
        displayName: form.displayName,
        description: form.description,
        price: form.price,
        stock: form.stock,
        rating: form.rating,
      }
      await api.put(`/products/${id}`, payload)
      // M:N category links — separate endpoint, only fired when user touched it.
      // We always sync to keep the source of truth aligned with what the UI shows.
      await api.put(`/products/${id}/categories`, { categoryIds: linkedCategoryIds })
      setMessage('Saved successfully')
    } catch (err) {
      setMessage(err.message || 'Failed to save')
    } finally {
      setSaving(false)
    }
  }

  function toggleCategory(catId) {
    setLinkedCategoryIds((prev) =>
      prev.includes(catId) ? prev.filter((x) => x !== catId) : [...prev, catId],
    )
  }

  if (loading) return <div className="center-spinner"><Spinner /></div>
  if (!product) return null

  const priceDisplay = form.price ? `$${(form.price / 100).toFixed(2)}` : '—'

  return (
    <div className="pd-page">
      <div className="pd-header">
        <div>
          <button className="pd-back" onClick={() => navigate('/catalog')}>
            <ArrowLeft size={14} /> Back to catalog
          </button>
          <div className="pd-breadcrumb">
            Catalog / <strong>{product.category || 'Uncategorized'}</strong> / {product.name}
          </div>
        </div>
        <Button variant="primary" pill onClick={handleSave} disabled={saving}>
          {saving ? 'Saving…' : 'Save changes'}
        </Button>
      </div>

      {message && (
        <div className={`pd-message ${message.includes('success') ? 'success' : 'error'}`}>{message}</div>
      )}

      <div className="pd-body">
        <div className="pd-main">
          <div className="pd-hero">
            {product.images?.length > 0
              ? <img src={product.images[0]} alt={product.name} className="pd-hero-img" />
              : <div className="pd-hero-img-empty">No image</div>}
            <div className="pd-hero-meta">
              <div>
                <div className="pd-hero-name">{form.displayName || product.name}</div>
                {product.originalName && product.originalName !== product.name && (
                  <div className="pd-hero-original">Original: {product.originalName}</div>
                )}
                <div className="pd-hero-tags">
                  {product.brand && <span className="pd-tag">{product.brand}</span>}
                  {product.category && <span className="pd-tag">{product.category}</span>}
                  {product.sku && <span className="pd-tag">SKU {product.sku}</span>}
                  {product.size && <span className="pd-tag">{product.size}</span>}
                  {product.color && <span className="pd-tag">{product.color}</span>}
                </div>
              </div>
              <div className="pd-hero-price">{priceDisplay}</div>
              {product.description && <p className="pd-hero-desc">{product.description}</p>}
            </div>
          </div>

          <div className="pd-section">
            <div className="pd-section-title">Listing overrides</div>
            <div className="pd-section-hint">These fields override the master record only for your tenant.</div>
            <div className="pd-grid">
              <div className="pd-field" style={{ gridColumn: '1 / -1' }}>
                <label className="pd-field-label">Display name</label>
                <input
                  value={form.displayName}
                  placeholder={product.originalName || product.name || 'Inherits master name'}
                  onChange={(e) => setForm({ ...form, displayName: e.target.value })}
                />
              </div>
              <div className="pd-field" style={{ gridColumn: '1 / -1' }}>
                <label className="pd-field-label">Description</label>
                <textarea rows={3} value={form.description} onChange={(e) => setForm({ ...form, description: e.target.value })} />
              </div>
              <div className="pd-field">
                <label className="pd-field-label">Price (cents)</label>
                <input type="number" value={form.price} onChange={(e) => setForm({ ...form, price: Number(e.target.value) })} />
              </div>
              <div className="pd-field">
                <label className="pd-field-label">Stock</label>
                <input type="number" value={form.stock} onChange={(e) => setForm({ ...form, stock: Number(e.target.value) })} />
              </div>
              <div className="pd-field">
                <label className="pd-field-label">Rating</label>
                <input type="number" step="0.1" min="0" max="5" value={form.rating} onChange={(e) => setForm({ ...form, rating: Number(e.target.value) })} />
              </div>
            </div>
          </div>

          <div className="pd-section">
            <div className="pd-section-title">Categories</div>
            <div className="pd-section-hint">Tenant categories this listing belongs to. Multiple allowed.</div>
            {allCategories.length === 0 ? (
              <div className="pd-cat-empty">No categories yet — add some on the Categories page.</div>
            ) : (
              <div className="pd-cat-list">
                {allCategories.map((c) => (
                  <label key={c.id} className="pd-cat-chip">
                    <input
                      type="checkbox"
                      checked={linkedCategoryIds.includes(c.id)}
                      onChange={() => toggleCategory(c.id)}
                    />
                    <span>{c.name}</span>
                    <span className="pd-cat-kind">{c.kind}</span>
                  </label>
                ))}
              </div>
            )}
          </div>

          <div className="pd-section">
            <div className="pd-section-title">History</div>
            <div className="pd-section-hint">Last 10 changes to this listing.</div>
            {history.length === 0 ? (
              <div className="pd-cat-empty">No history yet — edits will appear here.</div>
            ) : (
              <ul className="pd-history">
                {history.map((h) => (
                  <li key={h.id}>
                    <div className="pd-history-meta">
                      {new Date(h.createdAt).toLocaleString()} · {h.actorKind} · {h.action}
                    </div>
                    {h.fieldChanges && (
                      <div className="pd-history-diff">
                        {Object.entries(h.fieldChanges).map(([k, v]) => (
                          <span key={k} className="pd-history-field">
                            <strong>{k}:</strong> {v.old !== undefined ? `${JSON.stringify(v.old)} → ` : ''}{JSON.stringify(v.new)}
                          </span>
                        ))}
                      </div>
                    )}
                  </li>
                ))}
              </ul>
            )}
          </div>

          <div className="pd-section">
            <div className="pd-section-title">Master fields (read-only)</div>
            <div className="pd-section-hint">Shared across tenants. Edits go through the curator service.</div>
            <div className="pd-grid">
              <div className="pd-field">
                <label className="pd-field-label">Brand</label>
                <input value={product.brand || '—'} disabled />
              </div>
              <div className="pd-field">
                <label className="pd-field-label">SKU</label>
                <input value={product.sku || '—'} disabled />
              </div>
              <div className="pd-field">
                <label className="pd-field-label">GTINs</label>
                <input value={(product.gtins && product.gtins.join(', ')) || '—'} disabled />
              </div>
              <div className="pd-field">
                <label className="pd-field-label">Size</label>
                <input value={product.size || '—'} disabled />
              </div>
              <div className="pd-field">
                <label className="pd-field-label">Color</label>
                <input value={product.color || '—'} disabled />
              </div>
              <div className="pd-field">
                <label className="pd-field-label">Weight (g)</label>
                <input value={product.weightG ?? '—'} disabled />
              </div>
              <div className="pd-field">
                <label className="pd-field-label">Volume (ml)</label>
                <input value={product.volumeMl ?? '—'} disabled />
              </div>
            </div>
          </div>

          <div className="pd-section">
            <div className="pd-section-title">Additional information</div>
            <div className="pd-grid">
              <div className="pd-field">
                <label className="pd-field-label">Social links</label>
                <input placeholder="https://…" disabled />
              </div>
              <div className="pd-field">
                <label className="pd-field-label">Gallery</label>
                <input placeholder="No images attached" disabled />
              </div>
              <div className="pd-field">
                <label className="pd-field-label">Stories</label>
                <input placeholder="—" disabled />
              </div>
              <div className="pd-field">
                <label className="pd-field-label">Reviews</label>
                <input placeholder="No reviews yet" disabled />
              </div>
            </div>
          </div>
        </div>

        <aside className="pd-rail">
          <div className="card">
            <div className="card-title">Variants</div>
            <div className="pd-variants-empty">Single variant — multi-variant support coming soon.</div>
          </div>
          <div className="card">
            <div className="card-title">Performance</div>
            <div className="pd-metric"><span>Views (30d)</span><span>—</span></div>
            <div className="pd-metric"><span>Add-to-cart</span><span>—</span></div>
            <div className="pd-metric"><span>Liked</span><span>—</span></div>
          </div>
          <div className="card">
            <div className="card-title">Quick actions</div>
            <div className="pd-quick-actions">
              <button className="pd-quick-action"><Eye size={14} /> Preview</button>
              <button className="pd-quick-action"><Copy size={14} /> Duplicate</button>
              <button className="pd-quick-action"><Archive size={14} /> Archive</button>
              <button className="pd-quick-action danger"><Trash2 size={14} /> Delete</button>
            </div>
          </div>
        </aside>
      </div>
    </div>
  )
}
