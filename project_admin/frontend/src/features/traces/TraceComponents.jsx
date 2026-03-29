import { useState } from 'react'

export function Section({ title, badge, color = 'gray', defaultOpen = false, children }) {
  const [open, setOpen] = useState(defaultOpen)
  return (
    <div className={`trace-section trace-section--${color}`}>
      <div className="trace-section-header" onClick={() => setOpen(!open)}>
        <span className="trace-section-arrow">{open ? '▾' : '▸'}</span>
        <span className="trace-section-title">{title}</span>
        {badge && <span className={`trace-badge trace-badge--${color}`}>{badge}</span>}
      </div>
      {open && <div className="trace-section-body">{children}</div>}
    </div>
  )
}

export function MetricCard({ label, value, sub, accent }) {
  if (value === undefined || value === null) return null
  return (
    <div className={`trace-metric ${accent ? 'trace-metric--accent' : ''}`}>
      <div className="trace-metric-value">{value}</div>
      <div className="trace-metric-label">{label}</div>
      {sub && <div className="trace-metric-sub">{sub}</div>}
    </div>
  )
}

export function Tag({ children, color }) {
  return <span className={`trace-tag ${color ? `trace-tag--${color}` : ''}`}>{children}</span>
}

export function tryPrettyJson(str) {
  if (!str) return str
  try {
    const parsed = JSON.parse(str)
    return JSON.stringify(parsed, null, 2)
  } catch {
    return str
  }
}

export function JsonBlock({ data, label, defaultExpanded = false }) {
  const [expanded, setExpanded] = useState(defaultExpanded)
  if (!data) return null

  let str
  if (typeof data === 'string') {
    str = tryPrettyJson(data)
  } else {
    str = JSON.stringify(data, null, 2)
  }

  return (
    <div className="trace-json-block">
      <div className="trace-json-header" onClick={() => setExpanded(!expanded)}>
        <span className="trace-json-arrow">{expanded ? '▾' : '▸'}</span>
        <span className="trace-json-label">{label}</span>
        <span className="trace-json-size">{str.length.toLocaleString()} chars</span>
      </div>
      {expanded && <pre className="trace-json">{str}</pre>}
    </div>
  )
}

export function TokensBar({ agent }) {
  if (!agent) return null
  const total = (agent.inputTokens || 0) + (agent.outputTokens || 0)
  if (total === 0) return null
  const inputPct = (agent.inputTokens / total) * 100
  const outputPct = (agent.outputTokens / total) * 100

  return (
    <div className="trace-tokens-bar">
      <div className="trace-tokens-visual">
        <div className="trace-tokens-segment trace-tokens-input" style={{ width: `${inputPct}%` }} title={`Input: ${agent.inputTokens}`} />
        <div className="trace-tokens-segment trace-tokens-output" style={{ width: `${outputPct}%` }} title={`Output: ${agent.outputTokens}`} />
      </div>
      <div className="trace-tokens-legend">
        <span><span className="trace-dot trace-dot--input" /> Input {agent.inputTokens?.toLocaleString()}</span>
        <span><span className="trace-dot trace-dot--output" /> Output {agent.outputTokens?.toLocaleString()}</span>
        {agent.cacheRead > 0 && <span><span className="trace-dot trace-dot--cache" /> Cache read {agent.cacheRead?.toLocaleString()}</span>}
        {agent.cacheWrite > 0 && <span><span className="trace-dot trace-dot--cachewrite" /> Cache write {agent.cacheWrite?.toLocaleString()}</span>}
        <span className="trace-tokens-cost">${agent.costUsd?.toFixed(4)}</span>
      </div>
    </div>
  )
}

export function WaterfallBar({ spans, totalMs }) {
  if (!spans || spans.length === 0) return null
  const colors = ['#8B5CF6', '#3B82F6', '#10B981', '#F59E0B', '#EF4444', '#EC4899']
  return (
    <div className="trace-waterfall">
      {spans.map((span, i) => {
        const leftPct = totalMs > 0 ? (span.startMs / totalMs) * 100 : 0
        const widthPct = totalMs > 0 ? (span.durationMs / totalMs) * 100 : 0
        return (
          <div key={i} className="trace-waterfall-row">
            <span className="trace-waterfall-label">{span.name}</span>
            <div className="trace-waterfall-track">
              <div
                className="trace-waterfall-bar"
                style={{
                  left: `${leftPct}%`,
                  width: `${Math.max(widthPct, 1)}%`,
                  background: colors[i % colors.length],
                }}
              />
            </div>
            <span className="trace-waterfall-ms">{span.durationMs}ms</span>
          </div>
        )
      })}
    </div>
  )
}
