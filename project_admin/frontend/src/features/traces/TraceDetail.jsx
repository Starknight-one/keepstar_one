import { useState, useEffect } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { api } from '../../shared/api/apiClient.js'
import Spinner from '../../shared/ui/Spinner.jsx'
import './traces.css'

function Section({ title, defaultOpen = false, children }) {
  const [open, setOpen] = useState(defaultOpen)
  return (
    <div className="trace-section">
      <div className="trace-section-header" onClick={() => setOpen(!open)}>
        <span className="trace-section-arrow">{open ? '▾' : '▸'}</span>
        <span className="trace-section-title">{title}</span>
      </div>
      {open && <div className="trace-section-body">{children}</div>}
    </div>
  )
}

function KV({ label, value }) {
  if (value === undefined || value === null || value === '') return null
  return (
    <div className="trace-kv">
      <span className="trace-kv-label">{label}</span>
      <span className="trace-kv-value">{typeof value === 'number' ? value : String(value)}</span>
    </div>
  )
}

function JsonBlock({ data, label }) {
  const [expanded, setExpanded] = useState(false)
  if (!data) return null
  const str = typeof data === 'string' ? data : JSON.stringify(data, null, 2)
  const isLong = str.length > 500
  return (
    <div className="trace-json-block">
      {label && <div className="trace-json-label">{label}</div>}
      <pre className="trace-json">{isLong && !expanded ? str.substring(0, 500) + '…' : str}</pre>
      {isLong && (
        <button className="trace-json-toggle" onClick={() => setExpanded(!expanded)}>
          {expanded ? 'Collapse' : 'Expand'} ({str.length} chars)
        </button>
      )}
    </div>
  )
}

function TokensRow({ agent }) {
  if (!agent) return null
  return (
    <div className="trace-tokens">
      <span>Input: {agent.inputTokens}</span>
      <span>Output: {agent.outputTokens}</span>
      {agent.cacheRead > 0 && <span>Cache Read: {agent.cacheRead}</span>}
      {agent.cacheWrite > 0 && <span>Cache Write: {agent.cacheWrite}</span>}
      <span>Cost: ${agent.costUsd?.toFixed(4)}</span>
    </div>
  )
}

function WaterfallBar({ spans, totalMs }) {
  if (!spans || spans.length === 0) return null
  return (
    <div className="trace-waterfall">
      {spans.map((span, i) => {
        const leftPct = totalMs > 0 ? (span.startMs / totalMs) * 100 : 0
        const widthPct = totalMs > 0 ? (span.durationMs / totalMs) * 100 : 0
        return (
          <div key={i} className="trace-waterfall-row">
            <span className="trace-waterfall-label">{span.name}</span>
            <div className="trace-waterfall-bar-container">
              <div
                className="trace-waterfall-bar"
                style={{ left: `${leftPct}%`, width: `${Math.max(widthPct, 1)}%` }}
              />
            </div>
            <span className="trace-waterfall-ms">{span.durationMs}ms</span>
          </div>
        )
      })}
    </div>
  )
}

