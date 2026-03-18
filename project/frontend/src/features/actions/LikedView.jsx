import { useMemo, useState, useEffect } from 'react'
import { useActions } from './ActionContext'
import { FormationRenderer } from '../../entities/formation/FormationRenderer'
import { fillFormation } from '../chat/model/fillFormation'
import { getActionView } from '../../shared/api/apiClient'
import { loadSessionCache } from '../chat/sessionCache'
import { log } from '../../shared/logger'

export function LikedView({ adjacentTemplates, entities, onWidgetClick, onBack }) {
  const { likedIds } = useActions()
  const [backendFormation, setBackendFormation] = useState(null)

  // Optimistic local formation (instant)
  const localFormation = useMemo(() => {
    if (likedIds.size === 0 || !entities) return null

    const products = entities.products || []
    const services = entities.services || []

    const widgets = []

    for (const id of likedIds) {
      // Try products first
      const product = products.find(p => p.id === id)
      if (product && adjacentTemplates?.product) {
        const filled = fillFormation(adjacentTemplates.product, product, 'product')
        if (filled?.widgets?.[0]) {
          widgets.push(filled.widgets[0])
          continue
        }
      }
      // Try services
      const service = services.find(s => s.id === id)
      if (service && adjacentTemplates?.service) {
        const filled = fillFormation(adjacentTemplates.service, service, 'service')
        if (filled?.widgets?.[0]) {
          widgets.push(filled.widgets[0])
        }
      }
    }

    if (widgets.length === 0) return null

    return {
      mode: 'grid',
      grid: { cols: 2 },
      widgets,
    }
  }, [likedIds, entities, adjacentTemplates])

  // Fetch backend formation on mount
  useEffect(() => {
    if (likedIds.size === 0) return
    const sessionId = loadSessionCache()?.sessionId
    if (!sessionId) return

    getActionView(sessionId, 'liked')
      .then(resp => {
        if (resp.success && resp.formation) {
          setBackendFormation(resp.formation)
        }
      })
      .catch(err => log.warn('LikedView backend fetch failed:', err))
  }, [likedIds.size])

  // Prefer backend formation, fallback to local
  const formation = backendFormation || localFormation

  return (
    <div>
      <div className="liked-view-header">
        <button className="cart-view-back" onClick={onBack} aria-label="Back">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <polyline points="15 18 9 12 15 6" />
          </svg>
        </button>
        <h2>Liked ({likedIds.size})</h2>
      </div>

      {formation ? (
        <FormationRenderer formation={formation} onWidgetClick={onWidgetClick} />
      ) : (
        <div className="liked-empty">
          <span className="liked-empty-icon">&#9825;</span>
          <span>No liked items yet</span>
        </div>
      )}
    </div>
  )
}
