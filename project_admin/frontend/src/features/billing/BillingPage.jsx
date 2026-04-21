import { useState, useEffect, useCallback } from 'react'
import { api } from '../../shared/api/apiClient'
import Button from '../../shared/ui/Button'
import Spinner from '../../shared/ui/Spinner'
import CurrentPlanCard from './CurrentPlanCard.jsx'
import AvailablePlans from './AvailablePlans.jsx'
import PreferencesCards from './PreferencesCards.jsx'
import InvoicesTable from './InvoicesTable.jsx'
import { nextPlanId } from './plans.js'
import './billing.css'

export default function BillingPage() {
  const [overview, setOverview] = useState(null)
  const [invoices, setInvoices] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(null)

  const reload = useCallback(() => {
    setLoading(true)
    Promise.all([
      api.get('/billing/overview'),
      api.get('/billing/invoices?limit=20'),
    ])
      .then(([ov, inv]) => {
        setOverview(ov)
        setInvoices(inv.items || [])
        setError(null)
      })
      .catch((e) => setError(e.message || 'Failed to load billing'))
      .finally(() => setLoading(false))
  }, [])

  useEffect(() => { reload() }, [reload])

  async function changePlan(planId) {
    await api.post('/billing/plan', { plan: planId })
    reload()
  }

  async function savePrefs(prefs) {
    await api.post('/billing/preferences', prefs)
    setOverview((o) => o ? { ...o, preferences: prefs } : o)
  }

  if (loading && !overview) {
    return (
      <div className="billing-loading">
        <Spinner />
      </div>
    )
  }
  if (error) {
    return <div className="billing-error">{error}</div>
  }
  if (!overview) return null

  const upgradeTarget = nextPlanId(overview.plan.id, overview.availablePlans)

  return (
    <div className="billing-page">
      <div className="page-header">
        <div>
          <h1 className="page-title">Subscription</h1>
          <div className="page-subtitle">Manage your plan and usage</div>
        </div>
        {upgradeTarget && (
          <Button variant="primary" pill onClick={() => changePlan(upgradeTarget)}>
            Upgrade plan
          </Button>
        )}
      </div>

      <CurrentPlanCard overview={overview} />

      <AvailablePlans
        currentPlanId={overview.plan.id}
        plans={overview.availablePlans}
        onSelect={changePlan}
      />

      <PreferencesCards
        preferences={overview.preferences}
        onSave={savePrefs}
      />

      <InvoicesTable invoices={invoices} />
    </div>
  )
}
