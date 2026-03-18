import { useState, useCallback, useEffect, useRef } from 'react'
import { saveActionCache, loadActionCache } from './actionCache'
import { syncAction } from '../chat/api/backgroundSync'
import { loadSessionCache } from '../chat/sessionCache'

function getSessionId() {
  try {
    return loadSessionCache()?.sessionId || null
  } catch { return null }
}

export function useActionState() {
  const [likedIds, setLikedIds] = useState(() => {
    const cached = loadActionCache()
    return cached?.likedIds || new Set()
  })

  const [cartItems, setCartItems] = useState(() => {
    const cached = loadActionCache()
    return cached?.cartItems || new Map()
  })

  // Persist on every change
  const isFirstRender = useRef(true)
  useEffect(() => {
    if (isFirstRender.current) {
      isFirstRender.current = false
      return
    }
    saveActionCache({ likedIds, cartItems })
  }, [likedIds, cartItems])

  const toggleLike = useCallback((id) => {
    setLikedIds(prev => {
      const next = new Set(prev)
      if (next.has(id)) {
        next.delete(id)
      } else {
        next.add(id)
      }
      return next
    })
    // Fire-and-forget sync to backend
    const sid = getSessionId()
    if (sid) syncAction(sid, 'toggle_like', 'product', id)
  }, [])

  const isLiked = useCallback((id) => likedIds.has(id), [likedIds])

  const addToCart = useCallback((entityType, entityId) => {
    setCartItems(prev => {
      const next = new Map(prev)
      const existing = next.get(entityId)
      if (existing) {
        next.set(entityId, { ...existing, quantity: existing.quantity + 1 })
      } else {
        next.set(entityId, { entityType, entityId, quantity: 1 })
      }
      return next
    })
    // Fire-and-forget sync to backend
    const sid = getSessionId()
    if (sid) syncAction(sid, 'add_to_cart', entityType, entityId)
  }, [])

  const removeFromCart = useCallback((id) => {
    setCartItems(prev => {
      const next = new Map(prev)
      next.delete(id)
      return next
    })
    // Fire-and-forget sync to backend
    const sid = getSessionId()
    if (sid) syncAction(sid, 'remove_from_cart', 'product', id)
  }, [])

  const updateCartQuantity = useCallback((id, delta) => {
    setCartItems(prev => {
      const next = new Map(prev)
      const item = next.get(id)
      if (!item) return prev
      const newQty = item.quantity + delta
      if (newQty <= 0) {
        next.delete(id)
      } else {
        next.set(id, { ...item, quantity: newQty })
      }
      return next
    })
    // Fire-and-forget sync to backend
    const sid = getSessionId()
    if (sid) syncAction(sid, 'update_cart_qty', 'product', id)
  }, [])

  const getCartQuantity = useCallback((id) => {
    return cartItems.get(id)?.quantity || 0
  }, [cartItems])

  const likedCount = likedIds.size
  const cartCount = [...cartItems.values()].reduce((sum, item) => sum + item.quantity, 0)

  return {
    likedIds,
    cartItems,
    toggleLike,
    isLiked,
    addToCart,
    removeFromCart,
    updateCartQuantity,
    getCartQuantity,
    likedCount,
    cartCount,
  }
}
