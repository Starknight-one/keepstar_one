// PendingPage — curator UI for the pending-approval queue introduced by
// the 2026-05-20 bundled milestone. Left rail: master_categories tree
// with pending counts. Right pane: master cards in the chosen category
// + bulk approve/reject. Card click opens a drawer with per-change
// checkboxes (granular approve for individual enrichment rows).
//
// MVP scope: no diff view, no full search, no filters beyond vertical.
import { useEffect, useMemo, useState } from 'react'
import { pendingApi } from '../api.js'

const VERTICALS = ['cosmetics', 'electronics', 'furniture', 'haircare', 'apparel', 'footwear', 'food', 'ski', 'unknown']

export default function PendingPage() {
  const [vertical, setVertical] = useState('cosmetics')
  const [tree, setTree] = useState([])
  const [selectedCategoryId, setSelectedCategoryId] = useState(null)
  const [cards, setCards] = useState([])
  const [cardsTotal, setCardsTotal] = useState(0)
  const [selectedMasterIds, setSelectedMasterIds] = useState(new Set())
  const [drawerMaster, setDrawerMaster] = useState(null)
  const [loadingTree, setLoadingTree] = useState(false)
  const [loadingCards, setLoadingCards] = useState(false)
  const [error, setError] = useState(null)

  // Load tree on vertical change.
  useEffect(() => {
    let cancelled = false
    setLoadingTree(true)
    pendingApi.tree(vertical)
      .then(res => { if (!cancelled) { setTree(res.categories || []); setError(null) } })
      .catch(err => { if (!cancelled) setError(err.message) })
      .finally(() => { if (!cancelled) setLoadingTree(false) })
    return () => { cancelled = true }
  }, [vertical])

  // Load cards on category select.
  useEffect(() => {
    if (!selectedCategoryId) {
      setCards([]); setCardsTotal(0); setSelectedMasterIds(new Set())
      return
    }
    let cancelled = false
    setLoadingCards(true)
    pendingApi.category(selectedCategoryId)
      .then(res => {
        if (!cancelled) {
          setCards(res.cards || [])
          setCardsTotal(res.total || 0)
          setSelectedMasterIds(new Set())
          setError(null)
        }
      })
      .catch(err => { if (!cancelled) setError(err.message) })
      .finally(() => { if (!cancelled) setLoadingCards(false) })
    return () => { cancelled = true }
  }, [selectedCategoryId])

  // Reload card list after an approve/reject so counts update.
  async function refreshCards() {
    if (!selectedCategoryId) return
    const res = await pendingApi.category(selectedCategoryId)
    setCards(res.cards || [])
    setCardsTotal(res.total || 0)
    setSelectedMasterIds(new Set())
    // Tree counts also need refresh.
    const tr = await pendingApi.tree(vertical)
    setTree(tr.categories || [])
  }

  const visibleCategories = useMemo(
    () => tree.filter(n => n.pending_master_count + n.pending_change_count > 0),
    [tree]
  )

  function toggleMaster(id) {
    setSelectedMasterIds(prev => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id); else next.add(id)
      return next
    })
  }

  function toggleAll() {
    if (selectedMasterIds.size === cards.length) {
      setSelectedMasterIds(new Set())
    } else {
      setSelectedMasterIds(new Set(cards.map(c => c.master_id)))
    }
  }

  async function bulkApprove() {
    const ids = [...selectedMasterIds]
    if (ids.length === 0) return
    try {
      const res = await pendingApi.approve({ masterIds: ids })
      await refreshCards()
      setError(null)
      console.log('approved', res)
    } catch (err) {
      setError(err.message)
    }
  }

  async function bulkReject() {
    const ids = [...selectedMasterIds]
    if (ids.length === 0) return
    const reason = window.prompt('Reject reason (optional)') || ''
    try {
      const res = await pendingApi.reject({ masterIds: ids, reason })
      await refreshCards()
      setError(null)
      console.log('rejected', res)
    } catch (err) {
      setError(err.message)
    }
  }

  return (
    <div className="pending-page">
      <header className="page-header">
        <h1>Pending Approval</h1>
        <label>
          Vertical:
          <select value={vertical} onChange={e => { setVertical(e.target.value); setSelectedCategoryId(null) }}>
            {VERTICALS.map(v => <option key={v} value={v}>{v}</option>)}
          </select>
        </label>
      </header>

      {error && <div className="error">{error}</div>}

      <div className="pending-layout">
        <aside className="pending-tree">
          <h3>Categories</h3>
          {loadingTree && <div>Loading…</div>}
          {!loadingTree && visibleCategories.length === 0 && (
            <div className="empty">Nothing pending in {vertical}.</div>
          )}
          <ul>
            {visibleCategories.map(node => (
              <li
                key={node.category_id}
                className={node.category_id === selectedCategoryId ? 'active' : ''}
                onClick={() => setSelectedCategoryId(node.category_id)}
              >
                <div className="cat-name">{node.category_name}</div>
                <div className="cat-counts">
                  {node.pending_master_count > 0 && <span className="badge badge-new">{node.pending_master_count} new</span>}
                  {node.pending_change_count > 0 && <span className="badge badge-enrich">+{node.pending_change_count}</span>}
                </div>
              </li>
            ))}
          </ul>
        </aside>

        <section className="pending-cards">
          {!selectedCategoryId && <div className="empty">Pick a category on the left.</div>}
          {selectedCategoryId && (
            <>
              <div className="bulk-bar">
                <label>
                  <input
                    type="checkbox"
                    checked={cards.length > 0 && selectedMasterIds.size === cards.length}
                    onChange={toggleAll}
                  />
                  {' '}Select all ({cards.length})
                </label>
                <span className="bulk-actions">
                  <button onClick={bulkApprove} disabled={selectedMasterIds.size === 0}>
                    Approve selected ({selectedMasterIds.size})
                  </button>
                  <button onClick={bulkReject} disabled={selectedMasterIds.size === 0}>
                    Reject selected ({selectedMasterIds.size})
                  </button>
                </span>
                <span className="total">total {cardsTotal}</span>
              </div>

              {loadingCards && <div>Loading…</div>}
              {!loadingCards && cards.length === 0 && (
                <div className="empty">Nothing pending in this category.</div>
              )}

              <ul className="cards">
                {cards.map(card => (
                  <li key={card.master_id} className="card">
                    <label className="card-select">
                      <input
                        type="checkbox"
                        checked={selectedMasterIds.has(card.master_id)}
                        onChange={() => toggleMaster(card.master_id)}
                      />
                    </label>
                    <div className="card-body" onClick={() => setDrawerMaster(card.master_id)}>
                      <div className="card-title">{card.name}</div>
                      <div className="card-meta">
                        {card.brand && <span className="brand">{card.brand}</span>}
                        {card.sku && <span className="sku">{card.sku}</span>}
                      </div>
                      <div className="card-badges">
                        {card.is_new_master && <span className="badge badge-new">new master</span>}
                        {card.pending_change_count > 0 && <span className="badge badge-enrich">+{card.pending_change_count} enrich</span>}
                        {card.pending_expires_at && <span className="expires">expires {new Date(card.pending_expires_at).toLocaleDateString()}</span>}
                      </div>
                    </div>
                  </li>
                ))}
              </ul>
            </>
          )}
        </section>
      </div>

      {drawerMaster && (
        <PendingMasterDrawer
          masterId={drawerMaster}
          onClose={() => setDrawerMaster(null)}
          onChanged={refreshCards}
        />
      )}
    </div>
  )
}

