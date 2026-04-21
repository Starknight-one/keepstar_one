import { formatNumber, formatDate } from './plans.js'

export default function CurrentPlanCard({ overview }) {
  const { plan, subscription, usage } = overview
  const pct = usage.tokensQuota > 0
    ? Math.min(100, Math.round((usage.tokensUsed / usage.tokensQuota) * 100))
    : 0
  const renewsStr = formatDate(subscription.renewsAt)

  return (
    <section className="card current-plan">
      <div className="cp-row">
        <div className="cp-head">
          <div className="cp-eyebrow">CURRENT PLAN</div>
          <div className="cp-plan-name">{plan.name}</div>
          <div className="cp-plan-meta">
            {formatNumber(plan.tokensPerMonth)} tokens / month · renews {renewsStr}
          </div>
        </div>
        <span className={`status-badge status-${subscription.status === 'active' ? 'active' : 'done'}`}>
          {capitalize(subscription.status)}
        </span>
      </div>

      <div className="cp-usage">
        <div className="cp-usage-label">Token usage this cycle</div>
        <div className="cp-usage-row">
          <div className="cp-usage-value">
            <span className="cp-usage-current">{formatNumber(usage.tokensUsed)}</span>
            <span className="cp-usage-quota">/ {formatNumber(usage.tokensQuota)}</span>
          </div>
          <div className="cp-usage-bar">
            <div className="cp-usage-fill" style={{ width: `${pct}%` }} />
          </div>
        </div>
      </div>

      <div className="cp-details">
        <DetailCell label="Conversations" value={plan.conversations} />
        <DetailCell label="Products" value={plan.products} />
        <DetailCell label="Seats" value={plan.seats} />
        <DetailCell label="Support" value={plan.support} />
      </div>
    </section>
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

function capitalize(s) {
  if (!s) return ''
  return s[0].toUpperCase() + s.slice(1)
}
