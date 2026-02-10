import './ui.css'

export default function Pagination({ total, limit, offset, onChange }) {
  const totalPages = Math.ceil(total / limit)
  const currentPage = Math.floor(offset / limit) + 1

  if (totalPages <= 1) return null

  return (
    <div className="pagination">
      <button
        className="btn btn-ghost btn-sm"
        disabled={currentPage <= 1}
        onClick={() => onChange((currentPage - 2) * limit)}
      >
        Previous
      </button>
      <span className="pagination-info">
        Page {currentPage} of {totalPages} ({total} items)
      </span>
      <button
        className="btn btn-ghost btn-sm"
        disabled={currentPage >= totalPages}
        onClick={() => onChange(currentPage * limit)}
      >
        Next
      </button>
    </div>
  )
}
