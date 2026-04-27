import { useEffect, useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import { api } from '../api.js'

export default function MasterDetailPage() {
  const { id } = useParams()
  const [master, setMaster] = useState(null)
  const [error, setError] = useState(null)

  useEffect(() => {
    api.get(`/curator/master/products/${id}`).then(setMaster).catch(e => setError(e.message))
  }, [id])

  if (error) return <div className="page"><div className="error">{error}</div></div>
  if (!master) return <div className="page"><div className="loading">loading…</div></div>

  return (
    <div className="page">
      <div className="breadcrumb"><Link to="/master">Master Catalog</Link> / {master.name}</div>
      <div className="master-header">
        {master.imageUrl ? <img src={master.imageUrl} alt="" className="master-img" /> : <div className="master-img" />}
        <div className="master-info">
          <h1>{master.name}</h1>
          <div className="tenant-meta">
            <Pill label="vertical" value={master.vertical || '—'} />
            <Pill label="brand" value={master.brand || '—'} />
            <Pill label="owner" value={master.ownerTenantSlug || '—'} />
            <Pill label="listings" value={String(master.listingCount)} />
            <Pill label="embedding" value={master.hasEmbedding ? 'yes' : 'no'} />
            <Pill label="PIM" value={master.hasPIM ? 'yes' : 'no'} />
          </div>
        </div>
      </div>

      {master.description && (
        <section className="master-section">
          <h2>Description</h2>
          <p className="prose">{master.description}</p>
        </section>
      )}

      {master.pimFields && Object.keys(master.pimFields).length > 0 && (
        <section className="master-section">
          <h2>PIM fields</h2>
          <div className="pim-grid">
            {Object.entries(master.pimFields).map(([k, v]) => (
              <div key={k} className="pim-row">
                <div className="pim-key">{k}</div>
                <div className="pim-val">{Array.isArray(v) ? v.join(', ') : String(v)}</div>
              </div>
            ))}
          </div>
        </section>
      )}

      <section className="master-section">
        <h2>Variants ({master.variants.length})</h2>
        {master.variants.length === 0 ?
          <div className="empty">no variants — listings link directly via master_product_id (legacy heybabes shape)</div> :
          <table className="products">
            <thead>
              <tr><th>SKU</th><th>GTINs</th><th>Color</th><th>Size</th><th>Material</th><th>Weight (g)</th><th>Volume (ml)</th></tr>
            </thead>
            <tbody>
              {master.variants.map(v => (
                <tr key={v.id}>
                  <td className="mono">{v.sku || <span className="muted">—</span>}</td>
                  <td className="mono">{v.gtins.length > 0 ? v.gtins.join(', ') : <span className="muted">—</span>}</td>
                  <td>{v.color || <span className="muted">—</span>}</td>
                  <td>{v.size || <span className="muted">—</span>}</td>
                  <td>{v.material || <span className="muted">—</span>}</td>
                  <td className="num">{v.weightG || <span className="muted">—</span>}</td>
                  <td className="num">{v.volumeMl || <span className="muted">—</span>}</td>
                </tr>
              ))}
            </tbody>
          </table>}
      </section>

      <section className="master-section">
        <h2>Linked listings ({master.linkedListings.length})</h2>
        {master.linkedListings.length === 0 ?
          <div className="empty">no tenant has a listing pointing at this master</div> :
          <table className="products">
            <thead>
              <tr><th>Listing</th><th>SKU</th><th>Brand</th><th>Price</th><th>Stock</th><th>Source</th></tr>
            </thead>
            <tbody>
              {master.linkedListings.map(l => (
                <tr key={l.id}>
                  <td>
                    <div className="prod-name">{l.name}</div>
                    {l.originalName && l.originalName !== l.name && <div className="prod-sub">{l.originalName}</div>}
                  </td>
                  <td className="mono">{l.sku || <span className="muted">—</span>}</td>
                  <td>{l.brand || <span className="muted">—</span>}</td>
                  <td className="num">{l.priceCents ? `${(l.priceCents / 100).toFixed(2)} ${l.currency || ''}` : <span className="muted">—</span>}</td>
                  <td className="num">{l.stock}</td>
                  <td><span className="muted">{l.sourceSystem || '—'}</span></td>
                </tr>
              ))}
            </tbody>
          </table>}
      </section>
    </div>
  )
}

function Pill({ label, value }) {
  return <div className="pill"><span className="pill-label">{label}</span><span className="pill-value">{value}</span></div>
}
