import { Zap } from 'lucide-react'
import Button from '../../shared/ui/Button'
import { actionForPlan, formatNumber } from './plans.js'

export default function AvailablePlans({ currentPlanId, plans, onSelect }) {
  return (
    <section className="plans">
      <header className="plans-header">
        <h2 className="plans-title">Available Plans</h2>
        <div className="plans-subtitle">Switch instantly, changes apply next cycle</div>
      </header>

      <div className="plans-list">
        {plans.map((p) => (
          <PlanRow
            key={p.id}
            plan={p}
            current={p.id === currentPlanId}
            action={actionForPlan(currentPlanId, p.id, plans)}
            onSelect={() => onSelect(p.id)}
          />
        ))}
      </div>
    </section>
  )
}

function PlanRow({ plan, current, action, onSelect }) {
  return (
    <div className={`plan-row ${current ? 'plan-row-current' : ''}`}>
      <div className="plan-row-top">
        <div className="plan-row-label">
          <Zap size={16} className="plan-row-icon" />
          <span className="plan-row-name">{plan.name}</span>
          <span className="plan-row-sep">·</span>
          <span className="plan-row-tokens">{formatNumber(plan.tokensPerMonth)} tokens / mo</span>
          {current && <span className="status-badge status-active">Active</span>}
        </div>
        <Button
          variant={current ? 'secondary' : 'primary'}
          pill
          disabled={current}
          onClick={onSelect}
        >
          {action}
        </Button>
      </div>

      {current && (
        <div className="plan-row-details">
          <DetailCell label="Conversations" value={plan.conversations} />
          <DetailCell label="Products" value={plan.products} />
          <DetailCell label="Seats" value={plan.seats} />
          <DetailCell label="Support" value={plan.support} />
        </div>
      )}
    </div>
  )
}

function DetailCell({ label, value }) {
  return (
    <div className="cp-detail">
      <div className="cp-detail-label">{label}</div>
      <div className="cp-detail-value">{value}</div>
    </div>
  )
}
