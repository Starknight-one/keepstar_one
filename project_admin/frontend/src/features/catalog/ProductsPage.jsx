import { useState, useEffect, useCallback } from 'react'
import { useNavigate } from 'react-router-dom'
import { api } from '../../shared/api/apiClient.js'
import Table from '../../shared/ui/Table.jsx'
import Pagination from '../../shared/ui/Pagination.jsx'
import Spinner from '../../shared/ui/Spinner.jsx'
import './catalog.css'

const LIMIT = 25

const columns = [
  {
    key: 'image',
    label: '',
    width: '50px',
    render: (row) =>
      row.images?.[0] ? (
        <img src={row.images[0]} alt="" className="product-thumb" />
      ) : (
        <div className="product-thumb-empty" />
      ),
  },
  { key: 'name', label: 'Name' },
  { key: 'brand', label: 'Brand' },
  { key: 'category', label: 'Category' },
  { key: 'priceFormatted', label: 'Price' },
  { key: 'stockQuantity', label: 'Stock' },
]

export default function ProductsPage() {
  const navigate = useNavigate()
  const [products, setProducts] = useState([])
  const [total, setTotal] = useState(0)
  const [offset, setOffset] = useState(0)
  const [search, setSearch] = useState('')
  const [categories, setCategories] = useState([])
  const [categoryId, setCategoryId] = useState('')
  const [loading, setLoading] = useState(true)

  const fetchProducts = useCallback(async () => {
    setLoading(true)
    try {
      const params = new URLSearchParams({ limit: LIMIT, offset })
      if (search) params.set('search', search)
      if (categoryId) params.set('categoryId', categoryId)
      const data = await api.get(`/products?${params}`)
      setProducts(data.products || [])
      setTotal(data.total || 0)
    } catch {
      // ignore
    } finally {
      setLoading(false)
    }
  }, [offset, search, categoryId])

  useEffect(() => {
    fetchProducts()
  }, [fetchProducts])

  useEffect(() => {
    api.get('/categories').then((data) => setCategories(data.categories || [])).catch(() => {})
  }, [])

  function handleSearch(e) {
    e.preventDefault()
    setOffset(0)
    fetchProducts()
  }

  return (
    <div>
      <h1 className="page-title">Products</h1>
      <form className="catalog-filters" onSubmit={handleSearch}>
        <input
          className="input catalog-search"
          placeholder="Search products..."
          value={search}
          onChange={(e) => setSearch(e.target.value)}
        />
        <select
          className="input catalog-category-filter"
          value={categoryId}
          onChange={(e) => { setCategoryId(e.target.value); setOffset(0) }}
        >
          <option value="">All categories</option>
          {categories.map((c) => (
            <option key={c.id} value={c.id}>{c.name}</option>
          ))}
        </select>
      </form>

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
  )
}