export default function TraceDetail() {
  const { id } = useParams()
  const navigate = useNavigate()
  const [trace, setTrace] = useState(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    api.get(`/traces/${id}`)
      .then((data) => setTrace(data))
      .catch(() => navigate('/traces'))
      .finally(() => setLoading(false))
  }, [id, navigate])

  if (loading) return <div className="center-spinner"><Spinner /></div>
  if (!trace) return null

  const td = typeof trace.traceData === 'string' ? JSON.parse(trace.traceData) : trace.traceData
  const agent1 = td?.agent1
  const agent2 = td?.agent2
  const stateAfter = td?.stateAfterAgent1
  const formation = td?.formationResult
  const spans = td?.spans

  return (
    <div className="trace-detail">
      <button className="btn btn-secondary trace-back" onClick={() => navigate('/traces')}>
        ← Back to Traces
      </button>

      <Section title="Summary" defaultOpen={true}>
        <div className="trace-summary-grid">
          <KV label="Query" value={trace.query} />
          <KV label="Session" value={trace.sessionId} />
          <KV label="Total" value={`${trace.totalMs}ms`} />
          <KV label="Cost" value={`$${trace.costUsd?.toFixed(4)}`} />
          {trace.error && <KV label="Error" value={trace.error} />}
          <KV label="Timestamp" value={new Date(trace.timestamp).toLocaleString()} />
        </div>
      </Section>

      <Section title={`Agent1 — ${agent1?.model || '?'}`} defaultOpen={true}>
        <div className="trace-agent-grid">
          <KV label="LLM Time" value={`${agent1?.llmMs}ms`} />
          <KV label="Tool Time" value={agent1?.toolMs ? `${agent1.toolMs}ms` : '—'} />
          <KV label="Total" value={`${agent1?.totalMs}ms`} />
          <KV label="Stop Reason" value={agent1?.stopReason} />
          <KV label="System Prompt" value={agent1?.systemPromptChars ? `${agent1.systemPromptChars} chars` : '—'} />
          <KV label="Messages" value={agent1?.messageCount} />
          <KV label="Tool Defs" value={agent1?.toolDefCount} />
        </div>
        <TokensRow agent={agent1} />
        <KV label="Tool" value={agent1?.toolName} />
        <JsonBlock data={agent1?.toolInput} label="Tool Input" />
        <JsonBlock data={agent1?.toolResult} label="Tool Result" />
        <JsonBlock data={agent1?.toolBreakdown} label="Tool Breakdown" />
        <JsonBlock data={agent1?.enrichedQuery} label="Enriched Query" />
        <JsonBlock data={agent1?.systemPrompt} label="System Prompt" />
      </Section>

      <Section title={`State after Agent1 — ${stateAfter?.productCount || 0} products, ${stateAfter?.serviceCount || 0} services`}>
        <div className="trace-agent-grid">
          <KV label="Products" value={stateAfter?.productCount} />
          <KV label="Services" value={stateAfter?.serviceCount} />
          <KV label="Fields" value={stateAfter?.fields?.join(', ')} />
          <KV label="Has Template" value={stateAfter?.hasTemplate ? 'yes' : 'no'} />
          <KV label="Deltas" value={stateAfter?.deltaCount} />
        </div>
        {stateAfter?.deltas && stateAfter.deltas.length > 0 && (
          <table className="trace-deltas-table">
            <thead>
              <tr>
                <th>Step</th>
                <th>Actor</th>
                <th>Type</th>
                <th>Path</th>
                <th>Tool</th>
                <th>Count</th>
              </tr>
            </thead>
            <tbody>
              {stateAfter.deltas.map((d, i) => (
                <tr key={i}>
                  <td>{d.step}</td>
                  <td>{d.actorId}</td>
                  <td>{d.deltaType}</td>
                  <td>{d.path}</td>
                  <td>{d.tool || '—'}</td>
                  <td>{d.count || '—'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Section>

      <Section title={`Agent2 — ${agent2?.model || '?'}`} defaultOpen={true}>
        <div className="trace-agent-grid">
          <KV label="LLM Time" value={`${agent2?.llmMs}ms`} />
          <KV label="Total" value={`${agent2?.totalMs}ms`} />
          <KV label="System Prompt" value={agent2?.systemPromptChars ? `${agent2.systemPromptChars} chars` : '—'} />
          <KV label="Messages" value={agent2?.messageCount} />
          <KV label="Tool Defs" value={agent2?.toolDefCount} />
        </div>
        <TokensRow agent={agent2} />
        <KV label="Tool" value={agent2?.toolName} />
        <JsonBlock data={agent2?.toolInput} label="Tool Input (visual_assembly params)" />
        <JsonBlock data={agent2?.toolBreakdown} label="Engine Breakdown" />
        <JsonBlock data={agent2?.promptSent} label="Prompt Sent" />
        <JsonBlock data={agent2?.rawResponse} label="Raw Response" />
        <JsonBlock data={agent2?.systemPrompt} label="System Prompt" />
      </Section>

      <Section title={`Formation — ${formation?.mode || '?'} (${formation?.widgetCount || 0} widgets)`} defaultOpen={true}>
        <div className="trace-agent-grid">
          <KV label="Mode" value={formation?.mode} />
          <KV label="Widget Count" value={formation?.widgetCount} />
          <KV label="Cols" value={formation?.cols} />
          <KV label="First Widget" value={formation?.firstWidget} />
        </div>
        {formation?.widgets && formation.widgets.length > 0 && (
          <table className="trace-deltas-table">
            <thead>
              <tr>
                <th>ID</th>
                <th>Template</th>
                <th>Size</th>
                <th>Atoms</th>
                <th>Entity Ref</th>
              </tr>
            </thead>
            <tbody>
              {formation.widgets.map((w, i) => (
                <tr key={i}>
                  <td className="trace-mono">{w.id?.substring(0, 8)}</td>
                  <td>{w.template}</td>
                  <td>{w.size || '—'}</td>
                  <td>{w.atomCount}</td>
                  <td className="trace-mono">{w.entityRef ? w.entityRef.substring(0, 8) : '—'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
        <JsonBlock data={formation?.fullJson} label="Full Formation JSON" />
      </Section>

      <Section title="Waterfall">
        <WaterfallBar spans={spans} totalMs={trace.totalMs} />
      </Section>
    </div>
  )
}
