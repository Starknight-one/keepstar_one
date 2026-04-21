export function planIndex(planId, plans) {
  return plans.findIndex((p) => p.id === planId)
}

export function nextPlanId(currentId, plans) {
  const i = planIndex(currentId, plans)
  if (i < 0 || i >= plans.length - 1) return null
  return plans[i + 1].id
}

export function actionForPlan(currentId, planId, plans) {
  const cur = planIndex(currentId, plans)
  const tgt = planIndex(planId, plans)
  if (cur < 0 || tgt < 0) return 'Change'
  if (cur === tgt) return 'Current plan'
  return tgt > cur ? 'Upgrade' : 'Downgrade'
}

export function formatNumber(n) {
  if (n == null) return '—'
  return n.toLocaleString('en-US')
}

export function formatDate(iso) {
  if (!iso) return '—'
  const d = new Date(iso)
  return d.toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })
}

export function formatPeriod(start, end) {
  if (!start || !end) return '—'
  const s = new Date(start)
  const e = new Date(end)
  const sameYear = s.getFullYear() === e.getFullYear()
  const opts = { month: 'short', day: 'numeric' }
  const sStr = s.toLocaleDateString('en-US', opts)
  const eStr = e.toLocaleDateString('en-US', sameYear
    ? { ...opts, year: 'numeric' }
    : { ...opts, year: 'numeric' })
  return `${sStr} – ${eStr}`
}
