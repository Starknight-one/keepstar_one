const CACHE_KEY = 'actionStateCache'
const TTL_MS = 24 * 60 * 60 * 1000 // 24 hours

export function saveActionCache({ likedIds, cartItems }) {
  try {
    const data = {
      likedIds: [...likedIds],
      cartItems: [...cartItems.entries()],
      timestamp: Date.now(),
    }
    localStorage.setItem(CACHE_KEY, JSON.stringify(data))
  } catch (_) { /* quota exceeded — ignore */ }
}

export function loadActionCache() {
  try {
    const raw = localStorage.getItem(CACHE_KEY)
    if (!raw) return null

    const data = JSON.parse(raw)
    if (Date.now() - data.timestamp > TTL_MS) {
      localStorage.removeItem(CACHE_KEY)
      return null
    }

    return {
      likedIds: new Set(data.likedIds || []),
      cartItems: new Map(data.cartItems || []),
    }
  } catch (_) {
    return null
  }
}

export function clearActionCache() {
  localStorage.removeItem(CACHE_KEY)
}
