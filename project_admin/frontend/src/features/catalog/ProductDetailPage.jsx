import { useState, useEffect } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { api } from '../../shared/api/apiClient.js'
import Input from '../../shared/ui/Input.jsx'
import Button from '../../shared/ui/Button.jsx'
import Spinner from '../../shared/ui/Spinner.jsx'
import './catalog.css'

export default function ProductDetailPage() {
  const { id } = useParams()
  const navigate = useNavigate()
  const [product, setProduct] = useState(null)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [form, setForm] = useState({})
  const [message, setMessage] = useState('')

  useEffect(() => {
    api.get(`/products/${id}`)
      .then((p) => {
        setProduct(p)
        setForm({
          name: p.name || '',
          description: p.description || '',
          price: p.price || 0,
          stock: p.stockQuantity || 0,
          rating: p.rating || 0,
        })
      })
      .catch(() => navigate('/catalog'))
      .finally(() => setLoading(false))
  }, [id, navigate])

  async function handleSave(e) {
    e.preventDefault()
    setSaving(true)
    setMessage('')
    try {
      await api.put(`/products/${id}`, form)
      setMessage('Saved successfully')
    } catch (err) {
      setMessage(err.message)
    } finally {
      setSaving(false)
    }
  }

  if (loading) return <div className="center-spinner"><Spinner /></div>
  if (!product) return null

  return (
    <div>
      <button className="btn btn-ghost btn-sm" onClick={() => navigate('/catalog')} style={{ marginBottom: 16 }}>
        &larr; Back to products
      </button>
      <h1 className="page-title">{product.name}</h1>

      <div className="product-detail-layout">
        <div className="product-detail-images">
          {product.images?.length > 0 ? (
            <img src={product.images[0]} alt={product.name} className="product-detail-img" />
          ) : (
            <div className="product-detail-img-empty">No image</div>
          )}
        </div>

        <form className="product-detail-form" onSubmit={handleSave}>
          <div className="product-detail-meta">
            <span>Brand: <strong>{product.brand || '—'}</strong></span>
            <span>Category: <strong>{product.category || '—'}</strong></span>
            <span>SKU: <strong>{product.masterProductId?.slice(0, 8) || '—'}</strong></span>
          </div>

          <Input label="Name" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} />
          <div className="input-group">
            <label className="input-label">Description</label>
            <textarea
              className="input"
              rows={3}
              value={form.description}
              onChange={(e) => setForm({ ...form, description: e.target.value })}
            />
          </div>
          <div className="product-detail-row">
            <Input label="Price (kopecks)" type="number" value={form.price} onChange={(e) => setForm({ ...form, price: Number(e.target.value) })} />
            <Input label="Stock" type="number" value={form.stock} onChange={(e) => setForm({ ...form, stock: Number(e.target.value) })} />
            <Input label="Rating" type="number" step="0.1" min="0" max="5" value={form.rating} onChange={(e) => setForm({ ...form, rating: Number(e.target.value) })} />
          </div>

          {message && <div className={message.includes('success') ? 'auth-success' : 'auth-error'}>{message}</div>}
          <Button type="submit" disabled={saving}>
            {saving ? 'Saving...' : 'Save changes'}
          </Button>
        </form>
      </div>
    </div>
  )
}
