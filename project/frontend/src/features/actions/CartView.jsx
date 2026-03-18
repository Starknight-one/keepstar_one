import { useMemo, useState, useEffect } from 'react'
import { useActions } from './ActionContext'
import { getActionView } from '../../shared/api/apiClient'
import { loadSessionCache } from '../chat/sessionCache'
import { log } from '../../shared/logger'

export function CartView({ entities, onBack }) {
  const { cartItems, updateCartQuantity, removeFromCart } = useActions()
  const [backendActions, setBackendActions] = useState(null)

  // Fetch backend state on mount for data consistency
  useEffect(() => {
    if (cartItems.size === 0) return
    const sessionId = loadSessionCache()?.sessionId
    if (!sessionId) return

    getActionView(sessionId, 'cart')
      .then(resp => {
        if (resp.success) {
          setBackendActions(resp.actions)
        }
      })
      .catch(err => log.warn('CartView backend fetch failed:', err))
  }, [cartItems.size])

  const items = useMemo(() => {
    if (cartItems.size === 0 || !entities) return []

    const products = entities.products || []
    const services = entities.services || []

    const result = []
    for (const [id, cartItem] of cartItems) {
      const entity = products.find(p => p.id === id) || services.find(s => s.id === id)
      if (entity) {
        result.push({ ...cartItem, entity })
      }
    }
    return result
  }, [cartItems, entities])

  const total = useMemo(() => {
    return items.reduce((sum, item) => {
      const price = item.entity.price || 0
      return sum + price * item.quantity
    }, 0)
  }, [items])

  return (
    <div className="cart-view">
      <div className="cart-view-header">
        <button className="cart-view-back" onClick={onBack} aria-label="Back">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <polyline points="15 18 9 12 15 6" />
          </svg>
        </button>
        <h2>Cart ({cartItems.size})</h2>
      </div>

      {items.length === 0 ? (
        <div className="cart-empty">
          <span className="cart-empty-icon">&#128722;</span>
          <span>Cart is empty</span>
        </div>
      ) : (
        <>
          <div className="cart-items">
            {items.map(item => (
              <div key={item.entityId} className="cart-item">
                <div className="cart-item-image">
                  {item.entity.images?.[0] && (
                    <img src={item.entity.images[0]} alt="" />
                  )}
                </div>
                <div className="cart-item-info">
                  <div className="cart-item-name">{item.entity.name}</div>
                  {item.entity.price != null && (
                    <div className="cart-item-price">
                      {item.entity.currency || '$'}{item.entity.price}
                    </div>
                  )}
                  <div className="cart-item-controls">
                    <button
                      className="cart-qty-btn"
                      onClick={() => updateCartQuantity(item.entityId, -1)}
                    >
                      -
                    </button>
                    <span className="cart-qty-value">{item.quantity}</span>
                    <button
                      className="cart-qty-btn"
                      onClick={() => updateCartQuantity(item.entityId, 1)}
                    >
                      +
                    </button>
                  </div>
                </div>
                <button
                  className="cart-item-remove"
                  onClick={() => removeFromCart(item.entityId)}
                  aria-label="Remove"
                >
                  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                    <path d="M18 6 6 18" /><path d="M6 6 18 18" />
                  </svg>
                </button>
              </div>
            ))}
          </div>

          <div className="cart-total">
            <span>Total</span>
            <span>${total.toFixed(2)}</span>
          </div>
        </>
      )}
    </div>
  )
}
