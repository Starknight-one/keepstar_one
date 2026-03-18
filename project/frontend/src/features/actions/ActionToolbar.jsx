import { useActions } from './ActionContext'

export function ActionToolbar({ viewMode, onViewChange }) {
  const { likedCount, cartCount } = useActions()

  return (
    <div className="action-toolbar">
      {/* Liked */}
      <button
        className={`action-toolbar-btn${viewMode === 'liked' ? ' active' : ''}`}
        onClick={() => onViewChange(viewMode === 'liked' ? 'normal' : 'liked')}
        aria-label="Liked items"
        title="Liked"
      >
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
          <path d="M20.84 4.61a5.5 5.5 0 0 0-7.78 0L12 5.67l-1.06-1.06a5.5 5.5 0 0 0-7.78 7.78l1.06 1.06L12 21.23l7.78-7.78 1.06-1.06a5.5 5.5 0 0 0 0-7.78z" />
        </svg>
        {likedCount > 0 && <span className="action-badge">{likedCount}</span>}
      </button>

      {/* Cart */}
      <button
        className={`action-toolbar-btn${viewMode === 'cart' ? ' active' : ''}`}
        onClick={() => onViewChange(viewMode === 'cart' ? 'normal' : 'cart')}
        aria-label="Cart"
        title="Cart"
      >
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
          <circle cx="9" cy="21" r="1" /><circle cx="20" cy="21" r="1" />
          <path d="M1 1h4l2.68 13.39a2 2 0 0 0 2 1.61h9.72a2 2 0 0 0 2-1.61L23 6H6" />
        </svg>
        {cartCount > 0 && <span className="action-badge">{cartCount}</span>}
      </button>

      <div className="action-toolbar-divider" />

      {/* Filters placeholder */}
      <button
        className="action-toolbar-btn"
        disabled
        aria-label="Filters"
        title="Filters (coming soon)"
      >
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
          <polygon points="22 3 2 3 10 12.46 10 19 14 21 14 12.46 22 3" />
        </svg>
      </button>

      {/* Sort placeholder */}
      <button
        className="action-toolbar-btn"
        disabled
        aria-label="Sort"
        title="Sort (coming soon)"
      >
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
          <line x1="12" y1="5" x2="12" y2="19" />
          <polyline points="19 12 12 19 5 12" />
        </svg>
      </button>
    </div>
  )
}
