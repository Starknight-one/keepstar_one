import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../api.js'

export default function MasterCatalogPage() {
  const [products, setProducts] = useState([])
  const [total, setTotal] = useState(0)
  const [vertical, setVertical] = useState('')
  const [search, setSearch] = useState('')
  const [offset, setOffset] = useState(0)
  const [loading, setLoading] = useState(true)
  const limit = 50

  useEffect(() => { load() }, [vertical, search, offset])
  async function load() {
    setLoading(true)
    try {
      const params = new URLSearchParams({ limit: String(limit), offset: String(offset) })
      if (vertical) params.set('vertical', vertical)
      if (search) params.set('search', search)
      const data = await api.get(`/curator/master/products?${params}`)
      setProducts(data.products || [])
      setTotal(data.total || 0)
    } catch {} finally { setLoading(false) }
  }

  return (
    <div className="page">
      <h1>Master Catalog</h1>
      <div className="toolbar">
        <select className="search" value={vertical} onChange={e => { setVertical(e.target.value); setOffset(0) }}>
          <option value="">all verticals</option>
          <option value="cosmetics">cosmetics</option>
          <option value="furniture">furniture</option>
          <option value="electronics">electronics</option>
          <option value="winter_sports">winter_sports</option>
        </select>
        <input className="search" placeholder="search name / brand…"
          value={search} onChange={e => { setSearch(e.target.value); setOffset(0) }} />
        <span className="muted">total {total.toLocaleString()}</span>
      </div>
      {loading ? <div className="loading">loading…</div> :
        products.length === 0 ? <div className="empty">no master products</div> :
          <table className="products">
            <thead>
              <tr>
                <th></th><th>Name</th><th>Brand</th><th>Vertical</th>
                <th>Listings</th><th>Coverage</th><th>Owner</th>
              </tr>
            </thead>
            <tbody>
              {products.map(p => (
                <tr key={p.id}>
                  <td>{p.imageUrl ? <img src={p.imageUrl} alt="" className="thumb" /> : <div className="thumb" />}</td>
                  <td><Link to={`/master/${p.id}`} className="prod-name">{p.name}</Link></td>
                  <td>{p.brand || <span className="muted">—</span>}</td>
                  <td><span className="vertical-pill">{p.vertical || '—'}</span></td>
                  <td className="num">{p.listingCount}</td>
                  <td>
                    <CoverageDots
                      hasEmbedding={p.hasEmbedding}
                      hasPIM={p.hasPIM}
                      hasDescription={p.hasDescription}
                    />
                  </td>
                  <td><span className="muted">{p.ownerTenantSlug || '—'}</span></td>
                </tr>
              ))}
            </tbody>
          </table>}
      <Pagination offset={offset} limit={limit} total={total} onOffset={setOffset} />
    </div>
  )
}

function CoverageDots({ hasEmbedding, hasPIM, hasDescription }) {
  return (
    <div className="dots">
      <Dot active={hasEmbedding} title="vector embedding" label="V" />
      <Dot active={hasPIM} title="PIM enrichment (skin_type/concern/ingredients)" label="P" />
      <Dot active={hasDescription} title="description" label="D" />
    </div>
  )
}
function Dot({ active, title, label }) {
  return <span className={`dot ${active ? 'dot-on' : 'dot-off'}`} title={title}>{label}</span>
}

function Pagination({ offset, limit, total, onOffset }) {
  if (total <= limit) return null
  const page = Math.floor(offset / limit) + 1
  const pages = Math.ceil(total / limit)
  return (
    <div className="pagination">
      <button disabled={offset === 0} onClick={() => onOffset(Math.max(0, offset - limit))}>← prev</button>
      <span className="muted">page {page} / {pages}</span>
      <button disabled={offset + limit >= total} onClick={() => onOffset(offset + limit)}>next →</button>
    </div>
  )
}