function PendingMasterDrawer({ masterId, onClose, onChanged }) {
  const [detail, setDetail] = useState(null)
  const [selectedChangeIds, setSelectedChangeIds] = useState(new Set())
  const [err, setErr] = useState(null)

  useEffect(() => {
    let cancelled = false
    pendingApi.master(masterId)
      .then(d => { if (!cancelled) setDetail(d) })
      .catch(e => { if (!cancelled) setErr(e.message) })
    return () => { cancelled = true }
  }, [masterId])

  function toggleChange(id) {
    setSelectedChangeIds(prev => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id); else next.add(id)
      return next
    })
  }

  async function approveChanges() {
    const ids = [...selectedChangeIds]
    if (ids.length === 0) return
    try {
      await pendingApi.approve({ changeIds: ids })
      // Reload drawer + parent.
      const d = await pendingApi.master(masterId)
      setDetail(d)
      setSelectedChangeIds(new Set())
      onChanged && onChanged()
    } catch (e) { setErr(e.message) }
  }

  async function rejectChanges() {
    const ids = [...selectedChangeIds]
    if (ids.length === 0) return
    const reason = window.prompt('Reject reason (optional)') || ''
    try {
      await pendingApi.reject({ changeIds: ids, reason })
      const d = await pendingApi.master(masterId)
      setDetail(d)
      setSelectedChangeIds(new Set())
      onChanged && onChanged()
    } catch (e) { setErr(e.message) }
  }

  const pendingChanges = (detail?.changes || []).filter(c => c.status === 'pending')

  return (
    <div className="drawer-overlay" onClick={onClose}>
      <div className="drawer" onClick={e => e.stopPropagation()}>
        <header>
          <h2>{detail?.card?.name || 'Loading…'}</h2>
          <button onClick={onClose}>×</button>
        </header>
        {err && <div className="error">{err}</div>}
        {detail && (
          <>
            <div className="drawer-meta">
              <div>SKU: {detail.card.sku || '—'}</div>
              <div>Brand: {detail.card.brand || '—'}</div>
              <div>Vertical: {detail.card.vertical}</div>
              <div>Status: {detail.card.approval_status}</div>
            </div>

            <h3>Pending changes ({pendingChanges.length})</h3>
            {pendingChanges.length === 0 && <div className="empty">No pending changes.</div>}
            {pendingChanges.length > 0 && (
              <>
                <div className="drawer-actions">
                  <button onClick={approveChanges} disabled={selectedChangeIds.size === 0}>
                    Approve selected ({selectedChangeIds.size})
                  </button>
                  <button onClick={rejectChanges} disabled={selectedChangeIds.size === 0}>
                    Reject selected ({selectedChangeIds.size})
                  </button>
                </div>
                <ul className="change-list">
                  {pendingChanges.map(c => (
                    <li key={c.id}>
                      <input
                        type="checkbox"
                        checked={selectedChangeIds.has(c.id)}
                        onChange={() => toggleChange(c.id)}
                      />
                      <div className="change-body">
                        <div className="change-field">{c.field_name} <span className="op">({c.op_kind})</span></div>
                        <div className="change-value">{c.pending_value}</div>
                      </div>
                    </li>
                  ))}
                </ul>
              </>
            )}
          </>
        )}
      </div>
    </div>
  )
}
