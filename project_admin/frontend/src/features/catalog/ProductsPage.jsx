import { useState, useEffect, useCallback } from 'react'
import { useNavigate } from 'react-router-dom'
import { Search } from 'lucide-react'
import { api } from '../../shared/api/apiClient.js'
import Table from '../../shared/ui/Table.jsx'
import Pagination from '../../shared/ui/Pagination.jsx'
import Spinner from '../../shared/ui/Spinner.jsx'
import Button from '../../shared/ui/Button.jsx'
import CategoryTree from './CategoryTree.jsx'
import './catalog.css'

const LIMIT = 25

function stockStatus(qty) {
  if (qty == null) return { cls: 'status-done', label: 'Unknown' }
  if (qty <= 0) return { cls: 'status-out', label: 'Out of stock' }
  if (qty < 10) return { cls: 'status-low', label: 'Low stock' }
  return { cls: 'status-active', label: 'Active' }
}

const columns = [
  {
    key: 'product',
    label: 'Product',
    render: (row) => (
      <div className="product-cell">
        {row.images?.[0]
          ? <img src={row.images[0]} alt="" className="product-thumb" />
          : <div className="product-thumb-empty" />}
        <div>
          <div className="product-cell-name">{row.name}</div>
          {row.originalName && row.originalName !== row.name && (
            <div className="product-cell-original">{row.originalName}</div>
          )}
          {row.brand && <div className="product-cell-brand">{row.brand}</div>}
        </div>
      </div>
    ),
  },
  {
    key: 'sku',
    label: 'SKU',
    width: '120px',
    render: (r) => r.sku || r.masterVariantId?.slice(0, 8) || r.masterProductId?.slice(0, 8) || '—',
  },
  { key: 'price', label: 'Price', width: '100px', render: (r) => r.priceFormatted || (r.price ? `$${(r.price / 100).toFixed(2)}` : '—') },
  { key: 'stock', label: 'Stock', width: '90px', render: (r) => r.stockQuantity ?? '—' },
  {
    key: 'status', label: 'Status', width: '130px',
    render: (r) => {
      const s = stockStatus(r.stockQuantity)
      return <span className={`status-badge ${s.cls}`}>{s.label}</span>
    },
  },
]

export default function ProductsPage() {
  const navigate = useNavigate()
  const [products, setProducts] = useState([])
  const [total, setTotal] = useState(0)
  const [offset, setOffset] = useState(0)
  const [search, setSearch] = useState('')
  const [category, setCategory] = useState({ id: '', name: 'All' })
  const [loading, setLoading] = useState(true)

  const fetchProducts = useCallback(async () => {
    setLoading(true)
    try {
      const params = new URLSearchParams({ limit: LIMIT, offset })
      if (search) params.set('search', search)
      if (category.id) params.set('categoryId', category.id)
      const data = await api.get(`/products?${params}`)
      setProducts(data.products || [])
      setTotal(data.total || 0)
    } catch {
      setProducts([])
    } finally {
      setLoading(false)
    }
  }, [offset, search, category])

  useEffect(() => { fetchProducts() }, [fetchProducts])

  function handleSearch(e) {
    e.preventDefault()
    setOffset(0)
    fetchProducts()
  }

  return (
    <div className="catalog-page">
      <CategoryTree value={category} onChange={(v) => { setCategory(v); setOffset(0) }} />

      <div className="catalog-main">
        <div className="page-header">
          <div>
            <div className="page-title">Catalog</div>
            <div className="catalog-breadcrumb">
              Products / <strong>{category.name || 'All'}</strong> · {total} items
            </div>
          </div>
          <form className="page-toolbar" onSubmit={handleSearch}>
            <div className="search-input">
              <Search size={14} />
              <input
                placeholder="Search products…"
                value={search}
                onChange={(e) => setSearch(e.target.value)}
              />
            </div>
            <Button variant="primary" pill type="submit">Search</Button>
          </form>
        </div>

        {loading ? (
          <div className="center-spinner"><Spinner /></div>
        ) : (
          <>
            <Table
              columns={columns}
              data={products}
              onRowClick={(row) => navigate(`/catalog/${row.id}`)}
            />
            <Pagination total={total} limit={LIMIT} offset={offset} onChange={setOffset} />
          </>
        )}
      </div>
    </div>
  )
}
