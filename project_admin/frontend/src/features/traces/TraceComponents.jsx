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

const SPAN_LABELS = {
  'pipeline': 'Пайплайн (общее)',
  'agent1': 'Агент 1 — Анализ запроса',
  'agent1.llm': 'Агент 1 — LLM вызов',
  'agent1.llm.ttfb': 'Агент 1 — До первого токена',
  'agent1.llm.body': 'Агент 1 — Чтение ответа',
  'agent1.tool': 'Агент 1 — Инструмент',
  'agent1.state': 'Агент 1 — Обновление стейта',
  'agent2': 'Агент 2 — Рендеринг',
  'agent2.llm': 'Агент 2 — LLM вызов',
  'agent2.tool': 'Агент 2 — Visual Assembly',
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
            <span className="trace-waterfall-label">{SPAN_LABELS[span.name] || span.name}</span>
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

export function CacheStatus({ agent }) {
  if (!agent) return null
  const totalInput = (agent.inputTokens || 0) + (agent.cacheRead || 0) + (agent.cacheWrite || 0)
  const hitRate = totalInput > 0 ? ((agent.cacheRead || 0) / totalInput * 100).toFixed(0) : 0
  return (
    <div className="trace-cache-status">
      <div className="trace-cache-row">
        <span className="trace-cache-label">System prompt:</span>
        <span className="trace-mono">{agent.systemPromptChars?.toLocaleString()} символов (~{Math.ceil((agent.systemPromptChars || 0) / 4).toLocaleString()} токенов)</span>
      </div>
      <div className="trace-cache-row">
        <span className="trace-cache-label">Кэш:</span>
        {agent.cacheRead > 0 && (
          <Tag color="green">HIT {hitRate}%: {agent.cacheRead.toLocaleString()} токенов из кэша</Tag>
        )}
        {agent.cacheWrite > 0 && (
          <Tag color="amber">MISS: запись {agent.cacheWrite.toLocaleString()} токенов</Tag>
        )}
        {!agent.cacheRead && !agent.cacheWrite && (
          <Tag color="gray">Без кэша (read=0, write=0)</Tag>
        )}
      </div>
      <div className="trace-cache-row trace-cache-debug">
        <span className="trace-cache-label">Токены:</span>
        <span className="trace-mono">
          input={agent.inputTokens || 0} · output={agent.outputTokens || 0} · cache_read={agent.cacheRead || 0} · cache_write={agent.cacheWrite || 0}
        </span>
      </div>
      <JsonBlock data={agent.systemPrompt} label="System Prompt" />
    </div>
  )
}

const MODEL_PRICING = {
  'claude-haiku-4-5-20251001': { input: 1.0, output: 5.0, name: 'Haiku' },
  'claude-sonnet-4-5-20251014': { input: 3.0, output: 15.0, name: 'Sonnet' },
}

function getPricing(model) {
  if (!model) return { input: 1.0, output: 5.0, name: '?' }
  return MODEL_PRICING[model] || { input: 1.0, output: 5.0, name: model.replace('claude-', '') }
}

export function TokenCostTable({ agent }) {
  if (!agent) return null
  const p = getPricing(agent.model)
  const inputCost = (agent.inputTokens || 0) * p.input / 1_000_000
  const outputCost = (agent.outputTokens || 0) * p.output / 1_000_000
  const cacheReadCost = (agent.cacheRead || 0) * p.input * 0.1 / 1_000_000
  const cacheWriteCost = (agent.cacheWrite || 0) * p.input * 1.25 / 1_000_000
  const cacheReadSavings = (agent.cacheRead || 0) * p.input * 0.9 / 1_000_000

  return (
    <table className="trace-token-table">
      <thead>
        <tr><th>Категория</th><th>Токены</th><th>Стоимость</th></tr>
      </thead>
      <tbody>
        <tr>
          <td>Input</td>
          <td className="trace-mono">{(agent.inputTokens || 0).toLocaleString()}</td>
          <td className="trace-mono">${inputCost.toFixed(6)}</td>
        </tr>
        <tr>
          <td>Output</td>
          <td className="trace-mono">{(agent.outputTokens || 0).toLocaleString()}</td>
          <td className="trace-mono">${outputCost.toFixed(6)}</td>
        </tr>
        {agent.cacheRead > 0 && (
          <tr className="trace-token-row--cache">
            <td>Cache read (0.1x)</td>
            <td className="trace-mono">{agent.cacheRead.toLocaleString()}</td>
            <td className="trace-mono">${cacheReadCost.toFixed(6)} <span className="trace-savings">(-${cacheReadSavings.toFixed(6)})</span></td>
          </tr>
        )}
        {agent.cacheWrite > 0 && (
          <tr className="trace-token-row--cache">
            <td>Cache write (1.25x)</td>
            <td className="trace-mono">{agent.cacheWrite.toLocaleString()}</td>
            <td className="trace-mono">${cacheWriteCost.toFixed(6)}</td>
          </tr>
        )}
        <tr className="trace-token-row--total">
          <td><strong>Итого</strong></td>
          <td></td>
          <td className="trace-mono"><strong>${agent.costUsd?.toFixed(6)}</strong></td>
        </tr>
      </tbody>
    </table>
  )
}

export function StateDetails({ snapshot }) {
  if (!snapshot) return null
  return (
    <div className="trace-state-details">
      {snapshot.meta && (
        <div className="trace-state-row">
          <span className="trace-state-label">Мета:</span>
          <span className="trace-mono">{snapshot.meta.count} items</span>
          {snapshot.meta.fields?.length > 0 && (
            <span className="trace-state-fields">{snapshot.meta.fields.join(', ')}</span>
          )}
        </div>
      )}
      <div className="trace-state-row">
        <span className="trace-state-label">Вью:</span>
        <Tag>{snapshot.viewMode || 'default'}</Tag>
        {snapshot.viewFocused && (
          <span className="trace-mono">{snapshot.viewFocused.type}: {snapshot.viewFocused.id?.substring(0, 12)}</span>
        )}
      </div>
      {snapshot.viewStackLen > 0 && (
        <div className="trace-state-row">
          <span className="trace-state-label">Стек навигации:</span>
          <span className="trace-mono">{snapshot.viewStackLen} уровней</span>
        </div>
      )}
      {(snapshot.likedCount > 0 || snapshot.cartCount > 0) && (
        <div className="trace-state-row">
          <span className="trace-state-label">Действия:</span>
          {snapshot.likedCount > 0 && <Tag color="red">{snapshot.likedCount} liked</Tag>}
          {snapshot.cartCount > 0 && <Tag color="green">{snapshot.cartCount} в корзине</Tag>}
        </div>
      )}
      {snapshot.templateSummary && (
        <div className="trace-state-row">
          <span className="trace-state-label">Шаблон:</span>
          <Tag color="purple">{snapshot.templateSummary}</Tag>
        </div>
      )}
    </div>
  )
}
